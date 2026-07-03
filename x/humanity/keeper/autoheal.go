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
	// TEMPORARY (2026-07-03, user-requested): dropped from 30 * time.Minute
	// to 5 * time.Minute to speed up verification of the same-evening
	// circuit-breaker fixes (commits ca43e28/cea66b0) without waiting out a
	// full 30-minute cooldown between test cycles. REVERT TO 30 MINUTES once
	// those fixes are confirmed stable — 5 minutes is fine for an actively-
	// monitored debugging session but is more restart-loop-prone than
	// intended for unattended long-term production operation if a resync
	// ever repeatedly fails to converge.
	autoHealCooldown      = 5 * time.Minute
	autoHealCheckInterval = 60 * time.Second

	// chainDivergenceCheckInterval paces the active primary-comparison check.
	// This is a network round trip against the primary, not a local counter,
	// so it doesn't need the mismatch check's 60s granularity.
	chainDivergenceCheckInterval = 5 * time.Minute
	// chainDivergenceSafetyMargin keeps the compared height well clear of the
	// live tip, where legitimate reorg/merge activity is still happening.
	chainDivergenceSafetyMargin = 50
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
// together can't turn into a restart loop. It does NOT resync live — see
// StartDivergenceAutoHeal's doc comment for why: it flags chain_config and
// exits, and the container restart policy brings the node back through the
// safe boot-time resync path in main.go.
func (dag *BlockDAG) triggerAutoResync(reason string) {
	var lastAt int64
	fmt.Sscan(dag.state.getConfigValueDB(autoResyncLastAtKey), &lastAt)
	if lastAt > 0 && time.Now().Unix()-lastAt < int64(autoHealCooldown.Seconds()) {
		return
	}
	fmt.Printf("[AUTO-HEAL] ⚠ %s — flagging an authoritative resync and restarting so the safe boot-time resync path can run.\n", reason)
	if err := dag.state.setConfigValueDB(autoResyncPendingKey, "1"); err != nil {
		fmt.Printf("[AUTO-HEAL] ✗ Could not persist the resync flag (%v) — staying up rather than restarting into an unflagged loop.\n", err)
		return
	}
	_ = dag.state.setConfigValueDB(autoResyncLastAtKey, fmt.Sprintf("%d", time.Now().Unix()))
	fmt.Println("[AUTO-HEAL] Restarting now.")
	os.Exit(1)
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
	fmt.Printf("[AUTO-HEAL] Enabled: will resync from %s (signer %s) if >%d StateRoot mismatches accumulate within 10 min.\n",
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
}

// primaryStatusResponse mirrors the subset of /api/status this check needs.
type primaryStatusResponse struct {
	Height int64 `json:"height"`
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
		ticker := time.NewTicker(chainDivergenceCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			remoteHeight, ok := fetchPrimaryHeight(primaryURL)
			if !ok {
				// Primary unreachable this cycle — not evidence of divergence.
				continue
			}
			localFinalized, _ := dag.state.GetFinalizedCheckpoint()
			compareHeight := localFinalized
			if remoteHeight < compareHeight {
				compareHeight = remoteHeight
			}
			compareHeight -= chainDivergenceSafetyMargin
			if compareHeight <= 0 {
				continue // chain too young to compare safely yet
			}
			localBlock := dag.GetBlockByHeight(compareHeight)
			if localBlock == nil {
				continue // not yet synced to this height locally — not evidence
			}
			remoteHash, ok := fetchPrimaryBlockHash(primaryURL, compareHeight)
			if !ok {
				continue
			}
			if remoteHash != localBlock.Hash {
				dag.triggerAutoResync(fmt.Sprintf(
					"chain hash mismatch at height %d against primary %s (ours=%s… theirs=%s…) — this node is on an isolated fork",
					compareHeight, primaryURL, localBlock.Hash[:min(16, len(localBlock.Hash))], remoteHash[:min(16, len(remoteHash))]))
			}
		}
	}()
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
