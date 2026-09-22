package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// ── Der Zaehler muss einen Neustart ueberleben ────────────────────────────
//
// # DER VORFALL, DEN DAS SCHLIESST
//
// uebersprungeneUeberweisungen oben ist ein atomic.Int64 im Prozess. Er faengt
// nach JEDEM Neustart und JEDEM Resync wieder bei null an.
//
// Am 15.09.2026 stand auf beiden Boxen "0 uebersprungen", und daraus wurde
// geschlossen, es sei nie eine Ueberweisung uebersprungen worden. Der Schluss
// war nicht gedeckt: die Null sagte nur, dass seit dem letzten Start keine
// uebersprungen wurde. Genau in dieser Zeit liefen die Kontenstaende
// auseinander, und zwar -- belegt in zwei_produzenten_realdb_test.go -- durch
// genau diesen Mechanismus.
//
// Ein Zaehler, den jeder Neustart loescht, ist fuer eine Divergenz, die
// dauerhaft bleibt, das falsche Werkzeug. /api/wache faerbt bei > 0 rot
// (api_wache.go) -- und der Neustart, den ein Betreiber nach einem roten
// Alarm als Erstes versucht, loescht den Alarm, ohne die Divergenz zu
// beheben. Das ist die schlechteste denkbare Kombination: es SIEHT aus, als
// haette der Neustart geholfen.
//
// # WIE ES JETZT LAEUFT
//
// Die Summe steht in chain_config und wird im SELBEN dbTx fortgeschrieben wie
// der Block, in dem uebersprungen wurde. Wird der Block zurueckgerollt, faellt
// die Erhoehung mit ihm -- gezaehlt wird nur, was auch angewandt wurde.
//
// Geschrieben wird EINMAL JE BLOCK und nur, wenn in diesem Block ueberhaupt
// etwas uebersprungen wurde. Im Normalfall (nichts uebersprungen) kostet das
// keinen einzigen Datenbankzugriff.
//
// Nur ein Resync raeumt den Stand ab -- das ist der Vorgang, der die
// Divergenz auch tatsaechlich behebt.
const uebersprungenConfigKey = "uebersprungene_ueberweisungen_gesamt"

// uebersprungeneLaden holt den dauerhaften Stand beim Start in den Zaehler.
// Fehler sind still: ein Knoten, der wegen eines fehlenden Konfigurationswerts
// nicht startet, waere schlimmer als ein Zaehler, der bei null beginnt.
func (cs *ChainState) uebersprungeneLaden() {
	v := strings.TrimSpace(cs.getConfigValueDB(uebersprungenConfigKey))
	if v == "" {
		return
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return
	}
	uebersprungeneUeberweisungen.Store(n)
	fmt.Printf("[ZUSTAND] %d uebersprungene Ueberweisungen aus frueheren Laeufen uebernommen -- "+
		"die Wache bleibt rot, bis ein Resync beide Boxen wieder gleichstellt\n", n)
}

// uebersprungeneSichern schreibt den Stand in den laufenden dbTx des Blocks.
// imBlock ist die Zahl der in DIESEM Block uebersprungenen Ueberweisungen.
//
// LESEN-ADDIEREN-SCHREIBEN, nicht den Zaehler aus dem Speicher abschreiben.
// Der Unterschied ist nicht kosmetisch: uebersprungeneUeberweisungen wird
// beim Ueberspringen erhoeht, also BEVOR feststeht, ob der Block ueberhaupt
// commitet. Rollt er zurueck, bleibt die Erhoehung im Speicher stehen -- und
// haette der naechste Block, der irgendetwas ueberspringt, sie mitgeschrieben.
// Dann staende in der Datenbank eine Zahl, die Ueberspringen mitzaehlt, das
// nie stattgefunden hat.
//
// Gelesen wird durch denselben ctx, also innerhalb des Block-dbTx: der Wert
// ist der zuletzt commitete, und das Zurueckschreiben faellt mit dem Block,
// wenn der faellt. Damit zaehlt die dauerhafte Summe genau das, was auch
// angewandt wurde.
//
// Der Aufrufer haelt cs.mu (Nachspielen tut das durchgehend) -- die
// Vorbedingung von getConfigValueCtx.
func (cs *ChainState) uebersprungeneSichern(ctx context.Context, imBlock int) {
	if imBlock <= 0 {
		return
	}
	var bisher int64
	if v := strings.TrimSpace(cs.getConfigValueCtx(ctx, uebersprungenConfigKey)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			bisher = n
		}
	}
	stand := bisher + int64(imBlock)
	if err := cs.setConfigValueCtx(ctx, uebersprungenConfigKey, strconv.FormatInt(stand, 10)); err != nil {
		// Still: der Block ist gueltig, und ihn an einer Diagnosezahl
		// scheitern zu lassen waere die falsche Richtung. Der Zaehler im
		// Speicher stimmt weiter, nur sein Ueberleben ist nicht gesichert.
		fmt.Printf("[ZUSTAND] Warnung: uebersprungene Ueberweisungen nicht dauerhaft gemacht: %v\n", err)
	}
}

// UebersprungeneZuruecksetzen raeumt den dauerhaften Stand ab. Gehoert
// ausschliesslich an das Ende eines Resyncs -- das ist der Vorgang, der die
// Divergenz behebt, die der Zaehler meldet.
//
// setConfigValueDB, nicht setConfigValue: beide Aufrufstellen im Resync
// (snapshot.go) halten cs.mu NICHT, und setConfigValue liest cs.activeTx --
// ein Feld, das allein von cs.mu synchronisiert wird. Ohne die Sperre koennte
// dieser Schreibvorgang in der laufenden Transaktion eines fremden Vorgangs
// landen. Genau davor warnt getConfigValue in seiner eigenen Vorbedingung.
func (cs *ChainState) UebersprungeneZuruecksetzen() {
	uebersprungeneUeberweisungen.Store(0)
	if err := cs.setConfigValueDB(uebersprungenConfigKey, "0"); err != nil {
		fmt.Printf("[ZUSTAND] Warnung: uebersprungene Ueberweisungen nicht zurueckgesetzt: %v\n", err)
	}
}
