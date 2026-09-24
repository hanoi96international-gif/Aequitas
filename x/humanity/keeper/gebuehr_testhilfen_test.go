package keeper

import (
	"testing"
)

// nachGebuehr: Saldo eines Absenders mit start nach Ueberweisungen der
// Betraege -- jede zahlt die Ueberweisungsgebuehr obendrauf
// (ueberweisungsgebuehr.go), berechnet am jeweiligen Saldo.
func nachGebuehr(start float64, betraege ...float64) float64 {
	s := NewDecimal(start)
	for _, b := range betraege {
		g := ueberweisungsGebuehrFuer(b, s.Float())
		s = s.Sub(NewDecimal(b)).Sub(NewDecimal(g))
	}
	return s.Float()
}

// wiederholt: n-mal betrag.
func wiederholt(betrag float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = betrag
	}
	return out
}

// gebuehrenUnterwegs: Ueberweisungsgebuehren, die Absendern schon abgezogen,
// dem Grundeinkommen aber noch nicht gutgeschrieben sind -- sie stehen in den
// ausstehenden Transaktionen (Ausgangskorb; beim WAL-Pfad erst nach dem
// Nachschreiben dorthin).
func gebuehrenUnterwegs(t testing.TB, cs *ChainState) float64 {
	t.Helper()
	if cs.wal != nil {
		cs.FlushWALNow()
	}
	s := NewDecimal(0)
	cs.walFlushMu.Lock()
	for _, it := range cs.walFlushQueue {
		s = s.Add(NewDecimal(it.tx.Gebuehr))
	}
	cs.walFlushMu.Unlock()
	if cs.db != nil {
		var db float64
		if err := cs.db.QueryRow(`SELECT COALESCE(SUM((tx_json::json->>'gebuehr')::numeric),0) FROM pending_txs`).Scan(&db); err != nil {
			t.Fatalf("Gebuehren im Ausgangskorb: %v", err)
		}
		s = s.Add(NewDecimal(db))
	}
	return s.Float()
}
