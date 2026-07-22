package keeper

// Guardian system + inactive-wallet escrow implementation.
//
// A Guardian is a trusted registered human who can confirm another human is
// still alive, preventing their AEQ from moving to escrow due to inactivity.
//
// Timeline (from Whitepaper):
//   Year 0–2    Normal use
//   Year 2      Warning 1 (guardian can respond)
//   Year 2+60d  Warning 2
//   Year 2+120d Warning 3
//   Year 2+180d Balance → ESCROW (recoverable by owner)
//   Year 4      If still inactive → escrow released to UBI pool
//
// The "2 years + 180 days" threshold before escrow = 2.5 years = 913 days.
// The "still inactive at 4 years" threshold from registration means from the
// moment funds entered escrow (2.5 yr mark) there is another 1.5 years before
// the UBI release. We track moved_at in escrow_accounts for this.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// inactivityEscrowSeconds is the inactivity threshold before funds move to escrow.
// 2 years + 180 days = 2.5 years ≈ 913 days.
const inactivityEscrowSeconds = int64((2*365 + 180) * 24 * 60 * 60)

// escrowToUBISeconds is how long funds sit in escrow before moving to the UBI
// pool if the owner stays inactive. 1.5 years ≈ 548 days.
const escrowToUBISeconds = int64((365 + 183) * 24 * 60 * 60)

// guardianTimelockSeconds is the 7-day timelock preventing guardian changes.
const guardianTimelockSeconds = int64(7 * 24 * 60 * 60)

// maxWardsPerGuardian limits how many wards a single guardian may have.
const maxWardsPerGuardian = 3

// ─── DB SCHEMA ────────────────────────────────────────────────────────────────

// InitGuardianTables creates the guardians and escrow_accounts tables if they
// don't already exist. Called from initDB so they're always present.
func (cs *ChainState) InitGuardianTables() error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.db.Exec(`CREATE TABLE IF NOT EXISTS guardians (
		wallet_address  TEXT PRIMARY KEY,
		guardian_address TEXT NOT NULL,
		set_at          BIGINT NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to create guardians table: %w", err)
	}
	if _, err := cs.db.Exec(`CREATE TABLE IF NOT EXISTS escrow_accounts (
		wallet_address TEXT PRIMARY KEY,
		amount         NUMERIC NOT NULL,
		moved_at       BIGINT NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to create escrow_accounts table: %w", err)
	}
	return nil
}

// ─── GUARDIAN STATE METHODS ───────────────────────────────────────────────────

// SetGuardian persists a guardian relationship. Validates:
//   - both wallet and guardian are registered humans
//   - no circular relationship
//   - guardian does not already have maxWardsPerGuardian wards
//   - 7-day timelock since last guardian change for this wallet
//
// The caller (API handler) is responsible for signature verification.
func (cs *ChainState) SetGuardian(wallet, guardian string) error {
	// Always use server time for the timelock — ignoring the caller-supplied
	// timestamp prevents a caller from passing a future value to bypass the lock.
	setAt := time.Now().Unix()
	wallet = strings.ToLower(wallet)
	guardian = strings.ToLower(guardian)

	if wallet == guardian {
		return fmt.Errorf("wallet cannot be its own guardian")
	}

	// Both must be registered humans.
	if !cs.IsHuman(wallet) {
		return fmt.Errorf("wallet %s is not a registered human", wallet)
	}
	if !cs.IsHuman(guardian) {
		return fmt.Errorf("guardian %s is not a registered human", guardian)
	}

	if cs.db == nil {
		return fmt.Errorf("guardian operations require a database — run with DATABASE_URL set")
	}

	// Anti-circular: A cannot be guardian of B if B is guardian of A.
	// Check outside the transaction (read-only, no TOCTOU risk for this check).
	var guardianOfGuardian string
	scanErr := cs.db.QueryRow(
		`SELECT guardian_address FROM guardians WHERE lower(wallet_address) = $1`, guardian,
	).Scan(&guardianOfGuardian)
	if scanErr == nil && strings.ToLower(guardianOfGuardian) == wallet {
		return fmt.Errorf("circular guardian relationship: %s is already guardian of %s", wallet, guardian)
	}

	// Max 3 wards per guardian (pre-check; re-checked under lock in confirmGuardian).
	var wardCount int
	cs.db.QueryRow(
		`SELECT COUNT(*) FROM guardians WHERE lower(guardian_address) = lower($1) AND lower(wallet_address) != lower($2)`,
		guardian, wallet,
	).Scan(&wardCount)
	if wardCount >= maxWardsPerGuardian {
		return fmt.Errorf("guardian %s already has %d wards (maximum %d)", guardian, wardCount, maxWardsPerGuardian)
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin guardian transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// P2-10 (audit): acquire an advisory lock on the guardian address before the
	// ward-count re-check. Two concurrent requests for different wards of the same
	// guardian can both pass the COUNT(*) check and then both INSERT, exceeding
	// maxWardsPerGuardian. pg_advisory_xact_lock is transaction-scoped (released
	// automatically on commit/rollback) and serializes by guardian address hash.
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(abs(hashtext($1)))`, strings.ToLower(guardian)); err != nil {
		return fmt.Errorf("could not acquire guardian advisory lock: %w", err)
	}

	var existingSetAt int64
	rowErr := tx.QueryRow(
		`SELECT COALESCE(set_at, 0) FROM guardians WHERE lower(wallet_address) = $1 FOR UPDATE`,
		wallet,
	).Scan(&existingSetAt)
	if rowErr != nil && rowErr != sql.ErrNoRows {
		return fmt.Errorf("guardian lookup failed: %w", rowErr)
	}

	if existingSetAt > 0 && setAt-existingSetAt < guardianTimelockSeconds {
		// Round UP (ceiling division): flooring understated the wait — e.g. 6
		// days 23 hours remaining showed as "6 days remaining" even though a
		// retry exactly 6 days later would still fail the timelock check.
		remaining := guardianTimelockSeconds - (setAt - existingSetAt)
		daysLeft := (remaining + 86399) / 86400
		return fmt.Errorf("guardian was set %d days ago — must wait 7 days before changing (%d days remaining)",
			(setAt-existingSetAt)/86400, daysLeft)
	}

	// Re-check ward count now that we hold the advisory lock — serializes with
	// any concurrent guardian change for the same guardian address.
	// FIX (P2, beta-launch audit 2026-07-05): this Scan's error used to be
	// discarded (//nolint:errcheck). A transient query failure (connection
	// blip, statement timeout) inside this already-open, already-locked
	// transaction left currentWards at Go's zero value, silently treating a
	// guardian who may already be at the maxWardsPerGuardian cap as having
	// zero wards — bypassing the exact re-check this code exists to enforce.
	// Fail closed instead: a re-check that can't run is not a passed check.
	var currentWards int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM guardians WHERE lower(guardian_address) = lower($1) AND lower(wallet_address) != lower($2)`,
		guardian, wallet,
	).Scan(&currentWards); err != nil {
		return fmt.Errorf("ward-count re-check failed: %w", err)
	}
	if currentWards >= maxWardsPerGuardian {
		return fmt.Errorf("guardian %s already has %d wards (maximum %d) — re-check under lock", guardian, currentWards, maxWardsPerGuardian)
	}

	// FIX 1: Anti-cycle depth-limited walk. The simple A→B/B→A check above only
	// catches 2-node cycles. Walk up to 5 levels of the guardian chain starting
	// from `guardian` to detect 3+ node cycles (A→B→C→A etc.) before inserting.
	current := guardian
	for depth := 0; depth < 5; depth++ {
		var nextGuardian string
		err := tx.QueryRow(
			`SELECT guardian_address FROM guardians WHERE lower(wallet_address) = lower($1)`, current,
		).Scan(&nextGuardian)
		if err != nil {
			break // no guardian for `current` — end of chain, safe to insert
		}
		if strings.EqualFold(nextGuardian, wallet) {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("circular guardian relationship detected (cycle depth %d)", depth+2)
		}
		current = nextGuardian
	}

	if _, execErr := tx.Exec(
		`INSERT INTO guardians (wallet_address, guardian_address, set_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (wallet_address) DO UPDATE
		   SET guardian_address = $2, set_at = $3`,
		wallet, guardian, setAt,
	); execErr != nil {
		return fmt.Errorf("db error: %w", execErr)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("guardian commit failed: %w", err)
	}

	// FIX (P2-5, beta-launch audit 2026-07-05): keep the EVM mirror's
	// guardianOf slot current — see syncGuardianEscrowSlotsLocked's comment
	// for why this was previously only ever written at deploy time.
	cs.SyncGuardianEscrowSlots(V7_CONTRACT_ADDR, wallet)

	fmt.Printf("[GUARDIAN] ✓ %s set guardian to %s\n", wallet, guardian)
	return nil
}

// GetGuardian returns the guardian address and the timestamp it was set, or
// ("", 0, sql.ErrNoRows) if no guardian is configured for wallet.
func (cs *ChainState) GetGuardian(wallet string) (guardian string, setAt int64, err error) {
	wallet = strings.ToLower(wallet)
	if cs.db == nil {
		return "", 0, fmt.Errorf("no database")
	}
	row := cs.db.QueryRow(
		`SELECT guardian_address, set_at FROM guardians WHERE lower(wallet_address) = $1`, wallet,
	)
	err = row.Scan(&guardian, &setAt)
	return guardian, setAt, err
}

// ConfirmAlive resets wallet's last_activity_at to now. The caller must be the
// guardian of wallet; the API handler verifies this before calling here.
// The guardian has ZERO financial access — this only resets the inactivity timer.
//
// expectedGuardian is the guardian address the API handler looked up before
// acquiring this lock. We re-fetch and compare inside the lock to close the
// TOCTOU window where a concurrent SetGuardian could swap the guardian between
// the API handler's DB lookup and this update.
func (cs *ChainState) ConfirmAlive(wallet, expectedGuardian string) error {
	wallet = strings.ToLower(wallet)
	expectedGuardian = strings.ToLower(expectedGuardian)

	// Re-verify guardian OUTSIDE the lock to avoid stalling chain ops.
	// A concurrent SetGuardian could still swap the guardian after this check
	// but before the lock below — that is acceptable: the window is tiny and
	// the worst outcome is a spurious "mismatch" error (the caller retries).
	// Holding the write lock for a DB query would block ALL chain operations
	// (Transfer, RegisterHuman, UBI distribution) for the full round-trip time.
	if cs.db != nil && expectedGuardian != "" {
		var currentGuardian string
		dbErr := cs.db.QueryRow(
			`SELECT guardian_address FROM guardians WHERE lower(wallet_address) = $1`, wallet,
		).Scan(&currentGuardian)
		if dbErr != nil || strings.ToLower(currentGuardian) != expectedGuardian {
			return fmt.Errorf("guardian mismatch — concurrent change detected, retry")
		}
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("account %s not found", wallet)
	}
	// FIX 3: Only registered humans participate in the guardian / inactivity system.
	// Non-human accounts have no inactivity clock, so resetting it is a no-op at
	// best and a logic error at worst (could mask a misconfigured guardian entry).
	if !acc.IsHuman {
		return fmt.Errorf("account %s is not a registered human — guardian confirm-alive not applicable", wallet)
	}
	touchActivity(acc)
	if err := cs.saveAccountToDB(acc); err != nil {
		return fmt.Errorf("could not persist activity timer reset for %s: %w", wallet, err)
	}
	fmt.Printf("[GUARDIAN] ✓ Guardian confirmed %s is alive — activity timer reset\n", wallet)
	return nil
}

// GetEscrow returns the escrow amount and moved_at timestamp for wallet, or
// (0, 0, nil) if no escrow entry exists.
func (cs *ChainState) GetEscrow(wallet string) (amount float64, movedAt int64, err error) {
	wallet = strings.ToLower(wallet)
	if cs.db == nil {
		return 0, 0, nil
	}
	row := cs.db.QueryRow(
		`SELECT amount, moved_at FROM escrow_accounts WHERE wallet_address = $1`, wallet,
	)
	err = row.Scan(&amount, &movedAt)
	if err == sql.ErrNoRows {
		return 0, 0, nil // no escrow entry — not an error
	} else if err != nil {
		return 0, 0, fmt.Errorf("escrow lookup failed: %w", err)
	}
	return amount, movedAt, nil
}

// RecoverFromEscrow lets a wallet owner reclaim their escrowed balance.
// The caller (API handler) is responsible for signature verification.
// Wrapped in runAtomicWithOutbox so the escrow DELETE, balance credit, and
// outbox TX are a single all-or-nothing DB transaction: secondary nodes
// replay this as an "escrow_recover" TX via applyEscrowRecoverDeltaLocked.
func (cs *ChainState) RecoverFromEscrow(wallet string) error {
	wallet = strings.ToLower(wallet)
	if cs.db == nil {
		return fmt.Errorf("no database")
	}
	return cs.runAtomicWithOutbox([]string{wallet}, false, func() (Transaction, error) {
		// See processTransferBatch's own comment for why capturing
		// cs.activeTx into ctx here (cs.mu held throughout) is safe.
		ctx := withTx(context.Background(), cs.activeTx)
		// DELETE...RETURNING inside the active DB transaction — atomically
		// claims the escrow row while joining the same commit/rollback unit
		// as the subsequent balance credit and outbox insert.
		var amount float64
		err := cs.dbExecCtx(ctx).QueryRow(
			`DELETE FROM escrow_accounts WHERE wallet_address = $1 RETURNING amount`,
			wallet,
		).Scan(&amount)
		if err == sql.ErrNoRows {
			return Transaction{}, fmt.Errorf("no escrow found for wallet %s", wallet)
		}
		if err != nil {
			return Transaction{}, fmt.Errorf("escrow retrieval failed: %w", err)
		}
		if amount <= 0 {
			return Transaction{}, fmt.Errorf("escrow amount is zero")
		}

		// Credit balance back. If the account was lost from memory recreate
		// it as a human — escrow only exists for registered humans.
		if _, ok := cs.accounts.Get(wallet); !ok {
			cs.accounts.Set(wallet, &AccountState{Address: wallet, IsHuman: true})
		}
		acc, _ := cs.accounts.Get(wallet)
		if _, err := cs.settleDemurrageLockedCtx(ctx, acc); err != nil {
			return Transaction{}, fmt.Errorf("could not settle demurrage for %s: %w", wallet, err)
		}
		acc.Balance = acc.Balance.Add(NewDecimal(amount))
		touchActivity(acc)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return Transaction{}, fmt.Errorf("could not enforce wealth cap for %s: %w", wallet, err)
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return Transaction{}, fmt.Errorf("could not persist recovered balance for %s: %w", wallet, err)
		}
		// FIX (P2-5, beta-launch audit 2026-07-05): cs.mu is already held
		// here (runAtomicWithOutbox's callback) — see syncGuardianEscrowSlotsLocked's
		// comment for why this keeps the EVM mirror's escrowOf slot current.
		cs.syncGuardianEscrowSlotsLockedCtx(ctx, V7_CONTRACT_ADDR, wallet)
		fmt.Printf("[ESCROW] ✓ %s recovered %.6f AEQ from escrow\n", wallet, amount)
		return Transaction{
			Type:   "escrow_recover",
			Wallet: wallet,
			Amount: amount,
		}, nil
	})
}

// ─── DAILY SCHEDULER METHODS ─────────────────────────────────────────────────

// CheckAndMoveToEscrow is called once per day. It finds every wallet whose
// last_activity_at is older than inactivityEscrowSeconds and moves their AEQ
// balance to the escrow_accounts table. The balance is removed from the
// wallet immediately but remains recoverable by the owner for up to 1.5 more
// years (see ReleaseEscrowToUBI). Returns exactly what was moved for each
// wallet (balance zeroed + demurrage settled at the same moment) so the
// caller can build "escrow_move" TXs for secondaries to replay — secondaries
// never run this function themselves (see main.go's primary-only gate), so
// they only need the resulting balance delta, not the escrow_accounts row.
//
// FIX (audit3, P0 #3): this used to run its own multi-phase RLock/Lock
// cycling specifically so its (potentially slow) escrow_accounts DB calls
// never held cs.mu — a deliberate latency tradeoff for a function that used
// to run as an independent, non-atomic step. It is now ALWAYS called from
// inside RunDailyDistributionAtomic's single locked+transactional scope (see
// that function), so cs.mu is already held for the whole call and there is
// no separate lock-juggling left to do. The escrow_accounts INSERT now goes
// through cs.dbExec() instead of cs.db directly, so it commits or rolls back
// as part of the SAME DB transaction as the balance zeroing and every other
// distribution sub-step — replacing the old ad-hoc manual revert-on-failure
// (which could only restore the in-memory balance, never undo an escrow
// half-write) with a real, whole-round rollback via the caller.
func (cs *ChainState) checkAndMoveToEscrowLocked(ctx context.Context) ([]DistributionShare, error) {
	if cs.db == nil {
		return nil, nil
	}
	// FIX (2026-07-07): every other pool-touching operation (swapLocked,
	// addLiquidityDeltaLocked, removeLiquidityDeltaLocked) reloads cs.pool
	// from the DB before computing AMM math, specifically to avoid the
	// stale-in-memory-pool class of bug the P2-7 audit fixed for swaps (see
	// reloadPoolFromDB's own comment). liquidateLPSharesForEscrowLocked/
	// convertTUsdForEscrowLocked below use cs.pool directly the same way
	// those do, but this loop never reloaded it first — the one pool-mutating
	// path in the codebase that didn't. Harmless if cs.pool was already
	// current, closes the gap if it wasn't.
	cs.reloadPoolFromDB()
	threshold := time.Now().Unix() - inactivityEscrowSeconds
	now := time.Now().Unix()

	type escrowEntry struct {
		acc            AccountState
		balance        float64
		demurrageLost  float64
		lpSharesBurned float64
		tusdConverted  float64
		inactiveSince  string
	}
	// FIX (P3, beta-launch audit 2026-07-05): this used to only iterate
	// cs.accounts (the in-memory cache) — a genuinely-inactive human whose
	// account had been evicted (or never loaded) beyond maxInMemAccounts was
	// silently never considered for escrow. Enumerate candidates from the DB
	// instead, same fix already applied to distributeUBIPoolLocked/
	// distributeLPPoolLocked — see idx_chain_accounts_is_human_activity.
	// FIX (P2-1, beta-launch audit 2026-07-05): use cs.dbExec(), not cs.db
	// directly — this function always runs inside RunDailyDistributionAtomic's
	// active transaction (see this function's own top-of-file comment), and
	// every other DB call in this function already goes through cs.dbExec()
	// for exactly that reason. Reading via cs.db bypassed the active
	// transaction's isolation for this one query — harmless today only
	// because cs.mu is also held for the whole call, but a future lock
	// refactor that relaxed cs.mu without noticing this one inconsistent
	// call site would silently reintroduce a real isolation gap.
	var candidateAddrs []string
	rows, err := cs.dbExecCtx(ctx).Query(`SELECT lower(address) FROM chain_accounts WHERE is_human = true AND last_activity_at > 0`)
	if err != nil {
		return nil, fmt.Errorf("could not enumerate human accounts for escrow check: %w", err)
	}
	for rows.Next() {
		var addr string
		rows.Scan(&addr)
		if addr != "" {
			candidateAddrs = append(candidateAddrs, addr)
		}
	}
	rows.Close()
	cs.ensureAccountsLoadedCtx(ctx, candidateAddrs) // page in cold accounts so escrow sweeps work beyond the in-memory cap

	var toEscrow []escrowEntry
	for _, addr := range candidateAddrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok || !acc.IsHuman {
			continue
		}
		if acc.LastActivityAt == 0 || acc.LastActivityAt > threshold {
			continue
		}
		// FIX (P2, beta-launch audit 2026-07-05): this used to gate purely on
		// effectiveBalance() (liquid AEQ), so an inactive human who had moved
		// their entire AEQ into LP shares or tUSD — balance 0, but real wealth
		// nonzero — was silently skipped forever: never escrowed, never
		// recycled to the UBI pool, permanently opting them out of the
		// inactivity-recovery policy this whole mechanism exists to guarantee.
		// Consider any form of wealth, not just liquid Balance.
		hasWealth := effectiveBalance(acc).Float() > 0 || acc.LPShares.Float() > 0 || acc.TUsdBalance.Float() > 0
		if !hasWealth {
			continue
		}
		var existing float64
		if scanErr := cs.dbExecCtx(ctx).QueryRow(
			`SELECT amount FROM escrow_accounts WHERE wallet_address = $1`, addr,
		).Scan(&existing); scanErr == nil && existing > 0 {
			continue // already escrowed
		}

		// Settle demurrage on the pre-existing liquid Balance first, before
		// any liquidation below adds newly-converted funds to it — otherwise
		// funds that were never idle AEQ (they were LP shares/tUSD) would get
		// retroactively decayed as if they had been.
		lost, err := cs.settleDemurrageLockedCtx(ctx, acc)
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", addr, err)
		}

		// Liquidate LP shares and tUSD into Balance first, via the same shared
		// helpers applyEscrowMoveDeltaLocked uses to replay this exact result
		// on secondary nodes — see those functions' comments for why this
		// must be shared code, not two hand-written copies of the same math.
		var lpSharesBurned, tusdConverted float64
		if acc.LPShares.Float() > 0 {
			burned, outAEQ, outTUSD, lerr := cs.liquidateLPSharesForEscrowLocked(ctx, acc, acc.LPShares.Float())
			if lerr != nil {
				return nil, fmt.Errorf("could not liquidate LP shares for %s: %w", addr, lerr)
			}
			lpSharesBurned = burned
			if burned > 0 {
				fmt.Printf("[ESCROW] Liquidated %.6f LP shares → %.6f AEQ + %.6f tUSD for inactive %s\n", burned, outAEQ, outTUSD, addr)
			}
		}
		// Convert any tUSD holdings (pre-existing plus whatever the LP
		// liquidation above just credited) to AEQ too, so the ENTIRE wealth —
		// not just the liquid-AEQ portion — is captured by the escrow
		// mechanism below, which only understands Balance.
		if tusdBefore := acc.TUsdBalance.Float(); tusdBefore > 0 {
			outAEQ, ok, cerr := cs.convertTUsdForEscrowLocked(ctx, acc, tusdBefore)
			if cerr != nil {
				return nil, fmt.Errorf("could not convert tUSD for %s: %w", addr, cerr)
			}
			if ok {
				// convertTUsdForEscrowLocked fully consumes the requested
				// amount on success, so the input we passed IS what converted.
				tusdConverted = tusdBefore
				fmt.Printf("[ESCROW] Converted %.6f tUSD → %.6f AEQ for inactive %s\n", tusdBefore, outAEQ, addr)
			}
			// !ok: pool too shallow to absorb this without breaching its own
			// reserve — leave the tUSD as-is rather than corrupt the pool.
			// Retried on the next daily sweep once liquidity allows.
		}

		bal := round6(acc.Balance.Float())
		// FIX (P3): a liquidation above may have already mutated cs.pool (and
		// persisted it to DB) even if the resulting balance rounds to 0 AEQ
		// (a dust-sized LP/tUSD position) — in that case we must still record
		// this wallet below so a Transaction carrying LPSharesBurned/
		// TUsdConverted gets emitted for secondary nodes to replay the exact
		// same pool mutation. Only truly skipping (no pool mutation at all,
		// nothing to move) is safe to fall through here.
		if bal <= 0 && lpSharesBurned <= 0 && tusdConverted <= 0 {
			continue
		}

		inactiveSince := time.Unix(acc.LastActivityAt, 0).Format("2006-01-02")
		acc.Balance = NewDecimal(0)

		toEscrow = append(toEscrow, escrowEntry{
			acc:            *acc,
			balance:        bal,
			demurrageLost:  lost.Float(),
			lpSharesBurned: lpSharesBurned,
			tusdConverted:  tusdConverted,
			inactiveSince:  inactiveSince,
		})
	}

	if len(toEscrow) == 0 {
		return nil, nil
	}

	// FIX 2: Use ON CONFLICT DO NOTHING so an existing escrow row's moved_at
	// clock is never reset — resetting it would restart the 1.5-year countdown.
	var moved []DistributionShare
	for _, entry := range toEscrow {
		if _, err := cs.dbExecCtx(ctx).Exec(
			`INSERT INTO escrow_accounts (wallet_address, amount, moved_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (wallet_address) DO NOTHING`,
			entry.acc.Address, entry.balance, now,
		); err != nil {
			return nil, fmt.Errorf("could not write escrow for %s: %w", entry.acc.Address, err)
		}
		acc := entry.acc
		if err := cs.saveAccountToDBCtx(ctx, &acc); err != nil {
			return nil, fmt.Errorf("could not save escrowed account %s: %w", entry.acc.Address, err)
		}
		cs.accounts.Set(entry.acc.Address, &acc)
		// FIX (P2-5, beta-launch audit 2026-07-05): keep the EVM mirror's
		// escrowOf slot current — see syncGuardianEscrowSlotsLocked's comment.
		cs.syncGuardianEscrowSlotsLockedCtx(ctx, V7_CONTRACT_ADDR, entry.acc.Address)
		fmt.Printf("[ESCROW] ✓ Moved %.6f AEQ from %s to escrow (inactive since %s)\n",
			entry.balance, entry.acc.Address, entry.inactiveSince)
		moved = append(moved, DistributionShare{
			Wallet:         entry.acc.Address,
			Amount:         entry.balance,
			DemurrageLost:  entry.demurrageLost,
			LPSharesBurned: entry.lpSharesBurned,
			TUsdConverted:  entry.tusdConverted,
		})
	}
	return moved, nil
}

// ReleaseEscrowToUBI is called once per day. It finds escrow entries older
// than escrowToUBISeconds (1.5 years from when the funds were escrowed) and
// moves them into the UBI pool for distribution. Returns exactly what was
// released for each wallet so the caller can build "escrow_release" TXs for
// secondaries to replay — secondaries never run this function themselves
// (see main.go's primary-only gate), and don't need an escrow_accounts
// table at all since only the resulting UBI-pool credit affects StateRoot.
//
// FIX (audit3, P0 #3): now assumes cs.mu is already held by the caller
// (RunDailyDistributionAtomic) and uses cs.dbExec() so the escrow DELETE and
// the UBI-pool credit commit or roll back as part of the same DB transaction
// as the rest of the distribution round — see checkAndMoveToEscrowLocked's
// comment for the equivalent reasoning.
func (cs *ChainState) releaseEscrowToUBILocked(ctx context.Context) ([]DistributionShare, error) {
	if cs.db == nil {
		return nil, nil
	}
	threshold := time.Now().Unix() - escrowToUBISeconds

	// FIX 3: Use DELETE...RETURNING for an atomic claim — no other goroutine
	// can claim the same row between a SELECT and a subsequent DELETE.
	rows, err := cs.dbExecCtx(ctx).Query(
		`DELETE FROM escrow_accounts
		 WHERE moved_at < $1
		 RETURNING wallet_address, amount`,
		threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("could not release escrow: %w", err)
	}
	type escrowEntry struct {
		addr   string
		amount float64
	}
	var entries []escrowEntry
	for rows.Next() {
		var e escrowEntry
		if scanErr := rows.Scan(&e.addr, &e.amount); scanErr == nil && e.amount > 0 {
			entries = append(entries, e)
		}
	}
	rows.Close()

	if len(entries) == 0 {
		return nil, nil
	}

	// FIX (Monster Audit 2026-07-12, P1): a cold ubiPoolAddr used to get
	// recreated as a blank Version==0 AccountState the first time this loop
	// touched it, silently overwriting the pool's real accumulated DB balance
	// — see state.go's distributeSwapFee for the same pattern. Loaded once
	// here since it's the same map entry on every iteration below.
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	var released []DistributionShare
	for _, e := range entries {
		// Credit UBI pool.
		ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
		if !ok {
			ubiAcc = &AccountState{Address: ubiPoolAddr}
			cs.accounts.Set(ubiPoolAddr, ubiAcc)
		}
		ubiAcc.Balance = ubiAcc.Balance.Add(NewDecimal(e.amount))
		if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
			return nil, fmt.Errorf("could not save UBI pool: %w", err)
		}
		// FIX (P2-5, beta-launch audit 2026-07-05): the escrow_accounts row was
		// just deleted above — sync escrowOf back to 0 so the EVM mirror
		// doesn't keep showing the forfeited amount forever. See
		// syncGuardianEscrowSlotsLocked's comment.
		cs.syncGuardianEscrowSlotsLockedCtx(ctx, V7_CONTRACT_ADDR, e.addr)
		fmt.Printf("[ESCROW] ✓ Released %.6f AEQ from %s escrow → UBI pool\n", e.amount, e.addr)
		released = append(released, DistributionShare{Wallet: e.addr, Amount: round6(e.amount)})
	}

	cs.save()
	return released, nil
}

// applyEscrowRecoverDeltaLocked is the secondary-node replay handler for an
// "escrow_recover" TX produced by RecoverFromEscrow on the primary.  The
// secondary never had an escrow_accounts row (it only zeroed the balance via
// applyEscrowMoveDeltaLocked), so this just credits the amount back and
// resets the activity timer — no DELETE needed.  Caller must hold cs.mu.
func (cs *ChainState) applyEscrowRecoverDeltaLocked(wallet string, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("escrow_recover amount must be positive, got %.6f", amount)
	}
	// FIX (Monster Audit follow-up, 2026-07-12, P0): same cold-cache
	// blind-create pattern as applyValidatorRewardDeltaLocked et al. — a cold
	// wallet here used to be blind-created (IsHuman:true, everything else
	// zero), silently wiping any real balance/tusd/lp/last-activity it
	// already had. wallet is already lowercased by block.go's caller.
	cs.ensureAccountLoaded(wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		acc = &AccountState{Address: wallet, IsHuman: true}
		cs.accounts.Set(wallet, acc)
	}
	acc.Balance = acc.Balance.Add(NewDecimal(amount))
	touchActivity(acc)
	if err := cs.enforceWealthCapLocked(acc); err != nil {
		return fmt.Errorf("could not enforce wealth cap for %s: %w", wallet, err)
	}
	if err := cs.saveAccountToDB(acc); err != nil {
		return fmt.Errorf("could not persist escrow recovery for %s: %w", wallet, err)
	}
	return nil
}
