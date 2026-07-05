package keeper

import (
	"testing"
	"time"
)

// TestPeerRegistry_ActivePeersExcludesStaleAndSelf is the regression guard
// for the 2026-07-05 audit finding: api.go's /api/peers handler used to
// call AllPeers(), which returns every URL ever registered with no
// staleness filter at all — a peer that registered once and then
// permanently disappeared (crashed, decommissioned, IP reassigned) stayed
// in every future discovery response forever. ActivePeers already existed
// with the correct 5-minute-heartbeat + self-exclusion semantics but had
// zero callers anywhere in the codebase before this fix wired it in.
func TestPeerRegistry_ActivePeersExcludesStaleAndSelf(t *testing.T) {
	pr := &PeerRegistry{peers: make(map[string]time.Time)}
	pr.peers["http://fresh-peer:8080"] = time.Now()
	pr.peers["http://stale-peer:8080"] = time.Now().Add(-10 * time.Minute) // e.g. a decommissioned node
	pr.peers["http://self:8080"] = time.Now()

	active := pr.ActivePeers("http://self:8080")

	hasStale, hasSelf, hasFresh := false, false, false
	for _, u := range active {
		switch u {
		case "http://stale-peer:8080":
			hasStale = true
		case "http://self:8080":
			hasSelf = true
		case "http://fresh-peer:8080":
			hasFresh = true
		}
	}
	if hasStale {
		t.Fatal("ActivePeers must exclude a peer whose last heartbeat is older than 5 minutes")
	}
	if hasSelf {
		t.Fatal("ActivePeers must exclude selfURL")
	}
	if !hasFresh {
		t.Fatal("ActivePeers must still include a genuinely recently-heartbeating peer")
	}

	// AllPeers, by contrast, returns the stale entry too — documenting
	// exactly why handlePeers (api.go) switching from AllPeers() to
	// ActivePeers() is a real behavior change, not just a rename.
	all := pr.AllPeers()
	found := false
	for _, u := range all {
		if u == "http://stale-peer:8080" {
			found = true
		}
	}
	if !found {
		t.Fatal("AllPeers should still return the stale entry — this documents the exact gap ActivePeers closes, not a claim that AllPeers itself is broken")
	}
}
