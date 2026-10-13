package keeper

import (
	"database/sql"
	"fmt"
	"os"
	"runtime/debug"
)

// atomicAbschluss: was runAtomicWithOutbox und
// runAtomicDistributionWithOutbox bei einer PANIK in fn tun (OFFEN 7b).
//
// Beide halten cs.mu ueber die ganze DB-Transaktion, mit cs.activeTx gesetzt
// und dem Speicher womoeglich schon veraendert. Paniert fn, fing bisher ein
// recover weiter oben die Panik (HTTP-Handler, SafeCall) -- cs.mu blieb aber
// gesperrt, die Transaktion offen, der Speicher halb geschrieben. Der Knoten
// stand, statt neu zu starten, und bei einem einzelnen Validator merkt das
// der Totmann nicht (kein Peer voraus). Ueber das oeffentliche RPC erreichbar.
//
// Jetzt dasselbe wie im Fehlerpfad: Transaktion zurueck, Speicher aus dem
// Schnappschuss, activeTx leer, Sperre frei -- und die Panik geht weiter
// nach oben (der Aufrufer bekommt seine 500). Gelingt das Zurueckrollen
// nicht, ist der Zustand nicht mehr verlaesslich: dann Neustart (der Zustand
// kommt aus der Datenbank), statt mit einem halben Speicher weiterzulaufen.
// Nach tx.Commit (abgeschlossen) wird nichts zurueckgerollt -- die Datenbank
// traegt dann den neuen Stand, der Speicher auch.
type atomicAbschluss struct {
	cs       *ChainState
	tx       *sql.Tx
	snap     *blockRollbackSnapshot
	gesperrt bool
	// imCommit: von tx.Commit bis zum Ende seiner Behandlung. Paniert es hier
	// (im Treiber, oder beim Zurueckrollen nach einem gescheiterten Commit),
	// ist der Ausgang der Transaktion unbekannt -- Neustart (Pruefung #339).
	imCommit bool
	// abgeschlossen: tx.Commit ist gelungen; die Datenbank traegt den neuen
	// Stand, der Speicher auch -- nichts zurueckrollen.
	abgeschlossen bool
}

// atomicNeustart: os.Exit, im Test ersetzbar.
var atomicNeustart = os.Exit

// entsperren: der regulaere Weg -- markiert und gibt frei, in einem Schritt.
func (a *atomicAbschluss) entsperren() {
	a.gesperrt = false
	a.cs.mu.Unlock()
}

// beiPanik: als defer direkt nach cs.mu.Lock(). Laeuft bei jedem Verlassen;
// war die Sperre regulaer freigegeben (entsperren), tut es nichts. Sonst ist
// die Funktion abnormal verlassen worden -- durch eine Panik oder durch
// runtime.Goexit (r == nil) -- und es wird aufgeraeumt. Der Neustart, wenn
// noetig, geschieht UNTER der Sperre: sonst koennte eine andere Goroutine
// den halben Zustand noch sehen oder speichern.
func (a *atomicAbschluss) beiPanik() {
	if !a.gesperrt {
		return
	}
	r := recover()
	switch {
	case a.abgeschlossen:
		a.cs.setActiveTx(nil)
	case a.imCommit:
		fmt.Printf("[ATOMIC] ✗ Panik waehrend des Commits (%v) -- Ausgang der Transaktion unbekannt; Neustart\n%s\n", r, debug.Stack())
		os.Stdout.Sync()
		atomicNeustart(9)
		a.cs.setActiveTx(nil)
	default:
		a.cs.setActiveTx(nil)
		a.tx.Rollback()
		if a.snap != nil && !a.zurueckrollen() {
			fmt.Printf("[ATOMIC] ✗ Panik (%v) und das Zurueckrollen des Speichers gelang nicht -- Zustand unzuverlaessig; Neustart\n%s\n", r, debug.Stack())
			os.Stdout.Sync()
			atomicNeustart(9)
		}
	}
	a.entsperren()
	if r != nil {
		panic(r)
	}
}

// zurueckrollen: true, wenn der Speicher wieder dem Schnappschuss entspricht.
func (a *atomicAbschluss) zurueckrollen() (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return a.cs.restoreFromRollbackLocked(a.snap) == nil
}

// zustandAendern: fn unter cs.mu, fuer Aenderungen OHNE Schnappschuss.
// Paniert fn, kann der Zustand halb geschrieben sein (etwa ein Kontostand
// ohne seinen Blattwert in accountSetXOR) -- und es gibt kein Zurueck. Dann
// Sperre frei und Neustart; der Zustand kommt aus der Datenbank. Still
// weiterzulaufen hiesse, mit einem falschen Zustand Bloecke zu bauen
// (OFFEN 7b).
func (cs *ChainState) zustandAendern(label string, fn func()) {
	cs.mu.Lock()
	defer func() {
		r := recover()
		if r != nil {
			// Unter der Sperre: niemand sonst sieht oder speichert den halben
			// Zustand, bevor der Prozess endet (Pruefung #339).
			fmt.Printf("[ZUSTAND] ✗ Panik in %s (%v) -- der Zustand kann halb geschrieben sein; Neustart\n%s\n", label, r, debug.Stack())
			os.Stdout.Sync()
			atomicNeustart(9)
		}
		cs.mu.Unlock()
		if r != nil {
			panic(r) // nur erreicht, wenn atomicNeustart im Test ersetzt ist
		}
	}()
	fn()
}
