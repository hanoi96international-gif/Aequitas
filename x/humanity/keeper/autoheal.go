package keeper

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	// chainDivergenceUnsettledSinceKey persists runChainDivergenceCheckOnce's
	// "unsettled since" clock to chain_config — see that function's own
	// comment (2026-07-09 fix) for why an in-memory-only clock let a node
	// that keeps restarting dodge chainDivergenceStallOverride forever.
	chainDivergenceUnsettledSinceKey = "chain_divergence_unsettled_since"
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
	autoHealCooldown = 30 * time.Minute
	// autoHealFailedResyncRetry is the cooldown that applies instead when the
	// PREVIOUS resync provably did not reattach this node — see
	// triggerAutoResync for the test (not one peer block merged since it ran).
	//
	// FIX (P0, 2026-07-25 — "es merged gar nix, wir drehen uns im Kreis"):
	// autoHealCooldown's rationale is protecting a legitimately slow but
	// EVENTUALLY SUCCESSFUL catch-up from being yanked into a fresh resync. A
	// node that has merged nothing at all since its last resync is the exact
	// opposite of that case, and the 30 minutes then buy nothing but 30 minutes
	// of guaranteed downtime. Both secondaries spent the day in this loop, and
	// the logs show it verbatim — Contabo2 printed
	//
	//   [AUTO-HEAL] ⏸ Divergence detected and actionable, but SUPPRESSED by the
	//   30m0s cooldown for another 22m15s ... this node is on an isolated fork
	//
	// once a minute at ascending heights from 1852371 to 1853751, roughly 26
	// minutes of confirmed, actionable, settled fork with healing switched off,
	// before finally resyncing at 17:54. Contabo1 was 11 minutes into its own
	// suppression window at the same moment, having attached 0 of 5092 received
	// blocks. That is the whole "nothing merges" symptom.
	//
	// Three minutes, not zero: a resync takes seconds and reattachment follows
	// within a block time or two, so this is still far longer than a successful
	// heal needs, and the resync path itself remains serialised by
	// resyncInProgress. It cannot become a storm — it can only stop the node
	// sitting broken for half an hour at a time.
	autoHealFailedResyncRetry = 3 * time.Minute
	autoHealCheckInterval     = 60 * time.Second

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
	//
	// FIX (P3-b, audit 2026-07-06): cross-reference — finality.go's
	// isolatedFinalityPauseWindow (10 min) addresses the same underlying
	// phenomenon (possible isolation/drift) via a lighter, self-healing
	// mechanism (pausing checkpoint hardening only, no resync). See that
	// constant's own comment for why the two timeouts deliberately differ.
	chainDivergenceStallOverride = 45 * time.Minute

	// syncStarvation* tune the FOURTH detection path (startSyncStarvationCheck
	// below), added 2026-07-24 after a live fork none of the other three paths
	// caught in time: both secondaries sat at an identical frozen tip while
	// the primary raced 400+ blocks ahead, receiving 1600+ raw block arrivals
	// per 30s window and attaching exactly ZERO of them (every peer block
	// orphaned against diverged ancestry, so the orphan backward-walk chased a
	// fork point it could not outpace). In that state:
	//   - the mismatch path is structurally blind: StateRoot mismatches only
	//     accrue on blocks that ATTACH, and nothing attaches;
	//   - the chain-divergence check compares a "settled" finalized height,
	//     which the initial-sync gate's un-settled state can defer;
	//   - the height-stall check does fire eventually — but only after 25
	//     minutes of total standstill, and every restart-based remediation
	//     attempt during the incident reset in-memory progress.
	// "Receiving plenty, attaching nothing, measurably behind the primary"
	// needs all three parts to hold SIMULTANEOUSLY for the full threshold
	// window, which no healthy state produces: normal catch-up attaches
	// constantly (sync-pulled blocks count — recordForeignAttachLatency fires
	// for every non-self block that clears the gates), a truly isolated node
	// receives nothing (no raw arrivals → heightStall's case, not this one),
	// and a node at the tip has no primary gap.
	syncStarvationCheckInterval = 60 * time.Second
	// syncStarvationThreshold: how long the starvation state must hold
	// CONTINUOUSLY before triggering. 5 minutes = 5 consecutive confirming
	// ticks — far past any transient orphan spike (those attach their backlog
	// within seconds once the parent arrives), far under the 25-minute
	// height-stall backstop this exists to beat.
	syncStarvationThreshold = 5 * time.Minute
	// syncStarvationMinArrivals: minimum raw arrivals per check interval for
	// the state to count as "receiving plenty" — well under a real flood
	// (1600+/30s live) but enough that a barely-connected node (a few stray
	// gossip blocks) doesn't qualify; that node's problem is isolation, which
	// the height-stall path already covers.
	syncStarvationMinArrivals = 30
	// syncStarvationMinGap: how far behind the primary's reported height the
	// local height must be for starvation to count — a node at/near the tip
	// that attaches nothing for a stretch is just quiet, not starving.
	syncStarvationMinGap = 120

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
// effectiveAutoHealCooldown picks which cooldown applies to this trigger, and
// reports whether the shortened one was chosen.
//
// The test is deliberately a fact about this node rather than a judgement about
// the reason string: lastSuccessfulPeerSyncAt is the timestamp of the last
// block genuinely MERGED from any peer (distinct from lastPeerContactAt, which
// mere arrival updates). If it has not moved since the last resync ran, then
// that resync did not reattach this node to the chain — nothing has come in
// since, however many blocks arrived and orphaned. Repeating it is the only
// remaining move, and the full 30 minutes is pure downtime.
//
// If it HAS moved, the node is merging peer blocks and this really might be the
// slow-but-successful catch-up autoHealCooldown exists to protect. Full
// cooldown, unchanged.
//
// lastAt == 0 (never resynced) needs neither: the caller's own guard skips the
// cooldown entirely.
func (dag *BlockDAG) effectiveAutoHealCooldown(lastAt int64) (time.Duration, bool) {
	if lastAt <= 0 {
		return autoHealCooldown, false
	}
	letzterMerge := dag.lastSuccessfulPeerSyncAt.Load()
	if letzterMerge <= lastAt {
		// Der letzte Resync hat nichts angehaengt -- ihn zu wiederholen ist
		// der einzige verbleibende Zug, und die vollen 30 Minuten waeren
		// reine Ausfallzeit.
		return autoHealFailedResyncRetry, true
	}
	// ERGAENZT 02.09.2026: der letzte Resync hat gewirkt -- aber das heisst
	// nicht, dass der Knoten JETZT in Ordnung ist.
	//
	// Genau dieser Fall ist an dem Tag am Primary aufgetreten. C1 wurde
	// resynct, lief danach sauber und mergte, und wurde SPAETER erneut
	// zugemauert: eine einzelne Ueberweisung scheiterte beim Nachspielen
	// ("insufficient balance (have 0.000000, need 0.000010)"), woraufhin der
	// GANZE Block verworfen wurde -- einmal beobachtet mit 3.525
	// Transaktionen wegen einer. Ab da fehlen die Gutschriften dieses Blocks
	// dauerhaft, jeder Folgeblock mit denselben Konten scheitert ebenso, und
	// nur ein Resync bringt den Knoten zurueck.
	//
	// Die Pruefung oben sah nur "der letzte Resync hat gemergt" und verhaengte
	// die vollen 30 Minuten. Der Knoten meldete waehrenddessen selbst
	// "structurally cut off from the canonical chain" und stand 671 Bloecke
	// zurueck -- als Primary, der Website und Explorer traegt.
	//
	// Deshalb zusaetzlich: hat der Knoten seit LAENGEREM nichts mehr
	// angehaengt, obwohl Bloecke ankommen, zaehlt er als erneut abgehaengt --
	// unabhaengig davon, wie erfolgreich der letzte Resync war. Die Schwelle
	// ist bewusst dieselbe, ab der die Aushunger-Erkennung ueberhaupt
	// ausloest: wer sie erreicht, ist nach dem Urteil dieses Knotens bereits
	// abgeschnitten.
	if time.Now().Unix()-letzterMerge > int64(syncStarvationThreshold.Seconds()) {
		return autoHealFailedResyncRetry, true
	}
	return autoHealCooldown, false
}

func (dag *BlockDAG) triggerAutoResync(reason string) {
	var lastAt int64
	fmt.Sscan(dag.state.getConfigValueDB(autoResyncLastAtKey), &lastAt)
	cooldown, lastResyncFailed := dag.effectiveAutoHealCooldown(lastAt)
	if lastAt > 0 && time.Now().Unix()-lastAt < int64(cooldown.Seconds()) {
		// FIX (observability, 2026-07-24 — cost a full diagnosis round on
		// Contabo1 tonight): this used to be a bare `return`. A node in a
		// CONFIRMED, actionable divergence state that is merely being held
		// back by the cooldown then logged absolutely nothing — indis-
		// tinguishable, from outside, from a node whose detection never
		// fired at all. Confirmed live: Contabo1 sat at height 1778536 with
		// foreign_attach count 0 against 2855 raw arrivals, having printed
		// "[AUTO-HEAL] Sync-starvation watch: received 6204 block(s) this
		// interval but attached 0 ... watching (threshold 5m0s)" and then
		// nothing whatsoever, for minutes past that threshold. The
		// suppression is legitimate; its invisibility is not — the whole
		// incident was being diagnosed through exactly these log lines.
		//
		// Rate-limited because four independent detection paths can each
		// reach this once a minute and one entry per window says all of it.
		nowNano := time.Now().UnixNano()
		last := dag.lastAutoResyncSuppressedLogAt.Load()
		if nowNano-last > int64(time.Minute) && dag.lastAutoResyncSuppressedLogAt.CompareAndSwap(last, nowNano) {
			remaining := time.Duration(int64(cooldown.Seconds())-(time.Now().Unix()-lastAt)) * time.Second
			note := ""
			if lastResyncFailed {
				note = " [shortened: the last resync merged nothing]"
			}
			fmt.Printf("[AUTO-HEAL] ⏸ Divergence detected and actionable, but SUPPRESSED by the %s cooldown for another %s%s (last auto-resync at unix %d): %s\n",
				cooldown, remaining.Round(time.Second), note, lastAt, reason)
		}
		return
	}
	if lastResyncFailed {
		fmt.Printf("[AUTO-HEAL] The resync at unix %d merged no peer block at all — not waiting out the full %s cooldown.\n", lastAt, autoHealCooldown)
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
	// FIX (P0, 2026-07-24 — why every resync re-forked within seconds at a
	// frozen, exactly-constant offset): the two steps above roll this node's
	// DAG back to a trusted checkpoint that necessarily trails the primary's
	// tip, but every "how far am I with this peer" marker outside dag.* still
	// held its pre-rollback value — so all three catch-up gates read "fully
	// caught up" on a node that was genuinely hundreds of blocks behind, and
	// production resumed immediately on a fresh fork. Both calls belong here,
	// while resyncInProgress (deferred above) still gates ProduceBlock: the
	// markers must be cleared and the gate re-armed BEFORE production can
	// resume, not a tick afterwards. See each function's own comment.
	dag.resetPeerSyncProgress()
	dag.armInitialSyncGate(true)
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
	SafeGoroutine("autoHeal-mismatch-ticker", func() {
		ticker := time.NewTicker(autoHealCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-tick — see
			// safeCall's comment (panic_recovery.go). This is the node's own
			// self-heal safety net; a panic silently ending it forever would be
			// exactly the kind of "worked fine in testing, quietly stopped
			// protecting production" failure this whole file exists to avoid.
			SafeCall("autoHeal-mismatch-tick", func() {
				if dag.TotalStateRootMismatches() < autoHealMismatchThreshold {
					return
				}
				dag.triggerAutoResync(fmt.Sprintf("%d StateRoot mismatches in the last 10 minutes — this node has diverged from its peers", dag.TotalStateRootMismatches()))
			})
		}
	})
	dag.startChainDivergenceCheck(primaryURL)
	dag.startHeightStallCheck()
	dag.startSyncStarvationCheck(primaryURL)
}

// syncStarvationTickResult is startSyncStarvationCheck's per-tick decision,
// extracted as a pure function so the exact trigger condition is unit-testable
// without tickers, network calls, or a live DAG. Inputs are the deltas since
// the previous tick plus the height comparison; returns whether this tick
// CONFIRMS the starvation state (all three conditions hold simultaneously).
// FIX (2026-08-22, after a fork this check watched and never reported):
// Contabo1 forked mid-load, froze at height 4467699 and fell 600+ blocks behind
// while receiving Contabo2's blocks the whole time. Every monitor was armed and
// correctly configured. This one, whose description fits that failure exactly,
// never confirmed a single tick.
//
// The reason was `attachDelta > 0`. A stuck node is rarely attaching NOTHING:
// it still bridges the odd orphan, still lands the occasional block on its own
// branch. One attachment anywhere in a 60-second tick reset the watch, so a
// node attaching one block a minute while losing sixty read as "normal
// (possibly slow) operation" indefinitely.
//
// Falling behind is the signal, not stillness. A node whose gap to the primary
// GROWS across a tick is losing ground however much it attached, and cannot
// recover by continuing — that is starvation whether the trickle is zero or
// not. Shrinking and steady gaps still return false, so an ordinary catch-up
// after a restart (large gap, closing fast) is untouched.
//
// prevGap is the previous tick's gap, negative when there is none yet (first
// tick, or the primary was unreachable). The returned gap is what the caller
// remembers for next time, and is negative when this tick could not measure one.
func syncStarvationTickConfirms(rawDelta, attachDelta, localHeight, primaryHeight, prevGap int64, primaryReachable bool) (confirmed bool, gap int64) {
	if !primaryReachable {
		return false, -1 // no height comparison possible — not evidence either way
	}
	if rawDelta < syncStarvationMinArrivals {
		return false, -1 // not actually receiving — isolation, heightStall's case
	}
	gap = primaryHeight - localHeight
	if gap < syncStarvationMinGap {
		return false, gap // close enough to be ordinary lag
	}
	if attachDelta == 0 {
		return true, gap // attached nothing at all while far behind
	}
	// Attached something, but still losing ground: a trickle is not recovery.
	if prevGap >= 0 && gap > prevGap {
		return true, gap
	}
	return false, gap
}

// startSyncStarvationCheck is the fourth, independent detection path — see
// the syncStarvation* constants' comment for the live 2026-07-24 fork it
// exists to catch quickly. Reads only monotonic counters and dag.Height()
// locally plus the primary's /api/status height (the same cheap probe
// runChainDivergenceCheckOnce already uses); no state mutated except via
// the shared, cooldown-gated triggerAutoResync.
func (dag *BlockDAG) startSyncStarvationCheck(primaryURL string) {
	if primaryURL == "" {
		fmt.Println("[AUTO-HEAL] Sync-starvation self-check disabled: PRIMARY_NODE_URL not set.")
		return
	}
	fmt.Printf("[AUTO-HEAL] Sync-starvation self-check enabled: will resync if this node receives blocks but attaches none for %s straight while %d+ behind %s.\n",
		syncStarvationThreshold, syncStarvationMinGap, primaryURL)
	SafeGoroutine("syncStarvation-ticker", func() {
		ticker := time.NewTicker(syncStarvationCheckInterval)
		defer ticker.Stop()
		prevRaw := dag.totalRawArrivalCount.Load()
		prevAttach := dag.totalForeignAttachCount.Load()
		var starvingSince time.Time
		// Negative means "no previous gap measured", so the first tick can never
		// confirm on a growth comparison it has no baseline for.
		prevGap := int64(-1)
		for range ticker.C {
			SafeCall("syncStarvation-tick", func() {
				curRaw := dag.totalRawArrivalCount.Load()
				curAttach := dag.totalForeignAttachCount.Load()
				rawDelta := curRaw - prevRaw
				attachDelta := curAttach - prevAttach
				prevRaw, prevAttach = curRaw, curAttach

				primaryHeight, ok := fetchPrimaryHeight(primaryURL)
				confirmed, gap := syncStarvationTickConfirms(rawDelta, attachDelta, dag.Height(), primaryHeight, prevGap, ok)
				prevGap = gap
				if !confirmed {
					starvingSince = time.Time{}
					return
				}
				if starvingSince.IsZero() {
					starvingSince = time.Now()
					// Reports what actually confirmed the tick. The old wording said
					// "attached 0" unconditionally, which became wrong the moment a
					// GROWING GAP could confirm too: an operator would read zero
					// attachments and go looking for an isolated node, when the real
					// state was a node attaching a trickle and still losing ground.
					fmt.Printf("[AUTO-HEAL] Sync-starvation watch: received %d block(s) this interval, attached %d, now %d behind the primary — watching (threshold %s).\n",
						rawDelta, attachDelta, gap, syncStarvationThreshold)
					return
				}
				if starving := time.Since(starvingSince); starving >= syncStarvationThreshold {
					dag.triggerAutoResync(fmt.Sprintf(
						"receiving peer blocks continuously for %s straight while falling further behind the primary (now %d blocks) — arrivals are orphaning against diverged ancestry faster than this node can attach them; it is structurally cut off from the canonical chain",
						starving.Round(time.Second), gap))
				}
			})
		}
	})
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
	SafeGoroutine("heightStall-ticker", func() {
		ticker := time.NewTicker(heightStallCheckInterval)
		defer ticker.Stop()
		lastHeight := int64(-1)
		var lastChangedAt time.Time
		for range ticker.C {
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-tick — see
			// safeCall's comment. lastHeight/lastChangedAt are closed over by
			// value across iterations (loop-scoped, not tick-scoped), so a
			// recovered panic mid-tick just skips that tick's update/decision —
			// the next tick re-reads dag.Height() fresh and continues correctly.
			SafeCall("heightStall-tick", func() {
				h := dag.Height()
				if h != lastHeight {
					lastHeight = h
					lastChangedAt = time.Now()
					return
				}
				if lastChangedAt.IsZero() {
					lastChangedAt = time.Now()
					return
				}
				if stalled := time.Since(lastChangedAt); stalled >= heightStallThreshold {
					dag.triggerAutoResync(fmt.Sprintf(
						"dag.height has not advanced at all in %s (stuck at %d) — a real historical catch-up snag resolves well within this window (the 2026-07-03 catch-up saga measured ~20 minutes worst case); this looks permanently stuck instead",
						stalled.Round(time.Second), h))
				}
			})
		}
	})
}

// startChainDivergenceCheck is the second detection path: instead of waiting
// to receive a conflicting block (which never happens for a fully isolated
// fork — see the package doc comment above), it actively asks the primary
// "does your block at height N match mine?" at a height old enough on both
// sides to be settled. A DAG's live tips can legitimately differ between
// honest nodes (see LatestBlock's doc comment) — a finalized height cannot.
// loadUnsettledSinceFromDB reads the persisted "unsettled since" timestamp
// (see chainDivergenceUnsettledSinceKey) so startChainDivergenceCheck's
// stall clock survives a process restart instead of resetting to zero.
// Returns the zero Time (== "currently settled, or never been unsettled")
// if the key is absent, empty, or unparseable.
func (dag *BlockDAG) loadUnsettledSinceFromDB() time.Time {
	v := dag.persistedUnsettledSince()
	if v == "" {
		return time.Time{}
	}
	unix, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

// persistedUnsettledSince and setPersistedUnsettledSince wrap
// chainDivergenceUnsettledSinceKey's chain_config read/write, nil-safe for
// dag.state — some lightweight BlockDAG test fixtures construct a DAG with
// no ChainState attached at all (only ever exercising the in-memory
// unsettledSince pointer, never real persistence), so this must degrade to
// a no-op/empty-read rather than panic when dag.state is nil.
func (dag *BlockDAG) persistedUnsettledSince() string {
	if dag.state == nil {
		return ""
	}
	return dag.state.getConfigValueDB(chainDivergenceUnsettledSinceKey)
}

func (dag *BlockDAG) setPersistedUnsettledSince(value string) {
	if dag.state == nil {
		return
	}
	dag.state.setConfigValueDB(chainDivergenceUnsettledSinceKey, value)
}

// No-op if primaryURL is empty (PRIMARY_NODE_URL not configured).
func (dag *BlockDAG) startChainDivergenceCheck(primaryURL string) {
	if primaryURL == "" {
		fmt.Println("[AUTO-HEAL] Chain-divergence self-check disabled: PRIMARY_NODE_URL not set.")
		return
	}
	fmt.Printf("[AUTO-HEAL] Chain-divergence self-check enabled: comparing against %s every %s.\n", primaryURL, chainDivergenceCheckInterval)
	SafeGoroutine("chainDivergenceCheck-ticker", func() {
		// FIX (durable fix, 2026-07-09 — closes a second self-heal blind
		// spot found live tonight, distinct from the one below): this used
		// to start from the zero value unconditionally, so a node that gets
		// restarted more often than chainDivergenceStallOverride (45min) —
		// exactly what happens during a live incident where an operator is
		// repeatedly redeploying/resyncing it — could NEVER accumulate
		// enough continuous unsettled time to trigger the override, no
		// matter how long it had actually been isolated for in wall-clock
		// terms. Confirmed live: a node stuck on an isolated fork for
		// well over an hour, but restarted every few minutes throughout,
		// never once reached the 45-minute override because each restart
		// reset this clock back to zero. Seeding from the DB-persisted
		// value (written by runChainDivergenceCheckOnce below every time
		// this transitions) instead of the zero value means the clock
		// survives restarts — only a genuine return to a settled state
		// resets it now, not a process restart.
		unsettledSince := dag.loadUnsettledSinceFromDB()
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
		// FIX (P0-3, beta-launch audit 2026-07-05): recover per-call — see
		// safeCall's comment. Same self-heal-safety-net reasoning as the two
		// tickers above: one panicked round must not silently end every
		// future round of this check for the rest of the node's uptime.
		SafeCall("chainDivergenceCheck-initial", func() { dag.runChainDivergenceCheckOnce(primaryURL, &unsettledSince) })
		ticker := time.NewTicker(chainDivergenceCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			SafeCall("chainDivergenceCheck-tick", func() { dag.runChainDivergenceCheckOnce(primaryURL, &unsettledSince) })
		}
	})
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
		// FIX (durable fix, 2026-07-09): clear the persisted clock too, not
		// just the in-memory one — otherwise a node that settles cleanly,
		// then restarts, would resume from the stale pre-settlement
		// timestamp instead of correctly starting fresh. See
		// startChainDivergenceCheck's own comment for the restart-survival
		// half of this fix.
		if !unsettledSince.IsZero() {
			dag.setPersistedUnsettledSince("")
		}
		*unsettledSince = time.Time{}
	} else if unsettledSince.IsZero() {
		*unsettledSince = time.Now()
		dag.setPersistedUnsettledSince(strconv.FormatInt(unsettledSince.Unix(), 10))
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
