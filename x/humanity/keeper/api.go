package keeper

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type APIServer struct {
	blockchain        *BlockDAG
	p2pNode           *P2PNode
	keeper            *Keeper
	startTime         time.Time
	proofServerStatus map[string]interface{}
	proofStatusMu     sync.RWMutex
	state             *ChainState
	// Shared EVM RPC server — one instance so all registration calls share
	// the same nonce map and mutex, preventing parallel registrations from
	// reading the same DB nonce and writing the same follower value.
	evmRPC *EVMRPCServer
}

func NewAPIServer(bc *BlockDAG, p2p *P2PNode, k *Keeper, state *ChainState) *APIServer {
	s := &APIServer{
		blockchain:        bc,
		p2pNode:           p2p,
		keeper:            k,
		startTime:         time.Now(),
		proofServerStatus: map[string]interface{}{},
		state:             state,
		evmRPC:            NewEVMRPCServer(bc, state),
	}
	go s.syncProofServerStatus()
	// FIX (audit 2026-06-28 recheck 4, P1-5): periodically retry any queued
	// proof-server bio_hash sync failures (see proof_server_sync_queue's
	// table comment in state.go and notifyProofServerWithRetryQueue in
	// register.go) — without this, a registration whose initial sync
	// attempt failed would stay queued forever with nothing ever
	// re-attempting it.
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			RetryProofServerSyncQueue(state)
		}
	}()
	// FIX (audit 2026-06-28 recheck 4, P1-6): same retry pattern as the
	// proof-server sync queue above, for EVM mirror slot-write failures —
	// see syncBalanceLocked's comment in evm_storage.go.
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			RetryEVMMirrorSyncQueue(state)
		}
	}()
	// FIX (BRUTAL-P1-01): retry registration_recovery records — EVM-committed
	// registrations whose Go-state sync failed. Runs every 5 minutes; on each
	// pass it calls RegisterHumanAtomic for every unrecovered record and marks
	// them recovered when they succeed or when Go-state already has the wallet
	// (meaning a previous pass or a peer-sync block replay already fixed it).
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			if n := state.RetryRegistrationRecoveries(); n > 0 {
				fmt.Printf("[RECOVERY] Recovered %d registration(s) from registration_recovery table\n", n)
			}
		}
	}()
	return s
}

// jsonError writes a properly JSON-marshaled error response, preventing JSON
// injection via concatenated error strings that may contain quote characters.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(enc)
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

// requireProofServerConfigured writes a clear 503 and returns ok=false if
// PROOF_SERVER_URL isn't set, so callers can bail out before constructing a
// request against an empty base URL (http.NewRequest with a schemeless,
// hostless URL like "/prove" fails, and the discarded error from that would
// otherwise nil-panic on the very next line that sets a header on the
// request).
func requireProofServerConfigured(w http.ResponseWriter) (string, bool) {
	base := proofServerBaseURL()
	if base == "" {
		http.Error(w, `{"error":"PROOF_SERVER_URL not configured on this node"}`, 503)
		return "", false
	}
	return base, true
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
func proofProxyClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DialContext: pinningDialer},
	}
}

func (a *APIServer) syncProofServerStatus() {
	for {
		base := proofServerBaseURL()
		if base == "" {
			time.Sleep(30 * time.Second)
			continue
		}
		proofHTTP := proofProxyClient(8 * time.Second)
		resp, err := proofHTTP.Get(base + "/health")
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
func (a *APIServer) handleCombinedHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	// instead of letting it run invisibly. Not an error by itself (it self-heals
	// as real blocks sync in behind it) — informational unless it persists.
	syntheticCheckpointCount := a.blockchain.SyntheticCheckpointCount()
	checkpointTrustMode := syntheticCheckpointCount > 0
	if checkpointTrustMode && status == "healthy" {
		status = "warn"
		notes = append(notes, fmt.Sprintf("%d synthetic-checkpoint stub(s) active — bridging a historical gap from peer snapshot trust, not full verification; normally self-heals as real blocks sync in behind them", syntheticCheckpointCount))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"chain": map[string]interface{}{
			"status":       status,
			"notes":        notes,
			"healthy":      status == "healthy", // kept for backward compatibility with existing callers
			"degraded_reason": degradedReason,
			"dag_degraded": dagDegraded,
			"dag_degraded_reason": dagDegradedReason,
			"checkpoint_trust_mode": checkpointTrustMode,
			"synthetic_checkpoint_count": syntheticCheckpointCount,
			"height":       latest.Height,
			"dag_tips_count": a.blockchain.TipsCount(),
			"state_root_mismatch_count": mismatchCount,
			"last_successful_peer_sync_age_secs": lastSyncAgeSecs,
			"total_humans": a.state.TotalHumans(),
			"total_supply": fmt.Sprintf("%.2f AEQ", a.state.TotalSupply()),
			"uptime_secs":  int64(time.Since(a.startTime).Seconds()),
			"destructive_flags_set": destructiveFlagsSet,
			"registration_recovery_pending": registrationRecoveryCount,
			// FIX (audit 2026-06-28 recheck 5, P2-3): "Health/Debug sollte
			// Chain-Nullifier, Chain-BioHash und Proof-BioHash getrennt
			// anzeigen." proof_server.last_status.bio_hash_count (below) is
			// the proof-server side of this comparison.
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
			"reachable": len(proofStatus) > 0,
			"last_status": proofStatus,
		},
	})
}

func (a *APIServer) Start(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/landing", a.handleLanding)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Root path: serve landing page; anything else falls to handleUI
		if r.URL.Path == "/" {
			a.handleLanding(w, r)
			return
		}
		a.handleUI(w, r)
	})
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/health/combined", a.handleCombinedHealth)
	mux.HandleFunc("/api/blocks", a.handleBlocks)
	mux.HandleFunc("/api/block", a.handleBlockByHash)
	mux.HandleFunc("/api/blocks/by-hash", a.handleBlocksByHash)
	mux.HandleFunc("/api/blocks/push", a.handleBlockPush)
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
	mux.HandleFunc("/api/snapshot", a.handleSnapshot)
	mux.HandleFunc("/api/gini/history", a.handleGiniHistory)
	mux.HandleFunc("/api/price-history", a.handlePriceHistory)
	mux.HandleFunc("/api/wealth-cap", a.handleWealthCap)
	mux.HandleFunc("/api/sign-validator-challenge", a.handleSignValidatorChallenge)
	mux.HandleFunc("/api/nonce", a.handleNonce)
	mux.HandleFunc("/api/peers", a.handlePeers)
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
	mux.HandleFunc("/api/register-validator-key", a.handleRegisterValidatorKey)
	mux.HandleFunc("/api/set-guardian", a.handleSetGuardian)
	mux.HandleFunc("/api/confirm-alive", a.handleConfirmAlive)
	mux.HandleFunc("/api/guardian", a.handleGetGuardian)
	mux.HandleFunc("/api/escrow", a.handleGetEscrow)
	mux.HandleFunc("/api/recover-escrow", a.handleRecoverEscrow)
	mux.HandleFunc("/registered", a.handleRegistered)
	mux.HandleFunc("/download/app.apk", a.handleAppDownload)
	for _, lg := range []string{"en", "de", "es", "fr", "id", "it", "pt", "tr"} {
		lg := lg
		up := strings.ToUpper(lg)
		mux.HandleFunc("/download/node-guide-"+lg+".pdf", func(w http.ResponseWriter, r *http.Request) {
			a.handleStaticDownload(w, r, "downloads/Aequitas_Node_Guide_"+up+".pdf", "Aequitas_Node_Guide_"+up+".pdf", "application/pdf")
		})
	}
	fmt.Println("── Starting EVM RPC ─────────────────────")
	// Use the shared EVMRPCServer (a.evmRPC) so /rpc and /api/register share
	// one nonce map + mutex — creating a second instance here caused separate
	// nonce maps, making the atomic nonce reservation ineffective.
	mux.HandleFunc("/rpc", a.evmRPC.handleRPC)
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
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[API] Server error: %v\n", err)
		}
	}()
}

func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	latest := a.blockchain.LatestBlock()
	uptime := int64(time.Since(a.startTime).Seconds())
	// Use a.state (PostgreSQL-backed ChainState) as the single source of
	// truth for human count, not a.keeper — the in-memory Keeper map is never
	// persisted and resets to 0 on every restart, which previously made this
	// "growth" figure silently diverge from total_humans below (which already
	// correctly used a.state).
	humans := a.state.TotalHumans()
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
		"height":       latest.Height,
		"latest_hash":  latest.Hash,
		"total_humans": a.state.TotalHumans(),
		"total_supply": fmt.Sprintf("%.2f AEQ", a.state.TotalSupply()),
		"node_id":      a.p2pNode.GetNodeID(),
		"uptime":       uptime,
		"is_primary":   os.Getenv("IS_PRIMARY_NODE") == "true",
		"block_time":   1,
		"contract_v7":  V7_CONTRACT_ADDR,
		// P3-8: V5/V6 legacy addresses removed from status — minimise attack surface.
		"bio_verifier": BIO_VERIFIER_ADDR,
		"chain_evm_id": 1926,
		"index":        a.state.CalcAequitasIndex(),
		"gini":         a.state.CalcGini(),
		"growth":       growth,
		"velocity":     50,
		"phase":        a.state.CalcPhase(),
		"fee_bps":      10,
		// P2-FIX: use the pool address constants from state.go instead of duplicating
		// the raw strings here. If addresses ever change, only one place needs updating.
		"pool_validators":      fmt.Sprintf("%.4f", a.state.GetBalance(validatorsPoolAddr)),
		"pool_lp":              fmt.Sprintf("%.4f", a.state.GetBalance(lpPoolAddr)),
		"pool_ubi":             fmt.Sprintf("%.4f", a.state.GetBalance(ubiPoolAddr)),
		"pool_treasury":        fmt.Sprintf("%.4f", a.state.GetBalance(treasuryPoolAddr)),
		"ubi_next_payout_secs": nextUBISecs,
	})
}

func (a *APIServer) handleBlocks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	blocks := a.blockchain.GetBlocks()
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
		afterHash := r.URL.Query().Get("after_hash")
		result := make([]*Block, 0, limit)
		if afterHash == "" {
			// No cursor — original behavior: blocks with Height > minHeight.
			for _, b := range blocks {
				if b.Height > minHeight {
					result = append(result, b)
					if len(result) >= limit {
						break
					}
				}
			}
		} else {
			// Cursor mode: return all blocks that come AFTER the block
			// identified by (minHeight, afterHash) in canonical order
			// (height ASC, blueScore DESC, hash ASC).
			// Scan through the sorted list; start collecting once we pass
			// the cursor position (the cursor block itself is excluded).
			pastCursor := false
			for _, b := range blocks {
				if !pastCursor {
					if b.Height > minHeight {
						// Past the cursor height — include from here.
						pastCursor = true
						result = append(result, b)
					} else if b.Height == minHeight && b.Hash == afterHash {
						// Found the cursor block — include everything after it.
						pastCursor = true
					}
					// blocks with Height < minHeight or Height == minHeight
					// but Hash != afterHash (and not yet past cursor) are skipped.
				} else {
					result = append(result, b)
				}
				if len(result) >= limit {
					break
				}
			}
		}
		json.NewEncoder(w).Encode(result)
		return
	}

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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Hashes []string `json:"hashes"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil || json.Unmarshal(body, &req) != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if len(req.Hashes) > maxBlocksByHashPerRequest {
		req.Hashes = req.Hashes[:maxBlocksByHashPerRequest]
	}
	found := make([]*Block, 0, len(req.Hashes))
	for _, h := range req.Hashes {
		// Reject non-hex or wrong-length strings before touching any index —
		// prevents garbage strings from making it into GetBlockByHash lookups.
		if len(h) != 64 {
			continue
		}
		if _, hexErr := hex.DecodeString(h); hexErr != nil {
			continue
		}
		// FIX (2026-06-30, confirmed live in production): never hand a
		// synthetic-checkpoint stub to a peer — see GetBlocks' identical
		// fix/comment. fetchMissingAncestors (this endpoint's caller) needs
		// to know "no node has the real block" so its own abandonment/bridge
		// logic can take over, not receive a placeholder that can never pass
		// the requester's own hash-mismatch check.
		if b := a.blockchain.GetBlockByHash(h); b != nil && b.Proposer != "synthetic-checkpoint" {
			found = append(found, b)
		}
	}
	json.NewEncoder(w).Encode(found)
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
)

var (
	blockPushBreakerMu    sync.Mutex
	blockPushFailRun      = map[string]int{}   // source IP -> consecutive non-attaching pushes
	blockPushBreakerUntil = map[string]int64{} // source IP -> unix-nano its cooldown expires
	lastBlockPushDropLog  atomic.Int64         // rate-limits the drop log to once/sec

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
// circuit breaker still inside its cooldown. Touches only blockPushBreakerMu,
// never dag.mu.
func blockPushShouldDrop(ip string) bool {
	if blockPushIPDenylist[ip] {
		return true
	}
	blockPushBreakerMu.Lock()
	defer blockPushBreakerMu.Unlock()
	until, ok := blockPushBreakerUntil[ip]
	if !ok {
		return false
	}
	if time.Now().UnixNano() >= until {
		delete(blockPushBreakerUntil, ip)
		// Single real probe, not a full reopen — see proposerBlockBlocked's
		// identical fix (block.go) for the full rationale. Seeding one short
		// of the threshold means the next outcome either clears it (success)
		// or immediately re-trips (failure), instead of reopening fully
		// until blockPushBreakerThreshold fresh failures rebuild from zero.
		blockPushFailRun[ip] = blockPushBreakerThreshold - 1
		return false
	}
	return true
}

// blockPushRecordOutcome feeds the per-IP breaker after AddPeerBlock returns.
// attached (the block joined the DAG) clears the IP's failure run; !attached
// (orphaned, rejected, or a re-push of a known block) advances it and trips the
// breaker once the run crosses blockPushBreakerThreshold.
func blockPushRecordOutcome(ip string, attached bool) {
	blockPushBreakerMu.Lock()
	defer blockPushBreakerMu.Unlock()
	if attached {
		delete(blockPushFailRun, ip)
		delete(blockPushBreakerUntil, ip)
		return
	}
	blockPushFailRun[ip]++
	if blockPushFailRun[ip] >= blockPushBreakerThreshold {
		blockPushBreakerUntil[ip] = time.Now().Add(blockPushBreakerCooldown).UnixNano()
		delete(blockPushFailRun, ip) // breaker gates now; rebuild the run after cooldown
	}
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
		w.Write([]byte(`{"ok":false,"reason":"push flood shield open","action":"resync_required"}`))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<10))
	if err != nil {
		http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
		return
	}
	var block Block
	if err := json.Unmarshal(body, &block); err != nil {
		http.Error(w, `{"error":"invalid block JSON"}`, http.StatusBadRequest)
		return
	}
	block.FromSync = true
	accepted := a.blockchain.AddPeerBlock(&block)
	blockPushRecordOutcome(ip, accepted)
	if accepted {
		fmt.Printf("[BLOCK-PUSH] ✓ Accepted block #%d via HTTP push\n", block.Height)
		w.Write([]byte(`{"ok":true}`))
	} else {
		// Not an error — block may already be known (idempotent). Only signal
		// resync_required when the rejection is unambiguous (this proposer's
		// own breaker is open) — an ordinary duplicate (arrived via P2P first)
		// must never trigger it.
		if a.blockchain.proposerBlockBlocked(block.Proposer) {
			w.Write([]byte(`{"ok":false,"reason":"proposer circuit breaker open","action":"resync_required"}`))
		} else {
			w.Write([]byte(`{"ok":false,"reason":"rejected or already known"}`))
		}
	}
}

func (a *APIServer) handleHumans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	accounts := a.state.GetAllAccounts()
	// Read pool state once, not per-account, so this scales to millions of
	// humans. Each human's LP value is their ownership fraction of the pool
	// reserves — surfaced alongside the liquid balance so a human who added
	// all their AEQ as liquidity doesn't appear to hold nothing.
	reserveAEQ, reserveTUSD, totalLPShares := a.state.GetPoolSnapshot()
	humans := []map[string]interface{}{}
	for _, acc := range accounts {
		if acc.IsHuman {
			liquid := effectiveBalance(acc).Float()
			lpShares := acc.LPShares.Float()
			var lpValueAEQ, lpValueTUSD float64
			if totalLPShares > 0 && lpShares > 0 {
				ownership := lpShares / totalLPShares
				lpValueAEQ = ownership * reserveAEQ
				lpValueTUSD = ownership * reserveTUSD
			}
			humans = append(humans, map[string]interface{}{
				"address": acc.Address,
				// Use effectiveBalance so the Lorenz curve and Score tab show the same Gini.
				// Raw acc.Balance ignores demurrage decay → different Gini than CalcGini().
				"balance":         liquid,
				"lp_shares":       lpShares,
				"lp_value_aeq":    lpValueAEQ,
				"lp_value_tusd":   lpValueTUSD,
				"total_value_aeq": liquid + lpValueAEQ,
			})
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":  len(humans),
		"humans": humans,
	})
}

func (a *APIServer) handleSepoliaHumans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	base, ok := requireProofServerConfigured(w)
	if !ok {
		return
	}
	proofHTTP2 := proofProxyClient(8 * time.Second)
	proofReq, _ := http.NewRequest("GET", base+"/humans", nil)
	addProofServerAuth(proofReq)
	resp, err := proofHTTP2.Do(proofReq)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	// own registration status), but it returns a wallet address linked to a
	// supplied bioHash with no throttle at all — unlike every other
	// bioHash/escrow-adjacent endpoint in this file. Reuse the same
	// package-level rate limiter as handleRecoverEscrow so it can't be used
	// to mass-probe bioHash values for wallet-address leakage.
	//
	// CONSIDERED (Gesamtaudit 2026-06-28, P2-5) and deliberately NOT changed
	// further: the audit's suggested mitigations (strip wallet/balance from
	// the response; require a signature proving wallet ownership) both
	// conflict with how the shipped AequitasBio app actually uses this
	// endpoint (App.tsx) — it's called specifically to RECOVER "which wallet
	// is this biometric registered to" before any wallet is connected (no
	// signature is available to require at that point), including by
	// polling it every 3 seconds while waiting for registration to confirm.
	// Stripping the wallet would break that recovery flow entirely;
	// tightening the rate limit below 3s would make the existing polling
	// noticeably less responsive (it already loses every other poll to this
	// 5s window). bioHash is therefore the deliberate credential this
	// endpoint trusts, same as the rest of this architecture's proof/dedupe
	// design — the real mitigation already in place is POST-only (so
	// bioHash never lands in a URL, and therefore never in server/proxy
	// logs) plus this rate limit, not response minimization.
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
	json.NewDecoder(r.Body).Decode(&bioHashBody)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
		decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		balanceFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(weiInt), decimals).Float64()
		balance = balanceFloat
	}

	return balance, isHuman
}

func (a *APIServer) handleRegistered(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// FIX 10: Add Content-Security-Policy to prevent XSS escalation on this HTML page.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self'; img-src 'self' data:")
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
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
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
<button class="btn" id="connectBtn" onclick="signBinding()">Connect Wallet &amp; Sign</button>
<div class="out" id="out"></div>
<div class="err" id="err"></div>
</div>
<script>
async function signBinding() {
  const errEl = document.getElementById('err');
  const outEl = document.getElementById('out');
  errEl.style.display = 'none';
  outEl.style.display = 'none';
  const signingAddr = document.getElementById('signingAddr').value.trim().toLowerCase();
  if (!/^0x[0-9a-f]{40}$/.test(signingAddr)) {
    errEl.textContent = 'Enter a valid signing address (0x followed by 40 hex characters).';
    errEl.style.display = 'block';
    return;
  }
  if (!window.ethereum) {
    errEl.textContent = 'No wallet found. Install MetaMask or another browser wallet extension.';
    errEl.style.display = 'block';
    return;
  }
  try {
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    const wallet = accounts[0];
    const message = 'Aequitas: authorize validator ' + signingAddr;
    const signature = await window.ethereum.request({
      method: 'personal_sign',
      params: [message, wallet],
    });
    outEl.innerHTML = 'Wallet: <span class="hl">' + wallet + '</span><br><br>' +
      'Set these on your node:<br><br>' +
      'NODE_OPERATOR_WALLET=' + wallet + '<br>' +
      'NODE_OPERATOR_BINDING_SIGNATURE=' + signature;
    outEl.style.display = 'block';
  } catch (e) {
    errEl.textContent = 'Signing failed or was rejected: ' + (e && e.message ? e.message : e);
    errEl.style.display = 'block';
  }
}
</script>
</body>
</html>`)
}

func (a *APIServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	// script-src must include unpkg.com: the price chart loads TradingView's
	// lightweight-charts library from there (api_html.go). Without it the CSP
	// silently blocks the script, window.LightweightCharts stays undefined,
	// initPriceChart() bails, and the price chart renders as an empty box.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://unpkg.com; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self' https://aequitas.digital; img-src 'self' data:")
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
	fmt.Fprint(w, explorerHTML)
}

// handleNonce returns the next swap nonce a wallet should sign with.
// GET /api/nonce?wallet=0x...
func (a *APIServer) handleNonce(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
func (a *APIServer) handlePriceHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	minutes := 240
	limit := 1000
	fmt.Sscanf(r.URL.Query().Get("minutes"), "%d", &minutes)
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	// Clamp to prevent memory exhaustion from large DB reads.
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 43200 {
		minutes = 43200
	} // max 30 days
	if limit < 1 {
		limit = 1
	}
	if limit > 5000 {
		limit = 5000
	}
	history := a.state.GetPriceHistory(minutes, limit)
	json.NewEncoder(w).Encode(map[string]interface{}{"history": history, "count": len(history)})
}

// handleGiniHistory returns Gini snapshots stored after each UBI distribution.
// Falls back to the current Gini as a single point when no history exists yet.
func (a *APIServer) handleGiniHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
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
//   human_signature:      personal_sign("Aequitas: authorize validator {signing_address}", human_wallet)
//   signing_key_signature: personal_sign("Aequitas: validator key linked to human {human_wallet}", signing_address)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	if err := a.state.RegisterValidatorKey(signingAddr, humanWallet); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	a.blockchain.AddAuthorizedValidator(signingAddr)
	fmt.Printf("[VALIDATOR] ✓ Registered key %s for human %s\n", signingAddr, humanWallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"signing_address": signingAddr,
		"human_wallet":    humanWallet,
	})
}

// handleValidatorList returns registered validator key pairs (signing_address +
// human_wallet) so peer nodes can verify credentials before trusting an address.
// Peers check IsHuman(human_wallet) locally before calling AddAuthorizedValidator.
// See syncValidatorsFromPeer in sync_blocks.go for the receiver-side verification.
func (a *APIServer) handleValidatorList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"validators": a.blockchain.ValidatorKeyPairs(),
	})
}

// handlePeerChallenge issues a one-time challenge that the peer must sign to
// prove ownership of their signing key (P1-3 validator signature verification).
// GET /api/peers/challenge?address=0x...
func (a *APIServer) handlePeerChallenge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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

// handlePeerRegister accepts a node registration and returns the current peer
// list plus all authorized validator addresses. A node that sends its
// signing_address is automatically added to the authorized validator set so
// its blocks are accepted without manual AUTHORIZED_VALIDATORS configuration.
// POST /api/peers/register  body: {"url":"https://...","signing_address":"0x...","signature":"0x..."}
func (a *APIServer) handlePeerRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var req struct {
		URL                string `json:"url"`
		SigningAddress     string `json:"signing_address"`
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

	// FIX (P0, 2026-07-02): a proposer whose blocks are being cleanly
	// rejected by the per-proposer circuit breaker (block.go, AddPeerBlock)
	// was still free to hammer this endpoint at will — re-authenticating
	// costs a full signature verification plus a BindValidatorSlot DB write
	// regardless of whether its blocks ever attach. Reuse the SAME breaker
	// state here: "authorized validator = yes, but only while it's actually
	// syncing correctly" is exactly proposerBlockBlocked's model already —
	// keyed on signing address (not IP, so it can't be sidestepped by
	// redeploying from a new host, but also never a permanent ban), and it
	// self-clears the moment the proposer produces an attaching block again
	// (recordProposerOutcome), so a diverged operator who fixes their node
	// and resyncs is let back in automatically, no manual action needed.
	if addr := strings.ToLower(strings.TrimSpace(req.SigningAddress)); addr != "" && a.blockchain.proposerBlockBlocked(addr) {
		fmt.Printf("[PEERS] ✗ Registration from %s skipped — its circuit breaker is open (producing non-attaching blocks); will accept again once it syncs correctly.\n", addr)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"ok":false,"reason":"proposer circuit breaker open — resync before re-registering"}`))
		return
	}

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
				GlobalPeerRegistry.Register(req.URL)
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
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.AllPeers(), "validators": validators})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.AllPeers()})
		}
		return
	}
	// No (valid-looking) signing address in this request — URL registration
	// can only be authorized via the PEER_SECRET bypass here.
	registerURLIfAuthorized()
	json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.AllPeers(), "validators": []string{}})
}

// handleProveProxy proxies POST /api/prove to the proof server backend-side,
// bypassing browser CORS restrictions. The proof server does not include
// Access-Control-Allow-Origin, so browser fetches fail. By proxying through
// the chain node (same origin as the website), CORS is not an issue.
func (a *APIServer) handleProveProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
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
	base, ok := requireProofServerConfigured(w)
	if !ok {
		return
	}
	// Add CHAIN_SERVICE_TOKEN so the proof server's auth check passes.
	// The token lives only in the chain node's env var and is never exposed to
	// browser clients — the proxy is the sole caller of the proof server.
	proofReq, _ := http.NewRequest("POST", base+"/prove", bytes.NewReader(body))
	proofReq.Header.Set("Content-Type", "application/json")
	addProofServerAuth(proofReq)
	resp, err := proofProxyClient(120 * time.Second).Do(proofReq)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleProveGetProxy proxies GET /api/prove/get/{id} to the proof server.
func (a *APIServer) handleProveGetProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := strings.TrimPrefix(r.URL.Path, "/api/prove/get/")
	// FIX 6: strict allowlist replaces denylist -- prevents path traversal.
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{1,64}$`, id)
	if !matched {
		http.Error(w, `{"error":"invalid proof id"}`, 400)
		return
	}
	base, ok := requireProofServerConfigured(w)
	if !ok {
		return
	}
	getReq, _ := http.NewRequest("GET", base+"/get/"+id, nil)
	addProofServerAuth(getReq)
	resp, err := proofProxyClient(30 * time.Second).Do(getReq)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, `{"error":"read error"}`, 500)
		return
	}
	base, ok := requireProofServerConfigured(w)
	if !ok {
		return
	}
	proofReq, _ := http.NewRequest("POST", base+"/store", bytes.NewReader(body))
	proofReq.Header.Set("Content-Type", "application/json")
	addProofServerAuth(proofReq)
	resp, err := proofProxyClient(30 * time.Second).Do(proofReq)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		http.Error(w, `{"error":"read error"}`, 500)
		return
	}
	base, ok := requireProofServerConfigured(w)
	if !ok {
		return
	}
	proofReq, _ := http.NewRequest("POST", base+"/check", bytes.NewReader(body))
	proofReq.Header.Set("Content-Type", "application/json")
	addProofServerAuth(proofReq)
	resp, err := proofProxyClient(30 * time.Second).Do(proofReq)
	if err != nil {
		http.Error(w, `{"error":"proof server unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handlePeers returns the list of all known peer nodes.
// GET /api/peers
func (a *APIServer) handlePeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{"peers": GlobalPeerRegistry.AllPeers()})
}

// handleSigningAddress returns this node's signing address, protected by
// SNAPSHOT_TOKEN. Secondary node operators need this for BOOTSTRAP_SIGNER.
// Not exposed in /api/status to avoid leaking validator addresses publicly.
func (a *APIServer) handleSigningAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

func (a *APIServer) handleAppDownload(w http.ResponseWriter, r *http.Request) {
	const apkPath = "downloads/aequitas-app.apk"
	const fallbackURL = "https://github.com/hanoi96international-gif/Aequitas/raw/main/downloads/aequitas-app.apk"
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
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://fonts.bunny.net; font-src https://fonts.bunny.net; connect-src 'self'; img-src 'self' data:")
	fmt.Fprint(w, landingHTML)
}

// ─── GUARDIAN ENDPOINTS ────────────────────────────────────────────────────────

// handleSetGuardian POST /api/set-guardian
// Body: {"wallet":"0x...","guardian":"0x...","signature":"0x..."}
// Signature must be personal_sign("Aequitas: set guardian {guardian_address}", wallet_key).
func (a *APIServer) handleSetGuardian(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
		jsonError(w, err.Error(), 400)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	ip := clientIP(r)
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
		jsonError(w, err.Error(), 400)
		return
	}
	newBalance := a.state.GetBalance(wallet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"wallet":      wallet,
		"new_balance": newBalance,
	})
}
