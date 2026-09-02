package keeper

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ── HARD FINALITY CHECKPOINTS ───────────────────────────────────────────────
//
// GHOSTDAG natively provides probabilistic finality (like Kaspa): after K+1
// blue blocks build on top of a block B, the probability that B will ever be
// reorged out is negligible. For K=18 that is ~19 blocks ≈ 2 minutes.
//
// This file adds HARD checkpoint finality on top: once a block's blue_score
// is more than finalityBlueScoreDepth below the current tip, it is declared
// a "finalized checkpoint" and AddPeerBlock refuses any block that would only
// be relevant for a reorg below that depth.
//
// finalityBlueScoreDepth = 50 ≫ 2*K=36 (K here is the epoch CEILING dag.k();
// KnightDAG's per-block K_eff is bounded above by it, and a smaller K_eff
// only slows blue-score accrual, so 50 blue-score units always span at least
// as many real blocks as under fixed-K GHOSTDAG): well inside the
// probabilistic window, giving extra safety margin against network jitter.
// finalityHeightSlack     = 50: the height-space equivalent used for the fast
// gate in AddPeerBlock (height and blue_score track each other closely in
// practice; using height avoids the need to compute GHOSTDAG for new blocks
// before the gate fires).
//
// The finalized checkpoint is stored in chain_config and advanced
// monotonically (never retreated). It only affects which new, previously
// unseen blocks the node will accept — already-known blocks in dag.blocks are
// never evicted, and gap-fill blocks for spans WITHIN the finality window are
// still accepted normally.
var (
	finalityBlueScoreDepth int64 = 50
	finalityHeightSlack    int64 = 50
)

// TuneFinalitySlackForBlockTime rescales finalityBlueScoreDepth and
// finalityHeightSlack for the actual configured BLOCK_TIME, mirroring
// TuneProposerBreakerForBlockTime's scaling exactly (same baseline, same
// extra-safety factor) — see that function's own comment for the general
// rationale. This constant pair was the one gap left when that scaling was
// added: everything else in the proposer-breaker/orphan-grace path got
// widened for a faster cadence, but finalityHeightSlack (originally tuned
// for "~50 blocks, a few minutes at BLOCK_TIME" — see this file's header
// comment, written when BLOCK_TIME was 6s, i.e. ~5 real minutes of
// coverage) stayed a bare 50, unscaled.
//
// ROOT CAUSE (2026-07-09/10, live 3-node non-convergence incident): at
// BLOCK_TIME=1s that bare 50 is only 50 SECONDS of real-world tolerance —
// far below the measured real cross-provider attach latency (100+ seconds,
// confirmed live via recordForeignAttachLatency/recordRawArrivalLatency).
// Unlike the proposer breaker's circuit (which has a cooldown and reopen
// probes — a trip is recoverable), isFinalityViolation's rejection is a
// HARD, PERMANENT wall with no retry path: once a legitimate peer block
// arrives even one tick after the local finalized checkpoint has advanced
// finalityHeightSlack past it, it is rejected forever. At 1s cadence with
// real propagation taking longer than the 50s window, EVERY foreign
// validator's block started missing this window, so each node's canonical
// chain became almost entirely self-produced — explaining both the
// permanent divergence (different nodes agree on almost nothing at a
// shared height) and the runaway orphan/retry flood (children of a
// permanently-rejected block re-orphan every time they're rediscovered,
// since the rejection is silent to the retry/deepScan machinery, which
// keeps finding the same "genuinely lost" gap over and over).
//
// Same "never lowers below the original tuning" guarantee as the proposer
// breaker: a slower-than-baseline BLOCK_TIME keeps the proven values.
// Call once at startup, right alongside TuneProposerBreakerForBlockTime,
// before any sync/production goroutines start.
func TuneFinalitySlackForBlockTime(blockTime time.Duration) {
	secs := blockTime.Seconds()
	if secs <= 0 {
		return
	}
	speedupFactor := proposerBreakerTuningBaselineSeconds / secs
	extra := 1.0
	if speedupFactor > 1 {
		extra = proposerBreakerExtraSafetyFactor
	}
	scaled := int64(speedupFactor * 50 * extra)
	if scaled > finalityBlueScoreDepth {
		finalityBlueScoreDepth = scaled
	}
	if scaled > finalityHeightSlack {
		finalityHeightSlack = scaled
	}
}

// isolatedFinalityPauseWindow bounds how long a node that KNOWS about other
// authorized validators may keep hardening its own finality checkpoint from
// self-produced blocks alone, without having successfully merged any of
// those other validators' blocks in.
//
// ROOT CAUSE (2026-07-04, Contabo 2 permanent-isolation incident): a node
// that falls out of real-time sync with its peers — for ANY reason: a
// registration hiccup, a network blip, a slow restart — keeps producing and
// self-finalizing its own one-parent chain the entire time it's isolated,
// because maybeAdvanceFinalizedCheckpoint previously advanced unconditionally
// on every self-produced block (see that ProduceBlock call site's own P0
// 2026-07-03 fix — added for the opposite failure, a solo validator whose
// checkpoint never advanced at all). Once the isolation gap exceeds
// finalityBlueScoreDepth/finalityHeightSlack worth of blocks (~50, a few
// minutes at BLOCK_TIME), isFinalityViolation permanently walls off the
// real peer chain at those heights — SelfFetched's circuit-breaker bypass,
// deepScan, fetchMissingAncestors, none of it can ever cross a HARD finality
// gate by design. Confirmed live: Contabo 2 re-diverged and re-locked itself
// within under an hour of a fresh checkpoint-seeded resync, because nothing
// locally distinguished "healthy, merging fine" from "isolated, hardening a
// dead-end fork" — both looked identical from inside ProduceBlock.
//
// The fix: only let self-produced blocks harden the checkpoint when either
// (a) this node doesn't know of any OTHER authorized validator yet (a
// genuinely solo network — the original 2026-07-03 fix's exact case, left
// fully intact), or (b) a foreign block was merged within this window. A
// node that's actually diverged just pauses its own hardening — its
// checkpoint stops advancing (never retreats) until real merging resumes,
// at which point the very next accepted peer block (AddPeerBlock's own
// unconditional call site) unfreezes it immediately. This can never
// reproduce the pre-2026-07-03 "frozen forever" bug: that only happened for
// a node with NO peers at all, which case (a) above still exempts.
//
// Generous relative to finalityBlueScoreDepth's own ~5-minute real-time
// equivalent (50 blocks * BLOCK_TIME) so ordinary propagation jitter across
// independently-hosted, cross-provider validators never falsely pauses
// hardening — only a gap already large enough to risk a real finality lock
// trips this.
//
// FIX (P3-b, audit 2026-07-06): cross-reference for a future reader —
// autoheal.go's chainDivergenceStallOverride (45 min) addresses the SAME
// underlying phenomenon (this node may have drifted onto its own isolated
// fork) but is a separate, heavier mechanism with a separate, longer
// timeout: this constant just pauses checkpoint HARDENING (self-correcting,
// no side effects, resumes the instant a real peer block merges), while
// chainDivergenceStallOverride forces a full divergence comparison that can
// trigger an actual corrective resync. The 10-vs-45-minute gap is
// deliberate — it gives this lighter, self-healing pause a real chance to
// resolve on its own before the heavier auto-heal mechanism escalates.
const isolatedFinalityPauseWindow = 10 * time.Minute

// recordForeignMerge marks now as the last time this node successfully
// attached another authorized validator's block — called from AddPeerBlock's
// commit path. Must be called without dag.mu held (atomic only).
func (dag *BlockDAG) recordForeignMerge() {
	dag.lastForeignMergeAt.Store(time.Now().Unix())
}

// recordForeignSeen marks now as the last time a genuinely signature-
// verified, authorized block claiming to be from proposer reached
// AddPeerBlock — regardless of what happens to it afterward. Called right
// after AddPeerBlock's signature+authorization gate passes, before any later
// gate (finality, suspension, missing-parent, GHOSTDAG) can reject the
// block. See lastSeenFromValidator's own struct comment (block.go) for why
// this exists separately from recordForeignMerge/recordForeignMergeForProposer.
func (dag *BlockDAG) recordForeignSeen(proposer string) {
	addr := strings.ToLower(proposer)
	dag.foreignValidatorActivityMu.Lock()
	dag.lastSeenFromValidator[addr] = time.Now().Unix()
	dag.foreignValidatorActivityMu.Unlock()
}

// recordForeignMergeForProposer is recordForeignMerge's per-validator
// equivalent — same call site (AddPeerBlock's commit path), additionally
// keyed by proposer. See lastMergedFromValidator's own struct comment
// (block.go).
func (dag *BlockDAG) recordForeignMergeForProposer(proposer string) {
	addr := strings.ToLower(proposer)
	dag.foreignValidatorActivityMu.Lock()
	dag.lastMergedFromValidator[addr] = time.Now().Unix()
	dag.foreignValidatorActivityMu.Unlock()
}

// foreignAttachLatencyLogInterval bounds how often the accumulated
// real-world attach-latency summary is logged — see
// recordForeignAttachLatency's own comment.
const foreignAttachLatencyLogInterval = 30 * time.Second

// latencyWindow is a closed accumulation window's summary — see
// lastForeignLatencyWindow's own comment (block.go) for why this is kept
// separately from the live, currently-accumulating counters.
type latencyWindow struct {
	Count int   `json:"count"`
	AvgMs int64 `json:"avg_ms"`
	MaxMs int64 `json:"max_ms"`
}

// LatencyTelemetry is the scaling-relevant subset of real, measured
// end-to-end block-propagation latency this node has observed — the actual
// numbers every BLOCK_TIME/circuit-breaker/finality-slack tuning decision
// should fit comfortably inside, surfaced for operators and any future
// adaptive-tuning logic instead of living only in log lines.
type LatencyTelemetry struct {
	ForeignAttach latencyWindow `json:"foreign_attach"` // sender ProducedAtMs → this node's AddPeerBlock commit (post-gates)
	RawArrival    latencyWindow `json:"raw_arrival"`    // sender ProducedAtMs → AddPeerBlock entry (pre-gates)
}

// GetLatencyTelemetry returns the most recently closed latency window for
// both measurement points. Read-only, safe for concurrent use with the
// recorder goroutines (separate mutexes per window, matching how they're
// written).
func (dag *BlockDAG) GetLatencyTelemetry() LatencyTelemetry {
	dag.foreignLatencyMu.Lock()
	fw := dag.lastForeignLatencyWindow
	dag.foreignLatencyMu.Unlock()
	dag.rawArrivalLatencyMu.Lock()
	rw := dag.lastRawArrivalLatencyWindow
	dag.rawArrivalLatencyMu.Unlock()
	return LatencyTelemetry{ForeignAttach: fw, RawArrival: rw}
}

// recordForeignAttachLatency accumulates a real, measured end-to-end
// latency sample (ProducedAtMs on the sender to time-of-attach here — see
// that field's own comment in block.go) and periodically logs a summary
// (count/avg/max over the interval) instead of one line per block, which at
// a fast BLOCK_TIME with several peers would flood the log for no added
// signal. This is the actual, directly-measured number every BLOCK_TIME and
// circuit-breaker tuning decision needs to fit comfortably inside — added
// 2026-07-05 as a permanent operational diagnostic after a long night of
// tuning those constants without ever measuring this directly.
func (dag *BlockDAG) recordForeignAttachLatency(ms int64) {
	dag.totalForeignAttachCount.Add(1) // monotonic — see the field's comment (sync-starvation check)
	dag.foreignLatencyMu.Lock()
	dag.foreignLatencyCount++
	dag.foreignLatencySumMs += ms
	if ms > dag.foreignLatencyMaxMs {
		dag.foreignLatencyMaxMs = ms
	}
	count := dag.foreignLatencyCount
	sum := dag.foreignLatencySumMs
	maxMs := dag.foreignLatencyMaxMs
	dag.foreignLatencyMu.Unlock()

	last := dag.lastForeignLatencyLogAt.Load()
	now := time.Now().Unix()
	if now-last < int64(foreignAttachLatencyLogInterval.Seconds()) {
		return
	}
	if !dag.lastForeignLatencyLogAt.CompareAndSwap(last, now) {
		return // another goroutine's log just won the race
	}
	avg := int64(0)
	if count > 0 {
		avg = sum / int64(count)
	}
	dag.foreignLatencyMu.Lock()
	dag.foreignLatencyCount = 0
	dag.foreignLatencySumMs = 0
	dag.foreignLatencyMaxMs = 0
	dag.lastForeignLatencyWindow = latencyWindow{Count: count, AvgMs: avg, MaxMs: maxMs}
	dag.foreignLatencyMu.Unlock()
	fmt.Printf("[LATENCY] Real end-to-end attach latency over the last %s: %d sample(s), avg %dms, max %dms — the actual real-world number every BLOCK_TIME/circuit-breaker decision needs to fit inside.\n",
		foreignAttachLatencyLogInterval, count, avg, maxMs)
}

// recordRawArrivalLatency is recordForeignAttachLatency's counterpart,
// measured unconditionally at AddPeerBlock's entry before any gate — see
// that call site's own comment for why this second measurement point
// exists (a node whose circuit breaker is open produces zero
// recordForeignAttachLatency samples for exactly the direction that's
// failing, since those blocks never reach that later point at all).
func (dag *BlockDAG) recordRawArrivalLatency(ms int64) {
	dag.totalRawArrivalCount.Add(1) // monotonic — see the field's comment (sync-starvation check)
	dag.rawArrivalLatencyMu.Lock()
	dag.rawArrivalLatencyCount++
	dag.rawArrivalLatencySumMs += ms
	if ms > dag.rawArrivalLatencyMaxMs {
		dag.rawArrivalLatencyMaxMs = ms
	}
	count := dag.rawArrivalLatencyCount
	sum := dag.rawArrivalLatencySumMs
	maxMs := dag.rawArrivalLatencyMaxMs
	dag.rawArrivalLatencyMu.Unlock()

	last := dag.lastRawArrivalLatencyLogAt.Load()
	now := time.Now().Unix()
	if now-last < int64(foreignAttachLatencyLogInterval.Seconds()) {
		return
	}
	if !dag.lastRawArrivalLatencyLogAt.CompareAndSwap(last, now) {
		return // another goroutine's log just won the race
	}
	avg := int64(0)
	if count > 0 {
		avg = sum / int64(count)
	}
	dag.rawArrivalLatencyMu.Lock()
	dag.rawArrivalLatencyCount = 0
	dag.rawArrivalLatencySumMs = 0
	dag.rawArrivalLatencyMaxMs = 0
	dag.lastRawArrivalLatencyWindow = latencyWindow{Count: count, AvgMs: avg, MaxMs: maxMs}
	dag.rawArrivalLatencyMu.Unlock()
	fmt.Printf("[LATENCY-RAW] Raw arrival latency (before any gate) over the last %s: %d sample(s), avg %dms, max %dms.\n",
		foreignAttachLatencyLogInterval, count, avg, maxMs)
}

// selfProducedFinalityAllowed reports whether ProduceBlock's own
// self-produced block may advance the hard finality checkpoint right now —
// see isolatedFinalityPauseWindow's comment for the full rationale. Must be
// called with dag.mu held (reads dag.authorizedValidators, dag.selfProposer).
func (dag *BlockDAG) selfProducedFinalityAllowed() bool {
	hasOtherValidator := false
	for addr := range dag.authorizedValidators {
		if addr != dag.selfProposer {
			hasOtherValidator = true
			break
		}
	}
	if !hasOtherValidator {
		return true // genuinely solo network — advance freely (2026-07-03 fix's exact case)
	}
	last := dag.lastForeignMergeAt.Load()
	if last == 0 || time.Since(time.Unix(last, 0)) > isolatedFinalityPauseWindow {
		return false // hasn't merged ANY foreign block recently — the original 2026-07-04 case
	}
	// FIX (P0, 2026-07-10 — Primary/Contabo1 permanent-partial-merge
	// incident): the dag-wide check above only proves SOME foreign
	// validator merged recently, which stayed satisfied — and hardening
	// never paused — the entire time Primary was completely walled off
	// from Contabo1 specifically, simply because it kept merging a
	// DIFFERENT validator (cd20) fine. Pause additionally if there's a
	// validator we're still actively hearing FROM (a recent
	// lastSeenFromValidator entry — proves they're not just offline/
	// retired; see that field's own comment for why this distinction
	// matters) without a correspondingly recent successful merge.
	return !dag.isolatedFromASeenValidator()
}

// isolatedFromASeenValidator reports whether any authorized validator other
// than self has a recent lastSeenFromValidator entry (proving they're
// actively trying to reach us) without a correspondingly recent
// lastMergedFromValidator entry — see selfProducedFinalityAllowed's own
// comment for the incident this closes. A validator with NO recent "seen"
// entry is skipped entirely, not counted as isolation: validator addresses
// are never de-registered (see AuthorizedValidatorList's own comment), so a
// validator we simply haven't heard from in a long time (gone quiet,
// retired, decommissioned) must not freeze checkpoint hardening for the
// whole network forever — only a validator we're demonstrably still
// hearing from counts. Must be called with dag.mu held (reads
// dag.authorizedValidators, dag.selfProposer) — matching
// selfProducedFinalityAllowed's own precondition.
func (dag *BlockDAG) isolatedFromASeenValidator() bool {
	now := time.Now()
	dag.foreignValidatorActivityMu.Lock()
	defer dag.foreignValidatorActivityMu.Unlock()
	for addr := range dag.authorizedValidators {
		if addr == dag.selfProposer {
			continue
		}
		seenAt, seen := dag.lastSeenFromValidator[addr]
		if !seen || now.Sub(time.Unix(seenAt, 0)) > isolatedFinalityPauseWindow {
			continue // haven't heard from them recently — likely offline/retired, not our problem
		}
		mergedAt, merged := dag.lastMergedFromValidator[addr]
		if !merged || now.Sub(time.Unix(mergedAt, 0)) > isolatedFinalityPauseWindow {
			return true // hearing from them, but can't merge them — genuine isolation
		}
	}
	return false
}

// IsIsolatedFromPeers reports whether this node currently lacks a recent
// GENUINE peer merge (a block from another authorized validator actually
// accepted into the DAG) — the same signal selfProducedFinalityAllowed uses
// to pause checkpoint hardening, exported here for a second, independent
// consumer with the identical underlying risk: main.go's daily-distribution
// goroutine (distributionSyncHealthIssue).
//
// FIX (2026-07-08 incident — "merging stopped" / two independent daily
// distributions diverged the whole 3-node network): distributionSyncHealthIssue
// already refused to distribute on a degraded node, one with unverified
// synthetic-checkpoint stubs, or one with no successful PEER SYNC in the
// last hour — but a "successful peer sync" only proves this node could talk
// to a peer's HTTP API, not that it actually merged a block FROM one. A node
// can be actively isolated (circuit-breaker lockout, a fork it can't
// resolve, mid-recovery from a resync) while still passing all three of
// those checks, because polling/fetching keeps succeeding even while every
// fetched block gets orphaned. distributeValidatorsPoolLocked weights
// rewards by THIS node's own registered_nodes.blocks_produced (see
// IncrementBlockCount) — during any isolated stretch, blocks_produced for
// every OTHER validator silently stops incrementing on this node while its
// own keeps climbing, skewing the weights this node would compute if it won
// TryLockDistribution's race. Worse: TryLockDistribution's cross-node dedup
// itself depends on last_ubi_at propagating via ordinary block gossip — an
// isolated node never SEES that propagation either, so it can independently
// pass TryLockDistribution and run its OWN distribution at the same
// wall-clock trigger a healthy peer already used, producing two genuinely
// different, mutually-undetected canonical results. Confirmed as the root
// cause of the 2026-07-08 incident: Contabo1's divergence began exactly at
// the daily 20:00 distribution, immediately following a stretch where it
// was self-only producing (not merging any peer block).
func (dag *BlockDAG) IsIsolatedFromPeers() bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return !dag.selfProducedFinalityAllowed()
}

// GetFinalizedCheckpoint returns the current finalized height and blue_score.
// Returns (0, 0) on a fresh node before any checkpoint has been advanced.
// Safe to call without dag.mu/cs.mu held.
//
// FIX (P0, cadence audit 2026-07-03): this used to hit chain_config via
// getConfigValueDB on every call — two synchronous Postgres round trips —
// and is called at least twice per accepted peer block (isFinalityViolation,
// maybeAdvanceFinalizedCheckpoint), both while dag.mu is write-locked. Over
// the primary's remote DB proxy that reintroduced exactly the class of
// cadence bug the 2026-07-02 commits eliminated elsewhere (see ProduceBlock's
// post-commit bookkeeping comment), just in a path those commits didn't
// touch. The checkpoint is only ever written by this process — through
// SetFinalizedCheckpoint/ResetFinalizedCheckpoint below, both of which keep
// this cache in sync — so a cache populated once at first use is always
// current for the rest of the process's lifetime; no separate invalidation
// path is needed.
func (cs *ChainState) GetFinalizedCheckpoint() (height int64, blueScore int64) {
	cs.finalizedMu.RLock()
	if cs.finalizedCacheLoaded {
		h, s := cs.finalizedHeightCache, cs.finalizedBlueScoreCache
		cs.finalizedMu.RUnlock()
		return h, s
	}
	cs.finalizedMu.RUnlock()

	// First call this process: load once from durable storage.
	cs.finalizedMu.Lock()
	defer cs.finalizedMu.Unlock()
	if cs.finalizedCacheLoaded { // another goroutine won the race to load it
		return cs.finalizedHeightCache, cs.finalizedBlueScoreCache
	}
	var h, s int64
	fmt.Sscan(cs.getConfigValueDB("finalized_height"), &h)
	fmt.Sscan(cs.getConfigValueDB("finalized_blue_score"), &s)
	cs.finalizedHeightCache, cs.finalizedBlueScoreCache = h, s
	cs.finalizedCacheLoaded = true
	return h, s
}

// SetFinalizedCheckpoint advances the in-memory checkpoint immediately —
// every caller in this process sees the new value right away, even before
// the DB write below finishes — and persists it in the background. Caller
// must ensure this only advances (never retreats) the checkpoint.
//
// FIX (P0, cadence audit 2026-07-03): the 3 chain_config writes here used to
// run synchronously, 3 more round trips on top of GetFinalizedCheckpoint's,
// on every block that advances the checkpoint — the common case once past
// the initial finalityBlueScoreDepth blocks. Same reasoning as the
// ProduceBlock post-commit fix: this value is monotonic and
// self-re-advancing, so losing the very last write on an ill-timed crash
// only leaves the hard-finality gate briefly less strict than it could be
// after restart — never less safe than not having advanced yet at all —
// while GHOSTDAG's own probabilistic finality is unaffected either way.
func (cs *ChainState) SetFinalizedCheckpoint(hash string, height, blueScore int64) {
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = height
	cs.finalizedBlueScoreCache = blueScore
	cs.finalizedHashCache = hash
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	SafeGoroutine("SetFinalizedCheckpoint-persist", func() {
		cs.setConfigValueDB("finalized_height", fmt.Sprintf("%d", height))
		cs.setConfigValueDB("finalized_blue_score", fmt.Sprintf("%d", blueScore))
		cs.setConfigValueDB("finalized_hash", hash)
	})
}

// ResetFinalizedCheckpoint clears the finalized checkpoint back to its
// zero state (chain_config value "0" for all three keys, matching how a
// fresh node reads before any checkpoint has ever been set). Used only by
// ResyncFromSnapshotURL when chain_blocks is wiped for a clean re-sync — a
// stale finalized_height surviving the wipe deadlocks catch-up (see that
// call site's comment). Unlike SetFinalizedCheckpoint, this persists
// synchronously and returns any error: it runs during an already-slow,
// rare bootstrap path where correctness and error visibility matter more
// than shaving a few round trips. Caller must hold cs.mu (same precondition
// as the setConfigValue calls inside).
func (cs *ChainState) ResetFinalizedCheckpoint() error {
	return cs.ResetFinalizedCheckpointCtx(context.Background())
}

// ResetFinalizedCheckpointCtx ist die ctx-gefuehrte Fassung. Der Resync ruft
// sie mitten in seiner Transaktion; ihr eigener Kommentar dort sagt, dass beide
// Seiten -- Zwischenspeicher und Datenbank -- ATOMAR zuruecckgesetzt werden
// muessen. Ueber den stillen Rueckfall auf cs.activeTx war das bisher der Fall;
// jetzt steht es im Aufruf.
func (cs *ChainState) ResetFinalizedCheckpointCtx(ctx context.Context) error {
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = 0
	cs.finalizedBlueScoreCache = 0
	cs.finalizedHashCache = "0"
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	for _, fk := range []string{"finalized_height", "finalized_blue_score", "finalized_hash"} {
		if err := cs.setConfigValueCtx(ctx, fk, "0"); err != nil {
			return fmt.Errorf("could not reset %s: %w", fk, err)
		}
	}
	return nil
}

// isFinalityViolation returns true when block is so far below the current
// finalized checkpoint that accepting it would only matter for a deep reorg —
// which the finality guarantee explicitly forbids. The check uses block.Height
// (known before GHOSTDAG computation) with a generous finalityHeightSlack so
// that legitimate gap-fills within the finality window are never blocked.
// Must be called with dag.mu held (reads dag.state but not dag.blocks).
func (dag *BlockDAG) isFinalityViolation(block *Block) bool {
	if block.IsGenesis || block.FromSync {
		return false // never a finality violation for genesis or HTTP-SYNC canonical replay
	}
	// FIX (audit 2026-07-06, definitive root cause of a 3-node non-merging
	// incident): SelfFetched used to unconditionally exempt here, even
	// though it's already exempt from the circuit breaker (block.go ~3272)
	// and the suspension gate (block.go ~3179) for the identical reason — a
	// block THIS node deliberately fetched because something else is
	// actively waiting on it as a direct ancestor can never be "irrelevant
	// old history". That reasoning holds only in the PRESENT tense, though
	// (see hasAwaitingOrphan's own doc comment on exactly this past-tense-
	// vs-present-tense distinction, already applied to the bootHeight-skip
	// gate above in AddPeerBlock) — SelfFetched alone just proves a fetch
	// was ONCE issued for this exact hash, not that anything is still
	// waiting on it now. Unconditionally exempting every SelfFetched block
	// here let a much bigger problem back in: fetchMissingAncestors
	// recursively re-fetches a fetched block's OWN missing parent too, so
	// once even one genuinely stale, no-longer-relevant historical chain
	// (e.g. a peer relaying a large backlog of its own long-past
	// self-produced blocks after this node's own history diverged from
	// it) starts arriving, this exemption let the WHOLE thing cascade
	// through as an ever-deepening orphan chain — thousands of blocks
	// deep, confirmed live — instead of each one being cleanly rejected as
	// simply too old to matter. Requiring hasAwaitingOrphan(block.Hash) too
	// keeps the narrow, genuinely-needed case exempt (something real is
	// waiting on this exact hash right now) while letting a stale,
	// nothing-actually-needs-this-anymore chain hit the normal age check
	// below and get rejected instead of endlessly re-orphaning.
	if block.SelfFetched && dag.hasAwaitingOrphan(block.Hash) {
		return false
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight == 0 {
		return false // checkpoint not yet established
	}
	return block.Height < finalizedHeight-finalityHeightSlack
}

// releaseFinalitySealedStubs stops synthetic-checkpoint stubs from gating
// block production once the finalized checkpoint has moved more than
// finalityHeightSlack past them. Must be called with dag.mu held (writes
// dag.unverifiedStubHeights).
//
// Rationale: the production gate on UnverifiedSyntheticCheckpointCount
// (ProduceBlock, audit P1-05) exists so a node doesn't mint new blocks on
// top of bridged-but-unverified ancestry "until real history syncs in behind
// it". But once the finalized checkpoint is finalityHeightSlack above a
// stub's height, isFinalityViolation rejects the real block from every
// non-seed peer — the node's own hard-finality rule makes the heal the gate
// is waiting for impossible, so the gate degenerates from "wait until
// verified" into "never produce again". At that point the bridged region is
// below the hard-finality line (immutable by protocol), already reflected in
// this node's account state, and continuously cross-checked against peers
// via StateRoot comparison + divergence auto-heal — releasing the stub
// trades no real verification away. Confirmed live on Contabo 2026-07-03: a
// runtime-orphan-bridge stub over genuinely-lost blocks (gone from every
// node's DB, unhealable forever) silently turned a 2-validator network into
// a 1-validator network. A stub still WITHIN the finality window keeps
// gating production exactly as before — a fresh gap is genuinely suspect
// until the network finalizes past it.
func (dag *BlockDAG) releaseFinalitySealedStubs() {
	if dag.unverifiedSyntheticCheckpointCount.Load() == 0 || dag.state == nil {
		return
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight == 0 {
		return
	}
	for hash, h := range dag.unverifiedStubHeights {
		if h < finalizedHeight-finalityHeightSlack {
			delete(dag.unverifiedStubHeights, hash)
			dag.unverifiedSyntheticCheckpointCount.Add(-1)
			fmt.Printf("[FINALITY] ✓ Synthetic checkpoint at height %d (%s...) sealed by finality (checkpoint %d) — released from the production gate, %d unverified stub(s) remain\n",
				h, hash[:min(16, len(hash))], finalizedHeight, dag.unverifiedSyntheticCheckpointCount.Load())
		}
	}
}

// maybeAdvanceFinalizedCheckpoint walks back along the selected-parent chain
// from newBlock to find the block finalityBlueScoreDepth blue-score units
// below it, then advances the persisted checkpoint to that block if it is
// strictly higher than the current checkpoint. Must be called with dag.mu
// held (reads dag.blocks). Designed to be fast (O(finalityBlueScoreDepth)
// pointer-hops) since it is called on every accepted peer block.
func (dag *BlockDAG) maybeAdvanceFinalizedCheckpoint(newBlock *Block) {
	// Piggyback the stub-release sweep here: runs under dag.mu on every
	// accepted peer block, and no-ops via one atomic read when there are no
	// unverified stubs (the overwhelmingly common case).
	dag.releaseFinalitySealedStubs()
	if newBlock == nil || newBlock.IsGenesis || newBlock.BlueScore < finalityBlueScoreDepth {
		return
	}
	target := newBlock.BlueScore - finalityBlueScoreDepth

	// Walk selected-parent chain toward genesis until we drop to target depth.
	// Fast path only: pure in-memory pointer-hops, no DB access — this runs
	// while the caller holds dag.mu (see AddPeerBlock), so it must stay cheap.
	b := newBlock
	for b != nil && b.BlueScore > target {
		if b.SelectedParent == "" {
			return // chain not yet fully resolved (e.g. GHOSTDAG migration pending)
		}
		parent, ok := dag.blocks[b.SelectedParent]
		if !ok {
			// FIX (P0, merge-reliability audit 2026-07-03): pruneOldDAGBlocks
			// evicts in-memory blocks below (finalizedHeight - pruneBuffer()), a
			// HEIGHT-based cutoff — but this walk's target is BLUE-SCORE-based.
			// On a heavily-forked DAG (many non-selected/"red" blocks per
			// blue-score point — confirmed live: blue_score 1694 at height
			// 80095, a ~47:1 ratio) a finalityBlueScoreDepth-deep walk can need
			// to reach much further back in HEIGHT than the height-based buffer
			// guarantees, hitting an evicted (but NOT deleted — chain_blocks
			// retains it, see pruneOldDAGBlocks' own comment) parent. Silently
			// giving up here used to freeze finalized_height forever: every
			// later block hits the exact same gap on the exact same
			// selected-parent chain, which ALSO freezes pruneOldDAGBlocks' own
			// cutoff (it's finalizedHeight-relative) — a permanent,
			// self-reinforcing deadlock confirmed live on Contabo
			// (finalized_height stuck at 80095 while the tip passed 130,000,
			// which in turn left the isolated-fork auto-heal check in
			// autoheal.go permanently comparing the same stale, still-matching
			// height instead of anything recent enough to catch the actual
			// divergence). Finish the walk asynchronously against the DB
			// (which always still has it) instead of leaving the checkpoint
			// stuck — see finishCheckpointWalkFromDB.
			dag.finishCheckpointWalkFromDB(b.SelectedParent, target)
			return
		}
		b = parent
	}
	if b == nil || b.IsGenesis {
		return
	}

	curHeight, _ := dag.state.GetFinalizedCheckpoint()
	if b.Height <= curHeight {
		return // checkpoint already at or beyond this block
	}

	dag.state.SetFinalizedCheckpoint(b.Hash, b.Height, b.BlueScore)
	fmt.Printf("[FINALITY] ✓ Checkpoint advanced → height %d blue_score %d (%s...)\n",
		b.Height, b.BlueScore, b.Hash[:min(16, len(b.Hash))])
}

// finishCheckpointWalkFromDB continues the selected-parent walk started by
// maybeAdvanceFinalizedCheckpoint once it hits a block evicted from the
// in-memory DAG (dag.blocks), resuming from startHash and stopping once
// blue_score drops to target or below (or a genesis/unresolved parent is
// hit). Runs in its own goroutine, off dag.mu — safe because it only reads
// already-committed, immutable block headers (chain_blocks rows are never
// modified after insert by SaveBlockToDB's ON CONFLICT DO NOTHING) and its
// result feeds SetFinalizedCheckpoint, which has its own independent lock
// (finalizedMu — see GetFinalizedCheckpoint's comment) rather than dag.mu.
// registerFinalityWalkGap ensures fetchMissingAncestors (sync_blocks.go,
// already running on a ~1s ticker per active sync peer) will attempt to
// fetch hash from a peer.
//
// FIX (2026-07-10, closes a bug this fix's OWN first version introduced
// hours earlier): originally wrote into dag.orphans, the same map
// MissingParentHashes() exposes to doSyncOnce's wantDeepScan trigger
// (sync_blocks.go) — deepScan means "drop to height 0 and rescan the
// entire chain forward". A checkpoint walk finds a fresh gap on
// essentially every call as the target keeps sliding forward with the
// tip, so wantDeepScan stayed permanently true — confirmed live: a node
// stuck re-scanning its full history in a loop, re-importing thousands of
// ancient, disconnected block fragments as new dag.tips on every pass
// (5000+ tips, block production halted) instead of ever settling into
// steady-state real-time sync. finalityWalkGaps is a SEPARATE set
// (own field, block.go) that fetchMissingAncestors still services — see
// its own updated comment — but that wantDeepScan never looks at, so a
// checkpoint gap gets fetched without ever forcing a full rescan.
//
// Deliberately still bypasses queueOrphan: that function's far-ahead-
// frontier rejection and abandon bookkeeping are built around "a live block
// just arrived and its parent is missing" — a hard-finality checkpoint gap
// is the opposite shape (always far BEHIND the frontier, discovered by a
// background walk, not a fresh delivery) and would be wrongly dropped by
// the frontier check queueOrphan runs first.
func (dag *BlockDAG) registerFinalityWalkGap(hash string) {
	dag.orphansMu.Lock()
	if !dag.finalityWalkGaps[hash] {
		dag.finalityWalkGaps[hash] = true
		dag.orphanFirstSeen[hash] = time.Now()
	}
	dag.orphansMu.Unlock()
}

func (dag *BlockDAG) finishCheckpointWalkFromDB(startHash string, target int64) {
	SafeGoroutine("finishCheckpointWalkFromDB", func() {
		if dag.state == nil {
			return
		}
		hash := startHash
		var b *Block
		for {
			b = dag.state.LoadBlockFromDBByHash(hash)
			if b == nil {
				fmt.Printf("[FINALITY] ✗ Could not advance checkpoint: %s… is missing from both the in-memory DAG and the DB — a genuinely lost block, not just a pruned one\n",
					hash[:min(16, len(hash))])
				// FIX (2026-07-10 — closes the loop this message's own name
				// admits was never closed): this used to just log and give
				// up forever — the exact same DB lookup fails identically on
				// every subsequent call, since nothing here ever asked a
				// peer for the hash. See registerFinalityWalkGap's own
				// comment for why: a hard-finality checkpoint gap left by a
				// fresh RESYNC_FROM_SNAPSHOT whose checkpoint+sibling
				// seeding never brought this specific ancestor in is not
				// "pruned" (chain_blocks would still have it) — it is
				// genuinely absent locally and only fetchable from a peer.
				dag.registerFinalityWalkGap(hash)
				return
			}
			if b.BlueScore <= target || b.SelectedParent == "" {
				break
			}
			hash = b.SelectedParent
		}
		if b == nil {
			return
		}
		curHeight, _ := dag.state.GetFinalizedCheckpoint()
		if b.Height <= curHeight {
			return // another call already advanced past this point
		}
		dag.state.SetFinalizedCheckpoint(b.Hash, b.Height, b.BlueScore)
		fmt.Printf("[FINALITY] ✓ Checkpoint advanced (via DB fallback, in-memory DAG had pruned the path) → height %d blue_score %d (%s...)\n",
			b.Height, b.BlueScore, b.Hash[:min(16, len(b.Hash))])
	})
}
