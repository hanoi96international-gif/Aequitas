package keeper

import (
	"fmt"
	"os"
	"time"
)

// Divergence auto-heal: a secondary node that forks from the primary and racks
// up sustained StateRoot mismatches recovers itself, instead of sitting
// divergent until an operator manually sets RESYNC_FROM_SNAPSHOT=true and
// restarts it (exactly the manual intervention this codebase needed when the
// Contabo VPS diverged — 3000+ mismatches, 2 humans instead of 4).
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
	autoHealCooldown          = 30 * time.Minute
	autoHealCheckInterval     = 60 * time.Second
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

// StartDivergenceAutoHeal launches the background monitor described above.
// No-op unless AUTO_HEAL_ON_DIVERGENCE=true AND a signed snapshot source is
// configured (bootstrapURL+signer) — the primary has neither, so it can never
// wipe itself, and a misconfigured node never self-wipes blind.
//
// On sustained divergence it flags a resync and exits; it does NOT resync live,
// because ResyncFromSnapshotURL assumes it runs before block production and
// does not reset the in-memory DAG. The container restart policy brings the
// node back, main.go sees the flag, and the safe boot-time resync path runs.
// The resync is signature-verified against BOOTSTRAP_SIGNER, so even a false
// trigger only re-mirrors the trusted primary — the state a secondary should
// have anyway — never arbitrary data. The cooldown makes a resync that fails to
// converge fall back to running divergent rather than a restart loop.
func (dag *BlockDAG) StartDivergenceAutoHeal(bootstrapURL, signer string) {
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
			// Cooldown: never trigger twice within autoHealCooldown, so a resync
			// that doesn't fix the divergence can't become a restart loop.
			var lastAt int64
			fmt.Sscan(dag.state.getConfigValueDB(autoResyncLastAtKey), &lastAt)
			if lastAt > 0 && time.Now().Unix()-lastAt < int64(autoHealCooldown.Seconds()) {
				continue
			}
			fmt.Printf("[AUTO-HEAL] ⚠ %d StateRoot mismatches in the last 10 minutes — this node has diverged from its peers. Flagging an authoritative resync from %s and restarting so the safe boot-time resync path can run.\n",
				dag.TotalStateRootMismatches(), bootstrapURL)
			if err := dag.state.setConfigValueDB(autoResyncPendingKey, "1"); err != nil {
				fmt.Printf("[AUTO-HEAL] ✗ Could not persist the resync flag (%v) — staying up rather than restarting into an unflagged loop.\n", err)
				continue
			}
			_ = dag.state.setConfigValueDB(autoResyncLastAtKey, fmt.Sprintf("%d", time.Now().Unix()))
			fmt.Println("[AUTO-HEAL] Restarting now; the container restart policy will bring this node back to resync from the primary snapshot.")
			os.Exit(1)
		}
	}()
}
