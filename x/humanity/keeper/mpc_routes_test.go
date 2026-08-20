package keeper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// mpcTestKey returns a fresh signing key and its address, the way a validator
// already has one for signing blocks.
func mpcTestKey(t *testing.T) (hexKey, addr string) {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", crypto.FromECDSA(priv)),
		crypto.PubkeyToAddress(priv.PublicKey).Hex()
}

// The configuration must fail CLOSED. Every rejection below is a way a
// deployment could look like it is running multi-party matching while providing
// none of the protection.
func TestMPCConfigFailsClosed(t *testing.T) {
	keyA, addrA := mpcTestKey(t)
	_, addrB := mpcTestKey(t)

	peersAB := fmt.Sprintf("https://a|%s,https://b|%s", addrA, addrB)

	cases := []struct {
		name string
		env  map[string]string
		want string // "off", "error", or "on"
	}{
		{"unset", map[string]string{}, "off"},
		{"disabled", map[string]string{"MPC_ENABLED": "false"}, "off"},
		{
			"single party is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": fmt.Sprintf("https://only-me|%s", addrA),
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": keyA,
			}, "error",
		},
		{
			"peer without a signing address is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "https://a,https://b",
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": keyA,
			}, "error",
		},
		{
			"two parties sharing one address is refused",
			map[string]string{
				"MPC_ENABLED":     "true",
				"MPC_PEERS":       fmt.Sprintf("https://a|%s,https://b|%s", addrA, addrA),
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": keyA,
			}, "error",
		},
		{
			"missing signing key is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": peersAB,
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": "",
			}, "error",
		},
		{
			"key that does not match this party's listed address is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": peersAB,
				"MPC_PARTY_INDEX": "1", "RELAYER_PRIVATE_KEY": keyA,
			}, "error",
		},
		{
			"party index outside range is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": peersAB,
				"MPC_PARTY_INDEX": "5", "RELAYER_PRIVATE_KEY": keyA,
			}, "error",
		},
		{
			// Plaintext peers are accepted since 2026-08-20. The refusal was
			// written when a shared token was the only protection and forgery
			// was genuinely possible; per-round signatures replaced it, and
			// mpc.TestForgeryFailsWithoutTLS shows nothing an attacker on a
			// plaintext path submits is accepted. It was blocking committee
			// formation over a threat that no longer existed.
			//
			// The peers must still carry signing addresses — that is what the
			// contributions are verified against, and it is the part that
			// actually matters.
			"plaintext peers are accepted, because every round is signed",
			map[string]string{
				"MPC_ENABLED":     "true",
				"MPC_PEERS":       fmt.Sprintf("http://a.example|%s,http://b.example|%s", addrA, addrB),
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": keyA,
			}, "on",
		},
		{
			"two https peers with matching key is accepted",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": peersAB,
				"MPC_PARTY_INDEX": "0", "RELAYER_PRIVATE_KEY": keyA,
			}, "on",
		},
	}

	keys := []string{"MPC_ENABLED", "MPC_PEERS", "MPC_PARTY_INDEX", "MPC_ALLOW_INSECURE", "RELAYER_PRIVATE_KEY"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range keys {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			node, err := newMPCNodeFromEnv(nil)
			switch tc.want {
			case "off":
				if err != nil || node != nil {
					t.Fatalf("expected MPC simply off, got node=%v err=%v", node != nil, err)
				}
			case "error":
				if err == nil {
					t.Fatal("a configuration that provides no real protection was accepted")
				}
				if node != nil {
					t.Fatal("a node was returned alongside an error")
				}
			case "on":
				if err != nil {
					t.Fatalf("a valid configuration was rejected: %v", err)
				}
				if node == nil {
					t.Fatal("no node returned for a valid configuration")
				}
			}
		})
	}
}

// TestValidatorSignaturesVerifyAcrossParties: a node must accept its peer's
// signature and reject anyone else's, using nothing but published addresses.
func TestValidatorSignaturesVerifyAcrossParties(t *testing.T) {
	keyA, addrA := mpcTestKey(t)
	keyB, addrB := mpcTestKey(t)
	peers := fmt.Sprintf("https://a|%s,https://b|%s", addrA, addrB)

	mk := func(index int, key string) *mpcNode {
		t.Helper()
		for _, k := range []string{"MPC_ALLOW_INSECURE"} {
			t.Setenv(k, "")
		}
		t.Setenv("MPC_ENABLED", "true")
		t.Setenv("MPC_PEERS", peers)
		t.Setenv("MPC_PARTY_INDEX", fmt.Sprint(index))
		t.Setenv("RELAYER_PRIVATE_KEY", key)
		n, err := newMPCNodeFromEnv(nil)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	nodeA := mk(0, keyA)
	nodeB := mk(1, keyB)

	digest := mpc.RoundDigest("session", 4, 1, []byte("payload"))
	sigB, err := nodeB.auth.Sign(digest)
	if err != nil {
		t.Fatal(err)
	}

	// A accepts B's signature as party 1, having only B's address.
	if err := nodeA.auth.VerifyParty(1, digest, sigB); err != nil {
		t.Errorf("party 0 rejected party 1's genuine signature: %v", err)
	}
	// And refuses to accept it as party 0 — no validator may speak for another.
	if err := nodeA.auth.VerifyParty(0, digest, sigB); err == nil {
		t.Error("party 1's signature was accepted as party 0's — one validator could forge " +
			"another's answer about whether someone is already registered")
	}

	// An outsider's key verifies as nobody.
	outsiderKey, _ := mpcTestKey(t)
	priv, err := crypto.HexToECDSA(outsiderKey)
	if err != nil {
		t.Fatal(err)
	}
	sigOutsider, err := crypto.Sign(digest, priv)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := nodeA.auth.VerifyParty(i, digest, sigOutsider); err == nil {
			t.Errorf("an unlisted key was accepted as party %d", i)
		}
	}
}

// A broken configuration must not mount the endpoint. Half-configured is the
// dangerous state: peers would reach a live endpoint backed by nothing.
func TestBrokenConfigMountsNoEndpoint(t *testing.T) {
	keyA, addrA := mpcTestKey(t)
	t.Setenv("MPC_ENABLED", "true")
	t.Setenv("MPC_PEERS", fmt.Sprintf("https://only-me|%s", addrA))
	t.Setenv("MPC_PARTY_INDEX", "0")
	t.Setenv("RELAYER_PRIVATE_KEY", keyA)

	mux := http.NewServeMux()
	if node := registerMPCRoutes(mux, nil); node != nil {
		t.Fatal("a node was returned for an invalid configuration")
	}
	req := httptest.NewRequest(http.MethodPost, mpc.ExchangePath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the exchange endpoint answered with %d despite an invalid configuration", rec.Code)
	}
}

// TestAddingAValidatorNeedsNoSecret is the answer to "how does onboarding
// work": a new party is added by publishing its address. Nothing is rotated on
// the existing boxes, and no secret is handed to anybody.
func TestAddingAValidatorNeedsNoSecret(t *testing.T) {
	keyA, addrA := mpcTestKey(t)
	_, addrB := mpcTestKey(t)
	_, addrC := mpcTestKey(t)

	t.Setenv("MPC_ENABLED", "true")
	t.Setenv("MPC_PARTY_INDEX", "0")
	t.Setenv("RELAYER_PRIVATE_KEY", keyA)

	t.Setenv("MPC_PEERS", fmt.Sprintf("https://a|%s,https://b|%s", addrA, addrB))
	two, err := newMPCNodeFromEnv(nil)
	if err != nil {
		t.Fatal(err)
	}

	// The only change to add a third: one more entry. Party 0's key is
	// untouched, and it can already verify the newcomer.
	t.Setenv("MPC_PEERS", fmt.Sprintf("https://a|%s,https://b|%s,https://c|%s", addrA, addrB, addrC))
	three, err := newMPCNodeFromEnv(nil)
	if err != nil {
		t.Fatalf("adding a third validator by publishing its address failed: %v", err)
	}
	if two.auth.Parties() != 2 || three.auth.Parties() != 3 {
		t.Fatalf("party counts are %d and %d, want 2 and 3", two.auth.Parties(), three.auth.Parties())
	}
	if two.auth.addrs[0] != three.auth.addrs[0] {
		t.Error("party 0's identity changed when a validator was added; onboarding must not " +
			"disturb the existing parties")
	}
}

// TestCommitteeIsDiscoveredNotConfigured is the answer to "must every new
// validator be added to MPC_PEERS by hand": no. With MPC_PEERS empty the node
// derives the committee from the peers it already knows, so a validator becomes
// eligible by registering — no edit, no restart, no approval.
func TestCommitteeIsDiscoveredNotConfigured(t *testing.T) {
	keyA, addrA := mpcTestKey(t)
	_, addrB := mpcTestKey(t)
	_, addrC := mpcTestKey(t)

	known := []mpc.Party{
		{URL: "https://a.example", Address: strings.ToLower(addrA)},
		{URL: "https://b.example", Address: strings.ToLower(addrB)},
	}
	discover := func() []mpc.Party { return known }

	t.Setenv("MPC_ENABLED", "true")
	t.Setenv("MPC_PEERS", "")
	t.Setenv("MPC_COMMITTEE_SIZE", "2")
	t.Setenv("RELAYER_PRIVATE_KEY", keyA)

	node, err := newMPCNodeFromEnv(discover)
	if err != nil {
		t.Fatal(err)
	}
	if node == nil {
		t.Fatal("no node built from discovery")
	}
	first, err := node.committeeNow()
	if err != nil {
		t.Fatal(err)
	}

	// A third validator registers. Nothing on this node changes.
	known = append(known, mpc.Party{URL: "https://c.example", Address: strings.ToLower(addrC)})
	second, err := node.committeeNow()
	if err != nil {
		t.Fatalf("a newly registered validator broke committee resolution: %v", err)
	}
	if second.Size() != 2 {
		t.Errorf("committee size drifted to %d when a validator joined", second.Size())
	}
	t.Logf("committee before %s, after %s", first.ID, second.ID)

	// And the newcomer is genuinely eligible: with a larger committee it is drawn.
	t.Setenv("MPC_COMMITTEE_SIZE", "3")
	bigger, err := newMPCNodeFromEnv(discover)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := bigger.committeeNow()
	if err != nil {
		t.Fatal(err)
	}
	if c3.IndexOf(strings.ToLower(addrC)) < 0 {
		t.Error("a registered validator was not eligible for the committee; discovery is not " +
			"actually reading the peer set")
	}
}

// TestOversizedCommitteeIsRefused encodes the measurement rather than a taste:
// traffic grows with n(n-1), and every member must be online for any
// registration to complete.
func TestOversizedCommitteeIsRefused(t *testing.T) {
	keyA, _ := mpcTestKey(t)
	t.Setenv("MPC_ENABLED", "true")
	t.Setenv("MPC_PEERS", "")
	t.Setenv("RELAYER_PRIVATE_KEY", keyA)
	discover := func() []mpc.Party { return nil }

	for _, size := range []string{"1", "50"} {
		t.Setenv("MPC_COMMITTEE_SIZE", size)
		if _, err := newMPCNodeFromEnv(discover); err == nil {
			t.Errorf("committee size %s was accepted", size)
		}
	}
}
