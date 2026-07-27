package keeper

import "testing"

// wait_count is the field this exists for: it answers whether callers ever had
// to wait for a connection, instead of that being inferred from a throughput
// number that swings by 2x between runs. A state with no database must still
// answer coherently rather than panicking on a nil pool.
func TestDBPoolStats_NoDatabaseIsSafe(t *testing.T) {
	cs := newTestState()
	st := cs.DBPoolStats()
	if configured, _ := st["configured"].(bool); configured {
		t.Fatal("a state with no database must report configured=false")
	}
}

// The fields an operator needs must all be present, or the endpoint looks
// healthy precisely when it cannot answer the question.
func TestDBPoolStats_ReportsWhatDecidesThePoolQuestion(t *testing.T) {
	cs := newTestState()
	if cs.db == nil {
		t.Skip("no database in this environment; the no-DB path is covered above")
	}
	st := cs.DBPoolStats()
	for _, key := range []string{"max_open", "in_use", "wait_count", "wait_total_ms", "wait_avg_ms"} {
		if _, ok := st[key]; !ok {
			t.Fatalf("%q missing — without it the pool question stays a guess", key)
		}
	}
}
