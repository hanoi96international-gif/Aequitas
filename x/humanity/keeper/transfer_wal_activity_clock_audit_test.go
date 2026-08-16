package keeper

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// transferConcurrentWAL/recoverFromWAL require a real Postgres connection —
// see transfer_wal_test.go's own top-of-file comment. This file follows the
// exact same opt-in convention (newWALTestState skips unless
// AEQUITAS_TPS_BENCH=1 and DATABASE_URL are set).

// TestTransferConcurrentWAL_CrashRecovery_ActivityClockUsesRestartTimeNotOriginalTime
// was an ATTACK TEST — FAIL meant the bug was real and reproduced against the
// actual crash-recovery path rather than argued from reading it.
//
// FIXED 2026-08-16: walTransferRecord now carries `at` and both the live path
// and recoverFromWAL stamp it via touchActivityAt, so this is a regression
// guard and PASS is the expected outcome.
//
// NOTE it still needs Postgres and therefore SKIPS in an ordinary run. That is
// how the defect stayed invisible in a fully green suite in the first place —
// see the two database-free tests at the bottom of this file, which check the
// same property on every run.
//
// touchActivityAt's own doc comment (state.go) explains, for BLOCK replay,
// exactly why "now" is the wrong instant to stamp during replay: "The
// producing node stamps nowUnix() while processing the transaction. Every
// other node sees that same transaction inside a block, possibly seconds
// later... Stamping the replaying node's own clock there is the exact
// pattern FromDemurrageLost and DistributionAt exist to avoid." Block replay
// (applyTransferBatchParallel, applyTransferDeltaLocked) correctly follows
// that rule via touchActivityAt(acc, activityAt), using the block's own
// Timestamp.
//
// recoverFromWAL's applyFrom/applyTo closures (transfer_wal.go, roughly
// lines 1015-1036) do NOT follow it: both call plain touchActivity(acc),
// which stamps nowUnix() — the wall-clock instant crash recovery happens to
// run, not the original transfer's instant. walTransferRecord (the payload
// each WAL record actually stores) carries only {From, To, Amount, TxHash} —
// no timestamp — so there is currently no value recoverFromWAL COULD use to
// do the right thing here even if it called touchActivityAt instead.
//
// CONSEQUENCE: every crash-recovery replay of a WAL-durable transfer resets
// both participants' demurrage clock to "whenever this node happened to
// restart" — which can be much later than the real transfer (this test uses
// ~5.8 simulated days, chosen only to be unambiguously distinguishable from
// "now"; production downtime could be longer or shorter). That hands an
// idle account extra, unearned demurrage-free time — a value leak away from
// the four tokenomics pools, in the crash-recovering node's account
// holders' favor, purely as a function of how long the node happened to be
// down. This does NOT fork consensus (LastActivityAt is deliberately not
// part of accountLeaf/StateRoot — see accountLeaf, state.go), but it is a
// real, silent, permanent divergence from the documented demurrage model
// for every account touched by an unflushed WAL record at the moment of a
// crash.
//
// METHOD: freezes timeNowFunc (state.go's mock seam for nowUnix(), normally
// time.Now) to two distinct, controlled instants — one for the original
// transfer, a different one (safely restored immediately afterward, before
// any other test code runs) for the simulated restart — so the comparison
// is exact and deterministic rather than relying on real elapsed
// wall-clock time during a fast-running test.
func TestTransferConcurrentWAL_CrashRecovery_ActivityClockUsesRestartTimeNotOriginalTime(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")
	truncateDistTestTables(t) // also the opt-in skip gate — see its own comment

	realNow := time.Now().Unix()
	const transferToRecoveryGapSeconds = 500000 // ~5.8 days
	tTransfer := realNow - 1000
	tRecovery := tTransfer + transferToRecoveryGapSeconds

	origTimeNowFunc := timeNowFunc
	t.Cleanup(func() { timeNowFunc = origTimeNowFunc })

	csA := newWALTestState(t, walPath)
	from := distTestAddr(890)
	to := distTestAddr(891)
	// Already active shortly before the transfer, well inside the grace
	// period, so the fast path's demurrage-pending eligibility check does
	// not itself decline this transfer for an unrelated reason.
	seedConcurrentTestAccount(t, csA, from, 1000, tTransfer-100)
	seedConcurrentTestAccount(t, csA, to, 0, tTransfer-100)

	timeNowFunc = func() time.Time { return time.Unix(tTransfer, 0) }
	_, _, applied, err := csA.transferConcurrentWAL(from, to, 100, Transaction{
		Type: "transfer", Wallet: from, To: to, Amount: 100, TxHash: "0xactivityclock1",
	})
	timeNowFunc = origTimeNowFunc
	if !applied || err != nil {
		t.Fatalf("transfer: applied=%v err=%v", applied, err)
	}

	// Sanity: the LIVE path stamped exactly tTransfer, not something else —
	// if this fails, the test's premise is broken, not recoverFromWAL. This
	// part of the codebase is not in question (transfer_wal_test.go already
	// covers it); it is only the baseline this test's real assertion below
	// is measured against.
	csA.mu.RLock()
	fromAccLive, _ := csA.accounts.Get(from)
	gotLive := fromAccLive.LastActivityAt
	csA.mu.RUnlock()
	if gotLive != tTransfer {
		t.Fatalf("test setup: live transferConcurrentWAL stamped LastActivityAt=%d, want exactly tTransfer=%d — cannot proceed", gotLive, tTransfer)
	}

	// "Crash": abandon csA without ever flushing — same pattern as this
	// package's other crash-recovery tests (see
	// TestTransferConcurrentWAL_CrashRecovery_UnflushedTransfersReconstructed).
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	// "Restart, ~5.8 simulated days later": recovery runs synchronously
	// inside NewChainState (via initWALIfEnabled -> recoverFromWAL), under
	// cs.mu.Lock() for its whole duration per that function's own doc
	// comment — safe to restore the real clock immediately after this call
	// returns, before any other test code (including t.Cleanup handlers for
	// OTHER tests, since this package's tests do not run in parallel) sees
	// the mocked value.
	timeNowFunc = func() time.Time { return time.Unix(tRecovery, 0) }
	csB := newWALTestState(t, walPath)
	timeNowFunc = origTimeNowFunc

	csB.mu.RLock()
	fromAccB, _ := csB.accounts.Get(from)
	toAccB, _ := csB.accounts.Get(to)
	gotFrom := fromAccB.LastActivityAt
	gotTo := toAccB.LastActivityAt
	csB.mu.RUnlock()

	// Balance correctness after crash recovery is already covered by this
	// package's other WAL crash-recovery tests — this test is only about
	// the activity/demurrage clock.
	if gotFrom != tTransfer {
		t.Errorf("recovered SENDER LastActivityAt = %d, want %d (the ORIGINAL transfer's time) — got %d, the restart time. recoverFromWAL must stamp walTransferRecord.At via touchActivityAt(acc, at); stamping nowUnix() instead credits the account with the entire outage as demurrage-free time, at the tokenomics pools' expense.",
			gotFrom, tTransfer, tRecovery)
	}
	if gotTo != tTransfer {
		t.Errorf("recovered RECIPIENT LastActivityAt = %d, want %d (the ORIGINAL transfer's time) — got %d, the restart time (same defect, applyTo).",
			gotTo, tTransfer, tRecovery)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// FIXED 2026-08-16. walTransferRecord now carries `at`, the instant the
// transfer actually happened, and both the live path and recoverFromWAL
// stamp the clock with it via touchActivityAt.
//
// The attack test above proves the fix end-to-end, but it needs Postgres and
// therefore SKIPS in an ordinary run — which is exactly how this defect
// stayed invisible in a green suite. The two tests below need no database,
// so the property is checked on every run.
// ─────────────────────────────────────────────────────────────────────────

// TestWALTransferRecord_CarriesTheTransferInstant pins the wire format. If
// `at` is ever dropped or renamed, recovery silently loses the only value it
// can stamp the clock with and falls back to the restart time.
func TestWALTransferRecord_CarriesTheTransferInstant(t *testing.T) {
	const transferredAt = int64(1_700_000_000)
	encoded, err := json.Marshal(walTransferRecord{
		From: "0xaaa", To: "0xbbb", Amount: 12.5, TxHash: "0xdeadbeef", At: transferredAt,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded walTransferRecord
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.At != transferredAt {
		t.Errorf("walTransferRecord lost its timestamp across a round trip: got %d, want %d.\n"+
			"  Without it recoverFromWAL has nothing to stamp the activity clock with and reverts to\n"+
			"  the restart time, crediting every recovered account with the whole outage as\n"+
			"  demurrage-free time.", decoded.At, transferredAt)
	}

	// A record written before the field existed must still decode, and must
	// leave `at` at zero so touchActivityAt applies its documented fallback.
	var legacy walTransferRecord
	if err := json.Unmarshal(
		[]byte(`{"from":"0xaaa","to":"0xbbb","amount":12.5,"tx_hash":"0xdeadbeef"}`), &legacy,
	); err != nil {
		t.Fatalf("a pre-upgrade WAL record must still decode: %v", err)
	}
	if legacy.At != 0 {
		t.Errorf("a record without `at` must decode to 0, got %d", legacy.At)
	}
}

// TestRecoverFromWAL_StampsTheRecordedInstant reads the production source
// directly, so it cannot drift the way a behavioural model could. It asserts
// the one line that mattered: the recovery closures must stamp the recorded
// instant, never the recovering node's own clock.
func TestRecoverFromWAL_StampsTheRecordedInstant(t *testing.T) {
	body := functionBodyFromSource(t, "transfer_wal.go", "func (cs *ChainState) recoverFromWAL(")

	if strings.Contains(body, "touchActivity(acc)") {
		t.Error("recoverFromWAL still calls touchActivity(acc), which stamps nowUnix() — the " +
			"recovering node's restart time. It must call touchActivityAt(acc, at) with the " +
			"instant recorded in the WAL record, matching what block replay already does.")
	}
	if !strings.Contains(body, "touchActivityAt(acc, at)") {
		t.Error("recoverFromWAL no longer stamps the activity clock from the record's own " +
			"timestamp; the demurrage clock of every recovered account is only as correct as " +
			"whatever it stamps instead.")
	}
	// And the value has to come from the record rather than being recomputed.
	if !strings.Contains(body, "rec.At") {
		t.Error("recoverFromWAL does not read rec.At — the recorded instant is being ignored " +
			"even if the record still carries it.")
	}
}
