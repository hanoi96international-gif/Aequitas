package keeper

import (
	"os"
	"strings"
	"testing"
)

func TestSeedURLs_CombinesPrimaryAndPrimaryURLs(t *testing.T) {
	t.Setenv("PRIMARY_NODE_URL", "https://a.example.com")
	t.Setenv("PRIMARY_NODE_URLS", "https://b.example.com, https://c.example.com")
	got := seedURLs("https://self.example.com")
	want := []string{"https://a.example.com", "https://b.example.com", "https://c.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSeedURLs_DedupesAndExcludesSelf(t *testing.T) {
	t.Setenv("PRIMARY_NODE_URL", "https://a.example.com")
	t.Setenv("PRIMARY_NODE_URLS", "https://a.example.com,https://self.example.com,https://b.example.com")
	got := seedURLs("https://self.example.com")
	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSeedURLs_NormalizesSchemelessURLs(t *testing.T) {
	t.Setenv("PRIMARY_NODE_URL", "a.example.com")
	t.Setenv("PRIMARY_NODE_URLS", "")
	got := seedURLs("https://self.example.com")
	if len(got) != 1 || got[0] != "https://a.example.com" {
		t.Fatalf("got %v, want [https://a.example.com]", got)
	}
}

func TestSeedURLs_DefaultsToPublicSeedWhenUnset(t *testing.T) {
	os.Unsetenv("PRIMARY_NODE_URL")
	os.Unsetenv("PRIMARY_NODE_URLS")
	got := seedURLs("https://self.example.com")
	if len(got) != len(defaultPublicSeeds) {
		t.Fatalf("got %v, want %v (a fresh operator with no config must default to joining the public network)", got, defaultPublicSeeds)
	}
	// A VERIFIED VALIDATOR ADDRESS MUST COME FIRST, never the domain.
	//
	// Measured 2026-08-14: aequitas.digital resolved to a host serving a node
	// at height 96 with 0 humans and no peers, while the real chain was past
	// 3.74 million. A hostname only says who answers, never which chain they
	// answer for — so a zero-config newcomer taking the domain as its first
	// seed would have bootstrapped from an empty chain. The validator IPs are
	// addresses this project controls and can verify; the domain is kept in
	// the list but consulted last.
	if got[0] == defaultPublicSeed {
		t.Fatalf("got[0] = %s — the canonical domain must NOT be the first seed; a misconfigured or hijacked DNS record would then seed newcomers from the wrong chain", got[0])
	}
	if last := got[len(got)-1]; last != defaultPublicSeed {
		t.Fatalf("last seed = %s, want the canonical domain %s (it belongs in the list, just not first)", last, defaultPublicSeed)
	}
}

func TestSeedURLs_DefaultExcludedWhenSelfIsThePublicSeed(t *testing.T) {
	os.Unsetenv("PRIMARY_NODE_URL")
	os.Unsetenv("PRIMARY_NODE_URLS")
	got := seedURLs(defaultPublicSeed)
	for _, u := range got {
		if u == defaultPublicSeed {
			t.Fatalf("got %v — the public seed node itself must not appear in its own seed list", got)
		}
	}
}

// TestDefaultPublicSeeds_NoDecommissionedHosts is the HTTP-transport twin of
// TestDefaultBootstrapNodes_NoDecommissionedHosts. Both built-in entry points
// went stale simultaneously when Railway was decommissioned (2026-08-14),
// which is what left a zero-config node unable to reach the network at all —
// the P2P default and the HTTP default must never again depend on the same
// piece of infrastructure, nor on a decommissioned one.
func TestDefaultPublicSeeds_NoDecommissionedHosts(t *testing.T) {
	if len(defaultPublicSeeds) < 2 {
		t.Fatalf("defaultPublicSeeds has %d entr(ies) — keep a fallback so a single unavailable seed (expired domain, reissued TLS cert, box down) cannot strand newcomers", len(defaultPublicSeeds))
	}
	for _, seed := range defaultPublicSeeds {
		for _, banned := range []string{"rlwy.net", "railway.app", "railway.com"} {
			if strings.Contains(seed, banned) {
				t.Errorf("seed %q references decommissioned host %q — Railway no longer hosts this network", seed, banned)
			}
		}
		// Every built-in seed must survive the peer-URL filter, or it can
		// never be dialed and is dead weight that only costs a timeout.
		if !isAllowedPeerURL(seed) {
			t.Errorf("seed %q is rejected by isAllowedPeerURL — it could never be used", seed)
		}
	}
}

func TestBootstrapNodes_FallsBackToDefault(t *testing.T) {
	os.Unsetenv("BOOTSTRAP_P2P_ADDR")
	got := BootstrapNodes()
	if len(got) != len(defaultBootstrapNodes) {
		t.Fatalf("got %v, want %v", got, defaultBootstrapNodes)
	}
	for i := range defaultBootstrapNodes {
		if got[i] != defaultBootstrapNodes[i] {
			t.Fatalf("index %d: got %q, want %q", i, got[i], defaultBootstrapNodes[i])
		}
	}
}

// TestBootstrapAddrIsSelf covers the self-dial guard that replaced the
// hardcoded Railway-primary peer ID (2026-08-14). Both Contabo validators are
// now IN the default bootstrap set, so without this each of them would dial
// its own address on every startup.
func TestBootstrapAddrIsSelf(t *testing.T) {
	const selfID = "12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm"
	const otherID = "12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN"

	cases := []struct {
		name string
		addr string
		self string
		want bool
	}{
		{"own address", "/ip4/173.249.37.118/tcp/4001/p2p/" + selfID, selfID, true},
		{"own address, trailing slash", "/ip4/173.249.37.118/tcp/4001/p2p/" + selfID + "/", selfID, true},
		{"different peer, same host", "/ip4/173.249.37.118/tcp/4001/p2p/" + otherID, selfID, false},
		{"different peer, different host", "/ip4/194.163.188.71/tcp/4001/p2p/" + otherID, selfID, false},
		{"no /p2p/ component", "/ip4/173.249.37.118/tcp/4001", selfID, false},
		{"empty self id", "/ip4/173.249.37.118/tcp/4001/p2p/" + selfID, "", false},
		// A peer ID that merely CONTAINS ours as a prefix must not match —
		// otherwise a legitimate peer could be skipped.
		{"peer id with our id as prefix", "/ip4/1.2.3.4/tcp/4001/p2p/" + selfID + "XYZ", selfID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bootstrapAddrIsSelf(tc.addr, tc.self); got != tc.want {
				t.Fatalf("bootstrapAddrIsSelf(%q, %q) = %v, want %v", tc.addr, tc.self, got, tc.want)
			}
		})
	}
}

// TestDefaultBootstrapNodes_NoDecommissionedHosts guards the migration off
// Railway (2026-08-14). The built-in bootstrap address silently going stale
// has already broken P2P network-wide twice, both times because it pointed at
// a managed-platform proxy hostname that the platform regenerated
// (thomas.proxy.rlwy.net, then reseau.proxy.rlwy.net). Railway now hosts
// nothing, so any reappearance of such a host here is a straight regression:
// a zero-config node would dial an address that cannot resolve, and — since
// the HTTP seed went stale at the same time — would reach the network by no
// transport at all.
func TestDefaultBootstrapNodes_NoDecommissionedHosts(t *testing.T) {
	if len(defaultBootstrapNodes) < 2 {
		t.Fatalf("defaultBootstrapNodes has %d entr(ies) — keep at least two so one validator being down does not strand newcomers", len(defaultBootstrapNodes))
	}
	for _, addr := range defaultBootstrapNodes {
		for _, banned := range []string{"rlwy.net", "railway.app", "railway.com"} {
			if strings.Contains(addr, banned) {
				t.Errorf("bootstrap address %q references decommissioned host %q — Railway no longer hosts this network", addr, banned)
			}
		}
		if !strings.HasPrefix(addr, "/ip4/") && !strings.HasPrefix(addr, "/dns4/") {
			t.Errorf("bootstrap address %q is not a usable multiaddr", addr)
		}
		if !strings.Contains(addr, "/p2p/") {
			t.Errorf("bootstrap address %q carries no /p2p/ peer ID — libp2p cannot dial it", addr)
		}
	}
}

func TestBootstrapNodes_ParsesCommaSeparatedList(t *testing.T) {
	t.Setenv("BOOTSTRAP_P2P_ADDR", "/dns4/a.example.com/tcp/4001/p2p/QmA, /dns4/b.example.com/tcp/4001/p2p/QmB")
	got := BootstrapNodes()
	want := []string{"/dns4/a.example.com/tcp/4001/p2p/QmA", "/dns4/b.example.com/tcp/4001/p2p/QmB"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
