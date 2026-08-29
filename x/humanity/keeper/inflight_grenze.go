package keeper

import (
	"os"
	"strconv"
	"sync/atomic"
)

// Eine Obergrenze fuer gleichzeitig angenommene Arbeit.
//
// # DER MESSWERT, DER DAS AUSGELOEST HAT
//
// Am 29.08.2026 wurde der Lastgenerator von 150 auf 576 gleichzeitige Sender
// gestellt. Ergebnis: NULL Erfolge, 138.000 Fehlschlaege, alle
// Zeitueberschreitung. Nicht ein Absturz, nicht ein Fehler im Log -- der Knoten
// nahm einfach weiter an.
//
// Die Rechnung dahinter ist Warteschlangentheorie, kein Geheimnis:
//
//	150 Sender x 100 Stueck je Buendel = 15.000 gleichzeitig  -> 23 s je Buendel
//	576 Sender x 100 Stueck je Buendel = 57.600 gleichzeitig  -> 88 s je Buendel
//
// Der Client wartet 30 s. Bei 23 s kommt die Antwort knapp an, bei 88 s nie.
// Der Knoten arbeitet die ganze Zeit korrekt und mit voller Geschwindigkeit --
// er liefert nur an niemanden mehr aus, weil jeder Wartende vorher aufgibt.
// Angenommene und dann verworfene Arbeit ist doppelt verloren: der Client
// bekommt nichts, und die Rechenzeit ist trotzdem verbraucht.
//
// # WARUM DAS KEIN DURCHSATZ KOSTET
//
// Nach Little's Gesetz ist Durchsatz = gleichzeitige Arbeit / Latenz. Ist der
// Knoten die Grenze, senkt weniger gleichzeitige Arbeit die Latenz im gleichen
// Verhaeltnis -- der Durchsatz bleibt, nur die Wartezeit wird beschraenkt.
// Diese Schranke macht den Knoten also nicht langsamer, sie macht ihn
// VORHERSAGBAR: statt 88 s zu warten und dann leer auszugehen, bekommt der
// Aufrufer sofort ein -32005 und kann es erneut versuchen.
//
// # WARUM NICHT DIE VORHANDENE ANNAHMEKONTROLLE REICHT
//
// admission_control.go lehnt ab, wenn die Blockproduktion 30 s steht. Das ist
// ein NACHLAUFENDER Anzeiger: bis die Produktion so lange steht, sind alle
// Anfragen laengst in ihre Zeitgrenze gelaufen. Diese hier greift an der
// Ursache -- der Laenge der Warteschlange -- und damit bevor der Schaden
// entsteht.
//
// # WARUM DIE VORGABE 8.000 IST
//
// Gemessen wurde: 15.000 gleichzeitig ergaben 23 s Bearbeitungszeit je
// Buendel, gefaehrlich nah an den 30 s des Clients. 57.600 ergaben 88 s und
// damit gar nichts. 8.000 liegt bei rund 12 s -- genug Luft unter der
// Client-Grenze, und weit unter dem Wert, der nachweislich zusammenbricht.
// Wird der Knoten schneller, sinkt die Wartezeit bei gleicher Schranke
// automatisch mit.
//
// LIVE BESTAETIGT (29.08.2026, C2, 450 gleichzeitige Sender): der
// Hoechststand lag bei 3.500 gleichzeitigen Posten, die Schranke lehnte
// NICHTS ab. Das ist das gewuenschte Verhalten und zugleich der Beleg, dass
// die 8.000 richtig liegen -- deutlich ueber dem, was echter Betrieb mit
// aktivem Ratenbegrenzer erzeugt, und weit unter den 57.600, die
// nachweislich zusammenbrechen.
//
// WAS DAMIT NICHT GEZEIGT IST: dass die Schranke im Ernstfall greift. Dafuer
// muesste der Ratenbegrenzer abgeschaltet sein -- genau die Konfiguration, in
// der der Zusammenbruch auftrat (AEQUITAS_RPC_RATE_LIMIT_MAX=1000000). Live
// vorfuehren liess sich das nicht, weil es einen Neustart eines
// Produktionsknotens braucht. Belegt ist es nur durch
// TestInflight_576SenderWerdenAbgelehntStattAngenommen. Wer den Begrenzer je
// hochsetzt oder abschaltet, faehrt ab jetzt nicht mehr ohne Netz -- aber wer
// sich darauf verlaesst, sollte es einmal unter Last gesehen haben.
//
//	AEQUITAS_RPC_MAX_INFLIGHT   gleichzeitig angenommene Buendel-Posten
//	                            (Vorgabe 8000; ausdrueckliche 0 schaltet ab)
//
// Ein UNBRAUCHBARER Wert ergibt die Vorgabe, nicht "aus" -- dieselbe Regel,
// die evm_rpc.go fuer den Ratenbegrenzer aufstellt: ein Tippfehler in einer
// Umgebungsvariablen darf einen Schutz niemals abschalten. Nur eine
// ausdrueckliche 0 tut das, weil das eine bewusste Handlung ist.
const inflightVorgabe int64 = 8000

const inflightGrenzeEnv = "AEQUITAS_RPC_MAX_INFLIGHT"

var (
	inflightAktuell      atomic.Int64
	inflightHoechststand atomic.Int64
	inflightAbgelehnt    atomic.Int64
	inflightAngenommen   atomic.Int64
)

// inflightGrenze liefert die geltende Schranke. 0 heisst abgeschaltet.
func inflightGrenze() int64 {
	raw := os.Getenv(inflightGrenzeEnv)
	if raw == "" {
		return inflightVorgabe
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		// Unbrauchbar -> Vorgabe. Ein Tippfehler darf nicht entschaerfen.
		return inflightVorgabe
	}
	return n
}

// inflightEintritt versucht, n Posten anzumelden. Kommt false zurueck, war die
// Schranke erreicht und der Aufrufer darf die Arbeit NICHT beginnen.
//
// Als Vergleichsschleife und nicht als Semaphor-Kanal: die Schranke soll zur
// Laufzeit aenderbar bleiben (ein Kanal hat seine Groesse beim Anlegen fest),
// und ein Aufrufer soll nie blockieren -- sofort ablehnen ist genau der Punkt.
func inflightEintritt(n int64) bool {
	grenze := inflightGrenze()
	if grenze <= 0 {
		inflightAktuell.Add(n)
		inflightAngenommen.Add(n)
		return true
	}
	for {
		jetzt := inflightAktuell.Load()
		if jetzt+n > grenze {
			inflightAbgelehnt.Add(n)
			return false
		}
		if inflightAktuell.CompareAndSwap(jetzt, jetzt+n) {
			inflightAngenommen.Add(n)
			for {
				hoch := inflightHoechststand.Load()
				if jetzt+n <= hoch || inflightHoechststand.CompareAndSwap(hoch, jetzt+n) {
					break
				}
			}
			return true
		}
	}
}

// inflightAustritt meldet n Posten wieder ab. Gehoert IMMER in ein defer
// unmittelbar nach einem erfolgreichen inflightEintritt -- ein vergessener
// Austritt laesst die Schranke dauerhaft zulaufen und der Knoten lehnt fuer
// immer ab.
func inflightAustritt(n int64) {
	inflightAktuell.Add(-n)
}

// InflightStand zeigt die Schranke in /api/health/combined.
func InflightStand() map[string]interface{} {
	grenze := inflightGrenze()
	abgelehnt := inflightAbgelehnt.Load()
	angenommen := inflightAngenommen.Load()
	gesamt := abgelehnt + angenommen
	anteil := 0.0
	if gesamt > 0 {
		anteil = float64(abgelehnt) / float64(gesamt) * 100
	}
	return map[string]interface{}{
		"grenze":               grenze,
		"abgeschaltet":         grenze <= 0,
		"aktuell":              inflightAktuell.Load(),
		"hoechststand":         inflightHoechststand.Load(),
		"abgelehnt":            abgelehnt,
		"angenommen":           angenommen,
		"abgelehnt_pct":        anteil,
		"abweicht_von_vorgabe": grenze != inflightVorgabe,
		"bedeutung": "Obergrenze gleichzeitig angenommener RPC-Posten. Ohne sie nimmt der " +
			"Knoten unbegrenzt Arbeit an und liefert sie nach der Client-Zeitgrenze aus -- " +
			"am 29.08.2026 gemessen: 576 Sender ergaben 0 Erfolge und 138.000 " +
			"Zeitueberschreitungen. abgelehnt_pct > 0 heisst Rueckstau, nicht Defekt",
	}
}
