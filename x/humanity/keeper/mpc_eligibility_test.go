package keeper

import (
	"testing"
	"time"
)

// A validator becomes a committee CANDIDATE only by offering to serve.
//
// Committee selection draws by hash from the candidate list. Before this, that
// list was every registered peer with a signing address — including the great
// majority that never enabled MPC. Drawing one of those produces a committee
// with a member that cannot take part, and since additive sharing is
// all-or-nothing, every comparison it is asked for stalls. Registration halts
// for everybody, and the cause is a node that did nothing wrong by joining.
//
// The default is false, so a peer running older code is never drawn.
func TestOnlyMPCReadyPeersAreCommitteeCandidates(t *testing.T) {
	reg := &PeerRegistry{peers: map[string]time.Time{}}

	reg.RegisterWithMPC("https://serves.example", "0x1111111111111111111111111111111111111111", true)
	reg.RegisterWithMPC("https://declines.example", "0x2222222222222222222222222222222222222222", false)
	// A peer registered the plain way, as older nodes do.
	reg.RegisterWithAddress("https://legacy.example", "0x3333333333333333333333333333333333333333")
	// And one that never proved a signing address at all.
	reg.Register("https://anonymous.example")

	got := reg.MPCCandidates("https://self.example", "0x9999999999999999999999999999999999999999")

	urls := map[string]bool{}
	for _, p := range got {
		urls[p.URL] = true
	}
	if !urls["https://serves.example"] {
		t.Error("a peer that advertised MPC was not eligible")
	}
	for _, unwanted := range []string{
		"https://declines.example",
		"https://legacy.example",
		"https://anonymous.example",
	} {
		if urls[unwanted] {
			t.Errorf("%s is a candidate but never offered to serve. Drawing it would build a "+
				"committee with a member that cannot take part, and additive sharing is "+
				"all-or-nothing — every comparison would stall and registration would halt",
				unwanted)
		}
	}
}

// A node that has not configured MPC must not offer ITSELF either, for the same
// reason. The caller signals that by passing an empty address.
func TestANodeWithoutMPCDoesNotOfferItself(t *testing.T) {
	reg := &PeerRegistry{peers: map[string]time.Time{}}
	reg.RegisterWithMPC("https://peer.example", "0x1111111111111111111111111111111111111111", true)

	got := reg.MPCCandidates("https://self.example", "")
	for _, p := range got {
		if p.URL == "https://self.example" {
			t.Fatal("a node with no MPC configuration listed itself as a committee candidate")
		}
	}

	withMPC := reg.MPCCandidates("https://self.example", "0x9999999999999999999999999999999999999999")
	found := false
	for _, p := range withMPC {
		if p.URL == "https://self.example" {
			found = true
		}
	}
	if !found {
		t.Error("a node that IS serving did not offer itself — the committee could never " +
			"include the node that formed it")
	}
}
