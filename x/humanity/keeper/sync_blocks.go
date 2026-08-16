package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// ─── PEER REGISTRY ───────────────────────────────────────────────────────────

// PeerRegistry tracks known peer nodes with heartbeat timestamps.
//
// DOC FIX (audit 2026-08-14): this used to say "the primary node
// (IS_PRIMARY_NODE=true) collects registrations from secondary nodes". That
// has not been true for a long time and actively misleads — nothing in this
// file, or anywhere else, branches on IS_PRIMARY_NODE. EVERY node accepts
// /api/peers/register and serves /api/peers; "primary" is purely a matter of
// which URL other operators happen to have configured as their seed, not a
// role the code assigns or enforces. IS_PRIMARY_NODE today controls exactly
// two things, neither of them peer-related: the cosmetic "is_primary" field
// in /api/status, and the RESET_DB_STATE refusal in resetDBStateForBootstrap
// (state.go). Daily pool distribution is likewise decentralized — every node
// is eligible, deduplicated via TryLockDistribution's Postgres CAS (see
// main.go's own comment on the distribution round).
var GlobalPeerRegistry = &PeerRegistry{peers: make(map[string]time.Time)}

type PeerRegistry struct {
	mu    sync.RWMutex
	peers map[string]time.Time // URL → last heartbeat
}

func (pr *PeerRegistry) Register(url string) {
	if url == "" {
		return
	}
	url = strings.TrimRight(url, "/")
	pr.mu.Lock()
	pr.peers[url] = time.Now()
	pr.mu.Unlock()
}

// ActivePeers returns peers that sent a heartbeat in the last 5 minutes,
// excluding selfURL so a node never syncs with itself.
func (pr *PeerRegistry) ActivePeers(selfURL string) []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	self := strings.TrimRight(selfURL, "/")
	var result []string
	for url, lastSeen := range pr.peers {
		if time.Since(lastSeen) < 5*time.Minute && url != self {
			result = append(result, url)
		}
	}
	return result
}

func (pr *PeerRegistry) AllPeers() []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	result := make([]string, 0, len(pr.peers))
	for url := range pr.peers {
		result = append(result, url)
	}
	return result
}

// ─── BLOCK SYNC ──────────────────────────────────────────────────────────────

// pinningDialer resolves the hostname once, verifies all IPs are public,
// then connects directly to the first IP — bypassing any subsequent DNS
// re-resolution that DNS-rebinding attacks rely on.
func pinningDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// Literal IP: skip DNS entirely and connect directly.
	// net.LookupHost("173.x.x.x") can fail on Alpine/Docker even for valid
	// public IPs because the minimal resolver doesn't handle PTR/A lookups
	// for already-resolved addresses the same way on all platforms.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("connection to private/loopback IP rejected: %s", host)
		}
		d := &net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	// Hostname: resolve DNS and verify every IP is public (DNS-rebinding guard).
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS lookup failed for %s", host)
	}
	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsMulticast() || parsed.IsUnspecified() {
			return nil, fmt.Errorf("DNS resolved to private/loopback address %s for host %s", ip, host)
		}
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
}

var httpSyncClient = &http.Client{
	Timeout: 30 * time.Second,
	// Never follow redirects — a public URL could redirect internally after our check.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: pinningDialer,
	},
}

const maxSyncPeers = 20

// syncValidatorsFromPeer fetches /api/validators from peerURL and adds any
// previously-unknown addresses to this node's authorized-validator set.
//
// This is how validator registrations propagate across all nodes without
// requiring manual AUTHORIZED_VALIDATORS env-var maintenance: a new validator
// registers with ONE node (via /api/peers/register), and every other node
// that syncs from it — directly or transitively — learns about them here.
func (dag *BlockDAG) syncValidatorsFromPeer(peerURL string) {
	resp, err := httpSyncClient.Get(peerURL + "/api/validators")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	// P2-07: /api/validators has two historical formats:
	//   old nodes:  {"validators":["0xABCD...","0x1234..."]}  ([]string)
	//   new nodes:  {"validators":[{"signing_address":"0x...","human_wallet":"0x...","operator_binding_signature":"0x..."}]}
	// Try the new structured format first; fall back to the old string list.
	var result struct {
		Validators []ValidatorKeyPair `json:"validators"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil || (len(result.Validators) == 0) {
		// Try old format: {"validators":["0x..."]}
		var oldResult struct {
			Validators []string `json:"validators"`
		}
		if err2 := json.Unmarshal(bodyBytes, &oldResult); err2 == nil && len(oldResult.Validators) > 0 {
			fmt.Printf("[PEERS] ⚠ %s serves old validator list format ([]string) — upgrade recommended\n", peerURL)
			// P1-06 (audit): old format carries no binding signature or human_wallet,
			// so any address could be injected by a compromised peer. Only keep
			// addresses we already trust locally — never add new ones via this path.
			for _, addr := range oldResult.Validators {
				addr = strings.ToLower(strings.TrimSpace(addr))
				if !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
					continue
				}
				dag.mu.RLock()
				alreadyTrusted := dag.authorizedValidators[addr]
				dag.mu.RUnlock()
				if !alreadyTrusted {
					fmt.Printf("[PEERS] ⚠ Ignoring unverified legacy validator %s from %s — no binding signature\n", addr, peerURL)
				}
			}
			return
		}
		// Both decode attempts failed or body is empty — nothing to do.
		return
	}

	for _, vkp := range result.Validators {
		signingAddr := strings.ToLower(strings.TrimSpace(vkp.SigningAddress))
		humanWallet := strings.ToLower(strings.TrimSpace(vkp.HumanWallet))
		if !strings.HasPrefix(signingAddr, "0x") || len(signingAddr) != 42 {
			continue
		}
		if !strings.HasPrefix(humanWallet, "0x") || len(humanWallet) != 42 {
			continue
		}
		// P1-03: verify the operator_binding_signature before trusting this
		// signing address.  A malicious peer cannot pair an arbitrary signing
		// key with a known human wallet without also forging the human's
		// personal_sign over "Aequitas: authorize validator <signing_address>".
		// Nodes that predate P1-03 will send an empty signature — accept them
		// with a warning (backward compat) until all nodes are updated.
		if vkp.OperatorBindingSignature != "" {
			bindingMsg := "Aequitas: authorize validator " + signingAddr
			if err := verifyPersonalSign(bindingMsg, vkp.OperatorBindingSignature, humanWallet); err != nil {
				fmt.Printf("[PEERS] ✗ Rejected validator %s from %s: binding signature invalid (%v)\n", signingAddr, peerURL, err)
				continue
			}
		} else {
			// Binding signature absent — only accept if we already trust this
			// address (i.e. it was authorized via local registration), so we
			// don't silently downgrade security for new-to-us addresses.
			dag.mu.RLock()
			alreadyTrusted := dag.authorizedValidators[signingAddr]
			dag.mu.RUnlock()
			if !alreadyTrusted {
				fmt.Printf("[PEERS] ⚠ Skipping validator %s from %s: no binding signature (upgrade peer to propagate it)\n", signingAddr, peerURL)
				continue
			}
		}
		// Only add a signing address whose operator is a known registered human.
		// If the human hasn't registered here yet (registration TX not yet synced),
		// skip for now — the next sync cycle will retry once their TX propagates.
		if dag.state != nil && !dag.state.IsHuman(humanWallet) {
			fmt.Printf("[PEERS] Skipping validator %s: human_wallet %s not registered here yet\n", signingAddr, humanWallet)
			continue
		}
		dag.mu.RLock()
		already := dag.authorizedValidators[signingAddr]
		dag.mu.RUnlock()
		if !already {
			fmt.Printf("[PEERS] Auto-authorized validator from %s: %s (human: %s)\n", peerURL, signingAddr, humanWallet)
		}
		dag.AddAuthorizedValidator(signingAddr)
	}
}

// syncValidatorsFromAllPeers calls syncValidatorsFromPeer for every currently
// active sync peer. Called immediately when an unknown proposer is detected so
// the registration propagates within the current sync cycle rather than
// waiting up to validatorSyncInterval.
func (dag *BlockDAG) syncValidatorsFromAllPeers() {
	dag.syncPeerMu.Lock()
	peers := make([]string, 0, len(dag.activeSyncPeers))
	for p := range dag.activeSyncPeers {
		peers = append(peers, p)
	}
	dag.syncPeerMu.Unlock()
	for _, peer := range peers {
		dag.syncValidatorsFromPeer(peer)
	}
}

// startSyncForPeer starts a long-running syncWithNode goroutine for peerURL.
// No-op if already syncing that URL or if the peer cap is reached.
func (dag *BlockDAG) startSyncForPeer(peerURL string) {
	peerURL = strings.TrimRight(peerURL, "/")
	if !isAllowedPeerURL(peerURL) {
		fmt.Printf("[PEERS] Rejected peer URL (must be public HTTPS): %s\n", peerURL)
		return
	}
	dag.syncPeerMu.Lock()
	already := dag.activeSyncPeers[peerURL]
	tooMany := len(dag.activeSyncPeers) >= maxSyncPeers
	if !already && !tooMany {
		dag.activeSyncPeers[peerURL] = true
	}
	dag.syncPeerMu.Unlock()
	if already || tooMany {
		return
	}
	go func() {
		defer func() {
			dag.syncPeerMu.Lock()
			delete(dag.activeSyncPeers, peerURL)
			dag.syncPeerMu.Unlock()
			// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go's
			// comment — this is the exact call chain the audit flagged: a panic
			// anywhere in syncWithNode/doSyncOnce/fetchBlocksSince/
			// syncValidatorsFromPeer, reachable via the public
			// /api/peers/register endpoint, used to crash the entire node.
			if r := recover(); r != nil {
				fmt.Printf("[PANIC RECOVERED] sync goroutine for peer %s: %v\n%s\n", peerURL, r, debug.Stack())
			}
		}()
		dag.syncWithNode(peerURL)
	}()
}

// fetchBlocksSince fetches up to `limit` blocks with Height > minHeight from
// nodeURL. afterHash, when non-empty, uses cursor-based pagination to return
// blocks after (minHeight, afterHash) in canonical order — avoiding the
// same-height sibling skip bug (P1-02) where advancing min_height to the
// last-seen height could miss siblings that didn't fit on the previous page.
func (dag *BlockDAG) fetchBlocksSince(nodeURL string, minHeight int64, afterHash string, limit int) ([]*Block, error) {
	url := fmt.Sprintf("%s/api/blocks?min_height=%d&limit=%d", nodeURL, minHeight, limit)
	if afterHash != "" {
		url += "&after_hash=" + afterHash
	}
	resp, err := httpSyncClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// FIX: this used to decode resp.Body unconditionally regardless of HTTP
	// status — a 500/403/429/HTML error page got handed to json.Unmarshal
	// and surfaced as an opaque decode error indistinguishable from "peer
	// sent malformed JSON". Checking the status explicitly means operators
	// see the real cause (e.g. "peer returned 503") instead of a generic
	// decode failure during exactly the kind of outage/drift situation
	// where that distinction matters most.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("peer returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	var blocks []*Block
	if err := json.Unmarshal(body, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// fetchBlocksByHashes resolves multiple missing-parent hashes in a single
// HTTP round trip via /api/blocks/by-hash, instead of one request per hash.
// Returns only the blocks nodeURL actually has — silently omits any hash it
// doesn't (that's not an error here; the caller checks which hashes are
// still missing afterward). See fetchMissingAncestors' comment for why this
// batching, not a longer timeout, is the real fix for the orphan-abandon
// storm seen during a large catch-up.
func (dag *BlockDAG) fetchBlocksByHashes(nodeURL string, hashes []string) ([]*Block, error) {
	body, _ := json.Marshal(map[string][]string{"hashes": hashes})
	resp, err := httpSyncClient.Post(nodeURL+"/api/blocks/by-hash", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("peer returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	var blocks []*Block
	if err := json.Unmarshal(respBody, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// fetchMissingAncestors resolves orphaned blocks by walking backward one
// specific hash at a time, instead of waiting for them to fall inside
// doSyncOnce's height-windowed (?min_height=) pagination.
//
// FIX: that height window only ever looks near THIS node's own current
// frontier (dag.Height()-syncOverlap). Once a node's chain has drifted from
// a peer's by more than the overlap window — which can start from any
// transient gap, however brief — the actual common-ancestor blocks needed
// to bridge the two chains permanently fall outside that window and are
// never fetched again: every later block from that peer queues as an
// orphan whose missing parent doSyncOnce will never ask for. Confirmed in
// production: cd20 and a VPS secondary both briefly merged with the primary
// right after first connecting (small gap, within the 20-block overlap),
// then permanently regressed to fully isolated single-parent chains once
// that gap — for whatever transient reason — exceeded 20 blocks. This walks
// directly from "what hash is missing" instead of "what height window might
// contain it", so it has no such ceiling: each resolved ancestor's own
// AddPeerBlock call may reveal a further ancestor still missing, which gets
// picked up on the next call here (capped at maxAncestorFetchPerCycle per
// call to bound a single cycle's work, not the total depth reachable across
// repeated calls). Re-snapshots the orphan set after each batch so a chain
// of N missing ancestors in a row gets walked all the way back to a known
// block within a single call, not one hop per call.
// triggerOrphanResolve runs fetchMissingAncestors against every peer this
// node is currently syncing with, in parallel, right now — instead of
// waiting for each peer's own up-to-6s syncWithNode ticker to come around.
//
// Coordination: at most one resolve pass runs at a time (orphanResolveInFlight),
// since concurrent passes would just duplicate the same peer requests. If a
// new orphan triggers this while a pass is already running, that arrival is
// recorded (orphanResolveAgain) and the in-flight pass loops once more
// immediately after finishing — so a burst of orphans arriving faster than
// one pass can complete still gets a fresh attempt covering all of them,
// rather than silently relying on the next periodic tick.
func (dag *BlockDAG) triggerOrphanResolve() {
	dag.orphanResolveMu.Lock()
	if dag.orphanResolveInFlight {
		dag.orphanResolveAgain = true
		dag.orphanResolveMu.Unlock()
		return
	}
	dag.orphanResolveInFlight = true
	dag.orphanResolveMu.Unlock()

	for {
		dag.syncPeerMu.Lock()
		peers := make([]string, 0, len(dag.activeSyncPeers))
		for p := range dag.activeSyncPeers {
			peers = append(peers, p)
		}
		dag.syncPeerMu.Unlock()

		var wg sync.WaitGroup
		for _, peerURL := range peers {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go.
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("[PANIC RECOVERED] fetchMissingAncestors goroutine for peer %s: %v\n%s\n", p, r, debug.Stack())
					}
				}()
				dag.fetchMissingAncestors(p)
			}(peerURL)
		}
		wg.Wait()

		dag.orphanResolveMu.Lock()
		if !dag.orphanResolveAgain {
			dag.orphanResolveInFlight = false
			dag.orphanResolveMu.Unlock()
			return
		}
		dag.orphanResolveAgain = false
		dag.orphanResolveMu.Unlock()
		// loop again — another orphan arrived mid-pass
	}
}

func (dag *BlockDAG) fetchMissingAncestors(nodeURL string) {
	// FIX (2026-06-28, second incident): this used to cap at 2000 hashes
	// PER CALL and build toFetch by iterating dag.MissingParentHashes()'s
	// map — Go map iteration order is randomized per call, so once the
	// true backlog exceeded 2000 distinct hashes (confirmed live: tens of
	// thousands, after several restarts each fragmented a validator's own
	// chain into short-lived forks), each call only ever got a random
	// ~2000-hash sample of the total. Hashes unlucky enough to keep
	// missing the sample could sit unattempted for a long time — not
	// because any peer lacked them (verified live: the primary's
	// /api/blocks/by-hash answered one such hash immediately on request),
	// purely because of sampling. Now chunks ALL currently-pending,
	// not-on-cooldown hashes into <=maxBlocksByHashPerRequest-sized
	// batches (matching the server's own per-request cap, api.go) and
	// sends every chunk, so a single call genuinely attempts the entire
	// backlog rather than a random slice of it. totalFetched bounds a
	// single call's total work (not which hashes get tried) so a runaway
	// backlog can't make one call run forever.
	const maxBatchSize = maxBlocksByHashPerRequest
	const totalFetchedCap = 50000
	totalFetched := 0
	for totalFetched < totalFetchedCap {
		// PendingFetchHashes (not MissingParentHashes): also services
		// finality-checkpoint-walk gaps, which deliberately do NOT feed
		// wantDeepScan below — see registerFinalityWalkGap's own comment.
		hashes := dag.PendingFetchHashes()
		if len(hashes) == 0 {
			return
		}
		pending := make([]string, 0, len(hashes))
		for _, hash := range hashes {
			if dag.GetBlockByHash(hash) != nil {
				// Resolved (possibly via a path other than this loop, e.g. a
				// concurrent orphan resolution). finalityWalkGaps/
				// produceStuckGaps have no other cleanup path — clear both
				// here so a long-lived node doesn't accumulate one entry per
				// gap ever seen (clearProduceStuckGap is a harmless no-op if
				// hash was never a ProduceBlock stuck-gap).
				dag.clearFinalityWalkGap(hash)
				dag.clearProduceStuckGap(hash)
				continue
			}
			if !dag.shouldAttemptFetch(hash) {
				// Tried this exact hash too recently (orphanFetchCooldown) —
				// skip it this pass instead of re-hitting every peer for a
				// hash that just failed moments ago. See orphanFetchCooldown.
				continue
			}
			pending = append(pending, hash)
		}
		if len(pending) == 0 {
			return // every pending hash is either known now or on cooldown
		}
		fetchedThisRound := 0
		for i := 0; i < len(pending); i += maxBatchSize {
			chunk := pending[i:min(i+maxBatchSize, len(pending))]
			blocks, err := dag.fetchBlocksByHashes(nodeURL, chunk)
			if err != nil {
				fmt.Printf("[HTTP-SYNC] ✗ Could not batch-fetch %d missing ancestor(s) from %s: %v\n", len(chunk), nodeURL, err)
				continue // network failure — don't count as genuine peer confirmation
			}
			// Count an attempt only for hashes the peer confirmed it does NOT have
			// (i.e. the fetch succeeded but the hash was absent from the response).
			// A network error never counts — it says nothing about whether the peer
			// has the block, and we must not burn through orphanAbandonAfter budget
			// on transient connectivity issues.
			returned := make(map[string]bool, len(blocks))
			for _, block := range blocks {
				returned[block.Hash] = true
			}
			for _, h := range chunk {
				if !returned[h] {
					dag.RecordOrphanAttempt(h)
				}
			}
			for _, block := range blocks {
				fetchedThisRound++
				// SECURITY (P0, launch audit 2026-07-03): only an operator-
				// configured seed is a trust anchor for the FromSync gate
				// bypass — see isTrustedSyncSource / trustedSeeds field doc.
				// A block from any other (dynamically self-registered) sync
				// peer still goes through AddPeerBlock's normal authorization/
				// suspension/finality gates.
				block.FromSync = dag.isTrustedSyncSource(nodeURL)
				// FIX (durable fix, 2026-07-04): this IS the exact deliberate,
				// targeted ancestor fetch the circuit breaker's SelfFetched
				// exemption exists for — see that field's own comment (block.go).
				// Set regardless of trusted-seed status: authorization is a
				// wholly separate, still-fully-enforced gate a few lines below.
				block.SelfFetched = true
				if !dag.AddPeerBlock(block) {
					// Block was fetched from the peer but rejected locally
					// (bad signature, unauthorized proposer, etc.).  Count it
					// as an attempt so that orphans waiting on this hash can
					// age out via the normal TTL path instead of hanging
					// indefinitely.  abandonOrphansWaitingFor (called from
					// AddPeerBlock on unauthorized-proposer rejection) handles
					// the immediate cleanup; RecordOrphanAttempt here is a
					// backstop for other rejection reasons.
					dag.RecordOrphanAttempt(block.Hash)
				}
			}
		}
		totalFetched += fetchedThisRound
		if fetchedThisRound == 0 {
			return // peer had none of the currently-pending hashes (yet) — stop for this cycle
		}
	}
}

// doSyncOnce walks nodeURL's block history forward from our own current
// height, fetching pageSize-sized pages until it catches up to the peer's
// tip. Returns false on a network/decode error (used by syncWithNode to
// back off), true otherwise — including the normal "nothing new" case.
//
// FIX: this used to page via ?offset=dag.TotalBlocks() — i.e. treat "how
// many blocks do I have" as a position into the PEER's own array. That only
// works as long as both sides accumulate the exact same number of entries
// at the exact same pace, which breaks the moment more than one validator
// produces concurrently (the normal, intended BlockDAG case): each side
// merges multi-parent siblings at its own pace, so the two nodes' local
// block COUNTS drift apart from each other even when both are otherwise
// healthy. A node whose count fell out of step with the peer's array
// position kept requesting the same already-fully-known window forever —
// confirmed in production: a node stuck ~640 blocks behind never advanced,
// growing its own isolated, never-reconciled side chain instead. HEIGHT is
// the one frontier marker that stays meaningful regardless of how many
// duplicate-height siblings either side has — paging by "give me everything
// above the highest height I've already got" can't get stuck the same way.
// deepScanMinInterval throttles how often ONE peer's doSyncOnce call may run
// a full deepScan pass — see claimDeepScanSlot's own comment for why this is
// keyed per nodeURL, not shared across peers.
const deepScanMinInterval = 30 * time.Second

// claimDeepScanSlot reports whether nodeURL may run a deepScan pass right
// now, and if so, atomically claims the slot (starting this peer's own
// cooldown) so a concurrent call for the SAME peer can't also claim it.
//
// FIX (P0, 2026-07-04 — real root cause of a night of persistent merge
// failures): this cooldown used to be a single dag-wide timestamp shared by
// EVERY peer's syncWithNode goroutine. Each peer ticks independently and
// calls doSyncOnce, which checked/updated that one shared value — whichever
// peer's goroutine happened to check first within a given window claimed
// deepScan for ALL peers that window, permanently starving the others.
// Confirmed live: with Primary and a second, still-isolated peer both
// configured, the second peer's goroutine consistently won the shared slot,
// so Primary — the one peer whose bulk catch-up actually mattered — never
// got its own deepScan turn, leaving this node stuck on the slow,
// one-hash-at-a-time fetchMissingAncestors path indefinitely (too slow to
// keep pace with continuous multi-validator production). Keyed per nodeURL
// now so every peer gets its own independent cooldown.
func (dag *BlockDAG) claimDeepScanSlot(nodeURL string) bool {
	now := time.Now().Unix()
	dag.lastDeepScanAtMu.Lock()
	defer dag.lastDeepScanAtMu.Unlock()
	if now-dag.lastDeepScanAt[nodeURL] < int64(deepScanMinInterval.Seconds()) {
		return false
	}
	dag.lastDeepScanAt[nodeURL] = now
	return true
}

// deepScanPageBudgetPerCall bounds how many pages ONE doSyncOnce call walks
// while in deepScan mode — see deepScanResumeHeight's own struct comment
// (block.go) for the incident this closes. 20 pages (10,000 blocks at
// pageSize=500) keeps a single call's worst-case duration to roughly a
// second or two of real work instead of up to maxPagesPerCall (2000 pages,
// 1,000,000 blocks), which measured live took minutes and blocked this
// peer's entire sync loop for that whole span.
const deepScanPageBudgetPerCall = 20

// getDeepScanResumeHeight/setDeepScanResumeHeight read and update
// deepScanResumeHeight (see that field's own struct comment, block.go).
func (dag *BlockDAG) getDeepScanResumeHeight(nodeURL string) int64 {
	dag.lastDeepScanAtMu.Lock()
	defer dag.lastDeepScanAtMu.Unlock()
	return dag.deepScanResumeHeight[nodeURL]
}

func (dag *BlockDAG) setDeepScanResumeHeight(nodeURL string, height int64) {
	dag.lastDeepScanAtMu.Lock()
	defer dag.lastDeepScanAtMu.Unlock()
	dag.deepScanResumeHeight[nodeURL] = height
}

// getPeerSyncHeight/advancePeerSyncHeight track peerSyncHeight (see that
// field's own struct comment for the incident this exists to fix) — the
// highest height this node has actually imported FROM nodeURL specifically,
// independent of dag.Height() (which also reflects this node's own
// self-production and can race arbitrarily far ahead of real per-peer
// catch-up progress).
func (dag *BlockDAG) getPeerSyncHeight(nodeURL string) int64 {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	return dag.peerSyncHeight[nodeURL]
}

func (dag *BlockDAG) advancePeerSyncHeight(nodeURL string, height int64) {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	if height > dag.peerSyncHeight[nodeURL] {
		dag.peerSyncHeight[nodeURL] = height
	}
}

// cleanSyncStreakThreshold is how many CONSECUTIVE doSyncOnce cycles in a
// row must find nothing unmerged from a peer before hasCaughtUpWithAllPeers
// trusts it — one clean cycle could just be luck (e.g. a page landing
// between two concurrent bursts); several in a row is a genuine signal.
const cleanSyncStreakThreshold = 3

// recordCleanSyncCycle and resetCleanSyncStreak maintain cleanSyncStreak
// (see that field's own struct comment for the full incident this closes).
// doSyncOnce calls exactly one of these at the end of every cycle: clean if
// every page it fetched from nodeURL either had nothing new, or attached
// everything it did have; reset the instant a page returns blocks this
// node could NOT merge — the live signature of an active fork, not
// evidence of being caught up no matter how the raw height numbers read.
func (dag *BlockDAG) recordCleanSyncCycle(nodeURL string) {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	dag.cleanSyncStreak[nodeURL]++
}

func (dag *BlockDAG) resetCleanSyncStreak(nodeURL string) {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	dag.cleanSyncStreak[nodeURL] = 0
}

// hasCaughtUpWithAllPeers reports whether EVERY seed in seeds has reached
// cleanSyncStreakThreshold consecutive clean sync cycles — see
// cleanSyncStreak's own struct comment. Requires at least one seed to have
// been observed at all (an empty/all-unknown seed list is not "caught up",
// it's "hasn't started trying yet").
func (dag *BlockDAG) hasCaughtUpWithAllPeers(seeds []string) bool {
	if len(seeds) == 0 {
		return false
	}
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	for _, seed := range seeds {
		if dag.cleanSyncStreak[seed] < cleanSyncStreakThreshold {
			return false
		}
	}
	return true
}

// deepScanFloor is doSyncOnce's deepScan minHeight — see that call site's
// own FIX comment for the full 2026-07-04 permanent-non-convergence
// incident. BootHeight() is only a safe, inclusive-enough floor when
// BootHeightCheckpointBacked() is true: /api/blocks?min_height=N is
// EXCLUSIVE (Height > N), so using BootHeight() as the floor permanently
// excludes that exact height from every fetch — correct when a checkpoint-
// seeded resync already placed a real, verified block there (dag.blocks'
// anchor for the first block above it), but for a plain restart BootHeight
// is just a locally-computed number with no such guarantee. Confirmed live:
// a peer's real common-ancestor block sat exactly at BootHeight, and no
// number of deepScan passes could ever recover once it was permanently
// excluded. Without checkpoint backing, fall back to 0 (deepScan's
// original pre-checkpoint-seeding behavior) so a genuinely isolated node
// can still find its real common ancestor, however deep.
func (dag *BlockDAG) deepScanFloor() int64 {
	if dag.BootHeightCheckpointBacked() {
		return dag.BootHeight()
	}
	return 0
}

// finalityFloorLimit is the lowest height lowerDeepScanFloor will ever narrow
// a peer's floor down to. isFinalityViolation (finality.go) unconditionally
// rejects any block below finalizedHeight-finalityHeightSlack regardless of
// how far back deepScan searches — so searching past that point can never
// recover anything, it would only waste a full sweep re-scanning a range
// doSyncOnce's own isFinalityViolation pre-check already silently skips
// every block in. Returns 0 (no limit beyond deepScan's own genesis floor)
// before any checkpoint has been finalized yet.
func (dag *BlockDAG) finalityFloorLimit() int64 {
	if dag.state == nil {
		return 0
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight == 0 {
		return 0
	}
	limit := finalizedHeight - finalityHeightSlack
	if limit < 0 {
		return 0
	}
	return limit
}

// effectiveDeepScanFloor is deepScanFloor(), further narrowed to nodeURL's
// own deepScanFloorOverride if lowerDeepScanFloor has already lowered it for
// this specific peer (see that function's own comment for when/why). Reads
// the override under lastDeepScanAtMu — the same per-peer bookkeeping lock
// deepScanFloorOverride's own struct comment documents (block.go).
func (dag *BlockDAG) effectiveDeepScanFloor(nodeURL string) int64 {
	floor := dag.deepScanFloor()
	dag.lastDeepScanAtMu.Lock()
	override, ok := dag.deepScanFloorOverride[nodeURL]
	dag.lastDeepScanAtMu.Unlock()
	if ok && override < floor {
		return override
	}
	return floor
}

// clearDeepScanFloorOverride resets nodeURL back to trusting deepScanFloor()
// as-is — called once a deepScan sweep against this peer actually resolves
// cleanly, so a LATER, unrelated gap doesn't inherit a floor lowered to chase
// a completely different incident.
func (dag *BlockDAG) clearDeepScanFloorOverride(nodeURL string) {
	dag.lastDeepScanAtMu.Lock()
	delete(dag.deepScanFloorOverride, nodeURL)
	dag.lastDeepScanAtMu.Unlock()
}

// lowerDeepScanFloor narrows nodeURL's deepScan floor after a FULL sweep
// starting at sweptFrom (deepScanFloor()/the previous override, whichever
// doSyncOnce actually started this sweep from) reached the peer's tip
// (reachedPeerTip) yet still could not merge everything (sawUnmergedBlocks).
//
// FIX (P0, 2026-07-10 — Primary/Contabo1 permanent-partial-merge incident):
// deepScanFloor()'s checkpoint-backed branch only guarantees THIS node's own
// history is anchored by a real block at BootHeight — it says nothing about
// whether a SPECIFIC peer's fork was ever fully merged before BootHeight
// advanced past it. Confirmed live: after a sustained sync-starvation
// incident (see the deepScan-page-budget fix this session, 7a9dc58), Primary
// kept missing a contiguous run of Contabo1's own chain sitting BELOW its own
// (checkpoint-backed, correctly-anchored-for-ITS-OWN-history) BootHeight — a
// full deepScan sweep from that floor to Contabo1's tip reached the tip
// cleanly but never touched the gap, because the gap was never IN [floor,
// tip] to begin with. Halving the distance toward finalityFloorLimit() each
// time this happens (rather than jumping straight to 0/the finality limit)
// keeps the common case — a gap just below the floor, e.g. from a bounded
// starvation incident — cheap to find in one or two sweeps, while still
// guaranteeing convergence for a pathologically deep gap within
// O(log(floor)) sweeps. Never lowers at or below finalityFloorLimit(): past
// that point isFinalityViolation would reject the recovered block anyway
// (see that function's own comment), so there is nothing further to gain by
// searching there — see abandonOrphansWaitingFor's new call site
// (AddPeerBlock's finality gate) for what actually happens to orphans stuck
// at that boundary.
func (dag *BlockDAG) lowerDeepScanFloor(nodeURL string, sweptFrom int64) {
	limit := dag.finalityFloorLimit()
	if sweptFrom <= limit {
		return // already at (or below) the lowest floor worth trying
	}
	newFloor := limit + (sweptFrom-limit)/2
	dag.lastDeepScanAtMu.Lock()
	dag.deepScanFloorOverride[nodeURL] = newFloor
	dag.lastDeepScanAtMu.Unlock()
	fmt.Printf("[HTTP-SYNC] ⚠ Full deepScan sweep from height %d to %s's tip still left blocks unmerged — lowering this peer's deepScan floor to %d to search further back for a common ancestor\n",
		sweptFrom, nodeURL, newFloor)
}

func (dag *BlockDAG) doSyncOnce(nodeURL string) (ok bool) {
	const pageSize = 500
	const maxPagesPerCall = 2000 // hard cap: 1,000,000 blocks per call — headroom, not unbounded
	// FIX: requesting strictly "height > my own height" misses SIBLINGS at
	// or just below that height that other validators produced. A later
	// block's parent can be one of those siblings (e.g. the peer's own
	// previous block, at a height I already consider "done" because MY
	// own block at that height got there first) — if I never fetched it,
	// AddPeerBlock rejects every subsequent block built on top of it for
	// missing a parent I don't have, forever. Confirmed in production:
	// three concurrent validators each kept seeing ONLY their own
	// single-parent chain in their own /api/blocks output — never each
	// other's blocks — because the height-exclusive fetch never pulled in
	// the sibling branches needed to resolve later parents. Re-requesting
	// a small overlap window of already-"passed" heights each cycle is
	// cheap (AddPeerBlock dedupes by hash) and guarantees sibling forks
	// from other validators get imported before something builds on them.
	const syncOverlap = 20
	// deepScan: when we have orphan blocks whose parents aren't in our normal
	// overlap window, drop all the way to height 0 and scan forward.  The
	// hash-by-hash approach (fetchMissingAncestors) can only walk back one
	// level per HTTP request — for a peer whose validator chain started
	// thousands of blocks ago that takes hours.  Fetching in height-ordered
	// pages is O(N/pageSize) requests for N missing blocks, not O(N).
	// deepScan stays true for the duration of this call; the next call will
	// re-evaluate whether orphans still exist.
	//
	// THROTTLE (P2-01 audit, confirmed live on Contabo 2026-06-30): a large
	// genuinely-unresolvable orphan backlog (stale references to a node's own
	// pre-fix bad blocks, never resolvable from any peer) keeps
	// MissingParentHashes() non-empty for the full 15-minute
	// orphanAbandonAfter window. Without this throttle, every 6s sync tick
	// re-ran the O(chain length) full walk for that entire window — confirmed
	// live: 99% CPU sustained for minutes at chain length ~50,000 with ~8,500
	// distinct missing parents pending abandonment, and the normal windowed
	// sync below (which WOULD have kept making real progress) never got a
	// chance to run because deepScan always took over. At most one full
	// height-0 walk per deepScanMinInterval; fetchMissingAncestors (the
	// cheap, targeted hash lookup) still runs every single cycle regardless.
	wantDeepScan := len(dag.MissingParentHashes()) > 0
	// FIX (P0, 2026-07-10): a deepScan pass already IN PROGRESS (resume
	// height beyond the floor — see deepScanResumeHeight's struct comment)
	// must not wait out claimDeepScanSlot's 30s-per-peer cooldown between
	// each bounded continuation — that cooldown exists to throttle how
	// often an expensive full walk gets STARTED, not to throttle an
	// already-bounded, already-cheap continuation of one already running.
	// Gating continuations on it too would make walking past a large
	// already-known historical region (e.g. ~680,000 blocks, confirmed
	// live) take tens of minutes in 30s-spaced 10,000-block increments
	// instead of well under a minute back-to-back.
	resumingDeepScan := dag.getDeepScanResumeHeight(nodeURL) > 0
	deepScan := wantDeepScan && (resumingDeepScan || dag.claimDeepScanSlot(nodeURL))
	// FIX (audit 2026-07-06, definitive root cause of a 3-node non-merging
	// incident): this used to be dag.Height()-syncOverlap — dag.Height() is
	// the highest height from ANY source, including this node's own
	// continuous self-production, which races ahead of real per-peer catch-
	// up once this node produces a block every tick while nodeURL's blocks
	// arrive with any latency at all. Once self-production had raced far
	// enough ahead, this window permanently requested a range past
	// nodeURL's actual next block — and since deepScan only activates once
	// something has ALREADY failed to attach as an orphan, no missing-
	// parent entry was ever created to trigger recovery either: the gap
	// silently persisted forever, visible live as one node successfully
	// merging blocks from every peer while its peers' own canonical chains
	// never advanced past their pre-existing self-produced history no
	// matter how many times they were resynced. peerSyncHeight (see its own
	// struct comment) tracks progress against THIS peer specifically,
	// immune to this node's own production rate.
	minHeight := dag.getPeerSyncHeight(nodeURL) - syncOverlap
	if minHeight < 0 || deepScan {
		// See deepScanFloor's own comment for why this is NOT simply
		// dag.BootHeight() — the 2026-07-04 permanent-non-convergence
		// incident this fixes. effectiveDeepScanFloor further narrows this
		// to nodeURL's own deepScanFloorOverride if an earlier full sweep
		// against this specific peer already proved deepScanFloor() itself
		// sits above the real common ancestor — see lowerDeepScanFloor's
		// own comment.
		minHeight = dag.effectiveDeepScanFloor(nodeURL)
		if deepScan {
			// Resume a previously-bounded pass instead of restarting the
			// full historical walk from the floor every call — see
			// deepScanResumeHeight's own struct comment (block.go).
			if resume := dag.getDeepScanResumeHeight(nodeURL); resume > minHeight {
				minHeight = resume
			}
		}
	}
	// sweepFloor records where THIS deepScan sweep actually started (before
	// the per-page loop below advances minHeight forward as its own cursor)
	// — lowerDeepScanFloor needs the sweep's starting point, not wherever it
	// ended up, to know how far below it to search next.
	sweepFloor := minHeight
	totalAdded := 0
	highestSeen := int64(-1)
	// sawUnmergedBlocks feeds cleanSyncStreak (see that field's own struct
	// comment) — set true the instant a page contains a genuinely new block
	// (not genesis, not already known, not a finality-violation skip — see
	// the per-block loop below) that AddPeerBlock still could not attach.
	// That is the live signature of an active fork: real data exists on
	// nodeURL that this node cannot currently place, which no amount of
	// waiting on a height number alone can distinguish from "genuinely
	// nothing new".
	sawUnmergedBlocks := false
	// P1-02: track (minHeight, afterHash) cursor so same-height siblings that
	// don't fit in one page are not skipped.  afterHash is empty for the first
	// page (ordinary Height > minHeight query) and set to the last block's hash
	// whenever a full page is returned, advancing the cursor into the next page.
	var afterHash string
	// FIX (P0, 2026-07-10): deepScan uses a much smaller page budget than an
	// ordinary catch-up call — see deepScanPageBudgetPerCall's own comment
	// for the incident this closes (an unbounded deepScan pass blocking
	// this peer's entire sync loop for minutes). reachedPeerTip records
	// whether this call ended because the peer genuinely had nothing more
	// (a true empty page), vs. merely running out of this call's budget —
	// only the former means a deepScan pass has actually finished a
	// complete sweep (see the len(blocks)==0 branch below and this
	// function's post-loop bookkeeping).
	pagesBudget := maxPagesPerCall
	if deepScan {
		pagesBudget = deepScanPageBudgetPerCall
	}
	reachedPeerTip := false
	for page := 0; page < pagesBudget; page++ {
		blocks, err := dag.fetchBlocksSince(nodeURL, minHeight, afterHash, pageSize)
		if err != nil {
			fmt.Printf("[HTTP-SYNC] ✗ Could not fetch page (min_height=%d) from %s: %v\n", minHeight, nodeURL, err)
			if page == 0 {
				return false // never even got a first page — treat as a failed sync attempt
			}
			break // got at least one page this call; report what we added
		}
		if len(blocks) == 0 {
			reachedPeerTip = true
			break // caught up — peer has nothing newer than our height
		}
		addedThisPage := 0
		for _, block := range blocks {
			// FIX: genesis is always created locally and AddPeerBlock always
			// rejects a peer-supplied genesis (by design — see its own
			// comment). Without this skip, every single sync cycle forever
			// re-attempts and re-logs "Rejected peer genesis", since it's
			// never marked as "exists" and so never short-circuits like a
			// normal already-known block would.
			if block.IsGenesis {
				continue
			}
			if block.Height > highestSeen {
				highestSeen = block.Height
			}
			// SECURITY (P0, launch audit 2026-07-03): see isTrustedSyncSource.
			block.FromSync = dag.isTrustedSyncSource(nodeURL)
			dag.mu.RLock()
			_, exists := dag.blocks[block.Hash]
			// FIX (audit 2026-07-06): skip a block that's already below our
			// own finalized checkpoint before ever queueing it as an orphan —
			// isFinalityViolation would reject it later anyway (block.go's
			// AddPeerBlock calls it), but only after paying for a full
			// dag.mu write-lock, orphan-queue insert, and log line. Confirmed
			// live: a large backlog page fetched right after a snapshot
			// resync (this peer's history spans a window our own pruning
			// window has already moved past) produced hundreds of these
			// pointless orphan/reject cycles per page. Harmless to skip here
			// — a genuinely useful gap-fill within finalityHeightSlack of the
			// checkpoint still passes this check unchanged.
			violatesFinality := !exists && dag.isFinalityViolation(block)
			dag.mu.RUnlock()
			if violatesFinality {
				continue
			}
			// FIX (durable fix, 2026-07-04): doSyncOnce's own ordered paged
			// catch-up is exactly the deliberate, self-initiated fetch the
			// circuit breaker's SelfFetched exemption exists for — see that
			// field's own comment (block.go). Authorization remains a wholly
			// separate, still-fully-enforced gate below.
			block.SelfFetched = true
			if !exists {
				if dag.AddPeerBlock(block) {
					addedThisPage++
				} else {
					sawUnmergedBlocks = true
				}
			}
		}
		totalAdded += addedThisPage
		// FIX (P2-01 audit, confirmed live on Contabo 2026-06-30): a
		// short page (< pageSize) does NOT reliably mean "peer's tip is
		// within this page" once a deep scan is re-walking ALREADY-KNOWN
		// history. Block production isn't uniformly dense across height —
		// a sparse window (e.g. an early region with fewer concurrent
		// validators) can return fewer than pageSize blocks despite being
		// nowhere near the actual tip. Breaking here during deepScan made
		// every cycle restart from height 0 and exit at the exact same
		// premature sparse window, never reaching the real frontier —
		// confirmed live: dag.height sat motionless for 10+ minutes while
		// orphan housekeeping discarded hundreds of dead-end siblings,
		// because the one page containing the real next height was never
		// fetched. Outside deepScan this remains a correct, cheap signal
		// (normal forward sync only ever requests pages it expects to be
		// at or near the tip), so only deepScan skips the early break.
		if len(blocks) < pageSize && !deepScan {
			afterHash = "" // last page — reset cursor
			break          // peer's tip is within this page
		}
		// Full page returned: advance the cursor to the last block so the
		// next request picks up from that exact position in canonical order.
		// This avoids the same-height sibling skip: if all 500 blocks are at
		// height H, the next request uses ?min_height=H&after_hash=<last>
		// instead of ?min_height=H (which would re-fetch the same 500 blocks).
		lastBlock := blocks[len(blocks)-1]
		minHeight = lastBlock.Height
		afterHash = lastBlock.Hash
		if addedThisPage == 0 {
			if !deepScan {
				// Normal mode: nothing new in a full page — stop.
				// Looping again would get the same page forever.
				fmt.Printf("[HTTP-SYNC] ⚠ Page above height %d added 0 of %d blocks — stopping sync from %s for this cycle\n", minHeight, len(blocks), nodeURL)
				break
			}
			// Deep-scan mode: empty pages are expected while scanning
			// through the historical region before the missing chain starts.
			// Keep going — the first block of the missing validator chain
			// is somewhere ahead.
		}
	}
	if deepScan {
		if reachedPeerTip {
			// A full sweep actually completed — see deepScanResumeHeight's
			// struct comment for why this resets to 0 (not just stops
			// advancing): a LATER deepScan triggered by a fresh, unrelated
			// orphan must start a genuine full sweep from the true floor
			// again, not silently resume from "already at the tip" and skip
			// over whatever new gap it was actually triggered to find.
			dag.setDeepScanResumeHeight(nodeURL, 0)
			if sawUnmergedBlocks {
				// The sweep walked all the way from sweepFloor to nodeURL's
				// own tip and STILL couldn't merge everything — the real
				// common ancestor isn't in [sweepFloor, tip] at all, it must
				// be below sweepFloor. See lowerDeepScanFloor's own comment.
				dag.lowerDeepScanFloor(nodeURL, sweepFloor)
			} else {
				// Fully resolved — stop overriding so a later, unrelated gap
				// against this same peer starts from the normal floor again
				// instead of inheriting a floor lowered to chase this one.
				dag.clearDeepScanFloorOverride(nodeURL)
			}
		} else {
			// Budget exhausted mid-walk — continue from here next call
			// instead of restarting from the floor.
			dag.setDeepScanResumeHeight(nodeURL, minHeight)
		}
	}
	if highestSeen >= 0 {
		// Advance regardless of totalAdded: a page can be entirely orphans
		// (addedThisPage==0) yet still genuinely reflect nodeURL's real
		// height — fetchMissingAncestors resolves those by hash separately,
		// and re-requesting the same already-seen range forever would only
		// reproduce the exact non-convergence this cursor exists to avoid.
		dag.advancePeerSyncHeight(nodeURL, highestSeen)
	}
	if totalAdded > 0 {
		dag.mu.RLock()
		tipCount := len(dag.tips)
		dag.mu.RUnlock()
		fmt.Printf("[HTTP-SYNC] ✓ Added %d new blocks from %s | DAG tips: %d | height %d\n", totalAdded, nodeURL, tipCount, dag.Height())
	}
	if sawUnmergedBlocks {
		dag.resetCleanSyncStreak(nodeURL)
	} else {
		dag.recordCleanSyncCycle(nodeURL)
	}

	// Resolve any orphans (this cycle's or earlier ones) by fetching their
	// specific missing-parent hash directly — see fetchMissingAncestors for
	// why the height-windowed pagination above can't reach them once the
	// gap exceeds syncOverlap.
	dag.fetchMissingAncestors(nodeURL)
	return true
}

// validatorSyncInterval: re-sync the validator list from each peer this often.
// Ensures a validator that registered with any peer propagates to all nodes
// within this window, with zero manual configuration.
// validatorSyncInterval: how many HTTP-sync ticks between full validator-list
// refreshes from each peer. Base tick is 1s → 50 ticks ≈ 50s.
const validatorSyncInterval = 50

func (dag *BlockDAG) syncWithNode(nodeURL string) {
	// Fetch validator list immediately on first connect — this is the moment
	// a new peer (e.g. a VPS with its own validator registrations) becomes
	// known to us, and we want to accept their blocks from the first sync tick.
	dag.syncValidatorsFromPeer(nodeURL)

	// Try immediately on first call — no initial delay. doSyncOnce itself
	// pages through full history starting from whatever we already have
	// locally, so this one call already performs the initial catch-up.
	// 1s base: matches BLOCK_TIME so a peer's block arrives at our tips
	// within one tick and can be included as a parent in the next
	// ProduceBlock, enabling genuine multi-parent GHOSTDAG merges.
	// (Old value was 6s — blocks arrived 6 ticks late, no merges.)
	backoff := 1 * time.Second
	ticks := 0
	dag.doSyncOnce(nodeURL)
	ticker := time.NewTicker(backoff)
	defer ticker.Stop()
	for range ticker.C {
		ticks++
		if ticks%validatorSyncInterval == 0 {
			dag.syncValidatorsFromPeer(nodeURL)
		}
		if !dag.doSyncOnce(nodeURL) {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			ticker.Reset(backoff)
			continue
		}
		// Reset backoff on success
		if backoff > 1*time.Second {
			backoff = 1 * time.Second
			ticker.Reset(backoff)
		}
	} // end for range ticker.C
} // end syncWithNode

// ─── PEER DISCOVERY ──────────────────────────────────────────────────────────

// StartPeerDiscovery handles automatic peer registration and discovery.
//
// Environment variables:
//   SELF_URL          — this node's own public URL (required for registration)
//   PRIMARY_NODE_URL  — primary node to register with (omit on the primary itself).
//                        Daily pool distribution (UBI/validator/LP/escrow, see
//                        main.go) is gated separately by DISTRIBUTION_ENABLED,
//                        NOT by this variable — a node missing PRIMARY_NODE_URL
//                        used to silently self-identify as the distribution
//                        authority, which is exactly the duplicate-distribution
//                        failure class this whole mechanism exists to prevent.
//                        All nodes distribute by default; set DISTRIBUTION_ENABLED=false
//                        to opt a specific node out (cross-node dedup prevents double-credit).
//   PRIMARY_NODE_URLS — OPTIONAL comma-separated list of ADDITIONAL seed nodes
//                        to register/discover peers through, alongside
//                        PRIMARY_NODE_URL. Purely a bootstrap-resilience
//                        mechanism (scale audit): with only a single
//                        PRIMARY_NODE_URL, that one node being temporarily
//                        unreachable (deploy, restart, outage) stops a new
//                        node from ever discovering the rest of the network
//                        or registering as a validator, and stops every
//                        already-connected node from learning about NEW
//                        peers/validators until it comes back — a real
//                        single point of failure at a 100-node target where
//                        "exactly one fixed bootstrap node" doesn't hold up
//                        the way it might at 2-3 nodes. Has NO effect on
//                        DISTRIBUTION_ENABLED/distribution-authority
//                        semantics, which remain governed solely by
//                        PRIMARY_NODE_URL + DISTRIBUTION_ENABLED as before —
//                        this only widens which nodes can be used to learn
//                        the peer/validator list from.
//   PEER_NODES        — comma-separated static peer list (optional fallback)
//
// Flow for secondary nodes (Railway/VPS/self-hosted):
//   1. POST /api/peers/register to EVERY configured seed (PRIMARY_NODE_URL
//      plus PRIMARY_NODE_URLS) with our own URL, merging whichever respond
//   2. Receive current peer list, start syncing each peer
//   3. Every 30s: repeat against all configured seeds to heartbeat + discover new peers
//
// Flow for the primary node (IS_PRIMARY_NODE=true):
//   - Accepts registrations, serves peer list — no outbound registration needed
// NormalizeNodeURL prepends "https://" to rawURL if it has no http(s) scheme.
// Several hosting providers' "public domain" variables (e.g. Railway's
// RAILWAY_PUBLIC_DOMAIN) are bare hostnames with no scheme; SELF_URL/
// PRIMARY_NODE_URL set from those would otherwise fail isAllowedPeerURL's
// "must be public HTTPS" check and silently break peer registration.
func NormalizeNodeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return "https://" + rawURL
}

// seedURLs returns the deduplicated, normalized list of nodes to register
// and discover peers through: PRIMARY_NODE_URL plus any extra entries from
// PRIMARY_NODE_URLS (see StartPeerDiscovery's comment for why having more
// than one matters at scale). selfURL is excluded so a node never tries to
// register with itself.
// defaultPublicSeed is the well-known public Aequitas network entry point.
// Setup simplification (scale audit): a brand-new operator joining the
// existing public network should not have to look up or be handed a
// PRIMARY_NODE_URL before their node can do anything — that is exactly the
// kind of extra required variable this audit pass set out to eliminate. If
// the operator hasn't configured anything, default to this. PRIMARY_NODE_URL
// / PRIMARY_NODE_URLS remain available to override or extend it (e.g. a
// private/test deployment that must never reach the public network sets
// PRIMARY_NODE_URL to its own seed instead). Safe even for the official
// primary itself: seedURLs always excludes selfURL, so a node whose own
// SELF_URL happens to equal this default simply filters it out below.
const defaultPublicSeed = "https://aequitas.digital"

// defaultPublicSeeds is the built-in seed list used when neither
// PRIMARY_NODE_URL nor PRIMARY_NODE_URLS is set.
//
// MIGRATION (Railway decommissioned, 2026-08-14): this used to be the single
// defaultPublicSeed constant above. Railway hosted what that domain pointed
// at; with Railway gone the domain answers 404 (DNS still points at a
// non-Aequitas host), so a zero-config node's ONLY built-in HTTP entry point
// was dead — and the P2P bootstrap default had gone stale at the same time
// for the same reason (see defaultBootstrapNodes, p2p.go). A newcomer could
// therefore reach the network by neither transport. Confirmed live
// 2026-08-14: both surviving validators reported "peers":null.
//
// The domain is kept FIRST because it is the intended long-term entry point:
// it becomes available for this project on 2026-08-18 and will then point at
// Contabo1. Until it does — and whenever it is mid-migration, expired, or its
// TLS certificate is being reissued — the two validator IPs below keep
// zero-config onboarding working. They are plain http:// on purpose:
// isAllowedPeerURL (this file) deliberately permits http for LITERAL public
// IPs and requires https only for HOSTNAMES, because the https requirement
// exists to stop DNS rebinding, which a literal IP is not subject to.
//
// NOTE for the bootstrap validators themselves: these entries are aliases for
// those very nodes, and seedURLs can only filter out an exact selfURL match —
// it cannot know that "https://aequitas.digital" and "http://173.249.37.118:8080"
// are the same machine. Contabo1 and Contabo2 must therefore set
// PRIMARY_NODE_URLS explicitly (pointing at each other), which bypasses this
// default list entirely. That is already how both are configured; this note
// exists so it stays that way.
var defaultPublicSeeds = []string{
	defaultPublicSeed,
	"http://173.249.37.118:8080", // Contabo1
	"http://194.163.188.71:8080", // Contabo2
}

func seedURLs(selfURL string) []string {
	seen := map[string]bool{selfURL: true}
	var out []string
	add := func(raw string) {
		u := strings.TrimRight(NormalizeNodeURL(raw), "/")
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	add(os.Getenv("PRIMARY_NODE_URL"))
	for _, raw := range strings.Split(os.Getenv("PRIMARY_NODE_URLS"), ",") {
		add(raw)
	}
	if len(out) == 0 {
		for _, s := range defaultPublicSeeds {
			add(s)
		}
	}
	return out
}

// PrimarySeedURL returns the single best URL to compare against for
// divergence self-checks: the same PRIMARY_NODE_URL/PRIMARY_NODE_URLS/
// defaultPublicSeed resolution order seedURLs uses for peer discovery, so a
// zero-config node (no PRIMARY_NODE_URL set) still gets a real primary to
// compare against instead of the empty string. Exported for main.go, which
// hands this straight to StartDivergenceAutoHeal.
func PrimarySeedURL(selfURL string) string {
	urls := seedURLs(selfURL)
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

// isTrustedSyncSource reports whether nodeURL is one of this node's
// operator-configured seed/static-peer URLs (see the trustedSeeds field doc
// on BlockDAG for why this, and not activeSyncPeers, is the right trust
// anchor for block.FromSync).
func (dag *BlockDAG) isTrustedSyncSource(nodeURL string) bool {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	return dag.trustedSeeds[nodeURL]
}

// PeerSyncDiagnostics is a read-only snapshot of one currently-registered
// peer's sync state, for operator visibility (/api/health/combined) into
// internals that were previously only ever visible as scattered log lines —
// see SyncDiagnostics' own comment for why this exists.
type PeerSyncDiagnostics struct {
	PeerURL               string `json:"peer_url"`
	PeerSyncHeight        int64  `json:"peer_sync_height"`
	CleanSyncStreak       int    `json:"clean_sync_streak"`
	DeepScanResumeHeight  int64  `json:"deep_scan_resume_height"`
	DeepScanFloorOverride int64  `json:"deep_scan_floor_override"`
}

// SyncDiagnostics returns a snapshot of every currently-registered peer's
// sync state.
//
// FIX (P0, 2026-07-10): while investigating the Primary/Contabo1
// permanent-partial-merge incident, bootHeight/BootHeightCheckpointBacked/
// finalized_height/deepScanFloorOverride/deepScanResumeHeight — the exact
// values needed to tell "haven't searched far back enough yet" apart from
// "permanently walled off by hard finality" apart from "already fully
// converged" — turned out to be visible NOWHERE except grep-ing raw process
// logs (no API endpoint exposed any of them). That forced every diagnosis to
// go through a slow log-paste-and-guess cycle instead of one API call.
// Exposed via /api/health/combined's new sync_diagnostics field alongside
// this method's sibling values (BootHeight/BootHeightCheckpointBacked,
// dag.state.GetFinalizedCheckpoint(), finalityHeightSlack).
func (dag *BlockDAG) SyncDiagnostics() []PeerSyncDiagnostics {
	dag.syncPeerMu.Lock()
	peers := make([]string, 0, len(dag.activeSyncPeers))
	for p := range dag.activeSyncPeers {
		peers = append(peers, p)
	}
	dag.syncPeerMu.Unlock()
	sort.Strings(peers) // deterministic order for a human reading the JSON

	result := make([]PeerSyncDiagnostics, 0, len(peers))
	for _, p := range peers {
		dag.syncPeerMu.Lock()
		syncHeight := dag.peerSyncHeight[p]
		streak := dag.cleanSyncStreak[p]
		dag.syncPeerMu.Unlock()

		dag.lastDeepScanAtMu.Lock()
		resumeHeight := dag.deepScanResumeHeight[p]
		floorOverride := dag.deepScanFloorOverride[p]
		dag.lastDeepScanAtMu.Unlock()

		result = append(result, PeerSyncDiagnostics{
			PeerURL:               p,
			PeerSyncHeight:        syncHeight,
			CleanSyncStreak:       streak,
			DeepScanResumeHeight:  resumeHeight,
			DeepScanFloorOverride: floorOverride,
		})
	}
	return result
}

func (dag *BlockDAG) StartPeerDiscovery(selfURL string) {
	selfURL = strings.TrimRight(NormalizeNodeURL(selfURL), "/")

	fmt.Println("── Starting Peer Discovery ──────────────")
	if selfURL == "" {
		fmt.Println("[PEERS] SELF_URL not set — no peer sync (isolated node)")
		return
	}
	fmt.Printf("[PEERS] Self: %s\n", selfURL)

	// Seed from explicit PEER_NODES (backwards compat + manual override)
	staticP := staticPeers(selfURL)
	seeds := seedURLs(selfURL)
	// Populate the FromSync trust anchor before starting any sync goroutine
	// below that could read it (dag.startSyncForPeer / registerAndDiscover
	// spawn the goroutines that eventually call isTrustedSyncSource).
	dag.syncPeerMu.Lock()
	dag.trustedSeeds = make(map[string]bool, len(staticP)+len(seeds))
	for _, p := range staticP {
		dag.trustedSeeds[p] = true
	}
	for _, s := range seeds {
		dag.trustedSeeds[s] = true
	}
	dag.syncPeerMu.Unlock()

	for _, peer := range staticP {
		GlobalPeerRegistry.Register(peer)
		dag.startSyncForPeer(peer)
		fmt.Printf("[PEERS] Static peer: %s\n", peer)
	}

	if len(seeds) > 0 {
		for _, seed := range seeds {
			fmt.Printf("[PEERS] Seed: %s\n", seed)
			ok := dag.registerAndDiscover(selfURL, seed)
			// A seed never includes itself in its own peer list (/api/peers
			// only contains registered secondary nodes). Start syncing from
			// it directly so this node always receives that seed's blocks,
			// even if every OTHER peer it discovers is unreachable.
			dag.startSyncForPeer(seed)
			// FIX (durable fix, 2026-07-09): a failed registration (rate
			// limit, transient network error) used to be a one-shot,
			// permanent failure for this node's whole uptime — this node's
			// OWN block-fetching from the seed still worked via
			// startSyncForPeer above, but the seed never learned THIS
			// node's URL (so it couldn't push blocks back), and any OTHER
			// peer only the seed knew about was never discovered either.
			// Confirmed live: a registration that hit the seed's per-IP
			// rate limit during a burst of restarts left the node in
			// exactly that state for its entire remaining uptime, with no
			// automatic recovery short of a manual restart. Retry in the
			// background with backoff instead of accepting the first
			// failure as final.
			if !ok {
				seedCopy := seed
				SafeGoroutine("registerAndDiscover-retry-"+seedCopy, func() {
					backoff := 10 * time.Second
					for attempt := 1; attempt <= 8; attempt++ {
						time.Sleep(backoff)
						fmt.Printf("[PEERS] Retrying registration with %s (attempt %d)...\n", seedCopy, attempt+1)
						if dag.registerAndDiscover(selfURL, seedCopy) {
							fmt.Printf("[PEERS] ✓ Registration with %s succeeded on retry attempt %d.\n", seedCopy, attempt)
							return
						}
						backoff *= 2
						if backoff > 5*time.Minute {
							backoff = 5 * time.Minute
						}
					}
					fmt.Printf("[PEERS] ✗ Giving up retrying registration with %s after repeated failures — this node's own block-fetching from it still works, but it won't learn peers only %s knows about until the next restart.\n", seedCopy, seedCopy)
				})
			}
		}
		// Initial-sync gate: fetch each seed's current height so ProduceBlock
		// can defer production until we've caught up. This prevents the
		// "diverged chain on restart" bug: without this gate, a restarting node
		// produces on its own DB state before the sync loop has pulled in the
		// seed's newer blocks, creating a fork that requires RESYNC to fix.
		// Uses the highest reported height across all seeds as the target.
		//
		// FIX (P0, 2026-07-10 — root cause of Contabo1 forking almost
		// immediately after every RESYNC_FROM_SNAPSHOT boot, twice in a row):
		// this call used to fire exactly ONCE, in the same instant as peer
		// discovery starts — which is also essentially the same instant
		// RESYNC_FROM_SNAPSHOT just seeded dag.height near bootHeight from the
		// snapshot. Confirmed live: the one-shot query caught the seed barely
		// one block past the checkpoint it had JUST supplied, so target ended
		// up only marginally ahead of this node's own already-near-bootHeight
		// dag.height — satisfying dag.height >= target-10 (block.go's
		// ProduceBlock gate) and clearing the gate within seconds, while the
		// REST of boot (P2P setup, primary registration hitting a 429 rate
		// limit and retrying with backoff — all real wall-clock time the real
		// network keeps producing through) was still ahead. Production then
		// resumed on a node that was, in reality, dozens of blocks behind the
		// live tip, exactly reproducing the fork this gate exists to prevent.
		// Re-querying on a short ticker instead of once keeps target tracking
		// the seed's ACTUAL current height throughout the whole boot sequence,
		// not just the instant peer discovery began; fetchAndSetSyncTarget's
		// own "maxHeight <= dag.Height()" check already makes each call a
		// cheap no-op once genuinely caught up.
		//
		// FIX (found on self-review before this ever shipped): the first
		// version of this loop stopped as soon as syncTargetHeight read 0 —
		// but 0 is ALSO the value it starts at before the very first call
		// ever runs, and is exactly what the first call sets it to when the
		// gate fails to engage at all (maxHeight <= dag.Height(), the precise
		// failure mode this fix exists to close: the seed's height, queried
		// in the same instant as the snapshot, isn't meaningfully ahead of
		// this node's already-near-bootHeight dag.Height() yet). That reading
		// would have made the loop exit after its very first tick without
		// ever re-querying — silently reproducing the exact bug this commit
		// fixes. There is no cheap way to distinguish "genuinely caught up"
		// from "gate never engaged" from syncTargetHeight alone, so just keep
		// polling unconditionally for the whole bounded window instead —
		// each call is a couple of small HTTP GETs, negligible cost for the
		// duration below.
		SafeGoroutine("fetchAndSetSyncTarget", func() {
			dag.fetchAndSetSyncTarget(seeds)
			ticker := time.NewTicker(syncTargetRefreshInterval)
			defer ticker.Stop()
			deadline := time.Now().Add(syncTargetRefreshMaxDuration)
			for range ticker.C {
				if time.Now().After(deadline) {
					return
				}
				dag.fetchAndSetSyncTarget(seeds)
			}
		})
	} else {
		fmt.Println("[PEERS] No PRIMARY_NODE_URL/PRIMARY_NODE_URLS configured — accepting registrations from peers")
	}

	SafeGoroutine("registerAndDiscover-ticker", func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-tick, not
			// just once for the whole goroutine — see safeCall's comment.
			SafeCall("registerAndDiscover-tick", func() {
				// Re-resolve every tick, not just at startup: seedURLs only
				// every depends on env vars (cheap), and re-reading means an
				// operator can add PRIMARY_NODE_URLS to an already-running node
				// (e.g. to recover from a single seed going down) without a
				// restart.
				for _, seed := range seedURLs(selfURL) {
					dag.registerAndDiscover(selfURL, seed)
				}
				dag.healSyntheticCheckpoints()
			})
		}
	})
}

// healSyntheticCheckpoints actively tries to replace every synthetic-
// checkpoint stub this node currently trusts with the real block, by
// batch-fetching their hashes from every active sync peer (scale audit /
// "no fake blocks" fix). Without this, AddPeerBlock's new stub-healing
// support (block.go) only fires passively — i.e. only if a real block
// happens to arrive unprompted via gossip or a forward-sync page that
// happens to cover it — which could mean a node stays stuck trusting a
// placeholder indefinitely even once a peer with the real history is
// available. Cheap to call every tick: SyntheticCheckpointHashes itself
// short-circuits via the atomic counter when there's nothing to heal, which
// is the overwhelmingly common case.
func (dag *BlockDAG) healSyntheticCheckpoints() {
	hashes := dag.SyntheticCheckpointHashes()
	if len(hashes) == 0 {
		return
	}
	dag.syncPeerMu.Lock()
	peers := make([]string, 0, len(dag.activeSyncPeers))
	for p := range dag.activeSyncPeers {
		peers = append(peers, p)
	}
	dag.syncPeerMu.Unlock()
	healed := 0
	for _, peerURL := range peers {
		for i := 0; i < len(hashes); i += maxBlocksByHashPerRequest {
			chunk := hashes[i:min(i+maxBlocksByHashPerRequest, len(hashes))]
			blocks, err := dag.fetchBlocksByHashes(peerURL, chunk)
			if err != nil || len(blocks) == 0 {
				continue
			}
			isTrusted := dag.isTrustedSyncSource(peerURL)
			for _, b := range blocks {
				// SECURITY (P0, launch audit 2026-07-03): see isTrustedSyncSource.
				b.FromSync = isTrusted
				// FIX (durable fix, 2026-07-04): healing a synthetic checkpoint
				// is also a deliberate, self-initiated fetch — see
				// SelfFetched's own comment (block.go).
				b.SelfFetched = true
				if dag.AddPeerBlock(b) {
					healed++
				}
			}
		}
	}
	if healed > 0 {
		fmt.Printf("[BLOCK] ✓ Healed %d synthetic-checkpoint stub(s) with real data from peers (%d still active)\n", healed, dag.SyntheticCheckpointCount())
	}
}

// registerAndDiscover POSTs our URL and signing address to the primary's
// /api/peers/register. The primary adds our signing address to its authorized
// validator set so our blocks are accepted without any manual configuration.
// We receive the peer list and the current authorized validator addresses back.
// fetchAndSignPeerChallenge implements the same manual flow the node
// operator UI already documents (GET a challenge, sign it, send the
// signature back) automatically: fetches a fresh challenge for signerAddr
// from primaryURL and signs it with signingKey using the personal_sign /
// "Ethereum Signed Message" scheme VerifyPeerChallenge expects (see
// block.go). Returns "" (not an error) on any failure — signing is a
// best-effort upgrade over PEER_SECRET, not a hard requirement, since a
// node with PEER_SECRET configured should keep working exactly as before
// even if the challenge round-trip fails for some transient reason.
func fetchAndSignPeerChallenge(primaryURL, signerAddr string, signingKey *ecdsa.PrivateKey) string {
	if signingKey == nil || signerAddr == "" {
		return ""
	}
	resp, err := httpSyncClient.Get(primaryURL + "/api/peers/challenge?address=" + signerAddr)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	var result struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Challenge == "" {
		return ""
	}
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(result.Challenge), result.Challenge)
	hash := crypto.Keccak256Hash([]byte(msg))
	sig, err := crypto.Sign(hash.Bytes(), signingKey)
	if err != nil {
		return ""
	}
	return "0x" + hex.EncodeToString(sig)
}

// registerAndDiscover POSTs our URL and signing address to the primary's
// /api/peers/register, automatically proving private-key ownership via a
// signed challenge (see fetchAndSignPeerChallenge) rather than relying on
// PEER_SECRET alone.
//
// FIX (decentralization): this used to send peer_secret but never a
// signature, even though api.go's handlePeerRegister already accepts EITHER
// a matching PEER_SECRET OR a valid challenge-response signature
// (secretOK || sigOK) — the signature path existed only for the
// manual/operator-documented flow, never wired into the automatic one. In
// practice this meant a single shared secret was the ONLY thing that
// determined whether a new node could join: leaking it lets anyone register
// as a peer, and rotating it (e.g. after a leak) breaks every legitimate
// node's auto-join until each one is individually updated. Every node that
// can sign its own blocks (RELAYER_PRIVATE_KEY, required for block
// production anyway) can now prove ownership of its signing address the
// same way the manual flow always could, making PEER_SECRET an optional
// bootstrap fallback instead of the only practical path.
// registerAndDiscover returns whether registration succeeded, so callers can
// retry a transient failure (rate limit, network blip) instead of leaving
// this node permanently stuck — see the retry loop in StartPeerDiscovery.
func (dag *BlockDAG) registerAndDiscover(selfURL, primaryURL string) bool {
	signerAddr := ""
	if dag.signingKey != nil {
		signerAddr = strings.ToLower(crypto.PubkeyToAddress(dag.signingKey.PublicKey).Hex())
	}
	signature := fetchAndSignPeerChallenge(primaryURL, signerAddr, dag.signingKey)

	// Resolve the operator binding signature. Prefer the explicit env var
	// (set manually via /node-binding for cases where the operator wallet
	// is separate from the RELAYER key). Auto-sign when the two coincide:
	// if NODE_OPERATOR_WALLET matches the RELAYER address (same private
	// key), the node can produce the EIP-191 binding proof itself without
	// the operator doing anything out-of-band. This is the common
	// single-key deployment pattern.
	operatorBindingSig := os.Getenv("NODE_OPERATOR_BINDING_SIGNATURE")
	if operatorBindingSig == "" && dag.signingKey != nil && signerAddr != "" {
		nodeWallet := strings.ToLower(strings.TrimSpace(os.Getenv("NODE_OPERATOR_WALLET")))
		if nodeWallet == "" {
			nodeWallet = signerAddr
		}
		if nodeWallet == signerAddr {
			bindingMsg := "Aequitas: authorize validator " + signerAddr
			msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(bindingMsg), bindingMsg)
			hash := crypto.Keccak256Hash([]byte(msg))
			if sig, err := crypto.Sign(hash.Bytes(), dag.signingKey); err == nil {
				operatorBindingSig = "0x" + hex.EncodeToString(sig)
			}
		}
	}

	body, _ := json.Marshal(map[string]string{
		"url":                        selfURL,
		"signing_address":            signerAddr,
		"signature":                  signature,
		"peer_secret":                os.Getenv("PEER_SECRET"),
		"node_operator_wallet":       strings.ToLower(os.Getenv("NODE_OPERATOR_WALLET")),
		"operator_binding_signature": operatorBindingSig,
	})
	resp, err := httpSyncClient.Post(
		primaryURL+"/api/peers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[PEERS] Could not reach primary %s: %v\n", primaryURL, err)
		return false
	}
	defer resp.Body.Close()
	// FIX: this used to decode the response body unconditionally, regardless
	// of HTTP status. If the primary rejects registration (e.g. 403 because
	// NODE_OPERATOR_WALLET isn't a registered human yet), the body is
	// {"error":"..."} — decoding that into {Peers, Validators} silently
	// yields two empty slices with no error. The node then never learns the
	// primary's proposer address as an authorized validator, so every block
	// from the primary gets rejected by AddPeerBlock's "not an authorized
	// validator" check forever — visible only as "stuck at block 1", with no
	// indication why, since the actual rejection reason (this 403) was never
	// logged on the secondary's side at all.
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// FIX (durable fix, 2026-07-09): this used to be a one-shot attempt
		// with no retry anywhere — a transient failure (rate limit, brief
		// network blip) left the node permanently stuck with no peer/
		// validator discovery from this seed until an operator manually
		// restarted it. Confirmed live: a registration request that hit
		// this primary's own 30s-per-IP rate limit (itself triggered by
		// a burst of restarts in a short window) meant this node never
		// learned the OTHER secondary's peer URL via this seed's response
		// for the rest of its uptime — startSyncForPeer(primaryURL) below
		// still runs regardless and keeps pulling this seed's own blocks,
		// but peer discovery of anyone ELSE only this seed knows about
		// stayed stuck. Now returns false so StartPeerDiscovery's retry
		// loop tries again with backoff instead of accepting this as final.
		fmt.Printf("[PEERS] ✗ Registration with primary %s rejected (HTTP %d): %s — will retry with backoff (check NODE_OPERATOR_WALLET is a registered human, and PEER_SECRET/signature, if this keeps failing)\n",
			primaryURL, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
		return false
	}
	var result struct {
		Peers      []string `json:"peers"`
		Validators []string `json:"validators"`
	}
	json.Unmarshal(bodyBytes, &result)

	// FIX (audit 2026-07-06): a seed never lists itself in its own /api/peers
	// response (see StartPeerDiscovery's comment — that response only
	// contains OTHER registered secondary nodes, since a seed has no need to
	// sync from itself). That meant this node's own GlobalPeerRegistry —
	// and therefore its own /api/peers, used by e.g. the explorer's topology
	// view — never included the seed itself, even though registration just
	// succeeded and startSyncForPeer(primaryURL) below keeps it as an active
	// block-sync source. Register it explicitly now that we know it's live.
	primaryURLTrimmed := strings.TrimRight(primaryURL, "/")
	if primaryURLTrimmed != selfURL {
		GlobalPeerRegistry.Register(primaryURLTrimmed)
	}

	// Add newly discovered authorized validators to our local set so we
	// accept blocks from them without requiring AUTHORIZED_VALIDATORS env var.
	dag.mu.Lock()
	for _, addr := range result.Validators {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr != "" && !dag.authorizedValidators[addr] {
			dag.authorizedValidators[addr] = true
			fmt.Printf("[PEERS] Auto-authorized validator: %s\n", addr)
		}
	}
	dag.mu.Unlock()

	for _, peer := range result.Peers {
		peer = strings.TrimRight(peer, "/")
		if peer == selfURL {
			continue
		}
		GlobalPeerRegistry.Register(peer)
		dag.startSyncForPeer(peer)
	}
	return true
}

// isAllowedPeerURL returns true for URLs pointing to public IP addresses.
// HTTPS is preferred; HTTP is accepted for literal public IP addresses
// (e.g. http://173.249.37.118:8080) so VPS nodes without a domain can
// participate. HTTP with a hostname is still rejected (DNS-rebinding risk).
func isAllowedPeerURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "" || host == "0.0.0.0" || host == "[::]" {
		return false
	}

	// Literal IP: allow HTTP or HTTPS as long as the IP is public.
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast() && !ip.IsUnspecified()
	}

	// Hostname: require HTTPS to prevent DNS-rebinding attacks.
	if u.Scheme != "https" {
		return false
	}

	// FIX 10: DNS lookup removed from isAllowedPeerURL to eliminate TOCTOU race
	// (DNS may resolve differently at connect time vs. check time, enabling
	// DNS-rebinding). The actual IP validation is authoritative in pinningDialer,
	// which resolves DNS once and pins the connection to the resolved IP.
	// String-level checks for obviously private literal IPs are still done above.
	return true
}

// staticPeers reads the PEER_NODES env var for backwards compatibility.
func staticPeers(selfURL string) []string {
	raw := os.Getenv("PEER_NODES")
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.TrimRight(p, "/"))
		if p != "" && p != selfURL {
			out = append(out, p)
		}
	}
	return out
}

// StartHTTPBlockSync is an alias kept for call-site compatibility.
func (dag *BlockDAG) StartHTTPBlockSync(selfURL string) {
	dag.StartPeerDiscovery(selfURL)
}

// syncTargetRefreshInterval is how often StartPeerDiscovery's goroutine
// re-queries the seeds' current height while this node hasn't caught up yet
// (see that call site's own FIX comment) — frequent enough that the target
// stays close to the seed's real live tip throughout a slow boot sequence
// (P2P setup, rate-limited registration retries), not just the single
// instant peer discovery began. Matched to BLOCK_TIME's 1s floor, not a
// slower round number: ProduceBlock's gate (block.go, "dag.height >=
// target-10") checks the CURRENT value of a target that is only ever as
// fresh as the last refresh — any interval slower than production's own
// cadence reopens a window where a stale target lets a premature block
// through before the next refresh corrects it, which is exactly how the
// live fork this fix closes was produced (within the first 1-3 blocks of
// boot). The gate's own 10-block buffer already covers the residual one-tick
// lag this cadence still allows.
const syncTargetRefreshInterval = 1 * time.Second

// syncTargetRefreshMaxDuration bounds the refresh goroutine's own lifetime —
// see its call site's own comment. Generous margin above syncStallTimeout
// (90s): that gate already lets ProduceBlock resume independently long
// before this fires; this only guarantees the goroutine itself eventually
// stops polling a seed that never becomes reachable.
const syncTargetRefreshMaxDuration = 10 * time.Minute

// fetchAndSetSyncTarget queries each seed's /api/health/combined for its
// current block height and sets syncTargetHeight to the maximum found.
// ProduceBlock defers production until dag.height is within 10 of this
// target so a restarting node never produces on a stale fork while the
// sync loop is still catching up. Called in a goroutine from StartPeerDiscovery.
func (dag *BlockDAG) fetchAndSetSyncTarget(seeds []string) {
	type healthResp struct {
		Chain struct {
			Height int64 `json:"height"`
		} `json:"chain"`
	}
	var maxHeight int64
	caughtUpWithEverySeed := true
	for _, seed := range seeds {
		resp, err := httpSyncClient.Get(seed + "/api/health/combined")
		if err != nil {
			continue
		}
		var h healthResp
		if err := json.NewDecoder(resp.Body).Decode(&h); err == nil {
			if h.Chain.Height > maxHeight {
				maxHeight = h.Chain.Height
			}
			// FIX (P0, 2026-07-10 — remaining gap after the refresh-cadence
			// fix above): comparing against dag.Height() (raw dag.height,
			// which THIS node's own self-production also advances) let a
			// node that had started producing on still-incomplete data
			// silently look "caught up" the moment its own block count
			// happened to reach the seed's last-observed height — exactly
			// the same self-production-races-ahead hazard doSyncOnce's own
			// peerSyncHeight already exists to avoid (see that field's
			// struct comment). getPeerSyncHeight tracks blocks genuinely
			// PULLED from this specific seed, immune to this node's own
			// production rate, so it stays a faithful signal of real
			// historical absorption through the exact window this gate
			// protects. Confirmed live: even with the 1s refresh cadence,
			// Contabo1 still forked ~30 blocks after every fresh checkpoint
			// once self-production started outracing the dag.Height()
			// comparison.
			if h.Chain.Height > dag.getPeerSyncHeight(seed) {
				caughtUpWithEverySeed = false
			}
		}
		resp.Body.Close()
	}
	if caughtUpWithEverySeed {
		return // genuinely absorbed every seed's chain — no gate needed
	}
	dag.syncTargetHeight.Store(maxHeight)
	fmt.Printf("[SYNC] Initial-sync gate active: deferring block production until height %d (currently %d)\n",
		maxHeight, dag.Height())
}

// HTTPBroadcastBlock pushes a freshly-produced block to every active HTTP
// peer via POST /api/blocks/push. This is the HTTP-level complement to the
// libp2p BroadcastBlock call — it works even when port 4001 is firewalled
// (common on Railway). Each peer receives the block within one HTTP round
// trip (~100ms) and inserts it into its dag.tips immediately, so the next
// ProduceBlock tick at every peer includes this block as a parent, producing
// a genuine multi-parent GHOSTDAG merge.
func (dag *BlockDAG) HTTPBroadcastBlock(block *Block) {
	dag.syncPeerMu.Lock()
	peers := make([]string, 0, len(dag.activeSyncPeers))
	for p := range dag.activeSyncPeers {
		peers = append(peers, p)
	}
	dag.syncPeerMu.Unlock()

	if len(peers) == 0 {
		return
	}

	data, err := json.Marshal(block)
	if err != nil {
		return
	}

	for _, peerURL := range peers {
		peerURL := peerURL
		go func() {
			// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go.
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[PANIC RECOVERED] block-push goroutine to %s: %v\n%s\n", peerURL, r, debug.Stack())
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, peerURL+"/api/blocks/push", bytes.NewReader(data))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpSyncClient.Do(req)
			if err != nil {
				fmt.Printf("[BLOCK-PUSH] ✗ HTTP push block #%d to %s: %v\n", block.Height, peerURL, err)
				return
			}
			defer resp.Body.Close()
			// P0 fix (2026-07-02 liveness audit follow-up): read the response
			// instead of discarding it. A peer that rejects our block because
			// OUR proposer's breaker is open on ITS side is telling us WE may
			// be the diverged one — see handleBlockPush (api.go) for where
			// this signal originates. React through the same, already-gated
			// auto-heal path StartDivergenceAutoHeal uses (autoheal.go),
			// re-checking its exact safety gate here since triggerAutoResync
			// itself has none: without it, a single malicious/misconfigured
			// peer's response could force ANY node — including Primary,
			// which must never self-wipe — through os.Exit(1).
			//
			// FIX (P0, 2026-07-04 brutal audit — "sender ignores almost all
			// push rejections"): this used to look at Action alone, silently
			// discarding OK and Reason. A peer whose pushes keep getting
			// rejected for a real reason (not the benign, expected-to-
			// self-resolve "orphaned, within grace period" case) never
			// surfaced anywhere — a genuine, sustained divergence signal was
			// invisible. See pushRejectStreak's own comment for the reaction.
			var pushResp struct {
				OK     bool   `json:"ok"`
				Reason string `json:"reason"`
				Action string `json:"action"`
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
			if err != nil || json.Unmarshal(body, &pushResp) != nil {
				return
			}
			if pushResp.Action == "resync_required" {
				if !dag.recordResyncSignal(peerURL) {
					return // fewer than the current (peer-count-scaled) threshold so far
				}
				bootstrapURL := os.Getenv("BOOTSTRAP_SNAPSHOT_URL")
				bootstrapSigner := os.Getenv("BOOTSTRAP_SIGNER")
				if os.Getenv("AUTO_HEAL_ON_DIVERGENCE") != "true" || bootstrapURL == "" || bootstrapSigner == "" {
					return // same gate StartDivergenceAutoHeal requires — Primary has neither, so this is always a no-op there
				}
				dag.triggerAutoResync(fmt.Sprintf(
					"%d distinct peers signaled resync_required after rejecting our pushed blocks",
					dag.resyncSignalThresholdFor(os.Getenv("SELF_URL"))))
				return
			}
			dag.recordPushRejection(peerURL, pushResp.OK, pushResp.Reason)
		}()
	}
}

// pushRejectStreakThreshold bounds how many CONSECUTIVE non-benign push
// rejections from the SAME peer this node tolerates silently before
// treating it as a genuine divergence signal — roughly 2 minutes of
// sustained rejection at BLOCK_TIME=6s, well past ordinary propagation
// jitter. "orphaned, within grace period" never counts (see
// recordPushRejection): that case is expected to self-resolve via the
// normal pull-sync cycle and isn't evidence of anything wrong.
const pushRejectStreakThreshold = 20

// recordPushRejection is HTTPBroadcastBlock's reaction to a push response
// beyond the already-handled resync_required case — see that call site's
// FIX comment for the incident this closes. accepted=true (ok:true) clears
// the peer's streak. A benign "orphaned, within grace period" is ignored
// entirely: it is the expected shape of ordinary cross-network propagation
// lag, not evidence of divergence (see proposerBreakerOrphanGrace's
// identical reasoning on the receiving side, block.go). Any OTHER rejection
// reason (a genuine orphan past its grace period, or the ambiguous
// "rejected or already known" catch-all) advances a per-peer streak; once
// pushRejectStreakThreshold is crossed, this is exactly as strong a
// divergence signal as an explicit resync_required, so it feeds the SAME
// already-tested, already-safety-gated recordResyncSignal/triggerAutoResync
// path rather than inventing a new, separately-risked reaction.
func (dag *BlockDAG) recordPushRejection(peerURL string, accepted bool, reason string) {
	dag.pushRejectStreakMu.Lock()
	if accepted {
		delete(dag.pushRejectStreak, peerURL)
		dag.pushRejectStreakMu.Unlock()
		return
	}
	if reason == "orphaned, within grace period" {
		dag.pushRejectStreakMu.Unlock()
		return
	}
	if dag.pushRejectStreak == nil {
		dag.pushRejectStreak = make(map[string]int)
	}
	dag.pushRejectStreak[peerURL]++
	streak := dag.pushRejectStreak[peerURL]
	dag.pushRejectStreakMu.Unlock()

	nowNano := time.Now().UnixNano()
	if last := dag.lastPushRejectLogAt.Load(); nowNano-last > int64(time.Second) && dag.lastPushRejectLogAt.CompareAndSwap(last, nowNano) {
		fmt.Printf("[BLOCK-PUSH] ⚠ %s rejected our pushed block (%q) — %d consecutive non-benign rejection(s) (rate-limited)\n", peerURL, reason, streak)
	}
	if streak < pushRejectStreakThreshold {
		return
	}
	dag.pushRejectStreakMu.Lock()
	delete(dag.pushRejectStreak, peerURL) // reset — about to act on it, don't re-fire every push until it re-accumulates
	dag.pushRejectStreakMu.Unlock()

	if !dag.recordResyncSignal(peerURL) {
		return // fewer than the current (peer-count-scaled) threshold so far
	}
	bootstrapURL := os.Getenv("BOOTSTRAP_SNAPSHOT_URL")
	bootstrapSigner := os.Getenv("BOOTSTRAP_SIGNER")
	if os.Getenv("AUTO_HEAL_ON_DIVERGENCE") != "true" || bootstrapURL == "" || bootstrapSigner == "" {
		return // same gate StartDivergenceAutoHeal requires — Primary has neither, so this is always a no-op there
	}
	dag.triggerAutoResync(fmt.Sprintf(
		"%d distinct peers signaled resync_required after rejecting our pushed blocks",
		dag.resyncSignalThresholdFor(os.Getenv("SELF_URL"))))
}

// resyncSignalWindow gates recordResyncSignal's time window — see its own
// comment and HTTPBroadcastBlock's reaction above. minResyncSignalThreshold
// is resyncSignalThresholdFor's floor — see that function's own comment for
// why the real threshold now scales with peer count instead of being fixed.
const (
	resyncSignalWindow       = 60 * time.Second
	minResyncSignalThreshold = 2 // distinct peers within the window, at any peer count
)

// resyncSignalThresholdFor returns how many distinct peers must signal
// resync_required within resyncSignalWindow before this node treats it as a
// genuine divergence signal, scaled to a MAJORITY of currently-known active
// peers rather than a fixed absolute count.
//
// FIX (performance audit 2026-07-06): a fixed threshold of 2 is a
// reasonable, already-proven-safe bar at today's 3-node scale — with only
// 2 other peers possible, it already requires effectively unanimous peer
// agreement before triggering an authoritative, state-replacing resync.
// But the SAME fixed 2 would become a dangerously WEAK bar at a much
// larger peer target (2 out of 100+ known peers), letting a small minority
// of confused or malicious peers force any node through a resync. Never
// below minResyncSignalThreshold — this preserves today's exact behavior
// when the peer count is small (2 known peers -> threshold 2, identical to
// the constant it replaces) and never demands more peers than are known to
// exist, so the signal can't become permanently unreachable.
func (dag *BlockDAG) resyncSignalThresholdFor(selfURL string) int {
	known := len(GlobalPeerRegistry.ActivePeers(selfURL))
	majority := known/2 + 1
	if majority < minResyncSignalThreshold {
		return minResyncSignalThreshold
	}
	return majority
}

// recordResyncSignal records that peerURL just told this node
// action:"resync_required" in response to a block this node pushed, and
// reports whether resyncSignalThresholdFor's current threshold of distinct
// peers has now signaled within resyncSignalWindow. Re-signaling from the
// same peer only refreshes its timestamp — it can never count twice toward
// the threshold, so a single peer can never trigger this alone. Own mutex,
// never dag.mu.
func (dag *BlockDAG) recordResyncSignal(peerURL string) bool {
	dag.resyncSignalMu.Lock()
	defer dag.resyncSignalMu.Unlock()
	if dag.resyncSignalFrom == nil {
		dag.resyncSignalFrom = make(map[string]int64)
	}
	now := time.Now().Unix()
	dag.resyncSignalFrom[peerURL] = now
	distinct := 0
	for _, at := range dag.resyncSignalFrom {
		if now-at <= int64(resyncSignalWindow.Seconds()) {
			distinct++
		}
	}
	return distinct >= dag.resyncSignalThresholdFor(os.Getenv("SELF_URL"))
}
