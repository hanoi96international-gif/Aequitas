package keeper

import (
	"sync/atomic"
	"time"
)

// Wo die Zeit in runAtomicWithOutbox wirklich hingeht.
//
// # WARUM DIESE MESSUNG VOR DEM UMBAU KOMMT
//
// Das Mutex-Profil vom 29.08.2026 sagt, dass 74,7 % der gesamten Wartezeit
// dieses Knotens in runAtomicWithOutbox entstehen -- die globale Sperre wird
// dort ueber eine ganze DB-Transaktion gehalten. Was es NICHT sagt: wie sich
// dieser Halt aufteilt. Und davon haengt ab, was ein Umbau ueberhaupt bringt:
//
//   - Ist der COMMIT der groesste Anteil, ist es ein Netzwerk-Umlauf zu
//     Postgres, und die Sperre zu verengen hilft sofort und stark: nebenlaeufige
//     Ueberweisungen auf verschiedene Konten warten dann nicht mehr
//     aufeinander, sondern nur noch die Datenbank auf sich selbst.
//   - Ist die ARBEIT in fn der groesste Anteil, hilft Verengen weniger, und die
//     Frage ist eher, was fn tut.
//   - Ist die AUFNAHME (das Warten auf cs.mu) der groesste Anteil, ist die
//     Sperre schon jetzt der Engpass und nicht das, was sie schuetzt.
//
// Drei Vermutungen ueber den Engpass dieses Knotens waren in der Vergangenheit
// falsch (siehe contention_profile.go). Diese hier wird gemessen.
//
// Kosten: fuenf time.Now() je atomarer Operation. Neben einem
// Postgres-Umlauf nicht messbar, und anders als das Contention-Profil braucht
// es keinen Schalter -- eine Zahl, die man erst einschalten muss, sieht sich
// niemand an.
type atomicPhasen struct {
	laeufe     atomic.Int64
	wartenNs   atomic.Int64 // bis cs.mu erworben ist
	snapshotNs atomic.Int64 // snapshotForRollbackLocked
	fnNs       atomic.Int64 // die eigentliche Arbeit
	outboxNs   atomic.Int64 // savePendingTxExec
	commitNs   atomic.Int64 // tx.Commit()
	haltNs     atomic.Int64 // gesamte Haltezeit von Lock bis Unlock
}

var atomicPhasenStand atomicPhasen

func (p *atomicPhasen) notiere(feld *atomic.Int64, seit time.Time) {
	feld.Add(int64(time.Since(seit)))
}

// AtomicPhasenStand gibt die Aufteilung fuer /api/health/combined zurueck --
// Mittelwerte in Millisekunden, damit die Zahlen ohne Umrechnen lesbar sind.
func AtomicPhasenStand() map[string]interface{} {
	n := atomicPhasenStand.laeufe.Load()
	if n == 0 {
		return map[string]interface{}{
			"laeufe":    0,
			"bedeutung": "noch keine atomare Operation gemessen",
		}
	}
	ms := func(v *atomic.Int64) float64 {
		return float64(v.Load()) / float64(n) / 1e6
	}
	halt := ms(&atomicPhasenStand.haltNs)
	commit := ms(&atomicPhasenStand.commitNs)
	anteil := 0.0
	if halt > 0 {
		anteil = commit / halt * 100
	}
	return map[string]interface{}{
		"laeufe":            n,
		"warten_auf_sperre": ms(&atomicPhasenStand.wartenNs),
		"snapshot_ms":       ms(&atomicPhasenStand.snapshotNs),
		"arbeit_ms":         ms(&atomicPhasenStand.fnNs),
		"outbox_ms":         ms(&atomicPhasenStand.outboxNs),
		"commit_ms":         commit,
		"halt_gesamt_ms":    halt,
		"commit_anteil_pct": anteil,
		"bedeutung": "Aufteilung der globalen Sperre in runAtomicWithOutbox. Ein hoher " +
			"commit_anteil_pct heisst: die Sperre wird ueberwiegend fuer einen " +
			"Postgres-Umlauf gehalten, und sie auf die Konten-Shards zu verengen " +
			"wuerde nebenlaeufige Ueberweisungen sofort entkoppeln",
	}
}
