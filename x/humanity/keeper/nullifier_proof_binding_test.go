package keeper

import (
	"os"
	"strings"
	"testing"
)

// The bug this file exists for was not a wrong implementation. It was a
// CORRECT implementation that nothing called.
//
// nullifierMatchesProof shipped complete, with three passing unit tests and a
// doc comment naming the exact attack it prevents — and zero call sites in
// production code. The replay path went on doing precisely what that comment
// described as the bug: verify the Groth16 proof, then separately claim
// tx.Nullifier, never comparing the two. Any authorized validator could pair a
// published proof (they travel in the clear inside every register_human block)
// with a nullifier of its own choosing and a fresh wallet, and mint 1,000 AEQ
// per repetition.
//
// A unit test of the function could never have caught that, because the
// function was never wrong. Only a test of the WIRING could. That is what the
// first test below is.

// TestNullifierBinding_IsActuallyWiredIntoReplay reads block.go and asserts
// that the register_human replay path calls nullifierMatchesProof between
// verifying the proof and claiming the nullifier.
//
// Source-scanning, in the same spirit as evm_v7_slots_source_test.go: the
// property under test is "these two things are connected", which is a fact
// about the source, not about any value the function returns.
func TestNullifierBinding_IsActuallyWiredIntoReplay(t *testing.T) {
	src, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("read block.go: %v", err)
	}
	text := string(src)

	verify := strings.Index(text, "if !dag.verifyZKProof(tx)")
	if verify < 0 {
		t.Fatal("could not find the ZK proof check in block.go — this test needs updating")
	}
	claim := strings.Index(text, "dag.state.tryClaimNullifierLocked(")
	if claim < 0 {
		t.Fatal("could not find the nullifier claim in block.go — this test needs updating")
	}
	if claim < verify {
		t.Fatal("the nullifier claim now precedes proof verification — the ordering this test assumes no longer holds")
	}

	between := text[verify:claim]
	if !strings.Contains(between, "nullifierMatchesProof(") {
		t.Error("register_human replay verifies the ZK proof and then claims tx.Nullifier " +
			"WITHOUT binding the two together.\n\n" +
			"  nullifierMatchesProof exists and is tested, but is not called on this path.\n" +
			"  That is the whole vulnerability: a valid proof can be replayed with an\n" +
			"  arbitrary nullifier and a fresh wallet to mint 1,000 AEQ, repeatedly.\n\n" +
			"  Call it between verifyZKProof and tryClaimNullifierLocked.")
	}
}

// The binding must reject the attack shape specifically: a genuinely valid
// proof, paired with a nullifier the attacker picked. The existing tests in
// nullifier_canonical_test.go cover spelling variants of the SAME value; this
// covers a DIFFERENT value, which is what an attacker would actually submit.
func TestNullifierBinding_RejectsAnAttackerChosenValue(t *testing.T) {
	const realNullifier = "12857392037461937299283746192837461928374619283746192837461928374"
	pubSignals := []string{"commitment-value", realNullifier}

	if ok, err := nullifierMatchesProof(realNullifier, pubSignals); err != nil || !ok {
		t.Fatalf("the honest case must pass: ok=%v err=%v", ok, err)
	}

	// Exactly what the mint looks like: the attacker keeps the proof and
	// substitutes a nullifier nobody has claimed yet. Small, arbitrary values
	// are the cheapest to enumerate, so they are the ones to pin.
	for _, forged := range []string{"1", "2", "3", "99999999999", realNullifier + "0"} {
		ok, err := nullifierMatchesProof(forged, pubSignals)
		if err == nil && ok {
			t.Errorf("nullifier %q was accepted against a proof committing to %q — "+
				"this is the unlimited-mint case", forged, realNullifier)
		}
	}
}

// A proof carrying no nullifier signal must not silently pass the binding.
func TestNullifierBinding_RejectsAProofWithNoNullifierSignal(t *testing.T) {
	for _, pubSignals := range [][]string{nil, {}, {"only-a-commitment"}} {
		if ok, err := nullifierMatchesProof("12857392037461937299", pubSignals); err == nil && ok {
			t.Errorf("pubSignals %v carries no nullifier, yet the binding passed", pubSignals)
		}
	}
	// pubSignals[1] present but unusable (zero reads as an unset mapping slot
	// in the contract, so it must never be treated as a real nullifier).
	if ok, err := nullifierMatchesProof("0", []string{"c", "0"}); err == nil && ok {
		t.Error("a zero nullifier was accepted — zero is indistinguishable from 'never used'")
	}
}

// The activation constant gates rejection, never the check itself. If it is
// ever moved, it must stay a fixed block-timestamp anchor: reading wall-clock
// time here would make two nodes disagree about the same historical block.
func TestNullifierBinding_ActivationIsAFixedAnchor(t *testing.T) {
	if nullifierProofBindingActivationUnix <= 0 {
		t.Fatal("activation timestamp must be a real Unix time")
	}
	src, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("read block.go: %v", err)
	}
	text := string(src)
	idx := strings.Index(text, "nullifierProofBindingActivationUnix")
	if idx < 0 {
		t.Fatal("the activation constant is no longer referenced from block.go")
	}
	// It must be compared against the BLOCK's timestamp, not the node's clock.
	window := text[max(0, idx-120):idx]
	if !strings.Contains(window, "block.Timestamp") {
		t.Error("the binding's activation is not anchored on block.Timestamp — " +
			"anchoring it on wall-clock time would make two nodes reach different " +
			"verdicts for the same block, which is a fork, not a check")
	}
}
