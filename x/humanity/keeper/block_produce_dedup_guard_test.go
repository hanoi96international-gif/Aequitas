package keeper

import "testing"

// TestHasBlockFromProposerAtHeight_NilDBFailsOpen is the baseline safety
// check for ProduceBlock's post-boot double-production guard (see that call
// site's own comment, block.go): with no DB configured, this must report
// false (matching every other Load*FromDB helper's nil-DB contract) so the
// guard never blocks production when it can't actually check anything —
// the failure mode is "behave exactly like before this fix existed", never
// "block production forever".
func TestHasBlockFromProposerAtHeight_NilDBFailsOpen(t *testing.T) {
	cs := newTestState() // useDB: false, cs.db is nil
	if cs.HasBlockFromProposerAtHeight("0xabc", 100) {
		t.Fatal("HasBlockFromProposerAtHeight with no DB configured must return false, not block production on an unanswerable question")
	}
}

// NOTE: ProduceBlock's guard itself (a real HasBlockFromProposerAtHeight
// lookup against maxParentHeight+1, run on every tick — no post-boot time
// window since the 2026-07-24 fix, see that call site's own comment for why
// the earlier 45s window kept being too short) is not exercised end-to-end
// here — reaching it requires satisfying every earlier ProduceBlock gate
// (genesis, tips, signing key, a live Postgres for the query to mean
// anything), well beyond this suite's existing early-exit-focused
// ProduceBlock tests (e.g. TestProduceBlock_SkipsWhileResyncInProgress).
// Verified live instead: see this fix's own memory entry for the
// reproduction that motivated it.
