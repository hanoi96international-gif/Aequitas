package keeper

import (
	"database/sql"
	"fmt"
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
//   1st offense: 14-day validator suspension. NO balance loss.
//   2nd offense within secondOffenseWindowDays: 90-day suspension PLUS a
//               fixed 50 AEQ penalty (= secondOffensePenaltyAEQ, 5% of
//               the initial 1,000 AEQ grant that every human receives —
//               a fixed amount that every node computes identically, with
//               no timing-dependent balance percentage that could cause
//               StateRoot divergence between partially-synced nodes).
//               Penalty credited to the UBI pool, never destroyed
//               (consistent with demurrage/wealth-cap overflow handling).
//   3rd+ offense: permanent validator ban. Balance and UBI rights are
//               NEVER touched beyond the 2nd-offense penalty.
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
	dag.pruneEquivocationIndexIfNeeded()
	return nil, false
}

// equivocationIndexPruneThreshold caps how large dag.equivocationIndex is
// allowed to grow before a prune pass runs (audit 2026-07-01, P3-1: the map
// previously had no cleanup at all and grew by one entry per unique
// (proposer, parent-set) pair for the lifetime of the chain).
const equivocationIndexPruneThreshold = 100_000

// equivocationIndexPruneBatch is how much dag.equivocationIndex must grow
// past dag.equivocationIndexNextPruneAt before another prune pass is
// attempted (see pruneEquivocationIndexIfNeeded's comment for why this
// ratchet exists).
const equivocationIndexPruneBatch = 10_000

// pruneEquivocationIndexIfNeeded drops index entries whose indexed block is
// at or below the hard-finalized checkpoint. Once a height is finalized it
// can never be reorganized, so any equivocation there would already have
// been caught (and slashed) by whichever node(s) saw both blocks live — an
// entry that has survived un-conflicted past finality no longer needs
// O(1)-lookup protection, and unlike a blind reset (the pattern used for
// warnedUnknownProposers) this never discards a still-reorganizable, still
// security-relevant entry. Must be called under dag.mu (same lock
// checkAndIndexEquivocation requires).
//
// FIX (self-review after audit 2026-07-01, P3-1): the first version of this
// function re-scanned the ENTIRE map on every single insert once past
// equivocationIndexPruneThreshold, forever — if finality is lagging (so most
// entries aren't prunable yet), every new block then paid a full O(n) scan
// while holding dag.mu, for every subsequent block, not just the one that
// crossed the threshold. dag.equivocationIndexNextPruneAt ratchets forward
// after each attempt (whether or not it freed much) so a scan only runs
// again once the map has grown by another full batch — bounding the
// amortized cost regardless of how effective any single pass is.
func (dag *BlockDAG) pruneEquivocationIndexIfNeeded() {
	if dag.state == nil {
		return
	}
	n := len(dag.equivocationIndex)
	if n <= equivocationIndexPruneThreshold {
		dag.equivocationIndexNextPruneAt = 0 // reset the ratchet once back under threshold
		return
	}
	if dag.equivocationIndexNextPruneAt != 0 && n < dag.equivocationIndexNextPruneAt {
		return
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	for key, hash := range dag.equivocationIndex {
		if b, found := dag.blocks[hash]; !found || b.Height <= finalizedHeight {
			delete(dag.equivocationIndex, key)
		}
	}
	dag.equivocationIndexNextPruneAt = len(dag.equivocationIndex) + equivocationIndexPruneBatch
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
func (cs *ChainState) IsValidatorSuspended(addr string, blockTimestamp int64) (suspended bool, reason string) {
	if cs.db == nil {
		return false, ""
	}
	addrLower := strings.ToLower(addr)
	var banned bool
	var suspendedUntil, lastOffenseAt int64
	err := cs.db.QueryRow(
		`SELECT banned, suspended_until, last_offense_at FROM validator_penalties WHERE signing_address = $1`,
		addrLower,
	).Scan(&banned, &suspendedUntil, &lastOffenseAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// Genuine "no penalty record" — the common case, most
			// validators were never slashed.
			return false, ""
		}
		// Audit 2026-07-01, P2-1 (re-checked): a transient DB error must not
		// silently fail open (that would let an actually-suspended/banned
		// validator's blocks through). But this function is called for
		// EVERY block's proposer, so failing closed unconditionally on the
		// FIRST error turns a brief, isolated hiccup (e.g. lock contention
		// on this one table from a concurrent RecordEquivocationAndSuspend
		// write) into a full chain-wide liveness stall — every validator's
		// blocks get rejected for as long as the blip lasts, a much larger
		// blast radius than the original "fail open" bug. One immediate,
		// no-sleep retry (this runs under dag.mu, so no sleep) absorbs a
		// single transient blip; only a SECOND consecutive failure fails
		// closed, which by then more confidently indicates a real, not
		// fluky, problem.
		err2 := cs.db.QueryRow(
			`SELECT banned, suspended_until, last_offense_at FROM validator_penalties WHERE signing_address = $1`,
			addrLower,
		).Scan(&banned, &suspendedUntil, &lastOffenseAt)
		if err2 != nil {
			if err2 == sql.ErrNoRows {
				return false, ""
			}
			return true, fmt.Sprintf("could not verify suspension status after retry: %v", err2)
		}
	}
	// Historical block predates the ban — allow it during catch-up sync.
	if blockTimestamp > 0 && lastOffenseAt > 0 && blockTimestamp < lastOffenseAt {
		return false, ""
	}
	if banned {
		return true, "permanently banned for repeated equivocation"
	}
	if suspendedUntil > time.Now().Unix() {
		return true, fmt.Sprintf("suspended for equivocation until %s",
			time.Unix(suspendedUntil, 0).UTC().Format("2006-01-02 15:04 UTC"))
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

// MaybeQueueSlashOutboxTx creates a "slash_equivocation" pending TX that will
// deduct penaltyAEQ from walletAddr → UBI pool when replayed by all nodes.
// The balance deduction itself does NOT happen here — it happens exclusively
// in replayTransactions' "slash_equivocation" case, which is idempotent via
// the equivocation_evidence.slash_applied flag. This ensures every node
// applies the penalty exactly once, regardless of how many nodes independently
// detect the same equivocation and queue competing slash TXs.
//
// blockAHash and blockBHash identify the equivocation evidence pair; they are
// stored in the TX so the replay can CAS-update equivocation_evidence.
func (cs *ChainState) MaybeQueueSlashOutboxTx(signingAddr, walletAddr, blockAHash, blockBHash string, penaltyAEQ float64) error {
	if cs.db == nil {
		return nil
	}
	if blockAHash > blockBHash {
		blockAHash, blockBHash = blockBHash, blockAHash
	}
	return savePendingTxExec(cs.db, Transaction{
		Type:       "slash_equivocation",
		Wallet:     strings.ToLower(signingAddr),
		To:         strings.ToLower(walletAddr),
		Amount:     penaltyAEQ,
		BlockAHash: blockAHash,
		BlockBHash: blockBHash,
	})
}
