package keeper

import (
	"os"
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

func TestSeedURLs_EmptyWhenUnset(t *testing.T) {
	os.Unsetenv("PRIMARY_NODE_URL")
	os.Unsetenv("PRIMARY_NODE_URLS")
	got := seedURLs("https://self.example.com")
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestBootstrapNodes_FallsBackToDefault(t *testing.T) {
	os.Unsetenv("BOOTSTRAP_P2P_ADDR")
	got := BootstrapNodes()
	if len(got) != 1 || got[0] != defaultBootstrapNode {
		t.Fatalf("got %v, want [%s]", got, defaultBootstrapNode)
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
