package keeper

import "testing"

// TestStartChainDivergenceCheck_NoopWithoutPrimaryURL verifies the check
// never touches dag.state (which is nil in this minimal test DAG) when
// PRIMARY_NODE_URL isn't configured — it must return before starting the
// goroutine, not panic or spin up a ticker that dereferences a nil state.
func TestStartChainDivergenceCheck_NoopWithoutPrimaryURL(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.startChainDivergenceCheck("")
}
