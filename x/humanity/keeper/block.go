package keeper

import (
"crypto/ecdsa"
"crypto/rand"
"crypto/sha256"
"database/sql"
"encoding/hex"
"encoding/json"
"fmt"
"math/big"
"os"
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
	LPShares        float64 `json:"lp_shares,omitempty"`        // for add_liquidity
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
	// Real GHOSTDAG consensus fields (Sompolinsky-Zohar, 2018).
	// BlueScore = number of blue blocks in the past of this block (including
	// the selected-parent chain). Blocks with more blue-score ancestors are
	// preferred, giving a canonical total order over the DAG.
	BlueScore      int64    `json:"blue_score,omitempty"`
	SelectedParent string   `json:"selected_parent,omitempty"` // parent with highest blue score
	Blues          []string `json:"blues,omitempty"`           // blue blocks in the merge set
	// FromSync marks blocks fetched via HTTP-SYNC from the primary's
	// canonical chain. Never serialized — defaults false for all P2P/gossip
	// blocks. When true, the equivocation-suspension gate in AddPeerBlock is
	// bypassed: canonical blocks have already been validated at source.
	FromSync bool `json:"-"`
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
keeper                 *Keeper
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
pendingTxs             []Transaction
txMu                   sync.Mutex
signingKey             *ecdsa.PrivateKey
authorizedValidators   map[string]bool  // Ethereum addresses allowed to propose blocks
activeSyncPeers        map[string]bool  // peers with a running syncWithNode goroutine
// trustedSyncPeers holds only the operator-configured bootstrap/seed URLs
// (PRIMARY_NODE_URL/PRIMARY_NODE_URLS, or the default public seed) — see
// setTrustedSyncPeers. This is a strict subset of activeSyncPeers and is
// the ONLY set of peers whose blocks may be marked block.FromSync=true
// (audit 2026-07-01, P0-1/P0-2: FromSync used to be granted to every
// dynamically-discovered/registered sync peer, which let anyone who could
// register as a peer skip the authorized-validator, equivocation-suspension
// and hard-finality gates entirely — the same Sybil/ban-evasion attacks
// those gates exist to stop. Restricting the bypass to explicitly
// operator-trusted seeds keeps catch-up sync from deadlocking (the original
// reason FromSync was introduced) without extending that trust to arbitrary
// peers.
trustedSyncPeers       map[string]bool
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
	lastDeepScanAt atomic.Int64
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
	// equivocationIndexNextPruneAt ratchets forward pruneEquivocationIndexIfNeeded's
	// next scan point (slashing.go) — see that function's comment.
	equivocationIndexNextPruneAt int

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

func NewBlockchain(keeper *Keeper, nodeID string, state *ChainState) *BlockDAG {
dag := &BlockDAG{
blocks:                 make(map[string]*Block),
tips:                   make(map[string]bool),
keeper:                 keeper,
state:                  state,
nodeID:                 nodeID,
authorizedValidators:   loadAuthorizedValidators(),
activeSyncPeers:        make(map[string]bool),
trustedSyncPeers:       make(map[string]bool),
warnedUnknownProposers: make(map[string]bool),
peerChallenges:         make(map[string]peerChallenge),
replayedBlocks:         make(map[string]bool),
equivocationIndex:      make(map[string]string),
	stateRootMismatches:    make(map[string]int),
	stateRootMismatchLastAt: make(map[string]int64),
	orphans:                make(map[string][]*Block),
	orphanFirstSeen:        make(map[string]time.Time),
	orphanLastAttempt:      make(map[string]time.Time),
	orphanAttempts:         make(map[string]int),

	softRetryBlocks:        make(map[string]*Block),
	softRetryFirstAt:       make(map[string]time.Time),
}
if key, generated, err := loadOrCreateRelayerKey(); err != nil {
	fmt.Printf("[BLOCK] Warning: RELAYER_PRIVATE_KEY invalid, blocks will be unsigned: %v\n", err)
} else if key != nil {
	dag.signingKey = key
	// Always authorize ourselves — derived from the signing key, not the nodeID.
	selfAddr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
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
loaded, loadErr := state.LoadBlocksFromDB()
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
			for i, b := range blocks {
				d.mu.Lock()
				d.computeGHOSTDAGState(b)
				d.mu.Unlock()
				// FIX (2026-06-30, confirmed live in production — second half of
				// the same incident): per-block locking alone isn't enough. A
				// goroutine that does lock→tiny-amount-of-work→unlock→immediately
				// re-lock in a tight loop tends to win the re-acquisition race
				// against other goroutines that only just started waiting,
				// because Go's sync.Mutex has no FIFO fairness guarantee under
				// fast contention — confirmed live: block production, peer sync,
				// and even /api/status all starved for the migration's entire
				// duration (no new blocks produced, no logged sync activity)
				// despite the lock technically being released between every
				// single iteration. A short sleep every so often forces a real
				// scheduling gap so waiting goroutines actually get a turn.
				if i%20 == 19 {
					time.Sleep(5 * time.Millisecond)
				}
				if s != nil {
					// FIX (audit 2026-06-30 monster audit, P1-03): used to
					// discard SaveGHOSTDAGState's error — a failure here means
					// this block's freshly-computed SelectedParent/BlueScore
					// exist in memory (dag.blocks already has them, set by
					// computeGHOSTDAGState above) but never made it to disk.
					// A restart before this block's turn comes around again
					// would silently fall back to its stale pre-migration
					// values, the exact divergence-after-restart class
					// degradedReason exists to surface (see ProduceBlock's
					// degraded gate). Mark degraded instead of continuing
					// silently — same treatment AddPeerBlock's own
					// SaveGHOSTDAGState failure already gets.
					if err := s.SaveGHOSTDAGState(b); err != nil {
						d.degradedMu.Lock()
						if d.degradedReason == "" {
							d.degradedReason = fmt.Sprintf("GHOSTDAG migration: could not persist block #%d: %v", b.Height, err)
						}
						d.degradedMu.Unlock()
						fmt.Printf("[BLOCK] ✗ GHOSTDAG migration: persist failed for block #%d: %v — node marked degraded\n", b.Height, err)
					}
				}
			}
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
// Captured ONCE, after the restoration above and before any block
// processing begins — see bootHeight's field comment.
dag.bootHeight = dag.height
return dag
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
	// FIX (audit 2026-07-01, P0-4): the raw private key used to be printed
	// directly to stderr. On hosted platforms (Railway et al.) stderr/stdout
	// routinely gets shipped to aggregated log dashboards/third-party log
	// storage — anyone with log access (a hosting-platform employee, a
	// compromised log-aggregation integration, a leaked log export) could
	// lift a real, fund-signing private key straight out of logs, worse
	// still since operators are explicitly told they may reuse this same
	// key as their human NODE_OPERATOR_WALLET (a single-key setup). Writing
	// it to a local file with 0600 permissions and printing only the path
	// keeps the key out of the log stream; the file still needs to be read
	// and the env var set before the next restart, same urgency as before.
	keyFilePath := "RELAYER_PRIVATE_KEY.generated"
	fileErr := os.WriteFile(keyFilePath, []byte("0x"+encoded+"\n"), 0o600)
	fmt.Fprintln(os.Stderr, "════════════════════════════════════════")
	fmt.Fprintln(os.Stderr, "⚠ No RELAYER_PRIVATE_KEY found — generated a new one.")
	fmt.Fprintln(os.Stderr, "⚠ SAVE IT NOW — if this process restarts before you do, your validator")
	fmt.Fprintln(os.Stderr, "⚠ identity changes and any pending authorization/rewards binding is lost.")
	if fileErr == nil {
		fmt.Fprintf(os.Stderr, "Private key written to %q (0600 permissions) — read it from there, NOT from these logs, then set it as RELAYER_PRIVATE_KEY and restart.\n", keyFilePath)
		fmt.Fprintln(os.Stderr, "⚠ Delete that file once you've copied the key into your env var/secrets store.")
	} else {
		fmt.Fprintf(os.Stderr, "⚠ Could not write key file (%v) — falling back to printing it here. This key IS visible in hosted log dashboards; treat it as compromised once saved and rotate it if this deployment uses shared/third-party log storage.\n", fileErr)
		fmt.Fprintf(os.Stderr, "SET THIS AS RELAYER_PRIVATE_KEY, then restart the service:\n0x%s\n", encoded)
	}
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
func (dag *BlockDAG) RefreshBootHeightAfterSnapshotImport(resyncHappened bool) {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	if resyncHappened {
		dag.blocks = make(map[string]*Block)
		dag.tips = make(map[string]bool)
		dag.replayedMu.Lock()
		dag.replayedBlocks = make(map[string]bool)
		dag.replayedMu.Unlock()
		dag.bootHeight = 0
		dag.createGenesisBlock() // repopulates dag.blocks/dag.tips with genesis only, sets dag.height = 0
		fmt.Println("[RESYNC] ✓ Reset in-memory DAG to genesis-only — chain_blocks was just wiped, sequential resync from genesis starts now")
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
		dag.bootHeight = bootH
	}

	// dag.height = max_block_height ONLY — this is the sync frontier
	// doSyncOnce pages forward from. After a snapshot resync, max_block_height
	// is reset to 0 (see ResyncFromSnapshotURL) so the node re-downloads all
	// block headers sequentially from genesis. Raising dag.height here from
	// snapshot_import_height would cause doSyncOnce to start near the snapshot
	// height, where dag.blocks is empty (chain_blocks was cleared), making
	// every incoming block orphan on a missing parent permanently.
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
			// FIX (audit 2026-06-30 monster audit, P1-05): durable audit trail
			// for every stub this node has ever trusted instead of verified —
			// see RecordSyntheticCheckpointEvent's own comment. Best-effort,
			// must not block the bridge on a DB hiccup, so this runs
			// fire-and-forget rather than under dag.mu (already held here).
			if dag.state != nil {
				go dag.state.RecordSyntheticCheckpointEvent(ph, stubH, "startup-bridge")
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

// FIX (audit 2026-06-30 monster audit, P1-03): refuse to mint new blocks
// (which means picking a SelectedParent by BlueScore comparison) while
// the background GHOSTDAG migration is still backfilling old blocks'
// scores — see ghostdagMigrationPending's struct comment for the
// consistency window this closes.
if dag.ghostdagMigrationPending.Load() {
	fmt.Printf("[BLOCK] ✗ GHOSTDAG migration still in progress — block production paused until it completes.\n")
	return nil
}

// FIX (audit 2026-06-30 monster audit, P1-05): refuse to mint new blocks
// while this node is still trusting one or more synthetic-checkpoint
// stubs instead of having verified that part of its ancestry. Producing
// on top of unverified history would let a peer-induced trust bypass
// silently propagate into newly-minted, otherwise-fully-verified blocks.
// SyntheticCheckpointCount() now just reads an atomic counter (no lock),
// safe to call here even though dag.mu is already held write-locked.
if syntheticCount := dag.SyntheticCheckpointCount(); syntheticCount > 0 {
	fmt.Printf("[BLOCK] ✗ Node is bridging %d synthetic checkpoint(s) — block production halted until real history syncs in behind them.\n", syntheticCount)
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

// Collect all current tips as parents.
// Sort deterministically so the hash is identical regardless of map
// iteration order — both nodes must agree on parent_hashes ordering.
parentHashes := make([]string, 0, len(dag.tips))
for hash := range dag.tips {
parentHashes = append(parentHashes, hash)
}
sort.Strings(parentHashes)

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
var pendingTxIDs []int64
if dag.state != nil {
	dbTxs, ids := dag.state.LoadPendingTxs()
	if len(dbTxs) > 0 {
		fmt.Printf("[DAG] Including %d restart-surviving TX(s) from DB in block\n", len(dbTxs))
		txs = append(txs, dbTxs...)
		pendingTxIDs = ids
	}
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
StateRoot:    dag.state.StateRoot(),
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
if err := dag.state.SaveBlockWithPendingTxsAtomic(block, pendingTxIDs); err != nil {
	fmt.Printf("[BLOCK] ⚠ Could not persist block #%d (%s...): %v — skipping broadcast, TXs stay queued\n",
		block.Height, block.Hash[:16], err)
	return nil
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

// Record that this proposer produced a block — used for proportional
// validator-reward distribution in DistributeValidatorsPool.
//
// FIX (audit recheck3, P2): used to fire via `go` — if this process died
// right after producing the block (before the goroutine's single DB
// UPDATE ran), that block silently never counted toward this validator's
// own reward weight, with no error anywhere to reveal it. Synchronous now;
// it's one UPDATE statement, the same cost ProduceBlock already pays for
// setConfigValue("max_block_height", ...) a few lines below.
dag.state.IncrementBlockCount(proposer)

// Remove all parents from tips, add this block as new tip
for _, ph := range parentHashes {
delete(dag.tips, ph)
}
dag.tips[block.Hash] = true
dag.height = block.Height
// FIX (double-apply): persist so a restart can resume from the true
// cumulative height instead of dag.height resetting to 0 — see
// createGenesisBlock's restoration of this value and the comment on
// StateSnapshot.Height for why an in-memory-only height broke snapshot
// bootstrap.
dag.state.setConfigValue("max_block_height", fmt.Sprintf("%d", dag.height))

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
// maxLiveBlockClockSkew bounds how far block.Timestamp may deviate from this
// node's wall clock for a block NOT fetched via trusted HTTP-SYNC (see
// isTrustedSyncPeer / AddPeerBlock's timestamp plausibility gate, audit
// 2026-07-01 P1-1). Generous relative to the ~6s block interval to tolerate
// real NTP drift across globally distributed validators, while still making
// it infeasible for a live proposer to claim an arbitrarily old timestamp to
// evade IsValidatorSuspended's "predates the ban" exception.
const maxLiveBlockClockSkew = 10 * time.Minute

const maxOrphans = 200_000

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
			}
			dag.mu.Unlock()
			fmt.Printf("[DAG] (housekeeping) bridged permanently-unresolvable parent %s... (height ~%d) with a synthetic checkpoint — retrying %d block(s) that were waiting on it in the background, no effect on account balances\n",
				missingParent[:min(16, len(missingParent))], stubH, len(waiting))
			if stubInserted {
				dag.syntheticCheckpointCount.Add(1)
				// FIX (audit 2026-06-30 monster audit, P1-05): see
				// RecordSyntheticCheckpointEvent's comment — durable audit trail,
				// tagged "runtime-orphan-bridge" so it's distinguishable from a
				// startup-time BridgeHistoricalGap stub.
				if dag.state != nil {
					go dag.state.RecordSyntheticCheckpointEvent(missingParent, stubH, "runtime-orphan-bridge")
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
	go dag.triggerOrphanResolve()
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

func (dag *BlockDAG) AddPeerBlock(block *Block) bool {
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
// gate — accepting and replaying a peer block while old blocks' GHOSTDAG
// scores are still being backfilled risks computing this block's
// SelectedParent against a DAG view that won't match what the same chain
// converges to once migration finishes. The peer's own sync/retry loop
// re-delivers this block once migration completes and the gate clears.
if dag.ghostdagMigrationPending.Load() {
fmt.Printf("[DAG] ✗ Rejected peer block #%d: GHOSTDAG migration still in progress — retry after it completes\n", block.Height)
dag.mu.Unlock()
return false
}

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
	// Timestamp plausibility gate (audit 2026-07-01, P1-1): block.Timestamp is
	// chosen by the proposer and is only integrity-protected in the sense that
	// it's covered by the signature — nothing previously stopped a suspended
	// validator from signing a brand-new block with a backdated Timestamp to
	// slip through IsValidatorSuspended's "historical block predates the ban"
	// exception below. Live (non-FromSync) blocks must therefore be within a
	// generous clock-skew window of wall-clock time; only FromSync blocks
	// (operator-trusted historical replay, see isTrustedSyncPeer) are exempt,
	// since those legitimately carry old timestamps.
	if !block.FromSync {
		skew := time.Now().Unix() - block.Timestamp
		if skew < 0 {
			skew = -skew
		}
		if skew > int64(maxLiveBlockClockSkew.Seconds()) {
			fmt.Printf("[DAG] ✗ Rejected peer block #%d from %s: implausible timestamp (skew %ds)\n",
				block.Height, proposer, skew)
			dag.mu.Unlock()
			return false
		}
	}
	// Proposer must be in the authorized validator set. Without this check
	// anyone can generate an Ethereum key, sign a block, and feed it in.
	// Skipped for HTTP-SYNC blocks (block.FromSync): the primary already
	// validated them; abandoning orphans here would permanently deadlock
	// any child block waiting on a historical block from an early validator
	// whose registration was cleared from the local DB.
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
			go dag.syncValidatorsFromAllPeers()
		}
		return false
	}
}

// Equivocation suspension gate: a validator suspended or permanently banned
// for repeated equivocation may not produce further blocks until the penalty
// expires. Checked after the signature + authorization gates above so that
// the suspended proposer's identity is already confirmed cryptographically.
//
// Skipped for blocks fetched via HTTP-SYNC (block.FromSync == true): those
// blocks are part of the primary's canonical chain and were accepted before
// the local suspension record existed. Rejecting them here deadlocks
// catch-up sync whenever a historically-banned validator's blocks appear in
// the canonical history.
if dag.state != nil && !block.FromSync {
	if suspended, reason := dag.state.IsValidatorSuspended(proposer, block.Timestamp); suspended {
		fmt.Printf("[SLASHING] ✗ Rejected block #%d from %s: %s\n", block.Height, proposer, reason)
		dag.mu.Unlock()
		return false
	}
}

// Finality gate: reject blocks so far below the finalized checkpoint that
// they could only matter for a deep reorg — which the hard finality
// guarantee forbids. Legitimate gap-fills within finalityHeightSlack of
// the checkpoint are still accepted.
if dag.isFinalityViolation(block) {
	fH, _ := dag.state.GetFinalizedCheckpoint()
	fmt.Printf("[FINALITY] ✗ Rejected block #%d: below finalized checkpoint %d (slack %d)\n",
		block.Height, fH, finalityHeightSlack)
	dag.mu.Unlock()
	return false
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
for _, ph := range block.ParentHashes {
parent, parentExists := dag.blocks[ph]
if !parentExists {
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

// Structural validation passed. Release dag.mu before replay — replay
// uses dag.state's own lock (cs.mu), not dag.mu, and must never run while
// holding dag.mu (ProduceBlock and other dag.mu users would block for the
// duration of every peer block's replay otherwise).
dag.mu.Unlock()

// P1-03 (audit): GHOSTDAG fields (selected_parent, blue_score, blues) are NOT
// covered by the block hash and can be set to arbitrary values by a peer.
// Strip them before saving so the DB never holds peer-supplied GHOSTDAG state.
// computeGHOSTDAGState below will compute the correct values locally, and
// SaveGHOSTDAGState will persist them immediately after.
block.SelectedParent = ""
block.Blues = nil
block.BlueScore = 0

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
	fmt.Printf("[BLOCK] ✓ Synthetic checkpoint at height %d healed — %d still active\n", prev.Height, dag.syntheticCheckpointCount.Load())
}
dag.blocks[block.Hash] = block
dag.computeGHOSTDAGState(block) // real GHOSTDAG: sets SelectedParent, Blues, BlueScore
// P1-03 (audit): persist locally-computed GHOSTDAG fields immediately.
// SaveBlockToDB stored zeroes (peer values were stripped above); update now
// so the DB reflects the same state as dag.blocks after this block is accepted.
// P1-02 (audit): an unhandled error here used to be silently dropped — DB
// would keep the zeroed GHOSTDAG fields from SaveBlockToDB while dag.blocks
// has the real computed ones, so a restart loads different SelectedParent/
// Blues/BlueScore than this run is currently using, which can shift tip
// selection and merge-set computation. One retry (transient blips), then
// mark degraded so /api/health surfaces the divergence risk.
if dag.state != nil {
	if saveErr := dag.state.SaveGHOSTDAGState(block); saveErr != nil {
		time.Sleep(200 * time.Millisecond)
		if saveErr = dag.state.SaveGHOSTDAGState(block); saveErr != nil {
			dag.degradedMu.Lock()
			dag.degradedReason = fmt.Sprintf("SaveGHOSTDAGState failed for block #%d: %v", block.Height, saveErr)
			dag.degradedMu.Unlock()
			fmt.Printf("[BLOCK] ✗ DEGRADED: could not persist GHOSTDAG state for block #%d: %v\n", block.Height, saveErr)
		}
	}
}

// Remove parents from tips
for _, ph := range block.ParentHashes {
	delete(dag.tips, ph)
}

// Add this block as new tip
dag.tips[block.Hash] = true

if block.Height > dag.height {
	dag.height = block.Height
	dag.state.setConfigValue("max_block_height", fmt.Sprintf("%d", dag.height))
}

// Equivocation detection: index this block and trigger slashing if a
// second block from the same proposer for the same parent set is found.
// Runs under dag.mu (checkAndIndexEquivocation requires it) and spawns a
// goroutine for the DB work so it doesn't delay block acceptance.
// Skip triggering NEW slashing from FromSync blocks (audit 2026-07-01,
// P1-2): those are historical replay from an operator-trusted seed (see
// isTrustedSyncPeer), not live proposals. A validator's equivocation, if
// real, would already have been recorded and suspended by the node(s) that
// saw it happen live; re-deriving a fresh slash purely from two blocks
// encountered together during catch-up sync has no live-timing guarantee
// and would let a malicious or misconfigured seed manufacture false
// equivocation evidence against a validator who never actually double-signed
// in real time. Still index the block so a genuine future live conflict
// is caught the normal way.
if conflict, isEquivocation := dag.checkAndIndexEquivocation(block); isEquivocation && dag.state != nil && !block.FromSync {
	proposerAddr := block.Proposer
	blockAHash := conflict.Hash
	blockBHash := block.Hash
	detectedAt := block.Timestamp
	go func() {
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
	}()
}

// Advance the hard finality checkpoint now that GHOSTDAG has been computed
// for this block (SelectedParent and BlueScore are populated above).
dag.maybeAdvanceFinalizedCheckpoint(block)

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
// parent can be retried. Done via a fresh top-level AddPeerBlock call
// rather than recursing — this naturally cascades: if a retried orphan
// succeeds, its own dependents get resolved the same way when ITS
// insertion reaches this point.
for _, waiting := range dag.popOrphans(block.Hash) {
	dag.AddPeerBlock(waiting)
}

// GHOSTDAG soft-retry: now that a new block's state changes are committed,
// blocks that previously failed replayTransactions due to a state dependency
// (e.g. insufficient balance because a sibling block hadn't applied yet)
// get another chance.  Runs in a goroutine so the current AddPeerBlock call
// returns promptly — retries cascade through AddPeerBlock's own orphan
// resolution if they succeed.
go dag.retryAndFlushSoftRetry()

dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix())
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
// /api/status output, confirmed in production. Tie-break deterministically
// on hash so any two nodes holding the identical tip set always agree on
// which one to report, regardless of map iteration order.
func (dag *BlockDAG) LatestBlock() *Block {
dag.mu.RLock()
defer dag.mu.RUnlock()
var latest *Block
for hash := range dag.tips {
b := dag.blocks[hash]
if latest == nil || b.Height > latest.Height || (b.Height == latest.Height && b.Hash < latest.Hash) {
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

// GetBlockByHash returns the block with the given hash, or nil if unknown.
// Used by /api/block/{hash} so a syncing peer can fetch one specific
// missing-ancestor block directly instead of relying solely on the
// height-windowed /api/blocks pagination (see fetchMissingAncestors).
func (dag *BlockDAG) GetBlockByHash(hash string) *Block {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.blocks[hash]
}

// GetBlockByHeight returns a block at the given height, or nil if none
// exists. Multiple validators can produce a sibling at the same height —
// when that happens this prefers the one with the most parent hashes (the
// merge block), matching the explorer UI's own dedup-by-height preference,
// so a search for a specific height shows the same block the list view
// would have shown for it.
func (dag *BlockDAG) GetBlockByHeight(height int64) *Block {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	var best *Block
	for _, b := range dag.blocks {
		if b.Height != height {
			continue
		}
		// FIX (2026-06-30, confirmed live in production): never return a
		// synthetic-checkpoint stub here — see GetBlocks' identical
		// fix/comment. Both callers of this function (api.go's /api/block
		// and evm_rpc.go's eth_getBlockByNumber) are peer/RPC-facing, so
		// unlike GetBlockByHash there's no internal-only caller to preserve.
		if b.Proposer == "synthetic-checkpoint" {
			continue
		}
		if best == nil || len(b.ParentHashes) > len(best.ParentHashes) {
			best = b
		}
	}
	return best
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
			if err := dag.state.applyEscrowMoveDeltaLocked(wallet, tx.FromDemurrageLost); err != nil {
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

// ghostdagK is the GHOSTDAG k-parameter: the maximum number of blocks that
// can be in each other's anticone while all remaining blue.  K=18 matches
// the reference Kaspa implementation; for our ≤3-validator network every
// honest block is always blue in practice (merge sets are at most 2 blocks).
const ghostdagK = 18

// ghostdagMergeDepthLimit bounds how many parent-hops back ghostdagMergeSet
// and ghostdagIsAncestor will walk. Anything further back than this is, by
// construction, already outside any block's merge set (ghostdagMergeSet
// itself never looks further than this), so an ancestor query between two
// merge-set members — the ONLY thing ghostdagIsAncestor is ever used for —
// never needs to look further than this either: if neither block reaches
// the other within this many hops, they were never going to be ancestor/
// descendant of each other within the region this algorithm reasons about.
const ghostdagMergeDepthLimit = 2*ghostdagK + 1

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

	// Step 1: selected parent = highest-blue-score parent.
	var maxScore int64 = -1
	spHash := block.ParentHashes[0]
	for _, ph := range block.ParentHashes {
		if p, ok := dag.blocks[ph]; ok && p.BlueScore > maxScore {
			maxScore = p.BlueScore
			spHash = ph
		}
	}
	block.SelectedParent = spHash

	// Step 2: compute merge set — blocks in past(B) that are NOT in past(SP).
	// Bounded BFS: we walk back from non-SP parents, collecting blocks that
	// are not reachable from SP.  The depth limit (2K+1) bounds the search;
	// for an honest ≤3-validator network merge sets are always ≤2 blocks.
	mergeSet := dag.ghostdagMergeSet(block, spHash)

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
	const maxClassifiedMergeSetSize = maxMergeSetBFSVisits
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
			if !dag.ghostdagIsAncestor(bHash, mHash) && !dag.ghostdagIsAncestor(mHash, bHash) {
				antiCnt++
				if antiCnt > ghostdagK {
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
const maxMergeSetBFSVisits = 50

func (dag *BlockDAG) ghostdagMergeSet(block *Block, spHash string) map[string]bool {
	const depthLimit = ghostdagMergeDepthLimit

	// Build a shallow exclusion set: blocks definitely reachable from SP.
	type entry struct {
		hash  string
		depth int
	}
	excluded := map[string]bool{spHash: true}
	queue := []entry{{spHash, 0}}
	for len(queue) > 0 {
		if len(excluded) >= maxMergeSetBFSVisits {
			break
		}
		cur := queue[0]
		queue = queue[1:]
		if cur.depth > depthLimit {
			continue
		}
		if b, ok := dag.blocks[cur.hash]; ok {
			for _, ph := range b.ParentHashes {
				if !excluded[ph] {
					excluded[ph] = true
					queue = append(queue, entry{ph, cur.depth + 1})
					if len(excluded) >= maxMergeSetBFSVisits {
						break
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
		if len(mergeSet) >= maxMergeSetBFSVisits {
			fmt.Printf("[GHOSTDAG] ⚠ merge-set BFS for block %s hit the %d-node visit cap — treating remaining reachable ancestors as outside the merge set. Extreme concurrent-production burst; investigate gossip/sync latency if this recurs.\n", block.Hash, maxMergeSetBFSVisits)
			break
		}
		cur := queue[0]
		queue = queue[1:]
		if cur.depth > depthLimit {
			continue
		}
		if b, ok := dag.blocks[cur.hash]; ok {
			for _, ph := range b.ParentHashes {
				if !excluded[ph] && !mergeSet[ph] {
					mergeSet[ph] = true
					queue = append(queue, entry{ph, cur.depth + 1})
					if len(mergeSet) >= maxMergeSetBFSVisits {
						break
					}
				}
			}
		}
	}
	return mergeSet
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
func (dag *BlockDAG) ghostdagIsAncestor(ancestorHash, descendantHash string) bool {
	if ancestorHash == descendantHash {
		return true
	}
	type entry struct {
		hash  string
		depth int
	}
	visited := map[string]bool{descendantHash: true}
	queue := []entry{{descendantHash, 0}}
	for len(queue) > 0 {
		if len(visited) >= maxMergeSetBFSVisits {
			return false
		}
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= ghostdagMergeDepthLimit {
			continue
		}
		if b, ok := dag.blocks[cur.hash]; ok {
			for _, ph := range b.ParentHashes {
				if ph == ancestorHash {
					return true
				}
				if !visited[ph] {
					visited[ph] = true
					queue = append(queue, entry{ph, cur.depth + 1})
					if len(visited) >= maxMergeSetBFSVisits {
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

// retryAndFlushSoftRetry re-attempts every block in the soft-retry queue.
// Called as a goroutine after each successful AddPeerBlock: new state may
// have been applied that unblocks previously failing transactions.
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
