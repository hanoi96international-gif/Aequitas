package keeper

import (
	"testing"
)

// TestTrustedBootstrapSigner_NormalizesAndHandlesUnset pins the contract the
// 2026-07-24 detection-site fix depends on (see AddPeerBlock's equivocation
// goroutine): the comparison against a block's proposer must be
// case/whitespace-insensitive, and an unset BOOTSTRAP_SIGNER must return ""
// so the fix's own `trusted != ""` guard leaves third-party validators on the
// unchanged, immediate-suspension path.
func TestTrustedBootstrapSigner_NormalizesAndHandlesUnset(t *testing.T) {
	t.Setenv("BOOTSTRAP_SIGNER", "")
	if got := trustedBootstrapSigner(); got != "" {
		t.Fatalf("trustedBootstrapSigner() = %q, want \"\" when BOOTSTRAP_SIGNER is unset", got)
	}

	t.Setenv("BOOTSTRAP_SIGNER", "  0xAbCdEf0123456789012345678901234567890123  ")
	want := "0xabcdef0123456789012345678901234567890123"
	if got := trustedBootstrapSigner(); got != want {
		t.Fatalf("trustedBootstrapSigner() = %q, want %q (trimmed and lowercased)", got, want)
	}
}

// TestRecordEquivocationEvidenceOnly_NilDBReturnsError mirrors every other
// slashing helper's nil-DB contract. The detection site logs and continues on
// error rather than aborting, so the only thing that must hold here is "does
// not panic, reports the condition".
func TestRecordEquivocationEvidenceOnly_NilDBReturnsError(t *testing.T) {
	cs := newTestState() // useDB: false, cs.db is nil
	err := cs.RecordEquivocationEvidenceOnly("0xabc", "hashA", "hashB", equivocationSlashingActivationUnix+1)
	if err == nil {
		t.Fatal("RecordEquivocationEvidenceOnly with no DB configured must report an error, not silently claim success")
	}
}

// TestRecordEquivocationEvidenceOnly_PreActivationIsExempt verifies the
// evidence-only path carries the SAME pre-activation cutoff as
// RecordEquivocationAndSuspend. Without it, this new write path would become a
// way to reintroduce exactly the retroactive-penalty problem
// equivocationSlashingActivationUnix exists to prevent — evidence rows for
// pre-cutoff history on every freshly-synced node. Checked before the nil-DB
// guard would otherwise fire, which is what makes the nil-DB state a valid
// probe for "returned early".
func TestRecordEquivocationEvidenceOnly_PreActivationIsExempt(t *testing.T) {
	cs := newTestState()
	if err := cs.RecordEquivocationEvidenceOnly("0xabc", "hashA", "hashB", equivocationSlashingActivationUnix-1); err != nil {
		t.Fatalf("pre-activation evidence must be exempt (nil error, no write attempted), got %v", err)
	}
}
