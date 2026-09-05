package keeper

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// SCALING_ARCHITECTURE.md roadmap step 6 — parallel replay.
//
// WHY THIS IS THE BOTTLENECK (measured live 2026-07-26, and the reason the
// user's question "muss primary das nicht aushalten?" was the right one):
//
// The WAL fast path (transferConcurrentWAL) accelerates only the INGESTION of
// transactions — the path they take when they arrive by RPC on that specific
// node. Every OTHER node receives the same transactions as BLOCKS and applies
// them through replayTransactions, one at a time, under cs.mu.Lock().
//
// A 20-second load test against Contabo2 (~250 TPS, 4,948 transfers, blocks
// carrying 117-173 transactions each) therefore left the primary ~1,200
// blocks behind and effectively unreachable for minutes — replay holds cs.mu,
// so /api/status cannot answer either — until it caught up on its own.
//
// So network throughput is bounded by the SLOWEST REPLAY, not by the fastest
// ingestion. 50k TPS on one node is worth nothing if the others replay at a
// few hundred.
//
// WHY THE PREVIOUS ATTEMPT WAS REVERTED (41b1eee, 2026-07-25), and how this
// differs:
//
// That version guarded its workers with a local sync.Mutex while they wrote
// through dag.state.activeTx — a single *sql.Tx shared across goroutines. A
// local mutex serializes those workers against each other but not against any
// other goroutine touching the same field, so it never established the
// invariant it claimed. Two goroutines on one Postgres connection desync the
// wire protocol; the exact signature (`pq: unexpected Parse response "(D)
// DataRow"`) appeared on Contabo2 minutes after deploy and cost two consensus
// blocks.
//
// The lesson, which Block-STM (Aptos, 160k TPS) states directly: DO NOT TOUCH
// THE DATABASE DURING THE PARALLEL PHASE. This implementation therefore has
// three strictly separated phases:
//
//	1. serial   — warm every account the batch needs (the only DB reads)
//	2. parallel — pure in-memory arithmetic on disjoint AccountState structs
//	3. serial   — ONE batched write for every account the batch touched
//
// Phase 2 performs no DB access of any kind, so there is no shared *sql.Tx to
// corrupt. It needs no account locks either: the batch is disjoint BY
// CONSTRUCTION, so no two workers can reach the same AccountState, and the
// map itself is only read (every account was made resident in phase 1). The
// one genuinely shared piece of state, cs.accountSetXOR, is mutated through
// updateAccountLeafLocked, which already carries accountSetXORMu for exactly
// this case — see that field's own comment.
//
// DETERMINISM: a batch is only ever formed from CONSECUTIVE transfers whose
// touched addresses are pairwise disjoint, so applying them in any order
// yields the identical final state. That is not an assumption — it is the
// property TestReplayTransactions_DisjointTransfers_OrderIndependent and its
// fuzz variant already pin, written before any parallel implementation
// existed precisely so this step could rely on it.

// parallelReplayMinBatch is the smallest batch this path is used for.
//
// KORREKTUR (05.09.2026, gemessen): der Wert stand auf 16, begruendet mit
// "smallest batch worth spawning goroutines for -- below it the scheduling
// overhead exceeds the arithmetic". Diese Begruendung war falsch, weil sie
// die falsche Ersparnis betrachtet. Der Gewinn dieses Pfades ist NICHT die
// parallele Arithmetik in Phase 2 -- die ist ein paar Nanosekunden je
// Ueberweisung. Er ist Phase 3: EIN gebuendelter Multi-Row-Write fuer alle
// beruehrten Konten, statt zwei einzelner Datenbank-Umlaeufe je Ueberweisung
// im seriellen Pfad (saveAccountToDBCtx fuer Sender und Empfaenger).
//
// Und diese Ersparnis faellt schon ab zwei Ueberweisungen an, nicht ab
// sechzehn. Gemessen auf dem Primary unter Last (replay_phasen, 376 Bloecke):
//
//	seriell    4,724 ms je Ueberweisung   2.365 Stueck   48,2 % der Sperre
//	parallel   0,281 ms je Ueberweisung  27.565 Stueck   32,7 % der Sperre
//
// Sieben Komma neun Prozent der Ueberweisungen kosteten also fast die Haelfte
// der Zeit, in der das Nachspielen die globale Sperre haelt -- und in der
// nach Go's RWMutex-Semantik jede ankommende Ueberweisung ausgesperrt ist.
//
// Warum ueberhaupt so viele seriell liefen: collectDisjointTransferBatch
// beendet einen Lauf, sobald sich eine Adresse wiederholt. Bei wenigen
// hundert Konten und hunderten Ueberweisungen je Block passiert das staendig,
// und jeder Lauf, der unter dieser Schwelle blieb, ging Ueberweisung fuer
// Ueberweisung ueber den teuren Pfad. Die Schwelle hat sich also selbst
// gefuettert.
//
// Zwei ist das Minimum, ab dem Buendeln ueberhaupt etwas spart (bei einer
// einzelnen Ueberweisung schreibt auch der Buendelpfad zwei Konten). Der
// Goroutinen-Overhead bleibt bestehen, ist aber gegen einen eingesparten
// Datenbank-Umlauf um drei Groessenordnungen kleiner: Mikrosekunden gegen
// Millisekunden.
//
// Die Semantik aendert sich nicht. Dieser Pfad lehnt weiterhin bei jedem
// Zweifel ab (unbekanntes Konto, unzureichendes Guthaben, Wohlstandsgrenze)
// und ueberlaesst die Ueberweisung dann dem seriellen Pfad, der Fehler und
// Protokollzeilen unveraendert erzeugt. Die volle keeper-Testsuite --
// einschliesslich TestParallelReplay_MatchesSerialExactly und der
// Determinismus-Fuzz -- ist mit diesem Wert gruen.
const parallelReplayMinBatch = 2

// replayBatchItem is one transfer accepted into a parallel batch, resolved to
// its two account pointers during the serial warm-up phase so the parallel
// phase needs no map lookups at all.
type replayBatchItem struct {
	from    *AccountState
	to      *AccountState
	amount  float64
	fromKey string
	toKey   string
}

// collectDisjointTransferBatch walks txs starting at index start and returns
// the longest run of CONSECUTIVE transactions that may be applied in parallel.
//
// A transaction joins the run only if all of the following hold:
//   - it is a plain "transfer" with well-formed fields
//   - it carries NO demurrage (FromDemurrageLost/ToDemurrageLost both zero).
//     Demurrage settlement credits the tokenomics pools and persists them,
//     i.e. it is DB work and shared-state work, and it is exactly the
//     eligibility line transferConcurrentWAL already draws for the same
//     reason.
//   - neither of its addresses has appeared anywhere earlier in this run
//
// The first transaction that fails any of these ends the run. Returning early
// rather than skipping past it is what preserves ordering semantics: anything
// after a non-batchable transaction may depend on it.
func collectDisjointTransferBatch(txs []Transaction, start int) (batch []Transaction, touched map[string]bool) {
	touched = make(map[string]bool)
	for i := start; i < len(txs); i++ {
		tx := txs[i]
		if tx.Type != "transfer" {
			break
		}
		from := strings.ToLower(strings.TrimSpace(tx.Wallet))
		to := strings.ToLower(strings.TrimSpace(tx.To))
		if from == "" || to == "" || tx.Amount <= 0 || from == to {
			break
		}
		if tx.FromDemurrageLost != 0 || tx.ToDemurrageLost != 0 {
			break
		}
		if touched[from] || touched[to] {
			break
		}
		touched[from] = true
		touched[to] = true
		batch = append(batch, tx)
	}
	return batch, touched
}

// applyTransferBatchParallel applies an already-validated disjoint batch.
//
// Three-valued result, and the distinction is critical:
//
//	(false, nil)  — declined BEFORE mutating anything. The caller replays
//	                these transfers serially, reproducing the serial path's
//	                behaviour and error messages exactly. Pure speed-up.
//	(true,  nil)  — applied and persisted.
//	(false, err)  — memory was ALREADY mutated and persistence then failed.
//	                The caller MUST treat this as a hard block failure and
//	                roll back. It must never fall back to the serial path
//	                here: that would apply every transfer a second time.
//
// That last case is why this returns an error at all rather than a plain
// bool. An earlier draft of this function returned false on a failed batch
// write, which would have double-applied the whole batch — the same defect
// class that drove 74 production accounts negative on 2026-07-25.
//
// activityAt is the replayed block's own Timestamp, stamped onto every
// participant's demurrage clock exactly as the serial path does — see
// touchActivityAt for why this must not be the replaying node's wall clock.
//
// Caller must hold cs.mu (write), exactly as the serial path does.
func (cs *ChainState) applyTransferBatchParallel(ctx context.Context, batch []Transaction, activityAt int64) (applied bool, err error) {
	if len(batch) < parallelReplayMinBatch {
		return false, nil
	}

	// ---- Phase 1 (serial): warm every account. The ONLY DB access. ----
	items := make([]replayBatchItem, 0, len(batch))
	for _, tx := range batch {
		from := strings.ToLower(strings.TrimSpace(tx.Wallet))
		to := strings.ToLower(strings.TrimSpace(tx.To))
		cs.ensureAccountLoadedCtx(ctx, from)
		cs.ensureAccountLoadedCtx(ctx, to)
		fromAcc, okFrom := cs.accounts.Get(from)
		toAcc, okTo := cs.accounts.Get(to)
		if !okFrom || !okTo {
			return false, nil // unknown account — let the serial path report it
		}
		items = append(items, replayBatchItem{
			from: fromAcc, to: toAcc, amount: tx.Amount, fromKey: from, toKey: to,
		})
	}

	// Sufficiency is checked here, serially, BEFORE anything is mutated. The
	// batch is disjoint, so each sender appears at most once and its balance
	// cannot be changed by another member — meaning this check is exactly as
	// authoritative as the serial path's own, just hoisted. If any single
	// transfer would fail, the whole batch is declined and the serial path
	// replays all of them, reproducing its error handling verbatim.
	//
	// FIX (audit 2026-08-15): the wealth-cap check below was missing entirely.
	// The serial path this replaces (applyTransferDeltaLocked) calls
	// enforceWealthCapLockedCtx on every recipient, which trims a balance that
	// lands above avg×multiplier back down to the cap and credits the excess to
	// the four tokenomics pools. Phase 2 cannot do that — enforceWealthCap
	// credits pools through distributeSwapFeeCtx, i.e. shared state and DB
	// work, exactly what phase 2 is forbidden to touch — so a cap crossing
	// inside a batchable run was silently skipped: the recipient kept the full
	// uncapped amount and the pools were never credited.
	//
	// That is a fork, not a rounding difference. The three INGESTION fast paths
	// (transferConcurrent, transferBatchConcurrent, transferConcurrentWAL) each
	// draw exactly this eligibility line for exactly this reason and hand the
	// transfer to the slow path, which caps it — so a block genuinely can carry
	// a capped transfer, while every node replaying it through this batch path
	// would apply it uncapped. Proven by
	// TestParallelReplay_EnforcesWealthCapLikeSerial: recipient 26,000 vs
	// 25,000 AEQ, all four pools 0 vs 400/300/200/100, different StateRoot.
	//
	// Declining here (rather than trying to cap in phase 2) keeps this path
	// what its doc comment promises — a pure speed-up whose declines cost only
	// speed — and lets the serial path reproduce the cap, the pool credits and
	// the log line verbatim. Recipients appear at most once in a disjoint
	// batch, so post-transfer balance is exact; the Decimal add mirrors the
	// arithmetic enforceWealthCapLockedCtx itself would see, and tokenomics
	// pool addresses are exempt there, so they must not trigger a decline here.
	capAmt, hasCap := cs.wealthCapAmountLocked()
	for _, it := range items {
		if it.from.Balance.Float() < it.amount {
			return false, nil
		}
		if hasCap && !isTokenomicsPoolAddress(it.toKey) &&
			it.to.Balance.Add(NewDecimal(it.amount)).Float() > capAmt {
			return false, nil
		}
	}

	// ---- Phase 2 (parallel): pure in-memory arithmetic, NO database. ----
	workers := runtime.NumCPU()
	if workers > len(items) {
		workers = len(items)
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	chunk := (len(items) + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo := w * chunk
		if lo >= len(items) {
			break
		}
		hi := lo + chunk
		if hi > len(items) {
			hi = len(items)
		}
		wg.Add(1)
		go func(part []replayBatchItem) {
			defer wg.Done()
			for _, it := range part {
				amt := NewDecimal(it.amount)
				it.from.Balance = it.from.Balance.Sub(amt)
				touchActivityAt(it.from, activityAt)
				it.to.Balance = it.to.Balance.Add(amt)
				touchActivityAt(it.to, activityAt)
				// The one piece of genuinely shared state; guarded by
				// accountSetXORMu inside, which exists for precisely this.
				cs.updateAccountLeafLocked(it.from)
				cs.updateAccountLeafLocked(it.to)
			}
		}(items[lo:hi])
	}
	wg.Wait()

	// ---- Phase 3 (serial): ONE batched write for everything touched. ----
	seen := make(map[string]bool, len(items)*2)
	accs := make([]*AccountState, 0, len(items)*2)
	for _, it := range items {
		if !seen[it.fromKey] {
			seen[it.fromKey] = true
			accs = append(accs, it.from)
		}
		if !seen[it.toKey] {
			seen[it.toKey] = true
			accs = append(accs, it.to)
		}
	}
	if err := cs.saveAccountsToDBBatchCtx(ctx, accs); err != nil {
		// Memory is already mutated at this point. Returning (false, nil)
		// here would send the caller to the serial path and apply every
		// transfer in this batch a SECOND time. Surface it as a hard error
		// so the block is rolled back through the caller's existing
		// rollback snapshot instead — see this function's doc comment.
		return false, fmt.Errorf("parallel transfer batch: could not persist %d account(s): %w", len(accs), err)
	}
	return true, nil
}
