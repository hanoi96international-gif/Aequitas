package keeper

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ── EQUIVOCATION SLASHING ────────────────────────────────────────────────
//
// Equivocation = a validator signing two DIFFERENT blocks for the SAME
// parent set (what should have been a single, unique contribution). In a
// GHOSTDAG-based chain this is a real misbehavior: the protocol handles
// concurrent sibling blocks from DIFFERENT validators gracefully via
// merge-set/blue-red classification, but that mechanism was never designed
// to handle the same validator producing two conflicting alternatives for a
// single time slot — it wastes network resources, inflates merge sets, and
// degrades GHOSTDAG's blue-score accuracy.
//
// Graduated policy (confirmed with the project owner 2026-06-30):
//
//	1st offense: 14-day validator suspension. NO balance loss.
//	2nd offense within secondOffenseWindowDays: 90-day suspension PLUS a
//	            fixed 50 AEQ penalty (= secondOffensePenaltyAEQ, 5% of
//	            the initial 1,000 AEQ grant that every human receives —
//	            a fixed amount that every node computes identically, with
//	            no timing-dependent balance percentage that could cause
//	            StateRoot divergence between partially-synced nodes).
//	            Penalty credited to the UBI pool, never destroyed
//	            (consistent with demurrage/wealth-cap overflow handling).
//	3rd+ offense: permanent validator ban. Balance and UBI rights are
//	            NEVER touched beyond the 2nd-offense penalty.
//
// Rationale for the first-offense grace (no balance loss):
// GHOSTDAG's merge-set/blue-red classification already resolves concurrent
// blocks gracefully — a validator producing two conflicting blocks cannot
// by itself steal funds or rewrite settled history the way equivocation can
// in a classic BFT/PoS chain. Combined with the project's explicitly
// beginner-friendly target (non-technical operators on their own VPS, where
// accidentally running two node instances with the same key is a plausible
// honest mistake), a first-offense financial penalty would punish exactly
// the people this project exists to include. The real cost of a first
// offense (14 days without validator rewards) is a meaningful deterrent
// without being catastrophic.
const (
	equivocationFirstOffenseSuspensionDays  = 14
	equivocationSecondOffenseSuspensionDays = 90
	equivocationSecondOffenseWindowDays     = 90
	// secondOffensePenaltyAEQ: a fixed amount, not a percentage.
	// Using 5% of the 1,000 AEQ initial grant as a round, canonical value
	// avoids timing-sensitive percentage calculations (current balance
	// fluctuates with demurrage/UBI/transfers, so "5% of current balance"
	// would produce different numbers on nodes with slightly different sync
	// states, just like the wall-clock-based demurrage timing bug this
	// codebase already fixed). Every node independently computes 50 AEQ
	// from the same policy constant — no replay coordination needed.
	equivocationSecondOffensePenaltyAEQ = 50.0
	// equivocationSlashingActivationUnix (2026-07-05 00:00:00 UTC): offenses
	// evidenced by blocks whose timestamps predate this instant are indexed
	// for detection but NEVER penalized.
	//
	// Why an activation cutoff exists (confirmed live 2026-07-03): the chain's
	// history contains real same-parent-set duplicate blocks from BOTH honest
	// validators, produced during the late-June/early-July restart-and-resync
	// incident chaos — BEFORE this slashing feature was deployed (2026-06-30).
	// The nodes that were running back then never punished those events (no
	// detection code existed when the blocks arrived, and the startup index
	// rebuild in NewBlockchain deliberately only indexes). But a FRESH node
	// replaying the full history detects them on first contact and applies
	// suspensions anchored at those blocks' timestamps — suspensions that were
	// still active (until 2026-07-12 / 2026-09-25) when a new node synced on
	// 2026-07-03. Net effect without this cutoff: every new node that ever
	// joins the network ends up suspending ALL honest validators and can
	// never accept their live blocks — while long-running nodes hold no such
	// penalties. A slashing rule that only fresh nodes enforce is not a
	// consensus rule; it's a network-partition generator.
	//
	// The cutoff makes replay deterministic in BOTH directions: every node —
	// live back then, syncing today, or bootstrapping next year — computes
	// "no penalty" for pre-activation evidence and the identical
	// block-timestamp-anchored penalty for post-activation evidence.
	// The date is deliberately a day or two AFTER the fix's deploy date so
	// the tail of the same incident chaos (nodes still resyncing/catching up
	// on 2026-07-03/04) can't mint fresh honest-mistake offenses either.
	equivocationSlashingActivationUnix = 1783209600
)

// equivocationParentKey returns a deterministic, order-independent key for
// a block's parent set. ParentHashes order CAN legitimately differ between
// two honest blocks built on the same tip set (see calculateHash's own
// comment on sort-at-produce, not sort-at-verify) — detection must not
// treat different orderings of the SAME parents as different evidence.
func equivocationParentKey(parents []string) string {
	sorted := append([]string(nil), parents...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// checkAndIndexEquivocation records block in dag.equivocationIndex and
// returns a conflict (the earlier block from the same proposer referencing
// the exact same parent set with a different hash) if one exists. Call this
// every time a block is added to dag.blocks — it updates the index
// atomically so detection is always current. Must be called under dag.mu.
// Safe for all block types (genesis and synthetic-checkpoint stubs are
// excluded). Returns (nil, false) when there is no conflict.
func (dag *BlockDAG) checkAndIndexEquivocation(block *Block) (conflict *Block, isEquivocation bool) {
	if block.IsGenesis || block.Proposer == "" || block.Proposer == "synthetic-checkpoint" {
		return nil, false
	}
	key := strings.ToLower(block.Proposer) + "|" + equivocationParentKey(block.ParentHashes)
	if existingHash, ok := dag.equivocationIndex[key]; ok {
		if existingHash != block.Hash {
			if existing, found := dag.blocks[existingHash]; found {
				return existing, true
			}
		}
		return nil, false
	}
	dag.equivocationIndex[key] = block.Hash
	return nil, false
}

// initSlashingTables creates the tables equivocation slashing persists to.
// Safe to call repeatedly (CREATE TABLE IF NOT EXISTS / ALTER TABLE IF NOT EXISTS).
func (cs *ChainState) initSlashingTables() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS validator_penalties (
		signing_address TEXT PRIMARY KEY,
		offense_count   INT     NOT NULL DEFAULT 0,
		suspended_until BIGINT  NOT NULL DEFAULT 0,
		banned          BOOLEAN NOT NULL DEFAULT FALSE,
		last_offense_at BIGINT  NOT NULL DEFAULT 0
	)`)
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS equivocation_evidence (
		id              SERIAL  PRIMARY KEY,
		signing_address TEXT    NOT NULL,
		block_a_hash    TEXT    NOT NULL,
		block_b_hash    TEXT    NOT NULL,
		detected_at     BIGINT  NOT NULL,
		slash_applied   BOOLEAN NOT NULL DEFAULT FALSE,
		UNIQUE(block_a_hash, block_b_hash)
	)`)
	// Migration for nodes that already have the table without slash_applied.
	cs.db.Exec(`ALTER TABLE equivocation_evidence ADD COLUMN IF NOT EXISTS slash_applied BOOLEAN NOT NULL DEFAULT FALSE`)
	// Self-heal for nodes poisoned by pre-activation offenses (see
	// equivocationSlashingActivationUnix): any node that ran the pre-cutoff
	// code while replaying history recorded suspensions/bans against the
	// honest validators (confirmed live on the third node, 2026-07-03).
	// Deleting the penalty rows on startup un-suspends them without manual
	// SQL; the equivocation_evidence rows are deliberately KEPT as the
	// audit trail (same policy as synthetic_checkpoint_events) — evidence
	// alone never blocks anything, only validator_penalties rows do.
	if res, err := cs.db.Exec(
		`DELETE FROM validator_penalties WHERE last_offense_at < $1`,
		int64(equivocationSlashingActivationUnix),
	); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			fmt.Printf("[SLASHING] ✓ Cleared %d pre-activation validator penalty record(s) (offenses before 2026-07-05 UTC are exempt — see equivocationSlashingActivationUnix)\n", n)
			cs.invalidatePenaltyCache()
		}
	}
	cs.selfHealUncorroboratedSeedSuspension()
}

// selfHealUncorroboratedSeedSuspension clears a validator_penalties
// suspension against this node's own configured trusted bootstrap signer
// (BOOTSTRAP_SIGNER) when that suspension was never corroborated by the
// rest of the network. Confirmed-recurring false-positive pattern (Contabo1,
// 2026-07-10, 07-12, 07-17): RecordEquivocationAndSuspend applies a
// suspension on THIS node immediately, without waiting for network
// corroboration, on purpose (see its own comment — a node needs to protect
// itself right away, not wait on its own slow block production). That's
// correct for real attacks, but it means a conflict this node alone ever
// observed — e.g. two representations of what the rest of the network
// agrees was a single event, encountered while this node was deep in
// orphan/checkpoint-bridging catch-up (see the "[DAG] queued as orphan" /
// "bridged ... with a synthetic checkpoint" logging) — can suspend an
// honest, actively-producing validator with nothing else to correct it.
//
// The telltale sign every one of these incidents shared: offense_count
// reached 2 (the 90-day-suspension threshold) yet slash_applied never
// became true on ANY of that address's equivocation_evidence rows. A REAL,
// consensus-replicated 2nd offense always ends with slash_applied=true once
// this node's own queued slash_equivocation TX replays through its own
// chain (see the switch/case in RecordEquivocationAndSuspend and the
// slash_equivocation case in replayTransactions) — so "escalated to 2nd
// offense, zero applied penalties" is only possible when the underlying
// evidence never actually made it into this node's real canonical history.
//
// Scoped ONLY to BOOTSTRAP_SIGNER — the address this node's own operator
// already configured as its trusted bootstrap/snapshot source — so this
// cannot be used by an actually malicious THIRD validator to launder a real
// offense; it only ever un-suspends the one validator this node already
// implicitly trusts. Runs on every startup, same as the pre-activation
// self-heal above, so this specific pattern no longer needs a manual SSH +
// DB DELETE + container restart every time it recurs.
func (cs *ChainState) selfHealUncorroboratedSeedSuspension() {
	trustedSigner := trustedBootstrapSigner()
	if trustedSigner == "" {
		return
	}
	var offenseCount int
	if err := cs.db.QueryRow(
		`SELECT offense_count FROM validator_penalties WHERE signing_address = $1`,
		trustedSigner,
	).Scan(&offenseCount); err != nil || offenseCount < 2 {
		return // no row, or not yet escalated — nothing to self-heal
	}
	var hasApplied bool
	if err := cs.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM equivocation_evidence WHERE signing_address = $1 AND slash_applied = TRUE)`,
		trustedSigner,
	).Scan(&hasApplied); err != nil || hasApplied {
		return // a real applied penalty exists (or the check failed) — leave the suspension in place
	}
	if res, err := cs.db.Exec(`DELETE FROM validator_penalties WHERE signing_address = $1`, trustedSigner); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			fmt.Printf("[SLASHING] ✓ Self-healed an uncorroborated equivocation suspension against the trusted bootstrap signer %s (offense_count=%d reached but zero applied penalties ever corroborated it — see selfHealUncorroboratedSeedSuspension's own comment)\n", trustedSigner, offenseCount)
			cs.invalidatePenaltyCache()
		}
	}
}

// trustedBootstrapSigner returns this node's configured BOOTSTRAP_SIGNER,
// lowercased and trimmed, or "" if none is set. This is the one validator
// address the operator has explicitly declared as this node's trust anchor:
// the signer whose signed snapshot this node is willing to replace its
// ENTIRE account state from (see ResyncFromSnapshotURL / StartDivergenceAutoHeal).
func trustedBootstrapSigner() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_SIGNER")))
}

// loadPenaltyCacheLocked fills cs.penaltyCache from validator_penalties.
// Caller must hold cs.penaltyMu (write). The table is tiny (one row per
// ever-penalized validator — 0 rows in the healthy case), so a full load is
// a single cheap round trip paid once per process.
//
// OPERATIONAL NOTE (2026-07-12 incident): this cache is loaded once and only
// invalidated by this process's OWN code paths (invalidatePenaltyCache,
// called from e.g. the pre-activation-penalty cleanup above). A manual
// out-of-band correction to validator_penalties — e.g. `DELETE FROM
// validator_penalties WHERE ...` run directly via psql to clear a
// confirmed false-positive equivocation suspension (see block.go's
// postBootDuplicateGuardWindow comment for the incident this refers to) —
// does NOT reach this process at all, so IsValidatorSuspended keeps
// rejecting blocks against the stale cached row until the process is
// restarted. Confirmed live: a direct DB DELETE alone did nothing; only a
// subsequent `docker restart` (which reloads this cache fresh from the
// now-empty table) actually restored merging. If clearing a penalty
// manually again, a restart is not optional — it's the only way the change
// takes effect.
func (cs *ChainState) loadPenaltyCacheLocked() {
	cs.penaltyCache = make(map[string]validatorPenalty)
	cs.penaltyCacheLoaded = true // set even on query error: fail open, like the old per-call path did
	if cs.db == nil {
		return
	}
	// FIX (deadlock, concurrency audit 2026-07-21): cs.dbExec() instead of
	// cs.db — see state.go's ensureAccountLoaded FIX comment. Safe
	// unconditionally (falls back to cs.db when no transaction is active);
	// this closes the same hazard class for any call path that reaches
	// this cache load while cs.mu+cs.activeTx are already held.
	rows, err := cs.dbExec().Query(`SELECT signing_address, banned, suspended_until, last_offense_at FROM validator_penalties`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var addr string
		var p validatorPenalty
		if rows.Scan(&addr, &p.banned, &p.suspendedUntil, &p.lastOffenseAt) == nil {
			cs.penaltyCache[strings.ToLower(addr)] = p
		}
	}
}

// invalidatePenaltyCache forces the next IsValidatorSuspended call to reload
// from the DB. Called by every validator_penalties writer after a successful
// change (RecordEquivocationAndSuspend, initSlashingTables' activation
// cleanup) — writes are rare, so a full reload beats write-through
// bookkeeping for correctness-per-line.
func (cs *ChainState) invalidatePenaltyCache() {
	cs.penaltyMu.Lock()
	cs.penaltyCacheLoaded = false
	cs.penaltyCache = nil
	cs.penaltyMu.Unlock()
}

// IsValidatorSuspended reports whether addr is currently barred from
// producing blocks (a time-bounded suspension or a permanent ban).
// Checked by AddPeerBlock's proposer-authorization gate after confirming
// the cryptographic signature.
//
// blockTimestamp is the Unix timestamp of the block being validated. When
// non-zero, blocks produced BEFORE the ban was applied (blockTimestamp <
// last_offense_at) are allowed — this is the historical-sync case: a
// validator that was later slashed still legitimately signed those earlier
// blocks, and replaying them during catch-up must not be rejected.
//
// FIX (P0, cadence 2026-07-03 night): answered from the in-memory penalty
// cache (see penaltyMu's struct comment). This used to be a synchronous
// Postgres QueryRow — ~0.5s over the primary's remote DB proxy — executed
// while AddPeerBlock held dag.mu write-locked, ONCE PER non-FromSync peer
// block, including every block a later gate was about to reject anyway.
// Under a re-delivery flood of below-checkpoint blocks from
// isolated/diverged peers, those serialized round trips starved
// ProduceBlock's lock acquisition and inflated block cadence to 3-5s
// against the 1s target.
func (cs *ChainState) IsValidatorSuspended(addr string, blockTimestamp int64) (suspended bool, reason string) {
	if cs.db == nil {
		return false, ""
	}
	cs.penaltyMu.RLock()
	loaded := cs.penaltyCacheLoaded
	p, exists := cs.penaltyCache[strings.ToLower(addr)]
	cs.penaltyMu.RUnlock()
	if !loaded {
		cs.penaltyMu.Lock()
		if !cs.penaltyCacheLoaded { // another goroutine may have won the load race
			cs.loadPenaltyCacheLocked()
		}
		p, exists = cs.penaltyCache[strings.ToLower(addr)]
		cs.penaltyMu.Unlock()
	}
	if !exists {
		return false, "" // no penalty record: fail open (same as ErrNoRows before)
	}
	// Historical block predates the ban — allow it during catch-up sync.
	if blockTimestamp > 0 && p.lastOffenseAt > 0 && blockTimestamp < p.lastOffenseAt {
		return false, ""
	}
	if p.banned {
		return true, "permanently banned for repeated equivocation"
	}
	if p.suspendedUntil > time.Now().Unix() {
		return true, fmt.Sprintf("suspended for equivocation until %s",
			time.Unix(p.suspendedUntil, 0).UTC().Format("2006-01-02 15:04 UTC"))
	}
	return false, ""
}

// RecordEquivocationAndSuspend persists proof that signingAddress produced
// blockAHash and blockBHash for the same parent set, applies the graduated
// slashing policy, and returns the offense count plus the operator wallet
// address that should receive the financial penalty (empty string when no
// balance penalty is warranted — 1st offense or 2nd outside the window).
//
// UNIQUE constraint on (block_a_hash, block_b_hash) makes this idempotent:
// two nodes recording the same evidence pair produce one row, not two
// (ON CONFLICT DO NOTHING → rows=0 → early return with current count).
//
// The balance penalty for 2nd-offense is NOT applied here — the caller
// should call MaybeQueueSlashOutboxTx(pendingSlashWallet, ...) so that a
// "slash_equivocation" outbox TX goes into the next block and is replayed
// identically by every other node. This avoids the double-deduction race
// that would occur if both the detecting node and a TX-replaying node each
// deducted the balance independently.
func (cs *ChainState) RecordEquivocationAndSuspend(signingAddress, blockAHash, blockBHash string, now int64) (offenseCount int, pendingSlashWallet string, err error) {
	if cs.db == nil {
		return 0, "", fmt.Errorf("no database configured")
	}
	// Pre-activation evidence is exempt (see equivocationSlashingActivationUnix's
	// comment for the full rationale). The primary gate is at the detection call
	// site in AddPeerBlock (which skips the whole recording goroutine); this is
	// the backstop so no future caller can reintroduce retroactive penalties.
	if now < equivocationSlashingActivationUnix {
		return 0, "", nil
	}
	addr := strings.ToLower(signingAddress)
	// Canonical hash order so the same pair from two different nodes hits
	// the same UNIQUE (blockA, blockB) row regardless of detection order.
	if blockAHash > blockBHash {
		blockAHash, blockBHash = blockBHash, blockAHash
	}

	tx, txErr := cs.db.Begin()
	if txErr != nil {
		return 0, "", txErr
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	res, insErr := tx.Exec(
		`INSERT INTO equivocation_evidence
		    (signing_address, block_a_hash, block_b_hash, detected_at)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (block_a_hash, block_b_hash) DO NOTHING`,
		addr, blockAHash, blockBHash, now,
	)
	if insErr != nil {
		return 0, "", insErr
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		// Already recorded. Read current count and return.
		var count int
		tx.QueryRow(`SELECT offense_count FROM validator_penalties WHERE signing_address = $1`, addr).Scan(&count)
		tx.Commit()
		committed = true
		return count, "", nil
	}

	// Upsert penalty record, incrementing the offense counter.
	_, upsertErr := tx.Exec(`
		INSERT INTO validator_penalties
		    (signing_address, offense_count, suspended_until, banned, last_offense_at)
		VALUES ($1, 1, $2, FALSE, $3)
		ON CONFLICT (signing_address) DO UPDATE SET
		    offense_count   = validator_penalties.offense_count + 1,
		    last_offense_at = $3
		`,
		addr,
		now+equivocationFirstOffenseSuspensionDays*86400,
		now,
	)
	if upsertErr != nil {
		return 0, "", upsertErr
	}

	var count int
	var prevOffenseAt int64
	tx.QueryRow(
		`SELECT offense_count, last_offense_at FROM validator_penalties WHERE signing_address = $1`,
		addr,
	).Scan(&count, &prevOffenseAt)
	// prevOffenseAt was just set to `now` by the upsert above — we need
	// the PREVIOUS last_offense_at to decide whether we're within the
	// second-offense window. Re-read from equivocation_evidence instead:
	// the second-to-last detection_at for this address.
	var prevDetectedAt int64
	tx.QueryRow(
		`SELECT detected_at FROM equivocation_evidence
		 WHERE signing_address = $1 AND NOT (block_a_hash=$2 AND block_b_hash=$3)
		 ORDER BY detected_at DESC LIMIT 1`,
		addr, blockAHash, blockBHash,
	).Scan(&prevDetectedAt)
	withinWindow := prevDetectedAt > 0 && now-prevDetectedAt <= equivocationSecondOffenseWindowDays*86400

	switch {
	case count >= 3:
		tx.Exec(`UPDATE validator_penalties SET banned = TRUE WHERE signing_address = $1`, addr)
	case count == 2 && withinWindow:
		tx.Exec(
			`UPDATE validator_penalties SET suspended_until = $1 WHERE signing_address = $2`,
			now+equivocationSecondOffenseSuspensionDays*86400, addr,
		)
	default:
		// 1st offense, or a 2nd outside the window (treat as a fresh
		// 1st): the 14-day suspension was already set by the upsert above.
		if count >= 2 && !withinWindow {
			count = 1 // Reset counter — the prior offense is "stale"
			tx.Exec(
				`UPDATE validator_penalties SET offense_count = 1 WHERE signing_address = $1`, addr,
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	committed = true
	cs.invalidatePenaltyCache() // keep IsValidatorSuspended's cache current

	// Log every offense clearly so operators can investigate.
	var slashWallet string
	switch {
	case count >= 3:
		fmt.Printf("[SLASHING] ⛔ %s permanently banned after %d equivocation offense(s)\n", addr, count)
	case count == 2 && withinWindow:
		fmt.Printf("[SLASHING] ⚠ %s: 2nd equivocation within %d days — 90-day suspension; 50 AEQ penalty TX queued...\n",
			addr, equivocationSecondOffenseWindowDays)
		// Look up the operator wallet so the caller can queue the penalty TX.
		cs.db.QueryRow(
			`SELECT wallet_address FROM registered_nodes WHERE lower(signing_address) = $1`, addr,
		).Scan(&slashWallet)
		if slashWallet == "" {
			fmt.Printf("[SLASHING] ⚠ operator wallet not found for signer %s — penalty TX skipped\n", addr)
		}
	default:
		days := equivocationFirstOffenseSuspensionDays
		fmt.Printf("[SLASHING] ⚠ %s: 1st equivocation — %d-day suspension (no balance loss)\n", addr, days)
	}

	return count, slashWallet, nil
}

// QueueEquivocationEvidenceTx creates a "slash_equivocation" pending TX that
// carries ONLY the evidence (signer + conflicting block hashes + the
// original detection timestamp) — no wallet/penalty. Queued for EVERY
// offense (1st, 2nd, 3rd+), not just a 2nd-offense financial penalty.
//
// FIX (2026-07-07 — closes a node-local/consensus asymmetry): this used to
// be MaybeQueueSlashOutboxTx, called only when the offense already warranted
// a balance penalty, and only the balance deduction itself was replayed
// consensus-wide (replayTransactions' "slash_equivocation" case, idempotent
// via equivocation_evidence.slash_applied). The SUSPENSION decision
// (validator_penalties.suspended_until/banned, offense_count) was applied
// exclusively by whichever node's RecordEquivocationAndSuspend call detected
// the conflict locally — a node that never independently saw both
// conflicting blocks (e.g. one of them never reached it before the other was
// superseded/orphaned) never suspended the validator at all, silently
// diverging from a node that did. Now every offense is queued, and replay
// calls RecordEquivocationAndSuspend itself (same idempotent function, keyed
// on the same (block_a_hash, block_b_hash) UNIQUE constraint) so
// validator_penalties converges identically on every node that replays the
// TX, regardless of who detected it first.
func (cs *ChainState) QueueEquivocationEvidenceTx(signingAddr, blockAHash, blockBHash string, detectedAt int64) error {
	if cs.db == nil {
		return nil
	}
	if blockAHash > blockBHash {
		blockAHash, blockBHash = blockBHash, blockAHash
	}
	return savePendingTxExec(cs.db, Transaction{
		Type:       "slash_equivocation",
		Wallet:     strings.ToLower(signingAddr),
		BlockAHash: blockAHash,
		BlockBHash: blockBHash,
		DetectedAt: detectedAt,
	})
}
