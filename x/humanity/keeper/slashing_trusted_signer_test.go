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
