package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// One allocator decides which triples a comparison uses.
//
// # WHY A COUNTER PER PARTY COULD NOT WORK
//
// Each party used to keep its own consumed-triples counter. Party 0's triple at
// index k is only correct against party 1's triple at index k, so the counters
// had to stay equal -- and nothing made them. Measured on 2026-08-24: party 0
// stood at 10240, party 1 at 4096. Every comparison after that used
// non-corresponding triples and produced a value that was neither 0 nor 1.
// DistributedMatcher refuses to read such a value as a statement about a
// person, correctly, so the symptom was a permanent 503.
//
// They came apart because mpc_client.py asked the parties one after another
// while /mpc/check runs an interactive protocol: party 0 blocked on a peer the
// client had not asked yet, and mpcTriples advances BEFORE handing triples out
// (deliberately -- losing triples to a crash is safe, replaying them is not).
// So every deadlocked attempt burned party 0's supply and none of party 1's.
//
// WHY "ASK THE PEERS AND TAKE THE MAXIMUM" ALSO DID NOT WORK. That was the
// first attempt here, and it is a race, not a synchronisation: read and advance
// are not atomic. Measured, with the two parties 2048 apart -- party 1 read
// party 0's 18432 and wrote 20480; party 0 then read that 20480 and wrote
// 22528. Both had faithfully taken the maximum, and the gap survived intact,
// because each read the other AFTER it had already moved.
//
// WHAT THIS DOES INSTEAD. The party at index 0 is the allocator. Every party,
// including the allocator itself, asks it which range this session gets, and
// the answer is recorded per session:
//
//   - both parties therefore use the SAME range, by construction rather than by
//     agreement. There is nothing left to drift.
//   - a repeat of the same session returns the same range. That is correct, not
//     a leak: retrying one comparison re-blinds the same secrets with the same
//     triples, which reveals nothing a completed run would not have. Reuse is
//     only dangerous across DIFFERENT secrets, and a new session always gets a
//     new range.
//   - the allocator's counter is the single source of truth, so the historical
//     gap disappears on the next comparison without touching either database.
//
// If the allocator is unreachable, no comparison runs. That is the safe
// direction: a duplicate check that could not happen must never be reported as
// "not a duplicate", which is the irreversible mistake.
const (
	mpcTripleOffsetKey  = "mpc_triple_offset"
	mpcTripleRangePath  = "/mpc/triple-range"
	mpcTripleSessionKey = "mpc_triple_session:"
)

var (
	tripleAllocMu     sync.Mutex
	tripleHTTPTimeout = 10 * time.Second
	tripleHTTPClient  = &http.Client{Timeout: tripleHTTPTimeout}
)

type tripleRangeRequest struct {
	Session string `json:"session"`
	Want    int    `json:"want"`
}

type tripleRangeResponse struct {
	Offset int `json:"offset"`
}

// handleMPCTripleRange allocates (or recalls) the triple range for one session.
//
// Only meaningful on the allocator; a non-allocator still answers, which keeps
// the endpoint uniform and makes a misconfigured peers list fail loudly at the
// comparison rather than silently handing out two different ranges.
func (a *APIServer) handleMPCTripleRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		mpcJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if !mpcClientAuthorized(r) {
		mpcJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req tripleRangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		mpcJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Session) == "" {
		mpcJSONError(w, http.StatusBadRequest, "session is required")
		return
	}
	if req.Want <= 0 {
		mpcJSONError(w, http.StatusBadRequest, "want must be positive")
		return
	}
	offset, err := a.allocateTripleRange(req.Session, req.Want)
	if err != nil {
		mpcJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tripleRangeResponse{Offset: offset})
}

// allocateTripleRange hands out this session's range, allocating it once.
func (a *APIServer) allocateTripleRange(session string, want int) (int, error) {
	// Serialised in-process: two registrations arriving together must not read
	// the same counter and both advance from it.
	tripleAllocMu.Lock()
	defer tripleAllocMu.Unlock()

	sessionKey := mpcTripleSessionKey + session
	if raw := a.blockchain.state.getConfigValueDB(sessionKey); raw != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v >= 0 {
			return v, nil
		}
	}

	offset := 0
	if raw := a.blockchain.state.getConfigValueDB(mpcTripleOffsetKey); raw != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 {
			offset = v
		}
	}

	// Record the session BEFORE advancing the counter. A crash between the two
	// then costs a repeat of one comparison, not a silently reused range.
	if err := a.blockchain.state.setConfigValueDB(sessionKey, strconv.Itoa(offset)); err != nil {
		return 0, fmt.Errorf("mpc: could not record this session's triple range: %w", err)
	}
	if err := a.blockchain.state.setConfigValueDB(mpcTripleOffsetKey, strconv.Itoa(offset+want)); err != nil {
		return 0, fmt.Errorf("mpc: could not record triple consumption, refusing to proceed "+
			"rather than risk reusing them: %w", err)
	}
	return offset, nil
}

// tripleRangeFor returns the range every party must use for this session.
func (a *APIServer) tripleRangeFor(session string, want int) (int, error) {
	if a.mpc == nil {
		return 0, fmt.Errorf("mpc: this node is not an MPC party")
	}
	allocator, err := a.tripleAllocatorURL()
	if err != nil {
		return 0, err
	}
	if allocator == "" {
		// This node IS the allocator.
		return a.allocateTripleRange(session, want)
	}
	return fetchTripleRange(allocator, os.Getenv("MPC_CLIENT_TOKEN"), session, want)
}

// tripleAllocatorURL returns the allocator's URL, or "" when this node is it.
func (a *APIServer) tripleAllocatorURL() (string, error) {
	if len(a.mpc.peers) == 0 {
		return "", fmt.Errorf("mpc: no peers configured, so no allocator can be chosen")
	}
	if a.mpc.index == 0 {
		return "", nil
	}
	url := strings.TrimSpace(a.mpc.peers[0])
	if url == "" {
		return "", fmt.Errorf("mpc: party 0 has no URL, so no allocator can be reached")
	}
	return url, nil
}

func fetchTripleRange(baseURL, token, session string, want int) (int, error) {
	body, err := json.Marshal(tripleRangeRequest{Session: session, Want: want})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(baseURL, "/")+mpcTripleRangePath, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := tripleHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("mpc: the triple allocator (%s) is unreachable: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("mpc: the triple allocator (%s) answered HTTP %d", baseURL, resp.StatusCode)
	}
	var out tripleRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Offset < 0 {
		return 0, fmt.Errorf("mpc: the triple allocator returned a negative offset %d", out.Offset)
	}
	return out.Offset, nil
}
