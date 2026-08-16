package keeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// handlePeerStatuses answers "what is every other node in this network, and is
// it on the same build as me?" — from the NODE, not from the visitor's browser.
//
// WHY IT MOVED SERVER-SIDE (audit 2026-08-16, reported live). The Network tab
// used to fetch each peer's /api/status directly from the page. That cannot
// work, and the page's own console said so:
//
//	Connecting to 'http://173.249.37.118:8080/api/status' violates the following
//	Content Security Policy directive: "connect-src 'self' https://aequitas.digital …"
//
// Every peer read was refused by this page's own CSP, so the primary appeared on
// the secondary's Network tab as a node whose role could not be verified, and
// the build-commit column was permanently "—". The CSP is right and should stay:
// widening connect-src to arbitrary peer origins to power a diagnostic panel is
// a bad trade.
//
// It gets worse on 2026-08-18, when aequitas.digital starts serving this page
// over HTTPS while the validators answer on plain HTTP :8080 — those reads would
// then be blocked as mixed content too, which no CSP change can fix.
//
// The node has no such restrictions: it already talks to its peers constantly.
// So it asks, and the page makes one same-origin call.
//
// The peer list comes from GlobalPeerRegistry — this node's own view — and never
// from the request. There is no URL parameter to point this at an arbitrary
// host, which is what keeps it from becoming an SSRF gadget; each URL is checked
// against isAllowedPeerURL anyway, the same gate the sync path uses.

type peerStatus struct {
	URL       string `json:"url"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
	IsPrimary *bool  `json:"is_primary,omitempty"`
	Height    *int64 `json:"height,omitempty"`
	GitCommit string `json:"git_commit,omitempty"`
	Humans    *int   `json:"total_humans,omitempty"`
}

const peerStatusTTL = 15 * time.Second

var (
	peerStatusMu    sync.Mutex
	peerStatusCache []peerStatus
	peerStatusAt    time.Time
)

func (a *APIServer) handlePeerStatuses(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)

	peerStatusMu.Lock()
	fresh := time.Since(peerStatusAt) < peerStatusTTL && !peerStatusAt.IsZero()
	cached := peerStatusCache
	peerStatusMu.Unlock()

	if fresh {
		json.NewEncoder(w).Encode(map[string]interface{}{"peers": cached, "cached": true})
		return
	}

	peers := GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL"))
	out := make([]peerStatus, 0, len(peers))

	// Sequential and tightly bounded: this runs on a validator that is also
	// producing blocks, and the list is two entries long today. A slow or dead
	// peer must cost this handler a second, not a goroutine pile-up.
	client := &http.Client{Timeout: 3 * time.Second}
	for _, p := range peers {
		st := peerStatus{URL: p}
		if !isAllowedPeerURL(p) {
			st.Error = "peer URL failed validation"
			out = append(out, st)
			continue
		}
		resp, err := client.Get(p + "/api/status")
		if err != nil {
			st.Error = "unreachable"
			out = append(out, st)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != 200 {
			st.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			out = append(out, st)
			continue
		}
		var s struct {
			IsPrimary   bool   `json:"is_primary"`
			Height      int64  `json:"height"`
			GitCommit   string `json:"git_commit"`
			TotalHumans int    `json:"total_humans"`
		}
		if json.Unmarshal(body, &s) != nil {
			st.Error = "peer answered something that is not a status"
			out = append(out, st)
			continue
		}
		st.Reachable = true
		st.IsPrimary = &s.IsPrimary
		st.Height = &s.Height
		st.GitCommit = s.GitCommit
		st.Humans = &s.TotalHumans
		out = append(out, st)
	}

	peerStatusMu.Lock()
	peerStatusCache = out
	peerStatusAt = time.Now()
	peerStatusMu.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{"peers": out, "cached": false})
}
