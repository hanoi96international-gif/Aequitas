package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// The configuration must fail CLOSED. Every rejection below is a way a
// deployment could look like it is running two-party matching while providing
// none of the protection.
func TestMPCConfigFailsClosed(t *testing.T) {
	const goodToken = "0123456789abcdef0123456789abcdef"

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
				"MPC_ENABLED": "true", "MPC_PEERS": "https://only-me",
				"MPC_PARTY_INDEX": "0", "MPC_PEER_TOKEN": goodToken,
			}, "error",
		},
		{
			"empty token is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "https://a,https://b",
				"MPC_PARTY_INDEX": "0", "MPC_PEER_TOKEN": "",
			}, "error",
		},
		{
			"short token is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "https://a,https://b",
				"MPC_PARTY_INDEX": "0", "MPC_PEER_TOKEN": "secret",
			}, "error",
		},
		{
			"party index outside range is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "https://a,https://b",
				"MPC_PARTY_INDEX": "5", "MPC_PEER_TOKEN": goodToken,
			}, "error",
		},
		{
			"insecure with a remote peer is refused",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "http://a.example,http://b.example",
				"MPC_PARTY_INDEX": "0", "MPC_PEER_TOKEN": goodToken,
				"MPC_ALLOW_INSECURE": "true",
			}, "error",
		},
		{
			"two https peers is accepted",
			map[string]string{
				"MPC_ENABLED": "true", "MPC_PEERS": "https://a,https://b",
				"MPC_PARTY_INDEX": "1", "MPC_PEER_TOKEN": goodToken,
			}, "on",
		},
	}

	keys := []string{"MPC_ENABLED", "MPC_PEERS", "MPC_PARTY_INDEX", "MPC_PEER_TOKEN", "MPC_ALLOW_INSECURE"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range keys {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			node, err := newMPCNodeFromEnv()
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

// A broken configuration must not mount the endpoint. Half-configured is the
// dangerous state: peers would reach a live endpoint backed by nothing.
func TestBrokenConfigMountsNoEndpoint(t *testing.T) {
	t.Setenv("MPC_ENABLED", "true")
	t.Setenv("MPC_PEERS", "https://only-me")
	t.Setenv("MPC_PARTY_INDEX", "0")
	t.Setenv("MPC_PEER_TOKEN", "0123456789abcdef0123456789abcdef")

	mux := http.NewServeMux()
	if node := registerMPCRoutes(mux); node != nil {
		t.Fatal("a node was returned for an invalid configuration")
	}
	req := httptest.NewRequest(http.MethodPost, mpc.ExchangePath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the exchange endpoint answered with %d despite an invalid configuration", rec.Code)
	}
}
