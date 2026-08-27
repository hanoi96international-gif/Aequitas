package keeper

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
)

type APIServer struct {
	blockchain        *BlockDAG
	p2pNode           *P2PNode
	startTime         time.Time
	proofServerStatus map[string]interface{}
	proofStatusMu     sync.RWMutex
	state             *ChainState
	// Shared EVM RPC server — one instance so all registration calls share
	// the same nonce map and mutex, preventing parallel registrations from
	// reading the same DB nonce and writing the same follower value.
	evmRPC *EVMRPCServer

	// This node's participation in secure duplicate matching, or nil when it
	// is not one of the parties. Nil is the safe value: no MPC at all, rather
	// than one machine performing a two-party protocol by itself.
	mpc *mpcNode
}

// FIX (P2-7, beta-launch audit 2026-07-05): NewAPIServer used to also take a
// *Keeper (the package's separate, legacy in-memory human registry,
// keeper.go) purely to store it in a field nothing ever read — real
// registration has always gone entirely through ChainState.RegisterHumanAtomic
// (state.go). Removed the whole dead type rather than leave it sitting there
// as a tempting, always-empty duplicate of real state (see the 2026-06-22
// audit's divergence-bug class this exact pattern already caused once).
func NewAPIServer(bc *BlockDAG, p2p *P2PNode, state *ChainState) *APIServer {
	s := &APIServer{
		blockchain:        bc,
		p2pNode:           p2p,
		startTime:         time.Now(),
		proofServerStatus: map[string]interface{}{},
		state:             state,
		evmRPC:            NewEVMRPCServer(bc, state),
	}
	SafeGoroutine("syncProofServerStatus", s.syncProofServerStatus)
	// FIX (audit 2026-06-28 recheck 4, P1-5): periodically retry any queued
	// proof-server bio_hash sync failures (see proof_server_sync_queue's
	// table comment in state.go and notifyProofServerWithRetryQueue in
	// register.go) — without this, a registration whose initial sync
	// attempt failed would stay queued forever with nothing ever
	// re-attempting it.
	SafeGoroutine("RetryProofServerSyncQueue-loop", func() {
		for {
			time.Sleep(5 * time.Minute)
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-iteration —
			// see panic_recovery.go and registerAndDiscover-ticker's comment in
			// sync_blocks.go for why per-iteration, not just once for the loop.
			SafeCall("RetryProofServerSyncQueue", func() { RetryProofServerSyncQueue(state) })
		}
	})
	// FIX (audit 2026-06-28 recheck 4, P1-6): same retry pattern as the
	// proof-server sync queue above, for EVM mirror slot-write failures —
	// see syncBalanceLocked's comment in evm_storage.go.
	SafeGoroutine("RetryEVMMirrorSyncQueue-loop", func() {
		for {
			time.Sleep(5 * time.Minute)
			SafeCall("RetryEVMMirrorSyncQueue", func() { RetryEVMMirrorSyncQueue(state) })
		}
	})
	// FIX (BRUTAL-P1-01): retry registration_recovery records — EVM-committed
	// registrations whose Go-state sync failed. Runs every 5 minutes; on each
	// pass it calls RegisterHumanAtomic for every unrecovered record and marks
	// them recovered when they succeed or when Go-state already has the wallet
	// (meaning a previous pass or a peer-sync block replay already fixed it).
	SafeGoroutine("RetryRegistrationRecoveries-loop", func() {
		for {
			time.Sleep(5 * time.Minute)
			SafeCall("RetryRegistrationRecoveries", func() {
				if n := state.RetryRegistrationRecoveries(); n > 0 {
					fmt.Printf("[RECOVERY] Recovered %d registration(s) from registration_recovery table\n", n)
				}
			})
		}
	})
	return s
}

// writeJSON sets the standard JSON response Content-Type header. Centralizes
// boilerplate that was previously duplicated across ~35 handlers individually
// (P3-c, audit 2026-07-06).
// setHSTS asks the browser to refuse plain HTTP for this host for a year.
//
// FIX (M4, Audit 2026-08-18): the node set CSP, X-Frame-Options,
// X-Content-Type-Options and Referrer-Policy carefully on every HTML route and
// no Strict-Transport-Security anywhere — not here and not in deploy/Caddyfile,
// which explicitly delegates security headers to this file. Caddy redirects
// HTTP to HTTPS, but a redirect only protects a visitor who already arrived
// safely; without HSTS the FIRST request of a session is still downgradeable,
// on pages where wallets are connected and registrations are signed.
//
// Sent only over TLS. Emitting it on a plain-HTTP response is meaningless (a
// browser ignores it) and actively harmful on a node reached directly on :8080
// over HTTP for diagnostics, which would pin that host to HTTPS it does not
// serve. r.TLS covers direct TLS; X-Forwarded-Proto covers the Caddy path,
// where the proxy terminates TLS and the node itself sees plain HTTP.
//
// No preload directive: preloading is a one-way door for the whole domain,
// including any subdomain that might later need plain HTTP, and it is the
// operator's decision rather than a default.
func setHSTS(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return
	}
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
}

func writeJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
}

// writeJSONCORS is writeJSON plus an unauthenticated CORS allowance — only
// for endpoints meant to be callable from any origin (the public /api/*
// reads the dapp/explorer frontends call directly from the browser).
// Endpoints gated by SNAPSHOT_TOKEN or otherwise restricted to operators
// deliberately use writeJSON instead, to avoid making a browser-based
// cross-origin read of restricted data easier (P3-c, audit 2026-07-06).
func writeJSONCORS(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// jsonError writes a properly JSON-marshaled error response, preventing JSON
// injection via concatenated error strings that may contain quote characters.
func jsonError(w http.ResponseWriter, msg string, code int) {
	writeJSON(w)
	w.WriteHeader(code)
	enc, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(enc)
}

// jsonStateError answers a failure that came back from the state layer without
// handing the caller the internal error text.
//
// FIX (audit 2026-08-15): three endpoints — register-validator-key,
// set-guardian and recover-escrow — passed the state layer's error straight
// through as `jsonError(w, err.Error(), 400)`. Those functions wrap whatever
// went wrong underneath (`could not persist recovered balance for %s: %w`), so
// on a database failure an unauthenticated caller received the driver's own
// message: pq error codes, constraint and column names, and on a connection
// failure the host and database being dialed. That is the same leak class the
// proof server was fixed for on 2026-07-12; the chain's own API still had it.
//
// The status code was wrong in the same breath. A DB outage was reported as
// 400, telling the caller its request was malformed when the request was fine
// and the server was not — a client that trusts that will never retry.
//
// Genuine validation failures ("already registered", "no escrow to recover")
// are still passed through verbatim: they are the caller's business, they carry
// nothing internal, and existing clients display them.
func jsonStateError(w http.ResponseWriter, what, subject string, err error) {
	if isInternalError(err) {
		// Server-side only — the operator needs the detail, the caller does not.
		fmt.Printf("[API] %s failed for %s: %v\n", what, subject, err)
		jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		return
	}
	jsonError(w, err.Error(), http.StatusBadRequest)
}

// isInternalError reports whether err came from infrastructure (database,
// driver, cancelled context) rather than from this project's own validation.
// Detected through the wrap chain, so a validation error that merely happens to
// wrap one still counts as internal — the conservative direction.
func isInternalError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return true
	}
	for _, target := range []error{
		sql.ErrConnDone, sql.ErrTxDone, sql.ErrNoRows,
		context.DeadlineExceeded, context.Canceled, driver.ErrBadConn,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	// Not every driver failure is one of the sentinels above (a dial failure is
	// a plain *net.OpError, and it carries the host being connected to).
	var netErr net.Error
	return errors.As(err, &netErr)
}

// readBodyLimited reads r.Body capped at maxBytes.
//
// FIX (Monster Audit 2026-07-12, P2): several handlers used to read via
// io.ReadAll(io.LimitReader(r.Body, N)) — io.LimitReader silently truncates
// at N bytes with a nil error, so an oversized request was never rejected,
// just fed to json.Unmarshal as truncated (usually-but-not-guaranteed-invalid)
// JSON. http.MaxBytesReader is the correct primitive for an enforced limit:
// it returns a distinguishable *http.MaxBytesError once the client exceeds
// it, which callers can use to send a real 413 instead of a generic 400/500.
func readBodyLimited(w http.ResponseWriter, r *http.Request, maxBytes int64) (body []byte, tooLarge bool, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	body, err = io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		tooLarge = errors.As(err, &maxBytesErr)
	}
	return body, tooLarge, err
}

// isValidWalletAddr checks 0x-prefixed 40-hex-char Ethereum address format.
// P3-11: prevents garbage keys from entering cs.accounts map.
func isValidWalletAddr(addr string) bool {
	if len(addr) != 42 {
		return false
	}
	if addr[:2] != "0x" && addr[:2] != "0X" {
		return false
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// FIX (audit recheck2, P2 #1): this used to fall back to a specific,
// hardcoded Railway URL (the project's own original deployment) whenever
// PROOF_SERVER_URL was unset. For a project whose whole point is letting
// independent operators run their own node, silently routing proof
// requests — and CHAIN_SERVICE_TOKEN, via addProofServerAuth — to a
// specific third party's infrastructure on a misconfiguration is exactly
// backwards: it should fail loudly and locally, not succeed quietly
// against someone else's server. proofServerBaseURL now returns "" if
// unset; every caller below checks that explicitly via
// requireProofServerConfigured instead of building a request against an
// empty/wrong base URL.
func proofServerBaseURL() string {
	return strings.TrimRight(os.Getenv("PROOF_SERVER_URL"), "/")
}

// proofServerURLs returns every configured proof-server instance, for
// fairness/decentralization: PROOF_SERVER_URLS (comma-separated, new) lets
// an operator run one independent proof-server per validator box instead of
// funneling every node through a single instance. Falls back to the
// single PROOF_SERVER_URL for backward compatibility with existing
// single-instance deployments. Mirrors the PRIMARY_NODE_URLS pattern
// (sync_blocks.go) already used for multi-seed peer discovery.
//
// Correctness of running multiple UN-synchronized proof-server instances
// (each with its own local Postgres bio_hashes cache) rests on that cache
// being a pre-filter/rate-limit optimization only, never the authoritative
// uniqueness check: /prove and /store-bio both verify the wallet's is_human
// status against THIS chain's own (already consensus-replicated) state
// before trusting anything, so two instances disagreeing about their local
// cache can't actually let a duplicate registration through on-chain.
func proofServerURLs() []string {
	var out []string
	if raw := os.Getenv("PROOF_SERVER_URLS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimRight(strings.TrimSpace(u), "/")
			if u != "" {
				out = append(out, u)
			}
		}
	}
	if len(out) == 0 {
		if base := proofServerBaseURL(); base != "" {
			out = append(out, base)
		}
	}
	return out
}

// requireProofServerConfigured writes a clear 503 and returns ok=false if no
// proof-server URL is configured at all, so callers can bail out before
// constructing a request against an empty base URL (http.NewRequest with a
// schemeless, hostless URL like "/prove" fails, and the discarded error from
// that would otherwise nil-panic on the very next line that sets a header on
// the request). Returns the first configured URL for callers that don't need
// failover (kept for compatibility with call sites not yet migrated to
// doProofServerRequestFailover).
func requireProofServerConfigured(w http.ResponseWriter) (string, bool) {
	urls := proofServerURLs()
	if len(urls) == 0 {
		http.Error(w, `{"error":"no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node"}`, 503)
		return "", false
	}
	return urls[0], true
}

// doProofServerRequestFailover tries every configured proof-server URL in
// order, moving to the next only on a genuine request failure (network
// error, connection refused, timeout) -- NOT on a non-2xx HTTP status, since
// e.g. a 409 "already registered" from the first instance that answered is a
// meaningful response to forward, not a reason to ask a second instance
// (which would just waste a round-trip repeating the same authoritative
// on-chain check). Returns the first response that was actually received,
// or the last transport error if every URL failed.
func doProofServerRequestFailover(method, path string, body []byte, timeout time.Duration, extraHeaders map[string]string) (*http.Response, error) {
	urls := proofServerURLs()
	if len(urls) == 0 {
		return nil, fmt.Errorf("no proof-server URL configured")
	}
	client := proofProxyClient(timeout)
	return attemptURLsInOrder(urls, func(base string) (*http.Response, error) {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, base+path, bodyReader)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		addProofServerAuth(req)
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		return client.Do(req)
	})
}

// attemptURLsInOrder tries fn against each url in order, returning the
// first successful response (fn returning a nil error) or the last error
// if every url failed. Split out from doProofServerRequestFailover's
// request-building so this ordering/failover decision -- not the network
// I/O itself, which goes through pinningDialer's SSRF protections either
// way -- can be unit-tested without real network targets.
func attemptURLsInOrder(urls []string, fn func(url string) (*http.Response, error)) (*http.Response, error) {
	var lastErr error
	for _, url := range urls {
		resp, err := fn(url)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func addProofServerAuth(req *http.Request) {
	if tok := os.Getenv("CHAIN_SERVICE_TOKEN"); tok != "" {
		req.Header.Set("x-chain-token", tok)
	}
}

// proofProxyClient returns an http.Client for calling out to PROOF_SERVER_URL
// with pinningDialer (sync_blocks.go) and redirect-blocking, instead of a
// bare http.Client.
//
// FIX (audit recheck3, P2 — "Chain-Proof-Proxy validiert PROOF_SERVER_URL
// nicht gegen SSRF-Klasse"): notifyProofServer (register.go) already used
// httpSyncClient for exactly this reason, but every proof-proxy handler
// below (syncProofServerStatus, handleSepoliaHumans, handleProveProxy,
// handleProveGetProxy, handleProofCheckProxy) built a bare *http.Client
// with no IP validation and no redirect blocking. PROOF_SERVER_URL is an
// operator-set config value, not directly attacker-controlled, so this
// isn't remotely exploitable on its own — but a misconfigured value (or a
// proof server that starts redirecting) could make this chain node issue
// requests to a private/internal address, and CHAIN_SERVICE_TOKEN
// (addProofServerAuth) would be sent along with them. Each call site needs
// its own timeout (proof generation can legitimately take up to 120s),
// so this takes one instead of being a single shared client like
// httpSyncClient.
// FIX (2026-08-14, found live): pinningDialer rejects any address that
// resolves to a private/loopback IP. That is exactly right for PEER URLs,
// which are discovered from the network and effectively attacker-influenced —
// but it is wrong here, and it made the intended proof-server deployment
// impossible.
//
// The proof server is this operator's own component, and it is supposed to be
// unreachable from the internet: it listens only on loopback or on a private
// Docker network, and clients reach it through this node's authenticated
// /api/prove* proxy. Both production boxes run it exactly that way (container
// "proof-server" on the aequitas-net bridge). Every sane value of
// PROOF_SERVER_URLS is therefore private by design —
// http://127.0.0.1:3000 or http://proof-server:3000 — and pinningDialer
// refused all of them, so the proxy answered 502 "proof server unreachable"
// while the service was up and answering /health from the very same node
// container. Confirmed live on both boxes.
//
// Dropping the private-IP check here does not reintroduce SSRF. The danger
// pinningDialer defends against is an ATTACKER choosing the destination;
// PROOF_SERVER_URLS is set only by whoever configures the process, and no
// request field influences it. What did matter from the original hardening is
// kept: redirects are still never followed, so a compromised or misconfigured
// proof server cannot bounce this node — carrying CHAIN_SERVICE_TOKEN — to an
// arbitrary third address.
func proofProxyClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		},
	}
}

func (a *APIServer) syncProofServerStatus() {
	for {
		if len(proofServerURLs()) == 0 {
			time.Sleep(30 * time.Second)
			continue
		}
		resp, err := doProofServerRequestFailover("GET", "/health", nil, 8*time.Second, nil)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var data map[string]interface{}
			if json.Unmarshal(body, &data) == nil {
				a.proofStatusMu.Lock()
				a.proofServerStatus = data
				a.proofStatusMu.Unlock()
			}
		}
		time.Sleep(30 * time.Second)
	}
}

// handleCombinedHealth answers audit 2026-06-28 full recheck, P2-4: there was
// no single place to check whether BOTH halves of this system (chain node
// and proof server) were actually healthy — an operator had to separately
// curl /api/status here and /health on the proof server, then manually
// reconcile two different response shapes. This reuses the existing
// syncProofServerStatus() background poller (already running, already
// caching the proof server's last known /health response every 30s) instead
// of adding a second outbound HTTP call path; "proof_server_reachable"
// reflects whether that cache currently holds anything.
// handleStateRootComponents serves GET /api/debug/stateroot-components — the
// per-component breakdown of what stateRootLocked hashes (see
// StateRootComponents' own doc comment for why this exists and why it is
// safe to expose). Read-only; changes nothing.
//
// Purpose is strictly operational: when two nodes log a StateRoot mismatch,
// diffing this endpoint across them says WHICH input diverged, turning
// "the roots differ" into a specific, fixable finding.
// handleDAGGates serves GET /api/debug/dag-gates — the internal gates that
// decide whether this node attaches peer blocks and produces its own.
//
// Deliberately lock-free (see BlockDAG.DAGGates): it must answer even while
// cs.mu or dag.mu is held by something slow, because that is exactly the
// situation an operator needs it in. On 2026-07-26 the primary orphaned ~99%
// of incoming blocks while its own status endpoints took 11-16 seconds, and
// no diagnostic could distinguish "a gate is shut" from "the chain forked".
func (a *APIServer) handleDAGGates(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	body, err := json.Marshal(a.blockchain.DAGGates())
	if err != nil {
		jsonError(w, "internal error building response", http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (a *APIServer) handleStateRootComponents(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	body, err := json.Marshal(a.state.StateRootComponentBreakdown())
	if err != nil {
		jsonError(w, "internal error building response", http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

func (a *APIServer) handleCombinedHealth(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	latest := a.blockchain.LatestBlock()
	a.proofStatusMu.RLock()
	proofStatus := a.proofServerStatus
	a.proofStatusMu.RUnlock()
	// FIX (audit 2026-06-28 recheck 5, P1-3): degraded surfaces a failed
	// snapshot bootstrap/resync EVM-mirror migration here, instead of that
	// only ever existing as a one-time startup log line — see
	// SetBootstrapDegraded's own comment.
	degradedReason := a.state.BootstrapDegradedReason()
	// FIX (audit 2026-06-28 recheck 5, P2-1/P2-4): retry-queue depth/age
	// used to live only in printf logs — surfaced here so a stuck backlog
	// (proof-server unreachable, EVM mirror writes failing repeatedly) is
	// visible to an operator checking health instead of requiring a log dive.
	proofQueueCount, proofQueueDeadCount, proofQueueOldestSecs := a.state.CountProofServerSyncQueue()
	evmQueueCount, evmQueueDeadCount, evmQueueOldestSecs := a.state.CountEVMMirrorSyncQueue()
	// FIX (audit 2026-06-28 recheck 5, P2-5): "Beim Start klar in
	// /api/health/combined anzeigen, ob destruktive Maintenance-Flags
	// gesetzt sind." A destructive var that was refused at startup (e.g.
	// RESET_DB_STATE=true but ALLOW_DESTRUCTIVE_MAINTENANCE wasn't set)
	// stays set in the environment and could still trigger on a future
	// restart if conditions change — worth surfacing even when nothing
	// destructive actually ran this time.
	destructiveFlagsSet := []string{}
	if os.Getenv("RESET_DB_STATE") == "true" {
		destructiveFlagsSet = append(destructiveFlagsSet, "RESET_DB_STATE")
	}
	if os.Getenv("CLEAR_REGISTRATIONS") == "true" {
		destructiveFlagsSet = append(destructiveFlagsSet, "CLEAR_REGISTRATIONS")
	}
	if os.Getenv("RESET_STATE") == "true" {
		destructiveFlagsSet = append(destructiveFlagsSet, "RESET_STATE")
	}
	// Monster-Audit P1-06: ALLOW_PEER_SECRET_BYPASS lets any caller register
	// as a peer without the shared PEER_SECRET — fine for testnet/bootstrap,
	// a real security hole if left on in production. Surface it the same way
	// as the other destructive flags so it isn't silently forgotten.
	if os.Getenv("ALLOW_PEER_SECRET_BYPASS") == "true" {
		destructiveFlagsSet = append(destructiveFlagsSet, "ALLOW_PEER_SECRET_BYPASS")
	}

	// FIX (Gesamtaudit 2026-06-28, P2-4/P3-7): "healthy":true used to be
	// hardcoded. Compute a real tri-state (healthy/warn/unhealthy) from the
	// signals already gathered above plus StateRoot mismatch count and last
	// successful peer sync, with concrete recovery guidance attached
	// instead of just "Consider resync" in a log line.
	mismatchCount := a.blockchain.TotalStateRootMismatches()
	finalizedHeight, _ := a.state.GetFinalizedCheckpoint()
	lastSyncAt := a.blockchain.LastSuccessfulPeerSyncAt()
	var lastSyncAgeSecs int64 = -1
	if lastSyncAt > 0 {
		lastSyncAgeSecs = time.Now().Unix() - lastSyncAt
	}
	status := "healthy"
	var notes []string
	if degradedReason != "" {
		status = "unhealthy"
		notes = append(notes, "EVM mirror migration failed at last bootstrap/resync — restart to retry, or re-run with RESYNC_FROM_SNAPSHOT=true if Go-state itself looks wrong too")
	}
	if mismatchCount >= 5 {
		status = "unhealthy"
		notes = append(notes, fmt.Sprintf("%d StateRoot mismatches recorded — this node's state has likely diverged from its peers; recover with RESYNC_FROM_SNAPSHOT=true + BOOTSTRAP_SNAPSHOT_URL + BOOTSTRAP_SIGNER pointed at a healthy peer", mismatchCount))
	} else if mismatchCount > 0 && status == "healthy" {
		status = "warn"
		notes = append(notes, fmt.Sprintf("%d StateRoot mismatch(es) recorded this process — usually self-heals as later blocks catch up; investigate if this keeps climbing", mismatchCount))
	}
	if proofQueueCount > 0 && status == "healthy" {
		status = "warn"
	}
	if evmQueueCount > 0 && status == "healthy" {
		status = "warn"
	}
	if proofQueueDeadCount > 0 || evmQueueDeadCount > 0 {
		if status == "healthy" {
			status = "warn"
		}
		notes = append(notes, fmt.Sprintf(
			"%d proof-server sync and %d EVM-mirror sync entries hit the %d-attempt dead-letter limit — "+
				"retry has permanently stopped; fix the underlying issue and run: "+
				"UPDATE proof_server_sync_queue SET dead=FALSE; UPDATE evm_mirror_sync_queue SET dead=FALSE",
			proofQueueDeadCount, evmQueueDeadCount, retryQueueMaxAttempts,
		))
	}
	if len(destructiveFlagsSet) > 0 {
		status = "warn"
		notes = append(notes, "a destructive maintenance flag is set in this node's environment — see destructive_flags_set")
	}
	// BRUTAL-P1-01: surface pending registration recoveries in health output.
	registrationRecoveryCount := a.state.CountUnrecoveredRegistrations()
	if registrationRecoveryCount > 0 {
		if status == "healthy" {
			status = "warn"
		}
		notes = append(notes, fmt.Sprintf(
			"%d registration(s) have EVM tx committed but Go-state not yet synced — "+
				"background recovery is retrying; see /api/admin/registration-recovery",
			registrationRecoveryCount))
	}
	// Monster-Audit P1-01/P1-02: a chain_blocks write that keeps the DB
	// consistent with in-memory GHOSTDAG/replay state failed persistently
	// this run — see BlockDAG.degraded's struct comment.
	dagDegraded := a.blockchain.IsDegraded()
	dagDegradedReason := a.blockchain.DegradedReason()
	if dagDegraded {
		status = "unhealthy"
		notes = append(notes, "DAG degraded: "+dagDegradedReason)
	}
	// Monster-Audit P1-04: surface synthetic-checkpoint trust-bootstrap mode
	// instead of letting it run invisibly.
	syntheticCheckpointCount := a.blockchain.SyntheticCheckpointCount()
	unverifiedCheckpointCount := a.blockchain.UnverifiedSyntheticCheckpointCount()
	checkpointTrustMode := syntheticCheckpointCount > 0
	if unverifiedCheckpointCount > 0 {
		// Genuine mid-chain gap above the snapshot boundary — should heal as real
		// blocks sync in. Degrade to warn until it does.
		if status == "healthy" {
			status = "warn"
		}
		notes = append(notes, fmt.Sprintf("%d synthetic-checkpoint stub(s) active above the snapshot boundary — bridging a historical gap from peer snapshot trust, not full verification; normally self-heals as real blocks sync in behind them", unverifiedCheckpointCount))
	} else if boundaryStubs := syntheticCheckpointCount - unverifiedCheckpointCount; boundaryStubs > 0 {
		// Only the snapshot-boundary stub(s) remain: the signed-snapshot
		// start-of-history that no node retains blocks below. This is the normal,
		// permanent state of every snapshot-bootstrapped node — informational,
		// not a health concern, and does NOT gate production.
		notes = append(notes, fmt.Sprintf("%d synthetic-checkpoint stub(s) at the snapshot boundary — this node was bootstrapped from a signed snapshot; the boundary block predates all retained history and is trusted like genesis (expected, not an error)", boundaryStubs))
	}
	// FIX (P3-4, beta-launch audit 2026-06-27): checkV7SlotsMatchDeployedVersion
	// used to only print a stdout warning, easy to miss if logs aren't actively
	// monitored. Surfacing it here too means it shows up wherever this
	// dashboard/health check is already watched, same as every other
	// degraded-state note above.
	if !v7SlotsVerifiedFor(V7ContractVersion) {
		if status == "healthy" {
			status = "warn"
		}
		notes = append(notes, fmt.Sprintf(
			"V7ContractVersion (%q) has changed since evm_engine.go's hardcoded storage-slot persistence lists were last verified (%q) — if this version added/removed/reordered a state variable in AequitasV7.sol, writes to any untracked slot will silently never persist",
			V7ContractVersion, v7SlotsVerifiedForVersion))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		// runtime is the answer to "why did the primary restart again?".
		//
		// The primary has restarted under load repeatedly, and every single
		// time the cause stayed unknown — the node reported nothing about its
		// own memory, and Railway's logs need a token this repo does not have.
		// Without these numbers an operator can only observe that it happened.
		// With them, a sample taken before the next restart shows whether the
		// heap was climbing (a leak or an unbounded queue) or flat (killed for
		// some other reason entirely), which are opposite investigations.
		//
		// ReadMemStats briefly stops the world, so this is deliberately only on
		// this operator endpoint — never on /api/status, which the explorer
		// polls every few seconds from every open tab.
		"runtime": runtimeSnapshot(),
		// How much of the wall clock every concurrent transfer spends locked
		// out by an exclusive holder (block replay, distributions). See
		// exclusive_lock_stats.go: busy_pct is an upper bound on how much of
		// the time this node can accept transfers at all, regardless of how
		// well the transfer path itself performs.
		"exclusive_lock": ExclusiveLockStats(),
		// Whether the database connection pool is the constraint — wait_count
		// and wait_total_ms answer that directly, instead of inferring it from
		// a throughput number that swings by 2x between runs. See DBPoolStats.
		"db_pool": a.state.DBPoolStats(),
		// Which path transfers take, and how long each one actually takes —
		// see transfer_stats.go for why a derived 57ms needed measuring.
		"transfer_path": TransferPathStats(),
		// The WAL flush loop, which a mutex profile identified as the single
		// largest source of lock contention in the node (45.21%). addrs_per_flush
		// and hold_avg_ms are the two numbers that explain it; see wal_tuning.go.
		"wal_flush":  WALFlushStats(),
		"admission":  AdmissionStats(),
		"wal_writer": wal.WriterStats(),
		"tx_index":   TxIndexStats(),
		// The request split, so the ~50ms per transfer that TransferAtomic does
		// not account for can be subtracted out instead of guessed at. Read
		// unaccounted_in_send_ms first; see rpc_phase_stats.go.
		"rpc_phases": RPCPhaseStats(),
		// How much of the nonce cost the range reservation removed. Read
		// covered_pct first -- it only applies to consecutive runs from one
		// sender, so a differently-batching client correctly gets none of it.
		"batch_nonce": BatchNonceStats(),
		// The fast path split. Read other_ms first: it is the time no named
		// phase covers, and the named ones only add up to ~7ms of a measured
		// 78ms. See transfer_phase_stats.go.
		"transfer_phases": TransferPhaseStats(),
		// chain_tx_batches hatte keine Obergrenze und keinen DELETE-Pfad;
		// siehe tx_batch_prune.go.
		"tx_batch_prune": TxBatchPruneStats(),
		// Was das Produktionstor zuhaelt: welcher der beiden Reset-Gruende
		// feuert, und ob der Rueckstau-Ausweg ueberhaupt greift.
		// Siehe sync_streak_stats.go.
		"sync_streak": SyncStreakStats(),
		// Who is actually driving the block-serving endpoints, which a CPU
		// profile put at a quarter of the node's CPU with no identifiable
		// caller. See endpoint_stats.go.
		"endpoints": EndpointStats(),
		// How much of the pull path is actually being stripped. Built together
		// with the stripping itself and then never wired up, so the one question
		// it exists to answer -- is the saving being realised, or are bodies
		// unretrievable and everything going out whole? -- was unanswerable.
		"stripped_pull": StrippedPullStats(),
		"chain": map[string]interface{}{
			"status":                     status,
			"notes":                      notes,
			"healthy":                    status == "healthy", // kept for backward compatibility with existing callers
			"degraded_reason":            degradedReason,
			"dag_degraded":               dagDegraded,
			"dag_degraded_reason":        dagDegradedReason,
			"checkpoint_trust_mode":      checkpointTrustMode,
			"synthetic_checkpoint_count": syntheticCheckpointCount,
			"height":                     latest.Height,
			"dag_tips_count":             a.blockchain.TipsCount(),
			// FIX (Monster Audit 2026-07-12, P2): documenting the trust model
			// this counter represents, for anyone building against this API —
			// StateRoot is a diagnostic drift signal, not a consensus
			// commitment (see replayTransactions' StateRoot-mismatch comment
			// in block.go for the full reasoning). 0 here means no drift has
			// been DETECTED yet, not a cryptographic guarantee of identical
			// state across nodes — verify convergence for anything
			// consequential via an exact /api/block?height=N hash match
			// instead of trusting this count alone.
			"state_root_mismatch_count":          mismatchCount,
			"last_successful_peer_sync_age_secs": lastSyncAgeSecs,
			"total_humans":                       a.state.TotalHumans(),
			"total_supply":                       fmt.Sprintf("%.2f AEQ", a.state.TotalSupply()),
			"uptime_secs":                        int64(time.Since(a.startTime).Seconds()),
			"destructive_flags_set":              destructiveFlagsSet,
			"registration_recovery_pending":      registrationRecoveryCount,
			// FIX (audit 2026-06-28 recheck 5, P2-3): "Health/Debug sollte
			// Chain-Nullifier, Chain-BioHash und Proof-BioHash getrennt
			// anzeigen." proof_server.last_status.bio_hash_count (below) is
			// the proof-server side of this comparison.
			// The rule, the ledger, and the gap between them — see
			// SupplyReconciliation. total_supply above is the RULE
			// (humans x 1000); this is what the accounts actually hold.
			"supply_reconciliation": a.state.SupplyReconciliation(),
			// What the daily-round scheduler actually decided. Note this is NOT
			// last_ubi_at: that only advances when a round had something to pay.
			"distribution":     a.state.DistributionHealth(),
			"chain_nullifiers": a.state.CountChainNullifiers(),
			"chain_bio_hashes": a.state.CountChainBioHashes(),
			"proof_server_sync_queue": map[string]interface{}{
				"pending":         proofQueueCount,
				"dead":            proofQueueDeadCount,
				"oldest_age_secs": proofQueueOldestSecs,
			},
			"evm_mirror_sync_queue": map[string]interface{}{
				"pending":         evmQueueCount,
				"dead":            evmQueueDeadCount,
				"oldest_age_secs": evmQueueOldestSecs,
			},
		},
		"proof_server": map[string]interface{}{
			"reachable":   len(proofStatus) > 0,
			"last_status": proofStatus,
		},
		// FIX (P0, 2026-07-10): see SyncDiagnostics' own comment
		// (sync_blocks.go) — these values previously existed nowhere except
		// scattered log lines, forcing every "why won't peer X merge"
		// diagnosis through a slow log-paste-and-guess cycle.
		"sync_diagnostics": map[string]interface{}{
			"boot_height":                   a.blockchain.BootHeight(),
			"boot_height_checkpoint_backed": a.blockchain.BootHeightCheckpointBacked(),
			"finalized_height":              finalizedHeight,
			"finality_height_slack":         finalityHeightSlack,
			"peers":                         a.blockchain.SyncDiagnostics(),
		},
	})
}

// gzipResponseWriter wraps http.ResponseWriter, redirecting Write() through
// a gzip.Writer — see gzipMiddleware's own comment for why this exists.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// gzipMiddleware transparently compresses any response for a client that
// advertises gzip support, EXCEPT /download/ paths (PDFs/APK are already
// compressed formats — gzipping them again burns CPU for no size benefit,
// and both file types risk subtly corrupting on some proxies if double-
// encoded incorrectly). See Start's own call site comment for why this
// exists and why it's a pure win, unlike caching.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/events (SSE) is excluded for two independent reasons, either
		// one sufficient on its own: gzip.Writer buffers internally and is
		// only flushed on Close/explicit Flush (gzipResponseWriter.Write
		// never calls either), so a push would sit buffered indefinitely
		// instead of reaching the client promptly — and gzipResponseWriter
		// doesn't implement http.Flusher at all (embedding the
		// http.ResponseWriter INTERFACE only promotes the methods THAT
		// interface declares, which doesn't include Flush), so
		// handleBlockEvents' own Flusher type-assertion would fail for
		// every gzip-capable client — nearly all of them — turning the
		// whole endpoint into an immediate 500 rather than merely
		// unbuffered.
		// FIX (2026-07-25, 50k-TPS deep-dive, finding 1): /rpc responses are
		// tiny JSON objects (~80-150 bytes: a hash + an id) — gzip's own
		// header/trailer overhead can exceed the uncompressed size, and
		// spinning up a gzip.Writer per request is pure CPU cost with no
		// bandwidth win at exactly the throughput this endpoint needs to
		// sustain. Same rationale as the /download/ exclusion above, applied
		// to small-payload JSON instead of already-compressed binaries.
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || strings.HasPrefix(r.URL.Path, "/download/") || r.URL.Path == "/api/events" || r.URL.Path == "/rpc" || strings.HasSuffix(r.URL.Path, ".png") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		// Pooled and at BestSpeed, both for the same measured reason.
		//
		// A CPU profile taken while 597 senders drove the node put 48.52% of all
		// CPU in handleBlocks and 20.90% in compress/flate alone -- more than
		// three times what the entire signature-recovery path was getting
		// (15.32%). That is the peer sync path: /api/blocks serves up to 500
		// blocks per request, and under load a block carries thousands of
		// transactions, so every catching-up peer pulls megabytes and the node
		// pays to compress all of it. Every restart of this node makes its peers
		// fall behind and then re-sync hard, so this is not a rare condition.
		//
		// Level 1 rather than the default 6: flate's findMatch alone was 7.05%
		// of CPU, and that search is exactly what the higher levels buy. Measured
		// on a real /api/blocks response, 505KB raw compresses 5.7:1 at the
		// default; level 1 gives up a modest part of that ratio for roughly a
		// third of the CPU. This node is CPU-bound at 262% of 600% while peers
		// wait, and is nowhere near bandwidth-bound, so that trade is the right
		// way round.
		//
		// Pooled because gzip.NewWriter allocates its compressor state per
		// request -- visible in the same profile as runtime.mallocgcLarge at
		// 7.03%. A pooled writer is Reset onto the new response instead.
		gz := acquireGzipWriter(w)
		defer releaseGzipWriter(gz)
		next.ServeHTTP(gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

// recoverMiddleware is explicit defense-in-depth panic recovery around the
// whole mux. FIX (P1-2, beta-launch audit 2026-07-05): Go's own net/http
// stdlib already recovers a panic in a request handler (only that one
// connection dies), so this middleware is not closing an active hole today
// — but that stdlib recovery only covers the ORIGINAL request-handling
// goroutine. Any handler that spawns its OWN goroutine (a pattern already
// used throughout this file) reproduces the exact P0-3 crash risk with no
// explicit safety net at the mux level to catch it as a fallback, and nothing
// here would signal that gap to a future handler author. An explicit
// recover() here doesn't help with THAT case either (recover only catches a
// panic in the SAME goroutine — see panic_recovery.go), but it does mean a
// panic in the synchronous request path is handled uniformly and logged the
// same way every other recovered panic in this codebase is, rather than
// relying solely on the stdlib's silent default behavior.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				fmt.Printf("[PANIC RECOVERED] HTTP handler %s %s: %v\n%s\n", r.Method, r.URL.Path, rec, debug.Stack())
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// buildMux registers every route this node serves.
//
// Split out of Start (audit 2026-08-15) so the routing table can be exercised
// by a test at all: Start binds a listener, starts the EVM engine and deploys
// contracts, so nothing about which paths exist — or what an unrouted one
// answers — was reachable without standing up a real node. The catch-all's
// behaviour in particular had never been pinned, and it was wrong.
func (a *APIServer) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/landing", a.handleLanding)
	mux.HandleFunc("/landing.js", a.handleLandingJS)
	mux.HandleFunc("/favicon.svg", a.handleFaviconSVG)
	mux.HandleFunc("/favicon.ico", a.handleFaviconSVG)
	mux.HandleFunc("/apple-touch-icon.png", a.handleAppleTouchIcon)
	mux.HandleFunc("/og-image.png", a.handleOGImage)
	mux.HandleFunc("/explorer.css", a.handleExplorerCSS)
	mux.HandleFunc("/explorer.js", a.handleExplorerJS)
	mux.HandleFunc("/node-binding.js", a.handleNodeBindingJS)
	mux.HandleFunc("/coordinator-binding.js", a.handleCoordinatorBindingJS)
	mux.HandleFunc("/vendor/ethers.min.js", a.handleVendorEthersJS)
	mux.HandleFunc("/vendor/lightweight-charts.min.js", a.handleVendorLightweightChartsJS)
	mux.HandleFunc("/vendor/walletconnect-ethereum-provider.min.js", a.handleVendorWalletConnectJS)
	// Pflichtseiten. Sie MUESSEN vor dem Catch-all stehen, sonst
	// beantwortet dieser sie mit der SPA-Seite und Status 200 -- genau der
	// Zustand, der am 25.08.2026 dazu fuehrte, dass /impressum und
	// /datenschutz zu existieren SCHIENEN, ohne es zu tun.
	mux.HandleFunc("/impressum", a.handleImpressum)
	mux.HandleFunc("/datenschutz", a.handleDatenschutz)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Root path: serve landing page; anything else falls to handleUI
		if r.URL.Path == "/" {
			a.handleLanding(w, r)
			return
		}
		// FIX (audit 2026-08-15): this catch-all answered EVERY unrouted path
		// with HTTP 200 and the full 182 KB explorer page — including paths
		// under /api/ and /debug/. Two concrete consequences, both measured
		// against production:
		//
		//   - No API call can fail cleanly. A mistyped, removed or
		//     not-yet-deployed endpoint returns 200 with an HTML body, so a
		//     caller doing (await fetch(...)).json() gets a parse error, or
		//     worse, code that reads an optional field concludes the field is
		//     absent and states something false about the chain. That is
		//     exactly the defect class fixed twice in this same audit (the
		//     Lorenz curve and loadHumans both turned a non-data response into
		//     a claim about the registry). Renaming an endpoint would silently
		//     do this to every old client instead of failing loudly.
		//   - A 60-byte request produces a 182 KB response, unauthenticated
		//     and unlimited. Every background scanner probing /.env, /wp-admin
		//     and friends is served a full copy of the explorer, so the node
		//     pays roughly 3000x the request's own size in egress for traffic
		//     that was never going to be a visitor.
		//
		// The SPA still needs its client-side routes (/index/distribution,
		// /network/overview, ...) to return the page, so only the two prefixes
		// that are unambiguously machine-facing are answered honestly here.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/debug/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"no such endpoint"}`))
			return
		}
		a.handleUI(w, r)
	})
	// Secure duplicate matching, if this node is one of the parties. The exact
	// pattern beats the "/" SPA fallback on specificity, so peers reach the
	// endpoint rather than a copy of the explorer page.
	a.mpc = registerMPCRoutes(mux, func() []mpc.Party {
		// Offer THIS node as a candidate only once it is actually serving.
		// a.mpc is nil until registerMPCRoutes succeeds, and it stays nil for
		// any node that has not configured MPC — so an empty address keeps a
		// non-participant out of its own candidate list.
		self := ""
		if a.mpc != nil {
			self = a.blockchain.SelfSigningAddress()
		}
		return GlobalPeerRegistry.MPCCandidates(os.Getenv("SELF_URL"), self)
	})

	mux.HandleFunc("/mpc/enroll", a.handleMPCEnroll)
	mux.HandleFunc("/mpc/check", a.handleMPCCheck)
	mux.HandleFunc("/mpc/budget", a.handleMPCBudget)
	// Party 0 allocates the triple range for a session and both parties use
	// it; see mpc_triple_sync.go for why per-party counters could not work.
	mux.HandleFunc(mpcTripleRangePath, a.handleMPCTripleRange)

	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/events", a.handleBlockEvents)
	mux.HandleFunc("/api/health/combined", a.handleCombinedHealth)
	mux.HandleFunc("/api/debug/stateroot-components", a.handleStateRootComponents)
	mux.HandleFunc("/api/debug/dag-gates", a.handleDAGGates)
	mux.HandleFunc("/api/blocks", countEndpoint(&statBlocks, a.handleBlocks))
	mux.HandleFunc("/api/blocks/canonical", countEndpoint(&statCanonical, a.handleCanonicalBlocks))
	mux.HandleFunc("/api/validator-labels", a.handleValidatorLabels)
	mux.HandleFunc("/api/block", a.handleBlockByHash)
	mux.HandleFunc("/api/blocks/by-hash", countEndpoint(&statBlocksByHash, a.handleBlocksByHash))
	mux.HandleFunc("/api/blocks/push", a.handleBlockPush)
	mux.HandleFunc("/api/txbatch", a.handleTxBatch)
	mux.HandleFunc("/api/humanity/credential", a.handleHumanityCredential)
	mux.HandleFunc("/api/humans", a.handleHumans)
	mux.HandleFunc("/api/sepolia/humans", a.handleSepoliaHumans)
	mux.HandleFunc("/api/register", a.handleRegister)
	mux.HandleFunc("/api/balance", a.handleBalance)
	mux.HandleFunc("/api/check-registration", a.handleCheckRegistration)
	mux.HandleFunc("/api/check-registration-by-biohash", a.handleCheckRegistrationByBioHash)
	mux.HandleFunc("/api/check-nullifier", a.handleCheckNullifier)
	mux.HandleFunc("/api/swap", a.handleSwap)
	mux.HandleFunc("/api/add-liquidity", a.handleAddLiquidity)
	mux.HandleFunc("/api/remove-liquidity", a.handleRemoveLiquidity)
	mux.HandleFunc("/api/lp-position", a.handleLPPosition)
	mux.HandleFunc("/api/faucet", a.handleFaucet)
	mux.HandleFunc("/api/pool", a.handlePoolStatus)
	// Rein lesend, aber geschuetzt: er haengt einen Verdacht an
	// identifizierbare Konten. Siehe handleSybilReport.
	mux.HandleFunc("/api/sybil-report", a.handleSybilReport)
	// Vernichtet Geld. Dreifach verriegelt -- siehe handlePoolCorrection.
	mux.HandleFunc("/api/admin/pool-correction", a.handlePoolCorrection)
	mux.HandleFunc("/api/snapshot", a.handleSnapshot)
	mux.HandleFunc("/api/gini/history", a.handleGiniHistory)
	mux.HandleFunc("/api/price-history", a.handlePriceHistory)
	mux.HandleFunc("/api/wealth-cap", a.handleWealthCap)
	mux.HandleFunc("/api/sign-validator-challenge", a.handleSignValidatorChallenge)
	mux.HandleFunc("/api/nonce", a.handleNonce)
	mux.HandleFunc("/api/peers", a.handlePeers)
	// Peer roles and build commits, fetched BY THIS NODE. The page cannot do
	// it itself: its own CSP refuses cross-origin peer reads, and after the
	// domain switch they would be mixed content too. See handlePeerStatuses.
	mux.HandleFunc("/api/peers/status", a.handlePeerStatuses)
	mux.HandleFunc("/api/signing-address", a.handleSigningAddress)
	mux.HandleFunc("/api/admin/registration-debug", a.handleRegistrationDebug)
	mux.HandleFunc("/api/admin/registration-recovery", a.handleRegistrationRecovery)
	mux.HandleFunc("/api/admin/registration-recovery/retry", a.handleRegistrationRecovery)
	mux.HandleFunc("/api/prove", a.handleProveProxy)
	mux.HandleFunc("/api/prove/get/", a.handleProveGetProxy)
	mux.HandleFunc("/api/prove/store", a.handleProveStoreProxy)
	mux.HandleFunc("/api/proof/check", a.handleProofCheckProxy)
	mux.HandleFunc("/api/validators", a.handleValidatorList)
	mux.HandleFunc("/api/peers/challenge", a.handlePeerChallenge)
	mux.HandleFunc("/api/peers/register", a.handlePeerRegister)
	mux.HandleFunc("/node-binding", a.handleNodeBinding)
	mux.HandleFunc("/coordinator-binding", a.handleCoordinatorBinding)
	mux.HandleFunc("/api/register-validator-key", a.handleRegisterValidatorKey)
	// Das Coordinator-Register: derselbe Gedanke wie beim Bezeugungs-
	// schluessel, an der wichtigsten Stelle -- der Coordinator ist der
	// Eingang, an dem ein Mensch ankommt.
	mux.HandleFunc("/api/register-coordinator-key", a.handleRegisterCoordinatorKey)
	mux.HandleFunc("/api/coordinators", a.handleCoordinatorList)
	mux.HandleFunc("/api/set-guardian", a.handleSetGuardian)
	mux.HandleFunc("/api/confirm-alive", a.handleConfirmAlive)
	mux.HandleFunc("/api/guardian", a.handleGetGuardian)
	mux.HandleFunc("/api/escrow", a.handleGetEscrow)
	mux.HandleFunc("/api/recover-escrow", a.handleRecoverEscrow)
	mux.HandleFunc("/registered", a.handleRegistered)
	mux.HandleFunc("/dapp", a.handleDapp)
	mux.HandleFunc("/dapp.js", a.handleDappJS)
	mux.HandleFunc("/download/app.apk", a.handleAppDownload)
	for _, lg := range []string{"en", "de", "es", "fr", "id", "it", "pt", "tr"} {
		lg := lg
		up := strings.ToUpper(lg)
		mux.HandleFunc("/download/node-guide-"+lg+".pdf", func(w http.ResponseWriter, r *http.Request) {
			a.handleStaticDownload(w, r, "downloads/Aequitas_Node_Guide_"+up+".pdf", "Aequitas_Node_Guide_"+up+".pdf", "application/pdf")
		})
	}
	// Use the shared EVMRPCServer (a.evmRPC) so /rpc and /api/register share
	// one nonce map + mutex — creating a second instance here caused separate
	// nonce maps, making the atomic nonce reservation ineffective.
	mux.HandleFunc("/rpc", a.evmRPC.handleRPC)
	return mux
}

func (a *APIServer) Start(port int) {
	mux := a.buildMux()
	fmt.Println("── Starting EVM RPC ─────────────────────")
	if a.evmRPC.evm != nil {
		fmt.Println("✓ EVM Engine ready")
		// Ensure V7 contract is deployed — redeploys from hardcoded bytecode
		// if missing (e.g. after a DB reset). Without this the node fails with
		// "no code at address" on every registration attempt.
		deployerAddr := os.Getenv("RELAYER_ADDRESS")
		if deployerAddr == "" {
			deployerAddr = "0x0BE8b961CBf6564bd1931B0803D35C0659E0D016"
		}
		EnsureContractsDeployed(a.evmRPC.evm, a.state, deployerAddr)
	} else {
		fmt.Println("✗ EVM Engine failed")
	}
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("✓ API Server listening on port %d\n", port)
	// Use http.Server with explicit timeouts to prevent slowloris attacks and
	// goroutine leaks from clients that never send/read — the default mux has none.
	//
	// FIX (2026-07-05 — website audit finding): the explorer/landing HTML
	// response is ~800KB, served uncompressed with no Content-Encoding —
	// confirmed live, no gzip/br/deflate anywhere despite every browser
	// requesting it. Text (HTML/JS/CSS) compresses extremely well (typically
	// 70-80% smaller), so this is pure bandwidth/load-time savings with no
	// freshness tradeoff — unlike caching, which this project deliberately
	// keeps at "no-cache, no-store" (see that header's own site) precisely
	// because stale cached content was a real, repeated problem tonight.
	// Compression doesn't cache anything; every request still gets
	// re-validated, just transferred smaller.
	srv := &http.Server{
		Addr:         addr,
		Handler:      recoverMiddleware(gzipMiddleware(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	SafeGoroutine("http.ListenAndServe", func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[API] Server error: %v\n", err)
		}
	})
	startPprofServer()
}

// startPprofServer (2026-07-24, 50k-TPS merge-latency investigation) exposes
// Go's standard net/http/pprof handlers so a real CPU/goroutine profile can
// be captured from a live, struggling node instead of continuing to guess
// at the dag.mu/GHOSTDAG merge bottleneck from log timestamps alone.
// Deliberately bound to 127.0.0.1 only, on its own mux — NOT the public API
// server's mux, and NOT published via the deploy scripts' `docker run -p`
// flags — so it is reachable only from inside the container (e.g. `docker
// exec ... curl localhost:6061/debug/pprof/...`), never from the public
// internet. pprof's own handlers can trigger genuinely expensive work (a
// 30s CPU profile, a full heap dump) and reveal internal call-stack detail;
// neither belongs on an internet-facing port on a production consensus
// node. A failure to bind (port already in use, sandboxed environment
// without loopback) is logged and otherwise harmless — nothing else in the
// node depends on this listener.
func startPprofServer() {
	// The mutex and block profiles below are served by pprof.Index like any
	// other named profile, but both are governed by a sampling rate that is
	// zero unless something sets it — so without this call they answer with an
	// empty profile rather than an error, which is exactly how a real lock
	// contention problem stayed invisible through several rounds of
	// measurement here. Off by default; see contention_profile.go.
	StartContentionProfiling()

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{
		Addr:         "127.0.0.1:6061",
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // a requested CPU/trace profile can legitimately run up to 60s+
	}
	SafeGoroutine("pprof.ListenAndServe", func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[PPROF] localhost-only debug server error (non-fatal, node continues normally): %v\n", err)
		}
	})
}

func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	latest := a.blockchain.LatestBlock()
	uptime := int64(time.Since(a.startTime).Seconds())
	// Use a.state (PostgreSQL-backed ChainState) as the single source of
	// truth for human count — see NewAPIServer's own comment for why there's
	// no separate in-memory keeper field to accidentally diverge from it.
	// ONE aggregated read under a single read lock, instead of the nine
	// separate lock acquisitions this used to make — four of them the WRITE
	// lock, via GetBalance's lazy account load. See StatusMetrics' comment
	// for the live 11-second /api/status that motivated this, and for the
	// peer-registration timeouts it was causing on the far side.
	m := a.state.StatusMetrics()
	// Measured ledger total, published beside the rule — see supply_measured
	// below. Cached for a minute inside MeasuredTotalAEQ, so this is one cheap
	// read even though /api/status is polled from every open browser tab.
	var supplyMeasured, supplyDifference, supplyMeasuredErr interface{}
	if measured, ok, reason := a.state.MeasuredTotalAEQ(); ok {
		supplyMeasured = fmt.Sprintf("%.6f AEQ", measured)
		supplyDifference = fmt.Sprintf("%+.6f AEQ", measured-m.Supply)
	} else {
		supplyMeasuredErr = reason
	}
	humans := m.Humans
	growth := humans * 10
	if growth > 100 {
		growth = 100
	}
	// Calculate time until next UBI distribution (24h after server start)
	// P3-3: compute next UBI based on last_ubi_at, not server uptime.
	nextUBISecs := a.state.SecondsUntilNextUBI()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"chain_id":     "aequitas-1",
		"version":      "v0.3.0",
		"git_commit":   buildGitCommit,
		"height":       latest.Height,
		"latest_hash":  latest.Hash,
		"total_humans": m.Humans,
		"total_supply": fmt.Sprintf("%.2f AEQ", m.Supply),
		"node_id":      a.p2pNode.GetNodeID(),
		"uptime":       uptime,
		"is_primary":   os.Getenv("IS_PRIMARY_NODE") == "true",
		"block_time":   ConfiguredBlockTimeSeconds(), // read from the real constant (see its own comment) — never hand-typed again
		"contract_v7":  V7_CONTRACT_ADDR,
		// P3-8: V5/V6 legacy addresses removed from status — minimise attack surface.
		"bio_verifier": BIO_VERIFIER_ADDR,
		"chain_evm_id": 1926,
		"index":        m.Index,
		"gini":         m.Gini,
		"growth":       growth,
		"velocity":     50,
		"phase":        m.Phase,
		"fee_bps":      10,
		// FIX (H1, Audit 2026-08-18): total_supply above is the RULE
		// (humans × 1000, see TotalSupply), and the explorer prints it as
		// "Total Supply". Measured from both validators' own databases on
		// 2026-08-15 the ledger actually held 15,305.278004 AEQ against a
		// claimed 15,000 — 2.04% more. The true figure existed only on
		// /api/health/combined, an operations endpoint nobody visiting the
		// site ever sees, so the public number was the one number about this
		// currency that was quietly wrong.
		//
		// Published here beside the rule rather than instead of it: the rule
		// is the protocol statement and stays, the measurement is what is
		// actually in the ledger, and the difference is the thing worth
		// watching. Null when it cannot be measured — never a plausible zero.
		"supply_measured":       supplyMeasured,
		"supply_measured_error": supplyMeasuredErr,
		"supply_difference":     supplyDifference,
		// Pool balances come from the same aggregated read (StatusMetrics) —
		// they used to be four separate GetBalance calls, i.e. four
		// acquisitions of the global state WRITE lock per status request.
		"pool_validators":      fmt.Sprintf("%.4f", m.PoolValidators),
		"pool_lp":              fmt.Sprintf("%.4f", m.PoolLP),
		"pool_ubi":             fmt.Sprintf("%.4f", m.PoolUBI),
		"pool_treasury":        fmt.Sprintf("%.4f", m.PoolTreasury),
		"ubi_next_payout_secs": nextUBISecs,
		// knightdag_activation_height: the AUTHORITATIVE, backend-configured
		// height (KNIGHTDAG_ACTIVATION_HEIGHT env var, or the default — see
		// knightdagActivationHeight's own comment in block.go). The explorer
		// used to hardcode its own copy of this number; if an operator ever
		// set the env var to something else, the two silently disagreed —
		// diamonds could appear on blocks the header text still called
		// "not yet active". The frontend now only uses this value for the
		// "activates at #X" status text; every per-block decision (which
		// blocks get the KnightDAG diamond) reads block.k_eff directly,
		// which is never guessed.
		"knightdag_activation_height": knightdagActivationHeight,
		// latency: the real, measured end-to-end block-propagation numbers
		// this node has observed (see LatencyTelemetry's own comment) — the
		// actual figures BLOCK_TIME/circuit-breaker/finality-slack tuning
		// should fit inside, surfaced for operators instead of living only
		// in log lines. Empty/zero until the first foreignAttachLatencyLogInterval
		// window closes after startup.
		"latency": a.blockchain.GetLatencyTelemetry(),
	})
}

// sseConnections bounds concurrent /api/events streams — a long-lived
// connection is a different resource-exhaustion shape than a normal
// request-response endpoint (each one holds a goroutine + a subscriber
// channel for its whole lifetime), so it gets its own cap alongside the
// project's other DoS shields rather than relying on the general request
// rate limiters, which are sized for short-lived calls.
var sseConnections atomic.Int64

const maxSSEConnections = 500

// handleBlockEvents is a Server-Sent Events stream that pushes one "block"
// event whenever a new block becomes visible on this node (self-produced or
// peer-accepted — see notifyNewBlock's two call sites in block.go). Purely a
// wake-up signal: the event payload is empty, clients react by re-fetching
// through the existing REST endpoints (loadStatus/loadBlocks in explorer.js)
// they already poll on a timer as a fallback. No auth, no per-connection
// state beyond the subscriber channel — same trust level as /api/status
// (public, read-only, carries nothing sensitive).
func (a *APIServer) handleBlockEvents(w http.ResponseWriter, r *http.Request) {
	if sseConnections.Load() >= maxSSEConnections {
		http.Error(w, "too many concurrent event streams", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	// The server-wide WriteTimeout (60s — see Start()) exists to bound an
	// ordinary request-response handler against a slow-loris client; applied
	// to a connection meant to stay open for as long as the browser tab is,
	// it would forcibly kill every SSE stream a minute after it opens,
	// regardless of how often this handler actually writes to it (Go sets
	// the deadline once, absolute, not a sliding per-write window). Clear it
	// for this response only — http.ResponseController is the documented
	// mechanism for exactly this "one long-lived handler on an otherwise
	// short-request server" case (Go 1.20+).
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsubscribe := a.blockchain.SubscribeNewBlocks()
	defer unsubscribe()
	sseConnections.Add(1)
	defer sseConnections.Add(-1)

	// Periodic keepalive comment (SSE-legal: a line starting with ':' is a
	// no-op the client's EventSource silently ignores) — without traffic on
	// an otherwise-idle connection, most reverse proxies/load balancers
	// close it after 30-60s of true silence, which a quiet chain (no new
	// blocks) would hit constantly.
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: block\ndata: {}\n\n")
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (a *APIServer) handleBlocks(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	limit := 50
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	if limit < 1 || limit > 500 {
		limit = 50
	}

	// FIX: ?min_height=N returns the first `limit` blocks with Height > N,
	// regardless of their position in the underlying array. The old
	// ?offset=M&limit= pagination indexed into the LOCAL GetBlocks() array
	// by raw position — which silently broke once multiple validators
	// produce concurrently (the normal, expected BlockDAG case): two nodes
	// accumulate a DIFFERENT number of same-height sibling entries (each
	// node merges at a different pace), so "how many blocks do I have" is
	// no longer a meaningful position into "how many blocks does the peer
	// have at the height I actually need next." A syncing node calling
	// ?offset=dag.TotalBlocks() ended up requesting a position that didn't
	// correspond to its actual sync frontier at all — confirmed in
	// production: a node stuck ~640 blocks behind kept re-fetching pages
	// that were "already known" (0 new) forever, never advancing, while
	// continuing to grow its own isolated, never-reconciled side chain.
	// Height is the one frontier marker that stays meaningful across
	// however many duplicate-height siblings either side has accumulated.
	if minHeightStr := r.URL.Query().Get("min_height"); minHeightStr != "" {
		var minHeight int64
		// FIX: an unparseable min_height silently became 0 (fmt.Sscanf leaves
		// the destination at its zero value on error, and the error itself
		// was discarded) — that returns the ENTIRE chain instead of failing
		// loudly, which is exactly the wrong default for a malformed request.
		if _, err := fmt.Sscanf(minHeightStr, "%d", &minHeight); err != nil {
			http.Error(w, `{"error":"invalid min_height parameter"}`, http.StatusBadRequest)
			return
		}
		// P1-02: support cursor-based pagination via ?after_hash=HASH so the
		// client can request "blocks at height >= minHeight that come after
		// this hash in canonical order".  Without this, a page of N blocks all
		// at the same height advances minHeight to H and the next request
		// (?min_height=H) uses Height > H — permanently skipping any remaining
		// siblings at height H that didn't fit in the first page.
		//
		// GetBlocksSince (not a plain in-memory scan): see its own comment —
		// dag.blocks is pruned to a rolling window, so this now falls back to
		// the DB whenever minHeight is below this node's current prune
		// cutoff, instead of silently serving the wrong (current-tip) window
		// to a peer catching up from far behind.
		afterHash := r.URL.Query().Get("after_hash")
		result := a.blockchain.GetBlocksSince(minHeight, afterHash, limit)
		// A peer that asks for stripped blocks gets headers carrying a TxRoot
		// and fetches the bodies separately from /api/txbatch. Opt-in by the
		// REQUESTER, which is what makes it safe against older peers: they
		// never send the parameter, so they never get a stripped response.
		// See stripBlocksForPeer for what this endpoint measured without it.
		if r.URL.Query().Get("stripped") == "1" {
			result = a.stripBlocksForPeer(result)
		}
		// FIX (2026-07-25, "es merged nix" incident): this used to be
		// json.NewEncoder(w).Encode(result), which streams directly to the
		// response and DISCARDS its error. Encoder.Encode marshals into an
		// internal buffer before writing, so a marshal failure partway
		// through a large slice (one unmarshalable field on just ONE block
		// in range) never reached the client as a clean error — but ANY
		// write-side interruption while flushing that buffer (peer reset,
		// server WriteTimeout under load) does produce a genuinely
		// truncated body with zero server-side trace, surfacing only as a
		// permanently-reproducible "unexpected end of JSON input" on every
		// syncing peer that ever requests a range spanning that height —
		// confirmed live: both secondaries stuck retrying the exact same
		// min_height forever, unable to advance, right after the primary's
		// own restart. Marshaling into a buffer FIRST and writing it in one
		// shot means: a marshal error is caught and logged here (was
		// silent before) instead of ever reaching the client, and the
		// actual write is a single call the runtime can complete or fail
		// atomically rather than a JSON encoder streaming piecemeal into a
		// slow/interrupted connection.
		body, err := json.Marshal(result)
		if err != nil {
			fmt.Printf("[API] ✗ /api/blocks marshal error for min_height=%d limit=%d (%d blocks): %v\n", minHeight, limit, len(result), err)
			jsonError(w, "internal error building response", http.StatusInternalServerError)
			return
		}
		w.Write(body)
		return
	}

	blocks := a.blockchain.GetBlocks()

	// Legacy ?limit=N&offset=M array-position paging — kept for the
	// explorer UI's "browse history" feature, which doesn't need sync
	// correctness, only a stable page of whatever currently exists.
	offset := 0
	fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
	if offset < 0 {
		offset = 0
	}
	// Default: newest blocks (offset from end)
	if r.URL.Query().Get("offset") == "" {
		offset = len(blocks) - limit
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + limit
	if end > len(blocks) {
		end = len(blocks)
	}
	if offset >= len(blocks) {
		offset = len(blocks)
	}
	json.NewEncoder(w).Encode(blocks[offset:end])
}

// handleCanonicalBlocks serves GET /api/blocks/canonical?limit=N — one block
// per height, walking back from the current tip using the exact same
// SelectedParent-chain logic (GetBlockByHeight/canonicalBlockAtHeightLocked)
// the divergence self-check trusts for cross-node hash comparison.
//
// FIX (durable fix, 2026-07-04 — "the explorer must show the same thing on
// every node"): /api/blocks (handleBlocks above) returns the raw in-memory
// window, including every DAG sibling at a height, and left the explorer's
// own JS to guess a "winner" per height by comparing blue_score client-side.
// That guess could differ node-to-node for reasons that have nothing to do
// with real chain divergence — e.g. two nodes simply holding a different
// subset of siblings in their recent in-memory window at request time — so
// the explorer could show a different proposer/score at the same height
// even on two nodes whose actual canonical chain already agreed (confirmed
// live: Contabo 1 and the primary showed different proposers and a
// ~23,000-point blue_score gap at neighbouring heights on 2026-07-04).
// Serving the authoritative, already-fixed backend computation directly
// removes the guess entirely — the explorer now can only ever show exactly
// what GetBlockByHeight (and therefore the autoheal divergence check) says
// the canonical chain is.
func (a *APIServer) handleCanonicalBlocks(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	limit := 30
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	if limit < 1 || limit > 200 {
		limit = 30
	}
	top := a.blockchain.Height()
	blocks := make([]*Block, 0, limit)
	for h := top; h > top-int64(limit) && h >= 0; h-- {
		if b := a.blockchain.GetBlockByHeight(h); b != nil {
			blocks = append(blocks, b)
		}
	}
	json.NewEncoder(w).Encode(blocks)
}

// handleBlockByHash serves GET /api/block?hash=0x... or /api/block?height=N
// — a single block by exact hash or height, or 404. The hash lookup is used
// by fetchMissingAncestors (sync_blocks.go) to resolve a specific
// missing-parent hash directly: /api/blocks' min_height pagination only
// ever looks near the calling node's OWN current height, so once a node's
// chain has drifted from a peer's by more than the sync overlap window, the
// actual common-ancestor blocks it needs to bridge the gap fall permanently
// outside that window and can never be fetched by height alone. The height
// lookup backs the explorer's search box, which previously only searched
// whatever ~50 most recent blocks happened to be cached client-side —
// searching for any older block silently found nothing.
func (a *APIServer) handleBlockByHash(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	hash := r.URL.Query().Get("hash")
	var block *Block
	if hash != "" {
		block = a.blockchain.GetBlockByHash(hash)
		// FIX (2026-06-30, confirmed live in production): never hand a
		// synthetic-checkpoint stub to a peer — see GetBlocks' identical
		// fix/comment. A peer requesting this exact hash genuinely needs to
		// know "no node has the real block", not receive a placeholder that
		// can never pass its own hash-mismatch check.
		if block != nil && block.Proposer == "synthetic-checkpoint" {
			block = nil
		}
	} else if heightStr := r.URL.Query().Get("height"); heightStr != "" {
		var height int64
		if _, err := fmt.Sscanf(heightStr, "%d", &height); err != nil {
			http.Error(w, `{"error":"invalid height parameter"}`, http.StatusBadRequest)
			return
		}
		block = a.blockchain.GetBlockByHeight(height)
	} else {
		http.Error(w, `{"error":"missing hash or height parameter"}`, http.StatusBadRequest)
		return
	}
	if block == nil {
		http.Error(w, `{"error":"block not found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(block)
}

// maxBlocksByHashPerRequest caps how many hashes a single
// handleBlocksByHash request can ask for. A worst-case response (blocks
// with full ZK proof payloads, ~2 KB each) at 500 hashes is ~1 MB — still
// comfortably under both the 20 MB client read cap (fetchBlocksByHashes,
// sync_blocks.go) and the 256 KB request-body cap on this handler covers
// the REQUEST side fine even at 500 (500 hashes x ~70 bytes each = ~35 KB).
// Raised from 50 (scale audit): at a 100-validator target, a burst after a
// partition heals or a node catching up from far behind can queue orphan
// backlogs in the thousands; resolving them 50-at-a-time means many more
// round trips than necessary when the response-size headroom clearly
// supports a much larger batch. sync_blocks.go sets maxBatchSize = this
// constant, so client and server stay in sync automatically.
const maxBlocksByHashPerRequest = 500

// handleBlocksByHash serves POST /api/blocks/by-hash with body {"hashes":[...]}.
//
// FIX (2026-06-28, "all nodes must converge to an identical block set" —
// real fix, not a timeout tweak): fetchMissingAncestors used to resolve one
// missing-parent hash per HTTP round trip via /api/block?hash=. During a
// large catch-up (a node thousands of blocks behind, with 2-3 validators
// still producing every ~6s), the number of distinct missing-parent hashes
// queued at once routinely reached the hundreds — resolving them one
// request at a time, even at a few hundred ms each, could not keep pace
// with new orphans arriving, so orphanAbandonAfter's 3-minute timeout
// started firing on ancestor blocks that genuinely still existed on this
// very peer, just not reached yet. This endpoint answers a whole batch of
// hashes in one round trip, so fetchMissingAncestors (sync_blocks.go) can
// resolve hundreds of pending ancestors in a single request instead of
// hundreds of sequential ones — the actual bottleneck, not the timeout
// value, which is why raising the timeout alone would not have fixed this.
func (a *APIServer) handleBlocksByHash(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		pushError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Hashes []string `json:"hashes"`
	}
	body, tooLarge, err := readBodyLimited(w, r, 256<<10)
	if tooLarge {
		pushError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if err != nil || json.Unmarshal(body, &req) != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if len(req.Hashes) > maxBlocksByHashPerRequest {
		req.Hashes = req.Hashes[:maxBlocksByHashPerRequest]
	}
	// Validate hashes first (reject non-hex / wrong-length before any lookup),
	// then resolve the whole batch under a SINGLE RLock via
	// GetBlocksByHashesForPeer — see its comment for why per-hash locking here
	// timed out peers mid-catch-up. Synthetic-checkpoint stubs are omitted by
	// that method (never served to a peer — see GetBlocks' fix/comment).
	valid := make([]string, 0, len(req.Hashes))
	for _, h := range req.Hashes {
		if len(h) != 64 {
			continue
		}
		if _, hexErr := hex.DecodeString(h); hexErr != nil {
			continue
		}
		valid = append(valid, h)
	}
	found := a.blockchain.GetBlocksByHashesForPeer(valid)

	// Cap the RESPONSE BY BYTES, not only by hash count.
	//
	// maxBlocksByHashPerRequest is 500 because this was sized against blocks
	// of "~2 KB each ... at 500 hashes is ~1 MB". A block carrying a few
	// thousand transfers is closer to 1 MB by itself, so under real load the
	// same 500 hashes produce hundreds of MB, the client stops reading at its
	// 20 MB cap, and the truncated body fails to parse. The client now halves
	// and retries, but every failed attempt still transfers ~20 MB first --
	// on 2026-08-21 Contabo1 spent its whole bandwidth on discarded responses
	// (327 -> 163 -> 81 -> 40 -> 21 -> 10) and could not catch up at all.
	//
	// Serving what fits is strictly better: the caller re-asks for whatever is
	// still missing on its next cycle, so progress is the same and nothing is
	// transferred twice.
	found, truncated := capBlocksByResponseBytes(found)
	if truncated {
		// The client must NOT read the omitted hashes as "this peer does not
		// have them" -- it counts those toward orphanAbandonAfter, and
		// abandoning a block the peer actually holds is how a node ends up
		// permanently unable to bridge to the chain.
		w.Header().Set("X-Blocks-Truncated", "1")
	}
	json.NewEncoder(w).Encode(found)
}

// blocksByHashResponseBudget is the byte budget for one /api/blocks/by-hash
// response. Below the client's 20 MB read cap with room for JSON overhead, so
// a response that fits this budget always fits the client.
const blocksByHashResponseBudget = 12 << 20

// capBlocksByResponseBytes keeps blocks while they fit the budget and reports
// whether anything was left out.
//
// Always keeps the FIRST block even when it alone exceeds the budget: an empty
// response would tell the caller the peer has nothing, and it would make no
// progress on that hash ever again. One oversized block is the client's
// problem to report, not a reason to stall the whole catch-up.
func capBlocksByResponseBytes(blocks []*Block) ([]*Block, bool) {
	total := 2 // the enclosing [ ]
	for i, b := range blocks {
		enc, err := json.Marshal(b)
		if err != nil {
			continue
		}
		total += len(enc) + 1 // + comma
		if total > blocksByHashResponseBudget && i > 0 {
			return blocks[:i], true
		}
	}
	return blocks, false
}

// --- /api/blocks/push flood shield (P0, 2026-07-02 fork-flood recurrence) ---
//
// A peer diverged onto its own fork floods POST /api/blocks/push with blocks
// that can never attach here: a far-ahead runaway tip (rejected by
// AddPeerBlock's lock-free height shield) and near-tip fork blocks (orphaned on
// a parent that lives only on the peer's own chain). Every such POST still costs
// a body read, a JSON unmarshal, and — for near-tip blocks — AddPeerBlock's hash
// recompute, ECDSA recovery and a dag.mu turn. The 2026-07-02 incident showed
// ONE such peer (the third-party 178.105.186.119 node, which this operator does
// not control and which auto-re-registers after every restart) is enough on its
// own to drag block production from 1s to 4-6s and API reads to 5-27s even with
// the height shield already in place.
//
// This trips a circuit breaker per SOURCE IP after a sustained run of pushes
// that fail to attach, then drops that IP's POSTs BEFORE the body is read for a
// cooldown — the earliest, cheapest possible rejection, so a sustained flood
// costs almost nothing. A peer whose pushes attach normally resets its run on
// every accepted block and never trips; legitimate validators (Contabo, cd20)
// are therefore unaffected. A falsely-tripped peer still merges via the 6s
// ordered-pull sync (doSyncOnce, which never goes through this path), so the
// worst case is a short merge delay, never a stuck block.
const (
	blockPushBreakerThreshold = 50               // consecutive non-attaching pushes from one IP before it trips
	blockPushBreakerCooldown  = 20 * time.Second // how long a tripped IP's POSTs are dropped pre-parse
	// blockPushBreakerReopenProbes mirrors proposerBreakerReopenProbes'
	// exact fix (block.go, 2026-07-04) for the identical single-probe
	// fragility here: confirmed live the same night as that fix, a freshly
	// restarted node's per-IP push breaker against a healthy peer failed to
	// clear for several minutes because the single post-cooldown probe kept
	// losing to ordinary transient noise. Widened to 5 for the same reason:
	// a genuinely diverged/flooding IP still fails all 5 and re-trips
	// within a handful of pushes, not a full fresh 50-push run.
	blockPushBreakerReopenProbes = 5
	// maxTrackedPushIPs caps blockPushBreaker's tracked-key count (performance
	// audit 2026-07-06): the key here is the caller's source IP, read before
	// any authentication — an unauthenticated caller can trivially generate
	// traffic from many source IPs (or spoof the header this reads it from,
	// depending on deployment), so this map needs the exact same bound
	// maxTrackedProposers already gives the proposer breaker (block.go),
	// which this consolidation onto boundedBreaker (breaker.go) now provides
	// automatically — previously this map had NO cap at all.
	maxTrackedPushIPs = 500
)

var (
	// blockPushBreaker is a boundedBreaker (breaker.go) keyed by source IP —
	// see the block comment above for the full rationale.
	blockPushBreaker     = newBoundedBreaker(blockPushBreakerThreshold, blockPushBreakerCooldown, blockPushBreakerReopenProbes, maxTrackedPushIPs)
	lastBlockPushDropLog atomic.Int64 // rate-limits the drop log to once/sec

	// blockPushIPDenylist is an operator hard-block, parsed once from
	// PEER_PUSH_DENYLIST (comma-separated IPs). Unlike the automatic breaker it
	// never expires — set it to permanently refuse a peer that has repeatedly
	// attacked (e.g. PEER_PUSH_DENYLIST=178.105.186.119). Checked before the body
	// is read.
	blockPushIPDenylist = func() map[string]bool {
		m := map[string]bool{}
		for _, ip := range strings.Split(os.Getenv("PEER_PUSH_DENYLIST"), ",") {
			if ip = strings.TrimSpace(ip); ip != "" {
				m[ip] = true
			}
		}
		return m
	}()
)

// blockPushShouldDrop reports whether an inbound push from ip must be dropped
// now without reading its body — either a permanent denylist entry or an open
// circuit breaker still inside its cooldown. Touches only blockPushBreaker's
// own dedicated mutex (boundedBreaker, breaker.go), never dag.mu.
func blockPushShouldDrop(ip string) bool {
	if blockPushIPDenylist[ip] {
		return true
	}
	return blockPushBreaker.ShouldDrop(ip)
}

// blockPushRecordOutcome feeds the per-IP breaker after AddPeerBlock returns.
// attached (the block joined the DAG) clears the IP's failure run; !attached
// (orphaned, rejected, or a re-push of a known block) advances it and trips the
// breaker once the run crosses blockPushBreakerThreshold.
func blockPushRecordOutcome(ip string, attached bool) {
	blockPushBreaker.RecordOutcome(ip, attached)
}

// handleBlockPush accepts a freshly-produced block from a peer via HTTP POST
// and feeds it directly into AddPeerBlock. This is the HTTP-level push path
// that enables DAG merges even when libp2p (port 4001) is firewalled — e.g.
// on Railway where only one TCP port per service is exposed. After each
// ProduceBlock() call, the producing node POSTs its block to all known peers'
// /api/blocks/push endpoints in parallel goroutines; the receiving node inserts
// it into dag.tips immediately, so the next ProduceBlock() includes it as a
// parent and creates a genuine multi-parent merge block.
func (a *APIServer) handleBlockPush(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	// Fork-flood shield: drop a flooding or denylisted peer's push BEFORE reading
	// or parsing the body — the cheapest possible rejection. See the
	// blockPushBreaker* block above for why one diverged peer needs this.
	ip := clientIP(r)
	if blockPushShouldDrop(ip) {
		nowNano := time.Now().UnixNano()
		last := lastBlockPushDropLog.Load()
		if nowNano-last > int64(time.Second) && lastBlockPushDropLog.CompareAndSwap(last, nowNano) {
			fmt.Printf("[BLOCK-PUSH] ✗ Dropping pushes from %s — flood shield open (denylist or sustained non-attaching flood). (rate-limited)\n", ip)
		}
		w.WriteHeader(http.StatusTooManyRequests)
		// action:"resync_required" (P0 fix, 2026-07-02 liveness audit follow-up):
		// an unambiguous signal — this IP has been sustainedly failing to attach —
		// so the sender can learn about its own divergence instead of pushing
		// into the void forever. See HTTPBroadcastBlock (sync_blocks.go) for the
		// sender-side reaction, safety-gated behind AUTO_HEAL_ON_DIVERGENCE.
		w.Write([]byte(`{"ok":false,"reason":"push flood shield open","action":"resync_required","tx_batch":"` + txBatchCapabilityToken + `"}`))
		return
	}
	// maxBlockStreamBytes (p2p.go), not a separate literal: this is the same
	// class of payload as the libp2p block-gossip path (one full block's
	// JSON), just over HTTP instead of a libp2p stream — this endpoint's own
	// doc comment above says it's the primary relay mechanism whenever port
	// 4001 is firewalled, so it needs the same headroom for maxTxsPerBlock
	// (evm_storage.go) or it silently caps effective block-relay throughput
	// right back down to ~2,200 transactions regardless of that constant.
	// Unlike the libp2p path's old bug, this one fails LOUDLY (413, via
	// tooLarge below) rather than silently truncating — still a real
	// functional ceiling, just a visible one instead of a silent one.
	body, tooLarge, err := readBodyLimited(w, r, maxBlockStreamBytes)
	if tooLarge {
		jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		pushError(w, http.StatusBadRequest, "read error")
		return
	}
	var block Block
	if err := json.Unmarshal(body, &block); err != nil {
		pushError(w, http.StatusBadRequest, "invalid block JSON")
		return
	}
	// FIX (P0, 2026-07-04 brutal audit): this endpoint is publicly reachable —
	// unlike a trusted seed's URL, which an operator explicitly configures via
	// PRIMARY_NODE_URL/PEER_NODES, ANY HTTP client can POST here. FromSync is
	// not just a label: block.go's isFinalityViolation, the authorized-
	// validator check, IsValidatorSuspended, and the lock-free proposer
	// circuit breaker ALL skip their check entirely when it's true — see
	// FromSync's own field comment. Setting it unconditionally on every
	// push meant an unauthenticated sender could get an unregistered
	// proposer's block accepted, a suspended validator's block un-suspended,
	// or a genuinely diverged/flooding proposer exempted from the very
	// breaker built to stop it. A legitimate peer's genuinely-authorized,
	// correctly-signed block still passes every one of these gates normally
	// (that's the whole point of them being real checks); removing this
	// bypass does not weaken the real-time push-merge path this endpoint
	// exists for, only the false trust extended to an unauthenticated one.
	// FIX (2026-07-05 — see hasBlockInMemory's own comment, block.go): a
	// live block routinely arrives here more than once (this endpoint is
	// pushed to independently by every producing peer, alongside P2P
	// direct+relay) — report the idempotent success immediately for a
	// block this node already has, instead of paying a full AddPeerBlock
	// call just to re-discover "already known" every time.
	if a.blockchain.hasBlockInMemory(block.Hash) {
		w.Write([]byte(`{"ok":true,"tx_batch":"` + txBatchCapabilityToken + `"}`))
		return
	}
	// Roadmap step 4 (tx_batch_transport.go): the sender may have stripped the
	// transactions and sent the header alone, which is what keeps a large block
	// inside HTTPBroadcastBlock's 3 s push timeout. Fetch the body and attach it
	// BEFORE AddPeerBlock, so every gate below — above all the hash check that
	// binds a block to its transactions — sees a complete block, exactly as if
	// it had arrived inline.
	//
	// A block whose body cannot be obtained is REJECTED, never accepted empty:
	// replaying it without its transactions would silently drop every transfer
	// it contains. action:"resend_full" makes the sender re-push the complete
	// block at once, so a failed fetch costs one extra round trip and nothing
	// else.
	if !a.ensureBlockBody(&block, r.Header.Get(txBatchSourceHeader)) {
		w.Write([]byte(`{"ok":false,"reason":"transaction body unavailable","action":"resend_full","tx_batch":"` + txBatchCapabilityToken + `"}`))
		return
	}
	block.FromSync = false
	accepted := a.blockchain.AddPeerBlock(&block)
	// FIX (durable fix, 2026-07-04 — closes the same mutual-lockout risk for
	// this breaker too, see proposerBreakerOrphanGrace's own comment in
	// block.go): a fresh, still-within-grace orphan must not count against
	// this per-IP breaker either, or two fully healthy nodes can still trip
	// this one against each other during ordinary propagation lag even after
	// the per-proposer breaker was fixed to tolerate it.
	if !accepted && a.blockchain.IsWithinOrphanGrace(&block) {
		w.Write([]byte(`{"ok":false,"reason":"orphaned, within grace period","tx_batch":"` + txBatchCapabilityToken + `"}`))
		return
	}
	blockPushRecordOutcome(ip, accepted)
	if accepted {
		fmt.Printf("[BLOCK-PUSH] ✓ Accepted block #%d via HTTP push\n", block.Height)
		w.Write([]byte(`{"ok":true,"tx_batch":"` + txBatchCapabilityToken + `"}`))
	} else {
		// Not an error — block may already be known (idempotent). Only signal
		// resync_required when the rejection is unambiguous (this proposer's
		// own breaker is open) — an ordinary duplicate (arrived via P2P first)
		// must never trigger it.
		if a.blockchain.proposerBlockBlocked(block.Proposer) {
			w.Write([]byte(`{"ok":false,"reason":"proposer circuit breaker open","action":"resync_required","tx_batch":"` + txBatchCapabilityToken + `"}`))
		} else {
			w.Write([]byte(`{"ok":false,"reason":"rejected or already known","tx_batch":"` + txBatchCapabilityToken + `"}`))
		}
	}
}

func (a *APIServer) handleHumans(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	// FIX (P2-8, beta-launch audit 2026-07-05): this was an O(N) full-account-
	// scan endpoint with no rate limit at all, unlike comparably-expensive
	// endpoints (handlePriceHistory, handleConfirmAlive). The explorer's own
	// loadHumans() polls this every 10s, so 3s is generous enough not to
	// interfere with normal multi-tab dashboard use while still capping
	// repeated-scan abuse. Prefixed rate-limit key, same reasoning as every
	// other endpoint sharing registerRateLimit — see /api/recover-escrow's
	// own fix for why an unprefixed key would couple unrelated endpoints'
	// cooldowns together.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("humans:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 3*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store("humans:"+ip, time.Now())
	// FIX (P2-d, audit 2026-07-06): limit/offset pagination over the
	// registered-human subset. Default limit (500) preserves today's exact
	// response (well under that at current scale) for every existing caller
	// that doesn't pass these params — this endpoint's cost that actually
	// matters at scale is unbounded JSON response size/serialization, which
	// this bounds; GetAllAccounts() itself still does one full in-memory map
	// pass (ChainState.accounts has no secondary index to page through
	// instead), same as before.
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	accounts := a.state.GetAllAccounts()
	// GetAllAccounts() iterates a Go map — unordered, and not even stable
	// from one call to the next. offset/limit pagination over that directly
	// would be unreliable (the same offset could return different accounts
	// on different requests, or skip/duplicate entries across pages).
	// Sorting by address first gives every page a fixed, reproducible
	// position in the same total order.
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Address < accounts[j].Address })
	// Read pool state once, not per-account, so this scales to millions of
	// humans. Each human's LP value is their ownership fraction of the pool
	// reserves — surfaced alongside the liquid balance so a human who added
	// all their AEQ as liquidity doesn't appear to hold nothing.
	reserveAEQ, reserveTUSD, totalLPShares := a.state.GetPoolSnapshot()
	totalHumans := 0
	humans := []map[string]interface{}{}
	skipped := 0
	for _, acc := range accounts {
		if acc.IsHuman {
			totalHumans++
			if skipped < offset {
				skipped++
				continue
			}
			if len(humans) >= limit {
				continue
			}
			// acc.Balance is ALREADY demurrage-adjusted: GetAllAccounts hands
			// back a copy whose Balance was set to effectiveBalance(acc). Calling
			// effectiveBalance again here re-applied the decay factor a second
			// time (factor², over the same idle period), so every account past
			// the grace period was published lighter than it really is — and the
			// Lorenz curve drawn from these numbers disagreed with the Gini in
			// /api/status, which computes from the real wealth. Measured live on
			// 2026-08-15: list 0.12361229 vs chain 0.13244174, stable to the last
			// digit across repeated calls. Exactly the divergence the comment on
			// humanAEQWealthLocked warns about, arriving through the other side.
			liquid := acc.Balance.Float()
			lpShares := acc.LPShares.Float()
			var lpValueAEQ, lpValueTUSD float64
			if totalLPShares > 0 && lpShares > 0 {
				ownership := lpShares / totalLPShares
				lpValueAEQ = ownership * reserveAEQ
				lpValueTUSD = ownership * reserveTUSD
			}
			humans = append(humans, map[string]interface{}{
				"address": acc.Address,
				// Demurrage-adjusted (once), so the Lorenz curve the frontend
				// draws from total_value_aeq reproduces CalcGini() exactly.
				"balance":         liquid,
				"lp_shares":       lpShares,
				"lp_value_aeq":    lpValueAEQ,
				"lp_value_tusd":   lpValueTUSD,
				"total_value_aeq": liquid + lpValueAEQ,
			})
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":  totalHumans, // the real total, not just this page's size — the explorer's header count relies on this
		"humans": humans,
		"limit":  limit,
		"offset": offset,
	})
}

func (a *APIServer) handleSepoliaHumans(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	// FIX (P3, beta-launch audit 2026-07-05): unlike its sibling proxies
	// (handleProveProxy, handleProveStoreProxy, handleProofCheckProxy — all
	// rate-limited per-IP), this endpoint had no rate limit at all. Every hit
	// becomes an outbound HTTP GET to the operator's proof server, which may
	// have a much scarcer request budget than this chain node — an
	// unthrottled amplification vector against it.
	ip := "sepolia-humans:" + clientIP(r)
	if ts, loaded := registerRateLimit.Load(ip); loaded {
		if time.Since(ts.(time.Time)) < 5*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store(ip, time.Now())
	if len(proofServerURLs()) == 0 {
		jsonError(w, "no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node", 503)
		return
	}
	resp, err := doProofServerRequestFailover("GET", "/humans", nil, 8*time.Second, nil)
	if err != nil {
		// FIX 11: Don't leak the internal URL or low-level network error to clients.
		jsonError(w, "proof server unavailable", 503)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		jsonError(w, "proof server unavailable", resp.StatusCode)
		return
	}
	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	json.NewEncoder(w).Encode(data)
}

func (a *APIServer) handleBalance(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if wallet == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"balance": 0, "tusd_balance": 0, "is_human": false})
		return
	}

	// Use ChainState (native balance) as the single source of truth.
	// This used to query the V7 contract's balanceOf()/isHuman() directly,
	// which was the right call back when registrations only wrote to EVM
	// storage and ChainState was never updated. Since registration now also
	// grants the native balance via state.RegisterHuman() (and transfers
	// move the native balance via state.Transfer()), ChainState reflects
	// the real, current state — while the contract's own balanceOf() can
	// lag behind it (it's no longer touched by ordinary native transfers
	// at all, and read-only contract calls are intentionally not persisted
	// per-call). Querying the contract here would show a wallet's balance
	// from whenever it last interacted with the contract directly, not its
	// real current native balance.
	balance := a.state.GetBalance(wallet)
	tusdBalance := a.state.GetTUsdBalance(wallet)
	isHuman := a.state.IsHuman(wallet)
	demurrage := a.state.GetDemurrageStatus(wallet)

	// LP position: AEQ deposited into the liquidity pool is no longer part of
	// the wallet's spendable "balance" — it lives as LP shares. Without
	// surfacing it here, a user who added all their AEQ as liquidity sees a
	// bare "balance: 0" and reasonably concludes their funds vanished. Report
	// the LP shares AND their current withdrawable AEQ/tUSD value (the pool's
	// reserves times this wallet's ownership fraction) so the frontend can show
	// "0 AEQ liquid + X AEQ worth of liquidity".
	lpShares, totalLPShares := a.state.GetLPShares(wallet)
	reserveAEQ, reserveTUSD := a.state.GetPoolReserves()
	var lpValueAEQ, lpValueTUSD float64
	if totalLPShares > 0 && lpShares > 0 {
		ownership := lpShares / totalLPShares
		lpValueAEQ = ownership * reserveAEQ
		lpValueTUSD = ownership * reserveTUSD
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet":                     wallet,
		"balance":                    balance,
		"tusd_balance":               tusdBalance,
		"is_human":                   isHuman,
		"lp_shares":                  lpShares,
		"lp_value_aeq":               lpValueAEQ,
		"lp_value_tusd":              lpValueTUSD,
		"total_value_aeq":            balance + lpValueAEQ,
		"demurrage_active":           demurrage.Active,
		"demurrage_days_until_start": demurrage.DaysUntilStart,
		"show_14_day_notice":         demurrage.ShowFourteenDayNotice,
		"show_7_day_notice":          demurrage.ShowSevenDayNotice,
	})
}

// handleCheckRegistration lets the app ask "did MY specific proof commitment
// get registered, and to which wallet?" — instead of reading the last entry
// in a global, unfiltered /api/humans list (which showed every user the
// most recently registered wallet, regardless of who they actually were).
func (a *APIServer) handleCheckRegistration(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)

	commitment := r.URL.Query().Get("commitment")
	if commitment == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"registered": false})
		return
	}

	wallet := a.state.GetWalletByCommitment(commitment)
	if wallet == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"registered": false})
		return
	}

	balance := a.state.GetBalance(wallet)
	isHuman := a.state.IsHuman(wallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"registered": true,
		"wallet":     wallet,
		"balance":    balance,
		"is_human":   isHuman,
	})
}

// handleCheckRegistrationByBioHash mirrors handleCheckRegistration, but
// keyed by the device's biometric identity hash rather than a proof
// commitment. Needed because, under the new website-side proof flow, the
// app only ever knows its own bioHash — it never computes a commitment
// itself anymore (that now happens on the website, after MetaMask
// supplies the real wallet) — so it can't poll by commitment the way the
// old flow did.
func (a *APIServer) handleCheckRegistrationByBioHash(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}

	// POST only — GET is removed because bioHash in the URL lands in
	// server/proxy logs creating unnecessary biometric linkability.
	if r.Method != "POST" && r.Method != "OPTIONS" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	// FIX: this endpoint is unauthenticated by design (anyone checking their
	// own registration status), but it accepts a caller-supplied bioHash
	// with no throttle at all — unlike every other bioHash/escrow-adjacent
	// endpoint in this file. Reuse the same package-level rate limiter as
	// handleRecoverEscrow so it can't be used to mass-probe bioHash values.
	//
	// UPDATE (fresh Monster Audit 2026-07-12, cleanup): a 2026-06-28 version
	// of this comment argued for keeping `wallet` in the response, because
	// the then-shipped AequitasBio app (App.tsx) polled this endpoint by
	// bioHash to recover its own wallet address before any wallet was
	// connected. That app is retired; the current aequitas-app has its own
	// SecureStore-backed wallet from the moment it exists and never asks
	// this endpoint (or any endpoint) what its wallet address is — see
	// BRUTAL-P3-13 below, which did go ahead and strip `wallet` from both
	// response branches. bioHash remains the credential this endpoint
	// trusts (POST-only so it never lands in a URL/logs, plus the rate
	// limit below), but response minimization is no longer in tension with
	// any real client — the two no longer need to be traded off.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("biohash-check:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 5*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store("biohash-check:"+ip, time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var bioHashBody struct {
		BioHash string `json:"bioHash"`
	}
	// FIX (fresh Monster Audit 2026-07-12, P2): a decode error here (malformed
	// JSON, wrong field type) used to be silently discarded, leaving bioHash
	// at its zero value and falling straight into the same "registered:
	// false" response as a genuinely empty bioHash — indistinguishable to
	// the client from "you're not registered yet." Return 400 instead so a
	// broken request body doesn't look like a real registration-status
	// answer.
	if err := json.NewDecoder(r.Body).Decode(&bioHashBody); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}
	var bioHash = bioHashBody.BioHash
	if bioHash == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"registered": false})
		return
	}

	wallet := a.state.GetWalletByBioHash(bioHash)
	if wallet == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"registered": false})
		return
	}

	balance := a.state.GetBalance(wallet)
	isHuman := a.state.IsHuman(wallet)

	// If the bioHash exists in bio_registrations but the wallet is NOT yet
	// marked as human on-chain, it means someone else used this biometric
	// hash to generate a proof but hasn't completed registration yet —
	// OR a different wallet tried to reuse this bioHash. Either way, the
	// current user should NOT see "success". Return a distinct status so
	// the app can show an appropriate message.
	if !isHuman {
		// FIX (BRUTAL-P3-13): omit wallet to avoid bioHash→wallet linkage leak.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"registered":       false,
			"biometric_in_use": true,
		})
		return
	}

	// FIX (BRUTAL-P3-13): removed wallet from response — the app already knows
	// its own wallet; returning it here exposes a bioHash→wallet mapping to any
	// caller who knows the bioHash, undermining the privacy model.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"registered": true,
		"balance":    balance,
		"is_human":   isHuman,
	})
}

// handleCheckNullifier lets the client ask "has this nullifier been used?"
// before submitting a registration. GET /api/check-nullifier?n=<hex>
func (a *APIServer) handleCheckNullifier(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	nullifier := r.URL.Query().Get("n")
	if nullifier == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"used": false})
		return
	}
	wallet := a.state.GetWalletByNullifier(nullifier)
	// Return only used/unused — never the associated wallet address.
	// The wallet linkage is a biometric identifier that should not be
	// publicly enumerable via the nullifier index.
	json.NewEncoder(w).Encode(map[string]interface{}{"used": wallet != ""})
}

// queryV7Status reads isHuman(address) and balanceOf(address) directly from
// the V7 contract via eth_call. Kept available for debugging/comparison
// against the contract's own bookkeeping, but no longer used by the
// balance-facing endpoints above — see handleBalance for why.
func (a *APIServer) queryV7Status(wallet string) (float64, bool) {
	// P2-AUDIT: Use the shared evmRPC instance instead of creating a new one per
	// call. Creating a new EVMRPCServer allocates a new EVM engine (including DB
	// initialization) on every invocation — wasteful and bypasses the shared nonce
	// map, which could cause nonce desync if this path ever submits transactions.
	evmRPC := a.evmRPC
	if evmRPC == nil || evmRPC.evm == nil {
		return 0, false
	}

	to := common.HexToAddress(V7_CONTRACT_ADDR)
	from := common.HexToAddress(wallet)

	// isHuman(address) — selector 0xf72c436f
	// persist=false: this is a read-only status query (used by the explorer
	// frontend's balance/status display), not a real registration. Previously
	// every poll of this endpoint silently wrote isHuman=true to evm_storage
	// as a side effect, which is part of why "already registered" kept
	// reappearing even right after a full database reset.
	isHumanData := append(common.Hex2Bytes("f72c436f"), common.LeftPadBytes(from.Bytes(), 32)...)
	isHumanRet, err := evmRPC.evm.CallContract(from, to, isHumanData, big.NewInt(0), false)
	isHuman := false
	if err == nil && len(isHumanRet) >= 32 {
		isHuman = isHumanRet[31] == 1
	}

	if !isHuman {
		return 0, false
	}

	// balanceOf(address) — selector 0x70a08231
	balanceData := append(common.Hex2Bytes("70a08231"), common.LeftPadBytes(from.Bytes(), 32)...)
	balanceRet, err := evmRPC.evm.CallContract(from, to, balanceData, big.NewInt(0), false)
	balance := 0.0
	if err == nil && len(balanceRet) >= 32 {
		weiInt := new(big.Int).SetBytes(balanceRet)
		balanceFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(weiInt), new(big.Float).SetInt(weiPerAEQ)).Float64()
		balance = balanceFloat
	}

	return balance, isHuman
}

func (a *APIServer) handleRegistered(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// FIX 10: Add Content-Security-Policy to prevent XSS escalation on this HTML page.
	// FIX (Monster Audit 2026-07-12, P2): this page has zero onclick=/onchange=
	// attributes and zero <script> tags at all (pure static markup plus one
	// escaped %s interpolation, see below) — script-src never needed
	// 'unsafe-inline' here.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// XSS fix: escape wallet parameter before writing to HTML — without this,
	// a crafted URL like /registered?wallet=<script>... would execute JS.
	wallet := html.EscapeString(r.URL.Query().Get("wallet"))
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Registered — Aequitas</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0A0E1A;color:#C9A84C;font-family:'Courier New',monospace;display:flex;align-items:center;justify-content:center;min-height:100vh;padding:20px;flex-direction:column;gap:20px;text-align:center}
.logo{font-size:2rem;font-weight:900;letter-spacing:8px;color:#C9A84C}
.box{background:#111827;border:1px solid #1E2D45;border-radius:12px;padding:32px;max-width:440px;width:100%%}
.title{color:#22C55E;font-size:1.4rem;font-weight:bold;margin-bottom:8px}
.wallet{color:#6B7A99;font-size:0.7rem;margin-bottom:20px;word-break:break-all}
.divider{border-top:1px solid #1E2D45;margin:16px 0}
.sub{color:#6B7A99;font-size:0.82rem;line-height:1.9}
.hl{color:#C9A84C;font-weight:bold}
.btn{display:inline-block;margin-top:16px;padding:12px 24px;background:#C9A84C;color:#0A0E1A;border-radius:8px;text-decoration:none;font-weight:bold;font-size:0.8rem;letter-spacing:1px}
</style>
</head>
<body>
<div class="logo">AEQUITAS</div>
<div class="box">
<div class="title">🎉 Registered as Human!</div>
<div class="wallet">%s</div>
<div class="divider"></div>
<div class="sub">
<span class="hl">1,000 AEQ</span> has been credited to your wallet.<br><br>
Return to the <span class="hl">Aequitas App</span> — it will confirm your registration automatically.<br><br>
<span style="color:#4FC3F7">Money exists because people exist.</span>
</div>
<a class="btn" href="/">← VIEW EXPLORER</a>
</div>
</body>
</html>`, wallet)
}

// handleNodeBinding serves a small, self-contained signing tool so a
// node operator can prove ownership of their NODE_OPERATOR_WALLET without
// any code or wallet-connect library — just a browser with MetaMask (or
// any EIP-1193 wallet) installed. The signature it produces is the
// NODE_OPERATOR_BINDING_SIGNATURE value referenced in BindValidatorSlot's
// comment: this page never talks to the chain or sends the signature
// anywhere itself, it only computes it client-side via window.ethereum's
// personal_sign and displays it for the operator to copy into their own
// node's environment variables.
func (a *APIServer) handleNodeBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// FIX (Monster Audit 2026-07-12 follow-up, P1): script-src no longer needs
	// 'unsafe-inline' now that signBinding() lives in the same-origin
	// /node-binding.js file (see nodeBindingJS's comment in api_html.go).
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Validator Binding — Aequitas</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0A0E1A;color:#C9A84C;font-family:'Courier New',monospace;display:flex;align-items:center;justify-content:center;min-height:100vh;padding:20px}
.box{background:#111827;border:1px solid #1E2D45;border-radius:12px;padding:32px;max-width:520px;width:100%}
.logo{font-size:1.6rem;font-weight:900;letter-spacing:6px;color:#C9A84C;margin-bottom:18px;text-align:center}
.sub{color:#6B7A99;font-size:0.78rem;line-height:1.8;margin-bottom:18px}
label{display:block;color:#C9A84C;font-size:0.72rem;margin-bottom:6px;margin-top:14px}
input{width:100%;background:#0A0E1A;border:1px solid #1E2D45;border-radius:6px;color:#fff;padding:10px;font-family:'Courier New',monospace;font-size:0.78rem}
.btn{display:block;width:100%;margin-top:18px;padding:12px;background:#C9A84C;color:#0A0E1A;border:none;border-radius:8px;font-weight:bold;font-size:0.82rem;letter-spacing:1px;cursor:pointer}
.btn:disabled{opacity:0.5;cursor:not-allowed}
.out{margin-top:18px;padding:14px;background:#0A0E1A;border:1px solid #22C55E;border-radius:8px;word-break:break-all;font-size:0.7rem;color:#22C55E;display:none}
.err{margin-top:18px;padding:14px;background:#0A0E1A;border:1px solid #f87171;border-radius:8px;font-size:0.75rem;color:#f87171;display:none}
.hl{color:#C9A84C;font-weight:bold}
</style>
</head>
<body>
<div class="box">
<div class="logo">AEQUITAS</div>
<div class="sub">
This page proves your <span class="hl">NODE_OPERATOR_WALLET</span> owns the signature your node needs to register as a validator. It signs a message locally in your wallet — nothing is sent anywhere by this page.
</div>
<label>Your node's signing address (find it via <code>/api/signing-address</code> on your own node, or in its startup logs)</label>
<input id="signingAddr" placeholder="0x...">
<button class="btn" id="connectBtn">Connect Wallet &amp; Sign</button>
<div class="out" id="out"></div>
<div class="err" id="err"></div>
</div>
<script src="/node-binding.js"></script>
</body>
</html>`)
}

func (a *APIServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	// FIX (Monster Audit 2026-07-12, P1): both ethers and the price-chart lib
	// now load from same-origin /vendor/*.min.js (self-hosted, version-pinned
	// — see vendorEthersJS's comment in api_html.go), so script-src no longer
	// needs to allow cdnjs.cloudflare.com/unpkg.com (previously required, or
	// window.LightweightCharts stayed undefined and initPriceChart() bailed).
	// FIX (Monster Audit 2026-07-12, P2): explorer.html's ~75 onclick=/
	// onchange=/oninput= attributes are gone — every interactive element now
	// carries data-act (+ optional data-args) and is wired up by one
	// delegated listener in explorer.js (see CLICK_ACTIONS there). explorer.html
	// itself has zero inline <script> blocks (only external /vendor + /explorer.js
	// src= tags), so script-src no longer needs 'unsafe-inline' at all.
	// connect-src additions: WalletConnect (see vendorWalletConnectJS) opens a
	// WebSocket to its relay to actually carry the wallet session, and fetches
	// the wallet list + a domain-verification check from two more of its own
	// hosts — replaced the WebAuthn "register via browser" flow, whose
	// device-bound credential never left this origin and needed no CSP change.
	// img-src needs blob:, not just the api.web3modal.org host: the wallet
	// icons in the "All Wallets" picker are fetched as a blob over connect-src
	// (already allowed above) and then rendered via URL.createObjectURL(),
	// i.e. an <img src="blob:..."> — the https://api.web3modal.org origin
	// itself is never used directly as an <img> src, only blob: is.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self' https://aequitas.digital wss://relay.walletconnect.org https://relay.walletconnect.org https://api.web3modal.org https://verify.walletconnect.org https://verify.walletconnect.com; img-src 'self' data: blob: https://api.web3modal.org; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	path := strings.Trim(r.URL.Path, "/")
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	// Backwards-compat: /swap redirects to /exchange.
	if path == "swap" {
		http.Redirect(w, r, "/exchange", http.StatusMovedPermanently)
		return
	}
	// All paths serve the same HTML — client-side JS handles tab activation
	// from window.location.pathname immediately on DOMContentLoaded.
	// This avoids all server-side HTML manipulation and the race conditions
	// it creates between server-injected classes and JS-driven tab switching.
	// explorerHTMLVersioned (not the raw explorerHTML) — see its own comment
	// in api_html.go for why: it points at content-hashed CSS/JS URLs so a
	// browser that cached last deploy's assets fetches this deploy's instead.
	fmt.Fprint(w, explorerHTMLVersioned)
}

// handleExplorerCSS/handleExplorerJS serve the Explorer UI's stylesheet and
// script, split out of the HTML document (and out of api_html.go) — see
// that file's own comment. explorerHTMLVersioned requests these by a
// content-hashed "?v=" query string (api_html.go), so a genuinely long,
// "immutable" cache lifetime is now actually safe — unlike the previous
// max-age=3600 on the bare unversioned path, a content change here changes
// the hash and therefore the URL a browser fetches, rather than relying on
// every browser's cache happening to have expired by the time of the next
// deploy (see explorerCSSVersion's own comment for the incident this fixes).
func (a *APIServer) handleExplorerCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	fmt.Fprint(w, explorerCSS)
}

func (a *APIServer) handleExplorerJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	fmt.Fprint(w, explorerJS)
}

// handleVendorEthersJS / handleVendorLightweightChartsJS serve self-hosted,
// version-pinned copies of the two third-party scripts the register/wallet
// page needs — see vendorEthersJS's own comment for why these replaced
// loading straight from cdnjs.cloudflare.com / unpkg.com. Long cache
// lifetime is safe: the embedded content only changes when this binary is
// rebuilt with a new vendored file, i.e. a real version bump.
func (a *APIServer) handleVendorEthersJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, vendorEthersJS)
}

func (a *APIServer) handleVendorLightweightChartsJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, vendorLightweightChartsJS)
}

func (a *APIServer) handleVendorWalletConnectJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, vendorWalletConnectJS)
}

// handleNonce returns the next swap nonce a wallet should sign with.
// GET /api/nonce?wallet=0x...
func (a *APIServer) handleNonce(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if wallet == "" {
		http.Error(w, `{"error":"wallet required"}`, 400)
		return
	}
	nonce := a.state.GetSwapNonce(wallet)
	json.NewEncoder(w).Encode(map[string]interface{}{"wallet": wallet, "nonce": nonce})
}

// handlePriceHistory returns AEQ/tUSD price snapshots for the chart.
// GET /api/price-history?minutes=240&limit=5000
//
// FIX (launch audit 2026-07-03): the 43200-minute (30-day) clamp that used
// to live here duplicated (and was shorter than) GetPriceHistory's own
// clamp — if only this one had been raised to support the 1mo/3mo/1y/all
// chart intervals, GetPriceHistory would never have seen a value above 30
// days anyway. Removed in favor of one clamp, in one place (GetPriceHistory,
// derived from priceHistoryRetentionDays) — this handler just does basic
// input sanitization (positive integers) and lets the state layer own the
// actual bound so the two can't drift apart again.
func (a *APIServer) handlePriceHistory(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Cache-Control", "no-cache")
	// FIX (P2, launch audit 2026-07-03): this was the one read-adjacent
	// endpoint in this file with no rate limiting at all, unlike every
	// comparable endpoint (see handleConfirmAlive, handleExportSnapshot).
	// Now more expensive than before given the much larger retention
	// window above, so an unauthenticated per-IP throttle matters more,
	// not less. 2s is generous for real chart usage (fetched on load and
	// on interval-button clicks, not on every poll) but meaningfully caps
	// repeated-scan abuse.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("price-history:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 2*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store("price-history:"+ip, time.Now())
	minutes := 240
	limit := 1000
	fmt.Sscanf(r.URL.Query().Get("minutes"), "%d", &minutes)
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	if minutes < 1 {
		minutes = 1
	}
	if limit < 1 {
		limit = 1
	}
	history := a.state.GetPriceHistory(minutes, limit)
	json.NewEncoder(w).Encode(map[string]interface{}{"history": history, "count": len(history)})
}

// handleGiniHistory returns Gini snapshots stored after each UBI distribution.
// Falls back to the current Gini as a single point when no history exists yet.
func (a *APIServer) handleGiniHistory(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Cache-Control", "no-cache")
	history := a.state.GetGiniHistory(60) // last 60 snapshots
	if len(history) == 0 {
		// First UBI hasn't run yet — return current state as bootstrap point.
		gini := a.state.CalcGini()
		humans := a.state.TotalHumans()
		history = []map[string]interface{}{
			{"idx": gini * 100, "gini": gini, "humans": humans, "timestamp": time.Now().Unix()},
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"history": history})
}

// handleWealthCap returns the current wealth cap parameters.
// Field names match the live wealth-cap widget in the Equality tab.
func (a *APIServer) handleWealthCap(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Cache-Control", "no-cache")
	// P2-2: use GetWealthCapInfo which internally calls bootstrapMultiplierLocked()
	// and getAverageBalanceLocked() — the SAME functions enforceWealthCapLocked uses.
	// The old implementation had its own formula that diverged from the enforcement logic.
	capAEQ, mult, avg, n := a.state.GetWealthCapInfo()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cap_aeq": capAEQ, "multiplier": mult, "average_aeq": avg, "humans": n,
	})
}

// handleSignValidatorChallenge signs the key-possession challenge message with
// RELAYER_PRIVATE_KEY. Restricted to loopback (127.0.0.1 / ::1) so only
// node operators with server access can use it — not an internet-accessible oracle.
// GET /api/sign-validator-challenge?wallet=0x...
func (a *APIServer) handleSignValidatorChallenge(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	// FIX: the doc comment above has always claimed this is "restricted to
	// loopback (127.0.0.1 / ::1)", but no such check actually existed — the
	// only gate was SNAPSHOT_TOKEN. That meant anyone who obtained the token
	// could call this from anywhere on the internet, contradicting the
	// stated design and removing the network-position defense-in-depth layer
	// the comment promised. Enforce it for real: the raw TCP peer (not the
	// XFF-trusting clientIP helper, since that would let a private-network
	// caller spoof an arbitrary forwarded IP) must be a loopback or private
	// address — i.e. this endpoint must be reached from the node's own host
	// or its private network (a co-located reverse proxy), never directly
	// from the public internet, even with a valid token.
	peerHost, _, splitErr := net.SplitHostPort(r.RemoteAddr)
	if splitErr != nil {
		peerHost = r.RemoteAddr
	}
	if !isPrivateOrLoopback(peerHost) {
		http.Error(w, `{"error":"this endpoint is restricted to the node's local/private network"}`, http.StatusForbidden)
		return
	}
	// FIX (P3-03): on Railway (and most cloud platforms) all TCP connections
	// arrive via an internal load balancer with a private/RFC1918 IP, so
	// isPrivateOrLoopback passes for every request, including those from the
	// public internet. Require an explicit opt-in env var so this endpoint is
	// disabled by default and operators must consciously enable it.
	if os.Getenv("ALLOW_SIGN_VALIDATOR_CHALLENGE") != "true" {
		http.Error(w, `{"error":"sign-validator-challenge is disabled; set ALLOW_SIGN_VALIDATOR_CHALLENGE=true on this node to enable"}`, http.StatusForbidden)
		return
	}
	// F12-FIX: Require SNAPSHOT_TOKEN unconditionally. Previously the endpoint
	// was open when SNAPSHOT_TOKEN was not set. An open endpoint leaks that the
	// node is running and allows unauthenticated challenge generation.
	token := os.Getenv("SNAPSHOT_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SNAPSHOT_TOKEN not configured on this node"}`, http.StatusForbidden)
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), []byte(token)) != 1 {
		http.Error(w, `{"error":"unauthorized — set Authorization: Bearer <SNAPSHOT_TOKEN>"}`, 401)
		return
	}
	humanWallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if humanWallet == "" || !strings.HasPrefix(humanWallet, "0x") || len(humanWallet) != 42 {
		http.Error(w, `{"error":"wallet required (0x...)"}`, 400)
		return
	}
	key := a.blockchain.GetSigningKey()
	if key == nil {
		http.Error(w, `{"error":"RELAYER_PRIVATE_KEY not configured"}`, 500)
		return
	}
	message := "Aequitas: validator key linked to human " + humanWallet
	msgHash := accounts.TextHash([]byte(message))
	sig, err := crypto.Sign(msgHash, key)
	if err != nil {
		http.Error(w, `{"error":"signing failed"}`, 500)
		return
	}
	sig[64] += 27
	signingAddr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	json.NewEncoder(w).Encode(map[string]interface{}{
		"signing_address": signingAddr,
		"human_wallet":    humanWallet,
		"signature":       "0x" + hex.EncodeToString(sig),
		"message":         message,
	})
}

// handleRegisterValidatorKey links a node signing key to a registered human
// wallet, authorising that signing key to propose blocks.
//
// Requires TWO signatures proving control of BOTH keys:
//
//	human_signature:      personal_sign("Aequitas: authorize validator {signing_address}", human_wallet)
//	signing_key_signature: personal_sign("Aequitas: validator key linked to human {human_wallet}", signing_address)
//
// The double-signature requirement proves the requester controls both the
// human wallet AND the node signing key, preventing impersonation attacks
// where someone registers a victim's signing address using their own wallet.
// UNIQUE(human_wallet) ensures one human = one validator key.
//
// P1-05 (audit): the human_signature message is "Aequitas: authorize validator"
// (without "key") — same as peer-registration (sync_blocks.go) and auto-binding.
// Old "authorize validator key" variant is accepted as fallback during migration.
func (a *APIServer) handleRegisterValidatorKey(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		SigningAddress      string `json:"signing_address"`
		HumanWallet         string `json:"human_wallet"`
		HumanSignature      string `json:"human_signature"`
		SigningKeySignature string `json:"signing_key_signature"`
		// Optional: der Ed25519-Schluessel, mit dem dieser Validator Menschen
		// bezeugt, plus ein Besitznachweis darueber.
		//
		// Er gehoert ins Register und nicht in eine handgepflegte
		// Umgebungsvariable: eine Liste, die jemand auf jeder Box eintragen
		// muss, IST eine Genehmigung.
		PersonhoodKey       string `json:"personhood_key"`
		PersonhoodSignature string `json:"personhood_signature"`
		// Wo der Vergleichsdienst dieses Betreibers erreichbar ist.
		MatchingURL string `json:"matching_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, 400)
		return
	}
	signingAddr := strings.ToLower(strings.TrimSpace(req.SigningAddress))
	humanWallet := strings.ToLower(strings.TrimSpace(req.HumanWallet))
	if !strings.HasPrefix(signingAddr, "0x") || len(signingAddr) != 42 ||
		!strings.HasPrefix(humanWallet, "0x") || len(humanWallet) != 42 {
		http.Error(w, `{"error":"invalid address"}`, 400)
		return
	}
	// 1. Human wallet proves it authorises this signing key.
	// P1-05 (audit): canonical message is "authorize validator" (no "key").
	// Accept the old "authorize validator key" variant as a migration fallback.
	humanMsg := "Aequitas: authorize validator " + signingAddr
	if err := verifyPersonalSign(humanMsg, req.HumanSignature, humanWallet); err != nil {
		oldMsg := "Aequitas: authorize validator key " + signingAddr
		if err2 := verifyPersonalSign(oldMsg, req.HumanSignature, humanWallet); err2 != nil {
			jsonError(w, "invalid human_signature: "+err.Error(), 400)
			return
		}
	}
	// 2. Signing key proves it is linked to this human wallet (key-possession proof).
	signingMsg := "Aequitas: validator key linked to human " + humanWallet
	if err := verifyPersonalSign(signingMsg, req.SigningKeySignature, signingAddr); err != nil {
		jsonError(w, "invalid signing_key_signature — sign with RELAYER_PRIVATE_KEY: "+err.Error(), 400)
		return
	}
	// 3. Der Bezeugungsschluessel beweist, dass er zu DIESEM Menschen gehoert.
	//
	// Ohne diesen Nachweis koennte jemand einen fremden oeffentlichen
	// Schluessel eintragen -- und dessen Unterschriften wuerden fortan unter
	// seiner Registrierung zaehlen. Der Beweis ist derselbe Gedanke wie bei der
	// Signieradresse eine Zeile darueber: wer eintraegt, muss besitzen.
	personhood := strings.ToLower(strings.TrimSpace(req.PersonhoodKey))
	if personhood != "" {
		if len(personhood) != 64 || strings.Trim(personhood, "0123456789abcdef") != "" {
			jsonError(w, "personhood_key must be 64 hex characters (Ed25519 public key)", 400)
			return
		}
		if !verifyPersonhoodPossession(personhood, req.PersonhoodSignature, humanWallet) {
			jsonError(w, "invalid personhood_signature — sign \"Aequitas: personhood key for human <wallet>\" "+
				"with the Ed25519 key itself", 400)
			return
		}
	}
	// Nur HTTPS, und keine privaten Adressen: diese URL wird spaeter von
	// FREMDEN Coordinatoren aufgerufen. Eine http:// oder 127.0.0.1-Adresse im
	// Register waere entweder unbrauchbar oder ein Weg, andere auf das eigene
	// Netz zeigen zu lassen.
	matchingURL := strings.TrimRight(strings.TrimSpace(req.MatchingURL), "/")
	if matchingURL != "" && !isAllowedPeerURL(matchingURL) {
		jsonError(w, "matching_url must be a public https:// address", 400)
		return
	}
	if err := a.state.RegisterValidatorFull(signingAddr, humanWallet, personhood, matchingURL); err != nil {
		jsonStateError(w, "register-validator-key", signingAddr, err)
		return
	}
	a.blockchain.AddAuthorizedValidator(signingAddr)
	fmt.Printf("[VALIDATOR] ✓ Registered key %s for human %s\n", signingAddr, humanWallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"signing_address": signingAddr,
		"human_wallet":    humanWallet,
		"personhood_key":  personhood,
		"matching_url":    matchingURL,
	})
}

// verifyPersonhoodPossession prueft, dass der Eintragende den privaten Teil des
// Ed25519-Schluessels wirklich besitzt.
//
// Unterschrieben wird eine Nachricht, die den MENSCHEN nennt. Damit laesst sich
// eine abgefangene Signatur nicht unter einer anderen Registrierung
// wiederverwenden.
func verifyPersonhoodPossession(publicHex, signatureHex, humanWallet string) bool {
	roh, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(signatureHex), "0x"))
	if err != nil || len(roh) != ed25519.SignatureSize {
		return false
	}
	pub, err := hex.DecodeString(publicHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	msg := []byte("Aequitas: personhood key for human " + strings.ToLower(strings.TrimSpace(humanWallet)))
	return ed25519.Verify(ed25519.PublicKey(pub), msg, roh)
}

// handleValidatorList returns registered validator key pairs (signing_address +
// human_wallet) so peer nodes can verify credentials before trusting an address.
// Peers check IsHuman(human_wallet) locally before calling AddAuthorizedValidator.
// See syncValidatorsFromPeer in sync_blocks.go for the receiver-side verification.
func (a *APIServer) handleValidatorList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"validators": a.blockchain.ValidatorKeyPairs(),
	})
}

// handlePeerChallenge issues a one-time challenge that the peer must sign to
// prove ownership of their signing key (P1-3 validator signature verification).
// GET /api/peers/challenge?address=0x...
func (a *APIServer) handlePeerChallenge(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	addr := strings.ToLower(r.URL.Query().Get("address"))
	if !isValidWalletAddr(addr) {
		http.Error(w, `{"error":"invalid address"}`, 400)
		return
	}
	challenge := a.blockchain.IssuePeerChallenge(addr)
	if challenge == "" {
		jsonError(w, "too many pending challenges, retry after 90 seconds", 429)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"challenge":    challenge,
		"expires_in":   90,
		"instructions": "Sign the challenge string with your signing key and include the hex signature in POST /api/peers/register as 'signature'",
	})
}

// registrationRateLimitWindow bounds how often ONE signing address's
// registration attempts are processed — independent of, and much narrower
// than, that address's per-proposer block-acceptance circuit breaker
// (proposerBlockBlocked). Deliberately its OWN cooldown, not a reuse of the
// breaker's: see handlePeerRegister's call site for the incident this fixes.
const registrationRateLimitWindow = 20 * time.Second

var (
	registrationRateLimitMu sync.Mutex
	lastRegistrationAttempt = map[string]int64{} // lower-cased signing address -> unix-nano of last processed attempt
)

// registrationRateLimited reports whether addr registered too recently to
// process again right now, and if not, records this attempt as the new
// "last processed" time. Deliberately decoupled from proposerBlockBlocked —
// see handlePeerRegister's call site for why reusing that breaker here was
// a bug, not a feature.
func registrationRateLimited(addr string) bool {
	if addr == "" {
		return false
	}
	registrationRateLimitMu.Lock()
	defer registrationRateLimitMu.Unlock()
	now := time.Now().UnixNano()
	if last, ok := lastRegistrationAttempt[addr]; ok && now-last < int64(registrationRateLimitWindow) {
		return true
	}
	lastRegistrationAttempt[addr] = now
	return false
}

// handlePeerRegister accepts a node registration and returns the current peer
// list plus all authorized validator addresses. A node that sends its
// signing_address is automatically added to the authorized validator set so
// its blocks are accepted without manual AUTHORIZED_VALIDATORS configuration.
// POST /api/peers/register  body: {"url":"https://...","signing_address":"0x...","signature":"0x..."}
func (a *APIServer) handlePeerRegister(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var req struct {
		URL            string `json:"url"`
		SigningAddress string `json:"signing_address"`
		// MPCReady: this peer offers to serve the private duplicate check.
		// Absent (older nodes, or nodes not offering it) means false, which
		// keeps them out of committee selection — a drawn member that cannot
		// take part stalls every comparison the committee is asked for.
		MPCReady           bool   `json:"mpc_ready"`
		PeerSecret         string `json:"peer_secret"`
		Signature          string `json:"signature"` // P1-3 challenge-response
		NodeOperatorWallet string `json:"node_operator_wallet"`
		// OperatorBindingSignature proves NODE_OPERATOR_WALLET ownership —
		// see TryClaimValidatorSlot's old comment for why this was missing:
		// nothing previously verified that the requester actually controls
		// node_operator_wallet, only that SOME registered human owns that
		// address. Generated out-of-band (the operator's wallet signs
		// "Aequitas: authorize validator <signing_address>" via the web tool
		// at /node-binding or any EIP-191 personal_sign-capable wallet) since
		// the node process itself never has access to the operator's wallet
		// private key — that key lives with the human, not the server.
		OperatorBindingSignature string `json:"operator_binding_signature"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	// FIX: a peer's SELF_URL is often sourced from a hosting provider's
	// "public domain" variable (e.g. Railway's RAILWAY_PUBLIC_DOMAIN), which
	// never includes a scheme — that bare hostname fails isAllowedPeerURL's
	// "must be public HTTPS" check below and the registration is silently
	// dropped with no indication on the PEER's own side that anything is
	// wrong (only this node's logs show the rejection). Normalize defensively
	// here so a scheme-less but otherwise valid public hostname still works.
	req.URL = NormalizeNodeURL(req.URL)

	// Secret check comes FIRST. URL registration and sync goroutines are only
	// started for authenticated peers — prevents goroutine exhaustion via
	// unauthenticated registrations even when PEER_SECRET is not set.
	//
	// FIX (audit recheck3, P1 — "PEER_SECRET bleibt ein globales
	// Shared-Secret im Validator/Peer-System"): a leaked PEER_SECRET used
	// to be an equivalent, always-on bypass for both URL registration and
	// (until the operator-binding-signature check a few lines below, which
	// is unconditional regardless of secretOK) the validator path — hard
	// to rotate safely across independent node operators who all share one
	// value, the opposite of what this project's identity-based
	// (NODE_OPERATOR_WALLET + signature) model is for. Confirmed every
	// current secondary already sends a real challenge-response signature
	// on every registration call (see registerAndDiscover, sync_blocks.go)
	// in addition to peer_secret — PEER_SECRET there is redundant, not
	// load-bearing. So the bypass itself is now opt-in via
	// ALLOW_PEER_SECRET_BYPASS=true (testnet/bootstrap convenience only);
	// by default PEER_SECRET being set no longer grants anything, and only
	// the signature-based path (sigOK/sigOKEarly) authenticates peers.
	peerSecretBypassEnabled := os.Getenv("ALLOW_PEER_SECRET_BYPASS") == "true"
	peerSecret := os.Getenv("PEER_SECRET")
	// P1-2: constant-time comparison prevents timing-based secret oracle attacks.
	secretOK := peerSecretBypassEnabled && peerSecret != "" && subtle.ConstantTimeCompare([]byte(req.PeerSecret), []byte(peerSecret)) == 1

	// P1-2: compute sigOK early so it can gate URL registration.
	// A known validator address (keyAuthorizedEarly) alone is NOT sufficient —
	// anyone can read validator addresses from /api/blocks. Require PEER_SECRET
	// match OR a valid challenge-response signature to prove private-key ownership.
	sigOKEarly := req.Signature != "" && req.SigningAddress != "" &&
		a.blockchain.VerifyPeerChallenge(strings.ToLower(req.SigningAddress), req.Signature)

	// FIX (P0, 2026-07-02): a proposer whose blocks are being cleanly
	// rejected by the per-proposer circuit breaker (block.go, AddPeerBlock)
	// was still free to hammer this endpoint at will — re-authenticating
	// costs a full signature verification plus a BindValidatorSlot DB write
	// regardless of whether its blocks ever attach.
	//
	// FIX (P0, 2026-07-04 — Contabo 2 permanent-isolation incident):
	// the original fix here reused proposerBlockBlocked directly, on the
	// theory that it "self-clears the moment the proposer produces an
	// attaching block again, so a diverged operator who fixes their node and
	// resyncs is let back in automatically, no manual action needed." That
	// reasoning has a hole: registering is the SAME mechanism a diverged
	// node uses to fetch a fresh peer/validator list and resume real
	// catch-up — the SelfFetched-tagged sync fetches this endpoint's
	// response enables are exactly how a resynced node's blocks start
	// attaching again in the first place. Gating registration itself on the
	// breaker being CLOSED created a deadlock: confirmed live, Contabo 2's
	// registration was rejected (HTTP 429, this exact reason string) on
	// every single attempt for 2+ continuous hours, because the primary's
	// breaker against its address never got a successful attach to clear
	// against — precisely because registration (which would have helped
	// fix that) kept being refused. A resync on the Contabo 2 side alone
	// could never break this: it doesn't touch the PRIMARY's in-memory
	// breaker state. Now decoupled: registrationRateLimited uses its own
	// short, independent cooldown (registrationRateLimitWindow) — still
	// keyed on signing address, still cheap to enforce, but it always lets
	// a genuinely-recovering node back in on a short, predictable cadence
	// instead of only after its breaker happens to clear, which by
	// definition it can't do without the very thing this gate was blocking.
	//
	// FIX (P0, 2026-07-09 — unauthenticated registration-griefing DoS found
	// live): this check used to run BEFORE any authentication at all (right
	// after JSON decode), keyed purely on the caller-supplied
	// signing_address with no proof of ownership required. Validator
	// signing addresses are PUBLIC — visible in every block's proposer
	// field — so anyone on the open internet could send an unsigned POST
	// naming a real validator's address and consume/refresh that address's
	// rate-limit slot, with no signature and no PEER_SECRET at all.
	// Confirmed live: two legitimate secondaries' own signed re-registration
	// attempts were rejected 429 continuously for 10+ minutes even with a
	// fresh retry loop, while an unauthenticated curl request bearing one of
	// their addresses and an empty signature got exactly the same "rate
	// limited" response — proving the slot was being consumed by requests
	// that never proved key ownership at all (a scanner, or simply this
	// same 429 response itself being indistinguishable from a genuine hit,
	// masking who actually claimed the slot each time). Gating consumption
	// on secretOK||sigOKEarly closes this: only a request that has already
	// proven it holds the signing key (or the explicit PEER_SECRET bypass)
	// can claim or be blocked by this address's slot — an unauthenticated
	// caller naming someone else's address can no longer grief it.
	if addr := strings.ToLower(strings.TrimSpace(req.SigningAddress)); addr != "" && (secretOK || sigOKEarly) && registrationRateLimited(addr) {
		fmt.Printf("[PEERS] ✗ Registration from %s rate-limited — retried within %s of its last attempt\n", addr, registrationRateLimitWindow)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"ok":false,"reason":"registration rate-limited, retry shortly"}`))
		return
	}

	// FIX (audit 2026-06-28 full recheck, P1-6): URL registration (and the
	// sync goroutine it starts via startSyncForPeer) used to run immediately
	// here, gated only on secretOK||sigOKEarly — i.e. proof of holding SOME
	// private key with a previously-issued challenge, which says nothing
	// about whether that key belongs to an authorized validator, let alone
	// one bound to a verified human operator. Anyone could request a
	// challenge for an arbitrary, freshly generated address (VerifyPeerChallenge
	// only checks private-key possession, not validator status), sign it,
	// and get this node to register and actively sync with an
	// attacker-chosen URL — entirely bypassing the NODE_OPERATOR_WALLET
	// human-check and operator-binding-signature verification below, which
	// only ever gated the VALIDATOR authorization, not the URL registration
	// that had already happened by the time those checks ran. Now
	// urlAuthorized only becomes true via sigOKEarly once THIS SAME request
	// has also passed full validator binding (human-check + binding
	// signature + BindValidatorSlot) below. secretOK alone (the explicit,
	// opt-in PEER_SECRET bypass) still authorizes URL registration on its
	// own, exactly as documented where secretOK is computed above — moving
	// the registration call itself doesn't change that bypass's semantics.
	urlAuthorized := secretOK
	registerURLIfAuthorized := func() {
		if req.URL != "" && isAllowedPeerURL(req.URL) {
			if urlAuthorized {
				// Record the signing address alongside the URL: this
				// registration has just proved ownership of that key, and it
				// is the only moment the two halves are known together.
				GlobalPeerRegistry.RegisterWithMPC(req.URL, req.SigningAddress, req.MPCReady)
				fmt.Printf("[PEERS] Registered: %s\n", req.URL)
				a.blockchain.startSyncForPeer(req.URL)
			} else {
				fmt.Printf("[PEERS] URL rejected (no valid PEER_SECRET or validator key): %s\n", req.URL)
			}
		} else if req.URL != "" {
			fmt.Printf("[PEERS] URL rejected (must be public HTTPS): %s\n", req.URL)
		}
	}
	if addr := strings.ToLower(strings.TrimSpace(req.SigningAddress)); addr != "" && strings.HasPrefix(addr, "0x") && len(addr) == 42 {
		// Authorization: accept if PEER_SECRET matches OR if the address has
		// a registered validator key (individual human-signed credential) OR
		// if the peer provided a valid challenge-response signature (P1-3).
		// P2-FIX: VerifyPeerChallenge is one-time-use (deletes the
		// challenge on first call). sigOKEarly consumed it already above;
		// calling VerifyPeerChallenge again would always return false.
		sigOK := sigOKEarly && strings.ToLower(strings.TrimSpace(req.SigningAddress)) == addr
		keys := a.state.GetValidatorKeys()
		keyAuthorized := false
		for _, k := range keys {
			if k["signing_address"] == addr {
				keyAuthorized = true
				break
			}
		}
		// Authorization: PEER_SECRET match OR a valid challenge-response signature.
		// keyAuthorized alone is not sufficient — anyone can read validator addresses
		// from /api/blocks. The peer must prove private-key possession (sigOK) or
		// share the PEER_SECRET. (keyAuthorized && sigOK) is a subset of sigOK and
		// was removed as dead code (FIX 6).
		if secretOK || sigOK {
			nodeWallet := strings.ToLower(strings.TrimSpace(req.NodeOperatorWallet))
			if nodeWallet == "" {
				nodeWallet = addr
			}
			if !a.state.IsHuman(nodeWallet) {
				fmt.Printf("[PEERS] Rejected %s: NODE_OPERATOR_WALLET %s is not a registered human\n", addr, nodeWallet)
				http.Error(w, `{"error":"NODE_OPERATOR_WALLET is not a registered human — register first via the AequitasBio app"}`, http.StatusForbidden)
				return
			}
			// FIX (one-human-one-validator + ownership proof): NODE_OPERATOR_WALLET
			// being a verified human is necessary but not sufficient — IsHuman
			// only confirms SOME registered human owns that address, not that
			// THIS requester does. Without proof, anyone controlling a
			// validator signing key could submit any other human's wallet as
			// NODE_OPERATOR_WALLET and permanently squat their validator slot.
			// Require a signature from operatorWallet itself, over a message
			// naming THIS specific signing address — generated out-of-band via
			// the operator's own wallet (e.g. the /node-binding tool, any
			// EIP-191 personal_sign-capable wallet), since the node process
			// never has access to the human's wallet private key. The same
			// mechanism doubles as self-service rebind: a fresh signature
			// naming a new signing address overwrites the old binding, no
			// admin or biometric re-verification needed.
			bindingMsg := "Aequitas: authorize validator " + addr
			if err := verifyPersonalSign(bindingMsg, req.OperatorBindingSignature, nodeWallet); err != nil {
				fmt.Printf("[PEERS] Rejected %s: NODE_OPERATOR_WALLET %s ownership not proven: %v\n", addr, nodeWallet, err)
				http.Error(w, `{"error":"operator_binding_signature missing or invalid — sign 'Aequitas: authorize validator <your signing address>' with your NODE_OPERATOR_WALLET to prove ownership (see /node-binding)"}`, http.StatusForbidden)
				return
			}
			if err := a.state.BindValidatorSlot(nodeWallet, addr, req.OperatorBindingSignature); err != nil {
				fmt.Printf("[PEERS] Rejected %s: could not bind validator slot for %s: %v\n", addr, nodeWallet, err)
				http.Error(w, `{"error":"internal error binding validator slot"}`, http.StatusInternalServerError)
				return
			}
			a.blockchain.AddAuthorizedValidator(addr)
			// Fully validated now: human-owned wallet, proven binding
			// signature, key-proven signing address — safe to also
			// authorize this request's URL registration (see urlAuthorized's
			// own comment above for why this couldn't just be sigOKEarly).
			urlAuthorized = true
			method := "PEER_SECRET"
			if sigOK {
				method = "challenge-response signature"
			}
			if keyAuthorized && sigOK {
				method += " (registered key)"
			}
			fmt.Printf("[PEERS] Auto-authorized validator via %s: %s (wallet: %s)\n", method, addr, nodeWallet)
		} else if req.Signature == "" {
			fmt.Printf("[PEERS] Validator %s: no signature provided — request /api/peers/challenge first\n", addr)
		} else {
			fmt.Printf("[PEERS] Validator %s: invalid/expired challenge signature\n", addr)
		}
		registerURLIfAuthorized()
		a.blockchain.mu.RLock()
		validators := make([]string, 0, len(a.blockchain.authorizedValidators))
		for v := range a.blockchain.authorizedValidators {
			validators = append(validators, v)
		}
		a.blockchain.mu.RUnlock()
		// P2-9: only return validator list if authorized via secret or proven key ownership.
		// keyAuthorized alone (without sigOK or secretOK) must NOT reveal the validator list —
		// anyone can enumerate validator addresses from /api/blocks.
		if secretOK || sigOK {
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL")), "validators": validators})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL"))})
		}
		return
	}
	// No (valid-looking) signing address in this request — URL registration
	// can only be authorized via the PEER_SECRET bypass here.
	registerURLIfAuthorized()
	json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL")), "validators": []string{}})
}

// handleProveProxy proxies POST /api/prove to the proof server backend-side,
// bypassing browser CORS restrictions. The proof server does not include
// Access-Control-Allow-Origin, so browser fetches fail. By proxying through
// the chain node (same origin as the website), CORS is not an issue.
func (a *APIServer) handleProveProxy(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	body, tooLarge, err := readBodyLimited(w, r, 64<<10)
	if tooLarge {
		jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"read error"}`, 500)
		return
	}
	// FIX (audit 2026-06-28 full recheck, P1-5): the proof server's own
	// proveLimiter keys its rate limit on `req.ip` — but every request it
	// ever sees arrives FROM this proxy, on this proxy's IP, regardless of
	// which wallet originated it. That makes the proof server's entire
	// shared budget (5 requests/minute, see server.js's PROVE_RATE_MAX) a
	// single bucket for every wallet behind this chain node combined: one
	// wallet retry-looping (buggy client, or deliberate abuse) can exhaust
	// it and lock out every other legitimate registration attempt. This
	// proxy is the only layer that still knows which wallet a request came
	// from before it gets collapsed into that shared IP bucket, so the
	// per-wallet throttle has to live here.
	var proveBody struct {
		Wallet string `json:"wallet"`
	}
	if jsonErr := json.Unmarshal(body, &proveBody); jsonErr == nil && proveBody.Wallet != "" {
		walletKey := "prove-wallet:" + strings.ToLower(proveBody.Wallet)
		if ts, loaded := registerRateLimit.Load(walletKey); loaded {
			if time.Since(ts.(time.Time)) < 15*time.Second {
				jsonError(w, "rate limited, try again shortly", 429)
				return
			}
		}
		registerRateLimit.Store(walletKey, time.Now())
	}
	// FIX (Gesamtaudit 2026-06-28, P2-8): the wallet-keyed throttle above
	// doesn't stop an attacker rotating wallet addresses from a single
	// browser/IP — each new wallet gets its own fresh 15s budget. This is
	// the one layer that still sees the ORIGINAL caller's IP before the
	// request collapses into this proxy's own outbound IP at the proof
	// server, so add that as a second, independent key.
	ipKey := "prove-ip:" + clientIP(r)
	if ts, loaded := registerRateLimit.Load(ipKey); loaded {
		if time.Since(ts.(time.Time)) < 3*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store(ipKey, time.Now())
	if len(proofServerURLs()) == 0 {
		http.Error(w, `{"error":"no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node"}`, 503)
		return
	}
	// CHAIN_SERVICE_TOKEN is added inside doProofServerRequestFailover
	// (addProofServerAuth) so the proof server's auth check passes. The
	// token lives only in the chain node's env var and is never exposed to
	// browser clients — the proxy is the sole caller of the proof server.
	resp, err := doProofServerRequestFailover("POST", "/prove", body, 120*time.Second, nil)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	// Herkunft festhalten, BEVOR die Antwort rausgeht.
	//
	// Ein Status 200 vom Proof-Server heisst: die Bescheinigung wurde geprueft.
	// Dort steht BIO_ATTESTATION_MODE=required, ohne gueltige Bescheinigung
	// antwortet er 403. Dieser Knoten merkt sich also, dass DIESER Nullifier
	// durch die Pruefung gekommen ist -- /api/register nimmt nur solche.
	// Siehe prove_provenance.go fuer die Luecke, die das schliesst.
	if resp.StatusCode == http.StatusOK {
		merkeProveHerkunft(respBody)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleProveGetProxy proxies GET /api/prove/get/{id} to the proof server.
func (a *APIServer) handleProveGetProxy(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	id := strings.TrimPrefix(r.URL.Path, "/api/prove/get/")
	// FIX 6: strict allowlist replaces denylist -- prevents path traversal.
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{1,64}$`, id)
	if !matched {
		http.Error(w, `{"error":"invalid proof id"}`, 400)
		return
	}
	if len(proofServerURLs()) == 0 {
		http.Error(w, `{"error":"no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node"}`, 503)
		return
	}
	resp, err := doProofServerRequestFailover("GET", "/get/"+id, nil, 30*time.Second, nil)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleProveStoreProxy proxies POST /api/prove/store to the proof server's /store
// endpoint. This is the endpoint APKs should call instead of hitting the proof
// server directly — the APK does not hold (and must not hold) the
// CHAIN_SERVICE_TOKEN, so any direct call from an APK to the proof server's /store
// would fail auth. This proxy accepts the proof body from the APK, applies
// per-IP rate limiting, and forwards it to the proof server with the service token
// added server-side. (FIX BRUTAL-P2-06)
func (a *APIServer) handleProveStoreProxy(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	ipKey := "prove-store-ip:" + clientIP(r)
	if ts, loaded := registerRateLimit.Load(ipKey); loaded {
		if time.Since(ts.(time.Time)) < 10*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store(ipKey, time.Now())
	body, tooLarge, err := readBodyLimited(w, r, 64<<10)
	if tooLarge {
		jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"read error"}`, 500)
		return
	}
	if len(proofServerURLs()) == 0 {
		http.Error(w, `{"error":"no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node"}`, 503)
		return
	}
	resp, err := doProofServerRequestFailover("POST", "/store", body, 30*time.Second, nil)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleProofCheckProxy proxies POST /api/proof/check to the proof server.
func (a *APIServer) handleProofCheckProxy(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	body, tooLarge, err := readBodyLimited(w, r, 16<<10)
	if tooLarge {
		jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"read error"}`, 500)
		return
	}
	if len(proofServerURLs()) == 0 {
		http.Error(w, `{"error":"no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on this node"}`, 503)
		return
	}
	resp, err := doProofServerRequestFailover("POST", "/check", body, 30*time.Second, nil)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handlePeers returns the list of currently-active peer nodes (heartbeat
// within the last 5 minutes — see ActivePeers), excluding this node itself.
//
// FIX (2026-07-05 audit finding): this used to call AllPeers(), which
// returns every URL EVER registered with zero staleness filter — a node
// that registered once and then permanently disappeared (crashed,
// decommissioned, reassigned IP) stayed in every future discovery response
// forever, with no eviction path. New nodes bootstrapping via this
// endpoint learned about dead peers indefinitely instead of just the ones
// actually still running. ActivePeers already existed with the correct
// 5-minute-heartbeat semantics but had zero callers anywhere in the
// codebase until now.
// GET /api/peers
func (a *APIServer) handlePeers(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL"))})
}

// handleValidatorLabels returns a stable "Validator #N" ordinal for every
// signing address that has ever registered (registration order — see
// GetValidatorOrdinals' own comment for why this, not a hardcoded name per
// node). Deliberately public/unauthenticated: this reveals nothing beyond
// what /api/blocks already exposes to anyone — every block's Proposer field
// IS a signing address, fully enumerable without auth today (see P2-9's
// own comment on handlePeers' validator-list gating, which protects
// against a DIFFERENT concern, discovery-URL enumeration, not address
// secrecy). This endpoint only adds a friendlier, derived label for
// addresses that are already public.
// GET /api/validator-labels
func (a *APIServer) handleValidatorLabels(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	var ordinals map[string]int
	if a.blockchain != nil && a.blockchain.state != nil {
		ordinals = a.blockchain.state.GetValidatorOrdinals()
	}
	labels := make(map[string]string, len(ordinals)+len(validatorLabelOverrides))
	for addr, ord := range ordinals {
		labels[addr] = fmt.Sprintf("Validator #%d", ord)
	}
	// VALIDATOR_LABELS overrides take precedence — see its own comment
	// (block.go) for why this, not the per-node ordinal above, is the only
	// way "Primary" gets labeled at all and the only way numbering is
	// guaranteed identical across every node's explorer.
	for addr, label := range validatorLabelOverrides {
		labels[addr] = label
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"labels": labels})
}

// handleSigningAddress returns this node's signing address, protected by
// SNAPSHOT_TOKEN. Secondary node operators need this for BOOTSTRAP_SIGNER.
// Not exposed in /api/status to avoid leaking validator addresses publicly.
func (a *APIServer) handleSigningAddress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	token := os.Getenv("SNAPSHOT_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SNAPSHOT_TOKEN not configured"}`, http.StatusForbidden)
		return
	}
	authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(token)) != 1 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var addr string
	if sk := a.blockchain.GetSigningKey(); sk != nil {
		addr = strings.ToLower(crypto.PubkeyToAddress(sk.PublicKey).Hex())
	}
	json.NewEncoder(w).Encode(map[string]string{"signing_address": addr})
}

// handleRegistrationDebug reports, per-layer, whether a wallet shows up as
// already-registered anywhere — chain_accounts.is_human, nullifiers,
// bio_registrations, the chain's own bio_hashes table, and the V7 EVM
// isHuman storage slot. "Already registered" can come from any one of
// these independently, and they can disagree after a partial reset; this
// endpoint makes that visible instead of requiring a manual DB query.
// Protected by SNAPSHOT_TOKEN, same as the other operator-only endpoints.
// GET /api/admin/registration-debug?wallet=0x...
func (a *APIServer) handleRegistrationDebug(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	token := os.Getenv("SNAPSHOT_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SNAPSHOT_TOKEN not configured"}`, http.StatusForbidden)
		return
	}
	authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(token)) != 1 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if !isValidWalletAddr(wallet) {
		http.Error(w, `{"error":"wallet required (0x...)"}`, http.StatusBadRequest)
		return
	}
	info := a.state.GetRegistrationDebugInfo(wallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet":                  wallet,
		"chain_is_human":          info.ChainIsHuman,
		"chain_balance":           info.ChainBalance,
		"nullifier_exists":        info.NullifierExists,
		"bio_registration_exists": info.BioRegistrationExists,
		"bio_hash_exists":         info.BioHashExists,
		"evm_is_human_slot":       info.EVMIsHumanSlot,
		"note":                    "bio_hash_exists refers to the CHAIN's own bio_hashes table, not the separate proof-server service's bio_hashes table (different DB) — a 'biometric already registered' error from /api/proof/check is NOT reflected here.",
	})
}

// handleRegistrationRecovery lists registration_recovery records (BRUTAL-P1-01).
// Operator endpoint — protected by SNAPSHOT_TOKEN.
// GET  /api/admin/registration-recovery          → list all unrecovered records
// POST /api/admin/registration-recovery/retry    → trigger immediate retry pass
func (a *APIServer) handleRegistrationRecovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	token := os.Getenv("SNAPSHOT_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SNAPSHOT_TOKEN not configured"}`, http.StatusForbidden)
		return
	}
	authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(token)) != 1 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/retry") {
		n := a.state.RetryRegistrationRecoveries()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"recovered_this_pass": n,
			"still_pending":       a.state.CountUnrecoveredRegistrations(),
		})
		return
	}
	rows, err := a.state.DB().Query(`
		SELECT id, wallet, evm_tx_hash, nullifier, created_at, attempt_count,
		       last_attempt_at, recovered_at, last_error
		FROM registration_recovery ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type row struct {
		ID            int64  `json:"id"`
		Wallet        string `json:"wallet"`
		EVMTxHash     string `json:"evm_tx_hash"`
		Nullifier     string `json:"nullifier"`
		CreatedAt     int64  `json:"created_at"`
		AttemptCount  int    `json:"attempt_count"`
		LastAttemptAt *int64 `json:"last_attempt_at"`
		RecoveredAt   *int64 `json:"recovered_at"`
		LastError     string `json:"last_error"`
	}
	var records []row
	for rows.Next() {
		var rec row
		if scanErr := rows.Scan(&rec.ID, &rec.Wallet, &rec.EVMTxHash, &rec.Nullifier,
			&rec.CreatedAt, &rec.AttemptCount, &rec.LastAttemptAt,
			&rec.RecoveredAt, &rec.LastError); scanErr == nil {
			records = append(records, rec)
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":       len(records),
		"unrecovered": a.state.CountUnrecoveredRegistrations(),
		"records":     records,
	})
}

// handleSnapshot exports the full Go-state as a signed JSON snapshot.
// Protected by SNAPSHOT_TOKEN env var if set. A new node can bootstrap
// itself by setting BOOTSTRAP_SNAPSHOT_URL to this endpoint's URL.
//
// FIX (audit recheck3, P2 — "Snapshot-Endpoint ist token-geschuetzt, aber
// nicht netzwerkgebunden"): unlike handleSignValidatorChallenge, this
// endpoint genuinely needs to stay reachable over the public internet by
// design — every cross-cloud-provider bootstrap/resync this project relies
// on (a Railway node pulling from another Railway node, or from a
// self-hosted VPS, and vice versa) calls this exact endpoint across the
// open internet; restricting it to loopback/private by default the way
// handleSignValidatorChallenge does would break that mechanism outright,
// not harden it. So this adds the audit's second suggested option instead
// of its first: SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK=true is an opt-in for
// operators who don't need cross-network bootstrap and want the extra
// defense-in-depth layer; default behavior (this var unset) is unchanged
// from before — public reachability gated by SNAPSHOT_TOKEN alone, exactly
// as already documented above and already relied on in production.
func (a *APIServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK") == "true" {
		peerHost, _, splitErr := net.SplitHostPort(r.RemoteAddr)
		if splitErr != nil {
			peerHost = r.RemoteAddr
		}
		if !isPrivateOrLoopback(peerHost) {
			http.Error(w, `{"error":"this node restricts /api/snapshot to its local/private network (SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK=true)"}`, http.StatusForbidden)
			return
		}
	}
	// FIX (2026-06-28, SNAPSHOT_TOKEN redesign): this endpoint used to
	// reject every request outright unless SNAPSHOT_TOKEN was set AND
	// matched — meaning a brand-new, honest node operator had to contact
	// the network operator just to get the value needed to bootstrap at
	// all. That doesn't scale for a project whose whole point is
	// permissionless node operation. The actual thing worth gating is the
	// nullifier→wallet and bio_registrations linkage data (see
	// ExportSnapshot's doc comment) — not the ability to bootstrap a node
	// in the first place. So: a valid token now grants the FULL snapshot
	// (unchanged from before, for authoritative resync/recovery); no
	// token, or no SNAPSHOT_TOKEN configured on this node at all, serves
	// the PUBLIC tier (no bio_registrations, nullifier keys but no wallet
	// linkage) — still fully sufficient to bootstrap a correct, working
	// node, with no admin contact needed.
	token := os.Getenv("SNAPSHOT_TOKEN")
	// P2-15: token in Authorization header (not URL query param that lands in logs).
	authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	includeSensitive := token != "" && subtle.ConstantTimeCompare([]byte(authHeader), []byte(token)) == 1
	if !includeSensitive {
		// Public tier is no longer gated by a secret, so it needs its own
		// throttle against being used as a bulk-download/cost vector —
		// the same per-IP pattern used elsewhere in this file (e.g.
		// handleCheckRegistrationByBioHash).
		ip := clientIP(r)
		if ts, loaded := registerRateLimit.Load("snapshot-public:" + ip); loaded {
			if time.Since(ts.(time.Time)) < 30*time.Second {
				jsonError(w, "rate limited, try again shortly", 429)
				return
			}
		}
		registerRateLimit.Store("snapshot-public:"+ip, time.Now())
	}
	snap := a.state.ExportSnapshot(a.blockchain.GetSigningKey(), a.blockchain.Height(), includeSensitive)
	writeJSON(w)
	json.NewEncoder(w).Encode(snap)
}

// handleDapp serves the mobile wallet SPA (aequitas-dapp.html).
// FIX (P0-1, beta-launch audit 2026-07-05): this file existed in the repo,
// was actively maintained (last edited 2026-07-03 for launch fixes), and is
// a complete, working mobile wallet UI — but no route anywhere served it and
// the Dockerfile never copied it into the production image. Every path not
// otherwise registered on this mux falls through to the "/" handler
// (handleLanding/handleUI), so requests to any guessed URL for it were
// silently served the Explorer instead, with no error to signal the file
// was actually missing. Confirmed live: aequitas.digital/wallet returned
// the Explorer's <title>, not this file's "Aequitas Beta" — this endpoint
// simply didn't exist in production until this fix.
func (a *APIServer) handleDapp(w http.ResponseWriter, r *http.Request) {
	const path = "aequitas-dapp.html"
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "dapp not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "dapp not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// FIX (Monster Audit follow-up, 2026-07-12, P0/P1): this page used to set
	// NO Content-Security-Policy header at all — worse than the permissive
	// 'unsafe-inline' one every other HTML handler in this file carried. Its
	// script is now external+self-hosted (see dappJS's comment in
	// api_html.go) and its 21 onclick=/oninput= attributes are gone, so this
	// can go straight to a strict script-src with zero exceptions.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self' https://aequitas.digital; img-src 'self' data:; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	http.ServeContent(w, r, "aequitas-dapp.html", fi.ModTime(), f)
}

func (a *APIServer) handleDappJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprint(w, dappJS)
}

// defaultAPKReleaseURL is where /download/app.apk redirects when the mounted
// APK is absent. It stood at app-v1.4.1 while the shipped release was already
// app-v1.5.2 -- three releases back, and the older builds are exactly the ones
// whose personhood check was not yet enforced. It was written out twice in the
// handler below, so the two copies could drift apart; one constant now.
//
// This is the LAST resort: the boxes bind-mount the real APK at
// downloads/aequitas-app.apk and serve that (verified live 2026-08-24 --
// /download/app.apk returned 214,986,378 bytes, byte-for-byte app-v1.6.0).
// Operators override it per-box with AEQUITAS_APK_URL without a chain deploy,
// which is the normal path; keeping this constant current is belt-and-braces
// for a box where the variable was never set.
//
// Replacing the mounted file must WRITE INTO it (cat new > dest), never mv:
// a file bind-mount follows the inode, so a rename leaves the container
// serving the old file while the host shows the new one.
const defaultAPKReleaseURL = "https://github.com/hanoi96international-gif/Aequitas-App/releases/download/app-v1.6.0/app-release.apk"

func (a *APIServer) handleAppDownload(w http.ResponseWriter, r *http.Request) {
	const apkPath = "downloads/aequitas-app.apk"
	// FIX (N1, Audit 2026-08-18): this "fallback" is in practice the ONLY path.
	// downloads/*.apk is gitignored, so the file is never in the build context
	// and never in the image; os.Open therefore always fails and every download
	// takes the redirect. That made the app version a compile-time constant:
	// shipping a new APK required a code change and a full node redeploy, which
	// restarts both validators.
	//
	// AEQUITAS_APK_URL decouples the two. The default is the release that was
	// hardcoded here, so behaviour is unchanged until an operator sets it —
	// pointing it at a new release is now an env change and a container
	// restart, not a chain deploy.
	fallbackURL := os.Getenv("AEQUITAS_APK_URL")
	if fallbackURL == "" {
		fallbackURL = defaultAPKReleaseURL
	}
	// Only ever redirect to an absolute http(s) URL: an operator typo that left
	// a relative path here would otherwise turn this endpoint into an
	// open-redirect-shaped surprise on the node's own origin.
	if !strings.HasPrefix(fallbackURL, "https://") && !strings.HasPrefix(fallbackURL, "http://") {
		fmt.Printf("[APK] ⚠ AEQUITAS_APK_URL=%q is not an absolute http(s) URL — ignoring it\n", fallbackURL)
		fallbackURL = defaultAPKReleaseURL
	}
	f, err := os.Open(apkPath)
	if err != nil {
		// File not found in container — redirect to GitHub raw URL.
		http.Redirect(w, r, fallbackURL, http.StatusFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Redirect(w, r, fallbackURL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=aequitas-app.apk")
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, "aequitas-app.apk", fi.ModTime(), f)
}

func (a *APIServer) handleStaticDownload(w http.ResponseWriter, r *http.Request, path, filename, contentType string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "File not found", 404)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "File error", 500)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(filename)))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, filename, fi.ModTime(), f)
}

func (a *APIServer) handleLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	// FIX (Monster Audit 2026-07-12, P1): ethers now loads from same-origin
	// /vendor/ethers.min.js (self-hosted, version-pinned — see vendorEthersJS's
	// comment), so script-src no longer needs to allow cdnjs.cloudflare.com.
	// FIX (Monster Audit 2026-07-12 follow-up, P1): loadStats()/the smooth-scroll
	// handler now live in the same-origin /landing.js file (see landingJS's
	// comment in api_html.go), so script-src no longer needs 'unsafe-inline'.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	fmt.Fprint(w, landingHTML)
}

func (a *APIServer) handleLandingJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprint(w, landingJS)
}

// Brand assets. Cached for a day rather than the hour the scripts use: these
// change on a redesign, not on a deploy, and the favicon is requested on
// every single page load.
//
// /favicon.ico is served the SVG deliberately. Browsers still probe that path
// even when a page declares an icon, and answering it with the real image
// beats letting it fall through to the catch-all — content sniffing settles
// the format, and every browser that asks for .ico today also reads SVG.
func (a *APIServer) handleFaviconSVG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, faviconSVG)
}

func (a *APIServer) handleAppleTouchIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(appleTouchIcon)
}

// The link-preview card. Crawlers fetch it cross-origin from X and Telegram,
// so it carries an explicit permissive CORS header the way the APK download
// already does.
func (a *APIServer) handleOGImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(ogImagePNG)
}

func (a *APIServer) handleNodeBindingJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprint(w, nodeBindingJS)
}

// handleCoordinatorBinding is the coordinator's counterpart to
// handleNodeBinding.
//
// WHY THIS PAGE EXISTS
//
// A coordinator issues the attestation this chain mints on, so its Ed25519
// key has to be tied to a registered human before any matching service
// accepts what it signs. That tie needs a secp256k1 signature from the
// human's own wallet -- and wallets have no built-in UI for signing a plain
// string, which left every prospective operator to improvise one.
//
// The Ed25519 half is deliberately NOT here: that key lives on the
// coordinator's own host, and a page that asked for it would be teaching
// operators to paste their signing key into a web form.
func (a *APIServer) handleCoordinatorBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self'; style-src 'self' 'unsafe-inline'")
	w.Header().Set("X-Frame-Options", "DENY")
	setHSTS(w, r)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Coordinator Authorization &mdash; Aequitas</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0A0E1A;color:#C9A84C;font-family:'Courier New',monospace;display:flex;align-items:center;justify-content:center;min-height:100vh;padding:20px}
.box{background:#111827;border:1px solid #1E2D45;border-radius:12px;padding:32px;max-width:560px;width:100%}
.logo{font-size:1.6rem;font-weight:900;letter-spacing:6px;color:#C9A84C;margin-bottom:18px;text-align:center}
.sub{color:#6B7A99;font-size:0.78rem;line-height:1.8;margin-bottom:18px}
label{display:block;color:#C9A84C;font-size:0.72rem;margin-bottom:6px;margin-top:14px}
input{width:100%;background:#0A0E1A;border:1px solid #1E2D45;border-radius:6px;color:#fff;padding:10px;font-family:'Courier New',monospace;font-size:0.8rem}
.btn{display:block;width:100%;margin-top:18px;padding:12px;background:#C9A84C;color:#0A0E1A;border:none;border-radius:8px;font-weight:bold;cursor:pointer;font-family:'Courier New',monospace}
.btn:disabled{opacity:0.5;cursor:not-allowed}
.out{margin-top:18px;padding:14px;background:#0A0E1A;border:1px solid #22C55E;border-radius:8px;word-break:break-all;font-size:0.72rem;color:#9CA3AF;line-height:1.9;display:none}
.err{margin-top:18px;padding:14px;background:#0A0E1A;border:1px solid #f87171;border-radius:8px;font-size:0.75rem;color:#f87171;display:none}
.hl{color:#C9A84C;font-weight:bold}
.note{margin-top:16px;font-size:0.7rem;color:#6B7A99;line-height:1.8;border-left:2px solid #1E2D45;padding-left:12px}
</style>
</head>
<body>
<div class="box">
<div class="logo">AEQUITAS</div>
<div class="sub">
This authorizes your coordinator&rsquo;s signing key with your <span class="hl">human wallet</span>.
Until it is authorized, matching services refuse every attestation it issues &mdash; they report it as an unknown key.
Signing here costs nothing and moves nothing: it is <span class="hl">personal_sign</span> over a plain sentence, not a transaction.
</div>
<label>Your coordinator&rsquo;s public key (<code>attestation_public_key</code> from <code>GET /inventory</code> on your own coordinator)</label>
<input id="pubKey" placeholder="64 hex characters">
<button class="btn" id="connectBtn">Connect Wallet &amp; Sign</button>
<div class="out" id="out"></div>
<div class="err" id="err"></div>
<div class="note">
Any wallet works as long as it is a <span class="hl">registered human</span> &mdash; it does not have to be the one that operates a node.
Never paste your coordinator&rsquo;s signing key into this or any other page: it stays on your own host, and your coordinator proves possession of it there.
</div>
</div>
<script src="/coordinator-binding.js"></script>
</body>
</html>`)
}

func (a *APIServer) handleCoordinatorBindingJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprint(w, coordinatorBindingJS)
}

// ─── GUARDIAN ENDPOINTS ────────────────────────────────────────────────────────

// handleSetGuardian POST /api/set-guardian
// Body: {"wallet":"0x...","guardian":"0x...","signature":"0x..."}
// Signature must be personal_sign("Aequitas: set guardian {guardian_address}", wallet_key).
func (a *APIServer) handleSetGuardian(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	// FIX (audit 2026-06-29): every other signature-verification POST
	// endpoint in this file (handleRecoverEscrow, handleProveProxy,
	// handleCheckRegistrationByBioHash) rate-limits per IP — this one and
	// handleConfirmAlive below were the only two that didn't, despite doing
	// the same ecrecover + DB-write shape of work. Same package-level
	// registerRateLimit sync.Map, same 30s-per-IP window, keyed separately
	// from the other endpoints so it can't be used to also throttle them.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("set-guardian:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 30*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store("set-guardian:"+ip, time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Wallet    string `json:"wallet"`
		Guardian  string `json:"guardian"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, 400)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	guardian := strings.ToLower(strings.TrimSpace(req.Guardian))
	if !isValidWalletAddr(wallet) || !isValidWalletAddr(guardian) {
		http.Error(w, `{"error":"invalid wallet or guardian address"}`, 400)
		return
	}
	// Verify signature: wallet signs "Aequitas: set guardian {guardian_address}"
	msg := "Aequitas: set guardian " + guardian
	if err := verifyPersonalSign(msg, req.Signature, wallet); err != nil {
		jsonError(w, "invalid signature: "+err.Error(), 400)
		return
	}
	now := time.Now().Unix()
	if err := a.state.SetGuardian(wallet, guardian); err != nil {
		jsonStateError(w, "set-guardian", wallet, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"wallet":   wallet,
		"guardian": guardian,
		"set_at":   now,
	})
}

// handleConfirmAlive POST /api/confirm-alive
// Body: {"wallet":"0x...","signature":"0x..."}
// Caller must be the guardian of wallet.
// Signature = personal_sign("Aequitas: confirm alive {wallet_address}", guardian_key).
func (a *APIServer) handleConfirmAlive(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	// FIX (audit 2026-06-29): see handleSetGuardian's matching comment —
	// this endpoint did the same ecrecover + DB-write work with no rate
	// limiting at all, unlike every comparable signature-verification POST
	// endpoint elsewhere in this file.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("confirm-alive:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 30*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store("confirm-alive:"+ip, time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Wallet    string `json:"wallet"`
		Signature string `json:"signature"`
		Guardian  string `json:"guardian"` // FIX 9: optional client-supplied guardian for early mismatch detection
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, 400)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if !isValidWalletAddr(wallet) {
		http.Error(w, `{"error":"invalid wallet address"}`, 400)
		return
	}
	// FIX 3: Look up guardian from DB first, then immediately verify the
	// signature using that address before passing it into ConfirmAlive.
	// ConfirmAlive re-fetches under its own lock to close the TOCTOU window.
	guardianAddr, _, err := a.state.GetGuardian(wallet)
	if err != nil || guardianAddr == "" {
		http.Error(w, `{"error":"no guardian set for this wallet"}`, 404)
		return
	}
	guardianAddr = strings.ToLower(guardianAddr)
	// FIX 9: Defense-in-depth — if client supplied a guardian address, check it
	// matches the DB value before doing any signature work.
	if req.Guardian != "" && strings.ToLower(strings.TrimSpace(req.Guardian)) != guardianAddr {
		jsonError(w, "guardian address mismatch", 400)
		return
	}
	// Signature is by the guardian.
	msg := "Aequitas: confirm alive " + wallet
	if sigErr := verifyPersonalSign(msg, req.Signature, guardianAddr); sigErr != nil {
		jsonError(w, "invalid guardian signature: "+sigErr.Error(), 400)
		return
	}
	// FIX 3 (cont.): pass guardianAddr so ConfirmAlive can re-verify under lock.
	if confirmErr := a.state.ConfirmAlive(wallet, guardianAddr); confirmErr != nil {
		jsonError(w, confirmErr.Error(), 400)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"wallet":    wallet,
		"guardian":  guardianAddr,
		"confirmed": time.Now().Unix(),
	})
}

// handleGetGuardian GET /api/guardian?wallet=0x...
// Returns {"wallet":"0x...","guardian":"0x...","set_at":timestamp} or 404.
func (a *APIServer) handleGetGuardian(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if !isValidWalletAddr(wallet) {
		http.Error(w, `{"error":"invalid wallet address"}`, 400)
		return
	}
	guardian, setAt, err := a.state.GetGuardian(wallet)
	if err != nil || guardian == "" {
		http.Error(w, `{"error":"no guardian found"}`, 404)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet":   wallet,
		"guardian": strings.ToLower(guardian),
		"set_at":   setAt,
	})
}

// ─── ESCROW ENDPOINTS ─────────────────────────────────────────────────────────

// handleGetEscrow GET /api/escrow?wallet=0x...
// Returns escrow amount and moved_at timestamp, or 404 if no escrow.
func (a *APIServer) handleGetEscrow(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if !isValidWalletAddr(wallet) {
		http.Error(w, `{"error":"invalid wallet address"}`, 400)
		return
	}
	amount, movedAt, err := a.state.GetEscrow(wallet)
	if err != nil || amount == 0 {
		http.Error(w, `{"error":"no escrow found for this wallet"}`, 404)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet":   wallet,
		"amount":   amount,
		"moved_at": movedAt,
	})
}

// handleRecoverEscrow POST /api/recover-escrow
// Body: {"wallet":"0x...","signature":"0x..."}
// Signature = personal_sign("Aequitas: recover escrow {wallet_address}", wallet_key).
func (a *APIServer) handleRecoverEscrow(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, `{"error":"POST required"}`, 405)
		return
	}
	// FIX 5: IP-based rate limiting — reuse the package-level registerRateLimit
	// sync.Map so escrow recovery cannot be hammered faster than once per 30s per IP.
	// Use clientIP(r) helper to correctly handle X-Forwarded-For from Railway's proxy.
	// FIX (P3, beta-launch audit 2026-07-05): this used the bare IP as the map
	// key, same as handleRegister's own rate limit (register.go) — the two
	// endpoints shared one cooldown window instead of two independent ones
	// (calling /api/register then immediately /api/recover-escrow from the
	// same IP would hit this endpoint's limiter already "warmed up" by the
	// other call). Prefixed, matching every other endpoint on this map
	// (set-guardian:, confirm-alive:, etc).
	ip := "recover-escrow:" + clientIP(r)
	if ts, loaded := registerRateLimit.Load(ip); loaded {
		if time.Since(ts.(time.Time)) < 30*time.Second {
			jsonError(w, "rate limited, try again shortly", 429)
			return
		}
	}
	registerRateLimit.Store(ip, time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Wallet    string `json:"wallet"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, 400)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if !isValidWalletAddr(wallet) {
		http.Error(w, `{"error":"invalid wallet address"}`, 400)
		return
	}
	msg := "Aequitas: recover escrow " + wallet
	if err := verifyPersonalSign(msg, req.Signature, wallet); err != nil {
		jsonError(w, "invalid signature: "+err.Error(), 400)
		return
	}
	if err := a.state.RecoverFromEscrow(wallet); err != nil {
		jsonStateError(w, "recover-escrow", wallet, err)
		return
	}
	newBalance := a.state.GetBalance(wallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"wallet":      wallet,
		"new_balance": newBalance,
	})
}
