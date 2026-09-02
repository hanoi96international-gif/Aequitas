package keeper

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestMPCReadyIsActuallySent is the regression guard for a field that was dead
// for its entire existence.
//
// handlePeerRegister has always decoded mpc_ready into PeerRegistry.mpcReady,
// and MPCCandidates has always required it before a peer may join a committee.
// Nothing ever sent it. Measured 2026-08-23 against both production boxes, with
// a valid client token and a well-formed row:
//
//	503 "mpc: no committee could be formed from 1 candidates: mpc: 1 validators
//	     advertise an MPC endpoint, need 2 for a committee of that size"
//
// Each node counted itself and nobody else, on any number of nodes, forever.
// The failure is silent: the matching service's shadow run swallows the 503, so
// registrations keep succeeding while the corpus that is supposed to calibrate
// the MPC threshold stays empty and looks exactly like a healthy one.
//
// Asserted against the source rather than by standing up two nodes: the thing
// that was wrong is that the key is absent from the request body, and that is
// what this reads. Same approach as the matching service's own
// test_palm_is_not_collected.py.
func TestMPCReadyIsActuallySent(t *testing.T) {
	src, err := os.ReadFile("sync_blocks.go")
	if err != nil {
		t.Fatalf("cannot read sync_blocks.go: %v", err)
	}
	body := string(src)

	if !strings.Contains(body, `"mpc_ready":`) {
		t.Fatal("the peer registration body no longer carries mpc_ready — " +
			"committee selection needs it, and without it every node counts only itself")
	}
	// A bool in a map[string]string cannot compile, and encoding it as the
	// string "true" would decode into Go's bool field as false: the same dead
	// field with more steps.
	if strings.Contains(body, `body, _ := json.Marshal(map[string]string{`) {
		t.Fatal("the registration body is a map[string]string again — mpc_ready " +
			"cannot travel as a bool through it")
	}
}

// TestHeartbeatDoesNotClearMPCReadiness guards the second half of the same
// fault, which would have made sending the field pointless anyway.
//
// RegisterWithAddress used to call RegisterWithMPC(url, addr, false), asserting
// "this peer does not serve MPC" on every plain heartbeat — and Register() runs
// on every sync cycle. A peer that had advertised readiness was demoted again
// within seconds, so the candidate list could never grow past this node itself.
//
// Silence is not the same as "no". Only the authenticated registration that
// actually carries the field gets to answer the question.
func TestHeartbeatDoesNotClearMPCReadiness(t *testing.T) {
	pr := &PeerRegistry{peers: make(map[string]time.Time)}
	const peer = "http://peer:8080"
	const addr = "0x1a37dcdaa42cf3f7e1f6e41379961f40df44a4e3"

	pr.RegisterWithMPC(peer, addr, true)
	pr.Register(peer)                  // the sync-cycle heartbeat
	pr.RegisterWithAddress(peer, addr) // and the address-carrying variant

	candidates := pr.MPCCandidates("http://self:8080", "0x0be8b961cbf6564bd1931b0803d35c0659e0d016")

	found := false
	for _, c := range candidates {
		if c.URL == peer {
			found = true
		}
	}
	if !found {
		t.Fatal("a heartbeat dropped a peer that had advertised MPC readiness — " +
			"no committee can form once that happens")
	}
}

// TestExplicitNotReadyStillWins: preserving readiness must not mean it can never
// be withdrawn. A node that restarts without MPC says so through the same
// authenticated path, and that answer has to take effect.
func TestExplicitNotReadyStillWins(t *testing.T) {
	pr := &PeerRegistry{peers: make(map[string]time.Time)}
	const peer = "http://peer:8080"
	const addr = "0x1a37dcdaa42cf3f7e1f6e41379961f40df44a4e3"

	pr.RegisterWithMPC(peer, addr, true)
	pr.RegisterWithMPC(peer, addr, false)

	for _, c := range pr.MPCCandidates("http://self:8080", "0x0be8b961cbf6564bd1931b0803d35c0659e0d016") {
		if c.URL == peer {
			t.Fatal("a peer that withdrew its MPC readiness is still a committee candidate — " +
				"a drawn member that cannot take part stalls every comparison")
		}
	}
}

// TestMPCServingDefaultsFalse: the advertisement must be earned by actually
// mounting the exchange endpoint, never by an env var. MPC_ENABLED=true with a
// missing triple file or a bad party index leaves the endpoint unmounted, and a
// node that advertised on the strength of the variable would be drawn into
// committees it cannot serve — halting registration for everyone, not just for
// itself.
func TestMPCServingDefaultsFalse(t *testing.T) {
	if MPCServing() {
		t.Fatal("MPCServing() is true without registerMPCRoutes having mounted anything")
	}
}
