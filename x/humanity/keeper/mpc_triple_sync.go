package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Keeping the parties' triple counters in step.
//
// WHY THIS EXISTS
//
// Each party stores how many multiplication triples it has consumed in its own
// chain_config row (mpcTripleOffsetKey). Party 0's triple at index k is only
// meaningful against party 1's triple at index k — the pair is what makes the
// multiplication correct. Nothing kept the two counters equal.
//
// They came apart, and it was measured: on 2026-08-24 party 0 stood at 10240
// and party 1 at 4096. Every comparison after that point used non-corresponding
// triples, and the result was neither 0 nor 1. DistributedMatcher refuses to
// interpret such a value — correctly — so the symptom was a 503 telling the
// person to try again, forever.
//
// HOW THEY CAME APART. mpc_client.py's _submit posted to the parties one after
// another and waited for each answer. /mpc/check runs an interactive protocol,
// so party 0 blocked waiting for a peer the client had not asked yet. But
// mpcTriples advances the counter BEFORE handing the triples out (deliberately:
// losing triples to a crash is safe, replaying them is not). So every
// deadlocked attempt burned party 0's triples and none of party 1's. The client
// is fixed to ask both at once — but a fix that only removes today's cause
// leaves the invariant unguarded, and any partial failure would drift again.
//
// WHAT THIS DOES. Before allocating, a party asks its peers for their counters
// and moves to the highest. Consequences, in order of importance:
//
//   - It never moves BACKWARD, so a triple is never reused. Reuse is the one
//     truly dangerous outcome: a reused triple stops blinding and leaks the
//     difference of the secrets it was used on.
//   - It self-heals existing drift on the next comparison. No manual database
//     surgery, which is what the alternative would have been.
//   - It costs one small HTTP round trip per comparison, not per candidate.
//
// A peer that cannot be reached is skipped rather than fatal: the comparison
// then either works (counters happened to agree) or fails loudly the way it
// does today. Refusing to compare because a peer is briefly unreachable would
// turn a transient blip into a refused registration.

const mpcTripleOffsetKey = "mpc_triple_offset"

// mpcTripleOffsetPath is served by every party for its peers.
const mpcTripleOffsetPath = "/mpc/triple-offset"

// peerOffsetTimeout is short on purpose. This is a single row read on the
// peer; if it cannot answer quickly the comparison that follows would not have
// worked anyway.
var peerOffsetTimeout = 5 * time.Second

var peerOffsetClient = &http.Client{Timeout: peerOffsetTimeout}

// handleMPCTripleOffset reports this party's triple counter.
//
// Authorized with the same client token as /mpc/enroll and /mpc/check. The
// number says how much of a shared, finite resource has been spent; it is not
// secret in the way a share is, but it is not public information either, and
// an unauthenticated counter would let anyone probe how much MPC activity a
// validator has seen.
func (a *APIServer) handleMPCTripleOffset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		mpcJSONError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	if !mpcClientAuthorized(r) {
		mpcJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"offset": a.localTripleOffset()})
}

func (a *APIServer) localTripleOffset() int {
	raw := a.blockchain.state.getConfigValueDB(mpcTripleOffsetKey)
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// syncedTripleOffset returns the highest counter across this party and its
// peers, so every party allocates from the same place.
func (a *APIServer) syncedTripleOffset() int {
	best := a.localTripleOffset()
	if a.mpc == nil {
		return best
	}
	token := os.Getenv("MPC_CLIENT_TOKEN")
	if token == "" {
		return best
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, peer := range a.mpc.peers {
		peer = strings.TrimSpace(peer)
		if peer == "" || isLoopbackURL(peer) {
			continue
		}
		// Skip this node's own URL: asking ourselves adds a round trip and
		// can only return what we already read.
		if a.mpc.selfAddr != "" && strings.EqualFold(peer, a.mpc.selfAddr) {
			continue
		}
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			n, err := fetchPeerTripleOffset(url, token)
			if err != nil {
				fmt.Printf("[MPC] could not read %s's triple counter (%v) — continuing with the "+
					"counters that did answer; a comparison against a party that is out of step "+
					"fails loudly rather than deciding wrongly\n", url, err)
				return
			}
			mu.Lock()
			if n > best {
				best = n
			}
			mu.Unlock()
		}(peer)
	}
	wg.Wait()
	return best
}

func fetchPeerTripleOffset(baseURL, token string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+mpcTripleOffsetPath, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := peerOffsetClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Offset int `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	if body.Offset < 0 {
		return 0, fmt.Errorf("negative offset %d", body.Offset)
	}
	return body.Offset, nil
}
