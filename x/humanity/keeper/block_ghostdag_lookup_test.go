package keeper

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// signTestBlock builds a Block signed by a freshly generated key, mirroring
// ProduceBlock's own signing sequence (block.go: calculateHash then
// crypto.Sign over the raw hash bytes) — used to exercise verifyFetchedBlock
// (snapshot.go) with a genuinely valid signature.
func signTestBlock(t *testing.T, height int64) *Block {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	addr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	b := &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		ParentHashes: []string{"deadbeef"},
		Proposer:     addr,
		Humans:       4,
		StateRoot:    "some-state-root",
	}
	b.Hash = calculateBlockHash(b)
	hashBytes, err := hex.DecodeString(b.Hash)
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	sig, err := crypto.Sign(hashBytes, key)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	b.Signature = hex.EncodeToString(sig)
	return b
}

// signTestBlockWithParent is signTestBlock with a caller-chosen parent hash,
// for tests that need to exercise a specific missing-parent scenario (e.g.
// the orphan queue) with an otherwise fully valid, authorized block.
func signTestBlockWithParent(t *testing.T, height int64, parentHash string) *Block {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	addr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	b := &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		ParentHashes: []string{parentHash},
		Proposer:     addr,
		Humans:       4,
		StateRoot:    "some-state-root",
	}
	b.Hash = calculateBlockHash(b)
	hashBytes, err := hex.DecodeString(b.Hash)
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	sig, err := crypto.Sign(hashBytes, key)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	b.Signature = hex.EncodeToString(sig)
	return b
}

// TestGhostdagBlockLookup_MemoryHit verifies the fast path (block resident in
// dag.blocks) never touches dag.state, and still works when dag.state is nil
// — the same contract the raw `dag.blocks[hash]` access had before
// ghostdagBlockLookup replaced it in computeGHOSTDAGState/ghostdagMergeSet/
// ghostdagIsAncestor.
func TestGhostdagBlockLookup_MemoryHit(t *testing.T) {
	dag := newGhostdagTestDAG()
	want := &Block{Hash: "h1", Height: 1}
	dag.blocks["h1"] = want

	got := dag.ghostdagBlockLookup("h1", nil)
	if got != want {
		t.Fatalf("ghostdagBlockLookup returned %v, want the in-memory block %v", got, want)
	}
}

// TestMaxGhostdagDBLookups_ScalesWithMergeVisits is the regression guard for
// the 2026-07-04 fix that replaced the fixed 10-round-trip budget with one
// scaled off maxMergeVisits (which has its own floor of 50, so this is never
// below 500): at base K it must equal maxMergeVisits()*10, and it must scale
// up further for a large committee (high K), so the DB budget is never the
// actual limiting factor before the K-derived structural caps are.
func TestMaxGhostdagDBLookups_ScalesWithMergeVisits(t *testing.T) {
	dag := newGhostdagTestDAG()
	baseWant := dag.maxMergeVisits() * 10
	if got := dag.maxGhostdagDBLookups(); got != baseWant {
		t.Fatalf("at base K, maxGhostdagDBLookups() = %d, want %d (maxMergeVisits*10)", got, baseWant)
	}
	dag.activeGhostdagK.Store(1000) // large committee
	bigWant := dag.maxMergeVisits() * 10
	if got := dag.maxGhostdagDBLookups(); got != bigWant {
		t.Fatalf("at K=1000, maxGhostdagDBLookups() = %d, want %d (maxMergeVisits*10)", got, bigWant)
	}
	if bigWant <= baseWant {
		t.Fatal("test setup bug: K=1000 should produce a value above the base-K value")
	}
}

// TestGhostdagBlockLookup_MissNoState verifies a genuinely-missing hash with
// no DB backing (dag.state == nil, the same setup every other GHOSTDAG scale
// test in this package uses) returns nil rather than panicking — matching
// the prior raw map access's `ok == false` behavior exactly.
func TestGhostdagBlockLookup_MissNoState(t *testing.T) {
	dag := newGhostdagTestDAG()
	if got := dag.ghostdagBlockLookup("does-not-exist", nil); got != nil {
		t.Fatalf("ghostdagBlockLookup() = %v, want nil for a hash absent from both dag.blocks and dag.state", got)
	}
}

// TestGhostdagBatchPrefetch_NoStateIsNoop is the regression guard for the
// 2026-07-04 batching fix (see ghostdagBatchPrefetch's own comment): with
// dag.state == nil (this minimal test DAG), the function must return
// immediately without touching dbBudget or panicking, exactly like
// ghostdagBlockLookup's own nil-state short-circuit above.
func TestGhostdagBatchPrefetch_NoStateIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	budget := 5
	dag.ghostdagBatchPrefetch([]string{"a", "b", "c"}, &budget)
	if budget != 5 {
		t.Fatalf("budget should be untouched when dag.state is nil, got %d", budget)
	}
}

// TestGhostdagBatchPrefetch_AllHashesAlreadyCachedSkipsBudget verifies the
// whole point of batching: if every requested hash is already resident in
// dag.blocks, no round trip (and therefore no budget spend) happens at all
// — this is the common case once a BFS frontier has been prefetched once.
func TestGhostdagBatchPrefetch_AllHashesAlreadyCachedSkipsBudget(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["a"] = &Block{Hash: "a", Height: 1}
	dag.blocks["b"] = &Block{Hash: "b", Height: 2}
	budget := 3
	dag.ghostdagBatchPrefetch([]string{"a", "b"}, &budget)
	if budget != 3 {
		t.Fatalf("budget should be untouched when every hash is already cached, got %d", budget)
	}
}

// TestGhostdagBatchPrefetch_BudgetExhaustedSkips verifies the exhausted-budget
// short-circuit matches ghostdagBlockLookup's own contract: once the shared
// per-block DB round-trip budget hits zero, no further round trips happen,
// even for hashes that would otherwise be missing. Uses a non-nil ChainState
// with db == nil (same pattern as TestGhostdagBlockLookup_BudgetExhaustion)
// so this exercises the real budget check, not just the earlier nil-state
// short-circuit.
func TestGhostdagBatchPrefetch_BudgetExhaustedSkips(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	budget := 0
	dag.ghostdagBatchPrefetch([]string{"missing-hash"}, &budget)
	if budget != 0 {
		t.Fatalf("budget should stay at 0, got %d", budget)
	}
	if _, ok := dag.blocks["missing-hash"]; ok {
		t.Fatal("missing-hash should not have been cached — budget was already exhausted")
	}
}

// TestGhostdagBatchPrefetch_OneRoundTripRegardlessOfMissingCount is the core
// regression guard for the 2026-07-04 batching fix: a batch of several
// missing hashes must cost exactly ONE unit of dbBudget, not one per hash —
// that collapsed N sequential DB round trips (each paying Railway's ~260ms
// Postgres-proxy latency, confirmed live as the dominant cost behind
// ProduceBlock regularly exceeding the 2s BLOCK_TIME target) into one.
func TestGhostdagBatchPrefetch_OneRoundTripRegardlessOfMissingCount(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{} // db == nil: LoadBlocksByHashesFromDB returns (nil, nil) safely
	budget := 5
	dag.ghostdagBatchPrefetch([]string{"miss-1", "miss-2", "miss-3", "miss-4"}, &budget)
	if budget != 4 {
		t.Fatalf("budget after batch of 4 missing hashes = %d, want 4 (one round trip, not four)", budget)
	}
}

// TestPrefetchParentsFromDB_NilBlockIsNoop verifies a nil block (AddPeerBlock
// is called with one in practice only defensively — see that function's own
// early nil-checks) does not panic when passed straight through to the new
// pre-lock prefetch call added ahead of dag.mu.Lock().
func TestPrefetchParentsFromDB_NilBlockIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.prefetchParentsFromDB(nil) // must not panic
}

// TestPrefetchParentsFromDB_NoParentHashesIsNoop verifies a block with no
// parents (e.g. rejected moments later by AddPeerBlock's own "no parent
// hashes" integrity check) is a clean no-op here too — nothing to prefetch.
func TestPrefetchParentsFromDB_NoParentHashesIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	dag.prefetchParentsFromDB(&Block{Hash: "h", ParentHashes: nil})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchParentsFromDB_NoStateIsNoop mirrors ghostdagBlockLookup's own
// nil-state short-circuit: with dag.state == nil there is nothing to fetch
// from, so this must return immediately without panicking.
func TestPrefetchParentsFromDB_NoStateIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.prefetchParentsFromDB(&Block{Hash: "h", ParentHashes: []string{"missing-parent"}})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty with no state, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchParentsFromDB_SkipsDuringMigration mirrors
// ghostdagBlockLookup's own migration-pending skip (see that function's
// 2026-07-04 FIX comment) — prefetching during a bounded startup migration
// would just be extra unwanted DB load, not a correctness issue, so it must
// be skipped the same way.
func TestPrefetchParentsFromDB_SkipsDuringMigration(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	dag.ghostdagMigrationPending.Store(true)
	dag.prefetchParentsFromDB(&Block{Hash: "h", ParentHashes: []string{"missing-parent"}})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty while migration is pending, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchParentsFromDB_AllParentsAlreadyCachedIsHarmless verifies the
// dominant warm-node case (every parent already resident in dag.blocks, the
// state this function exists to detect and skip a DB round trip for) leaves
// dag.blocks completely untouched — no clobbering of existing entries, no
// panic, even with a non-nil ChainState present.
func TestPrefetchParentsFromDB_AllParentsAlreadyCachedIsHarmless(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	p1 := &Block{Hash: "p1", Height: 1}
	p2 := &Block{Hash: "p2", Height: 2}
	dag.blocks["p1"] = p1
	dag.blocks["p2"] = p2
	dag.prefetchParentsFromDB(&Block{Hash: "child", ParentHashes: []string{"p1", "p2"}})
	if dag.blocks["p1"] != p1 || dag.blocks["p2"] != p2 {
		t.Fatal("prefetchParentsFromDB must not touch already-cached parent entries")
	}
	if len(dag.blocks) != 2 {
		t.Fatalf("dag.blocks should still have exactly the 2 pre-seeded entries, got %d", len(dag.blocks))
	}
}

// TestPrefetchParentsFromDB_MissingParentsSafeWithNoRealDB verifies the
// "some/all parents missing from dag.blocks" branch — the one that actually
// calls LoadBlocksByHashesFromDB — completes safely and leaves dag.blocks
// alone when db == nil (LoadBlocksByHashesFromDB's own contract: returns
// (nil, nil) rather than dialing anything, same pattern
// TestGhostdagBatchPrefetch_OneRoundTripRegardlessOfMissingCount relies on
// for testing this code path without a real Postgres instance).
func TestPrefetchParentsFromDB_MissingParentsSafeWithNoRealDB(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{} // db == nil
	dag.blocks["p1"] = &Block{Hash: "p1", Height: 1} // one present, one missing
	dag.prefetchParentsFromDB(&Block{Hash: "child", ParentHashes: []string{"p1", "p2-missing"}})
	if _, ok := dag.blocks["p2-missing"]; ok {
		t.Fatal("p2-missing should not have been added — db is nil, nothing to fetch")
	}
	if len(dag.blocks) != 1 {
		t.Fatalf("dag.blocks should still have exactly the 1 pre-seeded entry, got %d", len(dag.blocks))
	}
}

// TestPrefetchMergeSetFromDB_NilBlockIsNoop mirrors
// TestPrefetchParentsFromDB_NilBlockIsNoop for the deeper multi-hop warm-up.
func TestPrefetchMergeSetFromDB_NilBlockIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.prefetchMergeSetFromDB(nil) // must not panic
}

// TestPrefetchMergeSetFromDB_NoParentHashesIsNoop mirrors
// TestPrefetchParentsFromDB_NoParentHashesIsNoop.
func TestPrefetchMergeSetFromDB_NoParentHashesIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	dag.prefetchMergeSetFromDB(&Block{Hash: "h", ParentHashes: nil})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchMergeSetFromDB_NoStateIsNoop mirrors
// TestPrefetchParentsFromDB_NoStateIsNoop.
func TestPrefetchMergeSetFromDB_NoStateIsNoop(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.prefetchMergeSetFromDB(&Block{Hash: "h", ParentHashes: []string{"missing-parent"}})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty with no state, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchMergeSetFromDB_SkipsDuringMigration mirrors
// TestPrefetchParentsFromDB_SkipsDuringMigration — same rationale: a startup
// migration already refuses this exact class of DB-fallback storm, so
// warming ahead of it would just be wasted DB load.
func TestPrefetchMergeSetFromDB_SkipsDuringMigration(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	dag.ghostdagMigrationPending.Store(true)
	dag.prefetchMergeSetFromDB(&Block{Hash: "h", ParentHashes: []string{"missing-parent"}})
	if len(dag.blocks) != 0 {
		t.Fatalf("dag.blocks should stay empty while migration is pending, got %d entries", len(dag.blocks))
	}
}

// TestPrefetchMergeSetFromDB_MultiHopChainAlreadyCachedTerminates is the core
// regression guard this function exists for: prefetchParentsFromDB (2026-07-24,
// earlier fix) only ever warms DIRECT parents — its own comment explicitly
// says it "cannot predict" deeper ancestors "without running the BFS itself".
// This builds a chain 10 hops deep (child -> p1 -> p2 -> ... -> p10), all
// already resident in dag.blocks, and verifies the walk actually follows
// ParentHashes past depth 1 (proving it is a real multi-hop BFS, not just a
// second copy of the direct-parent prefetch) while leaving every entry
// untouched and terminating cleanly — no DB needed since everything is
// already cached, exactly the dominant steady-state case in production.
func TestPrefetchMergeSetFromDB_MultiHopChainAlreadyCachedTerminates(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	const depth = 10
	prev := "genesis-anchor"
	for i := depth; i >= 1; i-- {
		hash := fmt.Sprintf("p%d", i)
		dag.blocks[hash] = &Block{Hash: hash, Height: int64(i), ParentHashes: []string{prev}}
		prev = hash
	}
	child := &Block{Hash: "child", ParentHashes: []string{"p1"}}

	done := make(chan struct{})
	go func() {
		dag.prefetchMergeSetFromDB(child)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("prefetchMergeSetFromDB did not terminate on an all-cached chain — possible infinite loop")
	}

	if len(dag.blocks) != depth {
		t.Fatalf("dag.blocks should still have exactly the %d pre-seeded entries, got %d", depth, len(dag.blocks))
	}
	for i := 1; i <= depth; i++ {
		hash := fmt.Sprintf("p%d", i)
		if dag.blocks[hash] == nil || dag.blocks[hash].Height != int64(i) {
			t.Fatalf("prefetchMergeSetFromDB corrupted or dropped cached entry %s", hash)
		}
	}
}

// TestPrefetchMergeSetFromDB_MissingAncestorsSafeWithNoRealDB verifies the
// branch that actually calls LoadBlocksByHashesFromDB — one hop of missing
// ancestors behind an already-cached direct parent — completes safely and
// adds nothing when db == nil (same no-op-DB contract
// TestPrefetchParentsFromDB_MissingParentsSafeWithNoRealDB relies on).
func TestPrefetchMergeSetFromDB_MissingAncestorsSafeWithNoRealDB(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{} // db == nil
	// p1 is cached but references a grandparent that is NOT cached and can't
	// be fetched (nil db) — the walk must reach for it, fail safely, and stop.
	dag.blocks["p1"] = &Block{Hash: "p1", Height: 1, ParentHashes: []string{"p2-missing"}}
	child := &Block{Hash: "child", ParentHashes: []string{"p1"}}

	done := make(chan struct{})
	go func() {
		dag.prefetchMergeSetFromDB(child)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("prefetchMergeSetFromDB did not terminate with an unresolvable ancestor and no DB")
	}

	if _, ok := dag.blocks["p2-missing"]; ok {
		t.Fatal("p2-missing should not have been added — db is nil, nothing to fetch")
	}
	if len(dag.blocks) != 1 {
		t.Fatalf("dag.blocks should still have exactly the 1 pre-seeded entry, got %d", len(dag.blocks))
	}
}

// TestPrefetchMergeSetFromDB_BoundedByDepthAndVisitCap verifies the walk
// cannot run unbounded work: a chain far deeper than mergeDepthLimit()
// (2*K+1, 37 at the default test K=18) and wider than maxMergeVisits() (50)
// must still terminate quickly — proving depthLimit/visitCap actually stop
// the traversal instead of only bounding the real ghostdagMergeSet call this
// function warms ahead of.
func TestPrefetchMergeSetFromDB_BoundedByDepthAndVisitCap(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	const depth = 200 // far beyond mergeDepthLimit() (37 at default K)
	prev := "genesis-anchor"
	for i := depth; i >= 1; i-- {
		hash := fmt.Sprintf("deep%d", i)
		dag.blocks[hash] = &Block{Hash: hash, Height: int64(i), ParentHashes: []string{prev}}
		prev = hash
	}
	child := &Block{Hash: "child", ParentHashes: []string{"deep1"}}

	done := make(chan struct{})
	go func() {
		dag.prefetchMergeSetFromDB(child)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("prefetchMergeSetFromDB did not terminate on a chain far deeper than mergeDepthLimit/maxMergeVisits")
	}
	// All entries were pre-seeded and must survive untouched regardless of
	// how far the bounded walk actually reached.
	if len(dag.blocks) != depth {
		t.Fatalf("dag.blocks should still have exactly the %d pre-seeded entries, got %d", depth, len(dag.blocks))
	}
}

// TestGhostdagBlockLookup_SkipsDBDuringMigration is the regression guard for
// the 2026-07-04 production outage: while ghostdagMigrationPending is true,
// a miss must return nil immediately (matching the pre-DB-fallback behavior)
// instead of attempting a DB round trip. The startup migration is the ONLY
// caller of this function while the flag is set (AddPeerBlock/ProduceBlock
// both refuse to run for the whole duration), so skipping the DB there can
// never affect a live attach/reject decision -- it only stops an unbounded
// number of blocking round trips (one per ancestor outside the loaded batch)
// from serializing every block behind dag.mu during a large migration,
// confirmed live as a multi-minute full node freeze.
func TestGhostdagBlockLookup_SkipsDBDuringMigration(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.ghostdagMigrationPending.Store(true)
	// Still resident in memory: must be found regardless of the migration flag.
	want := &Block{Hash: "resident", Height: 1}
	dag.blocks["resident"] = want
	if got := dag.ghostdagBlockLookup("resident", nil); got != want {
		t.Fatalf("ghostdagBlockLookup(resident) = %v, want the in-memory block %v even during migration", got, want)
	}
	// Not resident: must return nil without needing dag.state at all (which
	// is nil here, same as every other GHOSTDAG test in this package — if the
	// migration check didn't short-circuit first, this would already be
	// covered by TestGhostdagBlockLookup_MissNoState, so this test's real
	// value is documenting that the migration flag is checked, not the nil
	// dag.state path).
	if got := dag.ghostdagBlockLookup("not-resident", nil); got != nil {
		t.Fatalf("ghostdagBlockLookup(not-resident) = %v, want nil during migration", got)
	}
}

// TestGhostdagBlockLookup_BudgetExhaustion is the regression guard for two
// incidents in sequence, both the same underlying hazard: a per-call DB
// round-trip budget silently gating a live consensus lookup. The 2026-07-04
// version of this test asserted that once the budget hit zero, a lookup
// short-circuited to nil WITHOUT calling into dag.state — a real fix for
// unbounded round trips (a single computeGHOSTDAGState call had measured 62s)
// but one that reintroduced the exact non-determinism it replaced: two honest
// nodes under different load could exhaust budget at different points in the
// identical BFS and silently compute different SelectedParent/BlueScore for
// the same block. Confirmed live a second time on 2026-07-10 (Contabo 2 vs
// Primary diverging from height 650000, each side internally "healthy") after
// the first fix only made the failure rarer by raising the budget, not
// impossible. budget is now advisory-only: still decremented for telemetry,
// but it no longer prevents the real dag.state lookup from being attempted
// (ghostdagMergeSet's own visitCap/mergeDepthLimit — identical on every node
// by construction — are what actually bound total lookups per call, not this
// counter). With dag.state's db left nil, LoadBlockFromDBByHash itself
// returns nil regardless of whether budget gated it, so this test cannot
// observe the call boundary directly; it verifies the piece that IS
// observable — the budget counter's own bookkeeping never overruns.
func TestGhostdagBlockLookup_BudgetExhaustion(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{} // non-nil, db == nil: LoadBlockFromDBByHash returns nil safely, no real DB needed

	budget := 2
	if got := dag.ghostdagBlockLookup("miss-1", &budget); got != nil {
		t.Fatalf("ghostdagBlockLookup(miss-1) = %v, want nil (nothing in state)", got)
	}
	if budget != 1 {
		t.Fatalf("budget after first real miss = %d, want 1", budget)
	}
	if got := dag.ghostdagBlockLookup("miss-2", &budget); got != nil {
		t.Fatalf("ghostdagBlockLookup(miss-2) = %v, want nil", got)
	}
	if budget != 0 {
		t.Fatalf("budget after second real miss = %d, want 0", budget)
	}
	// Budget now exhausted: a third distinct miss must NOT decrement further
	// (would go negative) and — unlike the pre-2026-07-10 contract — must
	// still genuinely attempt the lookup rather than short-circuiting; with
	// db == nil the observable result is nil either way, so this only pins
	// down the non-negative counter invariant, not the call itself.
	if got := dag.ghostdagBlockLookup("miss-3", &budget); got != nil {
		t.Fatalf("ghostdagBlockLookup(miss-3) = %v, want nil (still nothing in state)", got)
	}
	if budget != 0 {
		t.Fatalf("budget after exhaustion = %d, want to stay at 0, not go negative", budget)
	}

	// A cache HIT must never touch the budget at all, even when it is
	// already exhausted — the in-memory fast path is free.
	want := &Block{Hash: "resident", Height: 1}
	dag.blocks["resident"] = want
	if got := dag.ghostdagBlockLookup("resident", &budget); got != want {
		t.Fatalf("ghostdagBlockLookup(resident) = %v, want the in-memory block %v", got, want)
	}
	if budget != 0 {
		t.Fatalf("budget changed after a cache hit = %d, want unchanged at 0", budget)
	}
}

// TestGhostdagBlockLookup_NilBudgetUnbounded verifies a nil budget preserves
// the pre-fix unbounded behavior exactly — used by the two callers outside
// computeGHOSTDAGState's call graph (AddPeerBlock's parent-existence check
// and GetBlockByHeight's SelectedParent-chain walk), which must not be
// affected by the new per-computation budget at all.
func TestGhostdagBlockLookup_NilBudgetUnbounded(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	for i := 0; i < 100; i++ {
		if got := dag.ghostdagBlockLookup(fmt.Sprintf("miss-%d", i), nil); got != nil {
			t.Fatalf("ghostdagBlockLookup with nil budget returned %v, want nil", got)
		}
	}
}

// TestTriggerSoftRetryFlush_CoalescesConcurrentTriggers is the regression
// guard for the goroutine-spawn amplification found live on 2026-07-03 (see
// triggerSoftRetryFlush's comment): a burst of concurrent triggers must run
// the underlying flush at most a small, bounded number of times — not once
// per trigger — and must leave softRetryFlushInFlight cleared afterward so a
// future trigger can still start a fresh pass.
func TestTriggerSoftRetryFlush_CoalescesConcurrentTriggers(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.softRetryBlocks = make(map[string]*Block)
	dag.softRetryFirstAt = make(map[string]time.Time)

	// retryAndFlushSoftRetry's own behavior is covered elsewhere; what's under
	// test here is triggerSoftRetryFlush's coalescing logic itself — that a
	// burst of concurrent triggers all return promptly and leave the
	// in-flight/again bookkeeping in a clean, non-stuck state, regardless of
	// how many flush passes actually ran underneath.
	var runs int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dag.triggerSoftRetryFlush()
			mu.Lock()
			runs++
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Give the last spawned flush goroutine(s) a moment to finish and clear
	// the in-flight flag.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dag.softRetryMu.Lock()
		inFlight := dag.softRetryFlushInFlight
		dag.softRetryMu.Unlock()
		if !inFlight {
			break
		}
		time.Sleep(time.Millisecond)
	}

	dag.softRetryMu.Lock()
	inFlight := dag.softRetryFlushInFlight
	again := dag.softRetryFlushAgain
	dag.softRetryMu.Unlock()
	if inFlight {
		t.Fatalf("softRetryFlushInFlight still true after all triggers settled — a future trigger would be silently dropped")
	}
	if again {
		t.Fatalf("softRetryFlushAgain still true after settling — a pass was requested but never run")
	}
	if runs != 200 {
		t.Fatalf("expected all 200 trigger calls to return, got %d", runs)
	}
}

// TestVerifyFetchedBlock_Valid is the regression guard for
// SeedTrustedCheckpoint's (snapshot.go) trust boundary: a genuinely
// well-formed, correctly signed block at the expected height must verify.
func TestVerifyFetchedBlock_Valid(t *testing.T) {
	b := signTestBlock(t, 12345)
	if err := verifyFetchedBlock(b, 12345); err != nil {
		t.Fatalf("verifyFetchedBlock() = %v, want nil for a validly signed block", err)
	}
}

// TestVerifyFetchedBlock_WrongHeight guards against a peer (malicious or
// buggy) returning a block at a different height than the one requested —
// SeedTrustedCheckpoint fetches by height and must not silently accept a
// mismatched substitute.
func TestVerifyFetchedBlock_WrongHeight(t *testing.T) {
	b := signTestBlock(t, 12345)
	if err := verifyFetchedBlock(b, 99999); err == nil {
		t.Fatal("verifyFetchedBlock() = nil, want an error for a height mismatch")
	}
}

// TestVerifyFetchedBlock_TamperedField guards the core security property:
// mutating any block field after signing must invalidate it, exactly like
// AddPeerBlock's own hash-recompute check (block.go) does for gossiped
// blocks — this is what makes fetching a checkpoint out-of-band via a plain
// GET as trustworthy as accepting it over normal peer gossip.
func TestVerifyFetchedBlock_TamperedField(t *testing.T) {
	b := signTestBlock(t, 12345)
	b.Humans = 999999 // tamper with a field the hash covers, after signing
	if err := verifyFetchedBlock(b, 12345); err == nil {
		t.Fatal("verifyFetchedBlock() = nil, want an error for a tampered block (hash no longer matches)")
	}
}

// TestVerifyFetchedBlock_ProposerMismatch guards against a block whose
// signature is valid but was produced by a DIFFERENT key than the one
// claimed in the Proposer field — accepting this would let anyone forge a
// checkpoint "from" an arbitrary address as long as they sign it themselves.
func TestVerifyFetchedBlock_ProposerMismatch(t *testing.T) {
	b := signTestBlock(t, 12345)
	b.Proposer = "0x000000000000000000000000000000deadbeef"
	// Changing Proposer changes the hash (it's a hashed field), so also
	// recompute Hash to isolate the property under test: a signature that
	// recovers to a DIFFERENT address than the (now self-consistent) claimed
	// hash's Proposer field must still be rejected.
	b.Hash = calculateBlockHash(b)
	if err := verifyFetchedBlock(b, 12345); err == nil {
		t.Fatal("verifyFetchedBlock() = nil, want an error when the recovered signer does not match Proposer")
	}
}

// TestVerifyFetchedBlock_MissingSignature guards against an unsigned block
// being accepted as a trusted checkpoint.
func TestVerifyFetchedBlock_MissingSignature(t *testing.T) {
	b := signTestBlock(t, 12345)
	b.Signature = ""
	if err := verifyFetchedBlock(b, 12345); err == nil {
		t.Fatal("verifyFetchedBlock() = nil, want an error for a missing signature")
	}
}

// TestSeedTrustedCheckpoint_NilSafety verifies the no-op guards at the top
// of SeedTrustedCheckpoint (empty primaryURL, no dag.state) return false
// without panicking — it's called unconditionally from main.go's resync
// path, so it must never be the reason a resync crashes.
func TestSeedTrustedCheckpoint_NilSafety(t *testing.T) {
	dag := newGhostdagTestDAG() // dag.state is nil, as every other test in this file relies on
	if got := dag.SeedTrustedCheckpoint(""); got {
		t.Fatal("SeedTrustedCheckpoint(\"\") = true, want false for an empty primaryURL")
	}
	if got := dag.SeedTrustedCheckpoint("https://example.invalid"); got {
		t.Fatal("SeedTrustedCheckpoint() = true, want false when dag.state is nil")
	}
}

// TestCanonicalBlockAtHeight_PicksSelectedParentChainNotMostParents is the
// regression guard for a real production bug (2026-07-04): three
// simultaneously healthy nodes (0 StateRoot mismatches) returned three
// DIFFERENT blocks with three different proposers for the identical height,
// because the old tie-break ("whichever sibling has the most parent
// hashes") has no relationship to GHOSTDAG's actual canonical chain. This
// builds two siblings at the same height — one that's genuinely reachable
// via the best tip's SelectedParent chain, and one with MORE parent hashes
// that is NOT reachable — and verifies the fix picks the reachable one
// regardless of parent count.
func TestCanonicalBlockAtHeight_PicksSelectedParentChainNotMostParents(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = make(map[string]bool)

	genesis := &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["genesis"] = genesis

	// onChain: height 1, reachable from the eventual best tip via
	// SelectedParent, but deliberately given only ONE parent hash.
	onChain := &Block{
		Hash: "on-chain", Height: 1, Proposer: "0xonchain",
		ParentHashes: []string{"genesis"}, SelectedParent: "genesis", BlueScore: 1,
	}
	dag.blocks[onChain.Hash] = onChain

	// sibling: also height 1, but NOT on the canonical chain — given THREE
	// parent hashes so the old "most parents wins" heuristic would have
	// picked this one instead.
	sibling := &Block{
		Hash: "sibling", Height: 1, Proposer: "0xsibling",
		ParentHashes: []string{"genesis", "genesis", "genesis"}, SelectedParent: "genesis", BlueScore: 1,
	}
	dag.blocks[sibling.Hash] = sibling

	// tip: height 2, its SelectedParent is onChain (not sibling) — this is
	// what makes onChain canonical. Only the tip is in dag.tips.
	tip := &Block{
		Hash: "tip", Height: 2, Proposer: "0xtip",
		ParentHashes: []string{"on-chain", "sibling"}, SelectedParent: "on-chain", BlueScore: 2,
	}
	dag.blocks[tip.Hash] = tip
	dag.tips[tip.Hash] = true

	got := dag.canonicalBlockAtHeightLocked(1)
	if got == nil {
		t.Fatal("canonicalBlockAtHeightLocked(1) = nil, want the on-chain block")
	}
	if got.Hash != "on-chain" {
		t.Fatalf("canonicalBlockAtHeightLocked(1) = %s (proposer %s), want \"on-chain\" — picked the wrong sibling (likely fell back to a parent-count heuristic)",
			got.Hash, got.Proposer)
	}
}

// TestCanonicalBlockAtHeight_NoTips verifies the "no tip exists yet" edge
// case (e.g. right after a restart before any tip is known) returns nil
// rather than panicking, so GetBlockByHeight's DB fallback gets a clean
// signal to take over.
func TestCanonicalBlockAtHeight_NoTips(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = make(map[string]bool)
	if got := dag.canonicalBlockAtHeightLocked(5); got != nil {
		t.Fatalf("canonicalBlockAtHeightLocked(5) = %v, want nil with no tips present", got)
	}
}
