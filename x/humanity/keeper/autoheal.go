package keeper

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Divergence auto-heal: a secondary node that forks from the primary and racks
// up sustained StateRoot mismatches recovers itself, instead of sitting
// divergent until an operator manually sets RESYNC_FROM_SNAPSHOT=true and
// restarts it (exactly the manual intervention this codebase needed when the
// Contabo VPS diverged — 3000+ mismatches, 2 humans instead of 4).
//
// That mismatch-based detection only fires when this node RECEIVES a
// conflicting block from a peer. A node that has drifted onto a fully
// isolated fork (e.g. it thinks it's ahead of everyone, so it never pulls
// from anyone) never receives anything conflicting, so mismatches never
// accumulate and this path never fires. startChainDivergenceCheck below is
// the second, independent detection path that covers exactly that case by
// actively comparing this node's own chain against the primary's.
const (
	// autoResyncPendingKey is the chain_config flag a runtime-detected
	// divergence sets so the NEXT boot performs an authoritative resync.
	autoResyncPendingKey = "pending_auto_resync"
	// autoResyncLastAtKey stamps the last auto-heal trigger for the cooldown.
	autoResyncLastAtKey = "auto_resync_last_at"
	// autoHealMismatchThreshold is how many StateRoot mismatches within the
	// 10-minute active window (see TotalStateRootMismatches) count as a real,
	// sustained divergence. A version rollout produces only a handful (the
	// live Railint→Contabo upgrade produced 5), so 50 is comfortably clear of
	// false positives while any genuine fork blows past it in a few minutes.
	autoHealMismatchThreshold = 50
	// autoHealCooldown gates how often triggerAutoResync can flag a fresh
	// resync — the shared backstop against two divergence signals firing
	// close together turning into a restart loop.
	//
	// REVERTED (2026-07-03, merge-reliability audit — see the temporary
	// 5-minute value's own history in commit c653867): the 5-minute value
	// was a deliberate, explicitly-temporary drop for one evening's
	// same-session verification of the circuit-breaker fixes. It was never
	// the fix for the actual "Contabo never merges" symptom — the real root
	// cause turned out to be that /api/blocks (and the hash-lookup sync
	// endpoints) only ever served dag.blocks' in-memory, pruned window, with
	// no DB fallback (see GetBlocksSince/GetBlockByHash/
	// GetBlocksByHashesForPeer's fix comments) — so a resyncing node could
	// never actually close the gap no matter how long it waited, and kept
	// looking like it needed ANOTHER resync every cooldown cycle. A short
	// cooldown only made that failure mode worse: it let a node get yanked
	// into a fresh resync before a legitimately-slow (but eventually
	// successful) catch-up ever had a chance to finish, which is exactly
	// the restart-loop risk this comment warned about at the time. Back to
	// 30 minutes now that the actual gap in the sync path is fixed.
	autoHealCooldown      = 30 * time.Minute
	autoHealCheckInterval = 60 * time.Second

	// chainDivergenceCheckInterval paces the active primary-comparison check.
	//
	// TIGHTENED (durable fix, 2026-07-04 — "nodes must stay on the same
	// block as the primary, permanently"): was 5 minutes, on the reasoning
	// that this is a network round trip against the primary rather than a
	// local counter. That reasoning undersold the actual cost: it's two
	// cheap HTTP GETs (a status call + one block-by-height fetch), not a
	// heavy operation, and correcting a real divergence now runs in-process
	// (see PerformResync) instead of forcing a disruptive restart — so there
	// is no longer a reason to let a real divergence sit for up to 5 minutes
	// before it's even noticed. 60s matches heightStallCheckInterval's
	// cadence; the unsettled-state skip and chainDivergenceStallOverride
	// above already guard against false positives from a node mid-catch-up,
	// so a tighter interval only means faster correction, not more noise.
	chainDivergenceCheckInterval = 60 * time.Second
	// chainDivergenceInitialDelay is how long startChainDivergenceCheck waits
	// before its FIRST comparison, run once outside the normal ticker cadence
	// — see that function's own comment for why: time.Ticker never fires on
	// its own start, only after a full interval, so without this a node that
	// gets restarted more often than chainDivergenceCheckInterval (routine
	// during active deploys, and only more likely as a fleet grows to many
	// independently-restarting nodes) could complete zero divergence checks
	// in its entire lifetime. Long enough for peer registration and initial
	// sync to get underway, far short of the full 5-minute interval.
	chainDivergenceInitialDelay = 45 * time.Second
	// chainDivergenceSafetyMargin keeps the compared height well clear of the
	// live tip, where legitimate reorg/merge activity is still happening.
	chainDivergenceSafetyMargin = 50

	// chainDivergenceMaxTipsForCheck gates the check on tip-count fragmentation
	// as a proxy for "not in a settled state right now" — see the
	// isCatchingUpLocked-OR-high-fragmentation guard in
	// startChainDivergenceCheck below for why this exists.
	chainDivergenceMaxTipsForCheck = 200

	// chainDivergenceStallOverride bounds how long the unsettled-state skip
	// above can suppress the check for the SAME node continuously. Found live
	// 2026-07-04: a node that has drifted onto its own isolated, self-produced
	// fork (signing under a key nobody else ever authorized — see
	// authorizedValidators/AddAuthorizedValidator, block.go) racks up
	// thousands of dangling tips and NEVER consolidates them, because
	// consolidation depends on real peers merging its blocks back in and a
	// truly isolated node has no real peers. That means len(dag.tips) >
	// chainDivergenceMaxTipsForCheck stays permanently true for exactly the
	// node this check exists to catch — the false-positive guard above
	// silently disabled the real detection for its entire runtime (confirmed:
	// 1000+ synthetic checkpoint stubs and 5000+ StateRoot mismatches
	// accumulated over 20+ minutes, tips never dropping, check skipped every
	// single round). Legitimate heavy catch-up (the scenario the guard above
	// was written for) consolidates within ~20 minutes even through a
	// historical orphan wall (see the 2026-07-03 catch-up saga) — so after
	// staying continuously unsettled for well past that, treat it as a
	// signal in its own right and run the comparison anyway rather than
	// skipping forever.
	chainDivergenceStallOverride = 45 * time.Minute

	// heightStallCheckInterval paces startHeightStallCheck's liveness poll.
	// Cheap (dag.Height() is a single RLock + field read), so this can be
	// much tighter than the divergence check's network round trip.
	heightStallCheckInterval = 60 * time.Second
	// heightStallThreshold is how long dag.height may sit completely
	// unchanged before startHeightStallCheck treats it as a genuine,
	// permanent liveness failure rather than a node still working through a
	// legitimate historical catch-up snag — see that function's own comment
	// for the exact live gap this closes (a mass-reconsolidation event pins
	// dag_tips_count above chainDivergenceMaxTipsForCheck, which makes
	// startChainDivergenceCheck correctly refuse to compare, but that also
	// means it can never detect a node that's permanently stuck in that
	// exact state rather than still progressing through it). Set comfortably
	// longer than the ~20-minute worst case the 2026-07-03 catch-up saga
	// measured for a real historical wall to fully clear on its own, so this
	// never fights that mechanism's natural recovery — it only fires for a
	// node that has demonstrably stopped moving, not one still working.
	heightStallThreshold = 25 * time.Minute
)

// AutoResyncRequested reports whether a previous run's auto-heal monitor
// flagged this node for an authoritative resync on boot. main.go ORs this into
// its RESYNC_FROM_SNAPSHOT check so the safe startup resync path runs.
func (cs *ChainState) AutoResyncRequested() bool {
	return cs.getConfigValueDB(autoResyncPendingKey) == "1"
}

// ClearAutoResyncRequest clears the pending-resync flag after a successful
// boot-time heal, so the node doesn't resync again on every subsequent restart.
func (cs *ChainState) ClearAutoResyncRequest() {
	_ = cs.setConfigValueDB(autoResyncPendingKey, "")
}

// triggerAutoResync is the shared, cooldown-gated trigger both detection
// paths below funnel through, so two divergence signals firing close
// together can't turn into a restart loop.
//
// FIX (durable fix, 2026-07-04 — direct answer to "nodes must stay on the
// same block as the primary, permanently"): this used to ONLY flag
// chain_config and os.Exit(1), relying on the container restart policy to
// bring the node back through the safe boot-time resync path in main.go.
// That worked, but the restart itself was a recurring source of NEW
// instability every single time it fired tonight — a fresh restart resets
// every in-memory timer (the orphan-bridge's 15-minute-per-hash TTL, the
// circuit breakers' failure-run counters, dag.tips/dag.blocks) and often
// forced a node straight back into the slow historical-catch-up state the
// resync was meant to fix in the first place, sometimes for 20+ minutes.
// Now: attempt the exact same resync sequence PerformResync runs today at
// startup, but in-process, live, without exiting — production/acceptance
// are gated off for its few-second duration (resyncInProgress) instead of
// the node going away entirely. Falls back to the old restart-based flag
// path only if the in-process attempt can't even be attempted (config
// missing) or itself fails, so a broken in-process path never leaves the
// node stuck worse off than before this change.
func (dag *BlockDAG) triggerAutoResync(reason string) {
	var lastAt int64
	fmt.Sscan(dag.state.getConfigValueDB(autoResyncLastAtKey), &lastAt)
	if lastAt > 0 && time.Now().Unix()-lastAt < int64(autoHealCooldown.Seconds()) {
		return
	}
	fmt.Printf("[AUTO-HEAL] ⚠ %s\n", reason)
	_ = dag.state.setConfigValueDB(autoResyncLastAtKey, fmt.Sprintf("%d", time.Now().Unix()))

	if dag.resyncBootstrapURL != "" && dag.resyncSigner != "" {
		fmt.Println("[AUTO-HEAL] Attempting an in-process resync (no restart) first.")
		if err := dag.PerformResync(dag.resyncBootstrapURL, dag.resyncSigner, dag.resyncPrimaryURL); err != nil {
			fmt.Printf("[AUTO-HEAL] ✗ In-process resync failed (%v) — falling back to the restart-based resync path.\n", err)
		} else {
			fmt.Println("[AUTO-HEAL] ✓ In-process resync succeeded — no restart needed.")
			return
		}
	}

	fmt.Println("[AUTO-HEAL] Flagging an authoritative resync and restarting so the safe boot-time resync path can run.")
	if err := dag.state.setConfigValueDB(autoResyncPendingKey, "1"); err != nil {
		fmt.Printf("[AUTO-HEAL] ✗ Could not persist the resync flag (%v) — staying up rather than restarting into an unflagged loop.\n", err)
		return
	}
	fmt.Println("[AUTO-HEAL] Restarting now.")
	os.Exit(1)
}

// PerformResync runs the exact same authoritative-resync sequence main.go
// performs at boot under RESYNC_FROM_SNAPSHOT=true (replace account state
// from a signed snapshot, seed a trusted checkpoint, refresh bootHeight,
// bridge any historical gap, clear stale breaker state) but in-process,
// live, without a container restart. Gated by resyncInProgress so
// ProduceBlock/AddPeerBlock cannot interleave with the state swap (see that
// field's own comment). Each step below manages its own, independent lock
// (cs.mu, proposerBreakerMu, dag.mu) rather than one held across the whole
// call, exactly like the boot-time sequence in main.go — safe to run
// alongside concurrent API reads, which simply block briefly on whichever
// lock they need.
func (dag *BlockDAG) PerformResync(bootstrapURL, signer, primaryURL string) error {
	dag.resyncInProgress.Store(true)
	defer dag.resyncInProgress.Store(false)

	if err := dag.state.ResyncFromSnapshotURL(bootstrapURL, signer); err != nil {
		return fmt.Errorf("resync from snapshot: %w", err)
	}
	dag.state.ClearAutoResyncRequest()
	dag.ClearProposerCircuitBreakers()
	// Best-effort: falls back to a full genesis replay internally if this
	// fails, never blocks or fails the resync itself (see its own comment).
	dag.SeedTrustedCheckpoint(primaryURL)
	dag.RefreshBootHeightAfterSnapshotImport(true)
	if primaryURL != "" {
		dag.BridgeHistoricalGap([]string{primaryURL})
	}
	return nil
}

// StartDivergenceAutoHeal launches both background divergence monitors
// described above. No-op unless AUTO_HEAL_ON_DIVERGENCE=true AND a signed
// snapshot source is configured (bootstrapURL+signer) — the primary has
// neither, so it can never wipe itself, and a misconfigured node never
// self-wipes blind.
//
// The resync is signature-verified against BOOTSTRAP_SIGNER, so even a false
// trigger only re-mirrors the trusted primary — the state a secondary should
// have anyway — never arbitrary data. The cooldown makes a resync that fails
// to converge fall back to running divergent rather than a restart loop.
func (dag *BlockDAG) StartDivergenceAutoHeal(bootstrapURL, signer, primaryURL string) {
	if os.Getenv("AUTO_HEAL_ON_DIVERGENCE") != "true" {
		return
	}
	if bootstrapURL == "" || signer == "" {
		fmt.Println("[AUTO-HEAL] Disabled: AUTO_HEAL_ON_DIVERGENCE=true but BOOTSTRAP_SNAPSHOT_URL/BOOTSTRAP_SIGNER are not both set — refusing to self-wipe without a trusted, signed snapshot source.")
		return
	}
	// Cached so triggerAutoResync can call PerformResync directly — see
	// these fields' own struct comment for why this avoids a signature
	// change at every one of triggerAutoResync's three call sites.
	dag.resyncBootstrapURL = bootstrapURL
	dag.resyncSigner = signer
	dag.resyncPrimaryURL = primaryURL
	fmt.Printf("[AUTO-HEAL] Enabled: will resync in-process from %s (signer %s) if >%d StateRoot mismatches accumulate within 10 min.\n",
		bootstrapURL, signer, autoHealMismatchThreshold)
	go func() {
		ticker := time.NewTicker(autoHealCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			if dag.TotalStateRootMismatches() < autoHealMismatchThreshold {
				continue
			}
			dag.triggerAutoResync(fmt.Sprintf("%d StateRoot mismatches in the last 10 minutes — this node has diverged from its peers", dag.TotalStateRootMismatches()))
		}
	}()
	dag.startChainDivergenceCheck(primaryURL)
	dag.startHeightStallCheck()
}

// primaryStatusResponse mirrors the subset of /api/status this check needs.
type primaryStatusResponse struct {
	Height int64 `json:"height"`
}

// startHeightStallCheck is a THIRD, independent detection path — a plain
// liveness check, not a divergence comparison. Found live 2026-07-04: a
// node hitting the historical-orphan-wall mass-reconsolidation (dag_tips_count
// in the low hundreds while it re-derives its own already-persisted history
// after a restart) is EXACTLY the state startChainDivergenceCheck's
// unsettled-guard was designed to skip (chainDivergenceMaxTipsForCheck) —
// correctly, since comparing against the primary mid-reconsolidation
// produces false positives (see that guard's own comment). But that means
// the divergence check can NEVER catch a node that's genuinely, permanently
// stuck in this state (as opposed to one still working through it): 5+
// minutes of dag.height not moving AT ALL, tips pinned above the unsettled
// threshold, with no comparison ever running because the guard correctly
// refuses to compare an unsettled node — a real liveness failure with zero
// detection path. This check doesn't compare against anything; it only asks
// "has dag.height changed at all recently" — if not, for long enough, that
// alone is conclusive regardless of divergence status.
func (dag *BlockDAG) startHeightStallCheck() {
	fmt.Printf("[AUTO-HEAL] Height-stall self-check enabled: will resync if dag.height doesn't advance at all for %s straight.\n", heightStallThreshold)
	go func() {
		ticker := time.NewTicker(heightStallCheckInterval)
		defer ticker.Stop()
		lastHeight := int64(-1)
		var lastChangedAt time.Time
		for range ticker.C {
			h := dag.Height()
			if h != lastHeight {
				lastHeight = h
				lastChangedAt = time.Now()
				continue
			}
			if lastChangedAt.IsZero() {
				lastChangedAt = time.Now()
				continue
			}
			if stalled := time.Since(lastChangedAt); stalled >= heightStallThreshold {
				dag.triggerAutoResync(fmt.Sprintf(
					"dag.height has not advanced at all in %s (stuck at %d) — a real historical catch-up snag resolves well within this window (the 2026-07-03 catch-up saga measured ~20 minutes worst case); this looks permanently stuck instead",
					stalled.Round(time.Second), h))
			}
		}
	}()
}

// startChainDivergenceCheck is the second detection path: instead of waiting
// to receive a conflicting block (which never happens for a fully isolated
// fork — see the package doc comment above), it actively asks the primary
// "does your block at height N match mine?" at a height old enough on both
// sides to be settled. A DAG's live tips can legitimately differ between
// honest nodes (see LatestBlock's doc comment) — a finalized height cannot.
// No-op if primaryURL is empty (PRIMARY_NODE_URL not configured).
func (dag *BlockDAG) startChainDivergenceCheck(primaryURL string) {
	if primaryURL == "" {
		fmt.Println("[AUTO-HEAL] Chain-divergence self-check disabled: PRIMARY_NODE_URL not set.")
		return
	}
	fmt.Printf("[AUTO-HEAL] Chain-divergence self-check enabled: comparing against %s every %s.\n", primaryURL, chainDivergenceCheckInterval)
	go func() {
		var unsettledSince time.Time
		// FIX (P0, 2026-07-04 — closes the self-heal blind spot found live
		// tonight): time.Ticker does NOT fire on start, only after the FIRST
		// full interval elapses — with chainDivergenceCheckInterval at 5
		// minutes, a node that gets restarted more often than that (routine
		// during any active deploy/debug session, and increasingly likely
		// as a fleet grows to many nodes each with their own independent
		// restart schedule) could go its ENTIRE life without this safety net
		// firing even once. Confirmed as the actual explanation for a real
		// divergence (a fixed, incorrect blue_score baseline from a botched
		// resync) surviving undetected for a long stretch of very frequent
		// redeploys tonight — not a logic bug in the check itself, just zero
		// completed cycles. chainDivergenceInitialDelay runs the same check
		// once, soon after startup (long enough for peer registration and
		// initial sync to get going, far short of a full interval), then the
		// ticker takes over for the normal cadence — so even a node that
		// only lives a few minutes gets at least one real chance to compare
		// itself against the primary.
		time.Sleep(chainDivergenceInitialDelay)
		dag.runChainDivergenceCheckOnce(primaryURL, &unsettledSince)
		ticker := time.NewTicker(chainDivergenceCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			dag.runChainDivergenceCheckOnce(primaryURL, &unsettledSince)
		}
	}()
}

// runChainDivergenceCheckOnce is startChainDivergenceCheck's single-round
// body, extracted so it can run once shortly after startup AND on the
// normal ticker cadence — see startChainDivergenceCheck's own comment for
// why the immediate first run matters. unsettledSince is owned by the
// caller's goroutine and threaded through by pointer so state carries
// across calls exactly as it did as a loop-local variable before.
func (dag *BlockDAG) runChainDivergenceCheckOnce(primaryURL string, unsettledSince *time.Time) {
	// FIX (durable fix, 2026-07-03 — real fix for the false-positive
	// resync loop documented in the catch-up saga): this check compares
	// a "safely finalized" local height against the primary's, on the
	// assumption both sides have settled. That assumption breaks down
	// during a mass-reconsolidation event (heavy catch-up or a tip
	// count blown up into the thousands from historically-resurrected
	// orphans) — confirmed live: a node mid-catch-up with dag_tips_count
	// in the 2000-4500 range had ProduceBlock taking 7+ seconds per
	// block, and its own finalized-checkpoint walk transiently picked a
	// different block than the primary's canonical one without a real
	// fork existing, tripping this check and forcing a full resync —
	// which promptly re-triggered the same fragmentation on the next
	// catch-up, a genuine restart-loop risk. Skip the round entirely
	// while either signal indicates "not settled right now"; the next
	// tick re-checks, so a transient burst just delays detection by one
	// interval rather than producing a false trigger.
	//
	// FIX (2026-07-04): but only skip like that for so long — see
	// chainDivergenceStallOverride's own comment. A node stuck
	// unsettled continuously past that ceiling falls through to the
	// real check below instead of skipping forever.
	dag.mu.RLock()
	unsettled := dag.isCatchingUpLocked() || len(dag.tips) > chainDivergenceMaxTipsForCheck
	tipsNow := len(dag.tips)
	dag.mu.RUnlock()
	if !unsettled {
		*unsettledSince = time.Time{}
	} else if unsettledSince.IsZero() {
		*unsettledSince = time.Now()
		fmt.Printf("[AUTO-HEAL] Chain-divergence self-check skipped this round: node is still catching up or has %d tips (fragmentation) — not a settled state to compare against the primary.\n", tipsNow)
		return
	} else if stalled := time.Since(*unsettledSince); stalled < chainDivergenceStallOverride {
		fmt.Printf("[AUTO-HEAL] Chain-divergence self-check skipped this round: node is still catching up or has %d tips (fragmentation) — not a settled state to compare against the primary (unsettled for %s so far).\n", tipsNow, stalled.Round(time.Second))
		return
	} else {
		fmt.Printf("[AUTO-HEAL] Chain-divergence self-check overriding the unsettled-state skip: %d tips and unsettled for over %s straight — legitimate catch-up should have consolidated well before now, this looks like a permanently isolated fork instead. Checking anyway.\n", tipsNow, chainDivergenceStallOverride)
	}
	remoteHeight, ok := fetchPrimaryHeight(primaryURL)
	if !ok {
		// Primary unreachable this cycle — not evidence of divergence.
		return
	}
	localFinalized, _ := dag.state.GetFinalizedCheckpoint()
	compareHeight := localFinalized
	if remoteHeight < compareHeight {
		compareHeight = remoteHeight
	}
	compareHeight -= chainDivergenceSafetyMargin
	if compareHeight <= 0 {
		return // chain too young to compare safely yet
	}
	localBlock := dag.GetBlockByHeight(compareHeight)
	if localBlock == nil {
		return // not yet synced to this height locally — not evidence
	}
	remoteHash, ok := fetchPrimaryBlockHash(primaryURL, compareHeight)
	if !ok {
		return
	}
	if remoteHash != localBlock.Hash {
		dag.triggerAutoResync(fmt.Sprintf(
			"chain hash mismatch at height %d against primary %s (ours=%s… theirs=%s…) — this node is on an isolated fork",
			compareHeight, primaryURL, localBlock.Hash[:min(16, len(localBlock.Hash))], remoteHash[:min(16, len(remoteHash))]))
	}
}

// fetchPrimaryHeight returns primaryURL's current /api/status height.
func fetchPrimaryHeight(primaryURL string) (int64, bool) {
	resp, err := httpSyncClient.Get(primaryURL + "/api/status")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, false
	}
	var status primaryStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return 0, false
	}
	return status.Height, true
}

// fetchPrimaryBlockHash returns the block hash primaryURL reports at height.
func fetchPrimaryBlockHash(primaryURL string, height int64) (string, bool) {
	resp, err := httpSyncClient.Get(fmt.Sprintf("%s/api/block?height=%d", primaryURL, height))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false
	}
	var block Block
	if err := json.NewDecoder(resp.Body).Decode(&block); err != nil {
		return "", false
	}
	return block.Hash, true
}
