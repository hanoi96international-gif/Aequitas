package keeper

import (
	"sync/atomic"
	"time"
)

// Wohin gehen die 1,17 Sekunden, die ein Rueckfall kostet?
//
// # DIE RECHNUNG, DIE DAS AUSLOEST
//
// Nach zwei gemessenen Verbesserungen am Nachspielen (Sperre je Block
// 292 -> 199 ms, Fehler je Lauf 1.659 -> 260) blieb der Durchsatz im
// Rauschband: 3.389 / 3.686 / 3.369 TPS. Die Kette saettigt also woanders.
//
// Die Latenz sagt, wo. Auf der annehmenden Box, unter Last:
//
//	transfer_path.avg_latency_ms   158,1   <- ueber ALLE Ueberweisungen
//	transfer_phases.total_ms        44,5   <- nur der Schnellpfad
//	fast_path_pct                   89,9
//
// Daraus folgt fuer die restlichen 10,1 %:
//
//	158,1 = 0,899 x 44,5 + 0,101 x X   =>   X = 1.169 ms
//
// Ein Rueckfall kostet also gut eine Sekunde und macht damit rund drei
// Viertel der mittleren Latenz aus, obwohl er nur jede zehnte Ueberweisung
// betrifft. Und weil Durchsatz = Gleichzeitigkeit / Latenz ist, deckelt
// genau das den Durchsatz.
//
// # WARUM DAS BISHER NICHT MESSBAR WAR
//
// Der Rueckfallpfad ist die einzige grosse Komponente ohne eigene Uhr. Es
// gibt Zaehler dafuer, WIE OFT zurueckgefallen wird (fallback_gruende) und
// wie oft ein kurzes Wiederholen half (shard_retry), aber keinen einzigen
// dafuer, was danach passiert. Die 1.169 ms sind deshalb bis hier eine
// Subtraktion aus zwei Mittelwerten -- plausibel, aber nicht zerlegt.
//
// # DER VERDACHT, DEN DIESE UHREN PRUEFEN
//
// runTransferBatcher sammelt bis zu 1.000 Anfragen und uebergibt sie einer
// Goroutine, die durch parallelBatchPoolSize = 4 gedeckelt ist. Bei rund
// 340 Rueckfaellen je Sekunde und vier Plaetzen muss jeder Platz alle
// 12 ms frei werden; dauert eine Charge laenger, staut sich die
// Warteschlange und die Wartezeit VOR der Verarbeitung dominiert.
//
// Die Vier stammt aus einer Zeit, in der MaxOpenConns auf 20 stand und
// gleichzeitige Chargen echte "sorry, too many clients"-Fehler ausloesten
// (siehe parallelBatchPoolSize). Der Verbindungspool steht inzwischen auf
// 100. Die Bedingung, die die Vier begruendete, gilt also nicht mehr --
// aber ob die Vier heute bindet, ist damit noch nicht gezeigt. Genau das
// entscheidet warten_ms gegen arbeit_ms:
//
//	warten dominiert   -> der Deckel bindet, Erhoehen ist der Hebel
//	arbeit dominiert   -> der Deckel ist unschuldig, die Charge selbst ist teuer
//
// Das ist dieselbe Trennung, die beim Nachspielen aus drei Vermutungen eine
// Messung gemacht hat. Kosten: vier atomare Additionen je Charge, keine je
// Ueberweisung.
var (
	btWartenNanos  atomic.Int64 // Warten auf einen freien Platz im Semaphor
	btArbeitNanos  atomic.Int64 // die Verarbeitung der Charge selbst
	btSammelNanos  atomic.Int64 // das Sammelfenster, bis die Charge geschlossen wird
	btChargen      atomic.Int64
	btPosten       atomic.Int64
	btRueckfaelle  atomic.Int64 // Chargen, die auf den serialisierten Pfad fielen
	btWartenMaxNs  atomic.Int64
	btArbeitMaxNs  atomic.Int64
	btChargeMaxLen atomic.Int64
)

func merkeBatcherHoechstwert(z *atomic.Int64, wert int64) {
	for {
		alt := z.Load()
		if wert <= alt || z.CompareAndSwap(alt, wert) {
			return
		}
	}
}

func merkeBatcherSammeln(d time.Duration, posten int) {
	btSammelNanos.Add(int64(d))
	btPosten.Add(int64(posten))
	merkeBatcherHoechstwert(&btChargeMaxLen, int64(posten))
}

func merkeBatcherWarten(d time.Duration) {
	btWartenNanos.Add(int64(d))
	merkeBatcherHoechstwert(&btWartenMaxNs, int64(d))
}

func merkeBatcherArbeit(d time.Duration, zurueckgefallen bool) {
	btArbeitNanos.Add(int64(d))
	btChargen.Add(1)
	merkeBatcherHoechstwert(&btArbeitMaxNs, int64(d))
	if zurueckgefallen {
		btRueckfaelle.Add(1)
	}
}

// BatcherPhasenStand zeigt die Aufteilung in /api/health/combined.
func BatcherPhasenStand() map[string]interface{} {
	n := btChargen.Load()
	msJe := func(z *atomic.Int64) float64 {
		if n == 0 {
			return 0
		}
		return float64(z.Load()) / float64(n) / 1e6
	}
	postenJe := float64(0)
	if n > 0 {
		postenJe = float64(btPosten.Load()) / float64(n)
	}
	warten, arbeit := msJe(&btWartenNanos), msJe(&btArbeitNanos)
	gesamt := warten + arbeit + msJe(&btSammelNanos)
	jefall := float64(0)
	if p := btPosten.Load(); p > 0 {
		jefall = (float64(btWartenNanos.Load()) + float64(btArbeitNanos.Load())) / float64(p) / 1e6
	}
	return map[string]interface{}{
		"chargen":            n,
		"posten":             btPosten.Load(),
		"posten_je_charge":   postenJe,
		"groesste_charge":    btChargeMaxLen.Load(),
		"sammeln_ms":         msJe(&btSammelNanos),
		"warten_ms":          warten,
		"warten_max_ms":      float64(btWartenMaxNs.Load()) / 1e6,
		"arbeit_ms":          arbeit,
		"arbeit_max_ms":      float64(btArbeitMaxNs.Load()) / 1e6,
		"gesamt_ms":          gesamt,
		"je_ueberweisung_ms": jefall,
		"serialisiert":       btRueckfaelle.Load(),
		"plaetze":            batcherPlaetze(),
		"bedeutung": "Der Rueckfallpfad, zerlegt. warten_ms ist die Zeit vor einem freien Platz im Semaphor " +
			"(plaetze), arbeit_ms die Verarbeitung selbst. Dominiert warten, bindet der Deckel und Erhoehen ist " +
			"der Hebel; dominiert arbeit, ist der Deckel unschuldig. je_ueberweisung_ms ist die Zahl, die gegen " +
			"transfer_phases.total_ms des Schnellpfads zu halten ist -- ihre Differenz erklaert die mittlere Latenz.",
	}
}

// BatcherPhasenZuruecksetzen macht zwei Messlaeufe vergleichbar.
func BatcherPhasenZuruecksetzen() {
	for _, z := range []*atomic.Int64{
		&btWartenNanos, &btArbeitNanos, &btSammelNanos, &btChargen,
		&btPosten, &btRueckfaelle, &btWartenMaxNs, &btArbeitMaxNs, &btChargeMaxLen,
	} {
		z.Store(0)
	}
}
