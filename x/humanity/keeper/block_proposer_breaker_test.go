package keeper

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// TestProposerBreaker_TripsAfterThreshold verifies a proposer whose blocks never
// attach trips exactly at the threshold and is then dropped lock-free.
func TestProposerBreaker_TripsAfterThreshold(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xbadf00d"

	for i := 0; i < proposerBreakerFailThreshold-1; i++ {
		dag.recordProposerOutcome(p, false)
		if dag.proposerBlockBlocked(p) {
			t.Fatalf("breaker tripped early after %d failures (threshold %d)", i+1, proposerBreakerFailThreshold)
		}
	}
	dag.recordProposerOutcome(p, false) // the threshold-th failure trips it
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker did not trip after %d consecutive failures", proposerBreakerFailThreshold)
	}
	// Case-insensitive: the same identity in a different case is still blocked.
	if !dag.proposerBlockBlocked("0xBADF00D") {
		t.Fatalf("breaker must key on the lowercased proposer")
	}
}

// TestProposerBreaker_SuccessResetsRun verifies a single attaching block clears
// the failure run so a healthy proposer never trips.
func TestProposerBreaker_SuccessResetsRun(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xhealthy"

	for cycle := 0; cycle < 5; cycle++ {
		for i := 0; i < proposerBreakerFailThreshold-1; i++ {
			dag.recordProposerOutcome(p, false)
		}
		dag.recordProposerOutcome(p, true) // attaches — resets the run
		if dag.proposerBlockBlocked(p) {
			t.Fatalf("healthy proposer tripped: a success must reset the run (cycle %d)", cycle)
		}
	}
}

// TestProposerBreaker_CooldownExpiryAllowsProbe verifies that once the cooldown
// elapses the proposer is allowed through again and its state is cleared.
func TestProposerBreaker_CooldownExpiryAllowsProbe(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xdiverged"

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker should be open immediately after tripping")
	}
	dag.proposerBreaker.mu.Lock()
	dag.proposerBreaker.breakerUntil[p] = time.Now().Add(-time.Second).UnixNano() // force cooldown expired
	dag.proposerBreaker.mu.Unlock()

	if dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker should let a probe through after the cooldown expires")
	}
	dag.proposerBreaker.mu.Lock()
	run := dag.proposerBreaker.failRun[p]
	_, stillOpen := dag.proposerBreaker.breakerUntil[p]
	dag.proposerBreaker.mu.Unlock()
	// Bounded reopen (P0 fix, 2026-07-02 liveness audit; widened from a single
	// probe to proposerBreakerReopenProbes on 2026-07-04 — see that constant's
	// comment for the live outage that motivated it): cooldown expiry clears
	// the until-map and seeds the run proposerBreakerReopenProbes short of the
	// threshold, not at 0 — up to that many outcomes decide, not another full
	// run of fresh failures.
	want := proposerBreakerFailThreshold - proposerBreakerReopenProbes
	if run != want || stillOpen {
		t.Fatalf("cooldown expiry must clear until and seed run at threshold-reopenProbes (run=%d want=%d open=%v)",
			run, want, stillOpen)
	}
}

// TestProposerBreaker_ReopenRetripsAfterProbesExhausted verifies the cooldown
// reopen is a BOUNDED reopen, not unlimited: fewer than proposerBreakerReopenProbes
// failing blocks right after cooldown expiry must NOT re-trip the breaker, but
// exactly proposerBreakerReopenProbes failures must. This is the balance this
// constant strikes: forgiving enough that one unlucky probe (a transient
// gossip-ordering blip, unrelated to the proposer actually being diverged —
// confirmed live as a repeated real outage with the old single-probe
// behavior) doesn't cost another full 30s cooldown, while still bounded
// enough that a genuinely diverged proposer re-trips within a handful of
// blocks, not a full fresh run of proposerBreakerFailThreshold.
func TestProposerBreaker_ReopenRetripsAfterProbesExhausted(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xrelapsed"

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	dag.proposerBreaker.mu.Lock()
	dag.proposerBreaker.breakerUntil[p] = time.Now().Add(-time.Second).UnixNano() // force cooldown expired
	dag.proposerBreaker.mu.Unlock()

	if dag.proposerBlockBlocked(p) {
		t.Fatalf("precondition: a probe should be let through after cooldown expiry")
	}
	for i := 0; i < proposerBreakerReopenProbes-1; i++ {
		dag.recordProposerOutcome(p, false)
		if dag.proposerBlockBlocked(p) {
			t.Fatalf("re-tripped after only %d of %d allotted reopen probes failed — reopen is too narrow again", i+1, proposerBreakerReopenProbes)
		}
	}
	dag.recordProposerOutcome(p, false) // the proposerBreakerReopenProbes-th failure
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker did not re-trip after all %d reopen probes failed", proposerBreakerReopenProbes)
	}
}

// TestProposerBreaker_ProbeSuccessClearsRun verifies the cooldown reopen still
// lets a genuinely-healed proposer back in on a single success, matching
// recordProposerOutcome's existing success branch.
func TestProposerBreaker_ProbeSuccessClearsRun(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xhealed"

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	dag.proposerBreaker.mu.Lock()
	dag.proposerBreaker.breakerUntil[p] = time.Now().Add(-time.Second).UnixNano() // force cooldown expired
	dag.proposerBreaker.mu.Unlock()

	if dag.proposerBlockBlocked(p) {
		t.Fatalf("precondition: probe should be let through after cooldown expiry")
	}
	dag.recordProposerOutcome(p, true) // the single probe attaches
	if dag.proposerBlockBlocked(p) {
		t.Fatalf("a single successful probe must clear the run, not leave the proposer blocked")
	}
}

// TestProposerBreaker_EmptyProposerNeverBlocks guards the nil/empty case so an
// unsigned or malformed block can never trip or be gated by the breaker (it is
// rejected by the signature/parent checks instead).
func TestProposerBreaker_EmptyProposerNeverBlocks(t *testing.T) {
	dag := newGhostdagTestDAG()
	for i := 0; i < proposerBreakerFailThreshold*2; i++ {
		dag.recordProposerOutcome("", false)
	}
	if dag.proposerBlockBlocked("") {
		t.Fatalf("empty proposer must never be blocked")
	}
}

// TestProposerBreaker_AddPeerBlockDropsTrippedProposer verifies the end-to-end
// wiring: once a proposer is tripped, AddPeerBlock drops its block at the
// lock-free top (returns false) rather than processing it.
func TestProposerBreaker_AddPeerBlockDropsTrippedProposer(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xflooder"
	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("precondition: proposer should be tripped")
	}
	// A block from the tripped proposer is dropped. (It would be rejected later
	// anyway for lacking a valid signature, but the breaker returns first — the
	// value here is that it does not panic and the wiring is exercised.)
	blk := &Block{Hash: "x", Height: 2, ParentHashes: []string{"genesis"}, Proposer: p}
	if dag.AddPeerBlock(blk) {
		t.Fatalf("AddPeerBlock must drop a block from a tripped proposer")
	}
}

// TestProposerBreaker_SelfFetchedBypassesLockFreeDrop is the regression
// guard for the 2026-07-04 fix (SelfFetched, see its own field comment):
// confirmed live, two fully-authorized secondary validators permanently
// deadlocked each other's circuit breakers, because neither treats the
// other as a statically-configured trusted seed (FromSync), so once
// tripped, a breaker could never see an attaching block from that proposer
// again to close it. SelfFetched exempts a block THIS node deliberately
// fetched via its own catch-up sync from the SAME lock-free drop, letting
// it reach the normal orphan/attach logic (proven here by checking it gets
// as far as being queued as an orphan, which only happens AFTER the
// lock-free breaker check) regardless of trusted-seed status.
func TestProposerBreaker_SelfFetchedBypassesLockFreeDrop(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{} // non-nil, db == nil: finality/GHOSTDAG DB fallbacks stay safe no-ops
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	p := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	// Authorization is a wholly separate, still-fully-enforced gate SelfFetched
	// deliberately does not touch -- set explicitly so this test exercises the
	// circuit-breaker exemption specifically, not the authorization path.
	dag.authorizedValidators = map[string]bool{p: true}
	sign := func(height int64) *Block {
		b := &Block{Height: height, Timestamp: time.Now().Unix(), ParentHashes: []string{"missing-parent"}, Proposer: p, Humans: 4, StateRoot: "some-state-root"}
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

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("precondition: proposer should be tripped")
	}

	// Without SelfFetched, dropped before ever reaching the orphan queue.
	dag.AddPeerBlock(sign(2))
	if _, tracked := dag.orphanAge("missing-parent"); tracked {
		t.Fatal("a block from a tripped proposer without SelfFetched must never reach the orphan queue")
	}

	// With SelfFetched, it passes the lock-free breaker gate and reaches the
	// orphan queue (still correctly orphaned — parent genuinely missing —
	// but that's a SEPARATE code path than the breaker's early drop).
	selfFetched := sign(3)
	selfFetched.SelfFetched = true
	dag.AddPeerBlock(selfFetched)
	if _, tracked := dag.orphanAge("missing-parent"); !tracked {
		t.Fatal("a SelfFetched block from a tripped proposer must reach the orphan queue, not be dropped by the lock-free breaker check")
	}
}

// TestProposerBreaker_TrackedProposerCountIsCapped verifies P2-c (audit
// 2026-07-06): proposerFailRun is keyed by an unauthenticated proposer
// string reachable before signature verification, so an attacker generating
// unlimited distinct proposer identities must not grow this map without
// bound. Once at maxTrackedProposers, a brand-new proposer is not admitted,
// while an ALREADY-tracked proposer keeps updating normally (the cap must
// not hand an attacker a way to reset an existing entry by flooding new
// ones — only wiping other maps like warnedUnknownProposers is safe for
// that, this one holds live circuit-breaker state).
func TestProposerBreaker_TrackedProposerCountIsCapped(t *testing.T) {
	dag := newGhostdagTestDAG()

	for i := 0; i < maxTrackedProposers; i++ {
		dag.recordProposerOutcome(fmt.Sprintf("0xattacker%d", i), false)
	}
	dag.proposerBreaker.mu.Lock()
	trackedBefore := len(dag.proposerBreaker.failRun)
	dag.proposerBreaker.mu.Unlock()
	if trackedBefore != maxTrackedProposers {
		t.Fatalf("expected exactly %d tracked proposers, got %d", maxTrackedProposers, trackedBefore)
	}

	// One more, brand-new proposer must NOT be admitted once at the cap.
	dag.recordProposerOutcome("0xoneTooMany", false)
	dag.proposerBreaker.mu.Lock()
	_, admitted := dag.proposerBreaker.failRun["0xonetoomany"]
	trackedAfter := len(dag.proposerBreaker.failRun)
	dag.proposerBreaker.mu.Unlock()
	if admitted || trackedAfter != maxTrackedProposers {
		t.Fatalf("a new proposer must not be admitted once at the cap (tracked=%d, admitted=%v)", trackedAfter, admitted)
	}

	// An ALREADY-tracked proposer must still update normally past the cap.
	const existing = "0xattacker0"
	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(existing, false)
	}
	if !dag.proposerBlockBlocked(existing) {
		t.Fatal("an already-tracked proposer must still be able to trip the breaker even while the map is at its cap")
	}
}
