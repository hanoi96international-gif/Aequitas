package keeper

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// KEIN RUECKSTAU: der annehmende Knoten nimmt nur so viel an, wie in den
// naechsten Block passt.
//
// # WAS PASSIERTE
//
// TPS-Messung 26.09.2026 23:20: C1 nahm 4.771 Ueberweisungen je Sekunde an,
// verblockte aber nur 3.774. Die Differenz -- angenommen, aber noch in keinem
// Block -- wuchs, solange die Last lief; C2 hing ihr hinterher. Die einzige
// Grenze war die Zahl gleichzeitig laufender RPC-Posten (inflight_grenze.go,
// am Arbeitsspeicher bemessen), nicht die Blockleistung. Ein Knoten, der
// schneller annimmt als er verblockt, baut per Konstruktion einen Rueckstau
// auf -- und jede Sekunde Rueckstau ist eine Sekunde, in der die Validatoren
// verschiedene Staende haben.
//
// # WIE (seit 27.09.2026: HOECHSTENS EIN BLOCK, LUECKENLOS)
//
// Ein Messer liest alle rueckstauTakt, was angenommen und noch nicht in einem
// Block ist: offene Zeilen in pending_txs (ueber den Teilindex
// idx_pending_txs_offen) plus die WAL-Warteschlange im Speicher, die noch gar
// nicht in pending_txs steht. Zwischen zwei Messungen zaehlt jede Zulassung
// in sendRawTransaction sofort mit (rueckstauPlatzNehmen); die Messung zieht
// nur ab, was sie selbst schon gesehen hat. Der Stand ist damit nie kleiner
// als die Wirklichkeit -- auch nicht, wenn tausende Anfragen innerhalb einer
// Messpause eintreffen.
//
// Die Grenze ist Vorgabe EIN voller Block: alles Angenommene passt in den
// naechsten Block. Eine Ueberweisung wartet also nie laenger als bis zu ihm;
// was darueber hinaus kaeme, bekommt -32005 ("gleich nochmal") und wird von
// der Wallet wiederholt, statt in einer Schlange zu stehen. Die Annahme kann
// so nie schneller sein als die Bloecke.
//
// Haengt der Messer oder ist die Datenbank weg, waechst der Zaehler weiter
// und die Annahme schliesst sich von selbst -- fail closed.
//
// # WAS VERBLOCKT IST, ZAEHLT SOFORT AB (seit 29.09.2026)
//
// Gemessen unter Last: die Zaehlabfrage kam 45 s lang nicht durch (gemessen
// blieb auf 11.872 stehen), waehrend die Bloecke den Rueckstau laengst
// geleert hatten -- ab da trugen sie 0 Ueberweisungen, und die Annahme
// lehnte trotzdem alles mit -32005 ab. Ein falsches "voll" legte den Knoten
// fuer Dutzende Sekunden still. Jetzt zieht jeder GESPEICHERTE Block seine
// Ueberweisungen sofort ab (rueckstauEingeschlossen), unabhaengig von der
// Messung. Fail closed bleibt: ohne Datenbank wird kein Block gespeichert,
// also auch nichts abgezogen. Nach einer Messung wird der Abzug auf den
// Stand NACH der Abfrage zurueckgesetzt -- was waehrend der Abfrage
// verblockt wurde, zaehlt damit hoechstens doppelt, nie zu wenig. Die
// Abfrage selbst hat eine Frist (rueckstauMessFrist).
//
//	AEQUITAS_RUECKSTAU_MAX   Obergrenze in Ueberweisungen (Vorgabe: was der naechste
//	                         Takt baut, siehe rueckstauBloeckeJeTakt; 0 = aus)

const rueckstauTakt = 200 * time.Millisecond

var (
	rueckstauAktuell    atomic.Int64 // letzte Messung
	rueckstauZugelassen atomic.Int64 // seit der letzten Messung zugelassen
	rueckstauGemessen   atomic.Int64 // Unix-Zeit der letzten Messung
	rueckstauAbgelehnt  atomic.Int64
	rueckstauMesserAn   atomic.Bool

	rueckstauEingeschlossen atomic.Int64 // seit der letzten Messung in gespeicherten Bloecken
	rueckstauMessFehler     atomic.Int64
)

// rueckstauMessFrist: laenger wartet der Messer nicht auf die Zaehlabfrage.
const rueckstauMessFrist = 2 * time.Second

// MerkeRueckstauVerblockt: n Ueberweisungen aus pending_txs stehen jetzt in
// einem gespeicherten Block (ProduceBlock, nach dem Speichern).
func MerkeRueckstauVerblockt(n int) {
	if n > 0 {
		rueckstauEingeschlossen.Add(int64(n))
	}
}

// rueckstauMax: die geltende Obergrenze.
func rueckstauMax() int64 {
	if raw := os.Getenv("AEQUITAS_RUECKSTAU_MAX"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return int64(rueckstauBloeckeJeTakt()) * rueckstauDeckel()
}

// MIT MEHREREN BLOECKEN JE TAKT (28.09.2026).
//
// "Alles Angenommene passt in den naechsten Block" war richtig, solange je
// Takt ein Block entstand. Mit ENABLE_MULTI_BLOCK_TICK=1 baut der Knoten aber
// bis zu 1+maxExtraBlocksPerTick Bloecke im selben Takt -- und zwar nur, wenn
// der vorige VOLL war. Eine Grenze von einem Block laesst nie genug fuer einen
// vollen Block uebrig, sobald der erste die Warteschlange geleert hat: der
// Zusatzblock entstand nie. Gemessen am 27.09.: 55 Bloecke im Mittel 4.153,
// der groesste 6.464 bei Deckel 7.000 -- kein einziger voll, trotz 9.202
// Ablehnungen "server busy". Die Kette stand damit bei rund 4.000 statt bis
// zu 5 x 7.000 Ueberweisungen/s.
//
// Die Zusage bleibt dieselbe, nur je TAKT statt je Block: alles Angenommene
// passt in das, was der naechste Takt baut. Greift eine Bremse (Peer-Lag,
// Eigenlast), entstehen keine Zusatzbloecke (blockVollAmDeckel) -- dann ist
// es wieder ein Block, und zwar mit dem gebremsten Deckel, damit der Knoten
// nicht mehr annimmt, als der schonungsbeduerftige Partner verkraftet.
func rueckstauBloeckeJeTakt() int {
	if os.Getenv("ENABLE_MULTI_BLOCK_TICK") != "1" {
		return 1
	}
	if d := blockDeckelZuletzt.Load(); d > 0 && !blockDeckelZuletztUngebremst.Load() {
		return 1 // gebremst: keine Zusatzbloecke
	}
	return 1 + maxExtraBlocksPerTick
}

// rueckstauDeckel: der Deckel des zuletzt gebauten Blocks (gebremst oder
// nicht), vor dem ersten Block der harte Deckel.
func rueckstauDeckel() int64 {
	if d := blockDeckelZuletzt.Load(); d > 0 {
		return d
	}
	return int64(blockTxHartDeckel())
}

func rueckstauStand() int64 {
	n := rueckstauAktuell.Load() + rueckstauZugelassen.Load() - rueckstauEingeschlossen.Load()
	if n < 0 {
		return 0
	}
	return n
}

func rueckstauMeldung(n, grenze int64) string {
	return fmt.Sprintf("server busy: %d accepted transfers are waiting for the next block (limit %d); try again shortly", n, grenze)
}

// rueckstauGrund: leer, solange noch Platz im naechsten Block ist. Nur
// Pruefung, belegt nichts (admissionRefusalReason, Batch-Vorlauf). Zaehlt
// jede Ablehnung; AdmissionStats ruft admissionRefusalReason ebenfalls auf,
// das ist eine Abfrage je Health-Aufruf und faellt nicht ins Gewicht.
func rueckstauGrund() string {
	grenze := rueckstauMax()
	if grenze == 0 || !rueckstauMesserAn.Load() {
		return ""
	}
	if n := rueckstauStand(); n >= grenze {
		rueckstauAbgelehnt.Add(1)
		return rueckstauMeldung(n, grenze)
	}
	return ""
}

// rueckstauPlatzNehmen belegt einen Platz im naechsten Block fuer eine
// Ueberweisung, die jetzt zugelassen wird -- oder lehnt ab. Atomar: auch
// gleichzeitige Anfragen koennen die Grenze nicht gemeinsam ueberschreiten.
func rueckstauPlatzNehmen() string {
	grenze := rueckstauMax()
	if grenze == 0 || !rueckstauMesserAn.Load() {
		return ""
	}
	z := rueckstauZugelassen.Add(1)
	if n := rueckstauAktuell.Load() + z - rueckstauEingeschlossen.Load(); n > grenze {
		rueckstauZugelassen.Add(-1)
		rueckstauAbgelehnt.Add(1)
		return rueckstauMeldung(n-1, grenze)
	}
	return ""
}

// rueckstauMessungUebernehmen: gemessen ist, was vor der Abfrage schon
// zugelassen war (vorher) -- das zieht der Zaehler ab; was waehrend der
// Abfrage kam, bleibt gezaehlt (lieber einmal doppelt als einmal zu wenig).
func rueckstauMessungUebernehmen(offen, vorher int64) {
	rueckstauAktuell.Store(offen)
	rueckstauZugelassen.Add(-vorher)
	// Den Abzug auf den Stand NACH der Abfrage zuruecksetzen: was sie noch
	// als offen gezaehlt hat, aber schon verblockt ist, zaehlt so einmal zu
	// viel -- nie einmal zu wenig.
	rueckstauEingeschlossen.Store(0)
	rueckstauGemessen.Store(time.Now().Unix())
}

// StarteRueckstauMesser: der Messer, nur auf einem Knoten mit Datenbank.
func (dag *BlockDAG) StarteRueckstauMesser() {
	cs := dag.state
	if cs == nil || cs.db == nil || rueckstauMesserAn.Swap(true) {
		return
	}
	SafeGoroutine("rueckstauMesser", func() {
		t := time.NewTicker(rueckstauTakt)
		defer t.Stop()
		for range t.C {
			vorher := rueckstauZugelassen.Load()
			var offen int64
			ctx, abbrechen := context.WithTimeout(context.Background(), rueckstauMessFrist)
			err := cs.db.QueryRowContext(ctx, `SELECT count(*) FROM pending_txs WHERE included_at = 0`).Scan(&offen)
			abbrechen()
			if err != nil {
				rueckstauMessFehler.Add(1)
				continue
			}
			rueckstauMessungUebernehmen(offen+int64(cs.WALFlushQueueDepth()), vorher)
		}
	})
}

// RueckstauStand fuer /api/health/combined.
func RueckstauStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung":              "Angenommen, aber noch in keinem Block (gemessen plus seit der Messung zugelassen). Die Grenze ist ein voller Block: alles Angenommene passt in den naechsten Block, darueber lehnt der Knoten mit -32005 ab.",
		"aktuell":                rueckstauStand(),
		"gemessen":               rueckstauAktuell.Load(),
		"messung_alter_s":        time.Now().Unix() - rueckstauGemessen.Load(),
		"grenze":                 rueckstauMax(),
		"abgelehnt":              rueckstauAbgelehnt.Load(),
		"messer_an":              rueckstauMesserAn.Load(),
		"verblockt_seit_messung": rueckstauEingeschlossen.Load(),
		"mess_fehler":            rueckstauMessFehler.Load(),
	}
}
