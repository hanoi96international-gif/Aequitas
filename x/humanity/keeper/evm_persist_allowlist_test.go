package keeper

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// Regression tests for the beta-launch audit (2026-07-05) G9 fix:
// checkPersistedCallAllowed used to be logic inlined only inside
// sendRawTransaction's HTTP handler (evm_rpc.go) — CallContract itself
// trusted its caller completely for the persist=true decision. It's now a
// shared function called by both, so a future persist=true call site can't
// forget to replicate the check. These tests exercise the shared function
// directly, independent of the HTTP layer.

func v7Addr() common.Address { return common.HexToAddress(V7_CONTRACT_ADDR) }

func TestCheckPersistedCallAllowed_NoSelectorAllowed(t *testing.T) {
	if err := checkPersistedCallAllowed(v7Addr(), []byte{0x01, 0x02}, "0xanyone"); err != nil {
		t.Fatalf("calldata shorter than 4 bytes should be allowed through (mirrors the original len(data)>=4 gate), got: %v", err)
	}
}

func TestCheckPersistedCallAllowed_NonV7ContractRejected(t *testing.T) {
	other := common.HexToAddress("0x1111111111111111111111111111111111111111")
	data := []byte{0xa9, 0x05, 0x9c, 0xbb} // transfer selector, but wrong contract
	if err := checkPersistedCallAllowed(other, data, "0xanyone"); err == nil {
		t.Fatal("expected an error for a persisting call to a non-V7 contract")
	}
}

func TestCheckPersistedCallAllowed_KnownSelectorsAllowed(t *testing.T) {
	knownSelectors := []string{"a9059cbb", "70a08231", "dd62ed3e", "18160ddd", "06fdde03", "95d89b41", "313ce567"}
	for _, sel := range knownSelectors {
		data := mustHexDecode(t, sel)
		if err := checkPersistedCallAllowed(v7Addr(), data, "0xanyone"); err != nil {
			t.Errorf("selector %s should be allowed, got error: %v", sel, err)
		}
	}
}

func TestCheckPersistedCallAllowed_UnknownSelectorRejected(t *testing.T) {
	data := mustHexDecode(t, "deadbeef")
	if err := checkPersistedCallAllowed(v7Addr(), data, "0xanyone"); err == nil {
		t.Fatal("expected an error for an unrecognized selector")
	}
}

func TestCheckPersistedCallAllowed_RegisterWithSig_RejectsNonRelayer(t *testing.T) {
	t.Setenv("RELAYER_ADDRESS", "0xrelayeraddr")
	data := mustHexDecode(t, "13b81eb0") // registerWithSig
	if err := checkPersistedCallAllowed(v7Addr(), data, "0xsomeoneelse"); err == nil {
		t.Fatal("expected registerWithSig to be rejected when the caller is not the relayer")
	}
}

func TestCheckPersistedCallAllowed_RegisterWithSig_AllowsRelayer(t *testing.T) {
	t.Setenv("RELAYER_ADDRESS", "0xrelayeraddr")
	data := mustHexDecode(t, "13b81eb0")
	if err := checkPersistedCallAllowed(v7Addr(), data, "0xRelayerAddr"); err != nil {
		t.Errorf("expected registerWithSig to be allowed for the relayer (case-insensitive), got: %v", err)
	}
}

func TestCheckPersistedCallAllowed_RegisterWithSig_RejectsWhenNoRelayerConfigured(t *testing.T) {
	// t.Setenv (not os.Unsetenv) so Go restores the real value after this
	// test, keeping this isolated from any other test in the package.
	t.Setenv("RELAYER_ADDRESS", "")
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	data := mustHexDecode(t, "13b81eb0")
	if err := checkPersistedCallAllowed(v7Addr(), data, "0xanyone"); err == nil {
		t.Fatal("expected registerWithSig to be rejected when no relayer address is configured at all")
	}
}

func mustHexDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}
