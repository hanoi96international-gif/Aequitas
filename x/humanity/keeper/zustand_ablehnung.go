package keeper

import (
	"errors"
	"sync/atomic"
)

// Ein Block darf nicht daran sterben, dass EINE Ueberweisung darin nicht
// anwendbar ist.
//
// # DER VORFALL
//
// Gemessen am 05.09.2026 auf dem Primary, unter Last:
//
//	mauer  = 5775433/18   <- dieselbe Hoehe 18-mal abgewiesen
//	tips   = 31           <- Waisen stapeln sich
//	refusing = true, stalled_seconds = 382
//
// Der Knoten lief, war erreichbar, meldete sich gesund -- und kam sechs
// Minuten lang an einem Block nicht vorbei. Ein abgewiesener Block wird nie
// wieder angenommen, also war das kein Verzoegern, sondern eine Wand.
//
// # WARUM ER ENTSTEHT
//
// replayTransactions behandelte JEDEN Fehler aus applyTransferDeltaLocked als
// hardFailure und rollte den ganzen Block zurueck. Das ist fuer einen
// DB-Fehler richtig: er ist voruebergehend, ein spaeterer Versuch kann
// gelingen, und einen Block halb anzuwenden waere schlimmer.
//
// Fuer "insufficient balance" ist es falsch. Dieser Fehler ist
// DETERMINISTISCH: derselbe Block scheitert beim tausendsten Versuch aus
// demselben Grund. Die Abweisung heilt nichts, sie haelt den Knoten nur an --
// und zwar endgueltig, weil die Hoehe nie wieder versucht wird.
//
// Der Fehler tritt auf, weil der erzeugende Knoten die Ueberweisung anwenden
// KONNTE und der nachspielende nicht -- die beiden sind sich ueber den
// Kontostand nach Demurrage uneins ("have 0.000000 after demurrage, need
// 0.000010"). Das ist eine echte Abweichung und muss sichtbar werden. Sie
// sichtbar zu machen ist aber Aufgabe des StateRoot-Vergleichs am Ende des
// Replays, nicht Aufgabe eines Stillstands: der StateRoot meldet die
// Abweichung UND laesst die Kette weiterlaufen.
//
// # WAS SICH AENDERT
//
// Deterministische Ablehnungen ueberspringen die Ueberweisung und laufen
// weiter -- genau das, was jede Kette mit einer fehlgeschlagenen Transaktion
// tut: sie steht im Block und hat keine Wirkung. Alles andere bleibt
// hardFailure.
//
// Die Unterscheidung ueber einen Sentinel und nicht ueber den Fehlertext:
// eine Zeichenkettenpruefung auf dem Geldpfad bricht still, sobald jemand
// eine Meldung umformuliert, und der Rueckfall waere wieder die Wand.
var ErrZustandLehntAb = errors.New("zustand lehnt diese transaktion ab")

// istZustandsAblehnung sagt, ob ein Fehler deterministisch ist -- also ob ein
// erneuter Versuch dasselbe Ergebnis haette.
func istZustandsAblehnung(err error) bool {
	return errors.Is(err, ErrZustandLehntAb)
}

// Wie oft eine Ueberweisung beim Nachspielen uebersprungen wurde.
//
// Diese Zahl MUSS sichtbar sein. Der Fix oben tauscht einen lauten Fehler
// (Knoten steht) gegen einen leisen (Ueberweisung wirkungslos) -- und ein
// leiser Fehler, den niemand zaehlt, ist schlimmer als ein lauter. Steigt sie,
// sind sich die Knoten ueber Kontostaende uneins, und das gehoert untersucht,
// auch wenn die Kette weiterlaeuft.
var uebersprungeneUeberweisungen atomic.Int64

func merkeUebersprungeneUeberweisung() {
	uebersprungeneUeberweisungen.Add(1)
}

// ZustandsAblehnungStand zeigt die Zahl in /api/health/combined.
func ZustandsAblehnungStand() map[string]interface{} {
	n := uebersprungeneUeberweisungen.Load()
	return map[string]interface{}{
		"uebersprungene_ueberweisungen": n,
		"bedeutung": "Ueberweisungen, die beim Nachspielen nicht anwendbar waren und " +
			"uebersprungen statt mit dem ganzen Block abgewiesen wurden. 0 ist der " +
			"Normalfall. Steigt der Wert, sind sich erzeugender und nachspielender " +
			"Knoten ueber einen Kontostand nach Demurrage uneins -- die Kette laeuft " +
			"weiter, aber die Abweichung gehoert untersucht (StateRoot-Vergleich)",
	}
}
