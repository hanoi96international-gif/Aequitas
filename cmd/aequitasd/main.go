package main

import (
"context"
"encoding/json"
"fmt"
"net"
"net/url"
"os"
"os/signal"
"strings"
"syscall"
"time"
_ "time/tzdata" // embed IANA timezone DB so Europe/Berlin works on Alpine without system tzdata

"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

// ── ENVIRONMENT VARIABLES ──────────────────────────────────────────────────
// Setup-simplification audit: this node reads ~30 env vars total, but the
// overwhelming majority are advanced/recovery-only and must never be touched
// by someone just standing up a validator node on their own VPS (Contabo or
// similar -- Railway is this project's test/demo deployment, not the target
// for "any human can run a node"). Tiered here once as the single source of
// truth for the node operator guide (see api_html.go's Network tab and
// build/generate_all_guides.py).
//
// REQUIRED (3 vars -- a fresh VPS, a fresh Postgres, nothing else):
//   DATABASE_URL        Postgres connection string for this node's own DB.
//   SELF_URL             This node's own public URL (e.g. https://1.2.3.4:8080
//                         or a domain you point at the VPS). Needed so peers
//                         can be told where to reach you.
//   NODE_OPERATOR_WALLET Your own wallet address -- MUST belong to a
//                         verified human (completed biometric registration
//                         first). This is what enforces one human = one
//                         validator: rewards are tied to a verified person,
//                         not to however many servers someone can rent.
//                         Recommended: also set RELAYER_PRIVATE_KEY to this
//                         same wallet's private key (the simple single-key
//                         setup) -- see RELAYER_PRIVATE_KEY below.
//
// AUTO-GENERATED ON FIRST RUN IF UNSET -- save the printed value, then set
// it explicitly so your node's identity survives a restart:
//   NODE_KEY              P2P network identity.
//   RELAYER_PRIVATE_KEY   Block-signing key (0x... hex). If this is also
//                          your NODE_OPERATOR_WALLET's own private key,
//                          rewards and block-signing use the same identity
//                          with no extra configuration (recommended). A
//                          freshly generated key still produces valid
//                          signed blocks with zero setup -- just bind
//                          NODE_OPERATOR_WALLET to it afterward if you want
//                          this same node to earn validator rewards.
//
// AUTO-DEFAULTED -- only set these to override the default:
//   PRIMARY_NODE_URL / PRIMARY_NODE_URLS   Defaults to the public Aequitas
//                          network. Only set if running a private/test
//                          deployment that must not reach the public chain.
//   RELAYER_ADDRESS       Derived automatically from RELAYER_PRIVATE_KEY.
//                          Only set if your block-signing key and your
//                          contract-deploy-authorized key intentionally
//                          differ (advanced).
//   PEER_NODES             Legacy static comma-separated peer list. Not
//                          needed -- peer discovery is automatic.
//
// ADVANCED / SINGLE-OPERATOR-ONLY -- a normal validator node never sets these:
//   DISTRIBUTION_ENABLED   Set to "false" to opt this node out of running
//                          daily pool distributions. All nodes are eligible
//                          by default; cross-node dedup (last_ubi_at replay +
//                          pre-pass in replayTransactions) prevents double-
//                          credit even when multiple nodes fire simultaneously.
//   BOOTSTRAP_SNAPSHOT_URL, BOOTSTRAP_SIGNER   For fast state bootstrap from
//                          a snapshot instead of full historical replay.
//   SNAPSHOT_TOKEN, SNAPSHOT_MAX_AGE_SECONDS, SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK
//                          Snapshot serving/verification tuning.
//   PEER_SECRET, ALLOW_PEER_SECRET_BYPASS, ALLOW_SIGN_VALIDATOR_CHALLENGE
//                          Legacy/manual peer-auth fallbacks; the automatic
//                          signature-based flow covers normal operation.
//   AUTHORIZED_VALIDATORS  Auto-synced from peers; manual config not needed.
//   PROOF_SERVER_URL, CHAIN_SERVICE_TOKEN   Only relevant if integrating
//                          with the mobile-app registration backend.
//   RESET_DB_STATE, RESET_STATE, CLEAR_REGISTRATIONS,
//   CLEAR_REGISTRATIONS_CONFIRM, RESYNC_FROM_SNAPSHOT,
//   ALLOW_DESTRUCTIVE_MAINTENANCE, ALLOW_RUNTIME_ORPHAN_BRIDGE
//                          Destructive recovery operations. Each requires an
//                          explicit, separate opt-in flag and is refused by
//                          default -- never set these for routine operation.
//   LOG_LEVEL              Optional log verbosity (debug/info/warn/error).

const (
VERSION       = "v0.3.0"
// NOTE: the actually-active contract addresses (V6, V7, bio verifier) live
// in x/humanity/keeper/evm_v6mirror.go (V6_CONTRACT_ADDR, V7_CONTRACT_ADDR,
// BIO_VERIFIER_ADDR) — that is the single source of truth. Do not redeclare
// addresses here; a previous version of this file had a stale CONTRACT_V6
// and BIO_VERIFIER value that didn't match what was actually deployed and
// was never even referenced anywhere in this file.
INITIAL_GRANT = 1000
CHAIN_ID      = "aequitas-1"
BLOCK_TIME    = 2 * time.Second
API_PORT      = 8080
)

type Genesis struct {
ChainID     string      `json:"chain_id"`
GenesisTime string      `json:"genesis_time"`
AppState    interface{} `json:"app_state"`
}

func loadGenesis() (*Genesis, error) {
data, err := os.ReadFile("genesis.json")
if err != nil {
return nil, err
}
var genesis Genesis
err = json.Unmarshal(data, &genesis)
return &genesis, err
}

func main() {
fmt.Println("╔════════════════════════════════════════╗")
fmt.Println("║         AEQUITAS CHAIN NODE            ║")
fmt.Println("║      Proof of Humanity Consensus       ║")
fmt.Println("╚════════════════════════════════════════╝")
fmt.Println()
fmt.Printf("Version:       %s\n", VERSION)
fmt.Printf("Chain ID:      %s\n", CHAIN_ID)
fmt.Printf("Block Time:    %s\n", BLOCK_TIME)
fmt.Println()

fmt.Println("── Loading Genesis Block ────────────────")
genesis, err := loadGenesis()
if err != nil {
fmt.Printf("✗ Genesis error: %v\n", err)
} else {
fmt.Printf("✓ Chain ID: %s\n", genesis.ChainID)
fmt.Printf("✓ Genesis Time: %s\n", genesis.GenesisTime)
}
fmt.Println()

humanKeeper := keeper.NewKeeper()

fmt.Println()

fmt.Println("── Initializing Blockchain ──────────────")
p2pNode, err := keeper.NewP2PNode(humanKeeper)
if err != nil {
fmt.Printf("✗ P2P Error: %v\n", err)
return
}

chainState := keeper.NewChainState("/tmp/aequitas_state.json")
bc := keeper.NewBlockchain(humanKeeper, p2pNode.GetNodeID(), chainState)
// Load individually-registered validator keys from DB into the DAG's
// authorized set so they survive node restarts without re-registration.
chainState.LoadValidatorKeysIntoDAG(bc)
fmt.Println()

p2pNode.SetDAG(bc)

	// FIX: NewAPIServer must be constructed (and therefore NewEVMRPCServer,
	// which sets dag.evm) BEFORE bc.StartHTTPBlockSync below. StartHTTPBlockSync
	// launches goroutines that immediately start pulling and replaying peer
	// blocks; replayTransactions calls verifyZKProof, which checks dag.evm and
	// permanently rejects the proof ("EVM not initialized, rejecting ZK proof
	// for block safety") if it's still nil — and a block, once processed, is
	// marked in replayedBlocks and never retried even after the EVM becomes
	// ready moments later. This used to construct the API server (and thus the
	// EVM engine) AFTER StartHTTPBlockSync, which raced exactly this window:
	// confirmed in production where a register_human TX in an early synced
	// block was permanently skipped on a freshly-started secondary because the
	// EVM engine hadn't been wired up yet when that block was replayed.
	fmt.Println("── Starting API Server ──────────────────")
	api := keeper.NewAPIServer(bc, p2pNode, humanKeeper, chainState)
	go api.Start(API_PORT)
	fmt.Println()

	// Bootstrap from a peer snapshot if this is a fresh node (no humans in DB),
	// OR perform an authoritative resync if RESYNC_FROM_SNAPSHOT=true is set
	// explicitly (audit recheck2, P1 #9 — see ResyncFromSnapshotURL's own
	// comment for why a divergent node needs REPLACE semantics, not merge).
	// Set BOOTSTRAP_SNAPSHOT_URL to the primary node's /api/snapshot endpoint.
	// Set SNAPSHOT_TOKEN to match the primary node's SNAPSHOT_TOKEN env var.
	// Set BOOTSTRAP_SIGNER to the primary node's signing address (0x...) to
	// verify the snapshot's ECDSA signature before importing — mandatory for
	// resync mode regardless of this var's own emptiness check below.
	// After startup, ongoing state sync happens via block TX replay in AddPeerBlock.
	resyncMode := os.Getenv("RESYNC_FROM_SNAPSHOT") == "true"
	// FIX (2026-06-28, production incident): TotalHumans()==0 alone used to
	// trigger the "fresh node" bootstrap import — indistinguishable from a
	// node whose chain_accounts load failed at startup (see loadFromDB's
	// comment). A node WITH real history that hit a transient DB hiccup
	// looked exactly like a brand-new one and got bootstrap-imported on
	// every restart that hit the hiccup, repeatedly knocking its visible
	// height back toward genesis. AccountsLoadFailed() being true means
	// "we don't actually know if this node is fresh" — refuse to guess.
	// resyncMode is an explicit, deliberate operator action (not a guess
	// based on TotalHumans()), so it is NOT gated by this check.
	freshNodeBootstrap := chainState.TotalHumans() == 0 && !chainState.AccountsLoadFailed()
	if chainState.AccountsLoadFailed() && !resyncMode {
		fmt.Println("[BOOTSTRAP] ⚠ chain_accounts failed to load at startup — skipping fresh-node bootstrap check this run (this node's real history, if any, is presumed intact; a future successful restart will re-evaluate)")
	}
	resyncSucceeded := false
	if bootstrapURL := os.Getenv("BOOTSTRAP_SNAPSHOT_URL"); bootstrapURL != "" && (freshNodeBootstrap || resyncMode) {
		// FIX 15: Validate URL scheme and host before fetching to prevent SSRF.
		parsedBootstrap, urlErr := url.Parse(bootstrapURL)
		if urlErr != nil || (parsedBootstrap.Scheme != "https" && parsedBootstrap.Scheme != "http") {
			fmt.Printf("[BOOTSTRAP] ✗ Refused: BOOTSTRAP_SNAPSHOT_URL must be an http or https URL (got %q)\n", bootstrapURL)
		} else if host := parsedBootstrap.Hostname(); isRFC1918OrLoopback(host) {
			fmt.Printf("[BOOTSTRAP] ✗ Refused: BOOTSTRAP_SNAPSHOT_URL must not point to a private/loopback address (got %q)\n", host)
		} else {
			expectedSigner := os.Getenv("BOOTSTRAP_SIGNER")
			if expectedSigner == "" {
				fmt.Println("[BOOTSTRAP] ✗ Refused: BOOTSTRAP_SIGNER must be set to the primary node's signing address (0x...)")
			} else if resyncMode {
				fmt.Printf("[RESYNC] RESYNC_FROM_SNAPSHOT=true — replacing local state from %s\n", bootstrapURL)
				if err := chainState.ResyncFromSnapshotURL(bootstrapURL, expectedSigner); err != nil {
					fmt.Printf("[RESYNC] ✗ Resync failed: %v\n", err)
					// FIX (audit 2026-06-28 recheck 5, P1-3): surface this via
					// /api/health/combined instead of only a startup log line —
					// an operator checking health after the fact would otherwise
					// have no way to know the EVM mirror might be stale.
					chainState.SetBootstrapDegraded("resync failed: " + err.Error())
				} else {
					resyncSucceeded = true
				}
			} else {
				fmt.Printf("[BOOTSTRAP] Fresh node — importing state from %s\n", bootstrapURL)
				if err := chainState.ImportSnapshotFromURL(bootstrapURL, expectedSigner); err != nil {
					fmt.Printf("[BOOTSTRAP] ✗ Import failed: %v\n", err)
					chainState.SetBootstrapDegraded("snapshot import failed: " + err.Error())
				}
			}
		}
	}
	// FIX (2026-06-28, root cause of Contabo VPS's permanent post-resync
	// catch-up failure): bc was constructed above (before this whole
	// bootstrap/resync block ran) from whatever max_block_height existed in
	// the DB at that time — 0, on a freshly wiped DB. ImportSnapshotFromURL/
	// ResyncFromSnapshotURL just wrote the real height into the DB, but the
	// already-constructed bc never re-reads it on its own. Without this
	// call, bc.StartHTTPBlockSync below starts paging from height 0 and
	// walks the ENTIRE historical block backlog one HTTP page at a time —
	// see RefreshBootHeightAfterSnapshotImport's own comment for why that's
	// what caused the orphan-buffer abandonment storm during a large catch-up.
	bc.RefreshBootHeightAfterSnapshotImport(resyncSucceeded)

	// Bridge permanent historical gaps unconditionally on every startup.
	// A node may be stuck below bootHeight not only after a fresh RESYNC
	// but also after any restart where the gap pre-dates this run (e.g. a
	// secondary node stuck at dag.height=395, bootHeight=45361 because a
	// previous primary RESYNC wiped blocks 396-44367 from the network).
	// BridgeHistoricalGap returns immediately if bootHeight is within 100
	// blocks of dag.height, so calling it unconditionally is cheap.
	if pURL := strings.TrimRight(keeper.NormalizeNodeURL(os.Getenv("PRIMARY_NODE_URL")), "/"); pURL != "" {
		bc.BridgeHistoricalGap([]string{pURL})
	}

	// Save price snapshots every 30 seconds so the chart interval buttons
	// (1m/5m/30m/1h/4h) show meaningful historical data even without swaps.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			chainState.SavePriceSnapshot()
		}
	}()

	// HTTP Block Sync between nodes.
	// SELF_URL must be set to this node's own public URL so the sync loop
	// can exclude it from the peer list — without this, a node would try to
	// sync from itself and generate spurious hash-mismatch rejections.
	selfURL := os.Getenv("SELF_URL")
	if selfURL == "" {
		// P2-09 (audit): defaulting to "https://aequitas.digital" when SELF_URL
		// is unset caused secondary nodes that forgot the env var to register as
		// the primary domain — overwriting the primary's peer record and making
		// the real primary invisible to the network.  Now: if SELF_URL is not
		// set, run in isolated mode (no peer registration, no sync goroutine
		// started).  The node can still accept inbound connections and produce
		// blocks; it just won't actively register with any primary.
		fmt.Println("[WARN] SELF_URL not set — running in isolated mode (no peer registration). Set SELF_URL to this node's public URL to participate in peer discovery.")
	}
	// FIX: Railway's RAILWAY_PUBLIC_DOMAIN variable (commonly used to set
	// SELF_URL, e.g. SELF_URL=${{RAILWAY_PUBLIC_DOMAIN}}) never includes a
	// scheme — it's just "myservice.up.railway.app". A scheme-less SELF_URL
	// fails isAllowedPeerURL's "must be public HTTPS" check, so the primary
	// rejects this node's peer registration entirely (silently — the node
	// itself prints nothing wrong, only the PRIMARY's logs show "URL
	// rejected"). Confirmed in production: a secondary's SELF_URL kept
	// reverting to the bare hostname even after manually adding "https://"
	// in Railway's UI. Normalize here instead of fighting that — any
	// SELF_URL without an http(s) scheme gets "https://" prepended.
	selfURL = keeper.NormalizeNodeURL(selfURL)
	bc.StartHTTPBlockSync(selfURL)
	p2pNode.Start()
	bc.ReconstructState(chainState)

// Humans register natively via the V7 contract (see register.go) and are
// reconstructed from blockchain transactions above via ReconstructState.
// An old Sepolia-polling sync (humanKeeper.StartSync(), in the now-removed
// sync.go) used to inject placeholder "sepolia_human_N" entries into the
// keeper on every tick — exactly the fake registrations that were
// deliberately removed earlier in this project. That code path is gone
// now, not just disabled, so it can't be accidentally re-enabled.

multiaddr := p2pNode.GetMultiaddr()
fmt.Println("── Share this address to join network ───")
fmt.Printf("%s\n", multiaddr)
fmt.Println()

// REVERTED: block production was temporarily gated to the primary only
// (PRIMARY_NODE_URL check) as a workaround for secondaries' self-produced
// blocks going nowhere and permanently forking their local chain. The
// actual root cause was a stale hardcoded P2P bootstrap address (see
// BootstrapNode in p2p.go) — every node's P2P bootstrap dial timed out
// forever, so BroadcastBlock had no connected peers to send to. Gating
// production defeats the entire point of a BlockDAG, which is meant to
// scale via MULTIPLE validators producing in parallel and merging — not
// funnel everything through one proposer like a plain linear chain. With
// the bootstrap address fixed, every node produces its own blocks again;
// genuine multi-parent merges in the DAG are the expected, healthy
// outcome of concurrent production, not a bug to engineer away.
fmt.Println("── Starting Block Production ────────────")
go func() {
ticker := time.NewTicker(BLOCK_TIME)
for range ticker.C {
block := bc.ProduceBlock()
			if block == nil {
				continue // catch-up gate — skip this tick
			}
			p2pNode.BroadcastBlock(block)
fmt.Printf("[Block #%d] Hash: %s... | Humans: %d | Time: %s\n",
block.Height,
block.Hash[:16],
block.Humans,
time.Unix(block.Timestamp, 0).Format("15:04:05"),
)
	}
	}()

fmt.Println("── Starting Daily Pool Distributions ───")
// FIX (P0, independent audit recheck 2026-06-27): the comment this replaced
// claimed TryLockDistribution's PostgreSQL CAS lock "ensures only ONE node
// actually executes" — that's false. Each node has its OWN, separate
// Postgres database (by design — see CLAUDE.md/AGENTS.md architecture
// notes), so the CAS lock only ever prevents the SAME node re-running
// within 24h. It provides ZERO cross-node coordination. Every node was
// independently winning its own local lock and running the full
// distribution (UBI + validator pool + LP pool + escrow) on its own
// state, then ALSO replaying the one TX that existed (ubi_distribution)
// on top — multiplying distribution by however many nodes are running.
// FIX 11: Create a cancellable context for the distribution goroutine so
// it can be cleanly stopped on node shutdown.
distCtx, distCancel := context.WithCancel(context.Background())
_ = distCancel // called in shutdown handler below
// FIX 6: distDone is closed when the goroutine exits so the shutdown handler
// can wait for any in-progress distribution to finish before the process exits.
distDone := make(chan struct{})
go func(ctx context.Context) {
	defer close(distDone)
berlin, err := time.LoadLocation("Europe/Berlin")
if err != nil {
berlin = time.FixedZone("CET", 2*60*60) // CEST fallback (summer, UTC+2)
}

nextDaily20 := func(after time.Time) time.Time {
t := after.In(berlin)
candidate := time.Date(t.Year(), t.Month(), t.Day(), 20, 0, 0, 0, berlin)
if !after.Before(candidate) {
// Use AddDate to get exactly 20:00 next day regardless of DST transitions.
// Add(24h) would be 1h off on the two DST changeover nights per year.
candidate = time.Date(t.Year(), t.Month(), t.Day()+1, 20, 0, 0, 0, berlin)
}
return candidate
}

lastAt := chainState.GetLastUBIAt()
var firstTarget time.Time
// FIX 10: Apply the same 1-hour guard for fresh DBs (lastAt == 0) to prevent
// the distribution from firing immediately on a brand-new node startup.
if lastAt == 0 || time.Since(time.Unix(lastAt, 0)) < time.Hour {
firstTarget = nextDaily20(time.Now().Add(time.Hour))
} else {
firstTarget = nextDaily20(time.Now())
}

firstDelay := time.Until(firstTarget)
chainState.SetNextUBIAt(firstTarget.Unix())
fmt.Printf("[POOLS] Next distribution at %s Berlin time (in %s)\n",
firstTarget.In(berlin).Format("02.01. 15:04:05"), firstDelay.Round(time.Minute))

for {
// FIX 11: Use a select so the goroutine unblocks on shutdown.
select {
case <-ctx.Done():
return
case <-time.After(time.Until(firstTarget)):
}
// Decentralized distribution: every node is eligible to trigger the
// daily round. Three dedup layers prevent double-credit:
//   1. TryLockDistribution: cross-node guard reads last_ubi_at (set by
//      TX replay) — if another node already ran this round and its block
//      propagated, this node returns false immediately. Intra-node CAS
//      on distribution_lock_at prevents the goroutine firing twice.
//   2. distributionSyncHealthIssue: nodes with incomplete history skip
//      rather than distribute with wrong validator reward weights.
//   3. replayTransactions pre-pass: when replaying a block from a
//      competing node, skips all distribution TXs if last_ubi_at is
//      within 24h of the block's DistributionAt (same-round window).
// Set DISTRIBUTION_ENABLED=false to opt a specific node out.
if os.Getenv("DISTRIBUTION_ENABLED") == "false" {
	fmt.Println("[POOLS] Distribution disabled on this node (DISTRIBUTION_ENABLED=false)")
} else if syncIssue := distributionSyncHealthIssue(bc); syncIssue != "" {
	// FIX (scale audit, SPOF): DistributeValidatorsPool weights every
	// validator's reward by registered_nodes.blocks_produced read from
	// THIS node's own local Postgres database (state.go) — there is no
	// cross-node reconciliation. If this node's own view of the chain is
	// known to be incomplete (active synthetic-checkpoint stubs means it
	// is trusting a placeholder instead of real history for at least one
	// span of blocks; degraded means a prior persistence failure left
	// memory ahead of what's durably saved), distributing now would
	// silently under-credit -- or in the synthetic-checkpoint case,
	// possibly never even SEE -- whatever validator produced blocks in
	// the gap this node doesn't actually have. Skip this round rather
	// than confidently distribute wrong amounts; the next scheduled
	// round re-checks. This does NOT consume TryLockDistribution's 24h
	// window (the check runs before attempting the lock), so a node that
	// heals mid-day is not stuck waiting for tomorrow.
	fmt.Printf("[POOLS] ✗ Skipping distribution this round: %s — will re-check at the next scheduled time\n", syncIssue)
} else if chainState.TryLockDistribution() {
	// FIX (audit3, P0 #3): the entire distribution round — UBI, validator
	// pool, LP pool, escrow move/release, AND every resulting outbox TX —
	// now runs as ONE all-or-nothing DB transaction via
	// RunDailyDistributionAtomic (state.go). The previous version ran each
	// sub-step as its own immediately-committing operation and only
	// serialized them against ProduceBlock's ticker via
	// WithBlockProductionPaused — that closed the block-timing race but
	// never made the mutations and their outbox TXs atomic with each other:
	// a crash or DB error between any mutation and its SavePendingTx call
	// still produced state no other node could ever replay, and an outbox
	// failure was only ever logged as an ALERT with an in-memory fallback,
	// never rolled back. WithBlockProductionPaused is kept wrapping this
	// call as defense-in-depth (it blocks ProduceBlock from even starting
	// tip/parent selection during this window, not just the cs.mu-guarded
	// part RunDailyDistributionAtomic itself already serializes against).
	bc.WithBlockProductionPaused(func() {
		ubiAt := time.Now().Unix()
		if err := chainState.RunDailyDistributionAtomic(ubiAt); err != nil {
			fmt.Printf("[POOLS] ✗ Distribution FAILED and was fully rolled back: %v\n", err)
			return
		}
		fmt.Printf("[POOLS] ✓ Distribution done at %s\n", time.Now().In(berlin).Format("02.01. 15:04:05"))
	})
} else {
	fmt.Printf("[POOLS] Distribution already ran within the last 24h on this or another node — skipping\n")
}
firstTarget = nextDaily20(time.Now())
chainState.SetNextUBIAt(firstTarget.Unix())
fmt.Printf("[POOLS] Next distribution at %s Berlin time\n", firstTarget.In(berlin).Format("02.01. 15:04:05"))
}
}(distCtx)

// Register this node's operator wallet so it participates in validator
// pool distributions. Any node that sets NODE_OPERATOR_WALLET gets
// included automatically — no code change needed when new nodes join.
if wallet := os.Getenv("NODE_OPERATOR_WALLET"); wallet != "" {
if warn := chainState.ValidateNodeOperatorWallet(wallet); warn != "" {
fmt.Printf("[NODE] ERROR: %s\n", warn)
fmt.Println("[NODE] NODE_OPERATOR_WALLET rejected — complete biometric registration first")
// Do NOT register — only verified humans receive validator rewards
} else {
chainState.RegisterNode(wallet)
}
} else {
fmt.Println("[NODE] NODE_OPERATOR_WALLET not set — this node won't receive validator rewards")
}

fmt.Println("╔════════════════════════════════════════╗")
fmt.Println("║     Aequitas Node Running ✓            ║")
fmt.Println("║     Producing blocks every 6 seconds   ║")
fmt.Println("╚════════════════════════════════════════╝")

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
// FIX 11: Signal the distribution goroutine to stop cleanly.
distCancel()
// FIX 6: Wait for the distribution goroutine to finish any in-progress work
// before exiting. A 10-second timeout prevents hanging indefinitely.
select {
case <-distDone:
case <-time.After(10 * time.Second):
	fmt.Println("[WARN] Distribution goroutine did not stop in 10 seconds — forcing exit")
}
fmt.Println("Node stopped.")
}

// distributionSyncHealthIssue returns a non-empty, human-readable reason if
// this node's view of the chain is not trustworthy enough to run daily pool
// distribution right now, or "" if it's safe to proceed. See its only call
// site (the distribution goroutine above) for why this matters: distribution
// weights validator rewards by THIS node's own locally-observed block
// counts, with no cross-node reconciliation.
func distributionSyncHealthIssue(bc *keeper.BlockDAG) string {
	if bc.IsDegraded() {
		return fmt.Sprintf("node is degraded (%s) — a prior persistence failure may have left this node's view of the chain incomplete", bc.DegradedReason())
	}
	if n := bc.SyntheticCheckpointCount(); n > 0 {
		return fmt.Sprintf("%d synthetic-checkpoint stub(s) still active — this node is trusting a placeholder instead of real history for at least one span of blocks", n)
	}
	// A node with peers configured but no successful peer sync in the last
	// hour is a red flag for isolation (e.g. every configured seed/peer
	// unreachable) -- distribution runs once a day, so an hour is a tight
	// bar relative to that cadence while still giving normal transient
	// connectivity blips (a redeploy, a brief network hiccup) comfortable
	// room to recover on their own well before the next scheduled round.
	if os.Getenv("PRIMARY_NODE_URL") != "" || os.Getenv("PRIMARY_NODE_URLS") != "" || os.Getenv("PEER_NODES") != "" {
		last := bc.LastSuccessfulPeerSyncAt()
		if last == 0 {
			return "peers are configured but this node has never successfully synced a block from any of them — its view of the chain cannot be trusted yet"
		}
		if age := time.Since(time.Unix(last, 0)); age > time.Hour {
			return fmt.Sprintf("no successful peer sync in %s — this node may be isolated from the rest of the network", age.Round(time.Minute))
		}
	}
	return ""
}

// isRFC1918OrLoopback returns true if host is a loopback address or an
// RFC 1918 private-network address. Used to block SSRF via BOOTSTRAP_SNAPSHOT_URL.
func isRFC1918OrLoopback(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Not a bare IP - hostname; resolve it for the check.
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false // cannot resolve; let the HTTP client fail naturally
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return false
		}
	}
	// FIX 9: Normalise IPv6-mapped IPv4 addresses (e.g. ::ffff:10.0.0.1) to their
	// IPv4 form so the RFC-1918 CIDR checks below match them correctly.
	// net.IP.To4() returns nil for a pure IPv6 address and a 4-byte slice for
	// both native IPv4 and IPv6-mapped IPv4 — exactly what we want here.
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() {
		return true
	}
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // link-local
	}
	for _, cidr := range private {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
