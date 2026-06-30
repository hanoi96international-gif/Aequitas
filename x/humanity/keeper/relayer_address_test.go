package keeper

import (
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestRelayerAddressFromEnv_ExplicitOverrideWins(t *testing.T) {
	t.Setenv("RELAYER_ADDRESS", "0xAAAA000000000000000000000000000000AAAA")
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	got := relayerAddressFromEnv()
	want := "0xaaaa000000000000000000000000000000aaaa"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRelayerAddressFromEnv_DerivesFromPrivateKey(t *testing.T) {
	os.Unsetenv("RELAYER_ADDRESS")
	// A well-known test private key (Hardhat/Anvil default account #0) with
	// a well-known corresponding address, used only to check the derivation
	// logic — not a real key with any funds anywhere.
	t.Setenv("RELAYER_PRIVATE_KEY", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	got := relayerAddressFromEnv()
	want := "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"
	if got != want {
		t.Errorf("got %q, want %q (derived address must match the well-known address for this test key)", got, want)
	}
}

func TestRelayerAddressFromEnv_EmptyWhenNeitherSet(t *testing.T) {
	os.Unsetenv("RELAYER_ADDRESS")
	os.Unsetenv("RELAYER_PRIVATE_KEY")
	if got := relayerAddressFromEnv(); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestRelayerAddressFromEnv_InvalidPrivateKeyYieldsEmpty(t *testing.T) {
	os.Unsetenv("RELAYER_ADDRESS")
	t.Setenv("RELAYER_PRIVATE_KEY", "not-a-valid-hex-key")
	if got := relayerAddressFromEnv(); got != "" {
		t.Errorf("got %q, want empty string for an invalid key", got)
	}
}

func TestLoadOrCreateRelayerKey_UsesExplicitKeyWhenSet(t *testing.T) {
	t.Setenv("RELAYER_PRIVATE_KEY", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	key, generated, err := loadOrCreateRelayerKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if generated {
		t.Error("want generated=false when RELAYER_PRIVATE_KEY is explicitly set")
	}
	gotAddr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	wantAddr := "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"
	if gotAddr != wantAddr {
		t.Errorf("got address %q, want %q", gotAddr, wantAddr)
	}
}

func TestLoadOrCreateRelayerKey_GeneratesWhenUnset(t *testing.T) {
	os.Unsetenv("RELAYER_PRIVATE_KEY")
	key, generated, err := loadOrCreateRelayerKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !generated {
		t.Error("want generated=true when RELAYER_PRIVATE_KEY is unset")
	}
	if key == nil {
		t.Fatal("want a non-nil generated key")
	}
	// Two independent calls must generate DIFFERENT keys (not some fixed
	// placeholder) -- the whole point is a fresh, unique validator identity.
	key2, _, err2 := loadOrCreateRelayerKey()
	if err2 != nil {
		t.Fatalf("unexpected error on second call: %v", err2)
	}
	if crypto.PubkeyToAddress(key.PublicKey) == crypto.PubkeyToAddress(key2.PublicKey) {
		t.Error("two independent generated keys must not collide")
	}
}

func TestLoadOrCreateRelayerKey_InvalidExplicitKeyErrors(t *testing.T) {
	t.Setenv("RELAYER_PRIVATE_KEY", "not-a-valid-hex-key")
	_, _, err := loadOrCreateRelayerKey()
	if err == nil {
		t.Error("want an error for an invalid explicit RELAYER_PRIVATE_KEY")
	}
}
