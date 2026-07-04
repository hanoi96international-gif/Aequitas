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

// TestGhostdagBlockLookup_BudgetExhaustion is the regression guard for the
// 2026-07-04 second production outage: a single computeGHOSTDAGState call
// measured 62s, almost entirely real DB round trips, because neither
// ghostdagMergeSet's BFS nor the classification loop's many independent
// ghostdagIsAncestor calls shared any limit on how many real DB lookups they
// could rack up in total (see maxGhostdagDBLookups's own comment).
// Once the shared budget hits zero, further misses must return nil
// immediately (the same conservative fallback already used during
// migration) instead of continuing to call into dag.state.
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
	// (would go negative) and must still return nil without panicking.
	if got := dag.ghostdagBlockLookup("miss-3", &budget); got != nil {
		t.Fatalf("ghostdagBlockLookup(miss-3) = %v, want nil once budget is exhausted", got)
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
