package keeper

import (
	"strings"
	"testing"
)

// Reproduces the configuration that was live on 2026-08-15: Contabo1, the
// authoritative chain past 3.8 million blocks, had BOOTSTRAP_SNAPSHOT_URL
// pointing at Contabo2, and Contabo2 was serving height 1. Its log carries a
// boot where it tried to replace its state from there. It failed on an HTTP 429,
// and the only other obstacle was that BOOTSTRAP_SIGNER matched neither node's
// actual signing key — two accidents outside this code.
//
// The guard turns that into a rule. These pin the decision itself, so it does
// not depend on being able to stand up two nodes.

func TestResyncGuard_RefusesASnapshotFarBehindLocalHeight(t *testing.T) {
	const local = 3_800_000
	cases := []struct {
		name       string
		snapHeight int64
		wantRefuse bool
	}{
		{"the live near-miss: an empty chain against 3.8M blocks", 1, true},
		{"a stale but plausible peer", local - 5000, true},
		{"just outside the tolerance", local - resyncBackwardsTolerance - 1, true},
		{"just inside the tolerance — normal staleness during a fetch", local - resyncBackwardsTolerance + 1, false},
		{"exactly level", local, false},
		{"ahead, which is what resync is actually for", local + 250_000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refuse := tc.snapHeight+resyncBackwardsTolerance < local
			if refuse != tc.wantRefuse {
				t.Fatalf("snapshot height %d against local %d: refuse=%v, want %v",
					tc.snapHeight, local, refuse, tc.wantRefuse)
			}
		})
	}
}

// The guard must not fire on a node that has no height yet — a genuine
// first-time bootstrap is exactly the case resync exists to serve, and it must
// not be blocked by its own protection.
func TestResyncGuard_DisabledWhenLocalHeightUnknown(t *testing.T) {
	cs := newTestState() // no DB configured
	if h := cs.localChainHeightForResync(); h != 0 {
		t.Fatalf("local height without a database = %d, want 0 (guard disabled)", h)
	}
}

// The refusal has to say what it refused and how to override it deliberately —
// an operator meeting this at 3am needs the numbers, not just a "no".
func TestResyncGuard_MessageNamesTheNumbersAndTheOverride(t *testing.T) {
	const local, snap = int64(3_800_000), int64(1)
	msg := resyncRegressionMessage("http://peer:8080/api/snapshot", snap, local)
	for _, want := range []string{"3800000", "1", "ALLOW_RESYNC_REGRESSION", "3799999"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal message does not mention %q:\n  %s", want, msg)
		}
	}
}
