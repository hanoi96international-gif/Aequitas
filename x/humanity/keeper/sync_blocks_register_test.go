package keeper

import "testing"

// TestRegisterAndDiscover_ReturnsFalseOnUnreachable is the regression guard
// for the 2026-07-09 fix (StartPeerDiscovery's registration retry loop):
// registerAndDiscover used to return nothing, so a caller had no way to
// tell a failed registration from a successful one and therefore no way to
// retry it — a transient failure (rate limit, network blip) left a node
// permanently stuck until a manual restart (confirmed live). This locks in
// the bool-return contract the retry loop depends on: an unreachable
// primaryURL must report failure (false), not silently succeed.
func TestRegisterAndDiscover_ReturnsFalseOnUnreachable(t *testing.T) {
	dag := newGhostdagTestDAG()
	const unreachable = "http://127.0.0.1:1" // pinningDialer rejects loopback instantly
	if ok := dag.registerAndDiscover("http://self.invalid:8080", unreachable); ok {
		t.Fatal("registerAndDiscover against an unreachable primary must return false, not true")
	}
}
