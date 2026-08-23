package keeper

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
)

// Chunked UBI distribution — roadmap step 3, implemented as chunking rather
// than lazy claim. See docs/UBI_DISTRIBUTION_DESIGN.md for why.
//
// THE PROBLEM. distributeUBIPoolLocked holds cs.mu — the write lock, which
// blocks every transaction on the node — across enumerating every human
// account, loading all of them into memory, settling demurrage for all of
// them, crediting all of them, and one batch write of N rows. At 14 humans
// that is unmeasurable. At a million it is a multi-minute full stop, once a
// day, and main.go additionally wraps the whole thing in
// WithBlockProductionPaused.
//
// WHY NOT LAZY CLAIM, which is what the roadmap proposed. settleDemurrageLocked
// decays idle balances and pays 20% of that decay back into the UBI pool —
// the chain's anti-hoarding mechanism. Unclaimed UBI would sit outside an
// account balance and so would NOT decay, making "never claim" the profitable
// strategy and inverting the exact mechanism the design rests on. Every way
// out of that is a decision about economic rules (does unclaimed UBI decay,
// and to whom; does it expire; is the wealth cap applied at distribution or at
// claim time), not a decision about code.
//
// Chunking asks none of those questions. The distribution stays a push with
// identical semantics; it is only split into bounded pieces. Demurrage, the
// wealth cap and total-supply accounting behave exactly as they do today.
//
// WHAT MAKES IT DETERMINISTIC, which is the whole game for a consensus change:
//
//   - Addresses are ordered by the database, not by map iteration, so every
//     node walks the same accounts in the same order.
//   - The per-human share is computed ONCE when the epoch opens and persisted.
//     Every later chunk reads that stored value, so demurrage collected while
//     the epoch is still running cannot change what earlier chunks paid.
//   - Chunk boundaries come from the persisted cursor, never from wall-clock
//     timing or from how much work a node felt like doing.
//   - The pool is reduced by exactly what was paid out, rather than zeroed.
//     Demurrage that arrives mid-epoch therefore rolls into the next epoch
//     instead of silently vanishing or double-counting.
//
// Each chunk still emits one ubi_distribution transaction per wallet carrying
// the exact credited amount, so secondaries replay numbers rather than
// recomputing them — the same contract ApplyUBIDelta already relies on.

// ubiChunkSize bounds how many accounts one chunk credits. It trades lock hold
// time against how many blocks a distribution spans: at a million humans this
// is 200 chunks, roughly three minutes at BLOCK_TIME=1s — of a chain that
// keeps producing blocks throughout, instead of three minutes of one that does
// not.
// A var, not a const, purely so tests can exercise the multi-chunk path
// without creating 5,000 accounts. Production never reassigns it.
var ubiChunkSize int64 = 5000

// Config keys holding the open epoch. Absent means no epoch is in progress.
const (
	ubiEpochCursorKey = "ubi_epoch_cursor" // accounts credited so far
	ubiEpochShareKey  = "ubi_epoch_share"  // per-human amount, fixed at epoch open
	ubiEpochTotalKey  = "ubi_epoch_total"  // account count at epoch open
)

// ubiChunkingActivationHeight is the block height at or above which chunked
// distribution replaces the single-shot path.
//
// NOT a consensus gate, despite the name and despite what the roadmap implies.
// Only ONE node distributes per round — main.go guards the whole thing with
// TryLockDistribution, whose own log line says "already ran within the last 24h
// on this or another node". Every other node simply replays the resulting
// ubi_distribution transactions, which carry exact amounts. Chunking therefore
// changes how the distributing node PACES its own work, not what any other node
// computes; two nodes could even run different chunk sizes without diverging.
//
// It is a rollout switch: zero means the existing single-shot path runs
// untouched, which is why that path was left in place rather than replaced. Set
// UBI_CHUNKING_ACTIVATION_HEIGHT once the behaviour has been observed on a node
// that is not serving users.
var ubiChunkingActivationHeight = int64FromEnv("UBI_CHUNKING_ACTIVATION_HEIGHT", 0)

func int64FromEnv(key string, def int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		fmt.Printf("[UBI] ⚠ %s=%q is not a non-negative integer — keeping the default of %d (chunking disabled)\n", key, raw, def)
		return def
	}
	return n
}

// UBIChunkingActive reports whether chunked distribution governs at height.
//
// IT ALWAYS RETURNS FALSE, and that is not an oversight.
//
// Chunking as built here cannot be switched on without diverging the network,
// for a reason outside this file. Every distribution round emits a
// distribution_round_marker carrying the round's timestamp, and
// distributionRoundToSkip (block.go) discards an ENTIRE round whose marker
// falls within 24h of the last one already applied — the guard added on
// 2026-08-16 after a proven double-credit.
//
// Chunks share one round timestamp. So the first chunk sets
// last_distribution_round_at, and every LATER chunk is then seen by every
// secondary as a duplicate of it and dropped whole. Measured, not reasoned:
//
//	distributionRoundToSkip(
//	    []Transaction{{Type: "ubi_distribution", ...},
//	                  {Type: "distribution_round_marker", DistributionAt: T}},
//	    /*lastDistributionRoundAt=*/ T, 0)
//	 => T          (nonzero: skip the whole block's round)
//
// The distributing node would credit everyone across all chunks; every other
// node would credit only the first chunk. Permanent state divergence, silent,
// in the ledger.
//
// The pull request that added this file argued chunking "changes how the
// distributing node paces its own work, not what any other node computes".
// That holds for the COMPUTATION and fails for the REPLAY GUARD, which keys on
// the round marker and cannot tell two chunks of one round apart.
//
// WHAT IT WOULD TAKE: the idempotency anchor has to identify the chunk, not
// just the round — a cursor on the marker and a matching comparison in
// distributionRoundToSkip, so "already applied" means "this chunk", not "this
// day". That is a change to replay logic that moves money, and it belongs in
// its own piece of work with its own tests, not in the tail of this one.
//
// Until then the single-shot path stays in charge. Setting
// UBI_CHUNKING_ACTIVATION_HEIGHT says so out loud rather than doing nothing,
// which is what it did before: the chunked functions below were never called
// from anywhere, so the variable had no effect and no explanation.
func UBIChunkingActive(height int64) bool {
	if ubiChunkingActivationHeight > 0 {
		warnChunkingBlockedOnce.Do(func() {
			fmt.Printf("[UBI] ⚠ UBI_CHUNKING_ACTIVATION_HEIGHT=%d is set, and chunked "+
				"distribution stays OFF anyway. Chunks share one round timestamp, and "+
				"distributionRoundToSkip drops every chunk after the first as a duplicate "+
				"round — the distributing node would pay everyone, every other node only "+
				"the first chunk. See UBIChunkingActive's comment for what has to change "+
				"first.\n", ubiChunkingActivationHeight)
		})
	}
	return false
}

var warnChunkingBlockedOnce sync.Once

// ubiEpoch is the in-memory epoch, and the authoritative copy while a node is
// running. The chain_config rows exist so a restart mid-distribution resumes
// instead of paying everyone a second time; they are durability, not the
// source of truth.
//
// It lives on ChainState rather than in a package var because a test may build
// several states, and a shared one would leak an open epoch between them.
type ubiEpochHolder struct {
	state ubiEpochState
	set   bool
}

// ubiEpochState is the progress of a distribution in flight.
type ubiEpochState struct {
	cursor int64
	share  float64
	total  int64
}

// open reports whether an epoch is mid-flight.
func (e ubiEpochState) open() bool { return e.total > 0 && e.cursor < e.total }

// loadUBIEpochLocked reads the open epoch, if any. A partially written epoch
// (some keys present, others not) is treated as no epoch rather than guessed
// at: resuming from half-known state could pay the wrong share.
func (cs *ChainState) loadUBIEpochLocked(ctx context.Context) ubiEpochState {
	if cs.ubiEpoch.set {
		return cs.ubiEpoch.state
	}
	var e ubiEpochState
	total, err1 := strconv.ParseInt(cs.getConfigValueCtx(ctx, ubiEpochTotalKey), 10, 64)
	share, err2 := strconv.ParseFloat(cs.getConfigValueCtx(ctx, ubiEpochShareKey), 64)
	cursor, err3 := strconv.ParseInt(cs.getConfigValueCtx(ctx, ubiEpochCursorKey), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || total <= 0 || share <= 0 || cursor < 0 {
		return ubiEpochState{}
	}
	e.total, e.share, e.cursor = total, share, cursor
	return e
}

// saveUBIEpochLocked persists epoch progress.
func (cs *ChainState) saveUBIEpochLocked(ctx context.Context, e ubiEpochState) error {
	cs.ubiEpoch.state, cs.ubiEpoch.set = e, true
	if err := cs.setConfigValueCtx(ctx, ubiEpochTotalKey, strconv.FormatInt(e.total, 10)); err != nil {
		return fmt.Errorf("persist ubi epoch total: %w", err)
	}
	// Full float precision, not round6: the share is what every chunk pays, so
	// rounding it here would compound across every account in the epoch.
	if err := cs.setConfigValueCtx(ctx, ubiEpochShareKey, strconv.FormatFloat(e.share, 'f', -1, 64)); err != nil {
		return fmt.Errorf("persist ubi epoch share: %w", err)
	}
	if err := cs.setConfigValueCtx(ctx, ubiEpochCursorKey, strconv.FormatInt(e.cursor, 10)); err != nil {
		return fmt.Errorf("persist ubi epoch cursor: %w", err)
	}
	return nil
}

// clearUBIEpochLocked marks the epoch finished.
func (cs *ChainState) clearUBIEpochLocked(ctx context.Context) error {
	cs.ubiEpoch = ubiEpochHolder{}
	for _, k := range []string{ubiEpochTotalKey, ubiEpochShareKey, ubiEpochCursorKey} {
		if err := cs.setConfigValueCtx(ctx, k, ""); err != nil {
			return fmt.Errorf("clear %s: %w", k, err)
		}
	}
	return nil
}

// countHumanAccountsLocked returns how many accounts the epoch will pay.
//
// A COUNT, not an enumeration: the point of chunking is that no single step is
// O(N), and loading every address just to learn how many there are would
// reintroduce exactly the cost being removed.
func (cs *ChainState) countHumanAccountsLocked(ctx context.Context) (int64, error) {
	if cs.db == nil {
		var n int64
		cs.accounts.Range(func(_ string, acc *AccountState) bool {
			if acc.IsHuman {
				n++
			}
			return true
		})
		return n, nil
	}
	var n int64
	if err := cs.dbExecCtx(ctx).QueryRow(`SELECT count(*) FROM chain_accounts WHERE is_human = true`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count human accounts: %w", err)
	}
	return n, nil
}

// humanAccountChunkLocked returns one ordered slice of human addresses.
//
// Ordering and paging happen in the database (ORDER BY ... LIMIT ... OFFSET),
// so each chunk transfers and holds only its own accounts. Ordering by address
// is what makes the split identical on every node; Postgres would otherwise be
// free to return rows in any order at all, and two nodes could credit
// different accounts for the same chunk index.
func (cs *ChainState) humanAccountChunkLocked(ctx context.Context, offset, limit int64) ([]string, error) {
	if cs.db == nil {
		var all []string
		cs.accounts.Range(func(addr string, acc *AccountState) bool {
			if acc.IsHuman {
				all = append(all, addr)
			}
			return true
		})
		sort.Strings(all) // same ordering the SQL path gets, so tests exercise the real shape
		if offset >= int64(len(all)) {
			return nil, nil
		}
		end := offset + limit
		if end > int64(len(all)) {
			end = int64(len(all))
		}
		return all[offset:end], nil
	}
	rows, err := cs.dbExecCtx(ctx).Query(
		`SELECT lower(address) FROM chain_accounts WHERE is_human = true ORDER BY lower(address) LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("enumerate human accounts chunk: %w", err)
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("scan human account: %w", err)
		}
		if a != "" {
			addrs = append(addrs, a)
		}
	}
	return addrs, rows.Err()
}

// distributeUBIChunkLocked credits one chunk and reports whether the epoch is
// now complete.
//
// Opening an epoch fixes the share and the account count. Every later chunk
// reads those stored values, so demurrage arriving mid-epoch cannot change
// what earlier chunks already paid — and the pool is reduced by exactly what
// was paid rather than zeroed, so that demurrage rolls into the next epoch
// instead of disappearing.
func (cs *ChainState) distributeUBIChunkLocked(ctx context.Context) (shares []DistributionShare, complete bool, err error) {
	epoch := cs.loadUBIEpochLocked(ctx)

	if !epoch.open() {
		cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
		poolAcc, ok := cs.accounts.Get(ubiPoolAddr)
		if !ok || poolAcc.Balance <= 0 {
			fmt.Println("[UBI] Pool is empty — nothing to distribute today")
			return nil, true, nil
		}
		total, cErr := cs.countHumanAccountsLocked(ctx)
		if cErr != nil {
			return nil, false, cErr
		}
		if total == 0 {
			fmt.Println("[UBI] No registered humans yet — pool left untouched")
			return nil, true, nil
		}
		share := poolAcc.Balance.Float() / float64(total)
		if round6(share) == 0 {
			fmt.Printf("[UBI] Share %.10f rounds to zero — pool left intact for next distribution\n", share)
			return nil, true, nil
		}
		epoch = ubiEpochState{cursor: 0, share: share, total: total}
		if err := cs.saveUBIEpochLocked(ctx, epoch); err != nil {
			return nil, false, err
		}
		fmt.Printf("[UBI] Epoch opened: %d humans × %.6f AEQ, %d accounts per chunk\n", total, round6(share), ubiChunkSize)
	}

	addrs, err := cs.humanAccountChunkLocked(ctx, epoch.cursor, ubiChunkSize)
	if err != nil {
		return nil, false, err
	}
	if len(addrs) == 0 {
		// The account set shrank mid-epoch (a registration reversal, say).
		// Finish rather than spin: the remaining share stays in the pool and
		// is redistributed next epoch.
		return nil, true, cs.finishUBIEpochLocked(ctx, epoch, epoch.cursor)
	}

	cs.ensureAccountsLoadedCtx(ctx, addrs)

	// Demurrage is settled per account in its own chunk, exactly as the
	// single-shot path settles it for all of them up front. The credit that
	// produces still lands in the pool; because the share is already fixed, it
	// simply belongs to the NEXT epoch rather than this one.
	shares = make([]DistributionShare, 0, len(addrs))
	batch := make([]*AccountState, 0, len(addrs))
	for _, addr := range addrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			continue
		}
		lost, dErr := cs.settleDemurrageLockedCtx(ctx, acc)
		if dErr != nil {
			return nil, false, fmt.Errorf("could not settle demurrage for %s: %w", addr, dErr)
		}
		acc.Balance = acc.Balance.Add(NewDecimal(epoch.share))
		touchActivity(acc)
		if wErr := cs.enforceWealthCapLockedCtx(ctx, acc); wErr != nil {
			return nil, false, fmt.Errorf("could not enforce wealth cap for %s: %w", addr, wErr)
		}
		batch = append(batch, acc)
		shares = append(shares, DistributionShare{Wallet: addr, Amount: round6(epoch.share), DemurrageLost: lost.Float()})
	}
	if err := cs.saveAccountsToDBBatchCtx(ctx, batch); err != nil {
		return nil, false, fmt.Errorf("could not save UBI rewards chunk: %w", err)
	}

	epoch.cursor += int64(len(addrs))
	if epoch.cursor < epoch.total {
		if err := cs.saveUBIEpochLocked(ctx, epoch); err != nil {
			return nil, false, err
		}
		fmt.Printf("[UBI] Chunk done: %d/%d accounts credited\n", epoch.cursor, epoch.total)
		return shares, false, nil
	}
	return shares, true, cs.finishUBIEpochLocked(ctx, epoch, epoch.cursor)
}

// finishUBIEpochLocked debits the pool by exactly what was paid out and closes
// the epoch.
//
// Deliberately NOT a zeroing. The single-shot path can zero because it credits
// everyone in the same instant the balance was read. Here the pool keeps
// receiving demurrage while the epoch runs, and zeroing would destroy those
// credits; subtracting the paid amount leaves them for the next epoch, which
// is where the fixed share says they belong.
func (cs *ChainState) finishUBIEpochLocked(ctx context.Context, epoch ubiEpochState, credited int64) error {
	if credited > 0 {
		cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
		if poolAcc, ok := cs.accounts.Get(ubiPoolAddr); ok {
			paid := epoch.share * float64(credited)
			remaining := poolAcc.Balance.Float() - paid
			if remaining < 0 {
				// Only reachable if the pool was drained by something else
				// mid-epoch. Clamp rather than mint a negative balance.
				remaining = 0
			}
			poolAcc.Balance = NewDecimal(remaining)
			if err := cs.saveAccountToDBCtx(ctx, poolAcc); err != nil {
				return fmt.Errorf("could not debit UBI pool: %w", err)
			}
		}
	}
	fmt.Printf("[UBI] Epoch complete: %d accounts credited %.6f AEQ each\n", credited, round6(epoch.share))
	return cs.clearUBIEpochLocked(ctx)
}
