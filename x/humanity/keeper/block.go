package keeper

import (
	"bytes"
	"context"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Transaction struct {
	Type           string  `json:"type"`
	Wallet         string  `json:"wallet"`
	To             string  `json:"to,omitempty"` // transfer destination
	Amount         float64 `json:"amount,omitempty"`
	AmountOut      float64 `json:"amount_out,omitempty"`       // swap output amount
	AmountPerHuman float64 `json:"amount_per_human,omitempty"` // for ubi_distribution
	LPShares       float64 `json:"lp_shares,omitempty"`        // for add_liquidity; also reused on escrow_move for LP shares force-liquidated due to inactivity (see checkAndMoveToEscrowLocked)
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
	DistributionAt int64  `json:"distribution_at,omitempty"`
	TxHash         string `json:"tx_hash"`
	// Nullifier and Commitment are set on register_human TXs so secondary
	// nodes can apply the registration to their local state when they receive
	// the block — without needing a separate snapshot or state sync.
	Nullifier  string `json:"nullifier,omitempty"`
	Commitment string `json:"commitment,omitempty"`
	// ZK proof fields for register_human — enables secondary nodes to
	// independently verify the proof via BioVerifier without trusting
	// the validator signature alone. Fields are omitted for non-registration
	// TXs and for blocks produced by old nodes (backward-compatible).
	ProofA     []string   `json:"proof_a,omitempty"`     // [2]string big.Int decimal
	ProofB     [][]string `json:"proof_b,omitempty"`     // [2][2]string big.Int decimal
	ProofC     []string   `json:"proof_c,omitempty"`     // [2]string big.Int decimal
	PubSignals []string   `json:"pub_signals,omitempty"` // public signals (decimal)
	// BlockAHash/BlockBHash identify the equivocation evidence pair for
	// "slash_equivocation" TXs so the replay can be idempotent (see the
	// slash_equivocation case in replayTransactions and
	// QueueEquivocationEvidenceTx in slashing.go).
	BlockAHash string `json:"block_a_hash,omitempty"`
	BlockBHash string `json:"block_b_hash,omitempty"`
	// DetectedAt (2026-07-07) is the ORIGINAL conflicting block's own
	// Timestamp — i.e. whatever value the node that first detected this
	// equivocation passed to RecordEquivocationAndSuspend — carried in the
	// TX so that EVERY node replaying it (not just the one that first
	// detected the conflict) calls RecordEquivocationAndSuspend with the
	// identical "now", producing the identical offense count/suspension
	// decision. Without this, validator suspension was a node-local side
	// effect of whichever node happened to have BOTH conflicting blocks in
	// its own dag.blocks at the moment the second one arrived — a node that
	// never independently saw both never suspended the validator at all,
	// even though the balance-penalty TX (this same TX type) was already
	// consensus-replicated. See the slash_equivocation case in
	// replayTransactions.
	DetectedAt int64 `json:"detected_at,omitempty"`
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
	// TxRoot is the digest calculateBlockHash commits to for Transactions.
	// Carrying it explicitly is what lets a block travel WITHOUT its body
	// (roadmap step 4, see tx_batch.go): the hash preimage already contained
	// only this digest, never the transaction list, so transporting the two
	// separately leaves block hashes byte-identical — no fork, no activation
	// height. Empty on blocks produced before this field existed, which
	// calculateBlockHash handles by computing the root the old way.
	TxRoot    string `json:"tx_root,omitempty"`
	Signature string `json:"signature,omitempty"`
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
	// KEff is the per-block K that KnightDAG's adaptive layer actually
	// inferred (the smallest k whose greedy blue set covers a majority of
	// the merge set — see knightdagInferK). Same trust model as BlueScore/
	// Blues: a locally-derived annotation recomputed by every node in
	// computeGHOSTDAGState, never trusted from the wire, excluded from the
	// block hash. nil below the KnightDAG activation height (pointer so
	// "not applicable" is distinguishable from a genuine k=0, which is the
	// common no-concurrency case). Serialized so the explorer/API can show
	// the inferred K next to the epoch ceiling.
	KEff *int `json:"k_eff,omitempty"`
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
	// What SelfFetched does and does NOT bypass, stated exactly — this used to
	// claim it was "orthogonal to FromSync's authorization/equivocation/
	// finality bypasses (deliberately NOT touched here)", and that stopped
	// being true when P2-2 (beta-launch audit 2026-07-05) extended it to the
	// suspension gate. Corrected 2026-08-18:
	//
	//   authorization  NOT bypassed. A proposer must still be authorized via
	//                  the exact same NODE_OPERATOR_BINDING_SIGNATURE-backed
	//                  check as any other block (AddPeerBlock keys that gate on
	//                  FromSync alone).
	//   finality       NOT bypassed.
	//   circuit breaker  bypassed — the original purpose: let an ALREADY-
	//                  authorized proposer's block, fetched by OUR OWN
	//                  deliberate request rather than unsolicited push/gossip,
	//                  past the reputation gate long enough to close the very
	//                  gap that tripped it.
	//   suspension     bypassed too, since P2-2. A re-fetched block can predate
	//                  a suspension record this node only holds today, and
	//                  rejecting it reintroduces exactly the merge-stall
	//                  SelfFetched exists to prevent. See that call site.
	//
	// Not attacker-reachable either way: the field is `json:"-"`, never
	// deserialized, and set only by this node's own fetch paths in
	// sync_blocks.go. Works
	// automatically for any current or future validator, regardless of
	// which peer URLs happen to be statically configured anywhere.
	SelfFetched bool `json:"-"`
	// Replayed mirrors chain_blocks.replayed — true once this block's
	// transactions have actually been committed to chain_accounts (not just
	// its header saved). See ChainState.MarkBlockReplayed's comment for the
	// live incident (self-deadlock killed mid-replay via SIGQUIT) this
	// closes: a block whose header made it to chain_blocks but whose replay
	// never completed used to be silently treated as "already applied" on
	// the next boot (see the startup loader's own old comment), permanently
	// losing that block's state effects with no error anywhere. Never
	// serialized; only meaningful as loaded from the DB at startup.
	Replayed bool `json:"-"`
}

// peerChallenge holds a one-time challenge issued to a registering peer.
type peerChallenge struct {
	value     string
	expiresAt int64
}

type BlockDAG struct {
	blocks map[string]*Block
	tips   map[string]bool
	mu     sync.RWMutex
	state  *ChainState
	evm    *EVMEngine // set by EVMRPCServer after construction; used by replayTransactions for ZK proof verification
	nodeID string
	height int64
	// heightSchnell spiegelt height, ist aber OHNE dag.mu lesbar.
	//
	// dag.Height() nimmt dag.mu.RLock(). Waehrend ein Block-Burst angewendet
	// wird, haelt die DAG die SCHREIBsperre -- und Go's RWMutex sperrt jeden
	// neuen Leser aus, sobald ein Schreiber ansteht. /api/status blockiert
	// dann mit, und der Knoten wirkt von aussen tot.
	//
	// Am 02.09.2026 genau so beobachtet: Container "Up 20 minutes", Logs
	// voller erfolgreich angehaengter Bloecke, Tips 1 -- und trotzdem keine
	// Antwort auf /api/status, weder von aussen noch aus dem Container
	// heraus. Die Deploy-Pruefung meldete "the node answers but its height
	// never moved in 10 minutes", der Betreiber sah einen Absturz. Es war
	// keiner: der Knoten arbeitete die ganze Zeit.
	//
	// Nur ueber setHeight zu schreiben, nie direkt -- dafuer gibt es einen
	// Waechter-Test (dag_height_spiegel_test.go).
	heightSchnell atomic.Int64
	// lastHeightAdvanceAt: wann die Hoehe zuletzt wirklich gestiegen ist.
	// Siehe setHeight fuer den Unterschied zu "zuletzt einen Block
	// angehaengt" -- der zweite Wert luegt bei einem abgehaengten Knoten.
	lastHeightAdvanceAt atomic.Int64
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
	bootHeight int64
	// bootTime is this process's own wall-clock start time, captured once at
	// construction and never updated — distinct from bootHeight (a block-height
	// concept). ProduceBlock's double-production guard runs unconditionally —
	// it distinguishes a redeploy overlap from ordinary operation via
	// producedHeights below, not via elapsed time — so bootTime is kept only
	// for that guard's log line ("how long has this process been alive when it
	// caught a conflicting durable row").
	bootTime time.Time
	// producedHeights records every height THIS process has itself produced a
	// block at. It is what lets the double-production guard below run for the
	// process's whole life instead of only for a 45-second window after boot —
	// see that guard's own comment for the five equivocation incidents that
	// motivated removing the window and the halt that forced it back.
	//
	// The halt happened because "chain_blocks already has a block from me at
	// this height" is true in ordinary steady-state operation, once this node's
	// tip selection lags one height behind what it just durably stored. That
	// sentence describes normal operation. "chain_blocks has a block from me at
	// this height that I did NOT write in this process" does not — it can only
	// mean another instance of this validator is running. This map is the
	// difference between the two.
	//
	// Bounded: entries below the finalized checkpoint are dropped by
	// pruneOldDAGBlocks, so it tracks the live window, not all history.
	producedHeights   map[int64]bool
	producedHeightsMu sync.Mutex
	// bootHeightCheckpointBacked is true only when bootHeight was set by
	// actually seeding dag.blocks/dag.tips with a real, stored block at that
	// exact height (RefreshBootHeightAfterSnapshotImport's checkpoint branch,
	// seededFromCheckpoint) — see AddPeerBlock's bootHeight-skip call site for
	// why this distinction is safety-critical, not cosmetic.
	bootHeightCheckpointBacked bool
	// unreplayedAtBoot holds blocks loaded from chain_blocks at startup whose
	// replayed column was false — i.e. a header was durably saved but the
	// process was killed before its transactions' effects were confirmed
	// committed to chain_accounts (see ensureReplayedColumn's comment for the
	// live incident). Populated once during the load loop in NewBlockchain,
	// consumed and cleared by repairUnreplayedBlocks() once dag.evm is
	// available (verifyZKProof needs it, and EVM isn't wired up until after
	// NewBlockchain returns — see NewEVMRPCServer's call site).
	unreplayedAtBoot     []*Block
	pendingTxs           []Transaction
	txMu                 sync.Mutex
	signingKey           *ecdsa.PrivateKey
	selfProposer         string          // lower-cased Ethereum address of this node's signing key
	authorizedValidators map[string]bool // Ethereum addresses allowed to propose blocks
	currentEpoch         *EpochCommittee // active block-producer committee for the current epoch
	epochMu              sync.RWMutex    // guards currentEpoch
	activeSyncPeers      map[string]bool // peers with a running syncWithNode goroutine
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
	peerSyncHeight map[string]int64
	// peerSyncSeenAt haelt fest, WANN zuletzt etwas von diesem Peer kam --
	// unabhaengig davon, ob seine Hoehe dabei gestiegen ist.
	//
	// peerSyncHeight allein genuegt der Bremse in peer_lag_bremse.go nicht:
	// die Karte ist MONOTON, ein verschwundener Peer behaelt also seine letzte
	// Hoehe fuer immer, und der daraus berechnete Rueckstand waechst mit
	// jedem eigenen Block weiter. Ohne Zeitstempel wuerde ein einmal
	// abgeschalteter Peer die Blockgroesse dauerhaft druecken.
	//
	// Bewusst bei JEDEM Kontakt gesetzt, nicht nur wenn die Hoehe steigt: ein
	// Peer, der feststeckt, meldet weiter seine unveraenderte Hoehe -- und
	// genau der soll die Bremse ausloesen, nicht von ihr ausgenommen werden.
	// Guarded by syncPeerMu, wie peerSyncHeight.
	peerSyncSeenAt map[string]time.Time
	// peerSyncEigeneHoehe haelt fest, wie hoch DIESER Knoten stand, als er
	// zuletzt etwas von dem Peer erfuhr.
	//
	// Ohne das ist der berechnete Rueckstand unbrauchbar: peerSyncHeight
	// waechst nur bei einem Abruf MIT Inhalt, die eigene Hoehe dagegen bei
	// jedem selbst produzierten Block. Ein Knoten, der produziert waehrend
	// vom Peer nichts Neues kommt, sieht dann einen Rueckstand, den es nicht
	// gibt -- am 02.09.2026 live: beide Knoten exakt auf derselben Hoehe, und
	// C1 meldete trotzdem lag=78 und drosselte auf den Boden.
	peerSyncEigeneHoehe map[string]int64
	// cleanSyncStreak tracks, per peer URL, how many CONSECUTIVE doSyncOnce
	// calls in a row found nothing this node failed to merge — see
	// recordCleanSyncCycle's own comment for the exact definition and the
	// 2026-07-10 incident it exists to close: dag.height, syncTargetHeight,
	// and even peerSyncHeight (see that field's own comment) can all read
	// "caught up" while blocks from a peer are actively being fetched but
	// rejected as orphans (the live signature of a fork already in
	// progress) — none of them can tell "genuinely nothing new" apart from
	// "new blocks exist but none of them merged". This can. Guarded by
	// syncPeerMu, same as peerSyncHeight.
	cleanSyncStreak map[string]int
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
	trustedSeeds map[string]bool
	// syncGateSeeds is the exact seed list StartPeerDiscovery armed the
	// initial-sync gate with (seedURLs(selfURL) — NOT trustedSeeds, which
	// additionally contains PEER_NODES static peers whose height was never
	// meant to gate this node's production). Kept so armInitialSyncGate can
	// re-arm the gate later, from a caller that has no selfURL of its own:
	// PerformResync (autoheal.go) rolls dag.height back to a trusted
	// checkpoint mid-life, which re-creates exactly the "behind the seed,
	// must not produce yet" situation the gate exists for — see
	// armInitialSyncGate's own comment for the incident. Guarded by
	// syncPeerMu, same as trustedSeeds.
	syncGateSeeds          []string
	syncPeerMu             sync.Mutex
	warnedUnknownProposers map[string]bool // suppresses repeated "not authorized" log lines
	// unknownProposerLastRecovery is when this node last tried to LEARN an unknown
	// proposer by pulling the validator lists from its peers, per proposer address.
	//
	// FIX (2026-07-26, confirmed live on the primary): the recovery used to be
	// driven off warnedUnknownProposers, whose documented job is suppressing
	// repeated log lines. Overloading a log-suppression flag to also gate the
	// recovery meant the recovery ran EXACTLY ONCE per proposer, ever. And
	// syncValidatorsFromAllPeers does nothing at all when activeSyncPeers is still
	// empty -- precisely the state a node is in for the first seconds after a
	// restart, when peer registration has not completed yet but peer blocks are
	// already arriving. One badly-timed attempt therefore left the proposer
	// unauthorized permanently, with every later block from it rejected SILENTLY
	// (the log line suppressed by that same flag) and its waiting orphans
	// abandoned.
	//
	// Measured on 2026-07-26: the primary restarted at 16:45, never learned
	// validator 0x0BE8b961..., and from then on merged nothing from either Contabo
	// while its own blocks kept flowing to them. A one-way fork with 1386
	// unresolvable missing parents on the primary and zero on both secondaries, a
	// blue-score gap growing monotonically (1457 -> 1580 -> 1806), and account
	// balances diverging because the rejected blocks carried the transfers.
	//
	// The secondaries are structurally immune: they hold the primary in
	// trustedSeeds, so blocks fetched from it carry FromSync=true and skip this
	// gate. The primary's trustedSeeds is permanently empty by construction (it is
	// the seed, not a syncer -- see the hasCaughtUpWithAllPeers gate's own comment
	// around bootHeight), so it is the ONE node that can be locked out of a
	// validator it does not already know.
	//
	// Time-based and retried for as long as blocks from that proposer keep
	// arriving, so a recovery lost to an empty peer list or a failed HTTP call is
	// retried instead of having been the node's only chance.
	unknownProposerLastRecovery map[string]time.Time
	peerChallenges              map[string]peerChallenge // address → pending challenge (P1-3)
	challengeMu                 sync.Mutex
	replayedBlocks              map[string]bool // tracks blocks already replayed — prevents double-credit on duplicate delivery
	replayedMu                  sync.Mutex
	// replayFailures backs off repeated replayTransactions attempts against a
	// block that keeps failing — see that function's own comment (the
	// "2026-07-25 hang incident") for why this exists: without it, a block
	// containing one deterministically-failing TX (e.g. a sender genuinely
	// out of balance) gets replayed from scratch on EVERY new descendant
	// block's arrival, forever, since a failed attempt never marks anything
	// and the next arriving block's ancestor walk finds the exact same
	// unreplayed ancestor again. Under sustained block production (multiple
	// validators, ENABLE_MULTI_BLOCK_TICK) that is many full replay attempts
	// per second, indefinitely — confirmed live to hang a node's replay path
	// (and, transitively, its HTTP API) for 9+ minutes on a single block.
	// Guarded by replayedMu, same as replayedBlocks — the two are always
	// updated together conceptually (one block, one outcome).
	replayFailures map[string]replayFailureState
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
	stateRootMismatchesMu   sync.Mutex // protects stateRootMismatches/stateRootMismatchLastAt (written under replayMu+cs.mu, read independently by TotalStateRootMismatches)
	// lastSuccessfulPeerSyncAt is the Unix timestamp of the last time this
	// node successfully accepted a peer block via AddPeerBlock. Read/written
	// with atomic.Int64 (not dag.mu) since it's set from AddPeerBlock's
	// success tail, after dag.mu has already been released — see
	// /api/health/combined (Gesamtaudit 2026-06-28, P2-4/P3-7: "Health/API
	// zeigt nicht ... seit wann [ein StateRoot-Mismatch existiert]").
	lastSuccessfulPeerSyncAt atomic.Int64
	// deferredWatch tracks, per peer URL, every block that AddPeerBlock
	// deferred behind a still-in-flight parent, together with the moment it
	// was first deferred. reconcileDeferrals (sync_blocks.go) uses the age to
	// tell an ordinary propagation delay from a fork; see its comment for why
	// judging a deferral within the cycle that produced it deadlocks
	// production on a live chain.
	deferredWatchMu sync.Mutex
	deferredWatch   map[string]map[string]int64
	// lastPeerContactAt is the Unix timestamp of the last time this node
	// received ANY block from a foreign peer, whether or not AddPeerBlock
	// went on to merge it — set unconditionally at AddPeerBlock's entry,
	// the same point recordRawArrivalLatency measures from (see that call
	// site's own comment for why "before any gate" matters here too).
	//
	// FIX (P0, 2026-07-24 — root cause of Contabo1 forking within minutes of
	// a resync, confirmed live during the diagnostic session that also added
	// prefetchParentsFromDB): every one of ProduceBlock's stall-timeout
	// escape valves (see syncStallTimeout's own comment) used
	// lastSuccessfulPeerSyncAt alone as "evidence the peer connection is
	// alive" — but that field only advances on a successful MERGE, never on
	// merely receiving data. Under a severe merge backlog (confirmed live:
	// 100-150s+ average AddPeerBlock latency, worse than syncStallTimeout's
	// 90s), a node can go the full timeout window without a single
	// successful merge while the primary is fully reachable and actively
	// sending a continuous stream of blocks — every one of them queueing as
	// an orphan instead of merging. The gate then concluded "primary must be
	// down" and let this node start producing independently, forking off its
	// own chain from a stale height while the real primary kept racing
	// ahead — which then added the fork's own blocks to every OTHER node's
	// orphan backlog too, a self-reinforcing spiral. lastPeerActivityAt()
	// (below) takes the more recent of this field and lastSuccessfulPeerSyncAt,
	// so the stall gates now correctly distinguish "genuinely hearing
	// nothing from any peer" (the actual "primary may be down" case they
	// exist for) from "hearing plenty, just can't merge it fast enough yet"
	// (which must keep waiting, not fork).
	lastPeerContactAt atomic.Int64
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
	// deepScanResumeHeight (P0, 2026-07-10 — real root cause of Primary never
	// merging a single live block from a peer with a large historical gap):
	// a deepScan pass used to walk the ENTIRE range from deepScanFloor() to
	// the peer's current tip in one synchronous doSyncOnce call — up to
	// maxPagesPerCall (2000 pages, 1,000,000 blocks). For a peer whose chain
	// is hundreds of thousands of blocks tall, that single call can take
	// minutes, during which THIS peer's entire 1s-ticker sync loop is
	// blocked inside it — confirmed live via recordRawArrivalLatency: avg
	// 174-176s (max 407s) latency for blocks that should attach in well
	// under a second, with dag_tips_count stuck at 1 and zero successful
	// peer merges for a node's entire uptime, even though the peer was
	// fully healthy, registered, and authorized the whole time. Every fresh
	// block orphaned on its own immediate predecessor, which was itself
	// stuck behind the same monopolized call. This map remembers, per peer,
	// how far a BOUNDED deepScan pass (see deepScanPageBudgetPerCall) has
	// progressed, so the next call resumes forward instead of restarting
	// the full historical walk from the floor every time — preserving
	// deepScan's complete-history guarantee while keeping the sync loop
	// responsive to live blocks throughout. Reset to 0 once a pass actually
	// reaches the peer's current tip (a real empty page, not just a short
	// one — see doSyncOnce's own comment on why a short page alone isn't a
	// reliable "done" signal in deepScan mode), so a later deepScan
	// triggered by a fresh, unrelated orphan still starts a genuine full
	// sweep instead of silently skipping straight back to "already at the
	// tip". Guarded by lastDeepScanAtMu (same per-peer bookkeeping lock).
	deepScanResumeHeight map[string]int64
	// deepScanFloorOverride (P0, 2026-07-10 — see lowerDeepScanFloor's own
	// comment, sync_blocks.go) narrows one specific peer's deepScan floor
	// below deepScanFloor()'s own value once a FULL sweep from that floor to
	// the peer's tip still left blocks unmerged — evidence the real common
	// ancestor lies below the floor deepScanFloor() computed (which only
	// guarantees THIS node's own history is anchored there, not that a
	// SPECIFIC peer's fork was ever fully merged before bootHeight advanced
	// past it — e.g. after a sustained sync-starvation incident). 0 (a map's
	// zero value) means "no override, trust deepScanFloor() as-is" — the
	// default, pre-2026-07-10 behavior for every peer that hasn't needed
	// this yet. Never lowered below finalityFloorLimit(): isFinalityViolation
	// unconditionally rejects anything past that point regardless of how far
	// deepScan searches (see that function's own comment), so searching
	// further would only burn a full sweep re-scanning a range every block
	// in it is silently skipped from anyway. Guarded by lastDeepScanAtMu
	// (same per-peer bookkeeping lock as the two maps above).
	deepScanFloorOverride map[string]int64
	// syncTargetHeight is set at startup to the seed node's current block
	// height. ProduceBlock defers production until this node has caught up
	// to within 10 blocks of the target, preventing the "produce on a stale
	// fork while sync is still running" divergence that requires manual
	// RESYNC_FROM_SNAPSHOT to fix. Cleared once caught up. If the seed is
	// unreachable at startup, or sync makes no further progress for
	// syncStallTimeout (see ProduceBlock's gate), production proceeds
	// independently so a downed seed never blocks all other nodes.
	syncTargetHeight atomic.Int64
	activeGhostdagK  atomic.Int32 // live GHOSTDAG K for current epoch; 0 → use ghostdagKBase
	startupTime      int64        // Unix timestamp of NewBlockchain — used by the initial-sync gate
	// (ghostdagStuckHash/ghostdagStuckCount removed 2026-07-10 — ProduceBlock's
	// stuck-ancestor escape hatch now shares the orphan-tracking machinery
	// via produceStuckGaps instead of its own raw tick counter; see that
	// field's comment and ProduceBlock's call site for why.)
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
	// foreignValidatorActivityMu guards lastSeenFromValidator and
	// lastMergedFromValidator immediately below. Separate from dag.mu to
	// keep this bookkeeping's own critical section tiny, even though every
	// current call site happens to already hold dag.mu when touching it.
	foreignValidatorActivityMu sync.Mutex
	// lastSeenFromValidator (P0, 2026-07-10 — Primary/Contabo1 permanent-
	// partial-merge incident) tracks, per lower-cased authorized-validator
	// address, the last time a genuinely signature-verified, authorized
	// block claiming to be theirs reached AddPeerBlock — regardless of
	// whether it was then successfully merged. Paired with
	// lastMergedFromValidator below, this is what lets
	// selfProducedFinalityAllowed distinguish "isolated from a validator
	// that is still actively trying to reach us" (must pause hardening)
	// from "this validator has simply gone quiet/retired" (must NOT pause
	// forever): validator addresses are never de-registered (see
	// AuthorizedValidatorList's own comment) — requiring a recent MERGE
	// from every entry in dag.authorizedValidators, with no "are they even
	// still around" escape hatch, would let a single permanently-offline
	// validator freeze checkpoint hardening for the whole network forever.
	// Confirmed live: lastForeignMergeAt alone (a single DAG-WIDE
	// timestamp) stayed fresh — and hardening never paused — the entire
	// time Primary was completely walled off from Contabo1, simply because
	// it kept merging a DIFFERENT validator (cd20) fine. Set by
	// recordForeignSeen, right after AddPeerBlock's signature+authorization
	// gate passes, before any later gate (finality, suspension, missing-
	// parent, GHOSTDAG) can reject the block — so a validator that's
	// chronically rejected downstream still counts as "seen".
	lastSeenFromValidator map[string]int64
	// lastMergedFromValidator is lastForeignMergeAt's per-validator
	// equivalent — see that field's own comment for the single-timestamp
	// version this extends, and lastSeenFromValidator's comment above for
	// why both are needed together. Set by recordForeignMergeForProposer,
	// the same call site as recordForeignMerge.
	lastMergedFromValidator map[string]int64
	// foreignLatency* accumulate real, measured end-to-end attach-latency
	// samples (ProducedAtMs on the sender to time-of-attach here) — see
	// recordForeignAttachLatency's own comment for why this exists as a
	// permanent operational diagnostic, not a temp one.
	foreignLatencyMu        sync.Mutex
	foreignLatencyCount     int
	foreignLatencySumMs     int64
	foreignLatencyMaxMs     int64
	lastForeignLatencyLogAt atomic.Int64
	// lastForeignLatencyWindow holds the avg/max/count from the most
	// recently CLOSED accumulation window — foreignLatencyCount/SumMs/MaxMs
	// above reset to zero every foreignAttachLatencyLogInterval (see
	// recordForeignAttachLatency), so without this, anything reading the
	// live counters between resets would see a value that's climbing from
	// zero rather than the actual last-known measurement. Exposed via
	// GetLatencyTelemetry for /api/status — a permanent operational signal,
	// not just a log line, per the launch-night scaling roadmap.
	lastForeignLatencyWindow    latencyWindow
	lastRawArrivalLatencyWindow latencyWindow
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
	// totalRawArrivalCount / totalForeignAttachCount are MONOTONIC lifetime
	// counters incremented alongside the windowed latency counters above
	// (which reset every log interval, so they can't answer "did ANYTHING
	// attach since the last check?"). Added 2026-07-24 for the sync-starvation
	// auto-heal check (autoheal.go): the fork incident that day showed a node
	// can sit receiving 1600+ raw arrivals per 30s while attaching exactly
	// ZERO of them (every peer block orphaning against a diverged ancestry) —
	// a state none of the existing detection paths sees quickly, because
	// StateRoot mismatches only accrue on blocks that DO attach, and the
	// height-stall check needs 25 minutes of zero height movement.
	totalRawArrivalCount    atomic.Int64
	totalForeignAttachCount atomic.Int64
	// newBlockSubs backs the /api/events SSE stream (scaling roadmap
	// 2026-07-21): each subscriber gets its own buffered channel; notified
	// non-blockingly (a full/slow reader is dropped from the broadcast, never
	// allowed to stall block production) whenever a block becomes visible in
	// dag.blocks — see notifyNewBlock's own comment for the two call sites.
	newBlockSubsMu sync.Mutex
	newBlockSubs   map[chan struct{}]struct{}
	// lastIsolationPauseLogAt rate-limits the "finality advance paused"
	// diagnostic the same way as the other log throttles above — this can
	// otherwise fire once per self-produced block (every BLOCK_TIME) for as
	// long as the isolation lasts.
	lastIsolationPauseLogAt atomic.Int64
	lastFinalityRejectLogAt atomic.Int64
	// lastMergeSetCapLogAt/lastMergeSetBFSCapLogAt rate-limit the GHOSTDAG
	// merge-set cap warnings the same way as the other log throttles above
	// (Performance audit 2026-07-06) — both are liveness backstops meant to
	// log rarely under a genuine burst, but computeGHOSTDAGState runs under
	// dag.mu on every block, so an unthrottled fmt.Printf here during a real
	// sustained burst could flood the log the same way lastFarAheadLogAt's
	// comment already documents for a diverged-fork flood.
	lastMergeSetCapLogAt    atomic.Int64
	lastMergeSetBFSCapLogAt atomic.Int64
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
	// trips. proposerBreaker (boundedBreaker, breaker.go) has its own dedicated
	// mutex, never dag.mu — so the hot reject path can never contend with block
	// production.
	proposerBreaker          *boundedBreaker
	lastProposerBreakerLogAt atomic.Int64 // rate-limits the breaker-drop log to once/sec
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
	// lastAutoResyncSuppressedLogAt rate-limits triggerAutoResync's
	// cooldown-suppression log line (UnixNano), same pattern as
	// lastFarAheadLogAt/lastFinalityRejectLogAt. Four independent detection
	// paths can each hit the suppression once a minute, and the line is only
	// worth one entry per window however many of them fired.
	lastAutoResyncSuppressedLogAt atomic.Int64
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

	// finalityWalkGaps holds hashes registerFinalityWalkGap (finality.go)
	// wants fetched — deliberately SEPARATE from orphans/orphanFirstSeen.
	// See registerFinalityWalkGap's own comment for why: orphans (via
	// MissingParentHashes) also drives doSyncOnce's wantDeepScan trigger
	// (sync_blocks.go), which is meant to fire only for a genuine "no
	// normal-window sync progress is possible" situation. A finality
	// checkpoint walk finds a NEW gap on essentially every call as the
	// checkpoint target keeps sliding forward with the tip — reusing
	// orphans kept wantDeepScan permanently true, confirmed live: a node
	// stuck re-scanning its entire history from height 0 in a loop,
	// re-importing thousands of ancient disconnected block fragments as
	// new dag.tips entries every pass instead of ever reaching steady-state
	// real-time sync. fetchMissingAncestors still services this set (see
	// its own updated comment), just without deepScan ever caring about it.
	finalityWalkGaps map[string]bool

	// produceStuckGaps holds hashes ProduceBlock's own stuck-ancestor escape
	// hatch (see that call site's 2026-07-10 FIX comment) wants actively
	// fetched. Same rationale as finalityWalkGaps just above: deliberately
	// separate from orphans so it never feeds MissingParentHashes/
	// wantDeepScan. Age/attempt bookkeeping reuses the shared
	// orphanFirstSeen/orphanAttempts maps (already source-agnostic — both
	// queueOrphan and registerFinalityWalkGap write into them the same way),
	// so ProduceBlock's bridge decision uses the exact same patient,
	// battle-tested orphanAbandonAfter/minOrphanAttemptsBeforeAbandon gate as
	// every other synthetic-checkpoint stub site instead of a separate, much
	// hastier raw tick counter — see ProduceBlock's call site for the
	// incident (620-783 stubs accumulated on two production nodes) that
	// motivated this.
	produceStuckGaps map[string]bool

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
		var g struct {
			GenesisTime string `json:"genesis_time"`
		}
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

// validatorLabelOverrides parses VALIDATOR_LABELS, an operator-configured
// list of explicit display labels — format "Label:0xAddress,Label:0xAddress,..."
// e.g. "Primary:0xB6...,Validator 1:0xD0...,Validator 2:0xa4...". Must be
// set IDENTICALLY on every node, the same trust model this codebase already
// uses for KNIGHTDAG_ACTIVATION_HEIGHT (see its own comment) and
// AUTHORIZED_VALIDATORS above — loaded once at startup into a package var.
//
// Two problems neither of this file's existing validator bookkeeping tables
// can solve made this necessary rather than deriving it automatically:
//  1. "Primary" is a deployment role (IS_PRIMARY_NODE, set per-process),
//     not anything recorded on-chain or in any table a node can look up
//     for a PEER's address — there is no existing signal any node can use
//     to learn "this OTHER validator is the primary" at all.
//  2. registered_nodes (GetValidatorOrdinals' source) is populated only by
//     each node's own RegisterNode(NODE_OPERATOR_WALLET) call for ITSELF
//     at startup — never for peers — so its contents, and therefore any
//     ordinal derived from it, can genuinely differ from one node's local
//     database to another's. This explicit, shared config is what actually
//     guarantees every node's explorer shows the identical label for the
//     identical address, which per-node-derived ordinals cannot.
//
// Any address not covered by this override falls back to the existing
// GetValidatorOrdinals()-derived "Validator #N" numbering — unchanged
// default behavior for any deployment that doesn't set this var, and a
// working (if not launch-fleet-curated) label for a newly-joined validator
// before an operator adds it here.
func loadValidatorLabelOverrides() map[string]string {
	out := map[string]string{}
	raw := os.Getenv("VALIDATOR_LABELS")
	if raw == "" {
		return out
	}
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		label := strings.TrimSpace(parts[0])
		addr := strings.ToLower(strings.TrimSpace(parts[1]))
		if label == "" || !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
			continue
		}
		out[addr] = label
	}
	return out
}

var validatorLabelOverrides = loadValidatorLabelOverrides()

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
	// Der Ed25519-Schluessel, mit dem dieser Validator Menschen bezeugt.
	//
	// Er steht hier, damit ein Proof-Server die Liste der anerkannten
	// Bezeuger AUS DER KETTE lesen kann, statt sie in einer
	// Umgebungsvariablen gereicht zu bekommen. Eine handgepflegte Liste ist
	// eine Genehmigung; ein on-chain-Register, an dem eine Doppelsignatur
	// haengt, ist keine.
	PersonhoodKey string `json:"personhood_key,omitempty"`
	// Die Adresse, unter der der Vergleichsdienst dieses Betreibers erreichbar
	// ist. Damit findet ein Coordinator seine Validatoren aus der Kette, statt
	// sie in VALIDATOR_URLS eingetragen zu bekommen -- sonst waere die Aufnahme
	// eines neuen Verifiers wieder eine Genehmigung, nur eine Ebene hoeher.
	MatchingURL string `json:"matching_url,omitempty"`
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

// SelfSigningAddress is the address this node signs blocks with, derived from
// RELAYER_PRIVATE_KEY. Empty when no key is configured.
//
// Used by MPC committee selection: a node has to know its own address to work
// out whether the committee drew it, and the peer registry has to publish that
// same address so others can verify its contributions.
func (dag *BlockDAG) SelfSigningAddress() string {
	pk := strings.TrimPrefix(strings.TrimSpace(os.Getenv("RELAYER_PRIVATE_KEY")), "0x")
	if pk == "" {
		return ""
	}
	priv, err := crypto.HexToECDSA(pk)
	if err != nil {
		return ""
	}
	return strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
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
		blocks:                      make(map[string]*Block),
		tips:                        make(map[string]bool),
		state:                       state,
		nodeID:                      nodeID,
		bootTime:                    time.Now(),
		authorizedValidators:        loadAuthorizedValidators(),
		activeSyncPeers:             make(map[string]bool),
		peerSyncHeight:              make(map[string]int64),
		peerSyncSeenAt:              make(map[string]time.Time),
		peerSyncEigeneHoehe:         make(map[string]int64),
		cleanSyncStreak:             make(map[string]int),
		warnedUnknownProposers:      make(map[string]bool),
		unknownProposerLastRecovery: make(map[string]time.Time),
		peerChallenges:              make(map[string]peerChallenge),
		replayedBlocks:              make(map[string]bool),
		replayFailures:              make(map[string]replayFailureState),
		equivocationIndex:           make(map[string]string),
		newBlockSubs:                make(map[chan struct{}]struct{}),
		proposerBreaker:             newBoundedBreaker(proposerBreakerFailThreshold, proposerBreakerCooldown, proposerBreakerReopenProbes, maxTrackedProposers),
		lastSeenFromValidator:       make(map[string]int64),
		lastMergedFromValidator:     make(map[string]int64),
		unverifiedStubHeights:       make(map[string]int64),
		stateRootMismatches:         make(map[string]int),
		stateRootMismatchLastAt:     make(map[string]int64),
		orphans:                     make(map[string][]*Block),
		orphanFirstSeen:             make(map[string]time.Time),
		orphanLastAttempt:           make(map[string]time.Time),
		orphanAttempts:              make(map[string]int),
		finalityWalkGaps:            make(map[string]bool),
		produceStuckGaps:            make(map[string]bool),

		producedHeights:       make(map[int64]bool),
		softRetryBlocks:       make(map[string]*Block),
		softRetryFirstAt:      make(map[string]time.Time),
		lastDeepScanAt:        make(map[string]int64),
		deepScanResumeHeight:  make(map[string]int64),
		deepScanFloorOverride: make(map[string]int64),
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
			// Normally already reflected in chain_accounts (committed when
			// these TXs were first applied, before this block was even
			// assembled) — must not be re-applied by replayTransactions. But
			// if replayed is false, this header was saved before its replay
			// completed and the process died mid-replay (see
			// ensureReplayedColumn's comment) — queue it for
			// repairUnreplayedBlocks() instead of trusting it.
			if b.Replayed {
				dag.replayedBlocks[b.Hash] = true
			}
			// else: leave it out of replayedBlocks. Not appended to
			// unreplayedAtBoot here — LoadUnreplayedBlocksFromDB below finds
			// every such row directly, unbounded by this loop's startupLoadWindow
			// (a repair candidate can be arbitrarily far behind the current tip).
			for _, ph := range b.ParentHashes {
				referenced[ph] = true
			}
			if b.Height > dag.height {
				dag.setHeight(b.Height)
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

	// LoadUnreplayedBlocksFromDB is a SEPARATE, unbounded query (not
	// limited by startupLoadWindow above) — a block needing repair can be
	// arbitrarily far behind the current tip (confirmed live: Contabo1's
	// one broken block was thousands of blocks behind by the time this was
	// diagnosed, well outside the RAM-bounded recent-blocks load). See
	// repairUnreplayedBlocks' own comment for why this can't run yet
	// (dag.evm isn't set until after NewBlockchain returns).
	if unrep, err := state.LoadUnreplayedBlocksFromDB(); err != nil {
		fmt.Printf("[BLOCK] Warning: could not check for unreplayed blocks: %v\n", err)
	} else if len(unrep) > 0 {
		dag.unreplayedAtBoot = unrep
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
			dag.setHeight(h)
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
		dag.finalityWalkGaps = make(map[string]bool)
		dag.produceStuckGaps = make(map[string]bool)
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
					dag.setHeight(cp.Height)
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
					dag.seedCheckpointParentStubsLocked(cp)
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
		dag.setHeight(maxH)
	}
}

// BootHeight returns the boot height (the DB-persisted chain height or
// snapshot import height, whichever is larger) — the frontier below which
// replayTransactions already encodes state and blocks need not be re-applied.
// ownsProducedHeight reports whether THIS process produced a block at height.
// Nil-safe so test helpers that build a BlockDAG literal (rather than going
// through NewBlockchain) behave as "produced nothing", which is the correct
// conservative answer.
func (dag *BlockDAG) ownsProducedHeight(height int64) bool {
	dag.producedHeightsMu.Lock()
	defer dag.producedHeightsMu.Unlock()
	return dag.producedHeights[height]
}

// noteProducedHeight records that this process durably produced height.
func (dag *BlockDAG) noteProducedHeight(height int64) {
	dag.producedHeightsMu.Lock()
	defer dag.producedHeightsMu.Unlock()
	if dag.producedHeights == nil {
		dag.producedHeights = make(map[int64]bool)
	}
	dag.producedHeights[height] = true
}

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
		Humans:    0,
		IsGenesis: true,
	}
	genesis.Hash = dag.calculateHash(genesis)
	genesis.BlueScore = 0
	genesis.SelectedParent = ""
	genesis.Blues = nil
	dag.blocks[genesis.Hash] = genesis
	dag.tips[genesis.Hash] = true
	dag.setHeight(0)
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
	// The commitment to this block's transactions.
	//
	// AN ATTACHED BODY ALWAYS WINS. b.TxRoot is consulted ONLY when the block
	// carries no transactions at all, and that asymmetry is load-bearing
	// security rather than a style choice. This hash is what the proposer signs
	// and what AddPeerBlock re-derives and compares (integrity check 1) before
	// accepting any peer block. If a declared TxRoot could override an attached
	// body, a peer could keep a genuine block's signed root, swap the
	// transaction list for one of its own, and still pass BOTH the hash check
	// and the signature check — arbitrary forged transfers riding on a validly
	// signed block. Deriving from the body whenever one is present keeps the
	// hash binding whatever will actually be replayed, so a swapped body simply
	// fails to hash to the signed value and is rejected.
	//
	// Roadmap step 4 (see tx_batch.go): when no body is attached, the declared
	// digest is used — and THAT is what lets a block travel without its
	// transactions while hashing identically, because the preimage below always
	// contained only this one digest, never the transaction list. A stripped
	// block and the same block carrying its body therefore produce the same
	// hash, with no activation height and no fork. AttachTxBatch independently
	// re-checks a fetched body against this same signed root before attaching
	// it, so on the by-reference path the body is verified twice over.
	//
	// A block carrying neither (produced before the field existed) derives the
	// root from its transactions exactly as before, keeping every historical
	// hash reproducible.
	//
	// Normalize nil to empty slice so JSON always produces "[]" not "null".
	// omitempty on the Transactions field strips the key during HTTP transport,
	// and the receiver deserialises to nil — without this normalisation the
	// tx_root differs between producer and receiver, causing hash mismatches.
	var txRoot string
	if len(b.Transactions) == 0 && b.TxRoot != "" {
		txRoot = b.TxRoot
	} else {
		txs := b.Transactions
		if txs == nil {
			txs = []Transaction{}
		}
		txData, _ := json.Marshal(txs)
		txRootBytes := sha256.Sum256(txData)
		txRoot = hex.EncodeToString(txRootBytes[:])
	}
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
	//
	// FIX (P0, 2026-07-10 — closes the one gate in this function that had no
	// escape valve): every gate below this one that can block production
	// indefinitely (the trusted-seed catch-up gate and the syncTargetHeight
	// gate, both further down) explicitly falls through once
	// dag.lastSuccessfulPeerSyncAt has been silent for syncStallTimeout — "a
	// downed/unreachable primary must not halt this node forever" is the
	// documented rationale for both. This gate predates that pattern and never
	// got it: if dag.height ever stops climbing while bootHeight stays fixed
	// above it — the exact shape a stuck sync loop (of any cause, not only the
	// deepScan-monopolization incident this same session also fixed in
	// sync_blocks.go) produces — this unconditional check blocked production
	// forever, with no recovery short of an operator restart, unlike its two
	// siblings immediately below. Same reference time and same threshold as
	// those two gates, for the identical reason: only trip once there has been
	// genuinely NO sync progress at all for a long stretch, never merely
	// because catch-up is still in progress and actively succeeding.
	if dag.bootHeight > 0 && dag.height+10 < dag.bootHeight {
		referenceTime := dag.lastPeerActivityAt()
		if referenceTime == 0 {
			referenceTime = dag.startupTime
		}
		if time.Now().Unix()-referenceTime < syncStallTimeout {
			fmt.Printf("[BLOCK] ⏳ Catch-up in progress (dag.height=%d, bootHeight=%d) — skipping block production\n",
				dag.height, dag.bootHeight)
			return nil
		}
		// else: no sync progress at all for syncStallTimeout — peers may be
		// unreachable → produce independently rather than halt forever, same
		// escape hatch as the two gates below.
	}

	// FIX (P0, 2026-07-10 — root cause of Contabo1 forking within its first
	// 30-45 blocks after every RESYNC_FROM_SNAPSHOT boot): every earlier
	// version of the gate below compares dag.height against some target — but
	// dag.height is exactly the field THIS node's own self-production also
	// advances. The instant that gate first opens even briefly, self-
	// production and the (correctly, continuously refreshed) target both then
	// climb at the same ~1-block/BLOCK_TIME pace, so "dag.height >= target-10"
	// stays satisfied forever after — a self-sustaining equilibrium with no
	// relation to whether genuine peer catch-up ever finished. Confirmed live:
	// even peerSyncHeight (immune to self-production, unlike raw dag.height)
	// didn't close this, because doSyncOnce deliberately advances it for every
	// block it SEES in a fetched page regardless of whether AddPeerBlock could
	// actually merge it. A fixed wall-clock minimum was tried next and also
	// confirmed insufficient: it bounds nothing about whether catch-up actually
	// finished, only how long it was blindly assumed to take — a slower cycle
	// (confirmed live: peer temporarily unreachable mid-boot) just let the
	// timer run out before catch-up was done, reproducing the same fork.
	//
	// hasCaughtUpWithAllPeers is a direct, positive signal instead of a proxy:
	// true only once doSyncOnce has completed cleanSyncStreakThreshold
	// CONSECUTIVE cycles against EVERY trusted seed with nothing left unmerged
	// (see cleanSyncStreak's own struct comment) — the actual condition this
	// gate needs, self-paced to however long real catch-up genuinely takes
	// rather than a guessed number.
	// FIX (P0, found live minutes after this shipped): bootHeight is set from
	// the DB's own persisted max_block_height on EVERY restart, not only a
	// RESYNC_FROM_SNAPSHOT boot (see bootHeight's own field comment) — so this
	// gate was firing for the PRIMARY node too after an ordinary redeploy.
	// Primary has no PRIMARY_NODE_URL/PRIMARY_NODE_URLS configured (it's the
	// seed, not a syncer — see StartPeerDiscovery's own "no PRIMARY_NODE_URL
	// configured" branch), so dag.trustedSeeds is permanently empty for it, and
	// hasCaughtUpWithAllPeers correctly refuses to call an empty seed list
	// "caught up" (that's the vacuous-truth trap it exists to avoid for a
	// secondary node whose discovery just hasn't run yet). Combined, the two
	// correct behaviors silenced Primary as a producer indefinitely: it kept
	// successfully receiving peer blocks (so the syncStallTimeout escape below
	// never triggered — that only fires on NO progress) while permanently
	// unable to produce (nothing it could ever be "caught up with"). Only
	// require this gate when there is genuinely something to catch up with.
	if dag.bootHeight > 0 && len(dag.trustedSeeds) > 0 {
		seeds := make([]string, 0, len(dag.trustedSeeds))
		for s := range dag.trustedSeeds {
			seeds = append(seeds, s)
		}
		if !dag.hasCaughtUpWithAllPeers(seeds) {
			// Same safety valve as the syncTargetHeight gate below, and for the
			// same reason: unlike that gate, hasCaughtUpWithAllPeers has no
			// timeout of its own, so a genuinely-unreachable seed (not just a
			// slow one) would otherwise block this node from ever producing —
			// this node's own liveness must not depend on a peer that may never
			// come back. lastSuccessfulPeerSyncAt reflects genuine progress
			// against ANY peer, so this only fires when NOTHING is getting
			// through, not merely because one of several configured seeds is
			// down while another still feeds real progress.
			referenceTime := dag.lastPeerActivityAt()
			if referenceTime == 0 {
				referenceTime = dag.startupTime
			}
			if time.Now().Unix()-referenceTime < syncStallTimeout {
				noteGateSkip()
				fmt.Printf("[BLOCK] ⏳ Not yet %d consecutive clean sync cycles with every trusted seed — skipping block production regardless of height-based gates\n",
					cleanSyncStreakThreshold)
				return nil
			}
			// else: no sync progress at all for syncStallTimeout — fall through,
			// same escape hatch as the gate below.
		}
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
			referenceTime := dag.lastPeerActivityAt()
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
	//
	// FIX (P0, 2026-07-10 — the actual root cause behind the produceStuckGaps
	// fix still not restoring production on Contabo1): a tip whose height has
	// fallen behind the finalized checkpoint can never become canonical again
	// — finality means the selected-parent chain already irreversibly grew
	// past that height via a different path, so a competing tip stuck below
	// it is provably a dead branch. Previously EVERY entry in dag.tips was a
	// merge-parent candidate forever, sorted only by BlueScore — so once one
	// of this node's own tips lost a race and stalled (nothing will ever
	// build on a doomed branch again, so it can never regain BlueScore),
	// every subsequent ProduceBlock tick still tried to merge it in. Confirmed
	// live: a Contabo1 tip stuck at height 675762, 9631 blocks behind the
	// finalized checkpoint at 685393, whose own parent is genuinely gone from
	// every peer's memory (correctly so — it predates their own finality
	// pruning). produceStuckGaps' active fetching could never resolve that:
	// the ancestor really is gone. Excluding sub-finality tips here stops
	// ProduceBlock from ever walking into that dead branch again; the tip
	// itself is left in dag.tips untouched (only this call's parent
	// candidates are filtered) since nothing here owns broader tip pruning.
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	type tipEntry struct {
		hash      string
		blueScore int64
	}
	allTips := make([]tipEntry, 0, len(dag.tips))
	for hash := range dag.tips {
		score := int64(0)
		if b, ok := dag.blocks[hash]; ok {
			if finalizedHeight > 0 && b.Height < finalizedHeight {
				continue
			}
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
	// FIX (performance audit 2026-07-06): dispatched through produceBlockPool
	// (workerpool.go) — 2 persistent workers reused every block (BLOCK_TIME
	// cadence, i.e. continuously for the node's whole lifetime) instead of
	// spawning 2 fresh goroutines per tick. Exactly 2 workers because exactly
	// 2 jobs are submitted per tick and both are always awaited before the
	// next tick's ProduceBlock call, so the pool is idle again before it's
	// needed next — same concurrency shape as before, just without the
	// per-tick goroutine spinup. Audit flagged this as low-effect (Go
	// goroutine creation is already cheap), kept for consistency with the
	// same class of fix applied elsewhere.
	produceBlockPool.submit(func() {
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
			// Kleinere Bloecke, wenn ein Peer nicht mehr hinterherkommt --
			// siehe peer_lag_bremse.go. Ohne das produziert dieser Knoten
			// dauerhaft mehr, als der andere nachvollziehen kann, und der
			// faellt zurueck, bis er minutenlang steht.
			dbTxs, pendingTxIDs = dag.state.LoadPendingTxsWithLimit(dag.blockTxCap())
			// Die tatsaechliche Groesse merken -- sie ist der Ankerpunkt, von
			// dem aus die Bremse beim naechsten Block drosselt.
			MerkeBlockGroesse(len(dbTxs))
		}
		pendingDur = time.Since(t0)
	})
	produceBlockPool.submit(func() {
		defer cadenceWG.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[PANIC RECOVERED] ProduceBlock StateRoot goroutine: %v\n%s\n", r, debug.Stack())
			}
		}()
		t0 := time.Now()
		stateRoot = dag.state.StateRoot()
		rootDur = time.Since(t0)
	})
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

	// FIX (P0, 2026-07-10 — Primary false-positive-equivocation-ban incident):
	// a brief window where an OLD and NEW instance of this exact validator run
	// simultaneously (e.g. a rolling redeploy that briefly overlaps the
	// outgoing and incoming container) can otherwise have BOTH instances
	// independently produce the "first" block from the identical pre-restart
	// tip, seconds apart — same height, same parents, same BlueScore, only the
	// timestamp (and therefore hash) differing. Every OTHER node's
	// checkAndIndexEquivocation (slashing.go) correctly flags that as this
	// validator signing two different blocks for the same parent set — because
	// from the network's perspective, that's exactly what happened. Confirmed
	// live: this exact pattern (identical height/parents/BlueScore/state_root,
	// only signature+timestamp differing) hit Primary at least four times
	// (2026-07-05, 07-08, 07-10, and again 07-12 — evidence in
	// equivocation_evidence on Contabo1/Contabo2); the 07-12 recurrence actually
	// triggered a real 14-day suspension on BOTH secondaries simultaneously
	// (cleared manually after confirming via evidence-pair comparison it was
	// this exact benign pattern, not real malicious equivocation).
	//
	// FIX (P0, 2026-07-12 — widened after the 07-12 recurrence): the original
	// version of this guard only checked the SINGLE literal first tick
	// (maxParentHeight+1 == bootHeight+1). The 07-12 incident's two colliding
	// blocks were produced 21 SECONDS apart — the outgoing instance was still
	// alive and still producing well past this process's first tick, so a
	// one-shot check missed it (by the time this process's first tick ran, the
	// outgoing instance's competing row for that exact height may not even
	// have been durably committed yet either — a narrow race the one-shot
	// version didn't close on top of the sustained-overlap gap). Widened from
	// "check once, at one height" to "check on every tick, for whatever height
	// is about to be produced, during a grace window after boot" — covers a
	// sustained overlap, not just an instant one. dag.state.HasBlockFromProposerAtHeight
	// hits the DURABLE store (chain_blocks), not this fresh process's own
	// (necessarily empty-for-this-height) in-memory dag.blocks, so it can see
	// what an outgoing sibling instance already committed moments ago even
	// though this instance never received it directly. Skip this tick rather
	// than mint a competing duplicate; ordinary peer sync pulls the other
	// instance's already-broadcast block in within the next tick or two.
	//
	// FIX (P0, 2026-07-24 — fifth recurrence, this time escalating the primary
	// to a real 2nd offense: 90-day suspension PLUS the 50 AEQ penalty, not
	// just the no-balance-loss 1st-offense grace): the 07-12 fix's 45-second
	// grace window was still too short — confirmed live again the same day
	// this comment was written, on the very redeploy triggered by pushing that
	// day's other merge-reliability fixes. Guessing a bigger constant would
	// just repeat the same mistake, because the real answer (how long the
	// hosting platform's rolling-deploy overlap can last) depends on Railway's
	// own health-check/traffic-drain timing, which this process has no way to
	// observe or bound. The check itself is a single cheap indexed EXISTS
	// lookup that costs nothing once this validator's own tips are current —
	// the overwhelmingly common case, and the only one steady-state production
	// ever hits — so there was never a real correctness or performance reason
	// to time-box it at all. Window removed entirely: this now runs on every
	// tick, for the process's whole life, closing the incident class instead
	// of re-guessing a magic number the next slow deploy will exceed again.
	//
	// REVERTED (P0, 2026-07-24, hours later — that reasoning was wrong and it
	// halted the primary): removing the window did not make the check
	// "harmless but always on", it made it PERMANENT, and the condition it
	// tests is routinely true in ordinary steady-state operation. Once this
	// node's own tip selection lags even one height behind what it has already
	// durably stored — which happens constantly under normal merging —
	// maxParentHeight+1 names a height where this validator's own block from a
	// moment ago is already in chain_blocks. The guard then fires on EVERY
	// tick and the node never produces again. Confirmed live from the
	// primary's own Railway log, once per second, indefinitely:
	//
	//	[BLOCK] ⏸ Skipping production at height 1774182 — this validator
	//	already has a block there in the durable store (likely a concurrent
	//	instance from a redeploy overlap, 1m34s into this process's life)
	//
	// "1m34s into this process's life" is the tell: well past any redeploy
	// overlap, in normal operation, blocking production for good.
	//
	// The window was never a magic number to be guessed away — it is what
	// makes "I have a block here that I did not produce this run" evidence of
	// an overlapping instance rather than a description of normal operation.
	// Restored. The recurrence risk that motivated removing it is separately
	// and much better handled now: a secondary no longer suspends its own
	// BOOTSTRAP_SIGNER over a self-collision at all (see AddPeerBlock's
	// equivocation goroutine, same date), so a missed overlap costs a
	// duplicate block the DAG merges anyway, not a 14-day network partition.
	// FIX (P1, Audit 2026-08-18 — the window is gone, and this time it stays
	// gone): the 45-second window was never the right control, it was a proxy
	// for one. The revert above is right that "chain_blocks already has a block
	// from me at this height" is routinely true in ordinary operation — but
	// only because this node wrote that block ITSELF, moments ago, in this same
	// process. The condition that actually means "another instance of me is
	// running" is narrower: a durable block from this validator at this height
	// that THIS PROCESS did not write.
	//
	// producedHeights (see its own comment) supplies the missing half. It turns
	// the guard from a time-boxed guess into an exact statement, so it can run
	// for the process's whole life without ever firing on the node's own work —
	// which is what halted the primary on 2026-07-24 and forced the window back.
	//
	// Why this matters beyond tidiness: the fallback that made a missed overlap
	// survivable — a node no longer suspending its own signer over a
	// self-collision — is deliberately scoped to BOOTSTRAP_SIGNER alone (see
	// the equivocation goroutine in AddPeerBlock). It protects this
	// deployment's own validators and nobody else's. A third-party operator
	// whose rolling deploy overlaps by more than 45 seconds was suspended by
	// every other node for doing nothing wrong. This project's premise is that
	// any registered human can run a validator, so a guard that only covers the
	// operator's own two boxes is not the guard it needs.
	if dag.state != nil && !dag.ownsProducedHeight(maxParentHeight+1) &&
		dag.state.HasBlockFromProposerAtHeight(proposer, maxParentHeight+1) {
		fmt.Printf("[BLOCK] ⏸ Skipping production at height %d — the durable store already holds a block from this validator there, and this process did not write it. That means a second instance of this validator is running (a redeploy overlap, %s into this process's life). Waiting for ordinary peer sync to pull it in instead of minting a conflicting duplicate every other node would correctly read as equivocation.\n", maxParentHeight+1, time.Since(dag.bootTime).Round(time.Second))
		return nil
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
	// Declare the body digest explicitly (roadmap step 4, tx_batch.go). This does
	// not change the hash — with a body attached, calculateBlockHash derives the
	// root from the transactions and ignores this field entirely, exactly as it
	// always did. What it buys is that the block can later be stripped of its
	// transactions for transport and still hash to this same value, and that the
	// digest travels with the header so a receiver knows what body to ask for.
	block.TxRoot = txBatchRoot(txs)
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
	//
	// FIX (P0, 2026-07-10): parentHashes were chosen from dag.tips moments ago
	// (already locally present), so a genuinely-unresolvable DEEPER ancestor was
	// assumed rare here — but not impossible (e.g. one of our own tips itself
	// inherited an unresolved-at-the-time reference). Unlike a peer block,
	// there is nothing to "queue as an orphan": this block doesn't exist yet.
	// Skip producing this tick and let the next tick retry.
	//
	// FIX (P0, 2026-07-10 — found live within minutes of the above shipping):
	// that assumption was wrong in one specific way that turns "rare" into
	// "permanent": a hash can be genuinely, PERMANENTLY lost from the entire
	// network's memory (confirmed earlier this session for the finality-
	// checkpoint walk — see registerFinalityWalkGap's own comment for that
	// class of incident), not just transiently in flight. Skip-and-retry alone
	// has no escape from that case: the same unresolvable hash reappears every
	// single tick forever, halting this node's own production indefinitely —
	// confirmed live, 1000+ consecutive ticks stuck on the identical hash.
	//
	// FIX (P0, 2026-07-10 — third revision, found live via 620-783 accumulated
	// stubs on the two secondary production nodes): the first revision here
	// bridged after a raw 5-consecutive-tick counter (~5s at BLOCK_TIME=1s) with
	// NO active fetch ever registered for the missing hash — it just passively
	// hoped ordinary gossip would deliver it in time. queueOrphan's own
	// identical-purpose runtime bridge, by contrast, only bridges after
	// orphanAbandonAfter (15 minutes) AND minOrphanAttemptsBeforeAbandon (3)
	// genuine peer-fetch attempts AND !isCatchingUp AND an explicit
	// ALLOW_RUNTIME_ORPHAN_BRIDGE opt-in — see that function's "abandon"
	// comment. The 5-second version had none of that: it fired constantly for
	// perfectly ordinary cross-validator propagation lag (ancestors that would
	// have resolved fine within a few more seconds if only something had asked
	// a peer for them), fabricating hundreds of permanent stub entries whose
	// BlueScore offset (even with safeStubBlueScoreLocked) can never self-heal
	// once committed. Now: register the hash via registerProduceStuckGap so
	// fetchMissingAncestors (already polling every ~1s per active sync peer)
	// actively tries it, and only bridge once produceStuckGapReady says this
	// hash has genuinely exhausted the SAME patient standard every other
	// synthetic-checkpoint stub site already holds itself to.
	missing, ok := dag.computeGHOSTDAGState(block)
	if !ok {
		dag.registerProduceStuckGap(missing)
		if !dag.produceStuckGapReady(missing) {
			fmt.Printf("[BLOCK] ⏳ Skipping production this tick — merge-set ancestor %s... not yet resolvable, actively fetching from peers\n",
				missing[:min(16, len(missing))])
			return nil
		}
		// FIX (P0, 2026-07-10 — found live via the explorer UI within minutes of
		// this bridge shipping): BlueScore=0 made the stub look like it belonged
		// at the very start of the chain's history. block itself then computed
		// its own BlueScore as roughly stub.BlueScore+1 — near zero, light-years
		// below the chain's real accumulated score — silently replacing this
		// node's canonical chain with a permanently wrong-scored fork built on
		// fabricated history (confirmed live: dropped from ~3.58M to ~1300).
		// safeStubBlueScoreLocked anchors the stub to the current known
		// frontier instead, so this block's own score stays roughly where the
		// real chain already is. Height mirrors queueOrphan's own stub
		// convention (minWaitingHeight-1, i.e. "immediately before whatever
		// needed it") rather than a hardcoded 0 for the same reason.
		fmt.Printf("[BLOCK] ⚠ Merge-set ancestor %s... still unresolvable after %s and %d peer-fetch attempt(s) — bridging with a synthetic checkpoint stub so production can continue (same class of genuinely-lost-block gap already handled elsewhere; no effect on account balances)\n",
			missing[:min(16, len(missing))], orphanAbandonAfter, minOrphanAttemptsBeforeAbandon)
		stubHeight := block.Height - 1
		if stubHeight < 0 {
			stubHeight = 0
		}
		dag.blocks[missing] = &Block{Hash: missing, Height: stubHeight, BlueScore: dag.safeStubBlueScoreLocked(), Proposer: "synthetic-checkpoint", ParentHashes: []string{}}
		dag.syntheticCheckpointCount.Add(1)
		if stubHeight > dag.bootHeight {
			dag.unverifiedSyntheticCheckpointCount.Add(1)
			dag.unverifiedStubHeights[missing] = stubHeight
		}
		if dag.state != nil {
			SafeGoroutine("RecordSyntheticCheckpointEvent-producestuck", func() { dag.state.RecordSyntheticCheckpointEvent(missing, stubHeight, "produce-block-stuck") })
		}
		dag.clearProduceStuckGap(missing)
		missing, ok = dag.computeGHOSTDAGState(block)
		if !ok {
			// The bridge resolved one hash but the walk hit a second, different
			// one — extremely unlikely (would need multiple independent gaps in
			// the same merge-set walk) but stay safe: skip this tick rather than
			// loop, the registration above will let that new hash reach the
			// same patient bridge on its own schedule.
			dag.registerProduceStuckGap(missing)
			fmt.Printf("[BLOCK] ⏳ Skipping production this tick — merge-set ancestor %s... not yet resolvable\n", missing[:min(16, len(missing))])
			return nil
		}
	}

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
	// Index this block's transactions for wallet lookups, exactly as the replay
	// path does for peer blocks — a transaction must resolve to its real block
	// no matter which node produced it or which node the wallet asks. See
	// tx_block_index.go. Non-fatal: the block is already durably saved.
	// Asynchronous: one row per transaction, up to maxTxsPerBlock of them,
	// and this is not consensus -- see tx_block_index_async.go. This call
	// site already treated a failure here as non-fatal, so deferring it is
	// weaker than what the code already tolerated.
	dag.state.IndexBlockTransactionsAsync(block.Height, block.Hash, block.Transactions)
	// Keep the body retrievable by digest so this node can serve it to a peer
	// that received the block stripped of its transactions (roadmap step 4,
	// tx_batch.go). Must happen before the broadcast below, or a peer could ask
	// for a body we have not stored yet. Non-fatal: a failure here only means
	// peers get the block with its body inline, as they always did.
	if err := dag.state.SaveTxBatch(block.TxRoot, block.Transactions); err != nil {
		fmt.Printf("[BLOCK] ⚠ Could not store the transaction body of block #%d for by-reference serving: %v\n", block.Height, err)
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
	// This process now owns this height — see producedHeights' own comment.
	// Recorded only after SaveBlockWithPendingTxsAtomic above returned without
	// error, so the map can never claim a height whose block was not actually
	// persisted; an unpersisted height must stay "not mine" so the guard still
	// fires for it on the next tick.
	dag.noteProducedHeight(block.Height)
	// GHOSTDAG already computed above (P1-04); no second call needed.
	dag.replayedMu.Lock()
	dag.replayedBlocks[block.Hash] = true
	dag.replayedMu.Unlock()

	// Remove all parents from tips, add this block as new tip
	for _, ph := range parentHashes {
		delete(dag.tips, ph)
	}
	dag.tips[block.Hash] = true
	dag.setHeight(block.Height)
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

	dag.notifyNewBlock(block)
	// Admission control's one input: this node can turn work into blocks right
	// now. See admission_control.go for why time-since-a-block is the signal
	// rather than queue depth.
	noteBlockProduced()
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

// unknownProposerRecoveryRetry is how long to wait before asking the peers
// again about a proposer this node still does not recognise. Short enough that
// a node which lost its one chance during a restart recovers within a minute
// rather than never; long enough that a genuinely forged proposer address
// hammering the node costs one validator-list sync per minute, not one per
// block. Only ever reached while blocks from that proposer keep arriving, so a
// proposer that goes away stops costing anything at all.
const unknownProposerRecoveryRetry = 60 * time.Second

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
	// FIX (P0, 2026-07-10 — root cause of Contabo1's permanent non-convergence
	// incident): while THIS node is still in its own initial catch-up
	// (isCatchingUpLocked — same signal AddPeerBlock's far-ahead breaker
	// already trusts for an identical judgment call), a 15-minute orphan
	// silence is not evidence the parent is unresolvable — it's evidence this
	// node is currently too loaded to reach it in time. Confirmed live: with
	// dag.mu serializing catch-up's own AddPeerBlock calls against concurrent
	// ProduceBlock/push/deepScan work, curl-ing the "missing" parent directly
	// from the peer returned it instantly and correctly the whole time — nothing
	// was actually gone. Bridging it into a synthetic-checkpoint stub anyway
	// added one more permanently-fake block AND one more dag.tips entry, making
	// every subsequent GHOSTDAG merge-set computation heavier and catch-up
	// slower still — a self-reinforcing spiral that fabricated 469,599 stubs
	// (chain_blocks held only 57,884 real rows) while a healthy sibling node
	// bootstrapped from the identical snapshot at the identical time finished
	// clean. Skipping the bridge here does not lose the block: it stays queued
	// and fetchMissingAncestors keeps retrying it every cycle regardless: the
	// deferred abandon-to-stub check below still applies in full once this node
	// is no longer the bottleneck.
	catchingUp := dag.isCatchingUp()
	dag.orphansMu.Lock()
	now := time.Now()
	if first, ok := dag.orphanFirstSeen[missingParent]; ok {
		if !catchingUp && now.Sub(first) > orphanAbandonAfter && dag.orphanAttempts[missingParent] >= minOrphanAttemptsBeforeAbandon {
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
				// FIX (P0, 2026-07-10 — found live via the explorer UI
				// within minutes of an equivalent hardcoded-0 bridge
				// shipping in ProduceBlock, same underlying hazard here):
				// BlueScore=0 made this stub look like it belonged at the
				// very start of the chain's history. Any real block built
				// with it as an ancestor computed its own BlueScore as
				// roughly stub.BlueScore+1 -- silently replacing this
				// node's canonical chain with a permanently wrong-scored
				// fork built on fabricated history. See
				// safeStubBlueScoreLocked's own comment for why frontier-1,
				// not 0 and not a height-derived estimate (that direction
				// was already tried and reverted for BridgeHistoricalGap,
				// see its own comment) is the fix for this MID-CHAIN case.
				dag.blocks[missingParent] = &Block{
					Hash:         missingParent,
					Height:       stubH,
					BlueScore:    dag.safeStubBlueScoreLocked(),
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
// know exactly which specific ancestor blocks to fetch by hash, AND by
// doSyncOnce's wantDeepScan trigger — deliberately does NOT include
// finalityWalkGaps, see PendingFetchHashes' own comment for why that
// distinction matters.
func (dag *BlockDAG) MissingParentHashes() []string {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	hashes := make([]string, 0, len(dag.orphans))
	for h := range dag.orphans {
		hashes = append(hashes, h)
	}
	return hashes
}

// clearFinalityWalkGap removes hash from finalityWalkGaps once it's resolved
// (found locally) — see PendingFetchHashes' call site (sync_blocks.go) for
// why this cleanup lives here rather than piggybacking on the orphan
// resolution path: finalityWalkGaps entries were never real orphans.
func (dag *BlockDAG) clearFinalityWalkGap(hash string) {
	dag.orphansMu.Lock()
	delete(dag.finalityWalkGaps, hash)
	dag.orphansMu.Unlock()
}

// registerProduceStuckGap marks hash as something ProduceBlock's own
// stuck-ancestor escape hatch is waiting on, so fetchMissingAncestors
// (already running on a ~1s ticker per active sync peer) actively tries to
// fetch it from a peer instead of ProduceBlock passively hoping ordinary
// gossip delivers it in time. Reuses orphanFirstSeen (shared, source-
// agnostic — queueOrphan and registerFinalityWalkGap already write into it
// the same way) so age tracking is consistent across all three gap sources.
// See produceStuckGaps' own field comment for why this is a separate map
// from dag.orphans.
func (dag *BlockDAG) registerProduceStuckGap(hash string) {
	dag.orphansMu.Lock()
	if !dag.produceStuckGaps[hash] {
		dag.produceStuckGaps[hash] = true
		if _, seen := dag.orphanFirstSeen[hash]; !seen {
			dag.orphanFirstSeen[hash] = time.Now()
		}
	}
	dag.orphansMu.Unlock()
}

// clearProduceStuckGap removes hash from produceStuckGaps once it's resolved
// or bridged — mirrors clearFinalityWalkGap.
func (dag *BlockDAG) clearProduceStuckGap(hash string) {
	dag.orphansMu.Lock()
	delete(dag.produceStuckGaps, hash)
	dag.orphansMu.Unlock()
}

// produceStuckGapReady reports whether hash has waited long enough, with
// enough genuine fetch attempts, for ProduceBlock to give up on ordinary
// resolution and bridge it with a synthetic-checkpoint stub — the exact same
// orphanAbandonAfter/minOrphanAttemptsBeforeAbandon standard queueOrphan's
// own runtime bridge already holds itself to (see that function's "abandon"
// comment), plus the same isCatchingUp carve-out (a loaded catch-up node
// reaching a peer slowly is not evidence the ancestor is gone — see
// queueOrphan's 2026-07-10 FIX comment for the 469,599-stub incident that
// taught us this) and the same ALLOW_RUNTIME_ORPHAN_BRIDGE opt-in (bridging
// is a deliberate trust-bypass operators must enable, not a default).
//
// Caller must already hold dag.mu (its only caller, ProduceBlock, holds it
// write-locked for its entire call — see that function's top-of-body
// comment). Uses isCatchingUpLocked, NOT the isCatchingUp RLock wrapper:
// calling the wrapper here self-deadlocks, since sync.RWMutex is not
// reentrant — confirmed live within minutes of first deploying this (both
// redeployed nodes' /api/status hung completely, including from inside the
// container itself via wget, immediately after this code path first ran).
func (dag *BlockDAG) produceStuckGapReady(hash string) bool {
	if dag.isCatchingUpLocked() {
		return false
	}
	if os.Getenv("ALLOW_RUNTIME_ORPHAN_BRIDGE") != "true" {
		return false
	}
	dag.orphansMu.Lock()
	first, seen := dag.orphanFirstSeen[hash]
	attempts := dag.orphanAttempts[hash]
	dag.orphansMu.Unlock()
	return seen && time.Since(first) > orphanAbandonAfter && attempts >= minOrphanAttemptsBeforeAbandon
}

// PendingFetchHashes returns every hash fetchMissingAncestors should attempt
// this round: real orphans (dag.orphans) PLUS finality-checkpoint-walk gaps
// (dag.finalityWalkGaps) PLUS ProduceBlock's own stuck-ancestor gaps
// (dag.produceStuckGaps). Unlike MissingParentHashes, this is NOT used for
// wantDeepScan — see registerFinalityWalkGap's own comment for the live
// incident (permanent deepScan, 5000+ fragmented dag.tips) that resulted
// from finality gaps feeding into that trigger; produceStuckGaps is excluded
// from MissingParentHashes for the identical reason.
func (dag *BlockDAG) PendingFetchHashes() []string {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	hashes := make([]string, 0, len(dag.orphans)+len(dag.finalityWalkGaps)+len(dag.produceStuckGaps))
	for h := range dag.orphans {
		hashes = append(hashes, h)
	}
	for h := range dag.finalityWalkGaps {
		if _, alreadyIncluded := dag.orphans[h]; !alreadyIncluded {
			hashes = append(hashes, h)
		}
	}
	for h := range dag.produceStuckGaps {
		if _, alreadyIncluded := dag.orphans[h]; alreadyIncluded {
			continue
		}
		if dag.finalityWalkGaps[h] {
			continue
		}
		hashes = append(hashes, h)
	}
	return hashes
}

// hasAwaitingOrphan reports whether hash is currently, genuinely needed by
// something waiting on it right now, in the present tense — a queued
// orphan's missing-parent key, a finality-checkpoint-walk gap, or a
// ProduceBlock stuck-ancestor gap. See AddPeerBlock's bootHeight-skip call
// site for why the present-tense distinction matters: a SelfFetched
// delivery only proves a fetch was deliberately issued for a hash something
// needed WHEN THE REQUEST WENT OUT, not that the need still exists when the
// response arrives — a resync in between can clear the queue entry that
// originally justified the fetch.
//
// FIX (P0, 2026-07-19 — root cause of both secondaries permanently unable
// to advance their finality checkpoint or resume block production):
// registerFinalityWalkGap/registerProduceStuckGap deliberately write into
// their OWN separate maps instead of dag.orphans (see each one's own
// comment — folding them into dag.orphans would wrongly re-trigger
// wantDeepScan's fresh-orphan detection, the exact 2026-07-10 incident
// finalityWalkGaps was split out to fix). But this function used to check
// ONLY dag.orphans, so isFinalityViolation's SelfFetched exemption (and the
// identical bootHeight-skip gate) never recognized a checkpoint-walk-gap or
// produce-stuck-gap fetch as "genuinely awaited" — the fetch would succeed
// (confirmed live: the peer had the block), but AddPeerBlock rejected it
// anyway as too far below the finalized height to matter, permanently. That
// made any historical gap discovered by either mechanism unrecoverable by
// construction: the checkpoint could never walk past it, and — since an
// unresolved checkpoint keeps the finality floor from advancing, which
// keeps doSyncOnce from ever cheaply skipping that same old region as
// already-settled — every ordinary sync cycle kept re-attempting to merge
// deep into it instead, so cleanSyncStreak (hasCaughtUpWithAllPeers) never
// reached a clean pass either, silently halting new block production too.
// Confirmed live on both Contabo1 and Contabo2: each stuck on a distinct
// missing self-produced block from ~15,000 blocks back, present and
// fetchable from Primary the whole time, rejected on every single retry.
func (dag *BlockDAG) hasAwaitingOrphan(hash string) bool {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	return len(dag.orphans[hash]) > 0 || dag.finalityWalkGaps[hash] || dag.produceStuckGaps[hash]
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

// isCatchingUp is isCatchingUpLocked's unlocked wrapper (see
// farAheadFrontier/farAheadFrontierLocked for the identical pattern), for
// callers like queueOrphan that reach this check without already holding
// dag.mu.
// DAGGates reports the internal gates that decide whether this node attaches
// peer blocks and produces its own — WITHOUT taking dag.mu or cs.mu.
//
// Why lock-free matters (2026-07-26): the night this was written, the primary
// spent long stretches orphaning ~99% of incoming blocks (422 "queued as
// orphan" against 4 "Added peer block" in one log window) while /api/status
// and /api/health/combined intermittently took 11-16 SECONDS to answer. Every
// diagnostic available from outside either needed a lock that was already
// contended, or reported a symptom rather than which gate was actually shut.
// Three plausible causes were checked and disproved from logs alone — a
// running repair pass, expensive block replay, database errors (zero pq
// errors, zero bad connections) — and the one remaining candidate,
// ghostdagMigrationPending, could not be confirmed because the log window no
// longer reached back to boot, which is when the migration would have
// announced itself.
//
// ghostdagMigrationPending is the specific reason this exists. While it is
// set, ghostdagBlockLookup returns nil for EVERY hash not already resident in
// memory, skipping the DB fallback entirely — so every incoming block whose
// parent has been pruned from the in-memory window orphans, with no error
// logged and the block sitting readable in chain_blocks the whole time. That
// failure is invisible from the outside and indistinguishable from a genuine
// fork, which is exactly the confusion it caused.
//
// pprof would answer this too, but the primary runs on Railway: its pprof
// listener is deliberately localhost-only and there is no docker exec. So the
// node has to be able to say it itself.
func (dag *BlockDAG) DAGGates() map[string]interface{} {
	dag.orphansMu.Lock()
	orphanKeys := len(dag.orphans)
	finalityGaps := len(dag.finalityWalkGaps)
	produceGaps := len(dag.produceStuckGaps)
	waiting := 0
	for _, blocks := range dag.orphans {
		waiting += len(blocks)
	}
	dag.orphansMu.Unlock()

	return map[string]interface{}{
		// THE gate that silently disables the DB fallback in
		// ghostdagBlockLookup — see this function's own comment.
		"ghostdag_migration_pending": dag.ghostdagMigrationPending.Load(),
		"resync_in_progress":         dag.resyncInProgress.Load(),
		"sync_target_height":         dag.syncTargetHeight.Load(),
		"boot_height":                dag.BootHeight(),
		"height":                     dag.Height(),
		// Orphan bookkeeping: how many distinct parents are being waited on,
		// and how many blocks that is holding up.
		"orphan_missing_parents": orphanKeys,
		"orphan_blocks_waiting":  waiting,
		"finality_walk_gaps":     finalityGaps,
		"produce_stuck_gaps":     produceGaps,
	}
}

func (dag *BlockDAG) isCatchingUp() bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.isCatchingUpLocked()
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

// (ghostdagStuckThreshold removed 2026-07-10 — ProduceBlock's stuck-ancestor
// bridge now reuses orphanAbandonAfter/minOrphanAttemptsBeforeAbandon via
// produceStuckGapReady instead of its own much hastier raw tick counter.)

// proposerBlockBlocked reports whether a proposer's blocks should be dropped now
// WITHOUT taking dag.mu, because its breaker is open (still inside the cooldown).
// Called on AddPeerBlock's lock-free hot path; touches only proposerBreaker's
// own dedicated mutex (see boundedBreaker, breaker.go).
func (dag *BlockDAG) proposerBlockBlocked(proposer string) bool {
	if proposer == "" {
		return false
	}
	return dag.proposerBreaker.ShouldDrop(strings.ToLower(proposer))
}

// recordProposerOutcome feeds the per-proposer circuit breaker. attached=true
// (the block joined the DAG) clears the proposer's failure run; attached=false
// (it was rejected far-ahead or orphaned on a missing parent) advances the run
// and trips the breaker once it crosses proposerBreakerFailThreshold. Uses
// proposerBreaker's own dedicated mutex only — never dag.mu — and is always
// called with dag.mu released, so it can never invert the lock order against
// block production. The unbounded-map DoS protection this used to implement
// inline (P2-c, audit 2026-07-06 — proposer is read from an unauthenticated
// block BEFORE signature verification, so an attacker can trivially generate
// unlimited distinct proposer strings) now lives in boundedBreaker itself
// (breaker.go), via proposerBreaker.MaxTracked.
func (dag *BlockDAG) recordProposerOutcome(proposer string, attached bool) {
	if proposer == "" {
		return
	}
	dag.proposerBreaker.RecordOutcome(strings.ToLower(proposer), attached)
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
	n := dag.proposerBreaker.Clear()
	if n > 0 {
		fmt.Printf("[AUTO-HEAL] Cleared circuit-breaker state for %d proposer(s) after resync — stale counts from before the resync no longer apply to the new history.\n", n)
	}
}

// SubscribeNewBlocks registers a fresh subscriber for the /api/events SSE
// stream and returns its notification channel plus an unsubscribe func the
// caller MUST call (deferred) when the HTTP connection ends, or the channel
// leaks forever in newBlockSubs. Buffered (size 1): a subscriber who hasn't
// drained the previous notification yet doesn't need a second one queued —
// the client's reaction to "a new block exists" is to re-fetch the full
// current state, not to process one event per block.
func (dag *BlockDAG) SubscribeNewBlocks() (ch chan struct{}, unsubscribe func()) {
	ch = make(chan struct{}, 1)
	dag.newBlockSubsMu.Lock()
	dag.newBlockSubs[ch] = struct{}{}
	dag.newBlockSubsMu.Unlock()
	return ch, func() {
		dag.newBlockSubsMu.Lock()
		delete(dag.newBlockSubs, ch)
		dag.newBlockSubsMu.Unlock()
	}
}

// notifyNewBlock wakes every /api/events subscriber. Deliberately
// non-blocking (select+default): a subscriber whose channel is already full
// (hasn't been read since the last notification) is simply skipped this
// round rather than allowed to stall the caller — both call sites
// (ProduceBlock, AddPeerBlock) hold dag.mu and must never block on a slow
// HTTP client. This is a pure UX signal (subscribers still re-fetch via the
// existing REST endpoints), so a dropped notification costs nothing beyond
// that one client's SSE push waiting for the next block or its own polling
// fallback — never a correctness issue.
func (dag *BlockDAG) notifyNewBlock(block *Block) {
	dag.newBlockSubsMu.Lock()
	defer dag.newBlockSubsMu.Unlock()
	for ch := range dag.newBlockSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// prefetchParentsFromDB batch-loads any of block.ParentHashes not already
// resident in dag.blocks, via a SINGLE Postgres round trip, BEFORE
// AddPeerBlock takes dag.mu — see ghostdagBlockLookup's own "P0, 2026-07-10"
// comment for why its single-hash DB fallback (called for every ancestor
// computeGHOSTDAGState's merge-set BFS can't resolve from dag.blocks) can no
// longer be skipped for correctness, yet still runs synchronously WHILE
// dag.mu is held write-locked. Confirmed live (Contabo1/Contabo2 diagnostic
// session, 2026-07-24) as the actual mechanism behind the multi-minute
// block-attach queueing seen during a catch-up burst: right after a
// checkpoint-seeded resync, dag.blocks' in-memory window is mostly cold, so
// a burst of peer blocks each needing several ancestor lookups serializes
// the WHOLE node (ProduceBlock, every other AddPeerBlock, every dag.mu-
// touching API read) behind a chain of one-row-at-a-time DB queries.
//
// This does not remove any of those single-hash lookups — computeGHOSTDAGState
// can still need DEEPER ancestors beyond direct parents, which this cannot
// predict without running the BFS itself. It only pre-warms the common,
// dominant case (a block's DIRECT parents, exactly what integrity check 3
// below needs first, and what most of a fresh catch-up burst's misses
// actually are) with one batched query taken OUTSIDE the lock, so the
// in-lock path hits dag.blocks' fast, in-memory branch instead of the DB
// fallback for however many of those hashes this call manages to warm.
// Harmless if it races with something else populating dag.blocks first —
// re-checked under the write lock before inserting, and whichever value
// lands is the same trusted, already-validated chain_blocks row either way.
//
// Mirrors ghostdagBlockLookup's own migration-pending skip: a startup
// migration already refuses this exact class of DB-fallback storm (see that
// function's 2026-07-04 FIX comment) for a bounded, already-loaded backfill —
// prefetching here while that skip is active would just be extra unwanted DB
// load, not a correctness issue, but pointless.
func (dag *BlockDAG) prefetchParentsFromDB(block *Block) {
	if block == nil || dag.state == nil || len(block.ParentHashes) == 0 || dag.ghostdagMigrationPending.Load() {
		return
	}
	dag.mu.RLock()
	missing := make([]string, 0, len(block.ParentHashes))
	for _, ph := range block.ParentHashes {
		if _, ok := dag.blocks[ph]; !ok {
			missing = append(missing, ph)
		}
	}
	dag.mu.RUnlock()
	if len(missing) == 0 {
		return
	}
	found, err := dag.state.LoadBlocksByHashesFromDB(missing)
	if err != nil || len(found) == 0 {
		return // best-effort: the in-lock single-hash fallback still covers correctness
	}
	dag.mu.Lock()
	for _, b := range found {
		if _, ok := dag.blocks[b.Hash]; !ok {
			dag.blocks[b.Hash] = b
		}
	}
	dag.mu.Unlock()
}

// prefetchMergeSetFromDB extends prefetchParentsFromDB (which only warms
// dag.blocks for block's DIRECT parents) to the full, possibly-multi-hop
// ancestor set computeGHOSTDAGState's merge-set BFS (ghostdagMergeSet) is
// about to walk — called BEFORE dag.mu.Lock() so the DB round trips this
// warms away happen concurrently with the rest of the node's work instead
// of one-by-one while dag.mu is held.
//
// Confirmed live twice as the dominant per-block DB cost of that in-lock
// walk: ghostdagBatchPrefetch's own 2026-07-04 FIX comment measured ~2.6s/
// block from this exact traversal in production, and a goroutine dump taken
// 2026-07-24 during a real Contabo2 stall caught a sync goroutine mid-flight
// in AddPeerBlock → ghostdagBlockLookup → a single-hash Postgres query —
// this exact call chain, still running synchronously inside dag.mu despite
// prefetchParentsFromDB already covering direct parents, because that
// function's own comment already flagged the gap it deliberately left open:
// "computeGHOSTDAGState can still need DEEPER ancestors beyond direct
// parents, which this cannot predict without running the BFS itself." This
// runs that BFS — read-only, before the lock — to close exactly that gap.
//
// Deliberately does NOT replicate ghostdagMergeSet's spHash/exclusion-set
// split: it walks backward from ALL of block's direct parents together,
// which is a strict superset of what the real two-phase BFS needs (that BFS
// only walks non-SP parents past the SP-exclusion frontier), so a single
// unified walk necessarily fetches everything the real computation could
// touch. Bounded by the same structural caps (mergeDepthLimit,
// maxMergeVisits, maxGhostdagDBLookups) so it can never do unbounded work.
//
// This is a warm-up, never a correctness dependency: if a hash is still
// missing by the time computeGHOSTDAGState runs (DB error, exhausted
// budget, or a genuine race with a concurrent sibling still in flight),
// that function's own in-lock DB fallback resolves it exactly as if this
// function did not exist.
//
// Locking: dag.mu.RLock() for every dag.blocks membership check (concurrent-
// safe with readers and with this same function running for another peer's
// block), and a brief dag.mu.Lock() only to insert freshly fetched blocks —
// the same pattern ghostdagBatchPrefetch itself uses, just invoked before
// AddPeerBlock's own dag.mu.Lock() instead of during it.
func (dag *BlockDAG) prefetchMergeSetFromDB(block *Block) {
	if block == nil || dag.state == nil || len(block.ParentHashes) == 0 || dag.ghostdagMigrationPending.Load() {
		return
	}
	depthLimit := dag.mergeDepthLimit()
	visitCap := dag.maxMergeVisits()
	dbBudget := dag.maxGhostdagDBLookups()

	type entry struct {
		hash  string
		depth int
	}
	visited := make(map[string]bool, len(block.ParentHashes))
	frontier := make([]entry, 0, len(block.ParentHashes))
	for _, ph := range block.ParentHashes {
		if !visited[ph] {
			visited[ph] = true
			frontier = append(frontier, entry{ph, 0})
		}
	}

	for len(frontier) > 0 && len(visited) < visitCap && dbBudget > 0 {
		hashes := make([]string, len(frontier))
		for i, e := range frontier {
			hashes[i] = e.hash
		}

		dag.mu.RLock()
		var missing []string
		resolved := make(map[string]*Block, len(frontier))
		for _, h := range hashes {
			if b, ok := dag.blocks[h]; ok {
				resolved[h] = b
			} else {
				missing = append(missing, h)
			}
		}
		dag.mu.RUnlock()

		if len(missing) > 0 {
			dbBudget--
			found, err := dag.state.LoadBlocksByHashesFromDB(missing)
			if err == nil && len(found) > 0 {
				dag.mu.Lock()
				for _, b := range found {
					if _, ok := dag.blocks[b.Hash]; !ok {
						dag.blocks[b.Hash] = b
					}
				}
				dag.mu.Unlock()
				for _, b := range found {
					resolved[b.Hash] = b
				}
			}
		}

		var next []entry
		for _, e := range frontier {
			if e.depth >= depthLimit || len(visited) >= visitCap {
				continue
			}
			b := resolved[e.hash]
			if b == nil {
				continue // unresolved — computeGHOSTDAGState's own in-lock fallback handles it
			}
			for _, ph := range b.ParentHashes {
				if !visited[ph] {
					visited[ph] = true
					next = append(next, entry{ph, e.depth + 1})
					if len(visited) >= visitCap {
						break
					}
				}
			}
		}
		frontier = next
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
		// See lastPeerContactAt's own struct comment: this must be set
		// unconditionally here, before any gate, so a severely backlogged (but
		// genuinely reachable) peer is never mistaken for a downed one by the
		// stall-timeout escape valves below.
		dag.lastPeerContactAt.Store(time.Now().Unix())
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
		// FIX (P0, 2026-07-25 night): accepted-as-covered blocks are never
		// stored in dag.blocks — the one acceptance outcome reconcileDeferrals'
		// presence check can't see. Clear any deferral watch entry for this
		// hash explicitly, or a block that first arrived as a deferral and was
		// later waved through here stays "unresolved" forever and blocks
		// production. See forgetDeferral's own comment.
		dag.forgetDeferral(block.Hash)
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
	dag.prefetchParentsFromDB(block)
	dag.prefetchMergeSetFromDB(block)
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
			// Recovery is driven by its OWN clock, not by firstSeen — see
			// unknownProposerLastRecovery's field comment. firstSeen answers "have
			// we logged about this proposer yet"; it must not also answer "may we
			// try to learn it again", or one attempt that happened to find no peers
			// becomes the only attempt this node will ever make.
			now := time.Now()
			lastTry, tried := dag.unknownProposerLastRecovery[proposer]
			shouldRecover := !tried || now.Sub(lastTry) >= unknownProposerRecoveryRetry
			if shouldRecover {
				// Lazily created: several construction paths — and every test
				// that builds a BlockDAG as a struct literal — leave this nil,
				// and a write to a nil map panics. Same 500 cap as
				// warnedUnknownProposers, for the same reason: forged proposer
				// addresses must not grow it without bound.
				if dag.unknownProposerLastRecovery == nil || len(dag.unknownProposerLastRecovery) > 500 {
					dag.unknownProposerLastRecovery = make(map[string]time.Time)
				}
				dag.unknownProposerLastRecovery[proposer] = now
				if !firstSeen {
					// Deliberately logged on every RETRY even though the
					// first-sight line is suppressed: a node silently rejecting a
					// peer's entire chain is exactly the state that went unnoticed
					// for hours on 2026-07-26. One line per proposer per retry
					// interval is bounded and is the only outward sign this is
					// happening at all.
					fmt.Printf("[DAG] Proposer %s still unknown after %s — retrying validator-list sync from all peers\n",
						proposer, now.Sub(lastTry).Truncate(time.Second))
				}
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
			if shouldRecover {
				// Pull the full validator list from every active sync peer right now.
				// If this proposer registered with any of them (but not with us),
				// AddAuthorizedValidator will add them and the next sync cycle will
				// accept their blocks — no manual AUTHORIZED_VALIDATORS config needed.
				SafeGoroutine("syncValidatorsFromAllPeers", dag.syncValidatorsFromAllPeers)
			}
			return false
		}
	}

	// FIX (P0, 2026-07-10): record that we genuinely heard from this authorized
	// validator right now — BEFORE any of the gates below (finality, suspension,
	// missing-parent, GHOSTDAG) get a chance to reject the block for an entirely
	// separate reason. See lastSeenFromValidator's own struct comment (block.go)
	// for why this must be unconditional here rather than skipped whenever a
	// later gate happens to reject it: a validator whose blocks are chronically
	// rejected downstream is exactly the case selfProducedFinalityAllowed needs
	// to be able to tell apart from "this validator has gone quiet".
	if dag.authorizedValidators[proposer] {
		dag.recordForeignSeen(proposer)
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
		hash := block.Hash
		dag.mu.Unlock()
		// FIX (P0, 2026-07-10): finalizedHeight only ever advances (never
		// retreats — see the "HARD FINALITY CHECKPOINTS" header comment,
		// finality.go), so a block rejected here can never stop being a
		// violation later; exactly the "permanently rejected" precondition
		// abandonOrphansWaitingFor requires (mirrors the unauthorized-proposer
		// gate above). Without this, any orphan waiting on this exact hash as
		// its missing parent stayed queued forever: fetchMissingAncestors
		// re-fetches it from a peer that genuinely has it every cycle,
		// AddPeerBlock rejects it here every time for the same reason,
		// RecordOrphanAttempt never fires for it (only recorded when a peer
		// does NOT have the hash — sync_blocks.go), so queueOrphan's own
		// TTL-based abandon check never re-triggers either. Net effect:
		// MissingParentHashes() (and therefore wantDeepScan) stayed permanently
		// non-empty for a gap that can never close, keeping deepScan spinning
		// against this peer forever with nothing it can ever find.
		dag.abandonOrphansWaitingFor(hash)
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
		hasStubParent := false
		for _, ph := range block.ParentHashes {
			parent := dag.ghostdagBlockLookup(ph, nil)
			if parent == nil {
				missingParent = ph
				break
			}
			if parent.Proposer == "synthetic-checkpoint" {
				// See the stub-tolerant height check below — a stub's Height is a
				// placeholder assigned at resync seeding, not this parent's real
				// chain position.
				hasStubParent = true
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
			// FIX (P0, 2026-07-25 night — Contabo1 permanently stuck at its fresh
			// resync checkpoint): when one of the resolved parents is a
			// synthetic-checkpoint stub, its Height is a PLACEHOLDER assigned at
			// resync seeding (checkpointHeight-1), not the parent's real chain
			// position — so the strict equation cannot be evaluated honestly.
			// Confirmed live: the merge-sibling at checkpointHeight-1 (referenced
			// as a merge-parent by essentially every block above the boundary) was
			// re-fetched and re-rejected with "invalid height" forever, walling
			// the node's ENTIRE catch-up behind it, minutes after an otherwise
			// clean authoritative resync. History at/below the boundary is sealed
			// by the checkpoint's finality anyway (the same trust already extended
			// to the stub itself), so tolerate the mismatch instead of rejecting —
			// the block still passes every signature/authorization/TX check.
			if hasStubParent {
				fmt.Printf("[DAG] ⚠ Height mismatch tolerated for block #%d (parent max %d includes a synthetic-checkpoint stub with placeholder height) — attaching at the resync trust boundary\n",
					block.Height, maxParentHeight)
			} else {
				fmt.Printf("[DAG] ✗ Rejected peer block #%d: invalid height (parent max %d)\n",
					block.Height, maxParentHeight)
				dag.mu.Unlock()
				return false
			}
		}
	}

	// Integrity check 4: transaction type whitelist — unknown types could
	// inject unrecognised state-change commands into the audit log.
	for _, tx := range block.Transactions {
		switch tx.Type {
		case "", "register_human", "transfer", "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity", "remove_liquidity", "faucet", "ubi_distribution", "ubi_distribution_finalize",
			"validator_distribution", "validator_distribution_pool_zero", "lp_distribution", "lp_distribution_pool_zero", "escrow_move", "escrow_release", "escrow_recover",
			"slash_equivocation", "distribution_round_marker", "pool_correction":
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
	// != ”" path in LoadBlocksFromDB's migration check) — it just silently
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
	// FIX (P0, 2026-07-10 — closes the remaining BlueScore-drift class:
	// computeGHOSTDAGState/ghostdagMergeSet now report when a DEEPER ancestor
	// (beyond the direct parents Integrity check 3 already verified) can't be
	// resolved yet, instead of silently computing from a truncated set — see
	// those functions' own FIX comments. Treat that exactly like Integrity
	// check 3's own missing-DIRECT-parent case: queue this block as an orphan
	// on the specific missing hash and retry once it arrives, rather than
	// attaching now with a BlueScore this node cannot yet compute correctly.
	if missing, ok := dag.computeGHOSTDAGState(block); !ok {
		dag.mu.Unlock()
		dag.queueOrphan(missing, block)
		return false
	}

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
		if err := dag.state.SaveBlockToDB(block, false); err != nil {
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
	dag.notifyNewBlock(block) // wake /api/events subscribers — see notifyNewBlock's own comment

	// Remove parents from tips
	for _, ph := range block.ParentHashes {
		delete(dag.tips, ph)
	}

	// Add this block as new tip
	dag.tips[block.Hash] = true

	if block.Height > dag.height {
		dag.setHeight(block.Height)
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
		// Captured here, under dag.mu (this whole branch runs with it held —
		// see checkAndIndexEquivocation's precondition), so the goroutine below
		// never reads dag.selfProposer unsynchronised.
		selfAddr := dag.selfProposer
		blockAHash := conflict.Hash
		blockBHash := block.Hash
		detectedAt := block.Timestamp
		SafeGoroutine("equivocation-slashing", func() {
			// FIX (P0, 2026-07-24 — source of the slash_equivocation TXs that
			// were STILL being minted against the primary hours after the
			// BOOTSTRAP_SIGNER guard below shipped): nothing stopped a node from
			// slashing ITSELF. A node receives copies of its own blocks back from
			// peers (gossip echo, HTTP push, ordered sync), so if it ever
			// produced two blocks for the same parent set — a redeploy overlap, a
			// duplicate-guard window miss, a re-fork — it observes that conflict
			// itself, records the evidence, and queues a slash TX against its own
			// signing address, which consensus then replicates to everyone.
			//
			// The BOOTSTRAP_SIGNER guard below cannot cover this: it keys off
			// THIS node's own BOOTSTRAP_SIGNER, and the primary is the seed —
			// it has none configured (StartDivergenceAutoHeal's own comment
			// records that: "the primary has neither"), so trustedBootstrapSigner()
			// returns "" and the guard is skipped entirely on exactly the node
			// whose address is being slashed. Confirmed live: block #1781171 on
			// aequitas.digital itself carried a fresh wave of slash_equivocation
			// TXs against 0x92cbedec…, long after both secondaries had been
			// correctly guarded and their queues drained to zero.
			//
			// A node is never a neutral judge of its own equivocation, and
			// propagating the verdict is a self-inflicted, network-wide ban. It
			// also gains nothing: the node already knows what it produced. Drop
			// the observation entirely — logged loudly, neither persisted nor
			// propagated, exactly like the BOOTSTRAP_SIGNER case below and for
			// the same reason. Any OTHER node that genuinely observes the same
			// conflict is unaffected and can still slash through consensus.
			if selfAddr != "" && strings.EqualFold(proposerAddr, selfAddr) {
				fmt.Printf("[SLASHING] ⚠ Observed an equivocation by %s (%s vs %s) but applied NO suspension and propagated nothing — that is THIS NODE'S OWN signing address. A node cannot judge its own equivocation, and broadcasting the verdict is a self-inflicted network-wide ban. See this call site's own comment.\n",
					proposerAddr, blockAHash, blockBHash)
				return
			}
			// FIX (P0, 2026-07-24 — sixth recurrence of the same false-positive
			// class, and the one that finally identified the structural cause):
			// a node must NEVER unilaterally suspend its own configured
			// BOOTSTRAP_SIGNER on evidence it alone observed. That address is,
			// by the operator's own explicit configuration, the validator this
			// node is willing to replace its ENTIRE account state from via a
			// signed snapshot (StartDivergenceAutoHeal refuses to run at all
			// without it). "I trust this signer enough to overwrite all my state
			// from it, but I will also reject every block it produces for 14
			// days based on something only I saw" is incoherent — and in
			// practice it is a self-inflicted network partition, which is
			// strictly worse than the misbehavior it purports to punish.
			//
			// Confirmed live repeatedly, most recently 2026-07-24 17:50 UTC:
			// both secondaries suspended the primary minutes after a restart,
			// while mid-catch-up, over what the rest of the network agreed was a
			// single event — see selfHealUncorroboratedSeedSuspension's own
			// comment for the same pattern on 07-10, 07-12 and 07-17. That
			// self-heal cannot catch this case: its "uncorroborated" signal only
			// exists at offense_count >= 2 (a 1st offense has no balance penalty,
			// so slash_applied is always false for it), yet a 1st offense already
			// applies the full 14-day suspension that stops merging dead.
			//
			// So: for THIS ONE address, this node's own unilateral observation is
			// DROPPED entirely — logged loudly, but neither persisted nor
			// propagated. It is deliberately not written to equivocation_evidence
			// either: RecordEquivocationAndSuspend returns EARLY (no penalty at
			// all) whenever the pair's evidence row already exists, so writing the
			// row here from a purely local observation would permanently suppress
			// the very consensus path this fix wants to preserve — the node would
			// become immune to a corroborated suspension of that address forever,
			// which is a much bigger silent policy change than the one intended.
			// The log line below is the audit trail; equivocation_evidence stays
			// reserved for pairs that actually went through consensus.
			//
			// Consensus is therefore still fully able to suspend this address on
			// this node: if any OTHER node observes the conflict and queues the
			// evidence TX, replayTransactions' slash_equivocation case calls
			// RecordEquivocationAndSuspend here with no pre-existing row, and the
			// suspension applies normally. What is removed is only the
			// "judge, jury and executioner on my own trust anchor" shortcut.
			// Deliberately scoped to BOOTSTRAP_SIGNER alone (same scoping as
			// selfHealUncorroboratedSeedSuspension, same reasoning): a genuinely
			// malicious THIRD validator is still suspended locally and
			// immediately, exactly as before.
			if trusted := trustedBootstrapSigner(); trusted != "" && strings.EqualFold(proposerAddr, trusted) {
				fmt.Printf("[SLASHING] ⚠ Observed an equivocation by %s (%s vs %s) but applied NO suspension and propagated nothing — that address is this node's configured BOOTSTRAP_SIGNER, and a unilateral suspension of it is a self-inflicted partition. If this conflict is real, another node's evidence TX will still suspend it here via consensus replay. See this call site's own comment.\n",
					proposerAddr, blockAHash, blockBHash)
				return
			}
			// FIX (2026-07-07 — closes a node-local/consensus asymmetry): apply
			// the suspension locally right away, same as before, so THIS node
			// protects itself without waiting on its own block production — but
			// RecordEquivocationAndSuspend only ever ran on whichever node(s)
			// independently detected the SAME conflict (both blocks present in
			// its own dag.blocks/equivocationIndex at the right moment). A node
			// that never independently saw both blocks never suspended the
			// validator at all, even though the financial-penalty TX (below) was
			// already consensus-replicated — see IsValidatorSuspended's callers
			// in AddPeerBlock, gated purely on this node's OWN validator_penalties
			// row. Queuing the evidence unconditionally (not just when a balance
			// penalty applies) lets replayTransactions' "slash_equivocation" case
			// call this same idempotent function on EVERY node that replays the
			// TX, so validator_penalties converges everywhere regardless of who
			// detected it first.
			count, slashWallet, rErr := dag.state.RecordEquivocationAndSuspend(proposerAddr, blockAHash, blockBHash, detectedAt)
			if rErr != nil {
				fmt.Printf("[SLASHING] ✗ Failed to record equivocation for %s: %v\n", proposerAddr, rErr)
				return
			}
			fmt.Printf("[SLASHING] ✓ Equivocation recorded for %s (offense #%d)\n", proposerAddr, count)
			if qErr := dag.state.QueueEquivocationEvidenceTx(proposerAddr, blockAHash, blockBHash, detectedAt); qErr != nil {
				fmt.Printf("[SLASHING] ✗ Could not queue equivocation evidence TX for %s: %v\n", proposerAddr, qErr)
			} else if slashWallet != "" {
				fmt.Printf("[SLASHING] ✓ Equivocation evidence TX queued for %s (offense #%d, %.0f AEQ penalty pending replay)\n",
					proposerAddr, count, equivocationSecondOffensePenaltyAEQ)
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
		dag.recordForeignMergeForProposer(block.Proposer)
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

// lastPeerActivityAt returns the more recent of lastSuccessfulPeerSyncAt
// (last successful merge) and lastPeerContactAt (last time ANY foreign
// block was received, merged or not) — see lastPeerContactAt's own struct
// comment for the incident this exists to fix. This is the correct
// reference point for every "has the peer connection gone silent" stall
// check: a peer that's flooding this node with blocks it can't merge fast
// enough is still a LIVE peer, not a downed one.
func (dag *BlockDAG) lastPeerActivityAt() int64 {
	success := dag.lastSuccessfulPeerSyncAt.Load()
	contact := dag.lastPeerContactAt.Load()
	if contact > success {
		return contact
	}
	return success
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

// setHeight ist der EINZIGE Weg, dag.height zu setzen -- er haelt den
// sperrfreien Spiegel mit. Aufrufer muessen dag.mu wie bisher halten.
func (dag *BlockDAG) setHeight(h int64) {
	if h > dag.height {
		// Zeitpunkt des letzten echten FORTSCHRITTS.
		//
		// Nicht "zuletzt einen Block angehaengt": ein zurueckgefallener Knoten
		// haengt laufend Bloecke an, die als Waisen liegenbleiben, und meldet
		// dabei "Added 37 new blocks ... height unveraendert". Genau darauf
		// ist die Selbstheilung am 02.09.2026 hereingefallen -- ihr Beleg
		// aktualisierte sich weiter, waehrend der Primary 1.400 Bloecke
		// zurueckfiel und seine Hoehe stillstand.
		//
		// Die Hoehe steigt dagegen nur, wenn wirklich etwas vorangeht. Sie ist
		// der ehrliche Beleg. setHeight ist der einzige Schreibweg (dafuer
		// gibt es einen Waechter-Test), also ist dies die einzige Stelle, an
		// der der Zeitstempel gesetzt werden muss.
		dag.lastHeightAdvanceAt.Store(time.Now().Unix())
	}
	dag.height = h
	dag.heightSchnell.Store(h)
}

// HeightSchnell liefert die Hoehe OHNE dag.mu.
//
// Fuer Auskuenfte, die immer antworten muessen -- /api/status, /health --
// auch dann, wenn gerade ein Block-Burst die Schreibsperre haelt. Der Wert
// kann um Sekundenbruchteile aelter sein als Height(); fuer eine
// Statusanzeige ist das richtig, fuer eine Konsensentscheidung nicht. Wer
// entscheidet, nimmt weiter Height().
func (dag *BlockDAG) HeightSchnell() int64 {
	return dag.heightSchnell.Load()
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
// safeStubBlueScoreLocked returns the BlueScore to give a NEWLY bridged
// synthetic-checkpoint stub for a genuinely-lost MID-CHAIN ancestor — see
// queueOrphan's and ProduceBlock's own FIX comments for the incident this
// closes. Must be called under dag.mu.
//
// Two failure modes sit on either side of this, both confirmed live at
// different points this same session:
//   - BlueScore=0 (the original value, still correct for the UNRELATED
//     startup-only BridgeHistoricalGap boundary stub — see that function's
//     own comment for why 0 is deliberate there) made a mid-chain stub look
//     like it belonged at the very start of the chain's history. Any real
//     block built with it as an ancestor computed its own BlueScore as
//     roughly stub.BlueScore+1 -- near zero, light-years below the chain's
//     real accumulated score -- silently replacing this node's canonical
//     chain with a permanently wrong-scored fork built on fabricated
//     history (confirmed live: dropped from ~3.58M to ~1300).
//   - A height-derived HIGH estimate was tried for BridgeHistoricalGap
//     before landing on 0 (see that function's own comment) for the
//     opposite reason: an inflated score can make the stub unfairly WIN the
//     "highest BlueScore" SelectedParent comparison against a genuinely
//     resolvable, honestly-scored real parent, the moment they compete
//     side by side.
//
// One less than the current known frontier (canonicalBlockAtHeightLocked's
// own tips scan, same exclusion for stubs) threads both: a real parent at
// or near the live frontier -- the common case, since anything competing
// for SelectedParent on a live block is itself recent -- still wins on
// score, so the stub can never unfairly hijack SelectedParent away from a
// genuinely resolvable option; but when the stub IS the only viable parent,
// its descendant starts from roughly where the real chain already is
// instead of resetting to scratch. Not a perfectly reconstructed historical
// value (that data is genuinely gone, by definition of needing to bridge at
// all) -- just close enough to never be mistaken for, or construct, a
// competing low-score fork.
func (dag *BlockDAG) safeStubBlueScoreLocked() int64 {
	var frontier int64
	for hash := range dag.tips {
		b := dag.blocks[hash]
		if b == nil || b.Proposer == "synthetic-checkpoint" {
			continue
		}
		if b.BlueScore > frontier {
			frontier = b.BlueScore
		}
	}
	if frontier <= 0 {
		return 0
	}
	return frontier - 1
}

// seedCheckpointParentStubsLocked is RefreshBootHeightAfterSnapshotImport's
// own fix for a permanent post-resync production deadlock (P0, 2026-07-11,
// confirmed live on Contabo1 and Contabo2). cp — the freshly-seeded trusted
// checkpoint block — is trusted "like genesis" (verified via the signed
// snapshot's BOOTSTRAP_SIGNER), but it still carries its own real
// ParentHashes from before this resync wiped chain_blocks. Without this,
// computeGHOSTDAGState (the merge-set walk both ProduceBlock and
// AddPeerBlock rely on) still tries to resolve that recorded parent like
// any other ancestor — and never can: nothing below cp exists locally, and
// deepScanFloor/isFinalityViolation correctly refuse to search past
// finalityHeightSlack below the fresh checkpoint (see lowerDeepScanFloor's
// own comment), so cp's parent — sitting just above that floor — is
// permanently unreachable via ordinary sync with nothing below cp to chain
// back through to it.
//
// The runtime self-heal that exists for exactly this shape of gap
// (queueOrphan's and ProduceBlock's synthetic-checkpoint-stub bridge) can
// never fire either: both gate on !isCatchingUpLocked(), which stays true
// forever here because dag.height can only ever advance past that gate BY
// the same bridge firing — confirmed live, stuck 15+ minutes past the
// bridge's own patience window on both nodes, a genuine deadlock rather
// than mere slowness. Seed the stub proactively at checkpoint-seed time
// instead: cp's own unresolvable parent is exactly as inherently trusted
// (or not) as cp itself — both predate everything this resync could
// verify — so there is nothing to gain by making production wait on an
// isCatchingUp escape that can never come.
//
// Must be called under dag.mu (write lock) with cp already installed in
// dag.blocks/dag.tips — safeStubBlueScoreLocked reads both to compute the
// stub's BlueScore relative to cp's own.
func (dag *BlockDAG) seedCheckpointParentStubsLocked(cp *Block) {
	for _, parentHash := range cp.ParentHashes {
		if parentHash == "" {
			continue
		}
		if _, exists := dag.blocks[parentHash]; exists {
			continue
		}
		stubHeight := cp.Height - 1
		if stubHeight < 0 {
			stubHeight = 0
		}
		dag.blocks[parentHash] = &Block{
			Hash:         parentHash,
			Height:       stubHeight,
			BlueScore:    dag.safeStubBlueScoreLocked(),
			Proposer:     "synthetic-checkpoint",
			ParentHashes: []string{},
		}
		dag.syntheticCheckpointCount.Add(1)
		fmt.Printf("[RESYNC] ✓ Seeded synthetic-checkpoint stub for checkpoint's own parent %s... at height %d — its history predates this resync's trust boundary exactly like the checkpoint itself, so it needs no further resolution\n",
			parentHash[:min(16, len(parentHash))], stubHeight)
		if dag.state != nil {
			capturedHash, capturedHeight := parentHash, stubHeight
			SafeGoroutine("RecordSyntheticCheckpointEvent-checkpointparent", func() {
				dag.state.RecordSyntheticCheckpointEvent(capturedHash, capturedHeight, "checkpoint-own-parent")
			})
		}
	}
}

// maxCanonicalWalkHops bounds canonicalBlockAtHeightLocked's linear
// SelectedParent walk.
//
// FIX (P0, security audit 2026-08-14 — unauthenticated remote DoS + OOM,
// reproduced live on Contabo1): the walk below had NO hop bound at all, and
// the DB-lookup budget it passed to ghostdagBlockLookup stopped gating that
// function on 2026-07-10 (see ghostdagBlockLookup's own "budget is now
// advisory only" comment). That change reasoned every caller's BFS already
// self-limits via maxMergeVisits/mergeDepthLimit — true for the merge-set
// callers it was written for, but this walk is a plain linear loop with no
// visitCap of any kind, so it was left with nothing bounding it whatsoever.
//
// Consequence: a single unauthenticated GET /api/block?height=1 (or
// eth_getBlockByNumber("0x1") on /rpc, same entry point via
// GetBlockByHeight) walked from the current tip all the way down — at the
// observed production height that is ~3.7 MILLION iterations, all but the
// most recent startupLoadWindow of them a synchronous Postgres round trip,
// every one of them holding the GLOBAL dag.mu WRITE lock. That serializes
// block production, peer sync and every other API read behind it for the
// whole duration. Measured live: four such requests took Contabo1 off the
// air for over an hour — it kept accepting TCP on :8080 while answering no
// HTTP at all, which is exactly how this was found. ghostdagBlockLookup also
// caches each fetched block into dag.blocks, so the same request grows the
// in-memory DAG without bound — an OOM path on top of the stall.
//
// The fix is the bound this function's own 2026-07-04 comment already
// describes as its contract ("a bounded budget, falling back to the caller's
// existing DB-heuristic fallback instead of blocking indefinitely"), made
// explicit here rather than borrowed from a shared helper whose gating
// semantics legitimately changed for its consensus callers. Two layers:
//
//  1. Distance precheck — if the requested height is further below the tip
//     than this bound, don't start the walk at all (O(1) rejection).
//  2. Hop counter + a HARD DB-lookup gate inside the loop, so a chain with
//     height gaps (one SelectedParent link can span many heights) or a
//     corrupt link whose parent does not have a lower height still cannot
//     exceed the bound — the latter would otherwise loop forever.
//
// Returning nil is not a behavior change for callers: GetBlockByHeight
// already falls back to LoadBlockFromDBByHeight for exactly this case, and
// that fallback is a single indexed `WHERE height = $1` query applying the
// SAME canonical selection rule this walk does (highest BlueScore, ties
// broken by lowest hash, synthetic-checkpoint stubs skipped). For any height
// deep enough to trip this bound the block is long since finalized, so there
// is no competing sibling for the two paths to disagree about; near-tip
// heights — the explorer's /api/blocks/canonical window (max 200) and
// autoheal's finalized-checkpoint comparison — stay well inside the bound
// and keep taking the exact SelectedParent walk as before.
const maxCanonicalWalkHops = 10000

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
	// Layer 1: reject deep lookups before walking a single hop.
	if best.Height-height > maxCanonicalWalkHops {
		return nil
	}
	// Layer 2: bound both the hop count and, separately, the number of
	// blocks this walk may pull from the DB. maxGhostdagDBLookups is still
	// passed to ghostdagBlockLookup so its telemetry/migration behavior is
	// unchanged, but it no longer gates anything there — dbLookups below is
	// what actually stops the walk, checked BEFORE the round trip happens.
	maxDBLookups := dag.maxGhostdagDBLookups()
	dbLookups := 0
	advisoryBudget := maxDBLookups
	cur := best
	for hops := 0; cur != nil && cur.Height > height; hops++ {
		if hops >= maxCanonicalWalkHops {
			return nil
		}
		if cur.SelectedParent == "" {
			return nil
		}
		if _, inMemory := dag.blocks[cur.SelectedParent]; !inMemory {
			if dbLookups >= maxDBLookups {
				return nil
			}
			dbLookups++
		}
		cur = dag.ghostdagBlockLookup(cur.SelectedParent, &advisoryBudget)
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
// replayFailureState tracks one block's replay-failure backoff — see
// BlockDAG.replayFailures' own comment for why this exists.
type replayFailureState struct {
	count       int
	lastTriedAt time.Time
}

// replayBackoffFor returns how long to wait before re-attempting a block
// that has already failed replay `failCount` times. Capped exponential
// (5s, 10s, 20s, ... up to 60s): long enough that a sustained stream of new
// descendant blocks (the exact trigger of the 2026-07-25 hang — see
// replayFailures' comment) can no longer force a fresh full replay attempt
// on every single arrival, short enough that a genuinely transient failure
// (a DB hiccup, not a deterministic one) still self-heals within a minute
// without any operator action.
func replayBackoffFor(failCount int) time.Duration {
	const base = 5 * time.Second
	const maxBackoff = 60 * time.Second
	if failCount <= 0 {
		return 0
	}
	d := base
	for i := 1; i < failCount && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// replayTransactions applies block's transactions to chain_accounts. force
// must be true ONLY from repairUnreplayedBlocks: it bypasses the
// skipHeight/bootHeight "already covered by the loaded snapshot" guard
// below, which would otherwise silently no-op the very repair this exists
// for — a block being repaired is, by construction, always at or below the
// CURRENT bootHeight (it's history from before this restart), so without
// force it would always hit that guard and never actually re-derive the
// missing effects. Every other caller (the live AddPeerBlock/
// replayInCanonicalOrder path) must keep passing false — see
// ensureReplayedColumn's comment for the incident this parameter closes.
//
// FIX (2026-07-25 hang incident): a block whose replay fails deterministically
// (e.g. a sender genuinely out of balance — a legitimate rejection, not a bug
// in itself) used to be retried from scratch on every single subsequent
// descendant block's arrival, forever, because a failed attempt never marked
// anything and the next block's ancestor walk (collectUnreplayedAncestors)
// finds the exact same unreplayed ancestor again. Confirmed live: Contabo1
// replayed the identical block, transaction-for-transaction, for 9+ minutes
// straight — hanging its replay path and, transitively, its HTTP API — while
// the rest of the network kept producing new blocks on top of it at roughly
// one per second (faster still with ENABLE_MULTI_BLOCK_TICK). The named
// return + defer below records the outcome of every exit path in ONE place
// so none of the function's existing return statements had to change.
// distributionRoundToSkip scans txs for a distribution-round anchor and
// returns the DistributionAt to skip if this round was already applied —
// 0 if not. Pure and side-effect-free on purpose (audit 2026-08-16): the
// decision used to be inlined in replayTransactions, reading two DB config
// values that are unavailable in every no-DB unit test in this package, so
// the mechanism it implements had never actually been exercised by a test —
// only approximated by hand-modeling its effect. Extracted so the decision
// itself — not just its DB plumbing — is directly testable.
//
// Two anchors are checked, not one:
//
//   - distribution_round_marker (state.go's RunDailyDistributionAtomic) fires
//     whenever a round produced ANY transaction, regardless of which sub-pool
//     paid. This is the primary signal, checked against lastDistributionRoundAt.
//   - ubi_distribution_finalize fires only `if ubiTotal > 0`, a strict subset.
//     Kept as a second, independent check (not a fallback contingent on the
//     marker missing) so a block from a node on either side of this exact
//     deploy — one that emits only finalize, one that emits both — is still
//     handled correctly, checked against lastUBIAt.
//
// The whole list is scanned rather than stopping at the first match:
// RunDailyDistributionAtomic appends the marker LAST (it can only fire once
// everything else in the round is known) while finalize sits earlier, so an
// early break on whichever appears first in this loop would silently skip
// checking the other for the rest of the block.
//
// The 24h window, not exact equality, is what makes this work across two
// nodes that fired 2 seconds apart on the same calendar round — see the
// call site's own historical comment for the derivation ("20:00:01 vs
// 20:00:03").
func distributionRoundToSkip(txs []Transaction, lastDistributionRoundAt, lastUBIAt int64) int64 {
	skip := int64(0)
	for _, tx := range txs {
		if tx.Type == "distribution_round_marker" && tx.DistributionAt > 0 {
			if lastDistributionRoundAt > 0 && tx.DistributionAt-lastDistributionRoundAt < 24*3600 {
				skip = tx.DistributionAt
			}
		}
		if skip == 0 && tx.Type == "ubi_distribution_finalize" && tx.DistributionAt > 0 {
			if lastUBIAt > 0 && tx.DistributionAt-lastUBIAt < 24*3600 {
				skip = tx.DistributionAt
			}
		}
	}
	return skip
}

// isDistributionRoundTxType reports whether a TX type belongs to a daily
// distribution round and must therefore be skipped when
// distributionRoundToSkip has flagged this round as already applied.
//
// FIX (audit 2026-08-16, proven double-credit): escrow_move and
// escrow_release were absent from this list, so neither was ever skipped
// even when the round they belong to was correctly detected as a duplicate —
// every other TX in the block got skipped, these two didn't. Proven by
// TestDoublePayAudit_EscrowReleaseIsNeverSkipped (a duplicated escrow_release
// inflated the UBI pool 40 -> 80). distribution_round_marker is included too:
// it must not itself be re-applied a second time once its own round has
// already been matched.
func isDistributionRoundTxType(txType string) bool {
	switch txType {
	case "ubi_distribution", "ubi_distribution_finalize",
		"validator_distribution", "validator_distribution_pool_zero",
		"lp_distribution", "lp_distribution_pool_zero",
		"escrow_move", "escrow_release", "distribution_round_marker":
		return true
	default:
		return false
	}
}

func (dag *BlockDAG) replayTransactions(block *Block, force bool) (ok bool) {
	// FIX (P0, 2026-07-25 night — Contabo2 permanently stuck behind one block
	// that failed replay exactly ONCE on a transient DB error): the backoff
	// guard's own early return used to fall through this defer and be recorded
	// as a brand-new failure — count++ AND lastTriedAt=now — on every skipped
	// attempt. With fetchMissingAncestors re-driving an awaited block every few
	// seconds, lastTriedAt was pushed forward faster than any backoff window
	// could ever elapse: the block was never genuinely re-attempted again, the
	// whole chain above it deferred forever, and the node fell behind until an
	// operator resynced it (which then hit the same trap on the next transient
	// error — confirmed live twice in one evening, at #1856714 and #1857181).
	// A skip is not an attempt: only a REAL replay attempt may update the
	// failure record, so the backoff can actually expire and the next re-drive
	// runs a genuine retry.
	skippedByBackoff := false
	defer func() {
		if skippedByBackoff {
			return
		}
		dag.replayedMu.Lock()
		if ok {
			delete(dag.replayFailures, block.Hash)
		} else {
			f := dag.replayFailures[block.Hash]
			f.count++
			f.lastTriedAt = time.Now()
			dag.replayFailures[block.Hash] = f
			// Same unbounded-growth guard replayedBlocks already applies
			// below (line ~6193) — this map can only grow from genuinely
			// failing blocks, which should be rare, but "rare, over a
			// process lifetime of weeks" is still worth bounding.
			if len(dag.replayFailures) > 50000 {
				dag.replayFailures = make(map[string]replayFailureState, 1000)
			}
		}
		dag.replayedMu.Unlock()
	}()

	// Fix 4: Deduplication guard — if this block has already been replayed,
	// skip it. Prevents double-credits when a block is delivered more than once.
	dag.replayedMu.Lock()
	if dag.replayedBlocks[block.Hash] {
		dag.replayedMu.Unlock()
		return true // already successfully replayed
	}
	dag.replayedMu.Unlock()
	// FIX (audit 2026-08-16, proven double-credit/double-spend): the
	// in-memory cache above is periodically wiped wholesale (see
	// IsBlockReplayedInDB's own comment) and was, until now, the ONLY dedup
	// check — a wipe followed by any re-delivery of an already-replayed block
	// re-applied its entire transaction list a second time, transfers and
	// swaps included, not just distribution TXs. The durable column is the
	// backstop for exactly the window the cache cannot cover. Checked only on
	// a cache MISS, so this adds no cost to the common hit path.
	if cs := dag.state; cs != nil && cs.IsBlockReplayedInDB(block.Hash) {
		dag.replayedMu.Lock()
		dag.replayedBlocks[block.Hash] = true // repopulate so the next delivery hits the fast path
		dag.replayedMu.Unlock()
		return true
	}
	dag.replayedMu.Lock()
	// Backoff guard (see replayFailures' own comment): force=true is ONLY
	// repairUnreplayedBlocks, a deliberate, operator-relevant, once-per-restart
	// retry that must always actually attempt — never rate-limited. Every
	// other (force=false) caller is a new block arrival that may be re-walking
	// an ancestor it has already retried recently; skip cheaply instead of
	// redoing the full transaction loop below.
	if !force {
		if fail, exists := dag.replayFailures[block.Hash]; exists {
			if wait := replayBackoffFor(fail.count); time.Since(fail.lastTriedAt) < wait {
				// See skippedByBackoff's comment at the top of this function:
				// a skip must NOT be recorded as a fresh failure, or the
				// backoff clock resets on every skipped attempt and never
				// expires (the exact livelock that stranded Contabo2).
				skippedByBackoff = true
				dag.replayedMu.Unlock()
				return false
			}
		}
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
	if !force && skipHeight > 0 && block.Height <= skipHeight {
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
	//
	// FIX (audit 2026-08-16, proven double-credit): this used to key SOLELY on
	// "ubi_distribution_finalize", which RunDailyDistributionAtomic only ever
	// emits `if ubiTotal > 0`. A round that paid validators and/or LP holders
	// while the UBI pool was empty — the ordinary case on a low-traffic chain,
	// measured live: pool_ubi 0.0000 — carried no anchor at all, so this
	// pre-pass never fired for it and every one of its distribution TXs
	// replayed in full on a second delivery. distribution_round_marker
	// (state.go) closes that: it is unconditional, firing whenever the round
	// produced ANY transaction. Checked against last_distribution_round_at, a
	// value kept deliberately separate from last_ubi_at — see that function's
	// own comment for why conflating them would be wrong. The finalize-based
	// check is kept alongside it rather than replaced, so a block from a node
	// on either side of this exact deploy is still handled correctly.
	var lastRoundAt, lastUBIAt int64
	fmt.Sscan(dag.state.getConfigValueDB("last_distribution_round_at"), &lastRoundAt)
	fmt.Sscan(dag.state.getConfigValueDB("last_ubi_at"), &lastUBIAt)
	skipDistributionRound := distributionRoundToSkip(block.Transactions, lastRoundAt, lastUBIAt)

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
	// Measures how long every concurrent transfer is shut out by this replay.
	// Deferred BEFORE the Unlock so it runs after it — see
	// exclusive_lock_stats.go for why this number decides whether the
	// deliberately-atomic scope below is worth reworking.
	exclusiveAcquired := time.Now()
	defer trackExclusiveHold(exclusiveAcquired, "block replay")
	// Phasenuhr fuer genau diesen Halt -- siehe replay_phasen_stats.go. Vor
	// dem Unlock registriert, damit sie NACH ihm laeuft (defers sind LIFO)
	// und die gemessene Haltezeit die ganze Sperre umfasst, exakt wie
	// trackExclusiveHold darueber.
	defer func() { merkeReplayBlock(time.Since(exclusiveAcquired)) }()
	defer dag.state.mu.Unlock()
	configBackup := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := dag.state.getConfigValueExists(key)
		configBackup[key] = configValueSnapshot{value: value, existed: existed}
	}
	phMarkReplay := time.Now()
	rollbackSnap := dag.state.snapshotForRollbackLocked(touchedAddrs, needsFullSnapshot, configBackup)
	merkeReplayPhase(&rpSnapshotNanos, phMarkReplay)
	// Sammelt die Konten aller Buendel dieses Blocks, damit sie in EINEM
	// Statement geschrieben werden statt in einem je Buendel. Siehe
	// replay_konten_sammler.go -- das ist der Schritt, der die
	// Datenbankkosten des Nachspielens von der Zahl der Ueberweisungen auf
	// die Zahl der beruehrten Konten umstellt.
	kontenSammlung := neuerKontenSammler()

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
		// Eigene Phase, weil dieser Aufruf eine Verbindung AUS DEM POOL holt --
		// und zwar bereits unter der globalen Sperre. db_pool.wait_avg_ms stand
		// am 05.09.2026 unter Last auf 85 ms bei 443 Wartevorgaengen. Faellt
		// davon etwas hierhin, wartet das Nachspielen auf eine Verbindung,
		// waehrend es die ganze Kette blockiert -- das waere ein Engpass ohne
		// jede Rechenarbeit, und ohne diese Uhr laege er unsichtbar in `rest`.
		phMarkBegin := time.Now()
		dbTx, err = dag.state.db.Begin()
		merkeReplayPhase(&rpBeginNanos, phMarkBegin)
		if err != nil {
			fmt.Printf("[REPLAY] ✗ Block #%d: could not begin replay transaction: %v — block rejected\n", block.Height, err)
			return false
		}
		dag.state.setActiveTx(dbTx)
	}
	// commitOrRollback finalizes dbTx according to success, clearing
	// activeTx either way so no write after this point accidentally joins
	// a transaction that's already been resolved. Returns an error if a
	// commit was attempted and failed (caller must then treat this exactly
	// like any other hardFailure, including the in-memory restore).
	commitOrRollback := func(success bool) error {
		if dbTx == nil {
			dag.state.setActiveTx(nil)
			return nil
		}
		dag.state.setActiveTx(nil)
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
	// transfersApplied aggregates the per-transfer success logging into ONE
	// line per block (printed after the loop). FIX (2026-07-25 night): a
	// 50k-transfer loadtest block used to emit 50k individual "[REPLAY] ✓
	// Applied transfer" lines — under Railway's log-ingestion backpressure
	// that stdout write itself became the dominant cost of replaying such a
	// block (minutes per block, both live and in the boot repair pass).
	// Failures and every other TX type keep their individual lines.
	transfersApplied := 0

	// NOTE (2026-07-25): a parallel pre-pass applying "provably disjoint"
	// transfers concurrently used to sit here (50k-TPS roadmap item 1). It
	// was REVERTED the same day after its exact failure signature appeared
	// live on Contabo2 minutes after deploy:
	//
	//   [BLOCK] ✗ Could not save peer block #1849827 header before replay:
	//           pq: unexpected Parse response "(D) DataRow" — skipping
	//   [REPLAY] ✗ Block #1849827: replay transaction commit failed
	//           (rolled back, block rejected): driver: bad connection
	//
	// That is a Postgres wire-protocol desync: two goroutines used the same
	// connection concurrently. The reverted code guarded its workers with a
	// LOCAL sync.Mutex, which serializes those workers against each other but
	// not against any other goroutine touching the same dag.state.activeTx —
	// so it never actually established the invariant it claimed. The same
	// signature had already been caught pre-deploy by
	// TestReplayTransactions_ParallelTransfers_RealDB and was wrongly assumed
	// fixed by that local mutex.
	//
	// Cost of the revert is small and was documented at the time: because
	// every worker had to serialize on the DB round trip anyway, only the
	// CPU-bound balance/demurrage arithmetic ever overlapped. Correctness of
	// consensus-critical replay is not worth that trade. Any future attempt
	// must first give each worker its OWN database connection (or move the DB
	// write out of the parallel phase entirely) — a local mutex around a
	// shared *sql.Tx is provably not sufficient.

	for txIdx := 0; txIdx < len(block.Transactions); txIdx++ {
		tx := block.Transactions[txIdx]
		if hardFailure {
			break // stop applying further TXs once we know this block is being rolled back
		}

		// Parallel fast path (roadmap step 6 — see replay_parallel.go for why
		// REPLAY, not ingestion, is what bounds network throughput, and how
		// this differs from the reverted 41b1eee attempt described above).
		//
		// Only ever engaged for a run of CONSECUTIVE, demurrage-free,
		// pairwise-disjoint transfers — a set the determinism tests already
		// prove is order-independent. Anything else ends the run and falls
		// through to the serial switch below, unchanged.
		if tx.Type == "transfer" && skipDistributionRound == 0 {
			if batch, _ := collectDisjointTransferBatch(block.Transactions, txIdx); len(batch) >= parallelReplayMinBatch {
				// withTx statt des leeren ctx: JEDE Kontoaenderung dieses
				// Replays gehoert in dbTx, sonst ueberlebt sie einen Ruecklauf.
				// Vorher kam sie ueber den stillen Rueckfall auf cs.activeTx
				// dorthin -- richtig, solange es das Feld gibt.
				phMarkPar := time.Now()
				ok, batchErr := dag.state.applyTransferBatchParallel(withTx(context.Background(), dbTx), batch, block.Timestamp, kontenSammlung)
				merkeReplayPhase(&rpParallelNanos, phMarkPar)
				if batchErr != nil {
					// Memory already mutated, persistence failed — must NOT
					// fall back to the serial path (that would apply every
					// transfer twice). Fail the block; the caller's rollback
					// snapshot undoes it.
					fmt.Printf("[REPLAY] ✗ %v (block #%d) — rolling back whole block\n", batchErr, block.Height)
					hardFailure = true
					continue
				}
				if ok {
					transfersApplied += len(batch)
					merkeReplayParallel(len(batch))
					txIdx += len(batch) - 1 // the loop's ++ moves past the last batched tx
					continue
				}
				// Declined before mutating anything — fall through so the
				// serial path handles this transaction exactly as before.
			}
		}

		// Skip distribution TXs from a round this node has already applied.
		//
		// FIX (audit 2026-08-16, proven double-credit): escrow_move and
		// escrow_release were absent from this list, so neither was ever
		// skipped even when the round they belong to was correctly detected
		// as a duplicate — every other TX in the block got skipped, these
		// two didn't. Proven by TestDoublePayAudit_EscrowReleaseIsNeverSkipped
		// (a duplicated escrow_release inflated the UBI pool 40 -> 80).
		// distribution_round_marker is included too: it must not itself be
		// re-applied (it would just re-stamp the same timestamp, harmless,
		// but there is no reason to let it through the skip once matched).
		if skipDistributionRound > 0 && isDistributionRoundTxType(tx.Type) {
			fmt.Printf("[REPLAY] ℹ Skipping %s (distribution round %d already applied, block #%d)\n",
				tx.Type, skipDistributionRound, block.Height)
			continue
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
			// FIX (P0, Audit 2026-08-18): bind the claimed nullifier to the
			// proof that was just verified.
			//
			// verifyZKProof above answers exactly one question — "is
			// (pA,pB,pC,pubSignals) a valid Groth16 proof?" — and says
			// NOTHING about tx.Nullifier. The claim below then took
			// tx.Nullifier at face value. The two were never compared, so a
			// validator could pair ANY published proof (they travel in the
			// clear inside every register_human block and are served by
			// /api/blocks) with a nullifier of its own choosing and a fresh
			// wallet: proof verifies, nullifier is unused, 1,000 AEQ are
			// minted. Repeatable with 1, 2, 3, … — each a different canonical
			// nullifier, so the dedup never fires. Unlimited supply creation
			// by a single authorized validator.
			//
			// nullifierMatchesProof was written for precisely this, and its
			// own doc comment describes precisely this attack — it was simply
			// never called from anywhere. It is called now.
			//
			// AequitasV7.sol:257-270 gets this right (effectiveNullifier =
			// bytes32(pubSignals[1]), plus a require() rejecting a mismatched
			// caller-supplied one), which is why honestly-produced blocks all
			// satisfy this check already: /api/register goes through a
			// registerWithSig dry-run before the TX is ever queued. But the
			// contract does not run during block replay, and replay is what
			// writes balances — so the guarantee has to exist here too.
			//
			// ACTIVATION (same reasoning and mechanism as
			// equivocationSlashingActivationUnix, slashing.go): a new
			// rejection rule applied to already-accepted history would make a
			// resync-from-genesis fail on any legacy block that predates the
			// contract-side binding, permanently stalling the node. Anchoring
			// on the block's own timestamp keeps replay deterministic in both
			// directions — every node, syncing today or bootstrapping in a
			// year, computes the same verdict for the same block — while
			// every block produced from now on is bound.
			// Pre-activation blocks are CHECKED but not rejected: a mismatch
			// there is logged loudly instead. That costs nothing, cannot stall
			// a resync, and means the operator finds out whether any legacy
			// block actually violates the binding — the one question this
			// activation window exists to be careful about — from the node's
			// own logs rather than from a manual DB survey.
			if bound, bindErr := nullifierMatchesProof(nullifier, tx.PubSignals); bindErr != nil || !bound {
				reason := "claimed nullifier does not match pubSignals[1]"
				if bindErr != nil {
					reason = bindErr.Error()
				}
				if block.Timestamp >= nullifierProofBindingActivationUnix {
					fmt.Printf("[REPLAY] ✗ register_human for %s (block #%d): %s — rolling back whole block\n",
						wallet, block.Height, reason)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ⚠ register_human for %s (block #%d, pre-activation): %s — ACCEPTED because this block predates the binding rule, but this is exactly the case that rule exists for. Investigate this registration.\n",
					wallet, block.Height, reason)
			}
			// FIX (audit 2026-06-28 recheck 5, P1-1): tryClaimNullifierLocked
			// now returns an error distinctly from "already used" — a genuine
			// DB failure during the claim must roll back the block, not be
			// silently treated as a normal duplicate-registration skip.
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			claimed, claimErr := dag.state.tryClaimNullifierLocked(context.Background(), nullifier, wallet)
			if claimErr != nil {
				fmt.Printf("[REPLAY] ✗ register_human for %s (block #%d): nullifier claim DB error: %v — rolling back whole block\n", wallet, block.Height, claimErr)
				hardFailure = true
				continue
			}
			if !claimed {
				continue // already registered
			}
			// dag.state.activeTx was set directly above (line ~5193), before
			// this loop runs — context.Background() carries no transaction of
			// its own, so dbExecCtx falls back to that field, exactly
			// matching pre-migration behavior. See dbExecCtx's comment.
			if err := dag.state.registerHumanLocked(context.Background(), wallet, block.Timestamp); err != nil {
				// FIX: release the nullifier claimed two lines above on failure —
				// it used to stay claimed forever ("nullifier recorded, balance
				// NOT credited"), permanently burning that biometric for
				// everyone even though no registration ever actually completed
				// with it (e.g. wallet already human via a different nullifier).
				dag.state.releaseNullifierLocked(context.Background(), nullifier)
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
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			phMarkSer := time.Now()
			errSeriell := dag.state.applyTransferDeltaLockedSammelnd(withTx(context.Background(), dbTx), wallet, to, tx.Amount, tx.FromDemurrageLost, tx.ToDemurrageLost, block.Timestamp, kontenSammlung)
			merkeReplaySeriellZeit(phMarkSer)
			if err := errSeriell; err != nil {
				// Eine deterministische Ablehnung toetet den Block NICHT. Ein
				// abgewiesener Block wird nie wieder angenommen, und dieser
				// Fehler faellt bei jedem Versuch gleich aus -- die Abweisung
				// heilt also nichts, sie haelt den Knoten endgueltig an.
				// Gemessen am 05.09.2026 auf dem Primary: 18 Abweisungen
				// derselben Hoehe, 31 Waisen-Tips, sechs Minuten Stillstand.
				// Die Abweichung selbst meldet der StateRoot-Vergleich am Ende
				// dieses Replays -- der meldet UND laesst die Kette weiterlaufen.
				// Siehe zustand_ablehnung.go.
				if istZustandsAblehnung(err) {
					fmt.Printf("[REPLAY] ⚠ Transfer %s->%s %.6f uebersprungen: %v (block #%d) — Block laeuft weiter\n", wallet, to, tx.Amount, err, block.Height)
					merkeUebersprungeneUeberweisung()
					continue
				}
				fmt.Printf("[REPLAY] ✗ Transfer %s->%s %.6f: %v (block #%d) — rolling back whole block\n", wallet, to, tx.Amount, err, block.Height)
				hardFailure = true
				continue
			}
			// Aggregated into one line per block after the loop — see
			// transfersApplied's declaration for the incident this closes.
			transfersApplied++
			merkeReplaySeriell()

		case "swap_aeq_tusd":
			if wallet == "" || tx.Amount <= 0 || tx.AmountOut <= 0 {
				fmt.Printf("[REPLAY] ⚠ Skipping swap_aeq_tusd in block #%d: missing fields\n", block.Height)
				continue
			}
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applySwapDeltaLocked(context.Background(), wallet, tx.Amount, tx.AmountOut, true, tx.FromDemurrageLost, block.Timestamp); err != nil {
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
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applySwapDeltaLocked(context.Background(), wallet, tx.Amount, tx.AmountOut, false, tx.FromDemurrageLost, block.Timestamp); err != nil {
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
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.addLiquidityDeltaLocked(context.Background(), wallet, tx.Amount, tx.AmountOut, tx.LPShares, tx.FromDemurrageLost, block.Timestamp); err != nil {
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
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.removeLiquidityDeltaLocked(context.Background(), wallet, tx.Amount, tx.FromDemurrageLost, block.Timestamp); err != nil {
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
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyFaucetDeltaLocked(context.Background(), wallet, tx.Amount); err != nil {
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
				// context.Background() is correct — see registerHumanLocked's
				// comment: dag.state.activeTx was already set directly above
				// this loop, and dbExecCtx falls back to it.
				if err := dag.state.applyUBIDeltaLocked(context.Background(), tx.AmountPerHuman, block.Timestamp); err != nil {
					fmt.Printf("[REPLAY] ✗ legacy flat ubi_distribution: %v (block #%d) — rolling back whole block\n", err, block.Height)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ✓ Applied legacy flat UBI distribution %.6f AEQ/human (block #%d)\n", tx.AmountPerHuman, block.Height)
			} else if wallet != "" && wallet != "0x0000000000000000000000000000000000000000" {
				// context.Background() is correct — see registerHumanLocked's
				// comment: dag.state.activeTx was already set directly above
				// this loop, and dbExecCtx falls back to it.
				if err := dag.state.applyUBIRewardDeltaLocked(context.Background(), wallet, tx.Amount, tx.FromDemurrageLost, block.Timestamp); err != nil {
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
			// dag.state.activeTx was already set directly above (before this
			// loop runs) — context.Background() carries no transaction of its
			// own, so dbExecCtx falls back to that field, exactly matching
			// pre-migration behavior. See registerHumanLocked's comment.
			if err := dag.state.applyUBIFinalizeDeltaLocked(context.Background(), tx.DistributionAt); err != nil {
				fmt.Printf("[REPLAY] ✗ ubi_distribution_finalize: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Finalized UBI round, last_ubi_at=%d (block #%d)\n", tx.DistributionAt, block.Height)

		case "pool_correction":
			// Die Korrektur eines Ueberschusses, der vor dem 20.08.2026
			// entstanden ist -- siehe pool_correction.go fuer das Warum.
			//
			// DIESELBE Funktion, die der erzeugende Knoten aufgerufen hat.
			// Zwei Fassungen derselben Rechnung sind in diesem Projekt die
			// haeufigste Quelle von StateRoot-Abweichungen gewesen.
			//
			// Amount = AEQ, AmountOut = tUSD. Beide Betraege stehen in der
			// Transaktion und werden NICHT neu bestimmt: ein Knoten, der
			// selbst ausrechnet, wieviel zuviel da ist, kaeme je nach eigenem
			// Zustand auf eine andere Zahl.
			if err := dag.state.applyPoolCorrectionLocked(context.Background(), tx.Amount, tx.AmountOut); err != nil {
				fmt.Printf("[REPLAY] ✗ pool_correction: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Reserve korrigiert: -%.6f AEQ, -%.6f tUSD (block #%d)\n", tx.Amount, tx.AmountOut, block.Height)

		case "distribution_round_marker":
			// See RunDailyDistributionAtomic's comment (state.go) and this
			// block's own skipDistributionRound pre-pass for what this closes:
			// an unconditional per-round anchor, independent of which sub-pool
			// actually paid, so a round that only credited validators/LP/escrow
			// is still detectable as "already applied" on a second delivery.
			if err := dag.state.applyDistributionRoundMarkerDeltaLocked(context.Background(), tx.DistributionAt); err != nil {
				fmt.Printf("[REPLAY] ✗ distribution_round_marker: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Marked distribution round, last_distribution_round_at=%d (block #%d)\n", tx.DistributionAt, block.Height)

		case "validator_distribution":
			wallet := strings.ToLower(tx.Wallet)
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyValidatorRewardDeltaLocked(context.Background(), wallet, tx.Amount, tx.FromDemurrageLost, block.Timestamp); err != nil {
				fmt.Printf("[REPLAY] ✗ validator_distribution %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied validator reward %.6f AEQ for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "validator_distribution_pool_zero":
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyValidatorPoolZeroDeltaLocked(context.Background()); err != nil {
				fmt.Printf("[REPLAY] ✗ validator_distribution_pool_zero: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Zeroed validators pool (block #%d)\n", block.Height)

		case "lp_distribution":
			wallet := strings.ToLower(tx.Wallet)
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyLPRewardDeltaLocked(context.Background(), wallet, tx.Amount, tx.FromDemurrageLost, block.Timestamp); err != nil {
				fmt.Printf("[REPLAY] ✗ lp_distribution %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied LP reward %.6f AEQ for %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "lp_distribution_pool_zero":
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyLPPoolZeroDeltaLocked(context.Background()); err != nil {
				fmt.Printf("[REPLAY] ✗ lp_distribution_pool_zero: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Zeroed LP pool (block #%d)\n", block.Height)

		case "escrow_move":
			wallet := strings.ToLower(tx.Wallet)
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyEscrowMoveDeltaLocked(context.Background(), wallet, tx.FromDemurrageLost, tx.LPShares, tx.EscrowTUsdConverted); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_move %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow move for %s (block #%d)\n", wallet, block.Height)

		case "escrow_release":
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyEscrowReleaseDeltaLocked(context.Background(), tx.Amount); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_release: %v (block #%d) — rolling back whole block\n", err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow release %.6f AEQ → UBI pool (block #%d)\n", tx.Amount, block.Height)

		case "escrow_recover":
			wallet := strings.ToLower(tx.Wallet)
			// context.Background() is correct — see registerHumanLocked's
			// comment: dag.state.activeTx was already set directly above
			// this loop, and dbExecCtx falls back to it.
			if err := dag.state.applyEscrowRecoverDeltaLocked(context.Background(), wallet, tx.Amount, block.Timestamp); err != nil {
				fmt.Printf("[REPLAY] ✗ escrow_recover %s: %v (block #%d) — rolling back whole block\n", wallet, err, block.Height)
				hardFailure = true
				continue
			}
			fmt.Printf("[REPLAY] ✓ Applied escrow recovery %.6f AEQ → %s (block #%d)\n", tx.Amount, wallet, block.Height)

		case "slash_equivocation":
			// tx.Wallet = signer (signing address of the equivocating validator)
			// tx.BlockAHash/BlockBHash = the conflicting evidence pair
			// tx.DetectedAt = the ORIGINAL conflicting block's own Timestamp
			if tx.Wallet == "" || tx.BlockAHash == "" || tx.BlockBHash == "" {
				fmt.Printf("[REPLAY] ⚠ slash_equivocation in block #%d: missing required fields — skipping\n", block.Height)
				continue
			}
			// FIX (2026-07-07 — closes the node-local suspension gap): every
			// node that replays this TX calls the SAME idempotent function
			// the detecting node already called locally (see AddPeerBlock's
			// equivocation-slashing goroutine) — so validator_penalties
			// (offense count / suspension / ban) converges identically on
			// EVERY node, not just whichever one first observed both
			// conflicting blocks. Idempotent via equivocation_evidence's
			// (block_a_hash, block_b_hash) UNIQUE constraint: a node that
			// already recorded this pair locally (or via an earlier
			// duplicate TX) gets a no-op here, just reads back the current
			// count.
			count, slashWallet, rErr := dag.state.RecordEquivocationAndSuspend(tx.Wallet, tx.BlockAHash, tx.BlockBHash, tx.DetectedAt)
			if rErr != nil {
				fmt.Printf("[REPLAY] ✗ slash_equivocation: could not record evidence for %s (block #%d): %v — rolling back whole block\n", tx.Wallet, block.Height, rErr)
				hardFailure = true
				continue
			}
			if slashWallet == "" {
				fmt.Printf("[REPLAY] ✓ slash_equivocation recorded for %s (offense #%d, no balance penalty, block #%d)\n", tx.Wallet, count, block.Height)
				continue
			}
			blockA, blockB := tx.BlockAHash, tx.BlockBHash
			if blockA > blockB {
				blockA, blockB = blockB, blockA
			}
			// Idempotent balance-deduction CAS: only ONE slash_equivocation TX per
			// evidence pair ever succeeds — competing TXs (from multiple nodes that
			// independently detected/queued the same offense) produce exactly one
			// deduction. Separate from RecordEquivocationAndSuspend's own idempotency
			// (that guards offense-count/suspension state; this guards the balance
			// transfer specifically, since it must run exactly once regardless of how
			// many nodes' TXs reference this same pair).
			res, claimErr := dag.state.dbExec().Exec(
				`INSERT INTO equivocation_evidence
				     (signing_address, block_a_hash, block_b_hash, detected_at, slash_applied)
				 VALUES ($1, $2, $3, $4, TRUE)
				 ON CONFLICT (block_a_hash, block_b_hash) DO UPDATE
				     SET slash_applied = TRUE
				     WHERE equivocation_evidence.slash_applied = FALSE`,
				strings.ToLower(tx.Wallet), blockA, blockB, tx.DetectedAt,
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
			// in case it has shrunk since the offense was recorded.
			//
			// FIX (audit 2026-08-15, cold-cache class): this read used to go
			// straight to cs.accounts with no ensureAccountLoaded, so a wallet
			// that is simply not resident (never paged in on this node, or
			// beyond the startup preload's maxInMemAccounts limit) reported
			// ok=false and left penaltyAmt at the full 50 AEQ — a value
			// assumption derived from "not found", which is exactly the pattern
			// that has bitten this file before. applyTransferDeltaLocked on the
			// very next line DOES warm the same address, which is what makes
			// this inconsistent rather than merely unlucky: it then finds the
			// real (smaller) balance and hard-fails the whole block with
			// "insufficient balance". Cache residency is per-node and not part
			// of consensus, so the identical block would be accepted by a node
			// holding the wallet warm and rejected by one that does not —
			// non-deterministic replay, i.e. a fork. Warming first makes the
			// cap read the same real balance on every node; it is a no-op
			// whenever the account was already resident.
			opWallet := strings.ToLower(slashWallet)
			penaltyAmt := equivocationSecondOffensePenaltyAEQ
			dag.state.ensureAccountLoadedCtx(context.Background(), opWallet)
			if acc, ok := dag.state.accounts.Get(opWallet); ok && acc.Balance.Float() < penaltyAmt {
				penaltyAmt = acc.Balance.Float()
			}
			if penaltyAmt > 0 {
				// context.Background() is correct — see registerHumanLocked's
				// comment: dag.state.activeTx was already set directly above
				// this loop, and dbExecCtx falls back to it.
				if err := dag.state.applyTransferDeltaLocked(withTx(context.Background(), dbTx), opWallet, ubiPoolAddr, penaltyAmt, 0, 0, block.Timestamp); err != nil {
					fmt.Printf("[REPLAY] ✗ slash_equivocation transfer %s→UBI %.4f: %v (block #%d) — rolling back whole block\n",
						opWallet, penaltyAmt, err, block.Height)
					hardFailure = true
					continue
				}
				fmt.Printf("[REPLAY] ✓ Applied slash_equivocation %.4f AEQ from %s → UBI pool (signer %s, offense #%d, block #%d)\n",
					penaltyAmt, opWallet, tx.Wallet, count, block.Height)
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
		// context.Background() is correct: commitOrRollback already cleared
		// dag.state.activeTx above, so there is no transaction left to join
		// regardless (same as before this migration — cs.dbExec() already
		// fell back to cs.db once activeTx was nil).
		for _, n := range claimedNullifiers {
			dag.state.releaseNullifierLocked(context.Background(), n)
		}
		fmt.Printf("[REPLAY] ✗ Block #%d rolled back due to a genuine state-inconsistency failure — block rejected\n", block.Height)
		// Zaehlen, ob DERSELBE Block wieder und wieder scheitert. Dann ist
		// dieser Knoten zugemauert und kommt ohne Resync nicht weiter --
		// siehe replay_mauer.go. Die Heilung laeuft in einer eigenen
		// Goroutine, weil hier die globale Zustandssperre gehalten wird.
		if mauer, folgen := merkeBlockAbweisung(block.Height); mauer {
			dag.loeseHeilungAus(block.Height, folgen)
		}
		return false
	}

	// Die gesammelten Kontenzeilen dieses Blocks in EINEM Statement absetzen.
	//
	// Hier und nicht frueher, weil erst jetzt feststeht, dass der Block nicht
	// ohnehin zurueckgerollt wird -- und nicht spaeter, weil der Schreibvorgang
	// in dbTx gehoeren muss und vor dem Commit stattfinden soll. Der
	// StateRoot-Vergleich darunter bleibt unberuehrt: er liest
	// cs.accountSetXOR, den Phase 2 je Konto bereits fortgeschrieben hat.
	//
	// Ein Fehlschlag hier ist genau derselbe Fall wie ein fehlgeschlagener
	// Buendelschreibvorgang zuvor: der Speicher ist bereits mutiert, also
	// muss der Block als Ganzes zurueck -- dieselbe Behandlung wie jeder
	// andere hardFailure, ueber denselben rollbackSnap.
	if kontenSammlung.anzahl() > 0 {
		phMarkSammler := time.Now()
		sammlerErr := dag.state.sammlerSchreiben(withTx(context.Background(), dbTx), kontenSammlung)
		merkeReplayPhase(&rpSammlerNanos, phMarkSammler)
		if sammlerErr != nil {
			fmt.Printf("[REPLAY] ✗ Block #%d: konnte %d gesammelte Konten nicht schreiben: %v — Block wird zurueckgerollt\n",
				block.Height, kontenSammlung.anzahl(), sammlerErr)
			commitOrRollback(false)
			if rbErr := dag.state.restoreFromRollbackLocked(rollbackSnap); rbErr != nil {
				fmt.Printf("[REPLAY] CRITICAL: rollback persistence failed for block #%d — memory/DB may now disagree: %v\n", block.Height, rbErr)
			}
			for _, n := range claimedNullifiers {
				dag.state.releaseNullifierLocked(context.Background(), n)
			}
			if mauer, folgen := merkeBlockAbweisung(block.Height); mauer {
				dag.loeseHeilungAus(block.Height, folgen)
			}
			return false
		}
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
		// withTx statt des Mantels: der Wert MUSS aus dbTx gelesen werden,
		// damit er widerspiegelt, was dieser Replay gerade geschrieben hat.
		// Vorher kam er ueber den stillen Rueckfall auf cs.activeTx dorthin --
		// dieselbe Transaktion, aber nur, solange es das Feld gibt.
		phMarkRoot := time.Now()
		localRoot := dag.state.stateRootLocked(
			dag.state.getConfigValueCtx(withTx(context.Background(), dbTx), "last_ubi_at"))
		merkeReplayPhase(&rpStateRootNanos, phMarkRoot)
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

	// FIX (self-deadlock incident, 2026-07-11): flip chain_blocks.replayed to
	// true for THIS block, joining the same dbTx as every account mutation
	// above (dag.state.activeTx is still set to dbTx here — dbExec() routes
	// through it). Must happen before commitOrRollback so a failed/rolled-
	// back commit also rolls this flag back — see ensureReplayedColumn's
	// comment for why "header saved" must never silently imply "effects
	// applied" on a later restart.
	if err := dag.state.MarkBlockReplayed(withTx(context.Background(), dbTx), block.Hash); err != nil {
		fmt.Printf("[REPLAY] Warning: could not mark block #%d replayed: %v\n", block.Height, err)
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
	phMarkCommit := time.Now()
	commitErrReplay := commitOrRollback(true)
	merkeReplayPhase(&rpCommitNanos, phMarkCommit)
	if commitErr := commitErrReplay; commitErr != nil {
		if rbErr := dag.state.restoreFromRollbackLocked(rollbackSnap); rbErr != nil {
			fmt.Printf("[REPLAY] CRITICAL: rollback persistence failed for block #%d — memory/DB may now disagree: %v\n", block.Height, rbErr)
		}
		// context.Background() is correct — see the hardFailure branch's
		// matching comment above: commitOrRollback already cleared
		// dag.state.activeTx.
		for _, n := range claimedNullifiers {
			dag.state.releaseNullifierLocked(context.Background(), n)
		}
		fmt.Printf("[REPLAY] ✗ Block #%d: replay transaction commit failed (rolled back, block rejected): %v\n", block.Height, commitErr)
		return false
	}

	// One aggregate line per block instead of one per transfer — see
	// transfersApplied's declaration for the 50k-lines-per-block incident.
	// Printed only after the commit actually succeeded, so the log never
	// claims transfers were applied on a path that then rolled back.
	if transfersApplied > 0 {
		fmt.Printf("[REPLAY] ✓ Applied %d transfer(s) in block #%d\n", transfersApplied, block.Height)
	}

	// Record which block these transactions went into, so
	// eth_getTransactionByHash / eth_getTransactionReceipt can answer with the
	// real block instead of a placeholder — see tx_block_index.go for the
	// wallet report that uncovered this. Deliberately AFTER the commit above:
	// the index must only ever describe transactions that actually applied.
	// A failure here is logged, not fatal — the block itself is valid and
	// committed, and a missing index entry degrades to the pre-existing
	// fallback behaviour rather than rejecting anything.
	// Asynchronous, and this is the call site that matters most: replay holds
	// the EXCLUSIVE state lock -- measured at 4.697s for one full block, with
	// every concurrent transfer blocked for that entire time. Writing up to
	// 10,000 index rows inside that window bought nothing consensus depends
	// on. See tx_block_index_async.go.
	dag.state.IndexBlockTransactionsAsync(block.Height, block.Hash, block.Transactions)

	dag.replayedMu.Lock()
	// FIX 1: Cap the cache to prevent unbounded growth (memory leak).
	// dag.blocks is the authoritative deduplication store; this is a fast-path cache.
	if len(dag.replayedBlocks) > 50000 {
		dag.replayedBlocks = make(map[string]bool, 1000)
	}
	// Der Knoten kommt voran -- die Mauer-Zaehlung zuruecksetzen.
	merkeBlockErfolg()
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

	// FIX (self-deadlock, live on Contabo1/Contabo2 2026-07-11): this is
	// called from inside replayTransactions while it already holds
	// cs.mu.Lock() — the plain CallContract() used to call newStateDB(),
	// which locks cs.mu itself, deadlocking this goroutine against its own
	// lock. See ChainState.getAccountsForAddressesLocked's comment for the
	// full incident.
	caller := common.HexToAddress(tx.Wallet)
	ret, err := dag.evm.CallContractLocked(caller, common.HexToAddress(BIO_VERIFIER_ADDR), verifyData, big.NewInt(0))
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

// logMergeSetBFSCap rate-limits the merge-set BFS visit-cap warning (see
// lastMergeSetBFSCapLogAt's comment) — shared by both call sites in the BFS
// loop below so a single burst doesn't double the log rate for no reason.
func (dag *BlockDAG) logMergeSetBFSCap(blockHash string, visitCap int) {
	nowNano := time.Now().UnixNano()
	last := dag.lastMergeSetBFSCapLogAt.Load()
	if nowNano-last > int64(time.Second) && dag.lastMergeSetBFSCapLogAt.CompareAndSwap(last, nowNano) {
		fmt.Printf("[GHOSTDAG] ⚠ merge-set BFS for block %s hit the %d-node visit cap — treating remaining reachable ancestors as outside the merge set. Extreme concurrent-production burst; investigate gossip/sync latency if this recurs. (rate-limited)\n", blockHash, visitCap)
	}
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
// GHOSTDAG (Sompolinsky-Zohar 2018) with KNIGHTDAG adaptive classification:
//   - SelectedParent = parent with highest blue score
//   - MergeSet       = past(B) ∩ anticone(SelectedParent)  — the "merged" branches
//   - Blues          = merge-set blocks whose anticone contains ≤K_eff blue
//     blocks, where K_eff ≤ dag.k() is inferred per block from the merge
//     set's own concurrency structure (see knightdagInferK)
//   - BlueScore      = blueScore(SP) + 1 + |Blues|
//
// Every node that holds the same block graph computes identical GHOSTDAG state,
// so the canonical ordering (height ASC, blueScore DESC, hash ASC) is
// deterministic across the network and StateRoots are reproducible.
// computeGHOSTDAGState's return reports whether the computation was based
// on the block's COMPLETE required ancestor closure (missingAncestor=="",
// ok==true) or had to give up on a genuinely-unresolvable hash within
// bounds (missingAncestor==that hash, ok==false) — either from Step 2's
// merge-set BFS (see ghostdagMergeSet's own FIX comment) or Step 4's
// pairwise ancestor checks during blue/red classification (see
// ghostdagIsAncestor's own FIX comment). On ok==false,
// block.SelectedParent/Blues/BlueScore are left UNTOUCHED (not partially
// computed) — callers must not treat this block as attached/scored yet.
func (dag *BlockDAG) computeGHOSTDAGState(block *Block) (missingAncestor string, ok bool) {
	if block.IsGenesis || len(block.ParentHashes) == 0 {
		block.SelectedParent = ""
		block.Blues = nil
		block.BlueScore = 0
		return "", true
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
		p := dag.ghostdagBlockLookup(ph, &dbBudget)
		// FIX (P0, 2026-07-10): a DIRECT parent should already be guaranteed
		// present by AddPeerBlock's Integrity check 3 (queues an orphan and
		// never reaches here otherwise) or ProduceBlock's own tip selection
		// — but defensively treat a miss here the same as a merge-set miss
		// below rather than silently excluding this parent from the
		// SelectedParent comparison, which would have been the exact same
		// class of silent-wrong-computation this whole fix closes.
		if p == nil {
			return ph, false
		}
		if p.BlueScore > maxScore {
			maxScore = p.BlueScore
			spHash = ph
		}
	}

	// Step 2: compute merge set — blocks in past(B) that are NOT in past(SP).
	// Bounded BFS: we walk back from non-SP parents, collecting blocks that
	// are not reachable from SP.  The depth limit (2K+1) bounds the search;
	// for an honest ≤3-validator network merge sets are always ≤2 blocks.
	//
	// block.SelectedParent is deliberately not assigned until AFTER this
	// succeeds — see this function's own return-contract comment: on
	// failure, every field must stay untouched, not partially computed.
	mergeSet, missing := dag.ghostdagMergeSet(block, spHash, &dbBudget)
	if missing != "" {
		return missing, false
	}

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
		nowNano := time.Now().UnixNano()
		last := dag.lastMergeSetCapLogAt.Load()
		if nowNano-last > int64(time.Second) && dag.lastMergeSetCapLogAt.CompareAndSwap(last, nowNano) {
			fmt.Printf("[GHOSTDAG] ⚠ merge set size %d for block %s exceeds classification cap %d — classifying only the %d blocks closest to SelectedParent, remainder treated as red. This indicates an extreme concurrent-production burst; investigate gossip/sync latency if this recurs. (rate-limited)\n",
				len(sorted), block.Hash, maxClassifiedMergeSetSize, maxClassifiedMergeSetSize)
		}
		sorted = sorted[:maxClassifiedMergeSetSize]
	}

	// Step 4: blue / red classification — KNIGHTDAG (adaptive K).
	//
	// Instead of classifying with the epoch ceiling dag.k() directly
	// (classic GHOSTDAG), infer the smallest K_eff ∈ [0, dag.k()] whose
	// blue set covers a strict majority of the merge set, and classify
	// with that (see knightdagInferK). If no K_eff below the ceiling
	// reaches a majority, classification falls back to the ceiling —
	// bit-for-bit the pre-KnightDAG behavior — so this is a strict
	// generalization: a well-connected DAG confirms with a tighter K than
	// the epoch worst case, a burst degrades to exactly what shipped
	// before.
	//
	// Determinism (the property TestGHOSTDAG_Determinism_OrderIndependent
	// pins): K_eff is a pure function of the topo-sorted merge set and the
	// pairwise ancestor relation — both derived from block content only,
	// never from arrival order — and the ceiling dag.k() is identical on
	// every node by construction, so every node infers the identical K_eff
	// for the identical block.
	cc := dag.newKnightdagConcCache(&dbBudget)
	// Index this merge set once so every classification pass below addresses
	// pairs by position instead of by hash string — see prepare's comment for
	// the O(K·n²) string-map cost this removes. `sorted` is final here (the
	// maxClassifiedMergeSetSize truncation above already applied).
	cc.prepare(sorted)
	var blues []string
	if block.Height >= knightdagActivationHeight {
		var kEff int
		var missingK string
		kEff, blues, missingK = dag.knightdagInferK(sorted, cc)
		if missingK != "" {
			return missingK, false
		}
		// KEff is a locally-derived annotation exactly like Blues/BlueScore:
		// recomputed by every node for every block (never trusted from the
		// wire — this function overwrites whatever a peer sent), excluded
		// from calculateBlockHash, and deterministic across nodes because
		// knightdagInferK is a pure function of block content (see the
		// determinism note above). Stored so the API/explorer can show the
		// inferred K instead of discarding the single number that proves
		// the adaptive layer is doing something.
		block.KEff = &kEff
	} else {
		// Below the activation height: classify with the ceiling directly,
		// bit-for-bit the pre-KnightDAG rule — see knightdagActivationHeight's
		// own comment for why this matters (a node re-deriving OLD state must
		// reproduce exactly what every other node already committed for it,
		// independent of which code version originally computed it).
		var missingK string
		blues, missingK = knightdagClassify(sorted, dag.k(), cc)
		if missingK != "" {
			return missingK, false
		}
		block.KEff = nil
	}

	// block.SelectedParent is only assigned once classification has fully
	// succeeded, same as Blues/BlueScore below — see this function's own
	// return-contract comment. Step 4 (knightdagInferK/knightdagClassify)
	// can now also report a genuinely-unresolvable ancestor via
	// ghostdagIsAncestor, not just Step 2's merge-set BFS, so this can no
	// longer be set right after Step 2 succeeds.
	block.SelectedParent = spHash
	block.Blues = blues
	block.BlueScore = maxScore + 1 + int64(len(blues)) // SP always contributes +1
	return "", true
}

// knightdagDefaultActivationHeight is the fallback used when
// KNIGHTDAG_ACTIVATION_HEIGHT is unset: the live chain height (~1,503,522)
// observed when KnightDAG shipped (2026-07-21, ~19:57 UTC, fleet: Contabo 1,
// Contabo 2, and the Railway-hosted Primary), plus a buffer sized to give
// every validator in the fleet time to redeploy before the chain reaches
// it, even at an aggressive ~1 block/s production rate. If the fleet's
// actual redeploy timing differs, set KNIGHTDAG_ACTIVATION_HEIGHT explicitly
// — to the SAME value on every node, since an inconsistent height across
// the fleet reproduces exactly the cross-node BlueScore divergence this
// gate exists to prevent.
const knightdagDefaultActivationHeight int64 = 1520000

// knightdagActivationHeight is the first block height at which adaptive K
// classification (knightdagInferK) applies. Below it, classification always
// uses the epoch ceiling directly (knightdagClassify at dag.k()) — bit-for-
// bit the classic pre-KnightDAG rule. This is what makes a node re-deriving
// historical GHOSTDAG state (resync-from-snapshot, deepscan, orphan
// catch-up) reproduce exactly what every other node already committed for
// an old block, regardless of which code version originally computed it —
// the alternative (adaptive from height 0) would let a resyncing node
// compute a DIFFERENT BlueScore than what the network already agreed on.
// A package var (not a const) so tests can override it; production nodes
// only ever set it via KNIGHTDAG_ACTIVATION_HEIGHT (loaded once at
// startup — this is a network-wide protocol parameter, not something that
// should change while a node is running).
var knightdagActivationHeight int64 = loadKnightdagActivationHeight()

func loadKnightdagActivationHeight() int64 {
	raw := strings.TrimSpace(os.Getenv("KNIGHTDAG_ACTIVATION_HEIGHT"))
	if raw == "" {
		return knightdagDefaultActivationHeight
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		fmt.Printf("[KNIGHTDAG] ⚠ invalid KNIGHTDAG_ACTIVATION_HEIGHT=%q (%v) — using default %d\n", raw, err, knightdagDefaultActivationHeight)
		return knightdagDefaultActivationHeight
	}
	return v
}

// knightdagConcCache memoizes the symmetric "concurrent" (mutual-anticone)
// relation between merge-set members for ONE computeGHOSTDAGState call.
// concurrent(x,y) is a pure function of DAG structure — it does not depend
// on the k being trialled — so knightdagInferK's multiple classification
// passes share one cache and the expensive bounded-BFS ancestor walks
// (ghostdagIsAncestor) are paid at most once per distinct pair, keeping
// total BFS cost at the same order as a single classic-GHOSTDAG pass.
type knightdagConcCache struct {
	dag      *BlockDAG
	dbBudget *int
	res      map[[2]string]bool

	// Index-based fast path (see prepare/concurrentAt). Populated only when
	// prepare() has run for the merge set being classified; every other
	// caller (tests, any future direct concurrent() user) keeps using the
	// map above unchanged.
	hashes   []string
	idx      map[string]int
	n        int
	resolved []uint64 // bit b set => pair b has been decided
	conc     []uint64 // bit b set => pair b is concurrent (valid iff resolved)

	// scratch is THE hot-path optimization (see ancestorScratch): one BFS
	// working set reused across every ancestor query this cache serves,
	// instead of a fresh map+queue per query. Used sequentially within a
	// single computeGHOSTDAGState call, so it needs no synchronization.
	scratch ancestorScratch
}

func (dag *BlockDAG) newKnightdagConcCache(dbBudget *int) *knightdagConcCache {
	return &knightdagConcCache{dag: dag, dbBudget: dbBudget, res: make(map[[2]string]bool)}
}

// prepare indexes one merge set so classification can address pairs by
// position instead of by hash string.
//
// PERFORMANCE (2026-07-25): knightdagInferK runs up to K+1 full
// classification passes (K = dag.k(), 18 at base), and each pass asks
// concurrent() about O(|sorted|·|blues|) pairs. Every one of those was a
// map[[2]string]bool lookup — two string hashes plus a comparison, on the
// order of 100ns — so the pure BOOKKEEPING cost around the (correctly
// memoized, paid-once) BFS walks grew as O(K·n²) string-map operations. At
// the 100-validator committee this design targets, n reaches maxMergeVisits
// (~95) and that is roughly 190,000 string-map lookups per block, which is
// the bulk of what block_ghostdag_scale_test.go measures as 30 validators ×
// 20 rounds ≈ 99s.
//
// With a dense index the same question becomes one bit test. The matrix is
// n² bits twice over — ~2.3 KB at n=95, and maxMergeVisits caps n by
// construction — so this trades a negligible allocation for the entire
// string-hashing cost.
//
// Deliberately NOT changed: which pairs get asked, in which order, and the
// early break in knightdagClassify. Precomputing the full matrix would be
// faster still, but it would resolve pairs the early-breaking loop never
// asks about — and a pair that resolves to "missing ancestor" would then
// defer a block that classifies fine today. After a night of orphan walls,
// introducing NEW deferrals to save microseconds is the wrong trade.
func (cc *knightdagConcCache) prepare(sorted []string) {
	n := len(sorted)
	if n == 0 {
		return
	}
	cc.hashes = sorted
	cc.idx = make(map[string]int, n)
	for i, h := range sorted {
		if _, dup := cc.idx[h]; !dup {
			cc.idx[h] = i
		}
	}
	cc.n = n
	words := (n*n + 63) / 64
	cc.resolved = make([]uint64, words)
	cc.conc = make([]uint64, words)
}

// concurrentAt is concurrent() addressed by merge-set position. Identical
// semantics and identical underlying ghostdagIsAncestor call order (the two
// hashes are still ordered lexicographically before the walks, so a pair
// that reports a missing ancestor reports the SAME one it always did) —
// only the memo lookup differs.
func (cc *knightdagConcCache) concurrentAt(i, j int) (bool, string) {
	if i == j {
		return false, ""
	}
	a, b := i, j
	if b < a {
		a, b = b, a
	}
	bit := a*cc.n + b
	word := bit >> 6
	mask := uint64(1) << uint(bit&63)
	if cc.resolved[word]&mask != 0 {
		return cc.conc[word]&mask != 0, ""
	}
	x, y := cc.hashes[a], cc.hashes[b]
	if y < x {
		x, y = y, x
	}
	xAncY, missing := cc.dag.ghostdagIsAncestorScratch(x, y, cc.dbBudget, &cc.scratch)
	if missing != "" {
		return false, missing
	}
	yAncX, missing := cc.dag.ghostdagIsAncestorScratch(y, x, cc.dbBudget, &cc.scratch)
	if missing != "" {
		return false, missing
	}
	v := !xAncY && !yAncX
	cc.resolved[word] |= mask
	if v {
		cc.conc[word] |= mask
	}
	return v, ""
}

// concurrent reports whether x and y are in each other's anticone (neither
// is an ancestor of the other). Symmetric, memoized under an ordered key.
//
// A non-empty missing return means ghostdagIsAncestor hit a genuinely
// unresolvable hash (see its own FIX comment) — the answer is NOT cached in
// that case, since it isn't one: a stale "not concurrent" or "concurrent"
// verdict computed while a sibling was still in flight must never be reused
// once that sibling actually arrives.
func (cc *knightdagConcCache) concurrent(x, y string) (concurrent bool, missing string) {
	if x == y {
		return false, ""
	}
	key := [2]string{x, y}
	if y < x {
		key = [2]string{y, x}
	}
	if v, ok := cc.res[key]; ok {
		return v, ""
	}
	xAncY, missing := cc.dag.ghostdagIsAncestorScratch(key[0], key[1], cc.dbBudget, &cc.scratch)
	if missing != "" {
		return false, missing
	}
	yAncX, missing := cc.dag.ghostdagIsAncestorScratch(key[1], key[0], cc.dbBudget, &cc.scratch)
	if missing != "" {
		return false, missing
	}
	v := !xAncY && !yAncX
	cc.res[key] = v
	return v, ""
}

// knightdagClassify runs the greedy GHOSTDAG blue/red pass over an already
// topo-sorted merge set with a GIVEN k: a member is blue iff at most k
// already-blue members are concurrent with it. The antiCnt>k early break
// (scale audit, 2026-07) is what keeps a trial at small k cheap — once more
// than k blues sit in M's anticone it is red per the k-cluster rule itself,
// so the rest of `blues` cannot change the outcome; without the break this
// loop is O(|sorted|·|blues|) unconditionally, which at 100-validator
// concurrent production did not finish within 120s in
// block_ghostdag_scale_test.go before the break existed.
// knightdagClassify's third return, missing, is non-empty only when a
// concurrent() check inside the loop hit a genuinely unresolvable ancestor
// hash (see ghostdagIsAncestor's FIX comment) — blues is nil in that case;
// the caller must treat this exactly like ghostdagMergeSet's own
// missingAncestor (retry once the hash resolves), not as "no blues".
func knightdagClassify(sorted []string, k int, cc *knightdagConcCache) (blues []string, missing string) {
	// Indexed fast path when prepare() has indexed exactly this merge set —
	// same loop, same order, same early break, only the memo lookup is a bit
	// test instead of a string-map hit (see prepare's comment).
	if cc != nil && cc.n > 0 && cc.n == len(sorted) {
		blueIdx := make([]int, 0, len(sorted))
		for i := range sorted {
			antiCnt := 0
			isBlue := true
			for _, bIdx := range blueIdx {
				conc, miss := cc.concurrentAt(bIdx, i)
				if miss != "" {
					return nil, miss
				}
				if conc {
					antiCnt++
					if antiCnt > k {
						isBlue = false
						break
					}
				}
			}
			if isBlue {
				blueIdx = append(blueIdx, i)
			}
		}
		blues = make([]string, len(blueIdx))
		for i, bIdx := range blueIdx {
			blues[i] = sorted[bIdx]
		}
		return blues, ""
	}

	blues = make([]string, 0, len(sorted))
	for _, mHash := range sorted {
		antiCnt := 0
		isBlue := true
		for _, bHash := range blues {
			conc, miss := cc.concurrent(bHash, mHash)
			if miss != "" {
				return nil, miss
			}
			if conc {
				antiCnt++
				if antiCnt > k {
					isBlue = false
					break
				}
			}
		}
		if isBlue {
			blues = append(blues, mHash)
		}
	}
	return blues, ""
}

// knightdagInferK is the KNIGHTDAG core: find the SMALLEST k ∈ [0, dag.k()]
// whose greedy blue set covers a strict majority (>50%) of the merge set,
// returning that k and its blue set. Inspired by DAGKNIGHT (Sompolinsky-
// Sutton 2022): rather than trusting a pre-agreed worst-case K, each
// block's k is inferred from the concurrency the DAG actually exhibits
// around it — a well-connected region confirms with a tight k, a bursty
// region needs a larger one. Two deliberate deviations from the paper,
// both because Aequitas is a small authorized-validator network, not open
// PoW:
//
//  1. The search is bounded above by the epoch ceiling dag.k() and falls
//     back to it when no smaller k reaches a majority — the ceiling pass
//     is bit-for-bit classic GHOSTDAG as shipped before KnightDAG, so the
//     adaptive layer can only tighten, never loosen, the previous
//     behavior. Every K-derived traversal/sizing bound (mergeDepthLimit,
//     maxMergeVisits, maxParents, pruneBuffer, maxGhostdagDBLookups)
//     deliberately stays on the ceiling: the merge set must be DISCOVERED
//     before a per-block k can be inferred from it, so bounding discovery
//     by the per-block value would be circular.
//  2. The majority is over the block's own (bounded) merge set rather
//     than a global past-cone weight — the merge set is exactly the
//     concurrency window this block is merging, and it is already capped
//     and deterministic on every node.
//
// A LINEAR scan (not binary search) is used on purpose: the greedy variant
// of the k-cluster rule is not formally monotone in k, and a binary search
// that assumed monotonicity could return a non-minimal k. The scan is
// deterministic and provably minimal; its bookkeeping cost is bounded by
// Σ_{k<K} O(|sorted|·k) cache hits (the BFS walks behind them are memoized
// in cc), which block_ghostdag_scale_test.go bounds end-to-end.
// missing (third return) is non-empty only when a classification trial hit
// a genuinely unresolvable ancestor hash — see knightdagClassify's own
// comment. kEff/blues are meaningless in that case; the caller must treat
// it exactly like ghostdagMergeSet's missingAncestor.
func (dag *BlockDAG) knightdagInferK(sorted []string, cc *knightdagConcCache) (kEff int, blues []string, missing string) {
	if len(sorted) == 0 {
		return 0, nil, ""
	}
	ceiling := dag.k()
	for k := 0; k < ceiling; k++ {
		var miss string
		blues, miss = knightdagClassify(sorted, k, cc)
		if miss != "" {
			return 0, nil, miss
		}
		if 2*len(blues) > len(sorted) {
			return k, blues, ""
		}
	}
	blues, missing = knightdagClassify(sorted, ceiling, cc)
	return ceiling, blues, missing
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
	// FIX (P0, 2026-07-10 — third occurrence of this exact fork class, same
	// root cause the 2026-07-04 fix above only made rarer, not impossible):
	// that fix scaled the budget up from a fixed 10 to maxMergeVisits()*10 to
	// stop it from being "the limiting factor" outside "a genuinely
	// pathological burst far beyond maxMergeVisits' own ceiling" — but a
	// genuinely pathological burst is exactly what real operation produces
	// periodically (e.g. a node's own RESYNC_FROM_SNAPSHOT catch-up pushing a
	// flood of concurrent blocks network-wide). Confirmed live: Primary and
	// Contabo 2 computed different SelectedParent/hash from height 650000
	// onward, each side internally "healthy" the whole time — the identical
	// symptom, and root cause, as the 2026-07-04 incident this budget was
	// raised to fix, just requiring a bigger burst to reach the now-larger
	// ceiling. Any FINITE, node-local, timing-derived budget has this same
	// hazard at some burst size; only a bound that is structurally identical
	// on every node closes it for good. ghostdagMergeSet's own visitCap
	// (maxMergeVisits) and mergeDepthLimit already ARE that bound — every
	// caller's BFS loop stops visiting NEW hashes once len(excluded)/
	// len(mergeSet) reaches visitCap regardless of this budget, so the total
	// number of real lookups this function can ever be asked to perform in
	// one computeGHOSTDAGState call is already capped at roughly visitCap by
	// construction. Letting a live consensus computation's lookup silently
	// return nil (== "treat as absent", i.e. wrong) once an ADDITIONAL,
	// purely-incidental round-trip counter runs out adds a hazard without
	// adding real protection. budget is now advisory only: still decremented
	// (kept for telemetry / the migration-loop caller, which never passes
	// live consensus state), never gates the lookup itself — a rare,
	// self-limiting-by-visitCap slowdown is the correct tradeoff against a
	// silent, permanent fork.
	if budget != nil && *budget > 0 {
		*budget--
	}
	b := dag.state.LoadBlockFromDBByHash(hash)
	if b != nil {
		dag.blocks[hash] = b
	}
	return b
}

// missingAncestor, if non-empty, means the BFS below hit a hash it needed
// (within depthLimit, before visitCap) that ghostdagBlockLookup could not
// resolve from EITHER dag.blocks or the DB — see this function's own FIX
// comment (2026-07-10) for why the caller must treat this as "not yet
// computable" rather than silently continuing with a truncated set.
func (dag *BlockDAG) ghostdagMergeSet(block *Block, spHash string, dbBudget *int) (mergeSet map[string]bool, missingAncestor string) {
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
	//
	// FIX (P0, 2026-07-10 — the actual remaining root cause of a live
	// BlueScore drift between Primary and a freshly-caught-up secondary,
	// confirmed by TestGHOSTDAG_Determinism_OrderIndependent only ever
	// proving determinism for a COMPLETE, fully in-memory block set): a
	// hash within bounds that ghostdagBlockLookup genuinely cannot resolve
	// (not budget-limited — that hazard was already closed — but truly
	// absent from both dag.blocks and the DB at this exact instant, e.g. a
	// concurrent sibling still in flight over the network) used to be
	// silently treated as "this branch has no further ancestors", exactly
	// like legitimately hitting depthLimit or visitCap. Once
	// computeGHOSTDAGState commits a BlueScore computed from that
	// artificially-truncated set, nothing ever revisits or corrects it —
	// unlike a missing DIRECT parent (Integrity check 3 in AddPeerBlock),
	// which already queues the block as an orphan and retries once the
	// parent arrives, a missing DEEPER ancestor here had no equivalent
	// safety net. Now: report it to the caller instead of guessing:
	// AddPeerBlock queues the block as an orphan on this exact hash (the
	// same, already-proven mechanism), so the block is only ever attached
	// — and its BlueScore only ever committed — once its full merge-set
	// window is genuinely complete. This is what actually makes the
	// "identical block graph → identical GHOSTDAG state" guarantee hold in
	// production, not just in the all-data-already-local test above.
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
			b := dag.ghostdagBlockLookup(cur.hash, dbBudget)
			if b == nil {
				return nil, cur.hash
			}
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

	// BFS backward from non-SP parents, stopping at excluded blocks.
	mergeSet = make(map[string]bool, len(block.ParentHashes))
	queue = queue[:0]
	for _, ph := range block.ParentHashes {
		if ph != spHash && !excluded[ph] && !mergeSet[ph] {
			mergeSet[ph] = true
			queue = append(queue, entry{ph, 0})
		}
	}
	for len(queue) > 0 {
		if len(mergeSet) >= visitCap {
			dag.logMergeSetBFSCap(block.Hash, visitCap)
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
				dag.logMergeSetBFSCap(block.Hash, visitCap)
				break
			}
			if cur.depth > depthLimit {
				continue
			}
			b := dag.ghostdagBlockLookup(cur.hash, dbBudget)
			if b == nil {
				return nil, cur.hash
			}
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
	return mergeSet, ""
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

// ghostdagIsAncestor returns whether ancestorHash can reach descendantHash
// by following parent links, searching no more than ghostdagMergeDepthLimit
// hops back. Used ONLY for anticone detection between two members of a
// bounded merge set (computeGHOSTDAGState) — see ghostdagMergeDepthLimit's
// comment for why a query between two such blocks never needs to look
// further than that bound. Must be called under dag.mu.
//
// FIX (audit 2026-07-21 — same missing-ancestor hazard ghostdagMergeSet was
// hardened against on 2026-07-10, found here by inspection while evaluating
// a persistent cross-call ancestor cache): unlike ghostdagMergeSet, this
// function used to treat ghostdagBlockLookup returning nil (a hash truly
// absent from both dag.blocks and the DB at this exact instant — e.g. a
// concurrent sibling still in flight over the network, NOT a definitive
// "this block doesn't exist") as a silent dead end, exactly the bug class
// that caused Primary/Contabo1 (2026-07-04) and Primary/Contabo2
// (2026-07-10) to independently compute different SelectedParent/BlueScore
// from the same height onward. Called via knightdagConcCache.concurrent()
// from inside the KNIGHTDAG blue/red classification loop, so an
// arrival-timing-dependent false here could flip a block's Blues/BlueScore
// differently on different nodes. Now returns the unresolved hash instead
// of guessing; the caller chain (concurrent → knightdagClassify →
// knightdagInferK → computeGHOSTDAGState) propagates it up to
// computeGHOSTDAGState's existing missingAncestor return, which
// AddPeerBlock/ProduceBlock already know how to handle (queue as orphan /
// retry, same as a missing merge-set ancestor) — no new retry mechanism
// needed, just reusing the one ghostdagMergeSet already proved out.
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
//
// ancestorQueueEntry is one BFS frontier item for ghostdagIsAncestor.
// Named (not a function-local type) so ancestorScratch can hold the queue
// across calls.
type ancestorQueueEntry struct {
	hash  string
	depth int
}

// ancestorScratch lets ONE computeGHOSTDAGState call reuse a single BFS
// working set across all of its ancestor queries.
//
// PERFORMANCE (2026-07-25, measured — this is where the time actually goes):
// a CPU profile of block_ghostdag_scale_test.go's 100-validator run
// (3001 blocks, ~45s) attributes 51% of total time to ghostdagIsAncestor,
// and inside it 29% to runtime.mapassign_faststr plus 16% to map rehash/grow
// — because every single query allocated a fresh map[string]bool and grew it
// from empty up to visitCap. Classification asks this question O(n²) times
// per block, so those short-lived maps also drove most of the ~27% the
// profile spends in GC.
//
// Reusing one map (cleared, not reallocated — Go's clear() keeps the buckets)
// and one queue removes both the allocation and the repeated growth. The
// scratch lives on knightdagConcCache, which is created per
// computeGHOSTDAGState call and used strictly sequentially within it, so no
// synchronization is needed and no state leaks between blocks.
//
// NOTE: this is what actually mattered. An earlier attempt in the same
// session optimized the memo lookup above this layer (string-keyed pair cache
// → bitset) and measured 45.45s → 45.00s, i.e. nothing: with merge sets
// capped at maxMergeVisits the memo was never the cost. Measure first.
type ancestorScratch struct {
	visited map[string]bool
	queue   []ancestorQueueEntry
}

func (dag *BlockDAG) ghostdagIsAncestor(ancestorHash, descendantHash string, dbBudget *int) (isAncestor bool, missing string) {
	return dag.ghostdagIsAncestorScratch(ancestorHash, descendantHash, dbBudget, nil)
}

// ghostdagIsAncestorScratch is ghostdagIsAncestor with an optional reusable
// working set (see ancestorScratch). Passing nil allocates per call, exactly
// as before this optimization — the traversal, its bounds, its ordering and
// its missing-ancestor reporting are identical either way.
func (dag *BlockDAG) ghostdagIsAncestorScratch(ancestorHash, descendantHash string, dbBudget *int, sc *ancestorScratch) (isAncestor bool, missing string) {
	if ancestorHash == descendantHash {
		return true, ""
	}
	visitCap := dag.maxMergeVisits()
	var visited map[string]bool
	var queue []ancestorQueueEntry
	if sc != nil {
		if sc.visited == nil {
			sc.visited = make(map[string]bool, visitCap)
			sc.queue = make([]ancestorQueueEntry, 0, visitCap)
		} else {
			clear(sc.visited)
		}
		visited = sc.visited
		queue = sc.queue[:0]
		// Store the (possibly regrown) backing array back, so the next query
		// reuses it instead of starting from capacity zero again.
		defer func() { sc.queue = queue }()
	} else {
		visited = make(map[string]bool, visitCap)
		queue = make([]ancestorQueueEntry, 0, visitCap)
	}
	visited[descendantHash] = true
	queue = append(queue, ancestorQueueEntry{descendantHash, 0})
	// head-index dequeue rather than queue = queue[1:]: re-slicing advances
	// the header away from the array start, which would make the reused
	// backing array useless after a few calls.
	for head := 0; head < len(queue); head++ {
		if len(visited) >= visitCap {
			return false, ""
		}
		cur := queue[head]
		if cur.depth >= dag.mergeDepthLimit() {
			continue
		}
		b := dag.ghostdagBlockLookup(cur.hash, dbBudget)
		if b == nil {
			// Genuinely unresolvable within bounds (not a depth/visitCap
			// truncation, both of which are deterministic protocol bounds
			// identical on every node) — report it instead of treating this
			// branch as a dead end. See this function's FIX comment.
			return false, cur.hash
		}
		for _, ph := range b.ParentHashes {
			if ph == ancestorHash {
				return true, ""
			}
			if !visited[ph] {
				visited[ph] = true
				queue = append(queue, ancestorQueueEntry{ph, cur.depth + 1})
				if len(visited) >= visitCap {
					return false, ""
				}
			}
		}
	}
	return false, ""
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
	// producedHeights only needs to cover the window where a redeploy overlap
	// could still be in flight; anything below the finalized checkpoint is
	// long settled. Trimming it here keeps the map bounded on a node that
	// runs for months without a restart.
	dag.producedHeightsMu.Lock()
	for h := range dag.producedHeights {
		if h < cutoff {
			delete(dag.producedHeights, h)
		}
	}
	dag.producedHeightsMu.Unlock()

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

	// replayedMu EINMAL fuer den ganzen Lauf, nicht je Vorfahre.
	//
	// Vorher stand Lock/Unlock INNERHALB der Schleife: bei N unabgespielten
	// Vorfahren waren das 2N Sperroperationen -- und diese Funktion laeuft
	// fuer JEDEN ankommenden Block, unter dag.mu.RLock().
	//
	// Damit waechst die Haltezeit der Lesesperre mit dem Rueckstand, und weil
	// Go's RWMutex ankommende Leser aussperrt, sobald ein Schreiber ansteht,
	// staut sich der ganze Sync-Pfad dahinter. Je weiter der Knoten
	// zurueckfaellt, desto teurer wird jeder einzelne Block -- eine
	// Rueckkopplung, die genau das Bild erzeugt, das am 02.09.2026 sechsmal
	// zu sehen war: Hoehe steht minutenlang, dann haengt alles auf einmal an
	// (zuletzt 513 Bloecke in einem Sprung).
	//
	// Der Inhalt der Schleife ist reine Map-Arithmetik ohne I/O; die Sperre
	// laenger zu halten kostet Mikrosekunden und spart 2N Erwerbe. Die
	// Reihenfolge bleibt unveraendert (dag.mu -> replayedMu), es entsteht
	// also keine neue Verschraenkung.
	dag.replayedMu.Lock()
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
			if !dag.replayedBlocks[ph] {
				result = append(result, parent)
				dfs(parent)
			}
		}
	}
	dfs(target)
	dag.replayedMu.Unlock()
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
		if !dag.replayTransactions(anc, false) {
			return false
		}
	}
	return dag.replayTransactions(block, false)
}

// repairUnreplayedBlocks re-drives replayTransactions for every block the
// startup loader found with chain_blocks.replayed = false — headers that
// were durably saved before their transactions' effects were confirmed
// committed (see ensureReplayedColumn's comment for the live incident: a
// self-deadlock killed by SIGQUIT mid-replay left exactly one such block on
// Contabo1, permanently missing a register_human registration from
// chain_accounts with no error anywhere).
//
// Must be called after dag.evm is set (verifyZKProof needs it for
// register_human TXs) — NewEVMRPCServer calls this right after wiring up
// the EVM engine, since dag.evm is nil for the whole of NewBlockchain.
// Safe to call with an empty list (the overwhelmingly common case): this is
// a no-op then. Processes in ascending height order so any genuine
// dependency between them (e.g. a balance a later TX in the gap spends)
// resolves the same way replayInCanonicalOrder already guarantees for live
// blocks. A block that still fails to replay here stays false in
// chain_blocks and is retried again on the next restart — never silently
// dropped.
func (dag *BlockDAG) repairUnreplayedBlocks() {
	dag.mu.Lock()
	gap := dag.unreplayedAtBoot
	dag.unreplayedAtBoot = nil
	dag.mu.Unlock()
	if len(gap) == 0 {
		return
	}
	sort.Slice(gap, func(i, j int) bool { return gap[i].Height < gap[j].Height })
	fmt.Printf("[REPAIR] Found %d block(s) saved but never confirmed replayed (likely a past crash mid-replay) — repairing in the background\n", len(gap))
	started := time.Now()
	repaired, failed := 0, 0
	// FIX (2026-07-25 night, same incident as the SafeGoroutine call site in
	// evm_rpc.go): replayMu used to be held for the ENTIRE pass. Fine when the
	// backlog was a handful of blocks; with a multi-thousand-block backlog it
	// would starve every live AddPeerBlock replay for the whole duration now
	// that this runs concurrently with normal operation. Acquire per block
	// instead: each individual replay keeps the exact same mutual exclusion it
	// always had, and live blocks interleave between repair blocks. MarkBlock-
	// Replayed commits atomically with each block's own replay transaction, so
	// progress is durable per block — a restart mid-pass resumes where it left
	// off instead of starting over.
	for _, b := range gap {
		dag.replayMu.Lock()
		ok := dag.replayTransactions(b, true)
		dag.replayMu.Unlock()
		if ok {
			repaired++
		} else {
			failed++
			fmt.Printf("[REPAIR] ✗ Block #%d (%s...) still failed to replay — will retry again on next restart\n", b.Height, b.Hash[:min(16, len(b.Hash))])
		}
	}
	fmt.Printf("[REPAIR] Done: %d block(s) repaired, %d failed, in %s\n", repaired, failed, time.Since(started).Round(time.Second))
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
