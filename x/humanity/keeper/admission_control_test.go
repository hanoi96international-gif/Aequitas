package keeper

import (
	"strings"
	"testing"
	"time"
)

// A node that cannot produce blocks must not accept transactions.
//
// Measured 2026-08-21: under sustained load the node applied 2,000-4,000
// transfers/s while its height did not move for over 15 seconds. Nothing
// stopped it accepting more, and every accepted transfer grew the backlog that
// kept the production gate shut -- backlog_vs_fork.go's "the state is a trap",
// reached from the ingest side.

func withProducedAt(t *testing.T, unix int64) {
	t.Helper()
	prev := lastBlockProducedUnix.Load()
	lastBlockProducedUnix.Store(unix)
	t.Cleanup(func() { lastBlockProducedUnix.Store(prev) })
}

func TestFreshNodeIsNeverRefusedForNeverHavingProduced(t *testing.T) {
	withProducedAt(t, 0)
	if r := admissionRefusalReason(); r != "" {
		t.Fatalf("a node that has not produced yet was refused: %q\n"+
			"  Startup is not a stall. Refusing here would make a node that has just come up "+
			"reject every transaction until its first block, which is the opposite of the "+
			"availability this protects.", r)
	}
}

func TestProducingNodeAccepts(t *testing.T) {
	withProducedAt(t, time.Now().Unix())
	if r := admissionRefusalReason(); r != "" {
		t.Fatalf("a node that just produced a block was refused: %q", r)
	}
}

func TestBriefGapDoesNotRefuse(t *testing.T) {
	// A few missed ticks are ordinary: a slow tick, one gated cycle, a
	// propagation hiccup. Refusing on those would trade a rare trap for
	// constant false rejections.
	withProducedAt(t, time.Now().Unix()-(admissionStallSeconds/2))
	if r := admissionRefusalReason(); r != "" {
		t.Fatalf("a %ds gap was refused: %q — that is normal jitter at BLOCK_TIME=1s",
			admissionStallSeconds/2, r)
	}
}

func TestSustainedStallRefuses(t *testing.T) {
	withProducedAt(t, time.Now().Unix()-(admissionStallSeconds+5))
	r := admissionRefusalReason()
	if r == "" {
		t.Fatalf("a %ds production stall was still accepting transactions.\n"+
			"  This is exactly the state that cannot recover on its own: the node cannot "+
			"include what it accepts, the backlog grows, and the gate that stopped production "+
			"stays shut because of it.", admissionStallSeconds+5)
	}
	// The message reaches an end user, so it has to say what to do rather than
	// name an internal gate.
	for _, want := range []string{"retry", "another validator"} {
		if !strings.Contains(r, want) {
			t.Errorf("refusal message %q does not mention %q — a client needs to know it can "+
				"retry or go elsewhere, otherwise a retryable condition reads as a failure", r, want)
		}
	}
}

func TestStallLimitIsOverridable(t *testing.T) {
	t.Setenv("AEQUITAS_ADMIT_STALL_SECONDS", "5")
	if got := admissionStallLimit(); got != 5 {
		t.Fatalf("override gave %d, want 5", got)
	}
	withProducedAt(t, time.Now().Unix()-10)
	if admissionRefusalReason() == "" {
		t.Error("a 10s stall was accepted under a 5s limit — the override did not take effect")
	}
}

func TestBadOverrideKeepsTheProtection(t *testing.T) {
	// Same rule as rpcRateLimitMax's own override: a typo must never turn a
	// protection off. Silently falling back to "no limit" is how a guard
	// disappears without anyone noticing.
	for _, bad := range []string{"", "abc", "0", "-1", "12.5"} {
		t.Setenv("AEQUITAS_ADMIT_STALL_SECONDS", bad)
		if got := admissionStallLimit(); got != admissionStallSeconds {
			t.Errorf("override %q gave %d, want the default %d — an unusable value must not "+
				"weaken or disable the stall check", bad, got, admissionStallSeconds)
		}
	}
}

func TestAdmissionStatsReportTheDecision(t *testing.T) {
	withProducedAt(t, time.Now().Unix()-(admissionStallSeconds+5))
	st := AdmissionStats()
	if st["refusing"] != true {
		t.Errorf("stats say refusing=%v while the node is refusing — an operator has to be able "+
			"to tell a refusal from a rate limit without reading logs", st["refusing"])
	}
	if s, ok := st["stalled_seconds"].(int64); !ok || s < int64(admissionStallSeconds) {
		t.Errorf("stalled_seconds is %v, want at least %d", st["stalled_seconds"], admissionStallSeconds)
	}
}
