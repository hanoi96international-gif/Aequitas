package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// These guard the 2026-07-26 one-way-fork fix.
//
// The primary restarted at 16:45, saw its first block from validator
// 0x0BE8b961... before peer registration had completed, fired its ONE
// validator-list recovery into an empty activeSyncPeers set, and never tried
// again. From then on it rejected every block from that validator silently —
// the log line was suppressed by the same flag that gated the recovery — and
// abandoned the orphans waiting on them. Measured consequence: 1386
// unresolvable missing parents on the primary against zero on both
// secondaries, its own blocks still reaching them while none of theirs
// reached it, and a blue-score gap growing monotonically (1457 -> 1580 ->
// 1806).
//
// The secondaries could not hit this: they hold the primary in trustedSeeds,
// so blocks they fetch from it carry FromSync=true and skip the authorization
// gate entirely. The primary's trustedSeeds is empty by construction, so it is
// the one node that can be locked out of a validator it does not already know.

// signTestBlockFromKey signs a block with a CALLER-SUPPLIED key, so a test can
// send several blocks from the SAME proposer. signTestBlockWithParent
// generates a fresh key per call, which is right for its own tests and wrong
// here: the whole subject is what happens on the second and third block from
// one unknown validator.
func signTestBlockFromKey(t *testing.T, key *ecdsa.PrivateKey, height int64, parentHash string) *Block {
	t.Helper()
	b := &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		ParentHashes: []string{parentHash},
		Proposer:     strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
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

// newUnknownProposerTestDAG builds the minimum state AddPeerBlock's
// authorization gate touches, with the proposer deliberately NOT authorized.
func newUnknownProposerTestDAG() *BlockDAG {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 0
	dag.authorizedValidators = map[string]bool{} // proposer is NOT registered
	dag.warnedUnknownProposers = map[string]bool{}
	dag.activeSyncPeers = map[string]bool{}
	dag.trustedSeeds = map[string]bool{}
	dag.blocks["deadbeef"] = &Block{Hash: "deadbeef", Height: 0, IsGenesis: true}
	return dag
}

// TestUnknownProposer_RecoveryRetriesAfterInterval is the core regression
// guard. Before the fix, recovery was gated on warnedUnknownProposers — a
// log-suppression flag — so the second block from the same unknown proposer
// scheduled no recovery at all, forever. Now the retry clock decides, so a
// first attempt that accomplished nothing is followed by another one.
func TestUnknownProposer_RecoveryRetriesAfterInterval(t *testing.T) {
	dag := newUnknownProposerTestDAG()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	blk1 := signTestBlockFromKey(t, key, 1, "deadbeef")
	blk1.FromSync = false
	if dag.AddPeerBlock(blk1) {
		t.Fatal("an unauthorized proposer's block must still be rejected — the fix must not weaken the authorization gate")
	}
	proposer := blk1.Proposer
	firstTry, tried := dag.unknownProposerLastRecovery[proposer]
	if !tried {
		t.Fatal("first sight of an unknown proposer must record a recovery attempt")
	}

	// A second block arriving immediately must NOT re-trigger: that would mean
	// one validator-list sync per block, which is the cost the original
	// one-shot design was avoiding.
	blk2 := signTestBlockFromKey(t, key, 2, "deadbeef")
	blk2.FromSync = false
	dag.AddPeerBlock(blk2)
	if got := dag.unknownProposerLastRecovery[proposer]; !got.Equal(firstTry) {
		t.Fatalf("a block arriving inside the retry interval must not re-trigger recovery: timestamp moved from %v to %v", firstTry, got)
	}

	// Age the clock past the retry interval — the situation the live incident
	// was stuck in: still unknown, blocks still arriving, nothing being done.
	aged := time.Now().Add(-unknownProposerRecoveryRetry - time.Second)
	dag.mu.Lock()
	dag.unknownProposerLastRecovery[proposer] = aged
	dag.mu.Unlock()

	blk3 := signTestBlockFromKey(t, key, 3, "deadbeef")
	blk3.FromSync = false
	dag.AddPeerBlock(blk3)

	retried := dag.unknownProposerLastRecovery[proposer]
	// Compare against the AGED value, not against firstTry.
	//
	// Comparing to firstTry made this test fail on Windows roughly two runs in
	// three: the system clock there has ~15.6ms granularity, so the retry's
	// time.Now() frequently returns a value identical to firstTry's — down to
	// the monotonic reading — even though the retry ran exactly as intended.
	// Linux CI has nanosecond resolution and never saw it, which is why a test
	// that fails most of the time locally was passing in CI.
	//
	// The aged timestamp is a full retry interval in the past, so advancing
	// past it is unambiguous at any clock resolution, and it is also the
	// stronger assertion: it proves the retry rewrote the value rather than
	// merely that two clock readings differ.
	if !retried.After(aged) {
		t.Fatalf("recovery must be retried once the interval has elapsed — this is the whole fix. timestamp %v did not advance past the aged %v", retried, aged)
	}
	if time.Since(retried) > 5*time.Second {
		t.Fatalf("retry timestamp should have been refreshed to now, got %v", retried)
	}
}

// TestUnknownProposer_LogSuppressionDoesNotGateRecovery states the defect in
// its own terms: warnedUnknownProposers may mark a proposer as already-logged
// without that having any bearing on whether recovery may run again. Before
// the fix these were the same flag, which is exactly how one lost attempt
// became permanent.
func TestUnknownProposer_LogSuppressionDoesNotGateRecovery(t *testing.T) {
	dag := newUnknownProposerTestDAG()

	blk := signTestBlockWithParent(t, 1, "deadbeef")
	blk.FromSync = false
	proposer := blk.Proposer

	// Pre-mark as "already logged" while leaving the recovery clock untouched,
	// i.e. the exact state the node is in for every block after the first.
	dag.mu.Lock()
	dag.warnedUnknownProposers[proposer] = true
	dag.mu.Unlock()

	dag.AddPeerBlock(blk)

	if _, tried := dag.unknownProposerLastRecovery[proposer]; !tried {
		t.Fatal("recovery must run even when the proposer has already been logged about — tying the two together is the bug this fixes")
	}
}

// TestUnknownProposer_RecoveryMapSurvivesNilConstruction guards the nil-map
// hazard: BlockDAG is built as a struct literal in several places (every test
// helper here included), so the gate must create the map rather than write
// into a nil one and panic.
func TestUnknownProposer_RecoveryMapSurvivesNilConstruction(t *testing.T) {
	dag := newUnknownProposerTestDAG()
	dag.unknownProposerLastRecovery = nil

	blk := signTestBlockWithParent(t, 1, "deadbeef")
	blk.FromSync = false
	dag.AddPeerBlock(blk) // must not panic

	if len(dag.unknownProposerLastRecovery) != 1 {
		t.Fatalf("gate must lazily create the recovery map, got %d entries", len(dag.unknownProposerLastRecovery))
	}
}
