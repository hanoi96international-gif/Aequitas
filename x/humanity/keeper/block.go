package keeper

import (
"bytes"
"crypto/ecdsa"
"crypto/rand"
"crypto/sha256"
"database/sql"
"encoding/hex"
"encoding/json"
"fmt"
"math/big"
"os"
"runtime/debug"
"strings"
"sort"
"sync"
"sync/atomic"
"time"

"github.com/ethereum/go-ethereum/accounts/abi"
"github.com/ethereum/go-ethereum/common"
"github.com/ethereum/go-ethereum/crypto"
)

type Transaction struct {
	Type            string  `json:"type"`
	Wallet          string  `json:"wallet"`
	To              string  `json:"to,omitempty"`               // transfer destination
	Amount          float64 `json:"amount,omitempty"`
	AmountOut       float64 `json:"amount_out,omitempty"`       // swap output amount
	AmountPerHuman  float64 `json:"amount_per_human,omitempty"` // for ubi_distribution
	LPShares        float64 `json:"lp_shares,omitempty"`        // for add_liquidity; also reused on escrow_move for LP shares force-liquidated due to inactivity (see checkAndMoveToEscrowLocked)
	// EscrowTUsdConverted carries the tUSD balance an escrow_move TX's wallet
	// held and had converted to AEQ (via the pool) before the escrow capture
	// below — see checkAndMoveToEscrowLocked's comment. Secondaries replay
	// this exact input amount through the same AMM math against their own
	// current pool state, mirroring how RemoveLiquidityDelta re-derives
	// output from a primary-supplied input rather than a primary-supplied
	// output (pool state can only be assumed identical for the INPUT side).
	EscrowTUsdConverted float64 `json:"escrow_tusd_converted,omitempty"`
	// FromDemurrageLost/ToDemurrageLost carry the exact AEQ amount the
	// primary node decayed off Wallet/To via settleDemurrageLocked while
	// processing this TX. Secondary nodes replay these exact numbers
	// (ApplyTransferDelta/ApplySwapDelta/AddLiquidityDelta/RemoveLiquidityDelta)
	// instead of recomputing decay from effectiveBalance() at replay time —
	// recomputing would use the replaying node's own wall-clock time, which
	// can differ from the primary's by anything from network latency to a
	// full historical resync, producing a different decay amount and a
	// StateRoot divergence identical in kind to the swap-fee bug fixed in 8e3f675.
	FromDemurrageLost float64 `json:"from_demurrage_lost,omitempty"`
	ToDemurrageLost   float64 `json:"to_demurrage_lost,omitempty"`
	// DistributionAt carries the exact Unix timestamp the primary chose for
	// a distribution round (e.g. the new last_ubi_at) on
	// "ubi_distribution_finalize" TXs. Audit recheck 2 (P0 #4) found the
	// primary used to call time.Now() directly inside DistributeUBIPool
	// while secondaries replayed block.Timestamp instead — two different
	// instants, guaranteeing a StateRoot mismatch on every UBI round even
	// when every credited amount was correct. The primary now picks this
	// value once and uses it for both its own immediate state and this
	// field, so secondaries replay the IDENTICAL value instead of any
	// wall-clock reading of their own.
	DistributionAt int64 `json:"distribution_at,omitempty"`
	TxHash          string  `json:"tx_hash"`
	// Nullifier and Commitment are set on register_human TXs so secondary
	// nodes can apply the registration to their local state when they receive
	// the block — without needing a separate snapshot or state sync.
	Nullifier  string  `json:"nullifier,omitempty"`
	Commitment string  `json:"commitment,omitempty"`
	// ZK proof fields for register_human — enables secondary nodes to
	// independently verify the proof via BioVerifier without trusting
	// the validator signature alone. Fields are omitted for non-registration
	// TXs and for blocks produced by old nodes (backward-compatible).
	ProofA     []string   `json:"proof_a,omitempty"`   // [2]string big.Int decimal
	ProofB     [][]string `json:"proof_b,omitempty"`   // [2][2]string big.Int decimal
	ProofC     []string   `json:"proof_c,omitempty"`   // [2]string big.Int decimal
	PubSignals []string   `json:"pub_signals,omitempty"` // public signals (decimal)
	// BlockAHash/BlockBHash identify the equivocation evidence pair for
	// "slash_equivocation" TXs so the replay can be idempotent (see the
	// slash_equivocation case in replayTransactions and
	// MaybeQueueSlashOutboxTx in slashing.go).
	BlockAHash string `json:"block_a_hash,omitempty"`
	BlockBHash string `json:"block_b_hash,omitempty"`
}

type Block struct {
	Height       int64         `json:"height"`
	Timestamp    int64         `json:"timestamp"`
	ParentHashes []string      `json:"parent_hashes"`
	Hash         string        `json:"hash"`
	Proposer     string        `json:"proposer"`
	Humans       int           `json:"humans"`
	IsGenesis    bool          `json:"is_genesis,omitempty"`
	StateRoot    string        `json:"state_root,omitempty"`
	Transactions []Transaction `json:"transactions,omitempty"`
	Signature    string        `json:"signature,omitempty"`
	// ProducedAtMs is a millisecond-precision production wall-clock
	// timestamp, set once by ProduceBlock and transmitted to peers —
	// deliberately NOT covered by calculateBlockHash (an explicit field list,
	// not reflection, so adding this here can never change any existing
	// hash/signature). Timestamp above is Unix-SECONDS and baked into the
	// signed hash, too coarse to ever tell whether a block actually attached
	// within a sub-second BLOCK_TIME window or not. Added 2026-07-05 as a
	// permanent operational diagnostic (not a temp/will-revert one) after a
	// long night of tuning circuit-breaker constants without ever directly
	// measuring the one number that actually determines whether any of that
	// tuning can work: real end-to-end propagation+processing latency
	// between independently-hosted validators. See AddPeerBlock's own use of
	// this field for the live [LATENCY] log line. Zero on a block from a
	// peer running an older binary without this field — logging skips that
	// case rather than reporting a nonsense latency.
	ProducedAtMs int64 `json:"produced_at_ms,omitempty"`
	// Real GHOSTDAG consensus fields (Sompolinsky-Zohar, 2018).
	// BlueScore = number of blue blocks in the past of this block (including
	// the selected-parent chain). Blocks with more blue-score ancestors are
	// preferred, giving a canonical total order over the DAG.
	BlueScore      int64    `json:"blue_score,omitempty"`
	SelectedParent string   `json:"selected_parent,omitempty"` // parent with highest blue score
	Blues          []string `json:"blues,omitempty"`           // blue blocks in the merge set
	// FromSync marks blocks fetched via HTTP-SYNC from an operator-configured
	// trusted seed (PRIMARY_NODE_URL/PRIMARY_NODE_URLS/PEER_NODES — see
	// BlockDAG.trustedSeeds and isTrustedSyncSource in sync_blocks.go), NOT
	// merely "fetched via sync from any peer". Never serialized — defaults
	// false for all P2P/gossip blocks. When true, the authorization,
	// equivocation-suspension, and finality gates in AddPeerBlock are
	// bypassed: a configured seed's canonical history is trusted by
	// construction. Blocks synced from a peer that is NOT a configured seed
	// (e.g. one discovered dynamically via /api/peers/register, which only
	// requires a human registration + challenge-signature) get FromSync=false
	// and go through every gate normally — see the launch audit 2026-07-03
	// P0 fix: this used to be set unconditionally for every active sync
	// peer, letting anyone who self-registered as a peer feed in blocks that
	// skipped authorization/suspension/finality entirely.
	FromSync bool `json:"-"`
	// SelfFetched marks a block THIS node deliberately fetched via its own
	// catch-up sync (fetchMissingAncestors' targeted ancestor resolution, or
	// doSyncOnce's ordered paged sync) — never serialized, defaults false for
	// every P2P/gossip/push-received block, set regardless of whether the
	// peer it came from happens to be a statically-configured trusted seed.
	//
	// FIX (durable fix, 2026-07-04 — explicit user requirement: this must
	// work automatically for every future validator, no manual per-node
	// config): the proposer circuit breaker's only prior bypass (FromSync)
	// required the source peer to be in the static trustedSeeds list
	// (PRIMARY_NODE_URL/PEER_NODES) — fine for syncing from the primary, but
	// confirmed live to permanently deadlock TWO SECONDARY validators
	// against each other: neither treats the other as a trusted seed, so
	// once either breaker tripped, it could never close again — closing it
	// requires a block from that proposer to actually attach, but attaching
	// requires fetching the exact ancestor the breaker is blocking. Adding
	// the other's URL to PEER_NODES would work for exactly two nodes, but
	// requires every existing node's config to be updated by hand every time
	// a new validator joins — explicitly rejected as not scaling to a
	// growing, non-technical-operator-friendly network.
	//
	// SelfFetched is orthogonal to FromSync's authorization/equivocation/
	// finality bypasses (deliberately NOT touched here) -- a proposer must
	// still be authorized via the exact same NODE_OPERATOR_BINDING_SIGNATURE-
	// backed check as any other block; this only lets an ALREADY-authorized
	// proposer's block, fetched by OUR OWN deliberate request (not
	// unsolicited push/gossip), get past the circuit breaker's reputation
	// gate long enough to actually close the gap that tripped it. Works
	// automatically for any current or future validator, regardless of
	// which peer URLs happen to be statically configured anywhere.
	SelfFetched bool `json:"-"`
}

// peerChallenge holds a one-time challenge issued to a registering peer.
type peerChallenge struct {
	value     string
	expiresAt int64
}

type BlockDAG struct {
blocks                 map[string]*Block
tips                   map[string]bool
mu                     sync.RWMutex
state                  *ChainState
evm                    *EVMEngine       // set by EVMRPCServer after construction; used by replayTransactions for ZK proof verification
nodeID                 string
height                 int64
// bootHeight is dag.height's value at construction time (after restoring
// it from the persisted "max_block_height" — see createGenesisBlock's
// caller), captured ONCE and never updated again. Used by
// replayTransactions to recognize "ancestor catch-up" blocks: cs.accounts
// is loaded fully from the DB at startup and already reflects every
// block up to and including bootHeight, but dag.blocks/dag.tips are
// purely in-memory and start empty on every restart — so the node must
// still fetch and insert those ancestor blocks for hash-chain/tips
// bookkeeping, WITHOUT re-applying their transactions (already accounted
// for) or comparing their claimed StateRoot against cs.accounts' current,
// much-later state (guaranteed to "mismatch" despite no real divergence).
bootHeight             int64
// bootHeightCheckpointBacked is true only when bootHeight was set by
// actually seeding dag.blocks/dag.tips with a real, stored block at that
// exact height (RefreshBootHeightAfterSnapshotImport's checkpoint branch,
// seededFromCheckpoint) — see AddPeerBlock's bootHeight-skip call site for
// why this distinction is safety-critical, not cosmetic.
bootHeightCheckpointBacked bool
pendingTxs             []Transaction
txMu                   sync.Mutex
signingKey             *ecdsa.PrivateKey
selfProposer           string           // lower-cased Ethereum address of this node's signing key
authorizedValidators   map[string]bool  // Ethereum addresses allowed to propose blocks
currentEpoch           *EpochCommittee  // active block-producer committee for the current epoch
epochMu                sync.RWMutex    // guards currentEpoch
activeSyncPeers        map[string]bool  // peers with a running syncWithNode goroutine
	// peerSyncHeight tracks, per peer URL, the highest block height this
	// node has actually SUCCESSFULLY imported FROM that specific peer via
	// doSyncOnce — see that function's own FIX comment (2026-07-06) for the
	// incident this closes. dag.Height() is the wrong basis for the normal
	// windowed sync's minHeight: it's the highest height from ANY source,
	// including this node's own continuous self-production, which races
	// ahead of real per-peer catch-up progress once a node produces blocks
	// reliably every tick while a specific peer's blocks arrive with any
	// latency at all. Once self-production has raced far enough ahead,
	// dag.Height()-syncOverlap permanently requests a window past where
	// that peer's actual next block is, and — since deepScan only
	// activates once something has ALREADY failed to attach as an orphan —
	// the gap can silently persist forever with no missing-parent entry
	// ever created to trigger recovery. Guarded by syncPeerMu, same as
	// activeSyncPeers.
	peerSyncHeight         map[string]int64
	// trustedSeeds holds the operator-configured seed/static-peer URLs
	// (PRIMARY_NODE_URL, PRIMARY_NODE_URLS, PEER_NODES — see seedURLs/
	// staticPeers in sync_blocks.go), populated once by StartPeerDiscovery.
	// This is the trust anchor for block.FromSync (see isTrustedSyncSource):
	// unlike activeSyncPeers, which grows to include ANY peer that
	// successfully self-registers via /api/peers/register (just a human
	// registration + challenge-signature, no validator privilege), these
	// URLs are set by this node's own operator and are not attacker-
	// reachable, so bypassing authorization/suspension/finality gates for
	// blocks fetched from them doesn't hand that bypass to an arbitrary peer.
	trustedSeeds           map[string]bool
syncPeerMu             sync.Mutex
warnedUnknownProposers map[string]bool  // suppresses repeated "not authorized" log lines
peerChallenges         map[string]peerChallenge // address → pending challenge (P1-3)
challengeMu            sync.Mutex
replayedBlocks         map[string]bool  // tracks blocks already replayed — prevents double-credit on duplicate delivery
replayedMu             sync.Mutex
	// replayMu serializes replayTransactions calls across concurrent
	// AddPeerBlock invocations (e.g. the same or different blocks arriving
	// via P2P and HTTP sync at the same time) — replay must happen in a
	// well-defined order since TX dependencies span blocks (a register_human
	// in block N must be applied before a transfer in block N+1 from the
	// same wallet). This replaces the old single-consumer-goroutine +
	// channel design, which serialized replay the same way but ran it
	// asynchronously — see AddPeerBlock for why that was a correctness bug,
	// not just a latency tradeoff.
	replayMu            sync.Mutex
	stateRootMismatches map[string]int // per-proposer StateRoot mismatch counters
	// stateRootMismatchLastAt (audit 2026-06-30 monster audit, P2-03): Unix
	// timestamp of each proposer's most recent mismatch. stateRootMismatches
	// only resets a proposer's counter to 0 on that SAME proposer's next
	// matching block — a proposer that stops producing (offline, lost a
	// GHOSTDAG race, network split) leaves its count stuck at whatever it
	// last reached, forever, with no future block from it ever arriving to
	// either raise or clear it. Confirmed live 2026-06-30: after a clean
	// dual-restart-at-genesis with both nodes converged and producing in
	// sync (heights matching, total_supply/total_humans identical, mismatch
	// counts flat for 150s+), /api/health still reported "unhealthy" with
	// stale counts (228/89) from the initial multi-validator startup race —
	// indistinguishable, from this counter alone, from a node actively
	// diverging right now. TotalStateRootMismatches uses this timestamp to
	// report only mismatches from proposers that have mismatched recently
	// (see its own comment), not every proposer's lifetime peak.
	stateRootMismatchLastAt map[string]int64
	stateRootMismatchesMu sync.Mutex   // protects stateRootMismatches/stateRootMismatchLastAt (written under replayMu+cs.mu, read independently by TotalStateRootMismatches)
	// lastSuccessfulPeerSyncAt is the Unix timestamp of the last time this
	// node successfully accepted a peer block via AddPeerBlock. Read/written
	// with atomic.Int64 (not dag.mu) since it's set from AddPeerBlock's
	// success tail, after dag.mu has already been released — see
	// /api/health/combined (Gesamtaudit 2026-06-28, P2-4/P3-7: "Health/API
	// zeigt nicht ... seit wann [ein StateRoot-Mismatch existiert]").
	lastSuccessfulPeerSyncAt atomic.Int64
	// lastDeepScanAt (P2-01 audit, confirmed live on Contabo 2026-06-30):
	// throttles how often doSyncOnce's deepScan mode is allowed to do a full
	// height-0 re-walk of the entire known chain. See doSyncOnce's own
	// comment for why this exists — a large, genuinely-unresolvable orphan
	// backlog (stale references to a node's own pre-fix bad blocks) keeps
	// MissingParentHashes() non-empty for the full 15-minute orphanAbandonAfter
	// window, and without this throttle every single 6s sync tick repeated an
	// O(chain length) re-scan for that entire window — confirmed live: 99%
	// CPU sustained for minutes with chain length ~50,000 and ~8,500 distinct
	// missing parents pending abandonment.
	//
	// FIX (P0, 2026-07-04 — real root cause of tonight's persistent merge
	// failures): this used to be a single atomic.Int64, shared across EVERY
	// peer's syncWithNode goroutine. syncWithNode runs one goroutine PER
	// peer, each ticking independently and each calling doSyncOnce, which
	// reads/writes this SAME shared timestamp — whichever peer's goroutine
	// happens to check first within a given 30s window claims the deepScan
	// slot for ALL peers that window, permanently starving the others.
	// Confirmed live: with Primary and a second (still-isolated) peer both
	// configured, the second peer's goroutine consistently won the shared
	// slot, so Primary — the one peer whose bulk catch-up actually mattered
	// — never got its own deepScan turn at all, leaving it stuck on the
	// slow, one-hash-at-a-time fetchMissingAncestors path indefinitely
	// (too slow to keep pace with continuous multi-validator production).
	// Now keyed per nodeURL so every peer gets its own independent cooldown.
	lastDeepScanAtMu sync.Mutex
	lastDeepScanAt   map[string]int64
	// syncTargetHeight is set at startup to the seed node's current block
	// height. ProduceBlock defers production until this node has caught up
	// to within 10 blocks of the target, preventing the "produce on a stale
	// fork while sync is still running" divergence that requires manual
	// RESYNC_FROM_SNAPSHOT to fix. Cleared once caught up. If the seed is
	// unreachable at startup, or sync makes no further progress for
	// syncStallTimeout (see ProduceBlock's gate), production proceeds
	// independently so a downed seed never blocks all other nodes.
	syncTargetHeight   atomic.Int64
	activeGhostdagK    atomic.Int32 // live GHOSTDAG K for current epoch; 0 → use ghostdagKBase
	startupTime        int64        // Unix timestamp of NewBlockchain — used by the initial-sync gate
	// lastFarAheadLogAt rate-limits the "dropped far-ahead orphan" log (unix
	// nanos, atomic) so a fork flood can't turn the log itself into the
	// bottleneck — see AddPeerBlock's lock-free flood shield.
	lastFarAheadLogAt atomic.Int64
	// lastFinalityRejectLogAt rate-limits the "[FINALITY] Rejected block"
	// log the same way — see isFinalityViolation's call site. Confirmed
	// live (2026-07-03): a diverged peer re-delivering whole pages of
	// far-below-checkpoint blocks logged one unthrottled line PER block,
	// hitting Railway's own platform-level deploy-log rate limit
	// ("Messages dropped") on the primary, silently swallowing unrelated
	// log output along with it.
	// lastForeignMergeAt (unix seconds) is the last time this node
	// successfully attached a block proposed by some OTHER authorized
	// validator (a genuine peer merge, via AddPeerBlock) — see
	// selfProducedFinalityAllowed's comment for why this exists: it is the
	// one local signal that distinguishes "isolated from peers I know
	// about" from "healthy — other validators exist and I'm merging with
	// them" or "genuinely alone, no other validators configured yet".
	lastForeignMergeAt atomic.Int64
	// foreignLatency* accumulate real, measured end-to-end attach-latency
	// samples (ProducedAtMs on the sender to time-of-attach here) — see
	// recordForeignAttachLatency's own comment for why this exists as a
	// permanent operational diagnostic, not a temp one.
	foreignLatencyMu        sync.Mutex
	foreignLatencyCount     int
	foreignLatencySumMs     int64
	foreignLatencyMaxMs     int64
	lastForeignLatencyLogAt atomic.Int64
	// rawArrivalLatency* mirrors foreignLatency* but measures BEFORE any
	// gate (circuit breaker, far-ahead cap, etc.) — see
	// recordRawArrivalLatency's own comment (AddPeerBlock's entry) for why
	// this second measurement point exists: a node whose circuit breaker is
	// currently open produces zero foreignLatency samples for exactly the
	// direction that's failing, since those blocks never reach that later
	// measurement point at all.
	rawArrivalLatencyMu        sync.Mutex
	rawArrivalLatencyCount     int
	rawArrivalLatencySumMs     int64
	rawArrivalLatencyMaxMs     int64
	lastRawArrivalLatencyLogAt atomic.Int64
	// lastIsolationPauseLogAt rate-limits the "finality advance paused"
	// diagnostic the same way as the other log throttles above — this can
	// otherwise fire once per self-produced block (every BLOCK_TIME) for as
	// long as the isolation lasts.
	lastIsolationPauseLogAt atomic.Int64
	lastFinalityRejectLogAt atomic.Int64
	// proposerBreaker* (P0, 2026-07-02 fork-flood, path-independent shield): the
	// per-IP push shield (blockPushBreaker, api.go) only guards /api/blocks/push,
	// but the third-party 178.105.186.119 node's flood reached AddPeerBlock by a
	// different ingress and slipped straight past it (confirmed live: 56 far-ahead
	// rejects + 52 orphan queues in one window with ZERO [BLOCK-PUSH] logs). This
	// breaker keys on the block PROPOSER instead — stable and present on every
	// block via every path (push, libp2p, pull) — and trips a proposer whose
	// blocks repeatedly fail to attach (far-ahead of local height, or orphaned on
	// a fork-parent this node will never hold), then drops its blocks at
	// AddPeerBlock's lock-free top, before dag.mu / hash recompute / ECDSA. A
	// proposer whose blocks attach normally resets its run every block and never
	// trips. Guarded by proposerBreakerMu — a DEDICATED mutex, never dag.mu — so
	// the hot reject path can never contend with block production.
	proposerBreakerMu        sync.Mutex
	proposerFailRun          map[string]int   // proposer -> consecutive non-attaching blocks
	proposerBreakerUntil     map[string]int64 // proposer -> unix-nano its cooldown expires
	lastProposerBreakerLogAt atomic.Int64     // rate-limits the breaker-drop log to once/sec
	// replayMismatchMu guards lastReplayMismatchHash — see AddPeerBlock's
	// tail (the recordProposerOutcome call after a successful replay) for
	// why this exists.
	//
	// FIX (P0, 2026-07-03 night, merge-reliability follow-up): AddPeerBlock's
	// success tail used to call recordProposerOutcome(block.Proposer, true)
	// unconditionally, clearing that proposer's breaker run on EVERY
	// successfully-attached block — including one whose replayTransactions
	// call just logged a StateRoot mismatch moments earlier (a WARNING, not
	// a hard rejection — see replayTransactions' own comment on why that
	// warning must not block attachment). Confirmed live: a validator on a
	// permanently isolated/diverged fork (0xAA08fE2c..., see project memory
	// "isolated fork human node") had literally every one of its blocks
	// mismatch, yet the breaker could never trip against it — each mismatch
	// recorded a strike, then the very same block's own successful
	// attachment immediately erased it again before the next block ever
	// arrived. lastReplayMismatchHash lets the tail tell "this exact block
	// just mismatched" apart from "this block matched cleanly (or never had
	// a StateRoot to check)" so it can call recordProposerOutcome with the
	// correct outcome instead of always clearing. Safe without a dedicated
	// per-call-site lock beyond this mutex: replayMu already serializes
	// every AddPeerBlock replay end-to-end (see AddPeerBlock's own P0-01
	// comment), so only one goroutine ever reads or writes this at a time —
	// the mutex here is defensive documentation of that invariant, not
	// load-bearing on its own.
	replayMismatchMu       sync.Mutex
	lastReplayMismatchHash string
	// resyncSignalMu guards resyncSignalFrom, this node's own record of peers
	// that responded action:"resync_required" to a block THIS node pushed —
	// i.e. peers telling THIS node it may be the one that's diverged. See
	// recordResyncSignal (sync_blocks.go) and HTTPBroadcastBlock's reaction.
	// A dedicated mutex, never dag.mu, for the same reason proposerBreakerMu is.
	resyncSignalMu   sync.Mutex
	resyncSignalFrom map[string]int64 // peer URL -> unix time of its last resync_required signal
	// pushRejectStreakMu guards pushRejectStreak, a per-peer count of
	// CONSECUTIVE non-benign push rejections — see recordPushRejection
	// (sync_blocks.go) and pushRejectStreakThreshold's own comment for the
	// 2026-07-04 brutal audit finding this closes ("sender ignores almost
	// all push rejections"). A dedicated mutex, never dag.mu, for the same
	// reason resyncSignalMu is.
	pushRejectStreakMu  sync.Mutex
	pushRejectStreak    map[string]int // peer URL -> consecutive non-benign rejection count
	lastPushRejectLogAt atomic.Int64   // rate-limits the rejection warning log to once/sec
	// syntheticCheckpointCount (audit 2026-06-30 monster audit, P1-05) is a
	// running counter of synthetic-checkpoint stubs currently trusted by
	// this node, maintained incrementally at each insertion site
	// (BridgeHistoricalGap, queueOrphan's runtime-bridge branch) instead of
	// recomputed by scanning dag.blocks on every read. Read by both
	// SyntheticCheckpointCount() (health, takes dag.mu RLock) and
	// ProduceBlock's production gate (already holds dag.mu write-locked,
	// so it reads this atomic directly — see ProduceBlock's comment for
	// why a full dag.blocks scan there specifically would be wrong: O(chain
	// length) on every single block produced, the same class of cost that
	// made the stub-tips bug pathological earlier this session).
	syntheticCheckpointCount atomic.Int32
	// unverifiedSyntheticCheckpointCount is the subset of the above that sit
	// ABOVE the trusted snapshot boundary (bootHeight) — i.e. genuine mid-chain
	// gaps in otherwise-verifiable history. A stub at or below bootHeight is the
	// snapshot's own start-of-history point: the block the operator deliberately
	// bootstrapped from via a signed snapshot, which no node retains (the whole
	// network was snapshot-resynced there) and which therefore can never heal
	// with a real block. Such a boundary stub is as trusted as genesis and must
	// NOT halt block production or flag the node unhealthy — otherwise a
	// snapshot-bootstrapped secondary (confirmed live on Contabo) can never
	// produce again. Maintained incrementally at the same three sites as the
	// total counter above, comparing each stub's height to bootHeight (stable
	// after startup: BridgeHistoricalGap runs only after
	// RefreshBootHeightAfterSnapshotImport has set it).
	unverifiedSyntheticCheckpointCount atomic.Int32
	// unverifiedStubHeights tracks hash → height for every stub currently
	// counted in unverifiedSyntheticCheckpointCount. Guarded by dag.mu (all
	// three counter-maintenance sites already hold it). Exists so finality can
	// RELEASE a stub: once the finalized checkpoint has moved more than
	// finalityHeightSlack past a stub's height, isFinalityViolation rejects
	// the real block from every non-seed peer — the "wait for real history to
	// sync in behind it" condition the production gate is written around
	// becomes unsatisfiable by the node's own finality rule. Keeping the gate
	// up past that point doesn't protect anything (the bridged history is
	// already below the hard-finality line and already reflected in this
	// node's state and StateRoot cross-checks); it just strands the node in
	// permanent observer mode. Confirmed live on Contabo 2026-07-03: a
	// runtime-orphan-bridge stub over a region of genuinely-lost blocks
	// (missing from EVERY node's DB) halted its block production forever —
	// with a 2-validator network, that silently halved the validator set.
	// See releaseFinalitySealedStubs (finality.go).
	unverifiedStubHeights map[string]int64
	// ghostdagMigrationPending (audit 2026-06-30 monster audit, P1-03) is
	// true from the moment LoadBlocksFromDB finds blocks needing GHOSTDAG
	// backfill until the background migration goroutine finishes. Backgrounding
	// the migration (see LoadBlocksFromDB's own comment) fixed startup
	// liveness, but opened a consistency window the audit correctly flagged:
	// for however long the migration runs, ProduceBlock/AddPeerBlock could
	// pick a SelectedParent by comparing a fully-migrated block's real
	// BlueScore against an old block's not-yet-migrated zero-value
	// SelectedParent/BlueScore — a different GHOSTDAG view than what the
	// same chain converges to once migration finishes, which a restart at
	// the wrong moment could bake in permanently. Gates production and peer
	// acceptance until migration completes (or there was nothing to migrate).
	ghostdagMigrationPending atomic.Bool
	// resyncInProgress gates ProduceBlock/AddPeerBlock during an in-process
	// self-heal resync (triggerAutoResync's new in-process path, autoheal.go
	// PerformResync) so a concurrent production/acceptance can't interleave
	// with the atomic account/DAG-state swap. Unlike ghostdagMigrationPending
	// (which can run for minutes under heavy backlog and was deliberately
	// made non-blocking after a live outage), a resync completes in seconds
	// thanks to checkpoint-seeding, so briefly blocking for its duration is
	// the safe trade-off here, not a repeat of that mistake. Checked
	// lock-free before either function takes dag.mu/replayMu, so a resync in
	// progress never has to contend with them for those locks either.
	resyncInProgress atomic.Bool
	// resyncBootstrapURL/resyncSigner/resyncPrimaryURL cache the three values
	// StartDivergenceAutoHeal was configured with, so triggerAutoResync
	// (autoheal.go) can call PerformResync without needing its own signature
	// changed at every one of its three call sites (the StateRoot-mismatch
	// ticker, runChainDivergenceCheckOnce, startHeightStallCheck). Written
	// once at startup before any auto-heal goroutine runs; read-only after.
	resyncBootstrapURL string
	resyncSigner       string
	resyncPrimaryURL   string
	// orphans holds blocks whose parent isn't known yet, keyed by the missing
	// parent's hash. When that parent is later added, every block waiting on
	// it is retried automatically. See AddPeerBlock for why this exists —
	// without it, a block whose parent arrived even one sync cycle late was
	// silently dropped forever, along with everything built on top of it.
	orphans   map[string][]*Block
	orphansMu sync.Mutex
	// orphanFirstSeen/orphanLastAttempt back orphan TTL + per-hash fetch
	// cooldown — see queueOrphan's "abandon" comment and fetchMissingAncestors'
	// cooldown skip for why both exist.
	orphanFirstSeen   map[string]time.Time
	orphanLastAttempt map[string]time.Time
	// orphanAttempts counts genuine fetch attempts (this hash was actually
	// included in a batch sent to a peer) per missing-parent hash — see
	// queueOrphan's "abandon" comment for why time-since-first-seen alone
	// is not sufficient to conclude a hash is unfetchable.
	orphanAttempts map[string]int
	// orphanResolveInFlight/orphanResolveAgain coordinate triggerOrphanResolve
	// (sync_blocks.go): at most one resolution pass runs at a time, and if a
	// new orphan arrives while one is running, exactly one more pass runs
	// immediately after instead of being dropped — see triggerOrphanResolve.
	orphanResolveInFlight bool
	orphanResolveAgain    bool
	orphanResolveMu       sync.Mutex

	// (blueScore map removed — BlueScore is now stored directly in Block.BlueScore
	// and persisted to chain_blocks; no separate in-memory map needed)

	// softRetryBlocks holds blocks that failed replayTransactions but may
	// succeed once a different predecessor's state changes are applied first
	// (e.g. Block Y transfers Bob→Carol, but Bob's balance came from Block X
	// which hadn't been applied yet when Y arrived).  After every successful
	// AddPeerBlock, all entries are retried; entries older than softRetryTTL
	// are abandoned.  Coordinated by softRetryMu, separate from replayMu so
	// retries can be dispatched from a goroutine without deadlocking.
	softRetryBlocks  map[string]*Block
	softRetryFirstAt map[string]time.Time
	softRetryMu      sync.Mutex
	// softRetryFlushInFlight/softRetryFlushAgain coalesce concurrent flush
	// triggers the same way orphanResolveInFlight/orphanResolveAgain do
	// above (see triggerOrphanResolve) — see triggerSoftRetryFlush's
	// comment for why this exists.
	softRetryFlushInFlight bool
	softRetryFlushAgain    bool

	// degradedReason is set when a critical storage failure occurs after state
	// has already been committed to memory (e.g. SaveBlockToDB fails for an
	// accepted peer block, or — audit P1-01/P1-02 — DeleteBlockFromDB fails
	// after a replay failure, or SaveGHOSTDAGState fails after a success).
	// When non-empty, ProduceBlock returns nil immediately to halt production
	// until the operator resolves the issue and restarts. Also surfaced via
	// /api/health so it's visible without a log dive.
	degradedReason string
	degradedMu     sync.Mutex

	// equivocationIndex maps "proposer|sorted-parent-hashes" -> the hash of
	// the first block seen from that proposer for that exact parent set.
	// Used by detectEquivocation (slashing.go) to catch a validator signing
	// two DIFFERENT blocks for what should have been one unique contribution
	// — O(1) per check instead of scanning all of dag.blocks. Populated at
	// every point a block is added to dag.blocks (ProduceBlock, AddPeerBlock,
	// and historical restore in NewBlockchain) so equivocation detection
	// works the same for freshly-synced history as for newly-arriving
	// blocks. Protected by dag.mu, same as dag.blocks.
	equivocationIndex map[string]string

}


// genesisTimestamp reads the genesis_time from genesis.json if present,
// falling back to the hardcoded date. P2-11: avoid hardcoded timestamp.
func genesisTimestamp() int64 {
if data, err := os.ReadFile("genesis.json"); err == nil {
var g struct { GenesisTime string `json:"genesis_time"` }
if json.Unmarshal(data, &g) == nil && g.GenesisTime != "" {
if t, err := time.Parse(time.RFC3339, g.GenesisTime); err == nil {
return t.Unix()
}
}
}
return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC).Unix()
}

// loadAuthorizedValidators reads the AUTHORIZED_VALIDATORS env var
// (comma-separated Ethereum addresses). Used to reject peer blocks from
// unknown signers so no one can inject arbitrary blocks into the DAG.
func loadAuthorizedValidators() map[string]bool {
	m := make(map[string]bool)
	for _, addr := range strings.Split(os.Getenv("AUTHORIZED_VALIDATORS"), ",") {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if strings.HasPrefix(addr, "0x") && len(addr) == 42 {
			m[addr] = true
		}
	}
	return m
}

// GetSigningKey returns the ECDSA private key used to sign blocks, or nil
// if no signing key is configured. Used by the snapshot handler to sign
// exported snapshots so peer nodes can verify their authenticity.
func (dag *BlockDAG) GetSigningKey() *ecdsa.PrivateKey {
	return dag.signingKey
}

// P1-3: Challenge-Response Validator Signature Verification ─────────────────

// IssuePeerChallenge generates a one-time challenge for a registering validator.
// The peer must sign this challenge with their signing key to prove ownership.
// Challenges expire after 90 seconds.
func (dag *BlockDAG) IssuePeerChallenge(signingAddr string) string {
	ts := time.Now().Unix()
	// P3-FIX: add 16 random bytes so two challenges issued for the same
	// address in the same second always produce different values.
	var nonce [16]byte
	rand.Read(nonce[:]) //nolint:errcheck — crypto/rand never returns an error on supported platforms
	raw := fmt.Sprintf("aequitas-validator:%s:%d:%s", strings.ToLower(signingAddr), ts, hex.EncodeToString(nonce[:]))
	h := sha256.Sum256([]byte(raw))
	challenge := hex.EncodeToString(h[:])
	dag.challengeMu.Lock()
	// Fix 7: Cap peerChallenges to prevent unbounded growth from floods of
	// challenge requests. Prune expired entries first; if still over cap, reject.
	now := time.Now().Unix()
	for addr, c := range dag.peerChallenges {
		if now > c.expiresAt {
			delete(dag.peerChallenges, addr)
		}
	}
	if len(dag.peerChallenges) > 200 {
		dag.challengeMu.Unlock()
		fmt.Printf("[DAG] ⚠ peerChallenges cap exceeded for %s — rejecting new challenge\n", strings.ToLower(signingAddr))
		return ""
	}
	dag.peerChallenges[strings.ToLower(signingAddr)] = peerChallenge{
		value:     challenge,
		expiresAt: ts + 90,
	}
	dag.challengeMu.Unlock()
	return challenge
}

// VerifyPeerChallenge verifies that signature is a valid secp256k1 signature of
// the previously issued challenge by the private key corresponding to signingAddr.
// Returns true only if: challenge exists, is not expired, and ecrecover matches.
func (dag *BlockDAG) VerifyPeerChallenge(signingAddr, signature string) bool {
	signingAddr = strings.ToLower(signingAddr)
	dag.challengeMu.Lock()
	ch, ok := dag.peerChallenges[signingAddr]
	if ok {
		delete(dag.peerChallenges, signingAddr) // one-time use
	}
	dag.challengeMu.Unlock()
	if !ok || time.Now().Unix() > ch.expiresAt {
		return false
	}
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sigBytes) != 65 {
		return false
	}
	// Ethereum signed message prefix
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(ch.value), ch.value)
	hash := crypto.Keccak256Hash([]byte(msg))
	// Normalize recovery id (v=27/28 → 0/1)
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}
	pubKey, err := crypto.SigToPub(hash.Bytes(), sigBytes)
	if err != nil {
		return false
	}
	recovered := strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex())
	return recovered == signingAddr
}

// AddAuthorizedValidator adds an Ethereum address to the set of addresses
// allowed to propose blocks. Thread-safe; safe to call after startup.
func (dag *BlockDAG) AddAuthorizedValidator(addr string) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return
	}
	dag.mu.Lock()
	dag.authorizedValidators[addr] = true
	dag.mu.Unlock()
}

// ValidatorKeyPair pairs a block-signing address with the human wallet that
// authorized it. Returned by /api/validators so peers can verify credentials
// rather than blindly trusting a raw address list.
// OperatorBindingSignature is the EIP-191 personal_sign of
// "Aequitas: authorize validator <signing_address>" by human_wallet,
// proving the human explicitly delegated this signing key (P1-03).
type ValidatorKeyPair struct {
	SigningAddress           string `json:"signing_address"`
	HumanWallet              string `json:"human_wallet"`
	OperatorBindingSignature string `json:"operator_binding_signature,omitempty"`
}

// ValidatorKeyPairs returns signing/human-wallet pairs for all registered
// validators, read from both validator_keys and validator_slots. Returns nil
// when no DB is configured (fresh node, no keys registered yet).
func (dag *BlockDAG) ValidatorKeyPairs() []ValidatorKeyPair {
	if dag.state == nil {
		return nil
	}
	return dag.state.GetValidatorKeyPairsForSync()
}

// AuthorizedValidatorList returns a snapshot of all currently-authorized
// proposer addresses.  Used by /api/validators so peer nodes can sync the
// full validator set from each other — removing the need for manual
// AUTHORIZED_VALIDATORS config as validators join over time.
func (dag *BlockDAG) AuthorizedValidatorList() []string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	list := make([]string, 0, len(dag.authorizedValidators))
	for addr := range dag.authorizedValidators {
		list = append(list, addr)
	}
	sort.Strings(list) // P2-11 (audit): deterministic for caches and peer-sync
	return list
}

func (dag *BlockDAG) AddTransaction(tx Transaction) {
dag.txMu.Lock()
defer dag.txMu.Unlock()
dag.pendingTxs = append(dag.pendingTxs, tx)
}

// FIX (P2-7, beta-launch audit 2026-07-05): NewBlockchain used to also take
// a *Keeper (the package's separate, legacy in-memory human registry,
// keeper.go) purely to store it in a field nothing ever read — removed the
// whole dead type; see NewAPIServer's comment (api.go) for the full reasoning.
func NewBlockchain(nodeID string, state *ChainState) *BlockDAG {
dag := &BlockDAG{
blocks:                 make(map[string]*Block),
tips:                   make(map[string]bool),
state:                  state,
nodeID:                 nodeID,
authorizedValidators:   loadAuthorizedValidators(),
activeSyncPeers:        make(map[string]bool),
		peerSyncHeight:         make(map[string]int64),
warnedUnknownProposers: make(map[string]bool),
peerChallenges:         make(map[string]peerChallenge),
replayedBlocks:         make(map[string]bool),
equivocationIndex:      make(map[string]string),
	unverifiedStubHeights:  make(map[string]int64),
	stateRootMismatches:    make(map[string]int),
	stateRootMismatchLastAt: make(map[string]int64),
	orphans:                make(map[string][]*Block),
	orphanFirstSeen:        make(map[string]time.Time),
	orphanLastAttempt:      make(map[string]time.Time),
	orphanAttempts:         make(map[string]int),

	softRetryBlocks:        make(map[string]*Block),
	softRetryFirstAt:       make(map[string]time.Time),
	lastDeepScanAt:         make(map[string]int64),
}
if key, generated, err := loadOrCreateRelayerKey(); err != nil {
	fmt.Printf("[BLOCK] Warning: RELAYER_PRIVATE_KEY invalid, blocks will be unsigned: %v\n", err)
} else if key != nil {
	dag.signingKey = key
	// Always authorize ourselves — derived from the signing key, not the nodeID.
	selfAddr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	dag.selfProposer = selfAddr
	dag.authorizedValidators[selfAddr] = true
	if generated {
		fmt.Printf("✓ Block signing enabled (auto-generated key — see SAVE THIS warning above), proposer addr: %s\n", selfAddr)
	} else {
		fmt.Printf("✓ Block signing enabled (RELAYER_PRIVATE_KEY loaded), proposer addr: %s\n", selfAddr)
	}
}

dag.createGenesisBlock()

// FIX (audit 2026-06-28 recheck 5, P1-2): recover any pending_txs row left
// "included" by a process that crashed before its block ever reached
// BroadcastBlock — see ResetStaleIncludedPendingTxs' own comment for why
// that's always safe to retry. 10 minutes comfortably exceeds how long a
// single ProduceBlock call could ever legitimately take.
state.ResetStaleIncludedPendingTxs(10 * time.Minute)

// FIX (audit 2026-06-28 full recheck, P1-3): restore every durably-saved
// block (see chain_blocks' own comment and SaveBlockToDB) BEFORE falling
// back to the bare max_block_height counter below. This is what lets a
// node recover its own previously produced/accepted blocks — and their
// full transaction lists — across a restart without needing any peer to
// still have them; the counter-only fallback further down only recovers
// the height NUMBER, not the actual block data.
// Load only the most-recent startupLoadWindow blocks into dag.blocks to
// bound startup RAM. bootHeight (set below from the DB's max_block_height
// entry) prevents re-replay of any block at or below the chain tip, so
// the full history need not be in memory.
startupMinH := int64(0)
if tip := state.getMaxBlockHeightDB(); tip > startupLoadWindow {
    startupMinH = tip - int64(startupLoadWindow)
}
loaded, loadErr := state.LoadBlocksFromDB(startupMinH)
if loadErr != nil {
	// FIX (2026-06-28, production incident): a transient DB error here
	// used to be silently treated as "this node has zero durably-saved
	// blocks" — for a node with a full chain_blocks table, that meant
	// starting fresh at genesis and forcing a complete peer resync of its
	// own history, repeatedly, on every restart that hit the hiccup (see
	// LoadBlocksFromDB's own comment). Crashing here and letting the
	// process supervisor (Docker --restart unless-stopped / Railway) retry
	// the whole startup is safer than silently continuing with a DAG that
	// doesn't reflect this node's real history.
	fmt.Printf("[BLOCK] ✗ FATAL: could not restore blocks from chain_blocks: %v — exiting so the process supervisor restarts cleanly instead of starting with a falsely-empty DAG\n", loadErr)
	os.Exit(1)
}
if len(loaded) > 0 {
	referenced := make(map[string]bool, len(loaded))
	for _, b := range loaded {
		dag.blocks[b.Hash] = b
		// Build equivocation index from history so detection works on
		// freshly-restarted nodes without needing to re-download everything.
		// No lock needed: NewBlockchain is single-threaded at this point.
		dag.checkAndIndexEquivocation(b)
		// Already reflected in chain_accounts (committed when these TXs
		// were first applied, before this block was even assembled) —
		// must not be re-applied by replayTransactions.
		dag.replayedBlocks[b.Hash] = true
		for _, ph := range b.ParentHashes {
			referenced[ph] = true
		}
		if b.Height > dag.height {
			dag.height = b.Height
		}
	}
	for hash := range dag.tips {
		if referenced[hash] {
			delete(dag.tips, hash)
		}
	}
	for hash := range loaded {
		if !referenced[hash] {
			dag.tips[hash] = true
		}
	}
	// Sort loaded blocks in topological order (height ASC) for GHOSTDAG computation.
	sortedForGHOSTDAG := make([]*Block, 0, len(loaded))
	for _, b := range loaded {
		sortedForGHOSTDAG = append(sortedForGHOSTDAG, b)
	}
	sort.Slice(sortedForGHOSTDAG, func(i, j int) bool {
		return sortedForGHOSTDAG[i].Height < sortedForGHOSTDAG[j].Height
	})
	// If any non-genesis block lacks GHOSTDAG data (DB predates persistence
	// columns), compute it now and save back to DB in the background.
	needsMigration := false
	for _, b := range sortedForGHOSTDAG {
		if !b.IsGenesis && b.Height > 0 && b.SelectedParent == "" {
			needsMigration = true
			break
		}
	}
	if needsMigration {
		// FIX (2026-06-30, confirmed live in production): this used to compute
		// GHOSTDAG state for every block SYNCHRONOUSLY here, before
		// NewBlockchain (and everything that calls it, including main.go's
		// http.ListenAndServe) ever returns. At chain length ~50,000 with
		// 18,148 blocks needing migration, that held up the HTTP listener
		// itself for minutes with zero progress output — Railway's proxy saw
		// "Application failed to respond" the whole time because nothing was
		// listening on the port yet, not because anything was deadlocked.
		// Run the compute + save both in the background instead: dag.mu is
		// now acquired per-block (not once for the whole migration) so normal
		// traffic (new peer blocks, ProduceBlock, API reads) interleaves with
		// migration progress instead of queuing behind it for the full
		// duration. A block touched by normal traffic before its turn in this
		// loop already has real GHOSTDAG data (SelectedParent != ""), so the
		// migration's own !exists-style skip isn't needed — computeGHOSTDAGState
		// is idempotent and just recomputes the same deterministic result.
		fmt.Printf("[BLOCK] GHOSTDAG migration: computing real blue scores for %d blocks in the background...\n", len(sortedForGHOSTDAG))
		dag.ghostdagMigrationPending.Store(true)
		go func(blocks []*Block, d *BlockDAG, s *ChainState) {
			defer d.ghostdagMigrationPending.Store(false)
			// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go.
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[PANIC RECOVERED] GHOSTDAG migration goroutine: %v\n%s\n", r, debug.Stack())
				}
			}()
			const batchSize = 100
			batch := make([]*Block, 0, batchSize)

			flushBatch := func() {
				if s == nil || len(batch) == 0 {
					batch = batch[:0]
					return
				}
				if err := s.SaveGHOSTDAGStateBatch(batch); err != nil {
					d.degradedMu.Lock()
					if d.degradedReason == "" {
						d.degradedReason = fmt.Sprintf("GHOSTDAG migration: batch persist failed (last block #%d): %v", batch[len(batch)-1].Height, err)
					}
					d.degradedMu.Unlock()
					fmt.Printf("[BLOCK] ✗ GHOSTDAG migration: batch persist failed: %v — node marked degraded\n", err)
				}
				batch = batch[:0]
			}

			for i, b := range blocks {
				d.mu.Lock()
				d.computeGHOSTDAGState(b)
				d.mu.Unlock()
				// DB write happens outside the lock — SaveGHOSTDAGStateBatch
				// only reads fields set above by computeGHOSTDAGState; the
				// same fields AddPeerBlock would compute to identical values
				// (deterministic), so no correctness hazard.
				if s != nil {
					batch = append(batch, b)
					if len(batch) >= batchSize {
						flushBatch()
					}
				}
				// FIX (2026-06-30): force a scheduling gap every 20 blocks
				// so ProduceBlock / AddPeerBlock goroutines get dag.mu turns
				// instead of starving behind the migration loop.
				if i%20 == 19 {
					time.Sleep(5 * time.Millisecond)
				}
			}
			flushBatch() // persist remaining partial batch
			fmt.Printf("[BLOCK] GHOSTDAG migration: computed and persisted %d blocks\n", len(blocks))
		}(sortedForGHOSTDAG, dag, state)
	}
	fmt.Printf("[BLOCK] Restored %d durable block(s) from chain_blocks — height=%d, tips=%d\n", len(loaded), dag.height, len(dag.tips))
}

// FIX (double-apply): dag.height/dag.blocks/dag.tips used to be purely
// in-memory — ReconstructState is a no-op when using Postgres, so they
// reset to genesis on every process restart regardless of how much
// chain history cs.accounts (loaded fresh from the DB above) actually
// reflects. This counter-only fallback covers any block produced before
// chain_blocks existed (or saved by a node that hadn't yet picked up
// this fix): it can only raise dag.height, never lower what the loaded
// blocks above already established, so ExportSnapshot reports the
// chain's true cumulative height, not "blocks observed since this
// process last started" — see the same fix's writes in
// ProduceBlock/AddPeerBlock and StateSnapshot.Height's comment for the
// bug this caused (a fresh-bootstrapped secondary's snapshot cutoff was
// reported far too low, so it still re-replayed — and double-applied —
// every block between the true height and the process-local one).
// FIX (audit 2026-06-28 recheck 4, P0-1): startup code, no lock held —
// must use the plain DB-only read.
if persisted := state.getConfigValueDB("max_block_height"); persisted != "" {
	var h int64
	fmt.Sscanf(persisted, "%d", &h)
	if h > dag.height {
		dag.height = h
	}
}

// FIX (P0, 2026-07-02 recurrence): a finalized checkpoint can never
// legitimately sit above the height this node has actually synced —
// GHOSTDAG finality is always behind-or-equal to the local tip, never
// ahead of it. Confirmed live on Contabo: finalized_height=67293 while
// chain_blocks held zero rows and max_block_height=0, because the wipe
// that produced this ran under a binary predating a12009d's RESYNC-path
// reset (stale cached Docker layer — see the fork-flood incident notes).
// isFinalityViolation then rejects every block below the stale
// checkpoint forever, hanging the node in a permanent "added 0 of 500"
// loop that previously needed manual SQL surgery to clear. Checking the
// invariant here — the one place every boot already reads both values —
// makes the fix path-independent: it self-corrects no matter what caused
// the incoherence (stale image, crash mid-wipe, manual tampering, or a
// future bug), not just the one call site that caused it last time.
if finalizedHeight, _ := state.GetFinalizedCheckpoint(); checkpointIsIncoherent(finalizedHeight, dag.height) {
	fmt.Printf("[FINALITY] ⚠ Stale checkpoint detected: finalized_height=%d exceeds synced height=%d — this is impossible under honest operation, auto-correcting to 0 so re-finalization can advance naturally as the node syncs.\n",
		finalizedHeight, dag.height)
	state.setConfigValueDB("finalized_height", "0")
	state.setConfigValueDB("finalized_blue_score", "0")
	state.setConfigValueDB("finalized_hash", "0")
}

// Captured ONCE, after the restoration above and before any block
// processing begins — see bootHeight's field comment.
dag.bootHeight = dag.height

// Background pruner: evict finalized blocks from dag.blocks every 60s
// to bound long-running RAM usage. DB retains the full history.
SafeGoroutine("pruneOldDAGBlocks-ticker", func() {
    t := time.NewTicker(60 * time.Second)
    defer t.Stop()
    for range t.C {
        // FIX (P0-3, beta-launch audit 2026-07-05): recover per-tick — see safeCall's comment.
        SafeCall("pruneOldDAGBlocks-tick", dag.pruneOldDAGBlocks)
    }
})

return dag
}

// checkpointIsIncoherent reports whether a persisted finality checkpoint is
// provably stale: GHOSTDAG finality can only ever sit behind or equal to the
// height this node has actually synced, never ahead of it. See NewBlockchain's
// call site for the incident this guards against.
func checkpointIsIncoherent(finalizedHeight, syncedHeight int64) bool {
	return finalizedHeight > syncedHeight
}

// relayerAddressFromEnv returns RELAYER_ADDRESS if explicitly set (an
// override for advanced setups where the deploy/operator address
// intentionally differs from the block-signing key), otherwise derives the
// address directly from RELAYER_PRIVATE_KEY — the same key that already
// signs this node's blocks (see NewBlockchain above). Setup simplification
// (scale audit): operators running the common single-key setup (one VPS,
// one key pair, signs blocks AND owns the validator reward wallet) never
// need to set RELAYER_ADDRESS at all. It was previously effectively
// required because some call sites read it directly with no fallback —
// state.go's RegisterNode silently wrote an empty signing_address when
// unset, meaning that operator's blocks were never credited toward their
// validator-pool reward despite producing them correctly. Every
// RELAYER_ADDRESS call site should go through this helper instead of
// reading the env var directly.
// loadOrCreateRelayerKey loads RELAYER_PRIVATE_KEY if set, otherwise
// generates a fresh secp256k1 key and prints it once so the operator can
// save it. Setup simplification (scale audit): until now, an operator had
// to generate an Ethereum-compatible private key through some external tool
// before a node could produce signed blocks at all -- the ONE genuinely
// required value with no in-repo path to get one (p2p.go's NODE_KEY already
// auto-generates the same way; this closes the same gap for the chain's own
// signing key). Returns (key, wasGenerated, error). A generated key is NOT
// persisted anywhere by this process — losing it before the operator saves
// RELAYER_PRIVATE_KEY means a new identity (and therefore a fresh
// authorization/binding cycle) next restart, which is why the warning below
// is deliberately loud, mirroring loadOrCreateKey's NODE_KEY warning in
// p2p.go. The simplest path for most operators remains setting
// RELAYER_PRIVATE_KEY to the private key of their already-verified-human
// NODE_OPERATOR_WALLET (the single-key deployment pattern documented in
// sync_blocks.go's registerAndDiscover) — auto-generation exists as a
// zero-config fallback, not the recommended flow.
func loadOrCreateRelayerKey() (key *ecdsa.PrivateKey, wasGenerated bool, err error) {
	if pkHex := strings.TrimPrefix(os.Getenv("RELAYER_PRIVATE_KEY"), "0x"); pkHex != "" {
		key, err = crypto.HexToECDSA(pkHex)
		return key, false, err
	}
	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, false, err
	}
	encoded := hex.EncodeToString(crypto.FromECDSA(key))
	addr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	fmt.Fprintln(os.Stderr, "════════════════════════════════════════")
	fmt.Fprintln(os.Stderr, "⚠ No RELAYER_PRIVATE_KEY found — generated a new one.")
	fmt.Fprintln(os.Stderr, "⚠ This key is visible in hosted log dashboards. Treat it as a secret.")
	fmt.Fprintln(os.Stderr, "⚠ SAVE IT NOW — if this process restarts before you do, your validator")
	fmt.Fprintln(os.Stderr, "⚠ identity changes and any pending authorization/rewards binding is lost.")
	fmt.Fprintf(os.Stderr, "SET THIS AS RELAYER_PRIVATE_KEY, then restart the service:\n0x%s\n", encoded)
	fmt.Fprintf(os.Stderr, "Its address (for RELAYER_ADDRESS / NODE_OPERATOR_WALLET, if this is also your verified-human wallet): %s\n", addr)
	fmt.Fprintln(os.Stderr, "════════════════════════════════════════")
	return key, true, nil
}

func relayerAddressFromEnv() string {
	if addr := strings.ToLower(strings.TrimSpace(os.Getenv("RELAYER_ADDRESS"))); addr != "" {
		return addr
	}
	pk := strings.TrimPrefix(os.Getenv("RELAYER_PRIVATE_KEY"), "0x")
	if pk == "" {
		return ""
	}
	key, err := crypto.HexToECDSA(pk)
	if err != nil {
		return ""
	}
	return strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
}

// RefreshBootHeightAfterSnapshotImport re-reads max_block_height/
// snapshot_import_height from the DB and raises dag.height/dag.bootHeight
// to match, if higher than what NewBlockchain captured at construction time.
//
// FIX (root cause behind Contabo VPS's permanent post-resync catch-up
// failure, found 2026-06-28): main.go constructs the BlockDAG (which seeds
// dag.height/dag.bootHeight from whatever max_block_height already exists
// in the DB) BEFORE RESYNC_FROM_SNAPSHOT/BOOTSTRAP_SNAPSHOT_URL ever runs.
// On a freshly wiped DB, that means dag.height/dag.bootHeight are captured
// as 0 — and bootHeight, not just height, matters here: replayTransactions'
// skipHeight check (see its own comment) takes max(dag.bootHeight,
// snapshot_import_height read live from DB), so in principle the live DB
// read alone should have been enough. In practice dag.height itself (the
// sync frontier doSyncOnce pages forward from) stayed frozen at 0, so the
// node still had to fetch, hash-verify, and insert into dag.blocks/dag.tips
// every single one of ~18,000 historical blocks one HTTP page at a time
// before reaching its true frontier — needless work that starved
// fetchMissingAncestors of cycles while validators kept producing new
// blocks every ~6s, which is what caused the orphan buffer to fall behind
// and start permanently abandoning blocks it could have resolved given
// less contention (see orphanAbandonAfter's comment). Calling this right
// after a successful snapshot import/resync, before HTTP sync starts, lets
// the node begin paging from near its true height immediately — the only
// blocks it then needs from peers are the handful actually referenced as
// parents going forward, fetched on demand via fetchMissingAncestors,
// never the full historical backlog.
// resyncHappened must be true only when ResyncFromSnapshotURL (an explicit
// RESYNC_FROM_SNAPSHOT=true operator action) just succeeded, never for the
// plain "fresh node" ImportSnapshotFromURL merge path.
//
// FIX (2026-06-28, third incident this session — confirmed live on
// Contabo): NewBlockchain (this dag's own constructor) runs and calls
// LoadBlocksFromDB BEFORE the bootstrap/resync block in main.go ever runs —
// see that comment. So by the time a resync actually wipes chain_blocks
// (ResyncFromSnapshotURL's own DELETE), dag.blocks/dag.tips/dag.height in
// THIS already-constructed struct are still whatever the OLD, now-deleted
// rows populated — stale, not empty. Without resetting them here, the
// sequential-genesis-walk this whole mechanism exists to enable (see this
// function's own comment two paragraphs up) never actually starts from
// genesis: dag.height stays at its old value, dag.tips still points at
// stale hashes nothing will ever reference again, and every subsequent
// peer block queues as a permanent orphan on a parent that used to exist
// in this node's view but no longer does anywhere. Confirmed live: after a
// resync with this bug, dag.height didn't move and the orphan log only
// ever showed "queued as orphan", never a single "Added N new blocks"
// bulk-sync line. resyncHappened being true means it's now safe — required,
// even — to discard everything loaded before this call and start over.
// mergeSiblingsIntoBlocks is RefreshBootHeightAfterSnapshotImport's
// checkpoint-sibling-loading half, split out as a pure function (no DB I/O)
// so it's directly unit testable — mirrors the verifyFetchedBlock/
// filterAndVerifySiblings extraction pattern elsewhere in this codebase.
// Only inserts a sibling that is genuinely AT checkpointHeight and not
// already the canonical block just seeded into blocks; never overwrites an
// existing entry (blocks[s.Hash] already present means it was already
// loaded, by this call or an earlier one). Returns the count actually added.
func mergeSiblingsIntoBlocks(blocks map[string]*Block, siblings []*Block, checkpointHeight int64, canonicalHash string) int {
	added := 0
	for _, s := range siblings {
		if s.Height != checkpointHeight || s.Hash == canonicalHash {
			continue
		}
		if _, exists := blocks[s.Hash]; !exists {
			blocks[s.Hash] = s
			added++
		}
	}
	return added
}

func (dag *BlockDAG) RefreshBootHeightAfterSnapshotImport(resyncHappened bool) {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	// FIX (P0, 2026-07-04 — permanent-isolation-after-plain-restart incident):
	// defaults to false; only the checkpoint-seeding branch below (where
	// dag.blocks/dag.tips is ACTUALLY populated with a real, stored block at
	// exactly this height) sets it true. See bootHeightCheckpointBacked's own
	// field comment and AddPeerBlock's bootHeight-skip call site for why this
	// distinction is safety-critical: the skip is only sound when SOMETHING
	// in dag.tips genuinely represents this height for later blocks to find
	// as a parent — true right after a checkpoint-seeded resync, never true
	// for a plain restart (bootHeight there is ratcheted from a persisted
	// height NUMBER in chain_config, with no matching entry in dag.blocks/
	// dag.tips to back it).
	checkpointBacked := false
	if resyncHappened {
		dag.blocks = make(map[string]*Block)
		dag.tips = make(map[string]bool)
		dag.replayedMu.Lock()
		dag.replayedBlocks = make(map[string]bool)
		dag.replayedMu.Unlock()
		dag.bootHeight = 0
		dag.startupTime = time.Now().Unix()
		// FIX (P0, 2026-07-04 — fourth layer of the same incident): a resync
		// wipes dag.blocks/dag.tips but, until now, never touched
		// dag.orphans/orphanFirstSeen/orphanAttempts. Every orphan queued
		// before this resync referenced a missing-parent hash from this
		// node's OWN pre-resync (possibly isolated, dead-end) history — none
		// of it is reachable from the fresh checkpoint going forward. With
		// the SelfFetched exemption above now correctly making
		// fetchMissingAncestors' resolutions stick instead of silently
		// discarding them, those stale entries stopped being harmless noise
		// and started costing real resolution attempts and orphanAbandonAfter
		// budget — confirmed live: a fresh-checkpoint node kept walking
		// backward through a dozens-deep pre-resync orphan chain that could
		// never connect to anything, crowding out attention that should have
		// gone to the live tip. Clearing here means the exact same "gossip of
		// stale pre-resync blocks" this whole mechanism was built to tolerate
		// (see this function's own top-level comment) never even reaches the
		// orphan queue post-resync — it's simply unknown, fresh territory.
		dag.orphansMu.Lock()
		dag.orphans = make(map[string][]*Block)
		dag.orphanFirstSeen = make(map[string]time.Time)
		dag.orphanLastAttempt = make(map[string]time.Time)
		dag.orphanAttempts = make(map[string]int)
		dag.orphansMu.Unlock()

		// FIX (durable fix, 2026-07-03): if SeedTrustedCheckpoint (snapshot.go)
		// already fetched, verified, and persisted a real checkpoint block at
		// max_block_height, seed dag.blocks/dag.tips from THAT block instead of
		// only genesis — the sequential resync then starts from the checkpoint,
		// not height 0. Falls back to genesis-only exactly as before if no
		// checkpoint was seeded (max_block_height still "0"/absent) or the DB
		// load fails for any reason.
		var seededFromCheckpoint bool
		if persisted := dag.state.getConfigValueDB("max_block_height"); persisted != "" {
			var checkpointHeight int64
			fmt.Sscanf(persisted, "%d", &checkpointHeight)
			if checkpointHeight > 0 {
				if cp := dag.state.LoadBlockFromDBByHeight(checkpointHeight); cp != nil {
					dag.blocks[cp.Hash] = cp
					dag.tips = map[string]bool{cp.Hash: true}
					dag.height = cp.Height
					dag.bootHeight = cp.Height
					seededFromCheckpoint = true
					checkpointBacked = true
					fmt.Printf("[RESYNC] ✓ Seeded in-memory DAG from trusted checkpoint at height %d (%s...) — sequential resync starts here, not genesis\n",
						cp.Height, cp.Hash[:min(16, len(cp.Hash))])
					// FIX (P0, 2026-07-04 — true root cause of a night of
					// persistent non-convergence): dag.blocks only had the
					// SINGLE canonical checkpoint block — see
					// fetchAndVerifySiblingsAtHeight's comment (snapshot.go)
					// for why a later block can legitimately reference a
					// DIFFERENT sibling at this exact height as a merge
					// parent, permanently orphaning if that sibling was
					// never seeded. SeedTrustedCheckpoint best-effort
					// persists those siblings to chain_blocks; load whatever
					// made it there into dag.blocks too (NOT dag.tips — the
					// canonical block above remains the sole starting tip).
					if sibs, err := dag.state.LoadBlocksSinceFromDB(checkpointHeight-1, "", 50); err == nil {
						added := mergeSiblingsIntoBlocks(dag.blocks, sibs, checkpointHeight, cp.Hash)
						if added > 0 {
							fmt.Printf("[RESYNC] ✓ Also loaded %d sibling block(s) at checkpoint height %d into the in-memory DAG\n", added, checkpointHeight)
						}
					}
				}
			}
		}
		if !seededFromCheckpoint {
			dag.createGenesisBlock() // repopulates dag.blocks/dag.tips with genesis only, sets dag.height = 0
			fmt.Println("[RESYNC] ✓ Reset in-memory DAG to genesis-only — chain_blocks was just wiped, sequential resync from genesis starts now")
		}
	}

	// bootHeight = max(max_block_height, snapshot_import_height): controls
	// replayTransactions' skipHeight so we never re-apply state that the
	// snapshot already encodes.
	var bootH int64
	if persisted := dag.state.getConfigValueDB("max_block_height"); persisted != "" {
		fmt.Sscanf(persisted, "%d", &bootH)
	}
	if snapHeightStr := dag.state.getConfigValueDB("snapshot_import_height"); snapHeightStr != "" {
		var snapHeight int64
		fmt.Sscanf(snapHeightStr, "%d", &snapHeight)
		if snapHeight > bootH {
			bootH = snapHeight
		}
	}
	if bootH > dag.bootHeight {
		// FIX (P0, 2026-07-04): this ratchet pushes bootHeight to a bare
		// persisted NUMBER, not a height any checkpoint-seeding actually
		// stored a block for — whatever checkpoint-backed guarantee held a
		// moment ago no longer applies to this higher value. Confirmed live:
		// after a plain restart, this ratchet raised bootHeight to
		// max_block_height (continuously bumped by this node's own ongoing
		// production) while dag.blocks/dag.tips still only held the
		// restored/pruned window — AddPeerBlock's bootHeight-skip then waved
		// through every real historical block up to that height WITHOUT ever
		// storing them, so anything built on top orphaned forever on a
		// "known-covered" parent that was never actually in dag.blocks.
		checkpointBacked = false
		dag.bootHeight = bootH
	}
	dag.bootHeightCheckpointBacked = checkpointBacked

	// FIX (P0, 2026-07-05 — real root cause of most of tonight's
	// instability): on a PLAIN restart (resyncHappened=false, e.g. any
	// routine no-resync-needed code deploy), checkpointBacked stayed
	// unconditionally false above — overly pessimistic. NewBlockchain's own
	// startup load (LoadBlocksFromDB, see its call site's comment) already
	// restores the most recent startupLoadWindow (2000) blocks into
	// dag.blocks, including a REAL block at exactly dag.bootHeight for any
	// node that has ever produced or synced normally — the same invariant
	// an explicit resync's checkpoint-seeding establishes, just reached via
	// the ordinary startup path instead of this run's resync branch above.
	// Without this, deepScanFloor() fell back to a full genesis walk
	// (floor 0) on EVERY plain restart. Confirmed live: a node mid-genesis-
	// walk was found adding real historical blocks in ascending order from
	// height ~10900 while its actual tip was past 188000 — silently
	// competing with real-time catch-up for bandwidth/attention on every
	// single deploy that didn't happen to also set
	// RESYNC_FROM_SNAPSHOT=true, which explains why the same real fixes
	// kept needing a fresh resync to show clean convergence, only to
	// degrade again after the NEXT ordinary restart.
	if !dag.bootHeightCheckpointBacked {
		for _, b := range dag.blocks {
			if b.Height == dag.bootHeight && b.Proposer != "synthetic-checkpoint" {
				dag.bootHeightCheckpointBacked = true
				fmt.Printf("[BOOT] ✓ bootHeight %d is backed by a real block already in dag.blocks from the normal startup load — no resync needed for this guarantee.\n", dag.bootHeight)
				break
			}
		}
	}

	// dag.height = max_block_height ONLY — this is the sync frontier
	// doSyncOnce pages forward from. After a plain snapshot resync (no
	// checkpoint seeded — see SeedTrustedCheckpoint), max_block_height stays
	// "0" so the node re-downloads all block headers sequentially from
	// genesis: raising dag.height from snapshot_import_height in that case
	// would make doSyncOnce start near the snapshot height while dag.blocks
	// is still empty there, orphaning every incoming block permanently. When
	// SeedTrustedCheckpoint DID succeed, max_block_height was written as the
	// checkpoint's own real height, and the block resync just seeded above
	// (this function's earlier section) already populated dag.blocks with
	// that exact block — so raising dag.height to match here is safe in that
	// case, not just tolerated. Same read, correct in both outcomes.
	var maxH int64
	if persisted := dag.state.getConfigValueDB("max_block_height"); persisted != "" {
		fmt.Sscanf(persisted, "%d", &maxH)
	}
	if maxH > dag.height {
		dag.height = maxH
	}
}

// BootHeight returns the boot height (the DB-persisted chain height or
// snapshot import height, whichever is larger) — the frontier below which
// replayTransactions already encodes state and blocks need not be re-applied.
func (dag *BlockDAG) BootHeight() int64 {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.bootHeight
}

// BootHeightCheckpointBacked reports whether BootHeight is backed by a real,
// stored block in dag.blocks/dag.tips at that exact height — see the
// bootHeightCheckpointBacked field's own comment.
func (dag *BlockDAG) BootHeightCheckpointBacked() bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.bootHeightCheckpointBacked
}

// IsDegraded and DegradedReason (audit P1-01/P1-02) report whether a DB
// write that keeps chain_blocks consistent with in-memory state has failed
// persistently this run. See the degradedReason field's struct comment.
func (dag *BlockDAG) IsDegraded() bool {
	dag.degradedMu.Lock()
	defer dag.degradedMu.Unlock()
	return dag.degradedReason != ""
}

func (dag *BlockDAG) DegradedReason() string {
	dag.degradedMu.Lock()
	defer dag.degradedMu.Unlock()
	return dag.degradedReason
}

// BridgeHistoricalGap handles a "permanent historical gap" that arises after a
// full RESYNC_FROM_SNAPSHOT when the blocks covering the gap no longer exist on
// any peer.  Without bridging, the first block above the gap references a parent
// that can never be downloaded, causing every subsequent block to orphan forever.
//
// The fix: fetch a page of blocks from each peer starting just above our current
// height, find any whose parent hash is not in dag.blocks, and insert a
// "synthetic checkpoint" stub with that exact hash.  The stub carries no
// transactions and is never replayed (bootHeight prevents it).  Its only purpose
// is to satisfy AddPeerBlock's parent-existence check so normal sync can resume.
//
// Call this after RefreshBootHeightAfterSnapshotImport (resyncHappened=true),
// before StartHTTPBlockSync.
func (dag *BlockDAG) BridgeHistoricalGap(peerURLs []string) {
	myHeight := dag.Height()
	dag.mu.RLock()
	bootH := dag.bootHeight
	dag.mu.RUnlock()
	if bootH <= myHeight+100 {
		return // gap small or absent — normal sync handles it
	}

	// Fetch candidates from two probe points:
	//   1. Just above myHeight — catches gaps immediately above the current frontier.
	//   2. bootH-1000       — catches gaps that pre-date myHeight (e.g. after a fresh
	//      RESYNC where myHeight=0 but the real gap is at heights 396-44367: probing
	//      at bootH-1000=44361 returns block 44368 whose parent is in the gap).
	// Both sets are merged; duplicate hashes are deduplicated via `seen`.
	var candidates []*Block
	seen := make(map[string]bool)
	probeHeights := []int64{myHeight}
	if probe2 := bootH - 1000; probe2 > myHeight {
		probeHeights = append(probeHeights, probe2)
	}
	for _, u := range peerURLs {
		if u == "" {
			continue
		}
		for _, ph := range probeHeights {
			blocks, err := dag.fetchBlocksSince(u, ph, "", 50)
			if err != nil || len(blocks) == 0 {
				continue
			}
			for _, b := range blocks {
				if !seen[b.Hash] {
					seen[b.Hash] = true
					candidates = append(candidates, b)
				}
			}
		}
	}
	if len(candidates) == 0 {
		fmt.Printf("[BRIDGE] No blocks available from peers — cannot detect historical gap (will retry on next sync cycle)\n")
		return
	}

	dag.mu.Lock()
	defer dag.mu.Unlock()

	// Build a quick lookup of all candidate hashes so we can distinguish between
	// a parent that is truly missing (permanent gap → needs stub) and one that
	// just hasn't been inserted into dag.blocks yet but IS among the fetched
	// candidates and will arrive via normal HTTP sync shortly (no stub needed).
	// Creating stubs for the latter causes BlueScore inflation: the stub gets a
	// height-approximate score that, once used as a parent for downstream blocks,
	// bakes in inflated scores permanently even after the real block overwrites
	// the stub entry (P0-bridge-stub-cascade fix).
	candidateHashes := make(map[string]bool, len(candidates))
	for _, b := range candidates {
		candidateHashes[b.Hash] = true
	}

	stubsInserted := 0
	for _, block := range candidates {
		for _, ph := range block.ParentHashes {
			if _, exists := dag.blocks[ph]; exists {
				continue
			}
			if candidateHashes[ph] {
				// Parent is among fetched candidates — will sync normally; no stub needed.
				continue
			}
			stubH := block.Height - 1
			if stubH < 0 {
				stubH = 0
			}
			dag.blocks[ph] = &Block{
				Hash:   ph,
				Height: stubH,
				// P1-05 (audit): BlueScore=0, NOT stubH. computeGHOSTDAGState picks
				// the parent with the HIGHEST BlueScore as SelectedParent — a
				// height-approximated score here (e.g. ~48000) would always beat a
				// real block's honestly-accumulated score (which grows far slower
				// than raw height once GHOSTDAG's K-bound starts reddening blocks,
				// confirmed live: primary's BlueScore was ~2053 at height ~45878).
				// A descendant that picks this stub as SelectedParent would then
				// inherit stubH+1 as ITS score, permanently baking in an inflated
				// baseline downstream — this is what caused cd20's BlueScore to
				// read ~46896 against primary's ~2053 at the same height. BlueScore
				// 0 still lets the stub serve its one real purpose (satisfying the
				// parent-existence check for the child that references it) without
				// ever winning the max-score comparison against a real block.
				BlueScore:    0,
				Proposer:     "synthetic-checkpoint",
				ParentHashes: []string{},
			}
			// FIX (2026-06-30, confirmed live in production): do NOT mark the
			// stub as a tip. Nothing ever legitimately builds on top of one
			// (it's a placeholder for a permanently-missing peer block, not
			// something other nodes know about or will ever reference as
			// THEIR parent), so unlike a real tip it's never removed from
			// dag.tips by the normal "delete(dag.tips, parentHash)" path —
			// it just accumulates forever. ProduceBlock uses every entry in
			// dag.tips as the new block's parent set, so each accumulated
			// stub permanently bloats every future block's merge-set
			// computation. Confirmed live: primary's dag.tips grew to 123+
			// entries (mostly stubs) after this session's bridging, and
			// ProduceBlock's computeGHOSTDAGState call started taking long
			// enough — walking/comparing across all of them — to hold dag.mu
			// long enough to starve every other dag.mu consumer (API reads,
			// peer sync) for extended stretches. The stub still does its one
			// real job (satisfying AddPeerBlock's parent-existence check via
			// dag.blocks) without needing to be in dag.tips for that.
			stubsInserted++
			displayHash := ph
			if len(displayHash) > 16 {
				displayHash = displayHash[:16]
			}
			fmt.Printf("[BRIDGE] ✓ Synthetic checkpoint at height %d (hash %s...) — bridging permanent gap in block history\n", stubH, displayHash)
			dag.syntheticCheckpointCount.Add(1)
			// Only a stub ABOVE the trusted snapshot boundary (bootHeight) gates
			// production/health — see unverifiedSyntheticCheckpointCount's comment.
			// The boundary stub itself (stubH <= bootHeight) is the snapshot's
			// start-of-history and is trusted like genesis.
			if stubH > dag.bootHeight {
				dag.unverifiedSyntheticCheckpointCount.Add(1)
				dag.unverifiedStubHeights[ph] = stubH
			}
			// FIX (audit 2026-06-30 monster audit, P1-05): durable audit trail
			// for every stub this node has ever trusted instead of verified —
			// see RecordSyntheticCheckpointEvent's own comment. Best-effort,
			// must not block the bridge on a DB hiccup, so this runs
			// fire-and-forget rather than under dag.mu (already held here).
			if dag.state != nil {
				SafeGoroutine("RecordSyntheticCheckpointEvent-startup", func() { dag.state.RecordSyntheticCheckpointEvent(ph, stubH, "startup-bridge") })
			}
		}
	}

	if stubsInserted == 0 {
		fmt.Printf("[BRIDGE] No gap detected at height %d — all parent hashes present in local DAG\n", myHeight)
	} else {
		fmt.Printf("[BRIDGE] Inserted %d synthetic stubs; sync will now proceed past the historical gap (height %d → %d+)\n", stubsInserted, myHeight, bootH)
	}
}

func (dag *BlockDAG) createGenesisBlock() {
genesis := &Block{
Height:       0,
Timestamp:    genesisTimestamp(), // P2-11: reads from genesis.json when available
ParentHashes: []string{},
Proposer:     "genesis",
// FIX: this used to be dag.state.TotalHumans() — i.e. however many humans
// THIS node's own DB currently has loaded at the moment it happens to
// start up. calculateHash() includes Humans in the hashed fields, so two
// nodes starting at different points in registration history (e.g. one
// freshly reset to 0 humans, another restarted after a registration
// already succeeded) computed two DIFFERENT genesis hashes. Since
// AddPeerBlock only removes a parent from dag.tips on an EXACT hash
// match, a secondary's own (differently-hashed) genesis tip was never
// removed when the primary's block #1 arrived — referencing the
// PRIMARY's genesis hash as its parent, not the secondary's. The
// secondary's orphaned genesis then sat in dag.tips forever (nothing
// ever referenced it as a parent to remove it), permanently showing
// "Tips: 2" with no merge ever happening — confirmed in production.
// Genesis must be 100% deterministic across every node by definition
// (it's the one block everyone is supposed to agree on without any
// data exchange), so it can never depend on a node's own live state.
Humans:       0,
IsGenesis:    true,
}
genesis.Hash = dag.calculateHash(genesis)
genesis.BlueScore = 0
genesis.SelectedParent = ""
genesis.Blues = nil
dag.blocks[genesis.Hash] = genesis
dag.tips[genesis.Hash] = true
dag.height = 0
fmt.Printf("✓ Genesis Block (DAG): %s\n", genesis.Hash[:16]+"...")
}

func (dag *BlockDAG) calculateHash(b *Block) string {
	return calculateBlockHash(b)
}

// calculateBlockHash is calculateHash's body, extracted as a free function so
// it can be called from contexts with no BlockDAG in scope (e.g.
// fetchAndVerifyBlockFromPeer in snapshot.go, verifying a fetched checkpoint
// block during resync) without needing dag state — the computation never
// touched dag in the first place.
func calculateBlockHash(b *Block) string {
// Normalize nil to empty slice so JSON always produces "[]" not "null".
// omitempty on the Transactions field strips the key during HTTP transport,
// and the receiver deserialises to nil — without this normalisation the
// tx_root differs between producer and receiver, causing hash mismatches.
txs := b.Transactions
if txs == nil {
txs = []Transaction{}
}
txData, _ := json.Marshal(txs)
txRootBytes := sha256.Sum256(txData)
txRoot := hex.EncodeToString(txRootBytes[:])
// Use parent hashes in the order stored on the block — do NOT sort here.
// Sorting must happen when PRODUCING a block (in ProduceBlock) so the order
// is baked into block.ParentHashes before the hash is computed. Re-sorting
// during verification would break hashes for blocks produced by peers using
// the original order.
data, _ := json.Marshal(map[string]interface{}{
"height":        b.Height,
"timestamp":     b.Timestamp,
"parent_hashes": b.ParentHashes,
"proposer":      b.Proposer,
"humans":        b.Humans,
"state_root":    b.StateRoot,
"tx_root":       txRoot,
})
hash := sha256.Sum256(data)
return hex.EncodeToString(hash[:])
}

func (dag *BlockDAG) ProduceBlock() *Block {
if dag.resyncInProgress.Load() {
	return nil // an in-process self-heal resync is atomically swapping account/DAG state right now — see resyncInProgress's field comment
}
// Ongoing health check, not tied to any specific past incident: warn if a
// single ProduceBlock call takes unusually long, since it holds dag.mu for
// its entire duration (every other dag.mu consumer — API reads,
// AddPeerBlock — stalls for the same span). The 2026-07-02/03 cadence
// investigation that originally added a detailed per-phase breakdown here
// found and fixed its root cause (batch-fetched GHOSTDAG merge-set lookups,
// commit 8c9321f); this simple total-time check is what's still worth
// keeping permanently.
produceStart := time.Now()
defer func() {
	if d := time.Since(produceStart); d > 500*time.Millisecond {
		fmt.Printf("[BLOCK] ⏱ ProduceBlock itself took %s\n", d)
	}
}()
// P0-01 (audit): acquire replayMu before dag.mu so ProduceBlock cannot
// interleave with an in-progress AddPeerBlock replay. AddPeerBlock holds
// replayMu from after its replay until after dag.mu.Unlock() on the success
// path — both functions take locks in (replayMu, dag.mu) order, no deadlock.
dag.replayMu.Lock()
defer dag.replayMu.Unlock()
dag.mu.Lock()
defer dag.mu.Unlock()

// P1-05 (audit): halt production when a prior peer-block persistence failure
// left memory state ahead of durable DB state.
dag.degradedMu.Lock()
dr := dag.degradedReason
dag.degradedMu.Unlock()
if dr != "" {
	fmt.Printf("[BLOCK] ✗ Node is degraded (%s) — block production halted. Restart to recover.\n", dr)
	return nil
}

// FIX (P0, 2026-07-04 — real production outage, superseding the
// audit-2026-06-30 gate this replaces): halting ALL production for the
// full duration of a GHOSTDAG migration turned a routine restart into a
// full network outage. Confirmed live, repeatedly, the same night this
// gate was first hardened against a *different* risk: with heavy
// concurrent traffic and a large migration backlog (~5,000 blocks,
// recurring on EVERY restart because of a separate not-yet-fixed
// two-phase block-save gap that keeps regenerating the backlog), the
// migration itself did not reliably finish within any tolerable window
// — height frozen, every API request timing out, for 6+ minutes at a
// stretch, twice in a row. The gate's original concern (a new block's
// SelectedParent chosen by comparing a migrated block's real BlueScore
// against another block's not-yet-migrated zero-value placeholder) is
// real but bounded and self-correcting: BlueScore/SelectedParent are
// locally-computed bookkeeping fields, not covered by the block hash and
// not consensus/security-critical (transactions are validated by
// signature + replay, entirely separately) — a suboptimal SelectedParent
// choice during the migration window heals itself as real scores get
// backfilled, it cannot cause a double-spend or a hash-verified fork.
// Recurring total outages are a strictly worse failure mode than that
// bounded, temporary scoring imprecision. Migration still runs and still
// backfills scores in the background; it just no longer blocks the node
// while doing so.

// FIX (audit 2026-06-30 monster audit, P1-05): refuse to mint new blocks
// while this node is still trusting one or more synthetic-checkpoint
// stubs instead of having verified that part of its ancestry. Producing
// on top of unverified history would let a peer-induced trust bypass
// silently propagate into newly-minted, otherwise-fully-verified blocks.
// SyntheticCheckpointCount() now just reads an atomic counter (no lock),
// safe to call here even though dag.mu is already held write-locked.
// Only stubs ABOVE the trusted snapshot boundary halt production. A stub AT the
// boundary (the signed-snapshot start-of-history) can never heal — no node
// retains blocks below it — so gating on the total count would strand a
// snapshot-bootstrapped node in permanent non-production (confirmed live on
// Contabo). See UnverifiedSyntheticCheckpointCount.
// Sweep finality-sealed stubs first (see releaseFinalitySealedStubs): the
// sweep normally piggybacks on maybeAdvanceFinalizedCheckpoint, which only
// runs on accepted peer blocks — a node whose peers are all down would
// otherwise never release a sealed stub and never produce, exactly the
// "downed primary must not halt everyone else" case the sync-stall valve
// below exists for. dag.mu is held (write) here, as the sweep requires.
dag.releaseFinalitySealedStubs()
if syntheticCount := dag.UnverifiedSyntheticCheckpointCount(); syntheticCount > 0 {
	fmt.Printf("[BLOCK] ✗ Node is bridging %d unverified synthetic checkpoint(s) above the snapshot boundary — block production halted until real history syncs in behind them.\n", syntheticCount)
	return nil
}

// After a snapshot resync, bootHeight is set to snapshot_import_height
// (e.g. 23093) while dag.height starts at 0.  Producing blocks here would
// create height-1 blocks whose StateRoot encodes the full snapshot state —
// peers replaying from genesis cannot reach that StateRoot and reject them
// as orphans.  Gate until the sequential catch-up sync has delivered enough
// headers that our state is consistent with the blocks we're building on.
// A 10-block buffer avoids false negatives from in-flight sync races.
if dag.bootHeight > 0 && dag.height+10 < dag.bootHeight {
	fmt.Printf("[BLOCK] ⏳ Catch-up in progress (dag.height=%d, bootHeight=%d) — skipping block production\n",
		dag.height, dag.bootHeight)
	return nil
}

// Initial-sync gate: after a restart, defer production until this node
// has caught up to within 10 blocks of the height the seed reported at
// startup. This prevents producing on a stale fork while the HTTP sync
// loop is still pulling in the seed's newer blocks — the root cause of
// "Contabo is ahead of Primary" divergence that required RESYNC_FROM_SNAPSHOT.
//
// Safety valve: if sync has made no progress for syncStallTimeout, the
// seed is likely down — fall through and produce independently so a
// downed primary never blocks all other nodes.
//
// FIX (P0, merge-reliability audit 2026-07-03): this used to measure the
// 90s window from dag.startupTime — a FIXED deadline from process start,
// regardless of whether sync was actively making progress. syncTargetHeight
// is captured ONCE at startup from the seed's height at that instant; a
// large historical gap (e.g. a fresh RESYNC_FROM_SNAPSHOT walking forward
// from genesis) can easily take longer than 90s to close even while
// succeeding continuously, since the seed keeps producing new blocks the
// whole time too. Confirmed live on Contabo: a genesis resync made 400+
// blocks of genuine progress (batches of successful "Added peer block"/
// "Merged" log lines) but the 90s deadline expired before it reached the
// target, so production resumed independently mid-catch-up — recreating
// the exact divergence (and the mutual circuit-breaker lockout that
// depends on it) the resync had just fixed. Now measured from the last
// successful peer-block acceptance (dag.lastSuccessfulPeerSyncAt, already
// tracked for /api/health/combined) instead: keeps waiting as long as sync
// keeps making progress, however long that takes, and only concludes the
// seed is down after syncStallTimeout of genuine silence.
if target := dag.syncTargetHeight.Load(); target > 0 {
	if dag.height >= target-10 {
		dag.syncTargetHeight.Store(0) // caught up — clear gate permanently
	} else {
		referenceTime := dag.lastSuccessfulPeerSyncAt.Load()
		if referenceTime == 0 {
			referenceTime = dag.startupTime // no progress yet — measure from boot
		}
		if time.Now().Unix()-referenceTime < syncStallTimeout {
			return nil // sync is actively progressing (or just started) — keep waiting
		}
		// else: no sync progress for syncStallTimeout — primary may be down → produce independently
	}
}

// Epoch-committee gate: only the selected committee members produce blocks.
// All registered node operators are ranked deterministically by
// sha256(addr+epochNum) and the top targetCommitteeSize are chosen.
// Non-committee nodes run in observer mode — syncing and verifying without
// producing — which keeps simultaneous producers bounded regardless of how
// many humans have registered. Returns nil (no block) when not selected;
// committee is recomputed lazily when the epoch number changes.
{
	nextHeight := dag.height + 1
	ec := dag.getEpochCommittee(nextHeight)
	if ec != nil && !ec.Members[dag.selfProposer] {
		return nil
	}
}
// Collect tips as parents, capped at maxParentsPerBlock.
// With many validators, every tip would create giant blocks and blow up
// GHOSTDAG merge-set computation. Select the highest-BlueScore tips
// (most recent, most authoritative) up to the cap, then sort by hash for
// deterministic block-hash computation across all nodes.
type tipEntry struct {
    hash      string
    blueScore int64
}
allTips := make([]tipEntry, 0, len(dag.tips))
for hash := range dag.tips {
    score := int64(0)
    if b, ok := dag.blocks[hash]; ok {
        score = b.BlueScore
    }
    allTips = append(allTips, tipEntry{hash, score})
}
sort.Slice(allTips, func(i, j int) bool {
    if allTips[i].blueScore != allTips[j].blueScore {
        return allTips[i].blueScore > allTips[j].blueScore
    }
    return allTips[i].hash < allTips[j].hash
})
if len(allTips) > dag.maxParents() {
    allTips = allTips[:dag.maxParents()]
}
parentHashes := make([]string, len(allTips))
for i, te := range allTips {
    parentHashes[i] = te.hash
}
sort.Strings(parentHashes) // deterministic ordering for block hash

// Height = max parent height + 1
maxParentHeight := int64(0)
for _, ph := range parentHashes {
if parent, ok := dag.blocks[ph]; ok {
if parent.Height > maxParentHeight {
maxParentHeight = parent.Height
}
}
}

dag.txMu.Lock()
txs := make([]Transaction, len(dag.pendingTxs))
copy(txs, dag.pendingTxs)
// P2-06: do NOT clear pendingTxs here — if SaveBlockWithPendingTxsAtomic
// fails below, in-memory TXs would be permanently lost.  Clear only the
// snapshot count AFTER a successful save (see post-save section below).
nTxsSnapshotted := len(txs)
dag.txMu.Unlock()

// Drain DB-persisted pending TXs — these survived a node restart and
// must now be included in a block so secondary nodes receive them.
// Without this, a transfer applied just before a restart would never
// reach secondary nodes and balances would diverge permanently.
//
// CADENCE FIX (P0, follow-up to the merge-reliability audit 2026-07-03):
// this and StateRoot() below are each their own synchronous Postgres round
// trip (~400-590ms, confirmed live on the primary's remote DB proxy) and
// neither depends on the other's result — StateRoot hashes cs.accounts'
// CURRENT state, which already reflects every TX's effect regardless of
// which block ends up bundling it (see AddPeerBlock's "post-state, not
// pre-state" comment), while this call only affects which TXs land in
// THIS block's Transactions list. Running them concurrently instead of
// back-to-back turns 2 sequential round trips into the wall-clock cost of
// one, directly shortening how long ProduceBlock holds dag.mu.
//
// This matters beyond raw speed: main.go's block-production ticker aligns
// every node to the same wall-clock BLOCK_TIME boundary specifically so
// concurrent production merges naturally (see its own comment — "both
// validators produce within <50ms of each other on every tick"). A
// secondary with a fast LOCAL Postgres (e.g. Contabo, same Docker network,
// near-zero round-trip) sustains close to the true 1s cadence, while a
// primary whose DB calls each cost hundreds of ms falls further and
// further behind tick-for-tick — not a one-time "overtake" but a
// permanently widening height gap between two nodes that are still
// technically merging each other's blocks correctly. Narrowing the
// primary's own per-block DB cost is what keeps both sides' cadence close
// enough for that wall-clock alignment to do its job.
var dbTxs []Transaction
var pendingTxIDs []int64
var stateRoot string
var pendingDur, rootDur time.Duration
dbPairStart := time.Now()
var cadenceWG sync.WaitGroup
cadenceWG.Add(2)
go func() {
	defer cadenceWG.Done()
	// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go. Also
	// prevents cadenceWG.Wait() below from deadlocking forever on a panic —
	// Done() (deferred above, so it still runs during this unwind) must fire
	// either way.
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] ProduceBlock LoadPendingTxs goroutine: %v\n%s\n", r, debug.Stack())
		}
	}()
	t0 := time.Now()
	if dag.state != nil {
		dbTxs, pendingTxIDs = dag.state.LoadPendingTxs()
	}
	pendingDur = time.Since(t0)
}()
go func() {
	defer cadenceWG.Done()
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] ProduceBlock StateRoot goroutine: %v\n%s\n", r, debug.Stack())
		}
	}()
	t0 := time.Now()
	stateRoot = dag.state.StateRoot()
	rootDur = time.Since(t0)
}()
cadenceWG.Wait()
// Ongoing health check: these two DB round trips run concurrently
// specifically to shorten how long ProduceBlock holds dag.mu (see the
// comment above this block) — worth knowing which one is slow if the pair
// itself is ever slow, permanently, not just for the resolved 2026-07-03
// cadence incident this concurrency fix was originally built for.
if d := time.Since(dbPairStart); d > 1200*time.Millisecond {
	fmt.Printf("[BLOCK] ⏱ dbpair detail: LoadPendingTxs=%s StateRoot=%s\n", pendingDur, rootDur)
}
if len(dbTxs) > 0 {
	fmt.Printf("[DAG] Including %d restart-surviving TX(s) from DB in block\n", len(dbTxs))
	txs = append(txs, dbTxs...)
}

proposer := dag.nodeID
if dag.signingKey != nil {
	// Use the Ethereum address derived from the signing key so peer nodes
	// can verify the block signature against a known Ethereum address.
	// The libp2p nodeID is used for network routing; the signing address
	// is what peers need for consensus verification.
	proposer = crypto.PubkeyToAddress(dag.signingKey.PublicKey).Hex()
}
block := &Block{
Height:       maxParentHeight + 1,
Timestamp:    time.Now().Unix(),
ParentHashes: parentHashes,
Proposer:     proposer,
Humans:       dag.state.TotalHumans(),
Transactions: txs,
StateRoot:    stateRoot,
ProducedAtMs: time.Now().UnixMilli(),
}
block.Hash = dag.calculateHash(block)
if dag.signingKey != nil {
	hashBytes := common.HexToHash(block.Hash)
	if sig, err := crypto.Sign(hashBytes[:], dag.signingKey); err == nil {
		block.Signature = hex.EncodeToString(sig)
	} else {
		fmt.Printf("[BLOCK] Warning: could not sign block #%d: %v\n", block.Height, err)
	}
}

// P1-04 (audit): compute GHOSTDAG before persisting so the DB row has
// correct selected_parent/blue_score/blues from the start — no crash window
// with empty GHOSTDAG fields. dag.mu is already held here; parents are in
// dag.blocks; the block is not yet in dag.blocks (not needed by compute).
dag.computeGHOSTDAGState(block)

// P1-06 (audit): persist to DB BEFORE inserting into dag.blocks/dag.tips or
// returning the block for broadcast. If the DB save fails this block will be
// lost on restart while peers may have accepted it — return nil to skip
// broadcast entirely. TXs stay in pending_txs (atomic rollback) for the next
// ProduceBlock tick to re-include.
saveStart := time.Now()
if err := dag.state.SaveBlockWithPendingTxsAtomic(block, pendingTxIDs); err != nil {
	fmt.Printf("[BLOCK] ⚠ Could not persist block #%d (%s...): %v — skipping broadcast, TXs stay queued\n",
		block.Height, block.Hash[:16], err)
	return nil
}
// Ongoing health check: this call runs synchronously while dag.mu is held
// write-locked, so if it's slow, EVERY other dag.mu consumer (API reads,
// AddPeerBlock) stalls for the same duration.
if saveDur := time.Since(saveStart); saveDur > 500*time.Millisecond {
	fmt.Printf("[BLOCK] ⏱ SaveBlockWithPendingTxsAtomic took %s for block #%d (dag.mu held throughout)\n", saveDur, block.Height)
}

// P2-06: clear exactly the TXs we snapshotted — any TXs queued AFTER the
// snapshot (positions [nTxsSnapshotted:]) stay for the next block.
dag.txMu.Lock()
if nTxsSnapshotted > 0 {
	if nTxsSnapshotted <= len(dag.pendingTxs) {
		dag.pendingTxs = dag.pendingTxs[nTxsSnapshotted:]
	} else {
		dag.pendingTxs = nil
	}
}
dag.txMu.Unlock()

dag.blocks[block.Hash] = block
// GHOSTDAG already computed above (P1-04); no second call needed.
dag.replayedMu.Lock()
dag.replayedBlocks[block.Hash] = true
dag.replayedMu.Unlock()

// Remove all parents from tips, add this block as new tip
for _, ph := range parentHashes {
delete(dag.tips, ph)
}
dag.tips[block.Hash] = true
dag.height = block.Height

// FIX (P0, merge-reliability audit 2026-07-03): AddPeerBlock advances the
// hard finality checkpoint after accepting a block, but ProduceBlock never
// did — this is the ONLY other place a block's GHOSTDAG state (BlueScore/
// SelectedParent, computed above) becomes final, so a node that produces
// its own blocks but never successfully accepts a peer block (e.g. fully
// isolated by a circuit-breaker lockout, or simply the only validator
// currently reachable) could NEVER advance its own checkpoint at all,
// confirmed live on Contabo: finalized_height stuck at 80094 through
// 50,000+ of its own self-produced blocks. dag.mu is already held here,
// matching maybeAdvanceFinalizedCheckpoint's precondition.
//
// FIX (P0, 2026-07-04 — Contabo 2 permanent-isolation incident): gated by
// selfProducedFinalityAllowed so a node that's actually isolated from known
// peers pauses its own hardening instead of permanently sealing off the
// real chain at heights it never merged — see that function's own comment
// and isolatedFinalityPauseWindow for the full incident and rationale.
if dag.selfProducedFinalityAllowed() {
	dag.maybeAdvanceFinalizedCheckpoint(block)
} else if last := dag.lastIsolationPauseLogAt.Load(); time.Now().Unix()-last > 30 &&
	dag.lastIsolationPauseLogAt.CompareAndSwap(last, time.Now().Unix()) {
	fmt.Printf("[FINALITY] ⏸ Self-produced checkpoint advance paused at height %d — no other authorized validator's block merged in over %s despite %d known validator(s); this node may be isolated. Checkpoint will resume advancing the moment a peer block merges again.\n",
		block.Height, isolatedFinalityPauseWindow, len(dag.authorizedValidators))
}

// Post-commit bookkeeping (block-count reward weighting + the max_block_height
// restart hint) — moved off the hot, lock-held path in ONE background write.
//
// CADENCE FIX (2026-07-02): both are plain DB writes that do NOT touch dag.mu
// state, yet they ran synchronously while dag.mu was write-locked. Over the
// primary's remote DB proxy (~560ms/round-trip, confirmed live) that added two
// extra round trips (~1.1s) to every block's lock-held critical section — on
// top of the block save itself — directly inflating cadence and starving peer
// sync/merge. The block itself is already durably persisted above
// (SaveBlockWithPendingTxsAtomic) BEFORE this point, so neither of these is
// consensus-critical: IncrementBlockCount is reward weighting (additive, and
// re-derivable), and max_block_height only seeds dag.height on the next boot —
// a value LoadBlocksFromDB already reconstructs from chain_blocks' own max, so
// this config key is a fast-path hint, not the source of truth. Losing at most
// the last block's write on an ill-timed crash is harmless and self-corrects.
heightVal := dag.height
SafeGoroutine("ProduceBlock-post-persist", func() {
	dag.state.IncrementBlockCount(proposer)
	// setConfigValueDB, not setConfigValue: this goroutine holds neither
	// cs.mu nor dag.mu, and setConfigValue's precondition (cs.mu held, so
	// cs.dbExec() safely reads cs.activeTx) does not hold here — see
	// setConfigValue's own doc comment. Using it unlocked would race on
	// cs.activeTx against any concurrently-running cs.mu-holding operation.
	dag.state.setConfigValueDB("max_block_height", fmt.Sprintf("%d", heightVal))
})

if len(parentHashes) > 1 {
fmt.Printf("[DAG] 🔀 Merged %d tips into block #%d\n", len(parentHashes), block.Height)
}

return block
}

// WithBlockProductionPaused runs fn while holding the same lock
// ProduceBlock takes for its entire body (tip/parent selection, pending-TX
// drain, and the final dag.state.StateRoot() read are all done under
// dag.mu — see ProduceBlock above).
//
// FIX (audit recheck 2, P0 #2): daily distribution (main.go) mutates state
// across several separate calls (DistributeUBIPool, then
// DistributeValidatorsPool, then DistributeLPPool, then escrow) and only
// persists the TX explaining each mutation a moment later via SavePendingTx
// — without this guard, ProduceBlock's 6-second ticker could fire in the
// gap between a mutation and its TX, assembling a block whose StateRoot
// already reflects the mutation but whose Transactions list doesn't yet
// include the TX that explains it. No other node could ever reproduce
// that StateRoot by replaying that block. Wrapping the entire distribution
// round (every mutation AND every corresponding SavePendingTx call) in
// this guard makes ProduceBlock block until the round finishes, the same
// way it already serializes against AddPeerBlock's replay via replayMu.
func (dag *BlockDAG) WithBlockProductionPaused(fn func()) {
	dag.mu.Lock()
	defer dag.mu.Unlock()
	fn()
}

// maxOrphans caps total queued orphan blocks across all missing-parent keys,
// so a malicious or buggy peer sending blocks that reference parents which
// will never arrive can't grow this map without bound.
//
// FIX: confirmed in production at 2000 — a node that fell significantly
// behind (multiple validators producing every ~6s while it was still
// catching up on a large historical gap) overflowed this buffer, silently
// DROPPING individual blocks with no record of which hash was missing.
// Once dropped, no mechanism (not even fetchMissingAncestors, which walks
// back from recorded missing-parent hashes) can ever learn to re-fetch that
// specific block — if the BlockDAG's multi-parent tolerance lets later tips
// route around the gap via a sibling branch instead, that block's
// transactions are gone from this node's view forever, a real, confirmed
// divergence (a transfer present on two other nodes never landed on the
// one that overflowed). Raised by 25x to make this far less likely to
// trigger under the same catch-up load; does not fix the underlying
// lossy-on-overflow design (tracked separately — recovery today is a full
// resync from a signed snapshot, see ImportSnapshotFromURL).
// maxOrphans is the orphan-buffer cap. A 3-validator chain at height 44k has
// up to ~132k distinct block hashes in flight; 200k gives comfortable headroom
// so a catch-up node can hold ALL historical orphans in memory until the
// sequential download cascade resolves them, without dropping blocks that would
// break the resolution chain. At ~500 bytes/block average: ~100 MB peak usage.
const maxOrphans = 200_000

// maxOrphanHeightGap is the fork-flood backpressure (P0, 2026-07-02 incident):
// a block whose height is more than this far ABOVE our own is never queued as
// an orphan. Such a block is either the tip of a fork that diverged from ours
// (its ancestors live only on that other fork and can never be fetched here —
// so queuing it just burns CPU on unresolvable resolution) or a legitimately-
// far-ahead peer we must catch up to via the ORDERED sync (doSyncOnce pages
// parents-first from our height forward), NOT via out-of-order orphan gossip.
// Either way, dropping it is correct: ordered sync fills any legitimate gap.
// Without this, three nodes that diverged onto 67k/80k/94k forks each pushed
// their whole fork at the primary, which queued tens of thousands of orphans it
// could never resolve and hung at 100% CPU. 5000 is far above real gossip lag
// (a handful of blocks) while decisively cutting off a runaway fork.
const maxOrphanHeightGap = 5000

// orphanAbandonAfter bounds how long this node will keep trying to resolve a
// single missing-parent hash before giving up on it for good.
//
// FIX (the real completion of the orphan saga, not another mitigation): the
// eager triggerOrphanResolve above fixed the *original* bug (an orphan
// sitting idle for up to 6s before the next retry) but, confirmed live on
// the VPS and CD20 immediately after deploying it, exposed a second one —
// every NEW orphan re-triggers a resolution pass over EVERY pending
// missing-parent hash, including ones that have been failing for minutes
// because the block genuinely no longer exists on ANY reachable peer (it
// was lost during the original pre-fix orphan-overflow incident: confirmed
// by checking the VPS itself, which produced/relayed that exact chain, and
// it doesn't have the ancestor either). With validators producing a new
// block every ~6s, that's a fresh full sweep — including an HTTP request to
// every peer for the dead hash — roughly 10x a minute, forever, compounding
// across however many nodes are simultaneously stuck on the same dead
// branch. That retry storm is what was timing out CD20→VPS requests, not a
// network failure: confirmed by checking account balances on every node
// (they match exactly, 1134/866 AEQ, total_supply 2000 everywhere) — the
// stuck branch is provably an empty/no-value side-chain, not a transaction
// any node still needs. Past this timeout, stop retrying a specific hash
// and drop everything waiting on it, freeing the memory and ending the
// storm, instead of retrying something proven unfetchable forever.
//
// FIX (2026-06-28, second incident — confirmed this hash WAS genuinely
// fetchable, not dead): raised 3min -> 15min AND made abandonment require
// minOrphanAttemptsBeforeAbandon real fetch attempts, not just elapsed
// time. Root cause: a single catch-up backlog after several restarts that
// each fragmented this node's own validator chain into short-lived forks
// produced tens of thousands of distinct missing-parent hashes at once —
// far more than fetchMissingAncestors' per-cycle batch could include. A
// hash sitting deep in that backlog could age past the old 3-minute window
// without ever once being included in a fetch batch — its "first seen"
// clock ran out purely from queueing delay, not because any peer actually
// failed to provide it. Verified live: curl against the primary's own
// /api/blocks/by-hash for one such "abandoned" hash returned the block
// immediately — it was never unreachable, just never tried in time.
const orphanAbandonAfter = 15 * time.Minute

// minOrphanAttemptsBeforeAbandon — see orphanAbandonAfter's comment. A hash
// must have been genuinely included in at least this many fetch batches
// (not just "time has passed") before it can be abandoned.
const minOrphanAttemptsBeforeAbandon = 3

// proposerBreakerOrphanGrace bounds how long a missing-parent gap gets the
// benefit of the doubt before AddPeerBlock starts counting it as a proposer
// circuit-breaker failure.
//
// FIX (durable fix, 2026-07-04 — explicit user requirement: this must work
// reliably for every new node, not just today's two): confirmed live, two
// healthy, fully-synced Contabo nodes tripped their circuit breakers against
// EACH OTHER during completely normal, steady-state operation (not a resync,
// not a divergence — both already agreed with the primary at settled
// heights) — an ordinary propagation gap between two independently-hosted
// validators (a block from one arriving before its very-recent parent from
// the other has finished propagating, entirely expected over a real
// intercontinental/inter-provider network under a 2s BLOCK_TIME) orphaned a
// SHORT run of blocks, and the breaker counted every single one as a
// failure the instant it was queued — with no way to tell "will resolve in
// the next second or two" apart from "this proposer is on a permanently
// diverged fork" at the moment the orphan is first seen. Once BOTH sides'
// breakers tripped against each other this way, each side's own pushed
// blocks kept getting dropped by the other's lock-free shield before ever
// reaching AddPeerBlock again — a self-sustaining mutual lockout neither
// side's cooldown alone reliably escaped, since a re-tripped breaker resets
// the wait. This is exactly the failure mode this project cannot afford as
// more independently-run, often non-technical-operator nodes join: a
// transient network hiccup between any two of them must never escalate
// into a lasting mutual block.
//
// The fix: don't count a failure the instant a gap is first observed — only
// once the SAME missing-parent hash has stayed unresolved for this long.
// Genuine propagation lag resolves within a round or two and is never
// penalized at all; a genuinely diverged fork's gap never resolves and
// still trips the breaker in well under a minute, comfortably fast enough
// to still protect against a real flood. A few multiples of BLOCK_TIME (2s)
// comfortably covers ordinary propagation variance without meaningfully
// slowing down real divergence detection.
//
// FIX (2026-07-05 — Contabo 2 still repeatedly tripping at BLOCK_TIME=1s
// even after proposerBreakerFailThreshold's own scaling): real
// cross-provider propagation+processing delay is a roughly FIXED
// wall-clock quantity, independent of BLOCK_TIME — it does not shrink
// just because blocks are produced faster. A var (not const) so
// TuneProposerBreakerForBlockTime can widen it at a faster-than-baseline
// cadence; never narrows it at a slower one.
var proposerBreakerOrphanGrace = 8 * time.Second

// orphanAge reports how long missingParent has been sitting in the orphan
// queue (zero, false if it isn't currently tracked at all — e.g. this is
// the very first block ever queued waiting on it). Used by AddPeerBlock to
// decide whether an orphan is still within its circuit-breaker grace period
// — see proposerBreakerOrphanGrace's own comment.
func (dag *BlockDAG) orphanAge(missingParent string) (time.Duration, bool) {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	first, ok := dag.orphanFirstSeen[missingParent]
	if !ok {
		return 0, false
	}
	return time.Since(first), true
}

// IsWithinOrphanGrace reports whether block currently fails to attach ONLY
// because of a missing-parent gap that is still within
// proposerBreakerOrphanGrace — i.e. this looks like ordinary propagation
// lag, not a genuine, sustained problem. Re-derives the same missing-parent
// determination AddPeerBlock itself just made (via ghostdagBlockLookup, the
// same unbounded lookup AddPeerBlock's own parent-existence check uses) so
// it stays accurate even when the real rejection reason was something else
// entirely (bad signature, unauthorized proposer, etc.) — in that case this
// correctly returns false, so THAT failure still counts immediately.
//
// Used by the HTTP block-push handler (api.go) to apply the same
// forgiveness to its own, independent per-IP circuit breaker that
// AddPeerBlock already applies to the per-proposer one — see
// proposerBreakerOrphanGrace's own comment for why: without this, the
// per-IP breaker still tripped on ordinary propagation gaps even after that
// fix, recreating the exact mutual-lockout risk between two healthy nodes
// through this second, independent breaker (confirmed live: both
// "[DAG] Circuit breaker open" AND "[BLOCK-PUSH] Dropping pushes" fired
// together during the same incident).
func (dag *BlockDAG) IsWithinOrphanGrace(block *Block) bool {
	if block == nil {
		return false
	}
	dag.mu.Lock() // ghostdagBlockLookup can cache-fill dag.blocks
	var missingParent string
	for _, ph := range block.ParentHashes {
		if dag.ghostdagBlockLookup(ph, nil) == nil {
			missingParent = ph
			break
		}
	}
	dag.mu.Unlock()
	if missingParent == "" {
		return false // not an orphan at all — rejected for some other reason
	}
	age, tracked := dag.orphanAge(missingParent)
	return !tracked || age < proposerBreakerOrphanGrace
}

// orphanFetchCooldown is the minimum gap between fetch attempts for the same
// missing-parent hash, checked by fetchMissingAncestors (sync_blocks.go).
// Without it, every new orphan's triggerOrphanResolve pass re-attempts every
// OTHER still-pending hash too, even ones whose last attempt was a second
// ago — multiplying request volume by however often new orphans arrive.
const orphanFetchCooldown = 10 * time.Second

// RecordOrphanAttempt marks hash as having been genuinely included in a
// fetch batch sent to a peer — called by fetchMissingAncestors
// (sync_blocks.go) for every hash it actually sends, never for ones merely
// pending. See orphanAbandonAfter's comment for why this, not wall-clock
// time alone, gates abandonment.
func (dag *BlockDAG) RecordOrphanAttempt(hash string) {
	dag.orphansMu.Lock()
	dag.orphanAttempts[hash]++
	dag.orphansMu.Unlock()
}

// abandonOrphansWaitingFor removes all orphans queued waiting on hash as
// their missing parent, then recurses into each abandoned block's own hash
// to clear any further orphans that were waiting on THOSE blocks.
//
// Called when a block is permanently rejected (e.g. unauthorized proposer),
// so descendants can never be resolved either.  Without this, orphans whose
// only missing parent was a permanently-rejected block would sit in the queue
// until orphanAbandonAfter — and since fetchMissingAncestors only calls
// RecordOrphanAttempt when the PEER doesn't have a block (not when AddPeerBlock
// rejects it), the attempt counter never incremented, so the TTL prune in
// queueOrphan never triggered.  This function short-circuits that wait.
func (dag *BlockDAG) abandonOrphansWaitingFor(hash string) {
	dag.orphansMu.Lock()
	waiting := dag.orphans[hash]
	delete(dag.orphans, hash)
	delete(dag.orphanFirstSeen, hash)
	delete(dag.orphanLastAttempt, hash)
	delete(dag.orphanAttempts, hash)
	dag.orphansMu.Unlock()
	if len(waiting) == 0 {
		return
	}
	fmt.Printf("[DAG] Abandoned %d orphan block(s) waiting for permanently-rejected block %s… (proposer not in authorized validator set)\n",
		len(waiting), hash[:min(16, len(hash))])
	for _, b := range waiting {
		dag.abandonOrphansWaitingFor(b.Hash)
	}
}

// queueOrphan stores block, which is waiting on missingParent to appear,
// and logs the wait (the old code dropped this case with zero logging).
func (dag *BlockDAG) queueOrphan(missingParent string, block *Block) {
	// Fork-flood backpressure — see maxOrphanHeightGap. Reject blocks whose
	// height is implausibly far above our frontier BEFORE they ever enter the
	// orphan buffer or its resolution machinery, so a diverged/runaway fork
	// can't hang this node. The frontier is max(dag.height, bootHeight), not
	// dag.height alone — see farAheadFrontierLocked for why a snapshot-
	// bootstrapped node (dag.height 0, bootHeight at the snapshot boundary) must
	// still accept the block that attaches to its checkpoint stub. dag.mu is NOT
	// held here (AddPeerBlock unlocked it before calling us).
	if frontier := dag.farAheadFrontier(); block.Height > frontier+maxOrphanHeightGap {
		fmt.Printf("[DAG] ✗ Dropped far-ahead orphan #%d from %s: %d blocks above frontier %d (cap %d) — likely a diverged fork; ordered sync will pull any legitimate gap parents-first\n",
			block.Height, block.Proposer, block.Height-frontier, frontier, maxOrphanHeightGap)
		return
	}
	dag.orphansMu.Lock()
	now := time.Now()
	if first, ok := dag.orphanFirstSeen[missingParent]; ok {
		if now.Sub(first) > orphanAbandonAfter && dag.orphanAttempts[missingParent] >= minOrphanAttemptsBeforeAbandon {
			waiting := append([]*Block{}, dag.orphans[missingParent]...)
			waiting = append(waiting, block) // this delivery too — it's the one that triggered this check
			delete(dag.orphans, missingParent)
			delete(dag.orphanFirstSeen, missingParent)
			delete(dag.orphanLastAttempt, missingParent)
			delete(dag.orphanAttempts, missingParent)
			dag.orphansMu.Unlock()
			// FIX (2026-06-30, confirmed live on Contabo): this used to just
			// discard `waiting` — correct for a genuine dead-end sibling, but
			// WRONG for a block whose only problem is depending on a
			// permanently-missing-from-every-peer ancestor (e.g. one of this
			// node's own stray blocks from an earlier broken run, embedded as a
			// merge-set parent deep in a peer's real chain — see
			// BridgeHistoricalGap's comment for the same class of gap). Without
			// this, every block depending — even transitively — on such a hash
			// was lost forever, and dag.height could never advance past it: a
			// fresh RESYNC just relocates the exact same wall to wherever the
			// next unresolvable historical reference happens to sit. Bridge it
			// the same way BridgeHistoricalGap does at startup (synthetic
			// checkpoint stub, BlueScore 0 — see its P1-05 comment for why),
			// then retry every block that was waiting on it through the normal
			// AddPeerBlock path instead of dropping them.
			//
			// FIX (audit 2026-06-30 monster audit, P1-05): unlike
			// BridgeHistoricalGap (which only ever runs once, at startup,
			// against the boot-time snapshot gap), this branch could fire
			// silently at any point during normal long-running operation —
			// turning an ordinary sync hiccup into a permanent trust-bypass
			// stub with no operator visibility or opt-in. Gate it behind an
			// explicit flag so this stays a deliberate operational decision
			// (the same way it was when we manually used it to unstick
			// Contabo) rather than something the node does to itself by
			// default. Default-off: an operator who hits this without the
			// flag set still gets the OLD behavior (drop `waiting`, the gap
			// surfaces as repeated orphan-queue log lines instead of being
			// silently bridged) until they explicitly opt in.
			if os.Getenv("ALLOW_RUNTIME_ORPHAN_BRIDGE") != "true" {
				fmt.Printf("[DAG] ✗ Abandoning %d block(s) waiting on permanently-unresolvable parent %s... — set ALLOW_RUNTIME_ORPHAN_BRIDGE=true to bridge gaps like this automatically (trust bypass, see synthetic_checkpoint_events for the audit trail any bridge would leave)\n",
					len(waiting), missingParent[:min(16, len(missingParent))])
				return
			}
			minWaitingHeight := waiting[0].Height
			for _, b := range waiting {
				if b.Height < minWaitingHeight {
					minWaitingHeight = b.Height
				}
			}
			stubH := minWaitingHeight - 1
			if stubH < 0 {
				stubH = 0
			}
			dag.mu.Lock()
			stubInserted := false
			if _, exists := dag.blocks[missingParent]; !exists {
				dag.blocks[missingParent] = &Block{
					Hash:         missingParent,
					Height:       stubH,
					BlueScore:    0,
					Proposer:     "synthetic-checkpoint",
					ParentHashes: []string{},
				}
				// FIX (2026-06-30, confirmed live in production): deliberately
				// NOT added to dag.tips — see BridgeHistoricalGap's identical
				// fix/comment for why an accumulating stub-as-tip bloats every
				// future ProduceBlock's merge-set computation enough to starve
				// the whole node. The stub only needs to exist in dag.blocks to
				// satisfy AddPeerBlock's parent-existence check.
				stubInserted = true
				dag.syntheticCheckpointCount.Add(1)
				// A runtime-orphan-bridge stub is created during normal operation
				// (past catch-up), so it sits above bootHeight and represents a
				// genuine mid-chain gap that DOES gate production — see
				// unverifiedSyntheticCheckpointCount's comment. Same bootHeight
				// comparison as BridgeHistoricalGap; raw field (not BootHeight(),
				// which takes RLock) because dag.mu is held here — which is also
				// what makes the unverifiedStubHeights write race-free.
				if stubH > dag.bootHeight {
					dag.unverifiedSyntheticCheckpointCount.Add(1)
					dag.unverifiedStubHeights[missingParent] = stubH
				}
			}
			dag.mu.Unlock()
			fmt.Printf("[DAG] (housekeeping) bridged permanently-unresolvable parent %s... (height ~%d) with a synthetic checkpoint — retrying %d block(s) that were waiting on it in the background, no effect on account balances\n",
				missingParent[:min(16, len(missingParent))], stubH, len(waiting))
			if stubInserted {
				// Counter/map maintenance moved INTO the dag.mu critical section
				// above (2026-07-03): the unverifiedStubHeights map requires it.
				// FIX (audit 2026-06-30 monster audit, P1-05): see
				// RecordSyntheticCheckpointEvent's comment — durable audit trail,
				// tagged "runtime-orphan-bridge" so it's distinguishable from a
				// startup-time BridgeHistoricalGap stub.
				if dag.state != nil {
					SafeGoroutine("RecordSyntheticCheckpointEvent-runtime", func() { dag.state.RecordSyntheticCheckpointEvent(missingParent, stubH, "runtime-orphan-bridge") })
				}
			}
			// FIX (2026-06-30, confirmed live on Contabo): retrying `waiting`
			// SYNCHRONOUSLY here (calling AddPeerBlock directly in this call
			// stack) could cascade into many nested AddPeerBlock calls — each
			// waiting block can itself be blocking on another hash whose own
			// 15-minute window has also just expired (a burst of orphans queued
			// around the same time during catch-up tends to expire together) —
			// and each successful add holds dag.mu for its full GHOSTDAG-compute
			// + DB-write duration. A long synchronous chain of those starved
			// every other goroutine waiting on dag.mu, including the health
			// endpoint's read lock: confirmed live as a 90+ second stretch of
			// zero new log output at 200-500% CPU with the HTTP API timing out.
			// A background goroutine still does the same retries (still bounded
			// by dag.mu's normal fairness — no special priority) but lets THIS
			// call return immediately instead of making the original
			// AddPeerBlock caller (and everything waiting on dag.mu behind it)
			// block for however long the whole retry chain takes.
			go func(toRetry []*Block) {
				// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go.
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("[PANIC RECOVERED] orphan-retry goroutine: %v\n%s\n", r, debug.Stack())
					}
				}()
				for _, b := range toRetry {
					dag.AddPeerBlock(b)
				}
			}(waiting)
			return
		}
	} else {
		dag.orphanFirstSeen[missingParent] = now
	}
	// FIX (audit recheck2, P2 #11): the same block can legitimately arrive
	// more than once before its parent shows up — once via P2P broadcast,
	// again via an HTTP-SYNC page that happens to cover the same height, and
	// again on every syncWithNode retry while the gap persists. Without a
	// hash check here, each delivery appended a fresh duplicate entry,
	// burning through maxOrphans' budget on copies of a block already
	// waiting rather than genuinely distinct blocks, and inflating the
	// "N block(s) now waiting" counts in the logs above their real value.
	for _, b := range dag.orphans[missingParent] {
		if b.Hash == block.Hash {
			dag.orphansMu.Unlock()
			return
		}
	}
	total := 0
	for _, v := range dag.orphans {
		total += len(v)
	}
	if total >= maxOrphans {
		dag.orphansMu.Unlock()
		fmt.Printf("[DAG] ✗ Dropped peer block #%d: orphan buffer full (%d waiting), missing parent never arrived\n",
			block.Height, total)
		return
	}
	dag.orphans[missingParent] = append(dag.orphans[missingParent], block)
	waitingCount := len(dag.orphans[missingParent])
	dag.orphansMu.Unlock()
	fmt.Printf("[DAG] ⏳ Block #%d from %s queued as orphan — missing parent %s... (%d block(s) now waiting on it)\n",
		block.Height, block.Proposer, missingParent[:min(16, len(missingParent))], waitingCount)

	// FIX (the actual completion of the orphan-buffer mitigation, not just a
	// bigger cap): before this, a newly queued orphan sat untouched until the
	// next periodic syncWithNode tick — up to 6s later, PER peer, and only
	// one peer's fetchMissingAncestors ran per tick. Under sustained load
	// (multiple validators producing every ~6s while a node is still deep in
	// catch-up) new orphans can arrive faster than that cadence drains them,
	// which is exactly how the buffer reached its cap in production. Kicking
	// off resolution immediately, against every currently-syncing peer in
	// parallel, the instant a gap is detected — instead of waiting for the
	// next tick — closes that race instead of just buying more headroom
	// before it recurs.
	SafeGoroutine("triggerOrphanResolve", dag.triggerOrphanResolve)
}

// popOrphans returns and removes every block that was waiting on parentHash.
func (dag *BlockDAG) popOrphans(parentHash string) []*Block {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	waiting := dag.orphans[parentHash]
	delete(dag.orphans, parentHash)
	delete(dag.orphanFirstSeen, parentHash)
	delete(dag.orphanLastAttempt, parentHash)
	delete(dag.orphanAttempts, parentHash)
	return waiting
}

// MissingParentHashes returns a snapshot of every hash currently blocking at
// least one queued orphan. Used by fetchMissingAncestors (sync_blocks.go) to
// know exactly which specific ancestor blocks to fetch by hash.
func (dag *BlockDAG) MissingParentHashes() []string {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	hashes := make([]string, 0, len(dag.orphans))
	for h := range dag.orphans {
		hashes = append(hashes, h)
	}
	return hashes
}

// hasAwaitingOrphan reports whether hash is currently the missing-parent key
// for at least one queued orphan — i.e. something is genuinely, actively
// waiting on it right now, in the present tense. See AddPeerBlock's
// bootHeight-skip call site for why this matters: a SelfFetched delivery
// only proves a fetch was deliberately issued for a hash something needed
// WHEN THE REQUEST WENT OUT, not that the need still exists when the
// response arrives — a resync in between can clear the orphan queue entry
// that originally justified the fetch.
func (dag *BlockDAG) hasAwaitingOrphan(hash string) bool {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	return len(dag.orphans[hash]) > 0
}

// hasBlockInMemory is a cheap, memory-only existence check (no DB fallback)
// for entry points that can receive the SAME block through multiple
// redundant channels — a live block routinely arrives via P2P direct, P2P
// gossip relay (broadcastExcept re-broadcasts every received block to every
// other connected peer), and HTTP push, all independently. Until 2026-07-05
// these entry points (p2p.go's handleBlockStream, api.go's handleBlockPush)
// called AddPeerBlock unconditionally for every one of those redundant
// deliveries; doSyncOnce (sync_blocks.go) already had this exact check.
// Confirmed live: raw arrival counts ran 2-4x the actual block-production
// rate, with recordRawArrivalLatency (finality.go) — added the same
// night specifically to see this — showing growing multi-second-to-minute
// "ages" for what were really just late redundant copies of blocks handled
// long before, wasting a full AddPeerBlock call (dag.mu, resyncInProgress,
// every gate) purely to rediscover "already known" each time. Not
// exhaustive — a block pruned from dag.blocks but still known via the DB
// reports false here even though AddPeerBlock's own deeper check would
// still recognize it; that's fine, this is an optimization for the common
// case (a block from within the last few seconds), not a correctness
// boundary — AddPeerBlock remains the real, authoritative check.
func (dag *BlockDAG) hasBlockInMemory(hash string) bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	_, ok := dag.blocks[hash]
	return ok
}

// shouldAttemptFetch reports whether enough time has passed since the last
// fetch attempt for hash to try again, and records this attempt if so. See
// orphanFetchCooldown for why this exists — without it, every new orphan's
// resolve pass re-hits every other pending hash regardless of how recently
// it was last tried.
func (dag *BlockDAG) shouldAttemptFetch(hash string) bool {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	if last, ok := dag.orphanLastAttempt[hash]; ok && time.Since(last) < orphanFetchCooldown {
		return false
	}
	dag.orphanLastAttempt[hash] = time.Now()
	return true
}

// isCatchingUpLocked reports whether this node is still performing an initial
// catch-up sync and therefore must NOT feed the per-proposer circuit breaker
// on far-ahead or orphaned peer blocks. During catch-up every honest
// validator's live blocks legitimately look far-ahead or orphaned simply
// because we haven't synced their ancestry yet — tripping the breaker then
// blocks the very peer we need to sync from (confirmed live on Contabo).
//
// Two independent catch-up signals, either sufficient:
//   - bootHeight: captured from the DB at startup (max_block_height /
//     snapshot_import_height). Covers the post-RESYNC_FROM_SNAPSHOT case where
//     local state already encodes a high height but dag.height starts low.
//   - syncTargetHeight: captured live from a seed's reported height at startup
//     (fetchAndSetSyncTarget). Covers the fresh-DB / genesis-start case where
//     bootHeight is legitimately 0 yet the network is thousands of blocks
//     ahead. Without this second signal, a brand-new node booting from genesis
//     (height 0, bootHeight 0) trips its breaker against the primary on the
//     primary's own far-ahead pushed blocks and can never sync past height 0 —
//     the exact stall observed on Contabo (registered with the primary, genesis
//     only, never advanced). Both signals self-clear once caught up
//     (dag.height reaches within 10 of the target), restoring normal breaker
//     behaviour.
//
// Caller must hold dag.mu (read or write). syncTargetHeight is atomic and
// independent of dag.mu.
func (dag *BlockDAG) isCatchingUpLocked() bool {
	if dag.bootHeight > 0 && dag.height+10 < dag.bootHeight {
		return true
	}
	if target := dag.syncTargetHeight.Load(); target > 0 && dag.height+10 < target {
		return true
	}
	return false
}

// farAheadFrontierLocked returns the height against which the far-ahead
// fork-flood cap (maxOrphanHeightGap) is measured: the highest height this node
// legitimately treats as its frontier. That is max(dag.height, bootHeight) —
// NOT dag.height alone.
//
// After a snapshot bootstrap/resync the node trusts state up to bootHeight (the
// snapshot boundary) and holds a synthetic-checkpoint stub at that height, but
// dag.height stays 0 because the stub is deliberately never marked a tip (see
// BridgeHistoricalGap). Measuring far-ahead against dag.height=0 then rejects
// the peer's earliest real block — the one exactly above the snapshot boundary
// whose parent IS that stub — as "far-ahead", so it can never attach and the
// node is stranded at height 0 forever. Confirmed live on Contabo: the primary,
// itself snapshot-resynced, serves nothing below block #76138 (parent = the
// stub at 76137); against dag.height=0 that block is 76138 > 0+5000 and was
// dropped before the parent-existence check that would have attached it.
//
// bootHeight is derived solely from the local DB (max_block_height /
// snapshot_import_height, set only by real persisted blocks or a signed
// snapshot resync) and never from a peer, so widening the window to bootHeight
// cannot be abused by a malicious peer to sneak a runaway fork past the cap.
// Caller must hold dag.mu (read or write).
func (dag *BlockDAG) farAheadFrontierLocked() int64 {
	if dag.bootHeight > dag.height {
		return dag.bootHeight
	}
	return dag.height
}

// farAheadFrontier is the lock-taking wrapper for callers that do not already
// hold dag.mu (e.g. queueOrphan, which AddPeerBlock calls after unlocking).
func (dag *BlockDAG) farAheadFrontier() int64 {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.farAheadFrontierLocked()
}

// proposerBreakerFailThreshold is how many consecutive non-attaching blocks a
// single proposer may deliver before its circuit breaker trips. Normal
// operation orphans at most a block or two transiently (a gossip block that
// briefly outran its parent), so a run this long unambiguously means the
// proposer is on a diverged fork whose blocks this node can never place.
//
// FIX (2026-07-04 — real root cause of Contabo nodes re-diverging at a
// faster BLOCK_TIME even after the DB-index and sibling-seeding fixes):
// this was a bare 40, tuned when every comment in this file explicitly
// assumed a 2s BLOCK_TIME (see proposerBreakerOrphanGrace's own comment).
// A single proposer produces roughly one block per BLOCK_TIME, so the
// WALL-CLOCK time to accumulate 40 failures shrinks in direct proportion
// to BLOCK_TIME — confirmed live: at BLOCK_TIME=1.5s a proposer's breaker
// tripped in well under a minute during perfectly ordinary propagation
// jitter, then a fixed 30s cooldown let the gap grow even further before
// the bounded reopen got a chance, and each subsequent trip compounded
// the same way. TuneProposerBreakerForBlockTime rescales this at startup
// so the wall-clock exposure window this constant was proven safe at
// (2s * 40 ≈ 80s) stays roughly constant regardless of cadence — a var,
// not a const, specifically so it can be tuned; defaults to the original
// 40 for every test and any code path that never calls the tuner.
var proposerBreakerFailThreshold = 40

// proposerBreakerTuningBaselineSeconds is the BLOCK_TIME (in seconds) every
// breaker constant in this file was originally tuned against — see
// TuneProposerBreakerForBlockTime.
const proposerBreakerTuningBaselineSeconds = 2.0

// configuredBlockTimeSeconds is the real, currently-configured BLOCK_TIME —
// set once at startup by TuneProposerBreakerForBlockTime (piggybacked there
// rather than a second main.go call site, since that function is already
// "call once at startup with the real BLOCK_TIME"). Defaults to the
// original 2s baseline for any test or code path that never calls it.
//
// FIX (2026-07-05 — website audit finding): api.go's /api/status used to
// hardcode "block_time": 1 as a bare literal — correct only by coincidence,
// because someone happened to manually update it to match BLOCK_TIME's
// value at the time. BLOCK_TIME changed 4 separate times the same night;
// every one of those changes needed this literal hand-edited too, and nothing
// enforced that the two ever stayed in sync. ConfiguredBlockTimeSeconds()
// is read directly by the status handler instead, so it can never drift
// from the actual constant again.
var configuredBlockTimeSeconds = proposerBreakerTuningBaselineSeconds

// ConfiguredBlockTimeSeconds returns the real, currently-configured
// BLOCK_TIME in seconds — see configuredBlockTimeSeconds' own comment.
func ConfiguredBlockTimeSeconds() float64 {
	return configuredBlockTimeSeconds
}

// proposerBreakerExtraSafetyFactor further widens both the fail threshold
// and the orphan grace beyond the "keep the original 2s-baseline wall-clock
// exposure window constant" calculation below.
//
// FIX (2026-07-05 — the first-pass scaling, matching the exact original 2s
// trip time, still wasn't enough): confirmed live, Contabo 2 kept
// repeatedly tripping its breaker against BOTH other validators at
// BLOCK_TIME=1s even with proposerBreakerFailThreshold doubled to 80. The
// original 2s-baseline tuning was itself just one operator's best guess,
// not a hard physical limit — real cross-provider propagation+processing
// delay has enough variance that matching the old exposure window exactly
// isn't a comfortable margin, only a bare one. This extra factor buys
// genuine headroom rather than re-deriving the same tight fit.
const proposerBreakerExtraSafetyFactor = 3.0

// TuneProposerBreakerForBlockTime rescales proposerBreakerFailThreshold and
// proposerBreakerOrphanGrace for the actual configured BLOCK_TIME. The
// threshold scales so the wall-clock time to trip stays close to (and then
// safety-multiplied beyond) what was proven stable at the original
// 2s-BLOCK_TIME tuning, instead of silently shrinking as BLOCK_TIME drops.
// The grace period widens too: real cross-provider propagation+processing
// delay is a roughly FIXED wall-clock quantity independent of BLOCK_TIME,
// so it does not shrink just because blocks are produced faster — a faster
// cadence needs MORE grace in block-count terms to cover the same fixed
// real-world delay, not the same or less. Neither value is ever lowered
// below its original tuning — a slower-than-baseline BLOCK_TIME keeps the
// already-proven values rather than becoming MORE trigger-happy. Call once
// at startup, before any sync/production goroutines start (main.go, right
// after BLOCK_TIME is known).
func TuneProposerBreakerForBlockTime(blockTime time.Duration) {
	secs := blockTime.Seconds()
	if secs <= 0 {
		return
	}
	configuredBlockTimeSeconds = secs
	speedupFactor := proposerBreakerTuningBaselineSeconds / secs
	// The extra safety margin only applies once we're actually running
	// FASTER than the baseline this was tuned against — at or below the 2s
	// baseline, this stays a no-op exactly like before, matching the
	// already-proven behavior there.
	extra := 1.0
	if speedupFactor > 1 {
		extra = proposerBreakerExtraSafetyFactor
	}
	scaledThreshold := int(speedupFactor * 40 * extra)
	if scaledThreshold > proposerBreakerFailThreshold {
		proposerBreakerFailThreshold = scaledThreshold
	}
	scaledGrace := time.Duration(float64(8*time.Second) * speedupFactor * extra)
	if scaledGrace > proposerBreakerOrphanGrace {
		proposerBreakerOrphanGrace = scaledGrace
	}
}

// proposerBreakerCooldown is how long a tripped proposer's blocks are dropped
// at the lock-free top of AddPeerBlock before probe blocks are let through to
// re-test whether it has stopped diverging.
const proposerBreakerCooldown = 30 * time.Second

// proposerBreakerReopenProbes is how many blocks get a chance to attach after
// cooldown before the breaker re-trips, if none of them succeed.
//
// FIX (durable fix, 2026-07-04 — real fix for a repeated live outage): a
// single probe (the previous value) means ANY one unlucky failure — a
// transient gossip-ordering blip, a probe block that itself references a
// still-orphaned recent parent, nothing to do with the proposer actually
// being diverged — re-arms the FULL 30s cooldown before another attempt.
// Confirmed live: two nodes each running their own per-proposer breaker
// against each other (and a third) got stuck in this exact trap for
// several consecutive minutes, well past any reasonable transient-glitch
// window, each cooldown's single probe happening to fail before the next
// could even be tried. recordProposerOutcome's success branch already
// clears the ENTIRE run on the very first attach, so this doesn't weaken
// protection against a genuinely diverged proposer — that case still fails
// every one of these probes and re-trips within a handful of blocks
// (worst case ~5s of extra exposure at this cadence), not the original
// unbounded 40-strike run. It just stops one bad-luck probe from costing
// another full 30s for an otherwise-honest, already-reconnected peer.
const proposerBreakerReopenProbes = 5

// maxTrackedProposers caps proposerFailRun (see recordProposerOutcome's own
// FIX comment) — matches warnedUnknownProposers' cap, the established
// precedent for bounding a map keyed by an unauthenticated proposer string.
const maxTrackedProposers = 500

// syncStallTimeout (seconds) is how long ProduceBlock's initial-sync gate
// tolerates no further sync progress (see dag.lastSuccessfulPeerSyncAt)
// before concluding the seed is unreachable and producing independently.
// Measured from the last successful peer-block acceptance, not from
// process startup — see the gate's own comment for why that distinction
// matters for a large historical catch-up (e.g. after RESYNC_FROM_SNAPSHOT).
const syncStallTimeout = 90

// proposerBlockBlocked reports whether a proposer's blocks should be dropped now
// WITHOUT taking dag.mu, because its breaker is open (still inside the cooldown).
// Called on AddPeerBlock's lock-free hot path; touches only proposerBreakerMu.
func (dag *BlockDAG) proposerBlockBlocked(proposer string) bool {
	if proposer == "" {
		return false
	}
	proposer = strings.ToLower(proposer)
	dag.proposerBreakerMu.Lock()
	defer dag.proposerBreakerMu.Unlock()
	until, ok := dag.proposerBreakerUntil[proposer]
	if !ok {
		return false
	}
	if time.Now().UnixNano() >= until {
		delete(dag.proposerBreakerUntil, proposer)
		// Bounded reopen, not a full reopen (P0 fix, 2026-07-02 liveness
		// audit; widened from a single probe to proposerBreakerReopenProbes
		// on 2026-07-04, see that constant's comment): seed the run
		// proposerBreakerReopenProbes short of the threshold so up to that
		// many outcomes get a real chance — the first attach fully clears it
		// (recordProposerOutcome's success branch), while proposerBreakerReopenProbes
		// consecutive failures re-trips — not another full run of
		// proposerBreakerFailThreshold fresh failures, each at full
		// processing cost, before it closes again. Without this, the comment
		// below was aspirational: deleting the run outright left the gate
		// fully open for every call until 40 fresh failures rebuilt from
		// zero — against a peer that pushes at high volume, that reopening
		// happened every single cooldown cycle.
		if dag.proposerFailRun == nil {
			dag.proposerFailRun = make(map[string]int)
		}
		dag.proposerFailRun[proposer] = proposerBreakerFailThreshold - proposerBreakerReopenProbes
		return false
	}
	return true
}

// recordProposerOutcome feeds the per-proposer circuit breaker. attached=true
// (the block joined the DAG) clears the proposer's failure run; attached=false
// (it was rejected far-ahead or orphaned on a missing parent) advances the run
// and trips the breaker once it crosses proposerBreakerFailThreshold. Uses
// proposerBreakerMu only — never dag.mu — and is always called with dag.mu
// released, so it can never invert the lock order against block production.
func (dag *BlockDAG) recordProposerOutcome(proposer string, attached bool) {
	if proposer == "" {
		return
	}
	proposer = strings.ToLower(proposer)
	dag.proposerBreakerMu.Lock()
	defer dag.proposerBreakerMu.Unlock()
	if attached {
		delete(dag.proposerFailRun, proposer)
		delete(dag.proposerBreakerUntil, proposer)
		return
	}
	if dag.proposerFailRun == nil {
		dag.proposerFailRun = make(map[string]int)
	}
	// FIX (P2-c, audit 2026-07-06): this map has no cap, unlike every other
	// per-key bookkeeping map in BlockDAG (replayedBlocks at 50,000,
	// warnedUnknownProposers at 500, orphans at maxOrphans) — and unlike
	// those, the key here (block.Proposer) is read from an unauthenticated
	// block BEFORE signature verification (this call site is reached from
	// the far-ahead gate and the missing-parent gate, both ahead of the
	// ECDSA check later in AddPeerBlock — see this function's own callers).
	// An attacker can trivially generate an unlimited number of distinct
	// "proposer" strings, each a new map entry, for a real memory-
	// exhaustion DoS. Unlike warnedUnknownProposers (a pure log-noise
	// suppressor, safe to wipe wholesale), this map holds live circuit-
	// breaker state — wiping it at cap would let an attacker who has
	// already tripped their OWN entry clear it early just by flooding new
	// proposer strings afterward. Instead, once at cap, stop admitting
	// BRAND NEW proposer keys (existing ones still update/trip/cool down
	// normally) — bounds memory without handing out a breaker-reset lever.
	if _, tracked := dag.proposerFailRun[proposer]; !tracked && len(dag.proposerFailRun) >= maxTrackedProposers {
		return
	}
	dag.proposerFailRun[proposer]++
	if dag.proposerFailRun[proposer] >= proposerBreakerFailThreshold {
		if dag.proposerBreakerUntil == nil {
			dag.proposerBreakerUntil = make(map[string]int64)
		}
		dag.proposerBreakerUntil[proposer] = time.Now().Add(proposerBreakerCooldown).UnixNano()
		delete(dag.proposerFailRun, proposer) // breaker gates now; rebuild the run after cooldown
	}
}

// ClearProposerCircuitBreakers resets all per-proposer circuit-breaker state.
// Called after a successful ResyncFromSnapshotURL (see main.go's bootstrap/
// resync sequence).
//
// FIX (P0, merge-reliability audit 2026-07-03 — permanent fix, second half):
// the FromSync exemption added to AddPeerBlock's breaker gate only covers
// blocks this node actively pulls via its own catch-up sync — it does
// nothing for blocks arriving via HTTP push or P2P gossip, which is how
// most of a peer's LIVE (non-catch-up) blocks actually arrive. Confirmed
// live: even after the FromSync fix, a resynced Contabo kept dropping
// Primary's freshly-produced blocks ("Circuit breaker open for 0xAA08...")
// because those arrive via push/gossip, not via Contabo's own sync pull —
// the breaker state left over from BEFORE the resync (when the two chains
// genuinely didn't share history) persisted after it, even though a
// successful resync cryptographically re-establishes exactly the shared
// trust the breaker's trip was originally warning about the lack of. Every
// proposer's fail-run/cooldown is stale the moment local history has just
// been authoritatively replaced wholesale; keeping it around only recreates
// the same deadlock (breaker blocks attachment -> attachment never happens
// -> breaker never clears) that made the resync necessary in the first
// place. A resync is infrequent and already a heavyweight operation, so
// resetting every proposer's state (not just the resync signer's) is cheap
// and correct: none of the old counts mean anything against the new history.
func (dag *BlockDAG) ClearProposerCircuitBreakers() {
	dag.proposerBreakerMu.Lock()
	defer dag.proposerBreakerMu.Unlock()
	n := len(dag.proposerBreakerUntil)
	dag.proposerFailRun = nil
	dag.proposerBreakerUntil = nil
	if n > 0 {
		fmt.Printf("[AUTO-HEAL] Cleared circuit-breaker state for %d proposer(s) after resync — stale counts from before the resync no longer apply to the new history.\n", n)
	}
}

func (dag *BlockDAG) AddPeerBlock(block *Block) bool {
// FIX (2026-07-05 — permanent operational diagnostic, not a temp one):
// recordForeignAttachLatency further down only fires once a block clears
// EVERY gate (circuit breaker, far-ahead cap, replay, etc.) — exactly the
// blocks that are NOT the problem. A node whose circuit breaker is
// currently open drops every foreign block right here, before that later
// measurement point is ever reached, so it silently produces zero latency
// samples for precisely the direction that's actually failing. Measure
// the RAW arrival latency unconditionally, before any gate, so the real
// network-transit number is visible even while a node is fully isolated.
if block != nil && block.ProducedAtMs > 0 && !strings.EqualFold(block.Proposer, dag.selfProposer) {
	if latency := time.Now().UnixMilli() - block.ProducedAtMs; latency >= 0 {
		dag.recordRawArrivalLatency(latency)
	}
}
if dag.resyncInProgress.Load() {
	return false // an in-process self-heal resync is atomically swapping account/DAG state right now — see resyncInProgress's field comment; the sender will redeliver, ordered sync fills the gap once the resync completes
}
// FIX (durable fix, 2026-07-04 — closes wasted-effort noise found live after
// a fresh checkpoint-seeded resync): a block at or below BootHeight is
// already fully accounted for by the snapshot/checkpoint this node just
// seeded from (see BootHeight's own comment) — accepting or even attempting
// to resolve it is pure waste, and confirmed live to actively hurt: gossip/
// relay of a proposer's own PRE-resync blocks (still circulating from
// before this node's chain_blocks was wiped) kept arriving after a fresh
// resync, each one genuinely missing its own equally-stale parent (also
// wiped), competing for the exact same orphan-resolution machinery that
// SHOULD be spending all its effort catching up to the CURRENT tip instead.
// Reported as accepted (true): this data is already correctly reflected in
// this node's state via the checkpoint, so there is nothing wrong to signal
// to the sender, and it correctly clears any breaker/orphan bookkeeping for
// that proposer instead of counting genuinely irrelevant old data against it.
//
// FIX (P0, 2026-07-04 — permanent-isolation-after-plain-restart incident):
// gated by BootHeightCheckpointBacked(). This skip is only sound when a
// checkpoint-seeded resync actually stored a real block at exactly
// BootHeight — SOMETHING later blocks can find as a parent. Confirmed live:
// after a PLAIN restart (no resync), bootHeight gets ratcheted up to a bare
// persisted max_block_height NUMBER (continuously bumped by this node's own
// ongoing production) with no matching entry in dag.blocks/dag.tips. Every
// real historical block up to that height got waved through here without
// ever being stored, so every later block referencing one of them as a
// parent orphaned permanently — the exact isolation loop found live on both
// Contabo nodes tonight. Without checkpoint backing, fall through to the
// normal path below instead, which safely resolves a genuinely-old-but-
// still-known parent via ghostdagBlockLookup's own DB fallback.
//
// FIX (P0, 2026-07-04 — third layer of the same incident, found live even
// WITH checkpoint backing correct): also excludes SelfFetched blocks now.
// fetchMissingAncestors deliberately, individually fetches ONE specific
// missing-parent hash because some already-orphaned child genuinely needs
// it — that is precisely why it's SelfFetched. Confirmed live: it kept
// resolving hashes that happened to land at or below BootHeight (a second
// isolated peer's own historical chain, walked backward one hash at a
// time), got the free pass here, and was reported "accepted" WITHOUT ever
// being stored — so the orphaned child waiting on that exact hash could
// never actually resolve, no matter how many times the fetch "succeeded".
// The original bug this whole skip exists for is passively-arriving STALE
// GOSSIP of a proposer's pre-resync blocks that nothing will ever need as a
// parent — never a deliberate, targeted fetch for a hash something is
// actively waiting on right now. Skipping storage for a SelfFetched block
// defeats the entire point of targeted ancestor resolution.
//
// FIX (2026-07-05 — fourth layer of the same incident, found live even
// with the SelfFetched exemption correct): "SelfFetched" only proves the
// fetch was deliberately, individually issued for a hash something needed
// AT THE TIME the request went out — it says nothing about whether that
// need still exists when the response finally arrives. Confirmed live,
// repeating on every single resync: fetchMissingAncestors/doSyncOnce
// requests already in flight when a resync clears dag.orphans land
// moments later still marked SelfFetched, and this exemption force-fed
// them through as if still needed — scattered ancient-height blocks (seen
// live: heights tens of thousands below a freshly-seeded checkpoint)
// re-entering AddPeerBlock, queueing as brand-new orphans nothing is
// waiting on anymore, burning real-time catch-up attention and proposer-
// breaker budget on work that became moot the instant the resync ran.
// hasAwaitingOrphan re-derives whether this exact hash is STILL the
// missing-parent key for at least one currently-queued orphan — the
// live, present-tense version of the "something genuinely needs it" claim
// SelfFetched alone can only make in the past tense.
if block != nil && block.Height > 0 && block.Height <= dag.BootHeight() && dag.BootHeightCheckpointBacked() && (!block.SelfFetched || !dag.hasAwaitingOrphan(block.Hash)) {
	return true
}
// Lock-free fork-flood shield (P0, 2026-07-02): reject a block whose height is
// far above ours BEFORE taking the write lock, so a diverged/runaway fork
// pushing thousands of unresolvable blocks can NEVER contend for dag.mu with
// block production — the difference between a responsive node and the hang this
// whole incident was. A brief RLock reads the height (concurrent with other
// readers, only momentarily blocks the writer); the queueOrphan check remains
// as a second line of defence. Ordered sync still fills any legitimate gap
// parents-first, so nothing real is lost. Logging is rate-limited to once per
// second so the flood can't make the log the new bottleneck.
if block != nil {
	// Per-proposer circuit breaker (see recordProposerOutcome). A proposer whose
	// recent blocks all failed to attach — a diverged fork spamming blocks this
	// node can never place — is dropped here, before the RLock and all dag.mu /
	// hash / ECDSA work, until its cooldown expires. This is the path-independent
	// counterpart to the per-IP push shield: it catches the flood no matter which
	// ingress delivered it. Log rate-limited to once/sec.
	// FIX (P0, merge-reliability audit 2026-07-03 — permanent fix for the
	// recurring Contabo/Primary resync deadlock): skip this gate for FromSync
	// blocks (fetched via HTTP-sync from an operator-configured trusted seed
	// — see isTrustedSyncSource/trustedSeeds). This gate runs before
	// FromSync's OTHER exemptions (the two "EXCEPT while WE are still in
	// initial catch-up" cases further below, added for the exact same
	// reason) ever get a chance to apply, so a trusted seed's catch-up
	// blocks were still being dropped here even while this node was
	// actively, legitimately resyncing from it. Confirmed live: after a
	// fresh RESYNC_FROM_SNAPSHOT, Contabo's sync-derived blocks
	// (FromSync=true, fetched straight from Primary) kept getting dropped by
	// Primary's own breaker against Contabo's address (tripped during the
	// PRE-resync divergence) — a permanent deadlock between "can't clear the
	// breaker without an attaching block" and "can't attach a block while
	// the breaker blocks even trusted-seed sync data". A block reaching
	// this function with FromSync=true was fetched specifically because a
	// configured seed vouches for it; the breaker's "untrusted, possibly
	// malicious flood" heuristic does not apply to it.
	//
	// SelfFetched (see its own field comment): the same exemption, but for a
	// block THIS node deliberately fetched via its own catch-up sync from
	// ANY already-authorized validator — not just a statically-configured
	// trusted seed. Fixes the identical deadlock between two SECONDARY
	// validators (neither is the other's trusted seed) without requiring
	// every node's PEER_NODES to be hand-updated whenever a new validator
	// joins.
	if !block.FromSync && !block.SelfFetched && dag.proposerBlockBlocked(block.Proposer) {
		nowNano := time.Now().UnixNano()
		last := dag.lastProposerBreakerLogAt.Load()
		if nowNano-last > int64(time.Second) && dag.lastProposerBreakerLogAt.CompareAndSwap(last, nowNano) {
			fmt.Printf("[DAG] ✗ Circuit breaker open for %s — dropping its blocks (e.g. #%d) lock-free after a sustained run of non-attaching blocks. (rate-limited)\n",
				block.Proposer, block.Height)
		}
		return false
	}
	dag.mu.RLock()
	localHeight := dag.height
	frontier := dag.farAheadFrontierLocked()
	catchingUp := dag.isCatchingUpLocked()
	dag.mu.RUnlock()
	if block.Height > frontier+maxOrphanHeightGap {
		// A far-ahead block never attaches here — feed the breaker so a sustained
		// runaway-fork flood trips it and is then dropped above without even this
		// RLock. EXCEPT while WE are still in initial catch-up (bootHeight far
		// above our height): every real validator's live blocks look "far-ahead"
		// purely because we haven't synced yet — that's not evidence the proposer
		// is bad. Feeding it to the breaker then would trip it against an honest
		// proposer (confirmed live on Contabo: tripped against Primary itself
		// during a fresh resync, permanently blocking the one peer it needed to
		// sync from). Still reject the block either way — only the breaker
		// bookkeeping is skipped.
		if !catchingUp {
			dag.recordProposerOutcome(block.Proposer, false)
		}
		nowNano := time.Now().UnixNano()
		last := dag.lastFarAheadLogAt.Load()
		if nowNano-last > int64(time.Second) && dag.lastFarAheadLogAt.CompareAndSwap(last, nowNano) {
			fmt.Printf("[DAG] ✗ Rejecting far-ahead blocks (e.g. #%d from %s, %d above frontier %d [local height %d], cap %d) — diverged fork; ordered sync pulls any real gap parents-first. (rate-limited)\n",
				block.Height, block.Proposer, block.Height-frontier, frontier, localHeight, maxOrphanHeightGap)
		}
		return false
	}
}
dag.mu.Lock()
// NOTE: no defer — we manually unlock before the channel send below (Fix 2).
// All early-return paths must call dag.mu.Unlock() explicitly.

// Skip if already known — UNLESS the existing entry is a
// synthetic-checkpoint stub (see BridgeHistoricalGap/queueOrphan's
// runtime-bridge branch). A stub exists only to satisfy this same
// parent-existence check for blocks built on top of it; it carries no
// transactions and is never replayed, so trusting it is a permanent,
// silent loss of whatever that block actually contained (a
// registration, a transfer, anything) — UNLESS this node later
// receives the real block with that exact hash, in which case it
// should heal the gap with real, verified, replayed data instead of
// staying stuck with the placeholder forever. A hash collision between
// a stub and an unrelated real block is not a concern: calculateHash
// covers the block's full content, so only the genuinely-corresponding
// real block can ever match a given stub's hash. The rest of this
// function (signature check, parent-existence, replay) still runs
// normally for the incoming block — this only removes the short-circuit
// that prevented it from ever being attempted.
if existing, exists := dag.blocks[block.Hash]; exists && existing.Proposer != "synthetic-checkpoint" {
dag.mu.Unlock()
return false
} else if exists {
fmt.Printf("[BLOCK] ⚕ Healing synthetic-checkpoint stub at height %d (%s...) with real block from peer\n", existing.Height, block.Hash[:min(16, len(block.Hash))])
}

// FIX (audit 2026-06-30 monster audit, P1-04): degraded means a prior
// persistence failure left this node's in-memory DAG ahead of what's
// durably saved (see ProduceBlock's own degraded gate above and
// degradedReason's struct comment). ProduceBlock already refuses to make
// the gap worse by minting new blocks on top of unconfirmed state — but
// AddPeerBlock had no equivalent gate, so a degraded node kept accepting
// and replaying peer blocks (mutating cs.accounts, advancing dag.height)
// on top of a DAG it already couldn't durably reconstruct. That widens
// the repair surface every block instead of freezing it. Reject outright;
// the peer's sync loop retries later once an operator restarts/resyncs
// this node and degradedReason clears.
dag.degradedMu.Lock()
dr := dag.degradedReason
dag.degradedMu.Unlock()
if dr != "" {
fmt.Printf("[DAG] ✗ Rejected peer block #%d: node is degraded (%s) — restart to recover\n", block.Height, dr)
dag.mu.Unlock()
return false
}

// FIX (audit 2026-06-30 monster audit, P1-03): see
// ghostdagMigrationPending's struct comment and ProduceBlock's matching
// gate — see the matching removal in ProduceBlock's own comment for the
// full reasoning (P0, 2026-07-04): blocking ALL peer-block acceptance for
// the full duration of a migration turned routine restarts into extended
// total outages, confirmed live and repeatedly, a strictly worse failure
// mode than the bounded, self-correcting scoring imprecision this gate
// was guarding against. Migration still runs and backfills scores in the
// background; it just no longer blocks acceptance while doing so.

// Genesis blocks are always created locally — never accept from peers.
// A peer could send any block with IsGenesis=true and it would bypass
// both the signature check and the parent check below.
if block.IsGenesis {
fmt.Printf("[DAG] ✗ Rejected peer genesis: genesis can only be created locally\n")
dag.mu.Unlock()
return false
}

// Integrity check 1: recompute hash from block fields.
expectedHash := dag.calculateHash(block)
if expectedHash != block.Hash {
fmt.Printf("[DAG] ✗ Rejected peer block #%d: hash mismatch (claimed %s..., computed %s...)\n",
block.Height, block.Hash[:min(16, len(block.Hash))], expectedHash[:16])
dag.mu.Unlock()
return false
}

// Integrity check 2: all non-genesis blocks must carry a valid ECDSA
// signature from the proposer. Unsigned blocks are rejected — this is the
// primary consensus enforcement mechanism.
if !block.IsGenesis && block.Signature == "" {
	fmt.Printf("[DAG] ✗ Rejected peer block #%d from %s: missing signature\n",
		block.Height, block.Proposer)
	dag.mu.Unlock()
	return false
}
proposer := strings.ToLower(block.Proposer)
if block.Signature != "" && !block.IsGenesis {
	sigBytes, sigErr := hex.DecodeString(block.Signature)
	if sigErr != nil || len(sigBytes) != 65 {
		fmt.Printf("[DAG] ✗ Rejected peer block #%d: malformed signature\n", block.Height)
		dag.mu.Unlock()
		return false
	}
	hashBytes := common.HexToHash(block.Hash)
	pubkeyBytes, recErr := crypto.Ecrecover(hashBytes[:], sigBytes)
	if recErr != nil {
		fmt.Printf("[DAG] ✗ Rejected peer block #%d: signature recovery failed: %v\n", block.Height, recErr)
		dag.mu.Unlock()
		return false
	}
	pubkey, parseErr := crypto.UnmarshalPubkey(pubkeyBytes)
	if parseErr != nil {
		fmt.Printf("[DAG] ✗ Rejected peer block #%d: invalid public key: %v\n", block.Height, parseErr)
		dag.mu.Unlock()
		return false
	}
	recoveredAddr := strings.ToLower(crypto.PubkeyToAddress(*pubkey).Hex())
	// Proposer must be the Ethereum address that produced the signature.
	// Blocks where the proposer field does not match the recovered signing
	// address are unconditionally rejected — no libp2p-nodeID exemption.
	if recoveredAddr != proposer {
		fmt.Printf("[DAG] ✗ Rejected peer block #%d: signature mismatch (signer %s, proposer %s)\n",
			block.Height, recoveredAddr, proposer)
		dag.mu.Unlock()
		return false
	}
	// Proposer must be in the authorized validator set. Without this check
	// anyone can generate an Ethereum key, sign a block, and feed it in.
	// Skipped for HTTP-SYNC blocks from a trusted seed (block.FromSync —
	// see its field doc): a configured seed's canonical history is trusted
	// by construction; abandoning orphans here would permanently deadlock
	// any child block waiting on a historical block from an early validator
	// whose registration was cleared from the local DB. Blocks synced from
	// a non-seed peer get FromSync=false and are still checked normally.
	if !dag.authorizedValidators[proposer] && !block.FromSync {
		// P3-2: cap to prevent unbounded memory growth from forged proposer addresses
		if len(dag.warnedUnknownProposers) > 500 {
			dag.warnedUnknownProposers = make(map[string]bool)
		}
		firstSeen := !dag.warnedUnknownProposers[proposer]
		if firstSeen {
			dag.warnedUnknownProposers[proposer] = true
			fmt.Printf("[DAG] Unknown proposer %s — fetching validator lists from all peers (block will be accepted if peer registration succeeds)\n", proposer)
		}
		hash := block.Hash
		dag.mu.Unlock()
		// Prune the orphan queue for this hash immediately: any block waiting
		// on this rejected block as a parent can never be resolved either, so
		// keeping them only burns through orphan-buffer space and delays the
		// "gap closed" signal.  Without this, fetchMissingAncestors never calls
		// RecordOrphanAttempt for a "peer has it but we reject it" block, so the
		// TTL prune in queueOrphan never fires — orphans hang for the full 15 min.
		dag.abandonOrphansWaitingFor(hash)
		if firstSeen {
			// Pull the full validator list from every active sync peer right now.
			// If this proposer registered with any of them (but not with us),
			// AddAuthorizedValidator will add them and the next sync cycle will
			// accept their blocks — no manual AUTHORIZED_VALIDATORS config needed.
			SafeGoroutine("syncValidatorsFromAllPeers", dag.syncValidatorsFromAllPeers)
		}
		return false
	}
}

// GATE ORDER (P0, cadence 2026-07-03 night): the finality gate below runs
// BEFORE the equivocation suspension gate. Both reject independently of
// each other, so ordering cannot change WHICH blocks get through — but it
// changes what a rejection COSTS. isFinalityViolation is pure in-memory
// (cached checkpoint), while IsValidatorSuspended is a Postgres round trip
// (~0.5s over the primary's remote DB proxy) executed while dag.mu is held
// write-locked. Confirmed live: isolated/diverged peers re-deliver whole
// pages of far-below-checkpoint blocks continuously (heights #80-129 vs a
// ~140k checkpoint, dozens of [FINALITY] rejects/minute) — with the
// suspension gate first, EVERY one of those doomed blocks held dag.mu
// through a full DB round trip before the free check rejected it anyway,
// starving ProduceBlock's lock acquisition and inflating cadence from the
// 1s target to 3-5s (rising with flood volume).

// Finality gate: reject blocks so far below the finalized checkpoint that
// they could only matter for a deep reorg — which the hard finality
// guarantee forbids. Legitimate gap-fills within finalityHeightSlack of
// the checkpoint are still accepted.
if dag.isFinalityViolation(block) {
	fH, _ := dag.state.GetFinalizedCheckpoint()
	nowNano := time.Now().UnixNano()
	last := dag.lastFinalityRejectLogAt.Load()
	if nowNano-last > int64(time.Second) && dag.lastFinalityRejectLogAt.CompareAndSwap(last, nowNano) {
		fmt.Printf("[FINALITY] ✗ Rejected block #%d: below finalized checkpoint %d (slack %d) (rate-limited)\n",
			block.Height, fH, finalityHeightSlack)
	}
	dag.mu.Unlock()
	return false
}

// Equivocation suspension gate: a validator suspended or permanently banned
// for repeated equivocation may not produce further blocks until the penalty
// expires. Checked after the signature + authorization gates above so that
// the suspended proposer's identity is already confirmed cryptographically,
// and after the free finality gate so a flood of doomed below-checkpoint
// blocks can't turn this check's cost into a dag.mu bottleneck (see the
// GATE ORDER comment above — the check itself is now an in-memory cache
// read, the ordering is defense in depth).
//
// Skipped for blocks fetched via HTTP-SYNC from a trusted seed
// (block.FromSync == true — see its field doc): those blocks are part of a
// configured seed's canonical chain and were accepted before the local
// suspension record existed. Rejecting them here deadlocks catch-up sync
// whenever a historically-banned validator's blocks appear in the canonical
// history. Blocks synced from a non-seed peer get FromSync=false and are
// still checked against the current suspension record.
//
// FIX (P2-2, beta-launch audit 2026-07-05): also skipped for
// block.SelfFetched, mirroring the circuit-breaker gate above (see its own
// !block.FromSync && !block.SelfFetched condition) — this suspension check
// previously exempted FromSync only, leaving a gap: a SelfFetched block
// from a validator suspended AFTER that block was originally produced
// (suspension is keyed by the block's own timestamp, but the SUSPENSION
// RECORD is whatever the node currently holds, which can postdate an old
// block being re-fetched) could still be rejected here even though the
// circuit-breaker already decided this exact ancestor should be trusted
// enough to re-fetch — silently reintroducing the same class of merge-stall
// SelfFetched was added to close, just one gate later.
if dag.state != nil && !block.FromSync && !block.SelfFetched {
	if suspended, reason := dag.state.IsValidatorSuspended(proposer, block.Timestamp); suspended {
		fmt.Printf("[SLASHING] ✗ Rejected block #%d from %s: %s\n", block.Height, proposer, reason)
		dag.mu.Unlock()
		return false
	}
}

// Integrity check 3: parent-existence and height validation.
//
// FIX (orphan buffer): this used to tolerate a missing parent only while
// len(dag.blocks) <= 10 — i.e. only during the first ~minute of a fresh
// node's life, since every node produces its own block every 6s regardless
// of sync status. Past that point, ANY block whose parent wasn't already
// in dag.blocks was silently dropped with NO log line (this branch had no
// fmt.Printf, unlike every other reject path in this function) and NEVER
// retried — and everything built on top of that block inherited the same
// fate, since ITS parent (the dropped block) would also never exist
// locally. Confirmed in production with 3 concurrent validators: every
// node's own /api/blocks ended up showing ONLY its own single-parent
// chain, never the other validators' blocks, because somewhere in their
// ancestry a single missing parent (a brief P2P gap, a sync page that
// didn't cover it, anything transient) permanently blocked the entire
// subtree above it — with no error anywhere to even reveal why.
//
// Now: a block with a missing parent is queued in dag.orphans, keyed by
// the missing hash, instead of being dropped. When that parent later
// arrives (via AddPeerBlock, below), every block waiting on it is
// automatically retried — and if THAT retry succeeds, its own dependents
// get retried too, recursively. A transient gap now costs one retry
// instead of permanently orphaning an entire branch.
if len(block.ParentHashes) == 0 {
fmt.Printf("[DAG] ✗ Rejected peer block #%d: no parent hashes\n", block.Height)
dag.mu.Unlock()
return false
}
if block.Height > 1 {
maxParentHeight := int64(-1)
missingParent := ""
// FIX (durable fix, 2026-07-03 — the actual deepest root cause behind
// tonight's whole "never merges" saga): this used to read dag.blocks[ph]
// directly, the same in-memory-only pattern already fixed for GHOSTDAG
// scoring (see ghostdagBlockLookup's comment) — but THIS is the gate
// that decides whether an incoming block attaches AT ALL, so a miss here
// is far more consequential than a miss in blue-score computation. Any
// restart only loads the most recent startupLoadWindow (2000) blocks
// into dag.blocks, regardless of how much further back a peer's parent
// reference points (routine after any catch-up/merge). Confirmed live:
// after a plain restart with NO resync, a node that had JUST reached
// parity with its peer (matching hashes at recent heights) immediately
// started orphaning peer blocks 60,000+ heights below its own tip —
// blocks that were fully present and valid in its own local DB the
// entire time, just outside the freshly-reloaded in-memory window.
// ghostdagBlockLookup already implements exactly the DB-fallback (plus
// re-caching into dag.blocks) this needs; reusing it here instead of a
// second bespoke lookup.
for _, ph := range block.ParentHashes {
parent := dag.ghostdagBlockLookup(ph, nil)
if parent == nil {
	missingParent = ph
	break
}
if parent.Height > maxParentHeight {
maxParentHeight = parent.Height
}
}
if missingParent != "" {
	dag.mu.Unlock()
	dag.queueOrphan(missingParent, block)
	// Feed the circuit breaker: a block that orphans on a missing parent did not
	// attach. A proposer on a diverged fork does this every block (its fork-parents
	// live only on its own chain), so proposerBreakerFailThreshold consecutive such
	// blocks trip the lock-free drop at the top of AddPeerBlock.
	//
	// EXCEPT while WE are still in initial catch-up: a freshly-bootstrapping
	// node's dag.blocks holds only genesis (plus any bridge stub), so EVERY
	// honest validator's block orphans on a missing parent here — not because
	// the proposer diverged, but because we haven't synced in its ancestry
	// yet. Recording that as a failure would (and on Contabo, did) trip our
	// breaker against the honest primary, permanently blocking the one peer
	// we need to sync from. Still queue the orphan for later resolution —
	// only the breaker bookkeeping is skipped.
	//
	// EXCEPT ALSO while this specific gap is still within its grace period —
	// see proposerBreakerOrphanGrace's own comment: even a fully-synced,
	// perfectly healthy node sees an occasional short-lived orphan from
	// ordinary cross-network propagation timing, and counting that
	// immediately (before giving it any chance to resolve on its own) is
	// what let two healthy Contabo nodes trip their breakers against each
	// other during completely normal operation.
	//
	// EXCEPT ALSO for SelfFetched/FromSync blocks (audit 2026-07-06): this
	// used to be the one gap left in the SelfFetched exemption described
	// above (2924-2930) and at line 2931's own gate — that gate only stops
	// an ALREADY-tripped breaker from blocking a self-fetched delivery, it
	// never stopped a self-fetched delivery from CONTRIBUTING to tripping
	// the breaker in the first place. isCatchingUpLocked() only covers the
	// gap between dag.height and bootHeight/syncTargetHeight, both of which
	// a node reaches almost immediately after a snapshot resync (its own
	// production catches up within a handful of blocks) — so a node can
	// stop being "catching up" by this definition while it is still working
	// through a large, late-arriving backlog of a PEER's pre-resync history
	// via its own doSyncOnce (confirmed live 2026-07-06: two secondaries
	// resynced ~60s apart each relayed the other's now-pruned ancestry,
	// arriving 60-240s late — far past proposerBreakerOrphanGrace's 8s — and
	// tripped each other's breakers even though every one of those blocks
	// was fetched deliberately, not pushed by a misbehaving proposer). A
	// block we asked for ourselves during our own catch-up can never be
	// evidence the PROPOSER is misbehaving — any orphaning here is about our
	// own missing ancestry, exactly the case SelfFetched/FromSync exist to
	// name.
	dag.mu.RLock()
	catchingUp := dag.isCatchingUpLocked()
	dag.mu.RUnlock()
	if age, tracked := dag.orphanAge(missingParent); !block.SelfFetched && !block.FromSync && !catchingUp && (!tracked || age >= proposerBreakerOrphanGrace) {
		dag.recordProposerOutcome(block.Proposer, false)
	}
	return false
}
if maxParentHeight >= 0 && block.Height != maxParentHeight+1 {
fmt.Printf("[DAG] ✗ Rejected peer block #%d: invalid height (parent max %d)\n",
block.Height, maxParentHeight)
dag.mu.Unlock()
return false
}
}

// Integrity check 4: transaction type whitelist — unknown types could
// inject unrecognised state-change commands into the audit log.
for _, tx := range block.Transactions {
switch tx.Type {
case "", "register_human", "transfer", "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity", "remove_liquidity", "faucet", "ubi_distribution", "ubi_distribution_finalize",
	"validator_distribution", "validator_distribution_pool_zero", "lp_distribution", "lp_distribution_pool_zero", "escrow_move", "escrow_release", "escrow_recover",
	"slash_equivocation":
// known / empty — OK
default:
fmt.Printf("[DAG] ✗ Rejected peer block #%d: unknown tx type %q\n", block.Height, tx.Type)
dag.mu.Unlock()
return false
}
}

// FIX (durable fix, 2026-07-04 — closes the two-phase block-save gap that
// was this session's deepest recurring root cause): compute GHOSTDAG state
// HERE, still holding dag.mu from the integrity checks above, BEFORE the
// header is ever persisted — not after replay completes, several DB round
// trips and a lock release/reacquire later, as it used to run. This needs
// nothing this block doesn't already have available: computeGHOSTDAGState
// only reads block.ParentHashes and looks up ANCESTORS (already verified
// present by Integrity check 3 above) via ghostdagBlockLookup — it never
// needs this block itself to be in dag.blocks/dag.tips yet, and it has
// nothing to do with transaction replay. This unconditionally OVERWRITES
// SelectedParent/Blues/BlueScore with locally-computed values, which is
// what P1-03's old "strip peer-supplied GHOSTDAG fields before saving"
// step existed to guarantee too — computing correctly up front makes that
// separate strip step unnecessary, not just redundant.
//
// Why this matters: the OLD two-phase order (save header with fields
// zeroed → replay → THEN compute and save the real values) left every
// single peer block with a real window — spanning the full replay phase,
// not a few instructions — where a restart (a crash, or simply another
// deploy; this session's own redeploys are exactly what triggered it
// live, repeatedly) permanently strands that block in chain_blocks with
// SelectedParent="". Nothing after that ever revisits or repairs it later
// (recordProposerOutcome/normal restarts always take the fast "SelectedParent
// != ''" path in LoadBlocksFromDB's migration check) — it just silently
// counts toward needsMigration forever. Confirmed live: this is why the
// startup GHOSTDAG migration kept re-triggering for roughly the same
// ~5,000-block count on EVERY restart tonight instead of shrinking, and
// directly undermined GetBlockByHeight's canonical SelectedParent-chain
// walk (a chain with holes in it can't be walked correctly). Computing
// once, correctly, before the very first save closes this gap: the only
// remaining risk window is between this computation and the save two
// statements below — no replay, no DB round trip, no lock release in
// between — and a crash there leaves the block simply absent from the DB
// (the pre-existing, already-handled "block not saved yet" case), never
// present-but-broken.
dag.computeGHOSTDAGState(block)

// Structural validation passed. Release dag.mu before replay — replay
// uses dag.state's own lock (cs.mu), not dag.mu, and must never run while
// holding dag.mu (ProduceBlock and other dag.mu users would block for the
// duration of every peer block's replay otherwise).
dag.mu.Unlock()

// P2-05 (audit): persist the block header BEFORE state replay.  Old order
// was replay-then-save, which could commit account-balance changes to
// chain_accounts and then fail to write the chain_blocks header — leaving
// the node in an inconsistent state (state committed, block invisible on
// restart) with no way to detect or recover.  Saving first means a DB
// failure aborts before any state is touched; ON CONFLICT DO NOTHING makes
// this idempotent if two concurrent deliveries of the same block race here.
// Failure mode of the new order (save OK, replay fails): P0-02 fix above
// deletes the pre-saved header when replay fails, so state is always consistent.
if dag.state != nil {
	if err := dag.state.SaveBlockToDB(block); err != nil {
		fmt.Printf("[BLOCK] ✗ Could not save peer block #%d header before replay: %v — skipping\n", block.Height, err)
		return false
	}
}

// FIX (the actual BlockDAG correctness bug, not just a hardening pass):
// this used to (1) compare block.StateRoot against dag.state.StateRoot()
// BEFORE replaying this block's own transactions, and (2) insert the block
// into dag.blocks/dag.tips unconditionally, with replay only queued
// asynchronously onto a channel that could silently drop it if full.
//
// (1) is not just "risky" — it is structurally wrong and guaranteed to
// "mismatch" on every single block that contains any transaction, even
// between two perfectly healthy, fully-synced nodes: block.StateRoot is
// computed by the PRODUCER *after* applying this block's own TXs (the
// producer's RPC handlers apply state changes synchronously before queuing
// them for inclusion — see evm_rpc.go/register.go/swap.go), so it is a
// POST-state root. Comparing it against the RECEIVER's StateRoot at this
// point — before the receiver has replayed this block's TXs — compares a
// post-state against a pre-state. That's why "[DAG] StateRoot mismatch ...
// accepted (warn only)" fired constantly throughout this project's history
// on nearly every non-empty block: the check could never have detected real
// divergence, it was comparing the wrong two snapshots by construction.
//
// (2) meant a block could be permanently "in the DAG" (counted in height,
// returned by /api/blocks, used as a valid parent for later blocks) before
// its own state changes were verified to apply cleanly, or even applied at
// all if the replay queue happened to be full.
//
// Fixed by replaying SYNCHRONOUSLY, right here, before the block is
// inserted anywhere — and only THEN comparing StateRoot, now correctly
// post-state vs. post-state. replayMu serializes this across concurrent
// AddPeerBlock calls (same ordering guarantee the old channel+goroutine
// provided, without the "silently drop if busy" failure mode: this blocks
// instead of dropping, and replayTransactions' own dedup guard makes that
// safe even under concurrent delivery of the same block).
dag.replayMu.Lock()
replayOK := dag.replayInCanonicalOrder(block)
// P0-01 (audit): do NOT release replayMu here on success — hold it through the
// dag.mu section below so ProduceBlock (which also takes replayMu before dag.mu)
// cannot read a post-replay state while this block is still absent from
// dag.blocks/dag.tips. Released on the failure path and after dag.mu.Unlock().

// FIX (block-level atomicity): replayTransactions now rolls back and
// returns false if any of this block's transactions hit a genuine
// state-inconsistency failure (not an expected idempotent skip like
// "already registered"), OR if the post-replay StateRoot doesn't match
// the producer's claimed root (audit recheck 2, P0 #1 — moved into
// replayTransactions itself so a mismatch can use that function's own
// rollback snapshot; see its comment).
//
// GHOSTDAG soft-retry: instead of permanently rejecting a replay failure,
// queue the block for a retry after the next successful peer block is
// accepted (which may have applied the state this block depends on —
// e.g. Block X gives Bob 100, Block Y spends Bob→Carol 50; if Y arrives
// first and Bob has 0, the old code permanently rejected Y).  Entries
// older than softRetryTTL (5 min) are abandoned by retryAndFlushSoftRetry.
if !replayOK {
	dag.replayMu.Unlock() // P0-01: release on failure — block won't enter DAG
	// P0-02 (audit): undo the pre-saved header — replay failed, state was
	// never applied. Without this, a restart would mark this block as
	// replayed in dag.replayedBlocks while chain_accounts has no record of
	// its state changes ("header committed, state missing"). Delete now so
	// the block is re-fetched from peers and replayed cleanly on next sync.
	// P1-01 (audit): a single failed delete used to just log a warning and
	// move on, leaving open the "header persisted, state missing" window
	// across a restart. Retry a few times (DB errors here are typically a
	// transient connection blip) and if it still fails, mark the node
	// degraded so /api/health surfaces it rather than silently continuing
	// on a DB that may now disagree with in-memory state after a restart.
	if dag.state != nil {
		var delErr error
		for attempt := 0; attempt < 3; attempt++ {
			if delErr = dag.state.DeleteBlockFromDB(block.Hash); delErr == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if delErr != nil {
			dag.degradedMu.Lock()
			dag.degradedReason = fmt.Sprintf("DeleteBlockFromDB failed for block #%d: %v", block.Height, delErr)
			dag.degradedMu.Unlock()
			fmt.Printf("[BLOCK] ✗ DEGRADED: could not remove unapplied block #%d header after 3 attempts: %v\n", block.Height, delErr)
		}
	}
	dag.softRetryMu.Lock()
	if _, alreadyQueued := dag.softRetryBlocks[block.Hash]; !alreadyQueued {
		dag.softRetryBlocks[block.Hash] = block
		dag.softRetryFirstAt[block.Hash] = time.Now()
		fmt.Printf("[GHOSTDAG] Soft-retry queued block #%d (%s...)\n", block.Height, block.Hash[:16])
	}
	dag.softRetryMu.Unlock()
	return false
}

dag.mu.Lock()
if prev, hadStub := dag.blocks[block.Hash]; hadStub && prev.Proposer == "synthetic-checkpoint" {
	// Healing a stub (see the "already known" check above): the gap this
	// stub was bridging is now closed with real, replayed data.
	dag.syntheticCheckpointCount.Add(-1)
	// Decrement the unverified counter only while this stub is still TRACKED
	// as unverified. Map membership (not the `prev.Height > dag.bootHeight`
	// comparison used at insertion) is the authoritative test now:
	// releaseFinalitySealedStubs may have already released this stub, and
	// decrementing a second time here would drive the counter negative and
	// desynchronize it from unverifiedStubHeights.
	if _, tracked := dag.unverifiedStubHeights[block.Hash]; tracked {
		delete(dag.unverifiedStubHeights, block.Hash)
		dag.unverifiedSyntheticCheckpointCount.Add(-1)
	}
	fmt.Printf("[BLOCK] ✓ Synthetic checkpoint at height %d healed — %d still active\n", prev.Height, dag.syntheticCheckpointCount.Load())
}
// FIX (durable fix, 2026-07-04): GHOSTDAG state was already computed AND
// persisted (as part of the single, correct SaveBlockToDB call) before
// replay ran — see that call site's comment for why. Recomputing here
// would be redundant (computeGHOSTDAGState is deterministic, so it can
// only recompute the identical result) and re-saving would just repeat
// the same DB write for no benefit — removed, not just deduplicated,
// because the OLD two-phase version of this (compute+save AFTER replay)
// is exactly the gap that let a restart during replay strand a block with
// SelectedParent="" forever. block.SelectedParent/Blues/BlueScore are
// already correct on this struct by the time it reaches dag.blocks here.
dag.blocks[block.Hash] = block

// Remove parents from tips
for _, ph := range block.ParentHashes {
	delete(dag.tips, ph)
}

// Add this block as new tip
dag.tips[block.Hash] = true

if block.Height > dag.height {
	dag.height = block.Height
	// setConfigValueDB: this runs under dag.mu only, not cs.mu, so
	// setConfigValue's cs.dbExec()/cs.activeTx precondition isn't met here
	// (same reasoning as the ProduceBlock post-commit goroutine above).
	dag.state.setConfigValueDB("max_block_height", fmt.Sprintf("%d", dag.height))
}

// Equivocation detection: index this block and trigger slashing if a
// second block from the same proposer for the same parent set is found.
// Runs under dag.mu (checkAndIndexEquivocation requires it) and spawns a
// goroutine for the DB work so it doesn't delay block acceptance.
if conflict, isEquivocation := dag.checkAndIndexEquivocation(block); isEquivocation && dag.state != nil && !block.FromSync &&
	// Activation cutoff: evidence anchored before equivocationSlashingActivationUnix
	// is indexed (above) but never penalized — otherwise a fresh node replaying
	// history re-punishes the pre-feature incident chaos that long-running nodes
	// never did, and ends up suspending every honest validator (confirmed live
	// 2026-07-03; full rationale at the constant's definition in slashing.go).
	block.Timestamp >= equivocationSlashingActivationUnix {
	proposerAddr := block.Proposer
	blockAHash := conflict.Hash
	blockBHash := block.Hash
	detectedAt := block.Timestamp
	SafeGoroutine("equivocation-slashing", func() {
		count, slashWallet, rErr := dag.state.RecordEquivocationAndSuspend(proposerAddr, blockAHash, blockBHash, detectedAt)
		if rErr != nil {
			fmt.Printf("[SLASHING] ✗ Failed to record equivocation for %s: %v\n", proposerAddr, rErr)
			return
		}
		fmt.Printf("[SLASHING] ✓ Equivocation recorded for %s (offense #%d)\n", proposerAddr, count)
		if slashWallet != "" {
			if qErr := dag.state.MaybeQueueSlashOutboxTx(proposerAddr, slashWallet, blockAHash, blockBHash, equivocationSecondOffensePenaltyAEQ); qErr != nil {
				fmt.Printf("[SLASHING] ✗ Could not queue slash TX for %s: %v\n", proposerAddr, qErr)
			} else {
				fmt.Printf("[SLASHING] ✓ Slash TX queued: %.0f AEQ from %s (signer %s)\n",
					equivocationSecondOffensePenaltyAEQ, slashWallet, proposerAddr)
			}
		}
	})
}

// Advance the hard finality checkpoint now that GHOSTDAG has been computed
// for this block (SelectedParent and BlueScore are populated above).
dag.maybeAdvanceFinalizedCheckpoint(block)
// FIX (P0, 2026-07-04 — Contabo 2 permanent-isolation incident): a real,
// successfully-attached OTHER validator's block is exactly the evidence
// selfProducedFinalityAllowed needs to keep trusting self-produced blocks'
// own hardening — see that function's comment. Compared case-insensitively;
// block.Proposer is the raw hex address as received, dag.selfProposer is
// stored lower-cased (see its own field comment).
if !strings.EqualFold(block.Proposer, dag.selfProposer) {
	dag.recordForeignMerge()
	// FIX (2026-07-05 — permanent operational diagnostic, not a temp one):
	// this is the actual number every circuit-breaker/BLOCK_TIME tuning
	// decision tonight was ultimately guessing at without ever measuring
	// directly — real end-to-end time from another validator producing a
	// block to THIS node successfully attaching it. See ProducedAtMs's own
	// field comment for why it's safe (not hash-covered) and why the
	// second-resolution Timestamp field was too coarse for this. Skipped
	// for a peer running an older binary without this field (ProducedAtMs
	// still zero) rather than logging a nonsense multi-decade "latency".
	if block.ProducedAtMs > 0 {
		if latency := time.Now().UnixMilli() - block.ProducedAtMs; latency >= 0 {
			dag.recordForeignAttachLatency(latency)
		}
	}
}

tipCount := len(dag.tips)
dag.mu.Unlock()
dag.replayMu.Unlock() // P0-01: released here, after block is fully visible in DAG

// FIX (audit recheck3, P2 — "IncrementBlockCount laeuft asynchron und
// ist nicht konsensual deterministisch"): this was worse than just
// asynchronous — it was never called here at all. ProduceBlock only
// ever incremented blocks_produced for blocks THIS node itself
// produced; a peer block accepted here never touched the counter for
// ITS proposer. distributeValidatorsPoolLocked reads blocks_produced
// as the proportional reward weight (falling back to a minimum of 1
// for any registered node stuck at 0) — so on whichever single node
// actually runs distribution (DISTRIBUTION_ENABLED=true), every OTHER
// validator's real block production was invisible and they were
// floored to the same token "1" weight regardless of how active they
// actually were, while the distribution node's own blocks counted
// fully. Real fix, not just "make it synchronous": count every
// accepted block here too, for whichever proposer signed it — that's
// the only way this node's blocks_produced table ends up reflecting
// every validator's actual production, not just its own.
dag.state.IncrementBlockCount(block.Proposer)

// Now that this block exists (and has been replayed), any blocks that were
// queued as orphans waiting specifically on this hash as their missing
// parent can be retried — this naturally cascades: if a retried orphan
// succeeds, its own dependents get resolved the same way when ITS
// insertion reaches this point.
//
// FIX (same class as the runtime-orphan-bridge cascade above, block.go
// ~1712): calling dag.AddPeerBlock(waiting) directly in this call stack IS a
// synchronous recursive call (Go has no TCO) despite the "fresh top-level
// call, not recursing" comment this replaces — a long chain of resolved
// orphans (plausible during any large catch-up burst) would block whichever
// goroutine originally called AddPeerBlock (an HTTP push handler or P2P
// stream handler) for the full cascade duration, the same stall the
// runtime-orphan-bridge fix above was written to eliminate. Backgrounding it
// lets this call return immediately; the retries still go through the
// normal dag.mu-guarded AddPeerBlock path with no special priority.
if waiting := dag.popOrphans(block.Hash); len(waiting) > 0 {
	go func(toRetry []*Block) {
		// FIX (P0-3, beta-launch audit 2026-07-05): see panic_recovery.go.
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[PANIC RECOVERED] orphan-retry goroutine: %v\n%s\n", r, debug.Stack())
			}
		}()
		for _, b := range toRetry {
			dag.AddPeerBlock(b)
		}
	}(waiting)
}

// GHOSTDAG soft-retry: now that a new block's state changes are committed,
// blocks that previously failed replayTransactions due to a state dependency
// (e.g. insufficient balance because a sibling block hadn't applied yet)
// get another chance.  Runs in a goroutine so the current AddPeerBlock call
// returns promptly — retries cascade through AddPeerBlock's own orphan
// resolution if they succeed.
dag.triggerSoftRetryFlush()

dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix())
// FIX (P0, 2026-07-03 night): don't blindly clear this proposer's breaker
// run — if replayTransactions just logged a StateRoot mismatch for THIS
// exact block, that already recorded a strike (see lastReplayMismatchHash's
// struct comment); clearing it right back to zero here would make the
// breaker unable to ever trip against a validator whose blocks always
// mismatch. Only report a clean "true" when this block did NOT just mismatch.
dag.replayMismatchMu.Lock()
mismatched := dag.lastReplayMismatchHash == block.Hash
if mismatched {
	dag.lastReplayMismatchHash = ""
}
dag.replayMismatchMu.Unlock()
if !mismatched {
	dag.recordProposerOutcome(block.Proposer, true) // block attached cleanly — clear this proposer's breaker run
}
fmt.Printf("[DAG] ✓ Added peer block #%d | Tips: %d\n", block.Height, tipCount)
return true
}

// TotalStateRootMismatches sums every proposer's consecutive StateRoot
// mismatch counter, and LastSuccessfulPeerSyncAt returns the Unix
// timestamp of the last accepted peer block (0 if none yet this process) —
// both exposed via /api/health/combined (Gesamtaudit 2026-06-28, P2-4/P3-7).
func (dag *BlockDAG) TipsCount() int {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return len(dag.tips)
}

// SyntheticCheckpointCount (audit P1-04) reports how many synthetic-checkpoint
// stubs BridgeHistoricalGap has inserted into the in-memory DAG that haven't
// yet been overwritten by the real block syncing in behind them. The audit's
// concern was that a node could silently enter this "trust bootstrap" mode
// without operator visibility — surfacing the count (and its boolean trust_mode
// derivative) in /api/health makes it visible without a log dive, without
// requiring a new mandatory env flag that operators could forget to set on a
// node that genuinely needs it (as Contabo did on 2026-06-30).
// FIX (audit 2026-06-30 monster audit, P1-05): used to scan every entry in
// dag.blocks on every call — O(chain length), and ProduceBlock now needs
// this exact count on every single block it produces (see ProduceBlock's
// gate). Reads the running counter maintained at each insertion site
// instead.
func (dag *BlockDAG) SyntheticCheckpointCount() int {
	return int(dag.syntheticCheckpointCount.Load())
}

// UnverifiedSyntheticCheckpointCount reports only the synthetic-checkpoint stubs
// ABOVE the trusted snapshot boundary (bootHeight) — genuine mid-chain gaps in
// otherwise-verifiable history. This, NOT the total, is what gates block
// production and the node's healthy flag: a stub at/below bootHeight is the
// snapshot's own start-of-history point (deliberately bootstrapped from a signed
// snapshot, unhealable because no node retains blocks below it) and is trusted
// like genesis. Same lock-free atomic-read pattern as SyntheticCheckpointCount,
// safe to call under ProduceBlock's held write lock.
func (dag *BlockDAG) UnverifiedSyntheticCheckpointCount() int {
	return int(dag.unverifiedSyntheticCheckpointCount.Load())
}

// SyntheticCheckpointHashes returns the hashes of every synthetic-checkpoint
// stub currently trusted by this node — the set healSyntheticCheckpoints
// (sync_blocks.go) tries to replace with real blocks from peers. Guarded by
// the atomic counter so the (otherwise O(len(dag.blocks))) full scan only
// runs when there's actually something to find.
func (dag *BlockDAG) SyntheticCheckpointHashes() []string {
	if dag.syntheticCheckpointCount.Load() == 0 {
		return nil
	}
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	var hashes []string
	for hash, b := range dag.blocks {
		if b.Proposer == "synthetic-checkpoint" {
			hashes = append(hashes, hash)
		}
	}
	return hashes
}

// stateRootMismatchActiveWindow (audit 2026-06-30 monster audit, P2-03): a
// proposer's mismatch count only counts toward health/alerting if it
// mismatched within this window. Wide enough to span a real, sustained
// divergence (which keeps refreshing the timestamp every block, easily
// clearing this window) while letting a one-off startup-race count age out
// instead of marking an otherwise-converged, healthy node "unhealthy"
// forever — see stateRootMismatchLastAt's struct comment for the incident
// this fixes.
const stateRootMismatchActiveWindow = 10 * time.Minute

func (dag *BlockDAG) TotalStateRootMismatches() int {
	dag.stateRootMismatchesMu.Lock()
	defer dag.stateRootMismatchesMu.Unlock()
	cutoff := time.Now().Add(-stateRootMismatchActiveWindow).Unix()
	total := 0
	for proposer, n := range dag.stateRootMismatches {
		if dag.stateRootMismatchLastAt[proposer] < cutoff {
			continue // stale — that proposer hasn't mismatched recently
		}
		total += n
	}
	return total
}

func (dag *BlockDAG) LastSuccessfulPeerSyncAt() int64 {
	return dag.lastSuccessfulPeerSyncAt.Load()
}

// Note: uses Go's built-in min() (available since Go 1.21; this module
// targets 1.24.1) rather than a custom helper — other files in this
// package already define min4()/min4b() specifically to avoid shadowing
// the built-in, so we follow that same convention here by not shadowing it.

// LatestBlock returns a single representative tip for display purposes
// (e.g. /api/status's "latest_hash"/"height"). With more than one validator
// producing concurrently, it's normal and expected to have multiple tips at
// the same max height for brief windows until the next block merges them —
// that's the DAG working as intended, not a fork.
//
// FIX: this used to pick whichever same-height tip happened to be visited
// first during Go's randomized map iteration — never replaced on a height
// TIE (only on strictly greater height) — so two nodes that both genuinely
// held the exact same set of tips could still report two DIFFERENT
// "latest_hash" values purely because their map iteration order differed.
// That's a misleading status signal, not an actual ledger divergence
// (StateRoot is computed from the full account/pool/nullifier state via
// replay, independent of which tip this function reports) — but it made
// "are these nodes in sync" impossible to answer just by comparing
// /api/status output, confirmed in production.
//
// FIX (P0, 2026-07-04 — root cause of the recurring "blue scores don't
// match across nodes" complaints): the height+hash tie-break above was
// itself inconsistent with the rest of the codebase's canonical-tip rule.
// canonicalBlockAtHeightLocked (used by /api/blocks/canonical, the
// authoritative endpoint) and ProduceBlock's own parent selection both pick
// the best tip by highest BlueScore first, hash only as the tie-break —
// GHOSTDAG's actual total order. Height alone is NOT a reliable proxy for
// "more blue work": with concurrent validators, a tip's blue_score reflects
// its full merge history, not just how many blocks deep it is, so two tips
// at the identical height can have very different blue_scores. Confirmed
// live: /api/status's latest_hash matched across all three validators while
// /api/blocks/canonical simultaneously showed genuinely different blocks —
// same node, same moment, two different answers to "what's canonical",
// because the two endpoints used two different rules. Now uses the
// identical BlueScore-DESC/Hash-ASC rule everywhere, so "do these nodes
// agree" is answerable consistently no matter which endpoint is checked.
func (dag *BlockDAG) LatestBlock() *Block {
dag.mu.RLock()
defer dag.mu.RUnlock()
var latest *Block
for hash := range dag.tips {
b := dag.blocks[hash]
if b == nil {
continue
}
if latest == nil || b.BlueScore > latest.BlueScore || (b.BlueScore == latest.BlueScore && b.Hash < latest.Hash) {
latest = b
}
}
return latest
}

func (dag *BlockDAG) Height() int64 {
dag.mu.RLock()
defer dag.mu.RUnlock()
return dag.height
}

func (dag *BlockDAG) GetBlocks() []*Block {
dag.mu.RLock()
defer dag.mu.RUnlock()
result := make([]*Block, 0, len(dag.blocks))
for _, b := range dag.blocks {
// FIX (2026-06-30, confirmed live in production): synthetic-checkpoint
// stubs (BridgeHistoricalGap / the orphan-bridge retry path) were being
// served here and then over /api/blocks like real blocks. A stub's Hash
// field is borrowed from whatever it was bridging (the real hash of the
// block it's standing in for) — it was never produced by calculateHash
// from the stub's own (mostly empty) fields, so it can NEVER pass a
// peer's hash-mismatch check. Every peer fetching one rejected it, and
// everything built on top of it (the real block that legitimately
// references that parent hash) got stuck behind that rejection forever.
// Stubs exist only to satisfy THIS node's own internal parent-existence
// lookups (dag.blocks[hash] != nil) — never to be handed to a peer as if
// they were a verifiable block.
if b.Proposer == "synthetic-checkpoint" {
continue
}
result = append(result, b)
}
// P1-03 (audit): stable tie-breaker prevents non-deterministic pagination
// when same-height siblings exist. Order: height ASC, blueScore DESC, hash ASC.
sort.Slice(result, func(i, j int) bool {
	a, b := result[i], result[j]
	if a.Height != b.Height {
		return a.Height < b.Height
	}
	if a.BlueScore != b.BlueScore {
		return a.BlueScore > b.BlueScore
	}
	return a.Hash < b.Hash
})
return result
}

// selectBlocksSince filters an already canonically-sorted (height ASC,
// blueScore DESC, hash ASC) slice down to what a min_height/after_hash sync
// request wants, capped at limit: either "Height > minHeight" (no cursor), or
// strictly after the (minHeight, afterHash) cursor position — which also
// includes same-height siblings that come after afterHash in canonical order
// (P1-02: a full page of same-height blocks must not silently skip the rest).
// Shared between the in-memory path (GetBlocksSince) and the DB fallback
// path (LoadBlocksSinceFromDB) so the two can never drift apart on
// pagination semantics.
func selectBlocksSince(sorted []*Block, minHeight int64, afterHash string, limit int) []*Block {
	result := make([]*Block, 0, limit)
	if afterHash == "" {
		for _, b := range sorted {
			if b.Height > minHeight {
				result = append(result, b)
				if len(result) >= limit {
					break
				}
			}
		}
		return result
	}
	pastCursor := false
	for _, b := range sorted {
		if !pastCursor {
			if b.Height > minHeight {
				pastCursor = true
				result = append(result, b)
			} else if b.Height == minHeight && b.Hash == afterHash {
				pastCursor = true
			}
		} else {
			result = append(result, b)
		}
		if len(result) >= limit {
			break
		}
	}
	return result
}

// GetBlocksSince backs GET /api/blocks?min_height=&after_hash= — the bulk
// paginated endpoint doSyncOnce uses for ordinary forward sync. Falls back to
// the DB when the requested range has been evicted from the in-memory
// dag.blocks window.
//
// FIX (P0, merge-reliability audit 2026-07-03 — root cause of "Primary and
// Contabo do not merge at all"): pruneOldDAGBlocks evicts everything below
// (finalizedHeight - pruneBuffer()) from dag.blocks every 60s, but this
// endpoint used to read ONLY that in-memory map (via GetBlocks()) regardless
// of min_height. A peer whose min_height fell below the current prune window
// (a fresh RESYNC_FROM_SNAPSHOT, a long outage, initial bootstrap — anything
// more than pruneBuffer() blocks behind) did NOT get an empty/short response
// signalling "nothing here" — every block currently resident in dag.blocks
// legitimately has Height > any sufficiently-low minHeight, so the peer got
// served a full page of blocks from THIS node's CURRENT tip window instead,
// whose parent chain the requesting node has no way to attach (its own
// history stops far below that window). Confirmed live: Primary at height
// ~131400 answered min_height=0&limit=5 with blocks #131400-131403, not
// anything reachable from height 0. A node in this state can never catch up
// no matter how long it waits or retries — pruneOldDAGBlocks keeps deleting
// the connecting blocks out of memory every single cycle, racing (and
// always winning) against the node's own catch-up — while chain_blocks (the
// DB) has always had the correct answer sitting right there, just never
// consulted by this path. When minHeight falls below this node's own prune
// cutoff, go straight to the DB instead of memory.
func (dag *BlockDAG) GetBlocksSince(minHeight int64, afterHash string, limit int) []*Block {
	if dag.state != nil {
		finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
		cutoff := finalizedHeight - dag.pruneBuffer()
		if cutoff > 0 && minHeight < cutoff {
			dbBlocks, err := dag.state.LoadBlocksSinceFromDB(minHeight, afterHash, limit)
			if err == nil {
				return dbBlocks
			}
			fmt.Printf("[BLOCK] GetBlocksSince: DB fallback failed for min_height=%d: %v — serving in-memory window instead (likely incomplete for this range)\n", minHeight, err)
		}
	}
	return selectBlocksSince(dag.GetBlocks(), minHeight, afterHash, limit)
}

// GetBlockByHash returns the block with the given hash, or nil if unknown.
// Used by /api/block/{hash} so a syncing peer can fetch one specific
// missing-ancestor block directly instead of relying solely on the
// height-windowed /api/blocks pagination (see fetchMissingAncestors).
//
// FIX (P0, merge-reliability audit 2026-07-03): pruneOldDAGBlocks evicts
// blocks below (finalizedHeight - pruneBuffer()) from dag.blocks — this
// used to return nil for any such hash even though chain_blocks (the DB)
// still has it, matching the exact gap GetBlockByHeight was already fixed
// for (416dfa7) but this sibling lookup was missed. That left
// fetchMissingAncestors' orphan resolution and eth_getBlockByHash unable to
// ever resolve a hash older than the in-memory window, no matter how long a
// catching-up node waited. Falls back to the DB, outside the lock, exactly
// like GetBlockByHeight already does.
func (dag *BlockDAG) GetBlockByHash(hash string) *Block {
	dag.mu.RLock()
	b := dag.blocks[hash]
	dag.mu.RUnlock()
	if b != nil || dag.state == nil {
		return b
	}
	return dag.state.LoadBlockFromDBByHash(hash)
}

// GetBlocksByHashesForPeer resolves many hashes under a SINGLE RLock and omits
// synthetic-checkpoint stubs (which must never be served to a peer — see
// GetBlocks' comment). This backs POST /api/blocks/by-hash.
//
// CADENCE/SYNC FIX (2026-07-02): the handler used to call GetBlockByHash in a
// loop — up to maxBlocksByHashPerRequest (500) separate RLock acquisitions per
// request. On a node whose ProduceBlock holds dag.mu for a slow remote-DB save
// (confirmed live on the primary), each of those 500 RLocks blocks behind the
// writer, so a single by-hash request could exceed the caller's 30s timeout and
// fail entirely — which is exactly what stranded a resyncing secondary
// (Contabo) mid-catch-up: it could never resolve its missing ancestors because
// every batch request timed out. One RLock for the whole batch is contended at
// most once, not 500 times.
func (dag *BlockDAG) GetBlocksByHashesForPeer(hashes []string) []*Block {
	dag.mu.RLock()
	out := make([]*Block, 0, len(hashes))
	var missing []string
	for _, h := range hashes {
		if b := dag.blocks[h]; b != nil && b.Proposer != "synthetic-checkpoint" {
			out = append(out, b)
		} else if b == nil {
			missing = append(missing, h)
		}
	}
	dag.mu.RUnlock()
	// FIX (P0, merge-reliability audit 2026-07-03): see GetBlockByHash's
	// matching fix — a hash pruneOldDAGBlocks already evicted from dag.blocks
	// used to be silently omitted here (indistinguishable from "peer genuinely
	// doesn't have it"), which is exactly the lookup fetchMissingAncestors
	// relies on to resolve orphaned ancestors during catch-up. Fall back to
	// the DB for whatever wasn't found in memory, outside the lock.
	if len(missing) > 0 && dag.state != nil {
		if dbBlocks, err := dag.state.LoadBlocksByHashesFromDB(missing); err == nil {
			out = append(out, dbBlocks...)
		}
	}
	return out
}

// GetBlockByHeight returns the CANONICAL block at the given height, or nil
// if none exists.
//
// FIX (durable fix, 2026-07-04 — real fix for "the explorer shows a
// different proposer at the same height on every node"): multiple
// validators routinely produce siblings at the same height (normal,
// expected GHOSTDAG behavior — see canonicalBlockAtHeightLocked's comment).
// This used to pick "whichever sibling has the most parent hashes" — a
// heuristic with no relationship to GHOSTDAG's actual canonical ordering.
// Confirmed live: three simultaneously healthy nodes (0 StateRoot
// mismatches, matching proposer-distribution stats) returned three
// DIFFERENT blocks with three different proposers for the identical
// height, purely because "most parents" isn't a deterministic,
// cross-node-agreeing tie-break — a node with a slightly different
// in-memory sibling set at query time picks a different "most parents"
// winner. Every node computes the same SelectedParent chain from its own
// best (highest-BlueScore) tip once views have converged (see this file's
// own header comment: "every node that holds the same block graph computes
// identical GHOSTDAG state") — walking that chain back to the target
// height is the actual canonical answer, not a per-node coin flip.
func (dag *BlockDAG) GetBlockByHeight(height int64) *Block {
	// canonicalBlockAtHeightLocked calls ghostdagBlockLookup, which can
	// cache a DB-fetched block into dag.blocks — needs the write lock, not
	// a read lock, for that mutation to be safe.
	dag.mu.Lock()
	best := dag.canonicalBlockAtHeightLocked(height)
	dag.mu.Unlock()
	if best != nil || dag.state == nil {
		return best
	}
	// FIX (P0, merge-reliability audit 2026-07-03): pruneOldDAGBlocks evicts
	// blocks below (finalizedHeight - pruneBuffer()) from dag.blocks, so a
	// height below that cutoff could still miss above if dag.tips itself has
	// been pruned/reset (e.g. right after a restart before any tip exists
	// yet). Fall back to the DB's own best-effort selection (chain_blocks
	// retains everything pruneOldDAGBlocks removes from memory), outside the
	// lock since this is a query, not a mutation.
	return dag.state.LoadBlockFromDBByHeight(height)
}

// canonicalBlockAtHeightLocked returns the canonical block at height by
// walking the SelectedParent chain backward from this node's own best tip
// (highest BlueScore, ties broken by lowest hash — the same "canonical
// ordering: height ASC, blueScore DESC, hash ASC" this file's other
// comments already describe as the deterministic total order every node
// converges to). Returns nil if no tip exists yet, or if the walk runs out
// of SelectedParent links before reaching height (a genuinely unresolvable
// gap — callers fall back to the DB's best-effort selection in that case).
// Must be called under dag.mu (write lock — see ghostdagBlockLookup's own
// locking contract, which this relies on for cache-filling DB fallback
// during the walk).
//
// FIX (P0, 2026-07-04 — same class of outage as maxGhostdagDBLookups,
// found on self-review the same night that one shipped): this walk used to
// pass a nil budget to ghostdagBlockLookup — unbounded DB round trips,
// exactly the hazard just fixed for computeGHOSTDAGState, on a path that's
// arguably hit MORE often (every /api/block?height=X request, i.e. every
// explorer page load and eth_getBlockByNumber call, not just block
// ingestion) and holds the very same dag.mu WRITE lock for the duration.
// A height whose SelectedParent chain has any gap in dag.blocks (routine
// after a restart, or with the pre-two-phase-save-gap-fix history this
// session already found riddled with them) could walk arbitrarily many
// hops, each a potential synchronous Postgres round trip, all while
// blocking every other dag.mu consumer — a single slow query, or an
// ordinary burst of them from a page load fetching several blocks, could
// reproduce the exact multi-second-to-minutes stall already fixed
// elsewhere tonight, just via this call site instead. Same fix: a bounded
// budget, falling back to the caller's existing DB-heuristic fallback
// (GetBlockByHeight already calls LoadBlockFromDBByHeight when this
// returns nil) instead of blocking indefinitely.
func (dag *BlockDAG) canonicalBlockAtHeightLocked(height int64) *Block {
	var best *Block
	for hash := range dag.tips {
		b := dag.blocks[hash]
		if b == nil || b.Proposer == "synthetic-checkpoint" {
			continue
		}
		if best == nil || b.BlueScore > best.BlueScore || (b.BlueScore == best.BlueScore && b.Hash < best.Hash) {
			best = b
		}
	}
	if best == nil {
		return nil
	}
	dbBudget := dag.maxGhostdagDBLookups()
	cur := best
	for cur != nil && cur.Height > height {
		if cur.SelectedParent == "" {
			return nil
		}
		cur = dag.ghostdagBlockLookup(cur.SelectedParent, &dbBudget)
	}
	if cur != nil && cur.Height == height && cur.Proposer != "synthetic-checkpoint" {
		return cur
	}
	return nil
}

func (dag *BlockDAG) TotalBlocks() int {
dag.mu.RLock()
defer dag.mu.RUnlock()
return len(dag.blocks)
}

func (dag *BlockDAG) GetTips() []string {
dag.mu.RLock()
defer dag.mu.RUnlock()
tips := make([]string, 0, len(dag.tips))
for hash := range dag.tips {
tips = append(tips, hash)
}
return tips
}

// replayTransactions applies all TX types from a peer block to the local
// state. The block's ECDSA signature was already verified against an
// authorized validator before this function is reached.
//
// Design principle: secondary nodes apply the STORED amounts directly
// rather than re-running business logic. This avoids divergence from
// pool-state differences, floating-point order sensitivity, and
// demurrage timing differences between nodes.
// replayTransactions applies block's transactions to local state and
// returns false if the block was rolled back due to a genuine
// state-inconsistency failure (and should therefore NOT be considered
// applied — see AddPeerBlock, which rejects the whole block in that case).
//
// FIX (block-level atomicity): this used to apply each TX with continue-on-
// error and no way to undo TXs that had already succeeded earlier in the
// SAME block. A block with TX1 (succeeds) and TX2 (genuinely fails —
// insufficient balance, missing account; NOT an expected idempotent skip
// like "already registered") ended up partially applied: this node's state
// reflected less than what the producer's block.StateRoot was computed
// against. Money-moving TX types (transfer/swap/add_liquidity/
// remove_liquidity/faucet) now snapshot the touched accounts + pool before
// replay and roll back to that snapshot if any of them hits a genuine
// failure, so a failed block changes nothing instead of changing some of
// what it intended to. register_human's existing per-TX skip conditions
// (already registered, invalid proof, malformed data) are deliberately
// NOT treated as block-wide failures — those are intentional content
// rejections, not signs of state divergence, and were already
// self-consistent (TryClaimNullifier/ReleaseNullifier are already a
// correctly paired claim/release).
func (dag *BlockDAG) replayTransactions(block *Block) bool {
	// Fix 4: Deduplication guard — if this block has already been replayed,
	// skip it. Prevents double-credits when a block is delivered more than once.
	dag.replayedMu.Lock()
	if dag.replayedBlocks[block.Hash] {
		dag.replayedMu.Unlock()
		return true // already successfully replayed
	}
	dag.replayedMu.Unlock()

	// FIX (double-apply on snapshot bootstrap): a node that imported a
	// snapshot already has the cumulative effect of every block up to and
	// including snapshot_import_height baked into cs.accounts. Without
	// this guard, the HTTP-SYNC catch-up that follows (which always starts
	// from height 0, since dag.blocks is empty in memory after any
	// restart regardless of what the snapshot seeded) would apply every
	// pre-snapshot block's transactions a second time on top of the
	// already-current balances — confirmed in production: two secondary
	// nodes that bootstrapped from the same primary snapshot both ended up
	// crediting one wallet +2 AEQ and debiting another -2 AEQ relative to
	// the primary, exactly matching one historical transfer being replayed
	// twice. Mark the block as replayed (so dedup/tips/hash-chain
	// bookkeeping in the caller proceeds normally) without touching state.
	skipHeight := dag.bootHeight
	// FIX (audit 2026-06-28 recheck 4, P0-1): this runs before
	// dag.state.mu.Lock() is taken further down in this function — must use
	// the plain DB-only read, never cs.dbExec()/cs.activeTx, or this could
	// race against a concurrent atomic operation's in-flight transaction.
	if heightStr := dag.state.getConfigValueDB("snapshot_import_height"); heightStr != "" {
		var snapshotHeight int64
		fmt.Sscanf(heightStr, "%d", &snapshotHeight)
		if snapshotHeight > skipHeight {
			skipHeight = snapshotHeight
		}
	}
	// FIX (audit recheck 2, P0 #1 follow-up): bootHeight covers the more
	// general case the snapshot_import_height guard above was written for
	// — ANY node whose cs.accounts already reflects history that its
	// in-memory dag.blocks/dag.tips don't, not just one that bootstrapped
	// via snapshot. Confirmed in production within minutes of deploying
	// the StateRoot hard-reject above: a plain node restart (no snapshot
	// involved) immediately got stuck rejecting every single ancestor
	// block during ordinary post-restart catch-up, because cs.accounts
	// (loaded fully from the DB) was already at the LATEST state while
	// each ancestor block's claimed StateRoot reflects state as of THAT
	// historical height — comparing "now" against "back then" was always
	// going to mismatch, with no real divergence involved. Below
	// skipHeight, the block is still fetched and inserted into
	// dag.blocks/dag.tips (hash-chain/tips bookkeeping needs it as a valid
	// parent for later blocks) but neither its transactions nor its
	// StateRoot claim are touched.
	// FIX (audit recheck2, P2 #4): naming this explicitly, since it's a real
	// trust boundary, not just a performance shortcut. Every block ABOVE
	// skipHeight is independently re-verified by replaying its transactions
	// and checking the resulting StateRoot — this node never just trusts a
	// peer's claim. Every block AT OR BELOW skipHeight is "snapshot trust
	// mode": this node accepts cs.accounts' already-loaded state (from its
	// own DB, or from a signed snapshot import) as correct for that range
	// without re-deriving it from block history, because re-deriving it
	// would require replaying transactions whose effects are already
	// baked into that state by definition (see bootHeight's and
	// snapshot_import_height's comments for why re-replaying them would
	// double-apply, not re-verify, them). The actual trust anchor for
	// snapshot-sourced state is ImportSnapshotFromURL/ResyncFromSnapshotURL's
	// mandatory ECDSA signature check against BOOTSTRAP_SIGNER — this skip
	// doesn't grant any trust itself, it just avoids re-deriving what that
	// signature check already vouched for.
	if skipHeight > 0 && block.Height <= skipHeight {
		dag.replayedMu.Lock()
		dag.replayedBlocks[block.Hash] = true
		dag.replayedMu.Unlock()
		return true
	}

	// Distribution idempotency pre-pass: if this block contains a
	// ubi_distribution_finalize TX for a round we've already applied,
	// skip ALL distribution TXs in the block so a competing distribution
	// from another node doesn't double-credit every human.
	//
	// Same-round detection uses a 24h window instead of exact equality:
	// two nodes firing at 20:00:01 and 20:00:03 produce DistributionAt
	// values that differ by 2 seconds. "lastUBIAt >= tx.DistributionAt"
	// would miss this case (1 < 3 → false → no skip → double credit).
	// "tx.DistributionAt - lastUBIAt < 24*3600" captures anything within
	// the same daily window; the negative case (lastUBIAt > DistributionAt)
	// is also correctly skipped since negative int64 < 86400.
	// Plain DB read before cs.mu — same pattern as snapshot_import_height.
	skipDistributionRound := int64(0)
	for _, tx := range block.Transactions {
		if tx.Type == "ubi_distribution_finalize" && tx.DistributionAt > 0 {
			var lastUBIAt int64
			fmt.Sscan(dag.state.getConfigValueDB("last_ubi_at"), &lastUBIAt)
			if lastUBIAt > 0 && tx.DistributionAt-lastUBIAt < 24*3600 {
				skipDistributionRound = tx.DistributionAt
			}
			break
		}
	}

	touchedAddrs, needsFullSnapshot := blockTouchedAddresses(block)
	// FIX (audit recheck3, P0/P1 — "Block-Replay-Rollback ist nicht gegen
	// parallele lokale Mutationen isoliert"): this used to take rollbackSnap
	// via the lock-acquiring, lock-releasing snapshotForRollback, then let
	// every individual Delta function below take and release cs.mu on its
	// own for just that one call. Between any two of those calls — or
	// between the snapshot and the first call — a concurrent API operation
	// or distribution round (each its own complete runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox critical section) could mutate the
	// very same account, fully commit, and report success to its own
	// caller — and if THIS replay later hit a hardFailure or StateRoot
	// mismatch unrelated to that account, rolling back with rollbackSnap
	// would revert it anyway, silently erasing an already-committed,
	// unrelated, successful operation. Holding cs.mu continuously from the
	// snapshot below through either a successful StateRoot match or a
	// rollback closes that gap: every Delta call in this loop now uses its
	// "...Locked" sibling (assumes cs.mu already held) instead of the
	// public lock-each-time wrapper, and the snapshot/rollback/StateRoot
	// comparison below do the same.
	dag.state.mu.Lock()
	defer dag.state.mu.Unlock()
	configBackup := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := dag.state.getConfigValueExists(key)
		configBackup[key] = configValueSnapshot{value: value, existed: existed}
	}
	rollbackSnap := dag.state.snapshotForRollbackLocked(touchedAddrs, needsFullSnapshot, configBackup)

	// FIX (audit 2026-06-28 full recheck, P0-4 — "Replay-Rollback ist nicht
	// als DB-Transaktion isoliert"): every DB write this replay makes (via
	// saveAccountToDB/savePoolToDB/setConfigValue, all routed through
	// cs.dbExec()) used to auto-commit immediately on its own, with
	// rollbackSnap/restoreFromRollbackLocked emulating "undo" at the
	// application level by recomputing and rewriting old values — not a
	// real SQL rollback. If a step partway through failed in a way that
	// left some writes committed and others not, the application-level
	// restore could only ever re-derive what it already knew about
	// (rollbackSnap's captured fields), not guarantee every write this
	// replay made was actually undone. A real DB transaction makes that
	// guarantee structurally instead of by careful bookkeeping: every
	// dbExec() call below joins dbTx (set as cs.activeTx for the duration),
	// and either ALL of them commit together or tx.Rollback() discards
	// every one of them atomically. rollbackSnap/restoreFromRollbackLocked
	// are still used for the IN-MEMORY side (cs.accounts/cs.pool are plain
	// Go maps with no transactional semantics of their own — only the DB
	// side can be made truly atomic this way), now redundant-but-harmless
	// for the DB side specifically when a real rollback already ran (the
	// same pattern runAtomicWithOutbox already established: tx.Rollback()
	// for the DB, restoreFromRollback for memory, in that order).
	var dbTx *sql.Tx
	if dag.state.db != nil {
		var err error
		dbTx, err = dag.state.db.Begin()
		if err != nil {
			fmt.Printf("[REPLAY] ✗ Block #%d: could not begin replay transaction: %v — block rejected\n", block.Height, err)
			return false
		}
		dag.state.activeTx = dbTx
	}
	// commitOrRollback finalizes dbTx according to success, clearing
	// activeTx either way so no write after this point accidentally joins
	// a transaction that's already been resolved. Returns an error if a
	// commit was attempted and failed (caller must then treat this exactly
	// like any other hardFailure, including the in-memory restore).
	commitOrRollback := func(success bool) error {
		if dbTx == nil {
			dag.state.activeTx = nil
			return nil
		}
		dag.state.activeTx = nil
		if !success {
			if err := dbTx.Rollback(); err != nil {
				fmt.Printf("[REPLAY] Warning: replay transaction rollback for block #%d failed: %v\n", block.Height, err)
			}
			return nil
		}
		return dbTx.Commit()
	}
	hardFailure := false
	var claimedNullifiers []string

	for _, tx := range block.Transactions {
		if hardFailure {
			break // stop applying further TXs once we know this block is being rolled back
		}
		// Skip distribution TXs from a round this node has already applied.
		if skipDistributionRound > 0 {
			switch tx.Type {
			case "ubi_distribution", "ubi_distribution_finalize",
				"validator_distribution", "validator_distribution_pool_zero",
				"lp_distribution", "lp_distribution_pool_zero":
				fmt.Printf("[REPLAY] ℹ Skipping %s (distribution round %d already applied, block #%d)\n",
					tx.Type, skipDistributionRound, block.Height)
				continue
			}
		}
		wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
		switch tx.Type {

		case "register_human":
			nullifier := strings.TrimSpace(tx.Nullifier)
			commitment := strings.TrimSpace(tx.Commitment)
			if wallet == "" || nullifier == "" {
				fmt.Printf("[REPLAY] ⚠ Skipping register_human in block #%d: missing wallet or nullifier (older node version?)\n", block.Height)
				continue
			}
			// FIX (audit recheck2, P1 #10): malformed wallet/nullifier, missing
			// proof fields, and an invalid proof used to just `continue` (skip
			// this TX, accept the rest of the block) instead of hardFailure.
			// Unlike the wallet==""/nullifier=="" case above (genuine legacy
			// compat — see its own comment), there is no legitimate node
			// version that ever produces a malformed wallet/short nullifier or
			// omits proof fields; register.go always populates them. A block
			// containing one of these is either a bug in the producing node
			// or a validator deliberately packing unverifiable "registrations"
			// into otherwise-valid block history — permanently, since an
			// accepted block is never revisited. Treating these as the same
			// genuine state-inconsistency failure every other case in this
			// switch already hardFails on closes that gap.
			if len(wallet) != 42 || wallet[:2] != "0x" {
				fmt.Printf("[REPLAY] ✗ register_human in block #%d: malformed wallet %q — rolling back whole block\n", block.Height, wallet)
				hardFailure = true
				continue
			}
			if len(nullifier) < 16 {
				fmt.Printf("[REPLAY] ✗ register_human in block #%d: nullifier too short %q — rolling back whole block\n", block.Height, nullifier)
				hardFailure = true
				continue
			}
			// Verify the ZK proof via BioVerifier before applying. This
			// eliminates unconditional trust in the validator ECDSA key for
			// registration TXs — a compromised validator key cannot inject
			// fake registrations without also producing a valid Groth16 proof.
			//
			// FIX: this used to skip verification entirely (falling through to
			// trust the block signature alone) whenever proof fields were
			// absent, "for backward compatibility with old nodes". Both
			// current TX-creation sites (register.go) always populate
			// ProofA/B/C/PubSignals, so no legitimate code path produces a
			// register_human TX without them anymore — that fallback was pure
			// attack surface letting any authorized validator (or one whose
			// signing key leaked) inject registrations for arbitrary wallets
			// with no biometric proof at all, defeating "one human, one
			// registration" silently.
			if len(tx.ProofA) != 2 || len(tx.ProofB) != 2 || len(tx.ProofC) != 2 || len(tx.PubSignals) < 2 {
				fmt.Printf("[REPLAY] ✗ register_human for %s (block #%d): missing ZK proof fields — rolling back whole block\n", wallet, block.Height)
				hardFailure = true
				continue
			}
			if !dag.verifyZKProof(tx) {
				fmt.Printf("[REPLAY] ✗ register_human for %s (block #%d): ZK proof verification failed — rolling back whole block\n", wallet, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ ZK proof verified for %s (block #%d)\n", wallet, block.Height)
			// FIX (audit 2026-06-28 recheck 5, P1-1): tryClaimNullifierLocked
			// now returns an error distinctly from "already used" — a genuine
			// DB failure during the claim must roll back the block, not be
			// silently treated as a normal duplicate-registration skip.
			claimed, claimErr := dag.state.tryClaimNullifierLocked(nullifier, wallet)
			if claimErr != nil {
				fmt.Printf("[REPLAY] ✗ register_human for %s (block #%d): nullifier claim DB error: %v — rolling back whole block\n", wallet, block.Height, claimErr)
				hardFailure = true
				continue
			}
			if !claimed {
				continue // already registered
			}
			if err := dag.state.registerHumanLocked(wallet); err != nil {
				// FIX: release the nullifier claimed two lines above on failure —
				// it used to stay claimed forever ("nullifier recorded, balance
				// NOT credited"), permanently burning that biometric for
				// everyone even though no registration ever actually completed
				// with it (e.g. wallet already human via a different nullifier).
				dag.state.releaseNullifierLocked(nullifier)
				fmt.Printf("[REPLAY] ✗ RegisterHuman %s: %v (nullifier released, balance NOT credited)\n", wallet, err)
				continue
			}
			// Track this claim so it can be released too if a LATER TX in
			// this same block hard-fails and the whole block gets rolled
			// back — the account-balance/IsHuman side of this registration
			// is already covered by rollbackSnap (this wallet is in
			// blockTouchedAddresses via tx.Wallet), but cs.nullifiers is a
			// separate map the account snapshot doesn't touch.
			claimedNullifiers = append(claimedNullifiers, nullifier)
			if commitment != "" {
				_ = dag.state.SaveBioRegistration(commitment, wallet, tx.TxHash, "")
			}
			fmt.Printf("[REPLAY] ✓ Applied register_human for %s (block #%d)\n", wallet, block.Height)

		case "transfer":
			to := strings.ToLower(strings.TrimSpace(tx.To))
			if wallet == "" || to == "" || tx.Amount <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping transfer in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.applyTransferDeltaLocked(wallet, to, tx.Amount, tx.FromDemurrageLost, tx.ToDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ Transfer %s->%s %.6f: %v (block #%d) — rolling back whole block\n", wallet, to, tx.Amount, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied transfer %.6f AEQ: %s->%s (block #%d)\n", tx.Amount, wallet, to, block.Height)

		case "swap_aeq_tusd":
			if wallet == "" || tx.Amount <= 0 || tx.AmountOut <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping swap_aeq_tusd in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.applySwapDeltaLocked(wallet, tx.Amount, tx.AmountOut, true, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ swap_aeq_tusd %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied swap_aeq_tusd %.6f AEQ->%.6f tUSD for %s (block #%d)\n", tx.Amount, tx.AmountOut, wallet, block.Height)

		case "swap_tusd_aeq":
			if wallet == "" || tx.Amount <= 0 || tx.AmountOut <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping swap_tusd_aeq in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.applySwapDeltaLocked(wallet, tx.Amount, tx.AmountOut, false, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ swap_tusd_aeq %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied swap_tusd_aeq %.6f tUSD->%.6f AEQ for %s (block #%d)\n", tx.Amount, tx.AmountOut, wallet, block.Height)

		case "add_liquidity":
			if wallet == "" || tx.Amount <= 0 || tx.AmountOut <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping add_liquidity in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.addLiquidityDeltaLocked(wallet, tx.Amount, tx.AmountOut, tx.LPShares, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ add_liquidity %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied add_liquidity %.6f AEQ + %.6f tUSD for %s (block #%d)\n", tx.Amount, tx.AmountOut, wallet, block.Height)

		case "remove_liquidity":
			if wallet == "" || tx.Amount <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping remove_liquidity in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.removeLiquidityDeltaLocked(wallet, tx.Amount, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ remove_liquidity %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied remove_liquidity %.6f shares for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "faucet":
			if wallet == "" || tx.Amount <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping faucet in block #%d: missing fields\n", block.Height)
				continue
			}
			if err := dag.state.applyFaucetDeltaLocked(wallet, tx.Amount); err != nil {
				fmt.Printf("[REPLAY] ✗ faucet %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied faucet %.6f tUSD for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "ubi_distribution":
			// FIX (audit recheck 2, P0 #5): main.go now emits ONE of these per
			// human (Wallet set, AmountPerHuman omitted) instead of a single
			// flat broadcast — see ApplyUBIRewardDelta's comment. The
			// AmountPerHuman>0 branch below only fires for historical blocks
			// produced by older node versions before this change; new blocks
			// never set that field.
			if tx.AmountPerHuman > 0 {
				if err := dag.state.applyUBIDeltaLocked(tx.AmountPerHuman, block.Timestamp); err != nil {
					fmt.Printf("[REPLAY] ✗ legacy flat ubi_distribution: %v (block #%d) — rolling back whole block\n", err, block.Height)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ✓ Applied legacy flat UBI distribution %.6f AEQ/human (block #%d)\n", tx.AmountPerHuman, block.Height)
			} else if wallet != "" && wallet != "0x0000000000000000000000000000000000000000" {
				if err := dag.state.applyUBIRewardDeltaLocked(wallet, tx.Amount, tx.FromDemurrageLost); err != nil {
					fmt.Printf("[REPLAY] ✗ ubi_distribution %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ✓ Applied UBI reward %.6f AEQ for %s (block #%d)\n", tx.Amount, wallet, block.Height)
			} else {
				// FIX (audit recheck2, P1 #10): see register_human's matching
				// comment — this TX type is only ever emitted internally by
				// RunDailyDistributionAtomic, never user-submitted, so a
				// malformed one (neither shape populated) means either a
				// producer bug or a validator fabricating distribution
				// history. hardFailure instead of a silent skip.
				fmt.Printf("[REPLAY] ✗ ubi_distribution TX in block #%d has neither amount_per_human nor a wallet — rolling back whole block\n", block.Height)
				hardFailure = true
				continue
			}

		case "ubi_distribution_finalize":
			if err := dag.state.applyUBIFinalizeDeltaLocked(tx.DistributionAt); err != nil {
				fmt.Printf("[REPLAY] ✗ ubi_distribution_finalize: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Finalized UBI round, last_ubi_at=%d (block #%d)\n", tx.DistributionAt, block.Height)

		case "validator_distribution":
			wallet := strings.ToLower(tx.Wallet)
			if err := dag.state.applyValidatorRewardDeltaLocked(wallet, tx.Amount, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ validator_distribution %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied validator reward %.6f AEQ for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "validator_distribution_pool_zero":
			if err := dag.state.applyValidatorPoolZeroDeltaLocked(); err != nil {
				fmt.Printf("[REPLAY] ✗ validator_distribution_pool_zero: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Zeroed validators pool (block #%d)\n", block.Height)

		case "lp_distribution":
			wallet := strings.ToLower(tx.Wallet)
			if err := dag.state.applyLPRewardDeltaLocked(wallet, tx.Amount, tx.FromDemurrageLost); err != nil {
				fmt.Printf("[REPLAY] ✗ lp_distribution %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied LP reward %.6f AEQ for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "lp_distribution_pool_zero":
			if err := dag.state.applyLPPoolZeroDeltaLocked(); err != nil {
				fmt.Printf("[REPLAY] ✗ lp_distribution_pool_zero: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Zeroed LP pool (block #%d)\n", block.Height)

		case "escrow_move":
			wallet := strings.ToLower(tx.Wallet)
			if err := dag.state.applyEscrowMoveDeltaLocked(wallet, tx.FromDemurrageLost, tx.LPShares, tx.EscrowTUsdConverted); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_move %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow move for %s (block #%d)\n", wallet, block.Height)

		case "escrow_release":
			if err := dag.state.applyEscrowReleaseDeltaLocked(tx.Amount); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_release: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow release %.6f AEQ → UBI pool (block #%d)\n", tx.Amount, block.Height)

		case "escrow_recover":
			wallet := strings.ToLower(tx.Wallet)
			if err := dag.state.applyEscrowRecoverDeltaLocked(wallet, tx.Amount); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_recover %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow recovery %.6f AEQ → %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "slash_equivocation":
			// Idempotent balance deduction: only ONE slash_equivocation TX per
			// evidence pair ever succeeds — the CAS on equivocation_evidence
			// ensures all competing TXs (from multiple nodes that independently
			// detected the same offense) produce exactly one deduction.
			//
			// tx.Wallet = signer (signing address of the banned validator)
			// tx.To     = operator wallet address (receives the deduction)
			// tx.Amount = penalty in AEQ (≤ 50, capped at balance on detection)
			if tx.To == "" || tx.Amount <= 0 || tx.BlockAHash == "" || tx.BlockBHash == "" {
				fmt.Printf("[REPLAY] ⚠ slash_equivocation in block #%d: missing required fields — skipping\n", block.Height)
				continue
			}
			blockA, blockB := tx.BlockAHash, tx.BlockBHash
			if blockA > blockB {
				blockA, blockB = blockB, blockA
			}
			// Atomic CAS: insert or update the evidence row setting slash_applied=TRUE.
			// Rows affected > 0 → we won the race, apply the balance deduction.
			// Rows affected == 0 → already applied by a previous TX, skip.
			res, claimErr := dag.state.dbExec().Exec(
				`INSERT INTO equivocation_evidence
				     (signing_address, block_a_hash, block_b_hash, detected_at, slash_applied)
				 VALUES ($1, $2, $3, $4, TRUE)
				 ON CONFLICT (block_a_hash, block_b_hash) DO UPDATE
				     SET slash_applied = TRUE
				     WHERE equivocation_evidence.slash_applied = FALSE`,
				strings.ToLower(tx.Wallet), blockA, blockB, block.Timestamp,
			)
			if claimErr != nil {
				fmt.Printf("[REPLAY] ✗ slash_equivocation CAS failed (block #%d): %v — rolling back whole block\n", block.Height, claimErr)
				hardFailure = true
				continue
			}
			rows, _ := res.RowsAffected()
			if rows == 0 {
				fmt.Printf("[REPLAY] ℹ slash_equivocation for %s already applied — skipping duplicate (block #%d)\n", tx.Wallet, block.Height)
				continue
			}
			// Apply the balance deduction, capping at the wallet's current balance
			// in case it has shrunk since the TX was queued.
			opWallet := strings.ToLower(tx.To)
			penaltyAmt := tx.Amount
			if acc, ok := dag.state.accounts[opWallet]; ok && acc.Balance.Float() < penaltyAmt {
				penaltyAmt = acc.Balance.Float()
			}
			if penaltyAmt > 0 {
				if err := dag.state.applyTransferDeltaLocked(opWallet, ubiPoolAddr, penaltyAmt, 0, 0); err != nil {
					fmt.Printf("[REPLAY] ✗ slash_equivocation transfer %s→UBI %.4f: %v (block #%d) — rolling back whole block\n",
						tx.To, penaltyAmt, err, block.Height)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ✓ Applied slash_equivocation %.4f AEQ from %s → UBI pool (signer %s, block #%d)\n",
					penaltyAmt, tx.To, tx.Wallet, block.Height)
			}

		default:
			// FIX (audit 2026-06-28 recheck 4, P2-2): unknown TX types used to
			// be silently ignored — applied no delta, but also didn't reject
			// the block. That's a forward-compatibility hazard: a node
			// running OLDER code that doesn't yet recognize a NEW TX type
			// introduced by upgraded peers would silently skip that TX's
			// economic effect while still accepting the block as valid. The
			// post-replay StateRoot comparison below would usually catch
			// this (the skipped delta means local state can't match the
			// proposer's claimed root) — but relying on StateRoot alone to
			// catch a known, structural gap is exactly the "we believe it's
			// atomic" pattern this audit pass has been closing elsewhere.
			// Hard-fail explicitly instead: an unrecognized type is treated
			// the same as any other genuine state-inconsistency failure.
			fmt.Printf("[REPLAY] ✗ Unknown TX type %q (block #%d) — rolling back whole block\n", tx.Type, block.Height)
			hardFailure = true
			continue
		}
	}

	if hardFailure {
		commitOrRollback(false) // real SQL ROLLBACK — see commitOrRollback's comment
		if rbErr := dag.state.restoreFromRollbackLocked(rollbackSnap); rbErr != nil {
			fmt.Printf("[REPLAY] CRITICAL: rollback persistence failed for block #%d — memory/DB may now disagree: %v\n", block.Height, rbErr)
		}
		for _, n := range claimedNullifiers {
			dag.state.releaseNullifierLocked(n)
		}
		fmt.Printf("[REPLAY] ✗ Block #%d rolled back due to a genuine state-inconsistency failure — block rejected\n", block.Height)
		return false
	}

	// FIX (audit recheck 2, P0 #1): StateRoot comparison moved here (from
	// AddPeerBlock, after this function returned) so a mismatch can use
	// THIS function's own rollbackSnap to actually undo the replay, not
	// just log it. This used to be warn-only: the block was accepted into
	// dag.blocks/dag.tips regardless, meaning a node could build on top of
	// a block whose state it could not itself reproduce. Sequenced after
	// the distribution-atomicity and per-human demurrage-replay fixes
	// (audit recheck 2, P0 #2-#6) specifically because those were the
	// known, frequent divergence sources that made this check fire on
	// nearly every block in practice — rejecting on every block would have
	// halted sync entirely rather than catching genuine divergence.
	// Known residual divergence sources (non-atomic nullifier persistence,
	// mirror-path outbox — audit recheck 2, P1 #7/#8) can still trigger
	// this; a rejected block is retried by a later sync cycle once local
	// state catches up, the same recovery path hardFailure above already
	// relies on.
	if block.StateRoot != "" {
		// Computed BEFORE commit/rollback while dbTx is still open, so it
		// reflects exactly what this replay just wrote within dbTx.
		localRoot := dag.state.stateRootLocked(dag.state.getConfigValue("last_ubi_at"))
		if block.StateRoot != localRoot {
			// StateRoot mismatch is a WARNING, not a hard rejection.
			//
			// In a multi-validator DAG, concurrent sibling blocks (two validators
			// producing at the same height) will ALWAYS produce different StateRoots
			// when verified by a third node: collectUnreplayedAncestors walks only
			// the target block's own parent chain, not sibling branches, so sibling
			// TXs from other validators remain in the verifying node's accumulated
			// state and shift its root away from what the proposer computed.
			//
			// Treating this as a hard rejection causes the orphan cascade to stall
			// permanently: the rejected block soft-retries (5 min TTL), times out,
			// and is abandoned — taking every block built on top of it with it.
			//
			// The real security layer is individual TX verification, which runs
			// above and cannot be bypassed:
			//   - register_human: Groth16 ZK proof verified (line ~1739)
			//   - transfer/swap/liquidity: sender balance checked (hardFailure on
			//     insufficient funds)
			//   - unknown TX types: hardFailure (not silently skipped)
			//
			// StateRoot is a diagnostic: it detects accumulated-state drift between
			// nodes (visible in /api/health and the mismatch counter) without
			// blocking valid individually-verified transactions from entering the DAG.
			// For persistent divergence the correct recovery is RESYNC_FROM_SNAPSHOT
			// from a healthy peer — not silently skipping blocks.
			fmt.Printf("[REPLAY] ⚠ StateRoot mismatch on block #%d from %s (claimed=%s..., local=%s...) — accepted (TXs individually verified)\n",
				block.Height, block.Proposer, block.StateRoot[:min(16, len(block.StateRoot))], localRoot[:min(16, len(localRoot))])
			dag.stateRootMismatchesMu.Lock()
			dag.stateRootMismatches[block.Proposer]++
			dag.stateRootMismatchLastAt[block.Proposer] = time.Now().Unix()
			alert := dag.stateRootMismatches[block.Proposer] >= 5
			dag.stateRootMismatchesMu.Unlock()
			if alert {
				fmt.Printf("[ALERT] 5+ StateRoot mismatches from %s — nodes may have diverged; investigate or resync from primary snapshot\n", block.Proposer)
			}
			// FIX (P0, 2026-07-03 night, merge-reliability follow-up): record
			// this as a proposer-breaker STRIKE, not a clean success. See
			// lastReplayMismatchHash's own struct comment for the incident
			// this closes: without it, AddPeerBlock's tail unconditionally
			// called recordProposerOutcome(proposer, true) for every
			// successfully-attached block — including one that JUST got a
			// StateRoot-mismatch warning right here — permanently clearing
			// any strike this call recorded before the breaker could ever
			// accumulate toward its trip threshold. A validator whose EVERY
			// block mismatches (a genuinely diverged/isolated fork, not the
			// normal occasional sibling-drift this warning is designed to
			// tolerate) now actually accumulates toward the same temporary,
			// self-clearing 30s-cooldown breaker every other failure mode
			// already uses — no permanent denylist, no manual intervention:
			// if that validator ever resyncs and starts matching cleanly
			// again, the very next probe block clears it automatically.
			dag.recordProposerOutcome(block.Proposer, false)
			dag.replayMismatchMu.Lock()
			dag.lastReplayMismatchHash = block.Hash
			dag.replayMismatchMu.Unlock()
			// Fall through: commit the individually-verified transactions.
		} else {
			dag.stateRootMismatchesMu.Lock()
			dag.stateRootMismatches[block.Proposer] = 0 // reset on match
			dag.stateRootMismatchesMu.Unlock()
		}
	}

	// FIX (audit 2026-06-28 full recheck, P0-4): commit dbTx now that every
	// check has passed — this is the moment every DB write this replay made
	// actually becomes durable, all together, as one SQL transaction. If
	// commit itself fails (rare: connection loss, constraint violation the
	// DB only catches at commit time), Postgres has already rolled the
	// transaction back server-side — treat it exactly like any other
	// rollback path: restore in-memory state and reject the block, so
	// memory and DB can't end up disagreeing about whether this block
	// applied.
	if commitErr := commitOrRollback(true); commitErr != nil {
		if rbErr := dag.state.restoreFromRollbackLocked(rollbackSnap); rbErr != nil {
			fmt.Printf("[REPLAY] CRITICAL: rollback persistence failed for block #%d — memory/DB may now disagree: %v\n", block.Height, rbErr)
		}
		for _, n := range claimedNullifiers {
			dag.state.releaseNullifierLocked(n)
		}
		fmt.Printf("[REPLAY] ✗ Block #%d: replay transaction commit failed (rolled back, block rejected): %v\n", block.Height, commitErr)
		return false
	}

	dag.replayedMu.Lock()
	// FIX 1: Cap the cache to prevent unbounded growth (memory leak).
	// dag.blocks is the authoritative deduplication store; this is a fast-path cache.
	if len(dag.replayedBlocks) > 50000 {
		dag.replayedBlocks = make(map[string]bool, 1000)
	}
	dag.replayedBlocks[block.Hash] = true
	dag.replayedMu.Unlock()
	return true
}

// ReconstructState is a no-op: the PostgreSQL database is the authoritative
// source of truth and is already loaded by ChainState.LoadFromDB() before
// this is called. Ongoing state sync happens via replayRegistrations(), which
// is called from AddPeerBlock for every received block.
func (dag *BlockDAG) ReconstructState(state *ChainState) {
	fmt.Printf("[CHAIN] State loaded from DB — skipping full block-replay reconstruction\n")
}

// Close is a no-op now that replay runs synchronously inside AddPeerBlock
// (see its comment for why the old async channel+goroutine design was
// removed) — there's no longer a background goroutine to shut down. Kept
// for call-site compatibility (main.go may call this on shutdown).
func (dag *BlockDAG) Close() {}

// verifyZKProof reconstructs the Groth16 proof from the TX's decimal string
// fields and calls the BioVerifier contract via the local EVM engine to check
// validity. Returns true when the proof is valid, false otherwise.
// Only called when all four proof fields (ProofA, ProofB, ProofC, PubSignals)
// are present — blocks from old nodes omit them and fall back to trust-based
// validation for backward compatibility.
func (dag *BlockDAG) verifyZKProof(tx Transaction) bool {
	if dag.evm == nil {
		fmt.Printf("[WARN] EVM not initialized, rejecting ZK proof for block safety\n")
		return false
	}

	// Parse ProofA [2]*big.Int
	if len(tx.ProofA) != 2 || len(tx.ProofC) != 2 || len(tx.PubSignals) < 2 || len(tx.ProofB) != 2 {
		return false
	}
	var pA [2]*big.Int
	for i := 0; i < 2; i++ {
		n := new(big.Int)
		if _, ok := n.SetString(tx.ProofA[i], 10); !ok {
			fmt.Printf("[REPLAY] ✗ verifyZKProof: invalid ProofA[%d]: %q\n", i, tx.ProofA[i])
			return false
		}
		pA[i] = n
	}

	// Parse ProofB [2][2]*big.Int
	var pB [2][2]*big.Int
	for i := 0; i < 2; i++ {
		if len(tx.ProofB[i]) != 2 {
			fmt.Printf("[REPLAY] ✗ verifyZKProof: ProofB[%d] has wrong length\n", i)
			return false
		}
		for j := 0; j < 2; j++ {
			n := new(big.Int)
			if _, ok := n.SetString(tx.ProofB[i][j], 10); !ok {
				fmt.Printf("[REPLAY] ✗ verifyZKProof: invalid ProofB[%d][%d]: %q\n", i, j, tx.ProofB[i][j])
				return false
			}
			pB[i][j] = n
		}
	}

	// Parse ProofC [2]*big.Int
	var pC [2]*big.Int
	for i := 0; i < 2; i++ {
		n := new(big.Int)
		if _, ok := n.SetString(tx.ProofC[i], 10); !ok {
			fmt.Printf("[REPLAY] ✗ verifyZKProof: invalid ProofC[%d]: %q\n", i, tx.ProofC[i])
			return false
		}
		pC[i] = n
	}

	// Parse PubSignals [2]*big.Int (only first two are needed by verifyProof)
	var pubSignals [2]*big.Int
	for i := 0; i < 2; i++ {
		n := new(big.Int)
		if _, ok := n.SetString(tx.PubSignals[i], 10); !ok {
			fmt.Printf("[REPLAY] ✗ verifyZKProof: invalid PubSignals[%d]: %q\n", i, tx.PubSignals[i])
			return false
		}
		pubSignals[i] = n
	}

	verifierABI, err := abi.JSON(strings.NewReader(bioVerifierABI))
	if err != nil {
		fmt.Printf("[REPLAY] ✗ verifyZKProof: ABI parse failed: %v\n", err)
		return false
	}
	verifyData, err := verifierABI.Pack("verifyProof", pA, pB, pC, pubSignals)
	if err != nil {
		fmt.Printf("[REPLAY] ✗ verifyZKProof: ABI encode failed: %v\n", err)
		return false
	}

	caller := common.HexToAddress(tx.Wallet)
	ret, err := dag.evm.CallContract(caller, common.HexToAddress(BIO_VERIFIER_ADDR), verifyData, big.NewInt(0), false)
	if err != nil {
		fmt.Printf("[REPLAY] ✗ verifyZKProof: BioVerifier call failed: %v\n", err)
		return false
	}
	if len(ret) != 32 || ret[31] != 1 {
		return false
	}
	return true
}

// softRetryTTL is how long a soft-retry block stays in the queue before
// being abandoned.  Five minutes comfortably exceeds any reasonable window
// during which a sibling block could arrive and unblock the failed replay.
const softRetryTTL = 5 * time.Minute

// epochLength is how many blocks form one epoch. At 1 s/block this is 1 hour.
// At every epoch boundary the active-producer committee is re-selected from
// all registered node operators, keeping simultaneous block producers bounded
// at targetCommitteeSize regardless of how many humans have registered.
const epochLength = int64(3600)

// targetCommitteeSize is the number of node operators selected per epoch when
// enough are registered. 100 producers at K=18 still gives a healthy blue-set
// ratio — K should grow with the committee in a later consensus upgrade.
const targetCommitteeSize = 100

// maxCommitteeSize hard-caps the epoch committee. Beyond this point the
// dynamic-K upgrade is needed before growing the committee further.
const maxCommitteeSize = 10_000

// EpochCommittee holds the active block-producer set for one epoch.
// Selected by ranking all registered node operators by
// sha256(lower(addr)+":"+epochNum) and taking the top N — fully
// deterministic, no external randomness or coordination needed.
type EpochCommittee struct {
	Number  int64
	Members map[string]bool // lower-cased signing addresses of active producers
	Size    int
}

// computeEpochCommittee builds the committee for epochNum from the live
// authorizedValidators map (which always contains this node's own signing key
// plus every peer validator discovered via registration). Must be called while
// dag.mu is already held by the caller (ProduceBlock holds it; getEpochCommittee
// is only invoked from there).
//
// Using authorizedValidators instead of validator_keys/validator_slots avoids
// the critical bug where the primary never registers with itself: its own
// signing address is absent from both DB tables, so a DB-based query would
// exclude it from its own committee, halting all primary block production.
func (dag *BlockDAG) computeEpochCommittee(epochNum int64) *EpochCommittee {
	allOps := make([]string, 0, len(dag.authorizedValidators))
	for addr := range dag.authorizedValidators {
		allOps = append(allOps, addr)
	}
	sort.Strings(allOps) // deterministic ordering before scoring
	if len(allOps) == 0 {
		return nil // no validators known yet → everyone can produce (bootstrap)
	}

	size := targetCommitteeSize
	if len(allOps) < size {
		size = len(allOps) // fewer operators than target → all are in committee
	}
	if size > maxCommitteeSize {
		size = maxCommitteeSize
	}

	type entry struct {
		addr  string
		score [32]byte
	}
	entries := make([]entry, len(allOps))
	for i, addr := range allOps {
		entries[i].addr = strings.ToLower(addr)
		entries[i].score = sha256.Sum256([]byte(fmt.Sprintf("%s:%d", entries[i].addr, epochNum)))
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].score[:], entries[j].score[:]) < 0
	})
	if len(entries) > size {
		entries = entries[:size]
	}
	members := make(map[string]bool, len(entries))
	for _, e := range entries {
		members[e.addr] = true
	}
	return &EpochCommittee{Number: epochNum, Members: members, Size: len(members)}
}

// getEpochCommittee returns the cached committee for the epoch that contains
// height, recomputing it (and logging the transition) only when the epoch
// number changes. Safe to call without dag.mu held.
func (dag *BlockDAG) getEpochCommittee(height int64) *EpochCommittee {
	epochNum := height / epochLength

	dag.epochMu.RLock()
	if dag.currentEpoch != nil && dag.currentEpoch.Number == epochNum {
		ec := dag.currentEpoch
		dag.epochMu.RUnlock()
		return ec
	}
	dag.epochMu.RUnlock()

	ec := dag.computeEpochCommittee(epochNum)

	dag.epochMu.Lock()
	if dag.currentEpoch == nil || dag.currentEpoch.Number != epochNum {
		dag.currentEpoch = ec
		if ec != nil {
			newK := ghostdagKBase
			if ec.Size/3 > newK {
				newK = ec.Size / 3
			}
			dag.activeGhostdagK.Store(int32(newK))
			role := "observer"
			if ec.Members[dag.selfProposer] {
				role = "producer"
			}
			fmt.Printf("[EPOCH] Epoch %d (height %d): committee=%d validators, K=%d, self=%s (%s)\n",
				epochNum, height, ec.Size, newK, dag.selfProposer, role)
		}
	} else {
		ec = dag.currentEpoch
	}
	dag.epochMu.Unlock()
	return ec
}

// ghostdagKBase is the minimum K used on a near-empty network. Once the
// active-producer committee exceeds 3*ghostdagKBase validators the epoch
// boundary raises K to committeeSize/3 so the blue-set ratio stays healthy.
// All nodes compute the same K from the same deterministic committee, so
// this is a safe consensus-layer change with no manual coordination.
const ghostdagKBase = 18

// activeGhostdagK is the live K for the current epoch, stored as an atomic
// so GHOSTDAG computations (called under dag.mu) can read it without a
// separate lock. Updated by getEpochCommittee at every epoch transition.
// Defaults to ghostdagKBase until the first committee is computed.
// dag.k() is the accessor — use it everywhere instead of ghostdagKBase.
func (dag *BlockDAG) k() int {
	v := int(dag.activeGhostdagK.Load())
	if v < ghostdagKBase {
		return ghostdagKBase
	}
	return v
}

// maxParents scales with K so the merge-set stays tractable as the committee
// grows: at K=18 the minimum is 64, for large committees it becomes 2*K.
func (dag *BlockDAG) maxParents() int {
	v := 2 * dag.k()
	if v < maxParentsPerBlock {
		return maxParentsPerBlock
	}
	return v
}

// maxMergeVisits bounds how many blocks the merge-set BFS visits and how many
// get blue/red-classified. It must be at least the number of blocks that can
// be produced CONCURRENTLY (all of them land in one block's merge set in the
// worst case), which is the committee size ≈ 3*K (K is set to committeeSize/3
// in getEpochCommittee). It must NOT grow faster than that: classification is
// roughly O(visits^2), so an over-large cap turns a burst into a multi-second
// stall (confirmed by block_ghostdag_scale_test at cap 185). 3*K tracks the
// real concurrency; the floor of 50 preserves small-network behaviour, where
// K stays at its base and merge sets are single digits anyway.
func (dag *BlockDAG) maxMergeVisits() int {
	// Exactly the historical constant (50) at base K, so small networks keep
	// their proven timing; grow by ~3 per K above base to track committee-size
	// concurrency without the O(visits^2) blowup a steeper curve caused.
	extra := dag.k() - ghostdagKBase
	if extra <= 0 {
		return 50
	}
	return 50 + 3*extra
}

// maxParentsPerBlock caps how many tips ProduceBlock includes as parents.
// Without a cap, 1000 simultaneous validators produce 1000 tips → a single
// block carries 40 KB of parent hashes and GHOSTDAG merge-set computation
// explodes.  64 parents cover all validators up to a few hundred; the best
// (highest-BlueScore) tips are selected so no validator's work is unfairly
// excluded when the cap bites.
const maxParentsPerBlock = 64

// dagPruneBuffer is how many block-heights above the finalized checkpoint
// dag.blocks keeps in RAM. ghostdagMergeDepth = 2*K+1 hops back; at K=333
// (1000-validator committee) that is 667 hops, so we keep 5× = 3350. The
// buffer scales with K at runtime via dag.pruneBuffer().
// Pruned blocks are never deleted from the DB (chain_blocks).
const dagPruneBufferBase = 200

// pruneBuffer returns the DAG in-memory retention window scaled to current K:
// max(dagPruneBufferBase, 5*(2*K+1)) so GHOSTDAG ancestor walks never reach
// a pruned block.
func (dag *BlockDAG) pruneBuffer() int64 {
	k := dag.k()
	need := int64(5 * (2*k + 1))
	if need < dagPruneBufferBase {
		return dagPruneBufferBase
	}
	return need
}

// startupLoadWindow: on startup, load only the most-recent N blocks from
// chain_blocks into dag.blocks. bootHeight (derived from the DB's
// max_block_height config entry) guards against re-replay of older blocks
// even though they are not in memory.  2000 >> dagPruneBuffer so the live
// pruning loop is the binding constraint, not the startup load.
const startupLoadWindow = 2000

// mergeDepthLimit returns 2*K+1: the maximum parent-hops ghostdagMergeSet
// and ghostdagIsAncestor will walk. Scales with the live K so large-committee
// epochs never truncate valid merge sets. Must be called while dag.mu is held.
func (dag *BlockDAG) mergeDepthLimit() int {
	return 2*dag.k() + 1
}

// computeGHOSTDAGState computes true GHOSTDAG state for a block and stores
// the result in block.SelectedParent, block.Blues, and block.BlueScore.
// Must be called under dag.mu.
//
// Real GHOSTDAG (Sompolinsky-Zohar 2018):
//   - SelectedParent = parent with highest blue score
//   - MergeSet       = past(B) ∩ anticone(SelectedParent)  — the "merged" branches
//   - Blues          = merge-set blocks whose anticone contains ≤K blue blocks
//   - BlueScore      = blueScore(SP) + 1 + |Blues|
//
// Every node that holds the same block graph computes identical GHOSTDAG state,
// so the canonical ordering (height ASC, blueScore DESC, hash ASC) is
// deterministic across the network and StateRoots are reproducible.
func (dag *BlockDAG) computeGHOSTDAGState(block *Block) {
	if block.IsGenesis || len(block.ParentHashes) == 0 {
		block.SelectedParent = ""
		block.Blues = nil
		block.BlueScore = 0
		return
	}

	// FIX (P0, 2026-07-04): one DB-roundtrip budget shared for this entire
	// computation — see maxGhostdagDBLookups' own comment.
	dbBudget := dag.maxGhostdagDBLookups()

	// Step 1: selected parent = highest-blue-score parent.
	// Batch-fetch any parents missing from dag.blocks in ONE round trip
	// before the loop below, instead of one round trip per missing parent —
	// see ghostdagBatchPrefetch's own comment for the live perf finding this
	// closes.
	dag.ghostdagBatchPrefetch(block.ParentHashes, &dbBudget)
	var maxScore int64 = -1
	spHash := block.ParentHashes[0]
	for _, ph := range block.ParentHashes {
		if p := dag.ghostdagBlockLookup(ph, &dbBudget); p != nil && p.BlueScore > maxScore {
			maxScore = p.BlueScore
			spHash = ph
		}
	}
	block.SelectedParent = spHash

	// Step 2: compute merge set — blocks in past(B) that are NOT in past(SP).
	// Bounded BFS: we walk back from non-SP parents, collecting blocks that
	// are not reachable from SP.  The depth limit (2K+1) bounds the search;
	// for an honest ≤3-validator network merge sets are always ≤2 blocks.
	mergeSet := dag.ghostdagMergeSet(block, spHash, &dbBudget)

	// Step 3: topological sort of the merge set (parents before children).
	sorted := ghostdagTopoSort(mergeSet, dag.blocks)

	// SAFETY VALVE (scale audit): depthLimit bounds merge-set DEPTH but not
	// BREADTH — a burst of many validators producing concurrently for
	// several rounds with no convergence (e.g. right after a network
	// partition heals, or simply many more validators than this was
	// originally sized for) can still produce a merge set with hundreds or
	// thousands of entries, each requiring anticone comparisons against
	// every other. Classification cost is bounded per-block by the K
	// early-break below, but the OUTER loop over `sorted` is still
	// O(len(sorted)) and a sufficiently large burst could still make a
	// single block's GHOSTDAG computation take unacceptably long while
	// holding dag.mu — confirmed via block_ghostdag_scale_test.go.  Cap the
	// number of merge-set blocks actually classified, same bounding
	// philosophy as maxOrphans elsewhere in this file: the closest-to-
	// SelectedParent blocks (earliest in topological order) are kept as
	// blue candidates, the (rare, burst-only) remainder is treated as red
	// without spending classification work on it. This is a liveness
	// backstop, not expected to trigger under normal multi-validator
	// operation — if it logs routinely in production, that means real
	// gossip propagation latency needs investigating, not this cap.
	maxClassifiedMergeSetSize := dag.maxMergeVisits()
	if len(sorted) > maxClassifiedMergeSetSize {
		fmt.Printf("[GHOSTDAG] ⚠ merge set size %d for block %s exceeds classification cap %d — classifying only the %d blocks closest to SelectedParent, remainder treated as red. This indicates an extreme concurrent-production burst; investigate gossip/sync latency if this recurs.\n",
			len(sorted), block.Hash, maxClassifiedMergeSetSize, maxClassifiedMergeSetSize)
		sorted = sorted[:maxClassifiedMergeSetSize]
	}

	// Step 4: blue / red classification.
	// A merge-set block M is blue if the number of already-blue merge-set
	// blocks in M's anticone is ≤ K.
	//
	// FIX (scale audit): break out of the inner loop as soon as antiCnt
	// exceeds K — M is definitely red at that point per the K-cluster rule
	// itself (anything with more than K blue blocks in its anticone is red,
	// full stop), so counting the rest of `blues` cannot change the outcome.
	// The original always scanned the entire `blues` slice for every M,
	// making this loop O(|mergeSet| x |blues|) unconditionally. Under
	// honest <=3-validator concurrency (the only scale this was exercised
	// at — see ghostdagK's comment) `blues` never grows past a couple of
	// entries so the difference is invisible; under realistic 100-validator
	// concurrent production almost every M in a large merge set has dozens
	// of true-anticone siblings, so this turns most classifications into an
	// O(K) early-exit instead of an O(|blues|) full scan. One of several
	// fixes needed together (see ghostdagMergeSet, ghostdagIsAncestor, and
	// the merge-set-size safety valve below) to bring 100 validators x 30
	// rounds of full concurrent merges from "does not finish in 120s" down
	// to ~41s in block_ghostdag_scale_test.go's deliberately worst-case
	// (zero convergence for 30 straight rounds) scenario — comfortably
	// inside a 6s-per-block production cadence in practice, since real
	// merge sets stay small once validators see each other's blocks.
	blues := make([]string, 0, len(sorted))
	blueScore := maxScore + 1 // SP always contributes +1

	for _, mHash := range sorted {
		antiCnt := 0
		isBlue := true
		for _, bHash := range blues {
			// bHash is in M's anticone iff they are concurrent (neither is
			// an ancestor of the other).
			if !dag.ghostdagIsAncestor(bHash, mHash, &dbBudget) && !dag.ghostdagIsAncestor(mHash, bHash, &dbBudget) {
				antiCnt++
				if antiCnt > dag.k() {
					isBlue = false
					break
				}
			}
		}
		if isBlue {
			blues = append(blues, mHash)
			blueScore++
		}
	}

	block.Blues = blues
	block.BlueScore = blueScore
}

// ghostdagMergeSet returns the set of blocks in past(block) that are NOT in
// past(spHash), using a bounded BFS (depth ≤ 2K+1 from each non-SP parent).
// Must be called under dag.mu.
//
// FIX (scale audit): both walks below mark a hash as seen at ENQUEUE time,
// not at dequeue/pop time. The original code (a DFS stack) only recorded a
// hash into excluded/mergeSet when it was POPPED, so any hash reachable via
// more than one path got pushed once per distinct path — at low validator
// counts/merge-set sizes (the only scale this was ever exercised at; see
// ghostdagK's own comment) the number of distinct paths to a shared ancestor
// is tiny and this is unnoticeable. At realistic 100-validator concurrent
// production (every block merging dozens of prior tips every round) the
// number of distinct paths to a common ancestor grows combinatorially with
// validator count, and the duplicate-push volume blew up accordingly:
// confirmed via block_ghostdag_scale_test.go, 100 validators x 30 rounds of
// full concurrent merges did not complete computeGHOSTDAGState within 120s.
// Marking visited at enqueue time (standard BFS dedup) means each hash is
// pushed at most once, bounding total work by reachable-set size instead of
// path count.
// maxMergeSetBFSVisits bounds the total number of DISTINCT blocks either BFS
// below will visit, and (via maxClassifiedMergeSetSize, which is defined as
// this same constant) how many of them computeGHOSTDAGState's blue/red
// classification loop processes. ghostdagMergeDepthLimit alone bounds path
// LENGTH, not the number of distinct blocks reachable within that length —
// it provides NO restriction at all for any chain shorter than the depth
// limit, which is exactly the common early-life-of-a-chain case. Measured
// directly (block_ghostdag_scale_test.go's profiling harness, all well
// inside any depth limit): with full concurrent merging every round,
// 20 validators x 20 rounds (401 blocks total) already took ~16s, 20 x 30
// (601 blocks) ~39s, 30 x 20 (601 blocks) ~99s — because both the merge-set
// BFS and the classification loop scale with however many blocks have
// accumulated so far, and every block pays that full cost independently.
// This bound exists so a single block's GHOSTDAG computation has a hard
// ceiling on cost regardless of how large the network or how sustained a
// non-converging burst is: once hit, the BFS/classification simply stop
// (excess blocks are conservatively treated as outside the merge set / red)
// — the same graceful-degradation-over-collapse philosophy as maxOrphans
// elsewhere in this file.
//
// This isn't just a per-call bound — it's effectively multiplied by how
// many times computeGHOSTDAGState's classification loop calls
// ghostdagIsAncestor for a single block (up to roughly
// 2 x maxClassifiedMergeSetSize x K comparisons), and the common case under
// real concurrent production is exactly the expensive one: true siblings
// are NOT ancestors of each other, so an "is ancestor" query against a
// sibling must exhaust its full search budget before concluding "no" every
// time. 5000, then 300, both still left aggregate per-block cost in the
// tens-of-seconds range at 100-validator scale (confirmed via
// block_ghostdag_scale_test.go); 50 keeps a single block's total
// classification cost low even under sustained non-convergence. Under
// normal operation (validators converging within a few rounds, as real
// gossip propagation within a ~6s block interval should achieve) actual
// merge sets are tiny — typically single digits — and this never triggers.
// maxMergeSetBFSVisits floor (50) — actual limit computed by dag.maxMergeVisits() = max(50, 5*(2K+1))

// maxGhostdagDBLookups bounds the number of REAL (cache-miss) database round
// trips a SINGLE computeGHOSTDAGState call may make in total, shared across
// its merge-set BFS (ghostdagMergeSet) AND every ghostdagIsAncestor call the
// blue/red classification loop makes on top of it.
//
// FIX (durable fix, 2026-07-04 — closes cross-node blue_score/canonical-
// choice divergence found live): this used to be a fixed constant (10),
// introduced the same night to stop a single computeGHOSTDAGState call from
// making unbounded DB round trips over Railway's slow external proxy
// (~200ms-1.2s/call there, confirmed live at 62s for one call). That fix
// traded a real outage risk for a subtler one: whether a given ancestor
// lookup succeeds (cache hit) or costs a real round trip (cache miss, spends
// budget) depends on THIS node's own, incidental cache-warmth at THIS
// moment — restart history, pruning timing, concurrent load all differ
// node-to-node. Two honest nodes computing the identical block's GHOSTDAG
// state could therefore each hit the SAME fixed budget at a DIFFERENT point
// in their own BFS, silently truncating the merge set differently and
// computing a different BlueScore/SelectedParent for the same block —
// confirmed live: the primary and Contabo 1 disagreed on both hash and
// proposer at a settled height (173000) despite the migration fix and a
// clean resync, with no other explanation fitting. That is exactly the
// determinism this file's header comment promises ("every node that holds
// the same block graph computes identical GHOSTDAG state") silently broken.
//
// The DB latency problem that justified a tight budget is gone: every node
// in this network now has fast DB access (the primary moved off Railway's
// public proxy onto its private network, ~<5ms; both Contabo nodes were
// always local-Postgres, ~sub-ms) — the 200ms-plus-per-call cost this budget
// was bounding no longer exists anywhere in the fleet. Scaling the budget
// with maxMergeVisits (the existing, already-deterministic, K-derived
// structural cap) instead of a fixed number means the DB budget can only
// ever be the limiting factor in a genuinely pathological burst far beyond
// maxMergeVisits' own ceiling — in the normal case, the structural caps
// (mergeDepthLimit, maxMergeVisits), which are identical on every node by
// construction, are what actually bound the computation, not incidental
// per-node cache state. At sub-5ms round trips, even this much larger
// budget costs low hundreds of milliseconds in the worst case, comfortably
// inside the 2s BLOCK_TIME window. No separate floor needed: maxMergeVisits
// already has its own floor (50), so this is never below 500.
func (dag *BlockDAG) maxGhostdagDBLookups() int {
	return dag.maxMergeVisits() * 10
}

// ghostdagBlockLookup returns the block for hash, falling back to the DB when
// it is not resident in dag.blocks. Without this, computeGHOSTDAGState and its
// helpers (below) treat any ancestor that merely isn't in RAM right now as if
// it had no further parents, silently truncating the BFS based on what
// happens to be loaded on THIS node at THIS moment — not the true DAG
// structure. That breaks the determinism this file's header comment promises
// ("every node that holds the same block graph computes identical GHOSTDAG
// state"): pruneOldDAGBlocks keeps dag.blocks within pruneBuffer() of the
// finalized checkpoint during steady-state operation, but a restart's
// startupLoadWindow only loads the most recent 2000 blocks regardless of how
// far the checkpoint lags — and a GHOSTDAG merge set can reach a much older
// ancestor in very few hops when a validator's tip carries a big single-hop
// height jump (normal here: a validator merging back in after falling behind
// routinely references a merge-parent 100+ heights back in one parent link).
// Two nodes with different restart histories can then silently compute
// different SelectedParent/BlueScore for the identical, hash-verified block —
// confirmed as the mechanism behind a real production fork (2026-07-03,
// Contabo 1 vs Primary diverging from height ~132664 while each side's own
// history looked internally healthy). Same fallback pattern already used for
// the finality checkpoint walk (see finishCheckpointWalkFromDB); mirrored
// here because this path is even more consensus-critical — it runs on every
// block's ingestion, not just periodically. Found blocks are cached back into
// dag.blocks so every other raw dag.blocks[hash] read later in the same
// computation (ghostdagTopoSort's allBlocks parameter, in particular) also
// sees them; pruneOldDAGBlocks evicts them again in its next normal sweep
// like any other block, so this cannot leak memory. Must be called under
// dag.mu, same contract as its callers.
func (dag *BlockDAG) ghostdagBlockLookup(hash string, budget *int) *Block {
	if b, ok := dag.blocks[hash]; ok {
		return b
	}
	// FIX (P0, 2026-07-04 — real production outage): skip the DB round trip
	// during the startup GHOSTDAG migration (ghostdagMigrationPending==true).
	// That migration recomputes scores for a BOUNDED, already-loaded batch of
	// blocks this node already holds canonically (headers hash/signature-
	// verified when first accepted) -- it is a local backfill of a derived
	// field, not a live attach/reject decision, so it does not need the same
	// cross-node determinism guarantee this DB fallback exists for elsewhere.
	// Confirmed live: with the fallback active here, a migration over ~5,000
	// blocks made a synchronous DB call (holding dag.mu, serializing
	// ProduceBlock/AddPeerBlock/every API read behind it) for every ancestor
	// outside the loaded batch — hundreds of round trips over Railway's DB
	// proxy, each tens to hundreds of ms, froze the node solid (ProduceBlock
	// measured 10+ seconds, HTTP requests timed out entirely) for many
	// minutes with the migration barely progressing. AddPeerBlock and
	// ProduceBlock both already refuse to do anything for the whole duration
	// of a migration (see their own ghostdagMigrationPending gates), so
	// ghostdagBlockLookup's only caller while this flag is true is the
	// migration loop itself -- falling back to the pre-DB-fallback behavior
	// (treat a not-yet-loaded ancestor as absent) here is exactly as safe as
	// it always was for that loop, and turns an unbounded number of blocking
	// DB round trips into zero.
	if dag.ghostdagMigrationPending.Load() {
		return nil
	}
	if dag.state == nil {
		return nil
	}
	// FIX (P0, 2026-07-04 — second production outage, same class): budget
	// shared across an entire computeGHOSTDAGState call bounds total real DB
	// round trips regardless of merge-set size or classification fan-out —
	// see maxGhostdagDBLookups's own comment. nil means unbounded,
	// for callers outside that call graph. Checked here (after the free
	// migration/nil-state short-circuits, right before the actual round
	// trip) so it only ever counts real DB calls, never a check that would
	// have returned nil anyway.
	if budget != nil {
		if *budget <= 0 {
			return nil
		}
		*budget--
	}
	b := dag.state.LoadBlockFromDBByHash(hash)
	if b != nil {
		dag.blocks[hash] = b
	}
	return b
}

func (dag *BlockDAG) ghostdagMergeSet(block *Block, spHash string, dbBudget *int) map[string]bool {
	depthLimit := dag.mergeDepthLimit()
	visitCap := dag.maxMergeVisits()

	// Build a shallow exclusion set: blocks definitely reachable from SP.
	//
	// FIX (2026-07-04, live perf finding): both BFS loops below now drain
	// and process the queue in ROUNDS (one full frontier at a time) instead
	// of popping and looking up one entry at a time, batch-prefetching each
	// round's hashes in a single round trip first — see
	// ghostdagBatchPrefetch's own comment for why. The traversal order and
	// every cap/limit check is unchanged; only how the DB is hit differs.
	type entry struct {
		hash  string
		depth int
	}
	excluded := map[string]bool{spHash: true}
	queue := []entry{{spHash, 0}}
	for len(queue) > 0 && len(excluded) < visitCap {
		round := queue
		queue = nil
		roundHashes := make([]string, len(round))
		for i, e := range round {
			roundHashes[i] = e.hash
		}
		dag.ghostdagBatchPrefetch(roundHashes, dbBudget)
		for _, cur := range round {
			if len(excluded) >= visitCap {
				break
			}
			if cur.depth > depthLimit {
				continue
			}
			if b := dag.ghostdagBlockLookup(cur.hash, dbBudget); b != nil {
				for _, ph := range b.ParentHashes {
					if !excluded[ph] {
						excluded[ph] = true
						queue = append(queue, entry{ph, cur.depth + 1})
						if len(excluded) >= visitCap {
							break
						}
					}
				}
			}
		}
	}

	// BFS backward from non-SP parents, stopping at excluded blocks.
	mergeSet := make(map[string]bool, len(block.ParentHashes))
	queue = queue[:0]
	for _, ph := range block.ParentHashes {
		if ph != spHash && !excluded[ph] && !mergeSet[ph] {
			mergeSet[ph] = true
			queue = append(queue, entry{ph, 0})
		}
	}
	for len(queue) > 0 {
		if len(mergeSet) >= visitCap {
			fmt.Printf("[GHOSTDAG] ⚠ merge-set BFS for block %s hit the %d-node visit cap — treating remaining reachable ancestors as outside the merge set. Extreme concurrent-production burst; investigate gossip/sync latency if this recurs.\n", block.Hash, visitCap)
			break
		}
		round := queue
		queue = nil
		roundHashes := make([]string, len(round))
		for i, e := range round {
			roundHashes[i] = e.hash
		}
		dag.ghostdagBatchPrefetch(roundHashes, dbBudget)
		for _, cur := range round {
			if len(mergeSet) >= visitCap {
				fmt.Printf("[GHOSTDAG] ⚠ merge-set BFS for block %s hit the %d-node visit cap — treating remaining reachable ancestors as outside the merge set. Extreme concurrent-production burst; investigate gossip/sync latency if this recurs.\n", block.Hash, visitCap)
				break
			}
			if cur.depth > depthLimit {
				continue
			}
			if b := dag.ghostdagBlockLookup(cur.hash, dbBudget); b != nil {
				for _, ph := range b.ParentHashes {
					if !excluded[ph] && !mergeSet[ph] {
						mergeSet[ph] = true
						queue = append(queue, entry{ph, cur.depth + 1})
						if len(mergeSet) >= visitCap {
							break
						}
					}
				}
			}
		}
	}
	return mergeSet
}

// ghostdagBatchPrefetch loads every hash in `hashes` that is not already
// resident in dag.blocks with a SINGLE database round trip
// (LoadBlocksByHashesFromDB), instead of the one-round-trip-per-hash cost
// that ghostdagBlockLookup pays when called individually. Counts as at most
// one unit against dbBudget regardless of how many hashes it fetches —
// dbBudget bounds ROUND TRIPS (wall-clock DB latency), not rows, see
// maxGhostdagDBLookups's own comment, so batching is a strict
// improvement: the same budget now covers more ground per round trip spent.
//
// FIX (P0, 2026-07-04 — live perf finding, root cause of primary block
// production regularly exceeding the 2s BLOCK_TIME target): confirmed live
// that computeGHOSTDAGState's "ghostdag+hash" phase cost a strikingly
// consistent ~2.6s on nearly every block — almost exactly
// maxGhostdagDBLookupsPerBlock(10) x the ~260ms per-query latency measured
// elsewhere in the same incident over Railway's Postgres proxy. The
// "[GHOSTDAG] merge-set BFS hit the visit cap" warning was firing routinely
// under completely normal 3-validator production (not just extreme
// bursts), meaning merge sets routinely reach 50+ distinct blocks — each
// one, when missing from dag.blocks, paying its own sequential DB round
// trip. Fetching a whole BFS frontier in one query collapses that to a
// single round trip per level instead of one per missing block.
func (dag *BlockDAG) ghostdagBatchPrefetch(hashes []string, dbBudget *int) {
	if dag.state == nil || dag.ghostdagMigrationPending.Load() {
		return
	}
	if dbBudget != nil && *dbBudget <= 0 {
		return
	}
	var missing []string
	for _, h := range hashes {
		if _, ok := dag.blocks[h]; !ok {
			missing = append(missing, h)
		}
	}
	if len(missing) == 0 {
		return
	}
	if dbBudget != nil {
		*dbBudget--
	}
	found, err := dag.state.LoadBlocksByHashesFromDB(missing)
	if err != nil {
		return
	}
	for _, b := range found {
		dag.blocks[b.Hash] = b
	}
}

// ghostdagIsAncestor returns true if ancestorHash can reach descendantHash
// by following parent links, searching no more than ghostdagMergeDepthLimit
// hops back. Used ONLY for anticone detection between two members of a
// bounded merge set (computeGHOSTDAGState) — see ghostdagMergeDepthLimit's
// comment for why a query between two such blocks never needs to look
// further than that bound. Must be called under dag.mu.
//
// FIX (scale audit, same class as ghostdagMergeSet above): two changes vs.
// the original:
//  1. Mark a hash visited at ENQUEUE time, not dequeue time. The original
//     pushed every parent of every dequeued block unconditionally (visited
//     was only checked/set after a pop), so a hash reachable via N distinct
//     paths was enqueued N times before the dedup check ever fired —
//     harmless at the small, low-fan-out merge sets this was exercised at
//     (see ghostdagK's comment), but compounds badly at realistic
//     100-validator merge-set sizes since this runs inside
//     computeGHOSTDAGState's classification loop.
//  2. Bound the walk to ghostdagMergeDepthLimit hops. The original had NO
//     depth limit at all — a "not an ancestor" answer (the common case
//     under concurrent production, where most candidates are true siblings)
//     required exhausting the ENTIRE reachable past, i.e. cost scaled with
//     total chain height, not merge-set size. Confirmed via
//     block_ghostdag_scale_test.go: at chain depth 20,000 a burst of just 50
//     concurrent blocks was dominated by this cost.
//  3. Bound total nodes visited (breadth), same cap as maxMergeSetBFSVisits.
//     Depth alone only restricts PATH LENGTH; for any chain shorter than
//     ghostdagMergeDepthLimit (the common case — 37 hops is a lot of blocks)
//     the depth bound is vacuous and a single query's BREADTH is what
//     matters: at high validator fan-out, "is X an ancestor of Y" walks
//     toward the entire accumulated graph before concluding "no". This
//     function is called up to ~2 x K times per merge-set member from
//     computeGHOSTDAGState's classification loop, so an unbounded-breadth
//     answer here defeats every other cap in this file — confirmed live:
//     even with ghostdagMergeSet's own 300-visit cap in place, 100
//     validators x 30 rounds still did not finish within 60s because this
//     function, called from inside that loop, was independently re-walking
//     the full graph on every single call. Once the cap is hit, conclude
//     "not an ancestor" (the same conservative direction as every other cap
//     here: under uncertainty, bias toward classifying more blocks red
//     rather than risking an incorrect blue).
func (dag *BlockDAG) ghostdagIsAncestor(ancestorHash, descendantHash string, dbBudget *int) bool {
	if ancestorHash == descendantHash {
		return true
	}
	type entry struct {
		hash  string
		depth int
	}
	visitCap := dag.maxMergeVisits()
	visited := map[string]bool{descendantHash: true}
	queue := []entry{{descendantHash, 0}}
	for len(queue) > 0 {
		if len(visited) >= visitCap {
			return false
		}
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= dag.mergeDepthLimit() {
			continue
		}
		if b := dag.ghostdagBlockLookup(cur.hash, dbBudget); b != nil {
			for _, ph := range b.ParentHashes {
				if ph == ancestorHash {
					return true
				}
				if !visited[ph] {
					visited[ph] = true
					queue = append(queue, entry{ph, cur.depth + 1})
					if len(visited) >= visitCap {
						return false
					}
				}
			}
		}
	}
	return false
}

// ghostdagTopoSort returns merge-set blocks in topological order (parents
// before children) using DFS reverse-postorder.  Deterministic: keys sorted
// before DFS so two nodes with the same block set always produce the same order.
func ghostdagTopoSort(subset map[string]bool, allBlocks map[string]*Block) []string {
	visited := make(map[string]bool)
	result := make([]string, 0, len(subset))
	var dfs func(h string)
	dfs = func(h string) {
		if visited[h] {
			return
		}
		visited[h] = true
		if b, ok := allBlocks[h]; ok {
			for _, ph := range b.ParentHashes {
				if subset[ph] {
					dfs(ph)
				}
			}
		}
		result = append(result, h)
	}
	keys := make([]string, 0, len(subset))
	for h := range subset {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	for _, h := range keys {
		dfs(h)
	}
	return result
}

// pruneOldDAGBlocks evicts from dag.blocks all non-tip blocks below
// (finalizedHeight - dagPruneBuffer). The DB (chain_blocks) retains the
// full history. ghostdagIsAncestor / ghostdagMergeSet walk at most
// ghostdagMergeDepthLimit = 2*K+1 hops back; dagPruneBuffer >> that
// depth, so pruned blocks never appear in live GHOSTDAG queries.
// collectUnreplayedAncestors is bounded by bootHeight (set at startup to
// the chain tip), so it never needs blocks below the prune cutoff either.
func (dag *BlockDAG) pruneOldDAGBlocks() {
	if dag.state == nil {
		return
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight <= dag.pruneBuffer()+10 {
		return
	}
	cutoff := finalizedHeight - dag.pruneBuffer()
	dag.mu.Lock()
	pruned := 0
	for hash, b := range dag.blocks {
		if !b.IsGenesis && b.Height < cutoff && !dag.tips[hash] {
			delete(dag.blocks, hash)
			pruned++
		}
	}
	// FIX (P2-b, audit 2026-07-06): equivocationIndex has no cap or eviction
	// anywhere — unlike every other bookkeeping map in BlockDAG (replayedBlocks
	// at 50,000, warnedUnknownProposers at 500, orphans at maxOrphans), it
	// grows with the chain's entire lifetime, a slow but permanent memory leak
	// under sustained production. A blind size-based wipe (the
	// warnedUnknownProposers pattern) isn't safe here — it would forget a
	// proposer's earlier block for a still-relevant parent set, weakening
	// equivocation detection for history that hasn't been finalized yet.
	// Instead, evict in lockstep with the prune above: an entry whose
	// recorded hash no longer exists in dag.blocks refers to a block that's
	// already finalized-and-pruned, so checkAndIndexEquivocation's own
	// dag.blocks[existingHash] lookup already treats it as "no conflict"
	// (slashing.go) — the entry is dead weight, not a detection gap, once
	// evicted.
	evicted := 0
	for key, hash := range dag.equivocationIndex {
		if _, stillPresent := dag.blocks[hash]; !stillPresent {
			delete(dag.equivocationIndex, key)
			evicted++
		}
	}
	dag.mu.Unlock()
	if pruned > 0 {
		fmt.Printf("[DAG] 🧹 Pruned %d finalized blocks from in-memory DAG (below height %d); DB retains full history\n", pruned, cutoff)
	}
	if evicted > 0 {
		fmt.Printf("[DAG] 🧹 Evicted %d stale equivocationIndex entries for pruned blocks\n", evicted)
	}
}

// collectUnreplayedAncestors returns the subset of block's transitive
// ancestors that are in dag.blocks but have not yet been applied to state
// (i.e. not in dag.replayedBlocks), sorted in canonical DAG depth order:
// height ASC, blueScore DESC, hash ASC.  This total order is identical on
// every node that holds the same DAG graph, so sibling blocks are always
// applied in the same sequence regardless of P2P delivery order.
// Must be called under dag.mu.
func (dag *BlockDAG) collectUnreplayedAncestors(target *Block) []*Block {
	visited := make(map[string]bool)
	var result []*Block
	var dfs func(b *Block)
	dfs = func(b *Block) {
		for _, ph := range b.ParentHashes {
			if visited[ph] {
				continue
			}
			visited[ph] = true
			parent, ok := dag.blocks[ph]
			if !ok {
				continue
			}
			dag.replayedMu.Lock()
			replayed := dag.replayedBlocks[ph]
			dag.replayedMu.Unlock()
			if !replayed {
				result = append(result, parent)
				dfs(parent)
			}
		}
	}
	dfs(target)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Height != result[j].Height {
			return result[i].Height < result[j].Height
		}
		if result[i].BlueScore != result[j].BlueScore {
			return result[i].BlueScore > result[j].BlueScore // heavier chain first
		}
		return result[i].Hash < result[j].Hash
	})
	return result
}

// replayInCanonicalOrder replays all unreplayed ancestors of block in canonical
// GHOSTDAG order, then replays block itself.  Ensures every node applies
// sibling blocks in the same sequence even if they arrived out of order.
// Caller must hold dag.replayMu; dag.mu must NOT be held.
func (dag *BlockDAG) replayInCanonicalOrder(block *Block) bool {
	dag.mu.RLock()
	ancestors := dag.collectUnreplayedAncestors(block)
	dag.mu.RUnlock()
	for _, anc := range ancestors {
		if !dag.replayTransactions(anc) {
			return false
		}
	}
	return dag.replayTransactions(block)
}

// triggerSoftRetryFlush starts a soft-retry flush pass if none is already
// running, coalescing concurrent triggers the same way triggerOrphanResolve
// does for orphan resolution (sync_blocks.go) — one more pass runs
// immediately after the current one if a new trigger arrives while it's
// still working, instead of being dropped.
//
// FIX (durable fix, 2026-07-03): the previous unconditional `go
// dag.retryAndFlushSoftRetry()` on every single successful AddPeerBlock
// spawned a brand-new goroutine that rescans the ENTIRE soft-retry queue,
// every time. Under heavy concurrent traffic (confirmed live on a node with
// dag_tips_count in the thousands during a mass-reconsolidation burst) that
// is one full queue scan per accepted block, all running concurrently
// against each other — the log showed the same block's "Soft-retry
// succeeded" message printing over and over with the tip count never
// moving, consistent with many overlapping scans repeatedly finding and
// reprocessing entries that a sibling goroutine hadn't deleted yet. This
// serializes flush passes to at most one in flight, which cannot lose any
// retry: a trigger that arrives mid-pass just queues one more full pass
// (softRetryFlushAgain) instead of spawning a redundant concurrent one, and
// every pass re-reads the current queue contents from scratch regardless.
func (dag *BlockDAG) triggerSoftRetryFlush() {
	dag.softRetryMu.Lock()
	if dag.softRetryFlushInFlight {
		dag.softRetryFlushAgain = true
		dag.softRetryMu.Unlock()
		return
	}
	dag.softRetryFlushInFlight = true
	dag.softRetryMu.Unlock()
	SafeGoroutine("softRetryFlush", func() {
		// FIX (P0-3, beta-launch audit 2026-07-05): softRetryFlushInFlight
		// must be reset even if a pass panics — safeGoroutine's own recover
		// happens in ITS wrapper, outside this function entirely, by which
		// point it's too late to clear the flag here. Without this, a panic
		// mid-pass would leave softRetryFlushInFlight stuck true forever,
		// silently disabling every future soft-retry flush for the rest of
		// this node's uptime (see the early-return guard above this
		// goroutine is launched from) — worse than just the crash itself.
		defer func() {
			if r := recover(); r != nil {
				dag.softRetryMu.Lock()
				dag.softRetryFlushInFlight = false
				dag.softRetryMu.Unlock()
				fmt.Printf("[PANIC RECOVERED] softRetryFlush goroutine: %v\n%s\n", r, debug.Stack())
			}
		}()
		for {
			dag.retryAndFlushSoftRetry()
			dag.softRetryMu.Lock()
			if !dag.softRetryFlushAgain {
				dag.softRetryFlushInFlight = false
				dag.softRetryMu.Unlock()
				return
			}
			dag.softRetryFlushAgain = false
			dag.softRetryMu.Unlock()
			// loop again — another trigger arrived mid-pass
		}
	})
}

// retryAndFlushSoftRetry re-attempts every block in the soft-retry queue.
// Called via triggerSoftRetryFlush after each successful AddPeerBlock: new
// state may have been applied that unblocks previously failing transactions.
// Entries older than softRetryTTL are silently abandoned.
func (dag *BlockDAG) retryAndFlushSoftRetry() {
	dag.softRetryMu.Lock()
	if len(dag.softRetryBlocks) == 0 {
		dag.softRetryMu.Unlock()
		return
	}
	candidates := make([]*Block, 0, len(dag.softRetryBlocks))
	for _, b := range dag.softRetryBlocks {
		candidates = append(candidates, b)
	}
	dag.softRetryMu.Unlock()
	// P1-07 (audit): deterministic replay order so state-dependent sibling
	// blocks (e.g. X funds Bob, Y spends Bob) are always applied in the same
	// sequence. Sort by (height ASC, blueScore DESC, hash ASC).
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Height != b.Height {
			return a.Height < b.Height
		}
		if a.BlueScore != b.BlueScore {
			return a.BlueScore > b.BlueScore
		}
		return a.Hash < b.Hash
	})

	now := time.Now()
	for _, b := range candidates {
		dag.softRetryMu.Lock()
		firstAt, exists := dag.softRetryFirstAt[b.Hash]
		dag.softRetryMu.Unlock()
		if !exists {
			continue // already resolved by a concurrent retry pass
		}
		if now.Sub(firstAt) > softRetryTTL {
			dag.softRetryMu.Lock()
			delete(dag.softRetryBlocks, b.Hash)
			delete(dag.softRetryFirstAt, b.Hash)
			dag.softRetryMu.Unlock()
			fmt.Printf("[GHOSTDAG] Abandoned soft-retry block #%d (%s...) after TTL\n", b.Height, b.Hash[:16])
			continue
		}
		// All parents must be in dag.blocks before retrying replay.
		dag.mu.RLock()
		allPresent := true
		for _, ph := range b.ParentHashes {
			if _, ok := dag.blocks[ph]; !ok {
				allPresent = false
				break
			}
		}
		dag.mu.RUnlock()
		if !allPresent {
			continue
		}
		// AddPeerBlock re-validates and handles insertion, blue-score update,
		// tips management, orphan resolution, and further soft-retry triggers.
		if dag.AddPeerBlock(b) {
			dag.softRetryMu.Lock()
			delete(dag.softRetryBlocks, b.Hash)
			delete(dag.softRetryFirstAt, b.Hash)
			dag.softRetryMu.Unlock()
			fmt.Printf("[GHOSTDAG] Soft-retry succeeded for block #%d (%s...)\n", b.Height, b.Hash[:16])
		}
	}
}
