package keeper

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// account_write_buffer_test.go pins the properties that make deferring
// account row writes safe rather than merely fast. Each test here
// corresponds to one claim in account_write_buffer.go's comment, and the
// buffer would be a genuinely dangerous optimisation without them:
// consensus-critical replay is exactly the code where "usually equivalent"
// is not good enough.

func newBufferTestDAG(t *testing.T, cs *ChainState) *BlockDAG {
	t.Helper()
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	return dag
}

// TestAccountWriteBuffer_CollapsesRepeatedWritesToFinalState is the unit-level
// claim: an account written several times within one operation produces ONE
// entry holding its latest state, not several entries racing to overwrite
// each other in the batch.
func TestAccountWriteBuffer_CollapsesRepeatedWritesToFinalState(t *testing.T) {
	ctx, buf := withAccountWriteBuffer(context.Background(), 4)

	acc := &AccountState{Address: "0xaaa", Balance: NewDecimal(10)}
	other := &AccountState{Address: "0xbbb", Balance: NewDecimal(5)}

	if got := accountWriteBufferFrom(ctx); got != buf {
		t.Fatal("buffer not reachable through its own ctx")
	}
	buf.add(acc)
	buf.add(other)
	acc.Balance = NewDecimal(30)
	buf.add(acc) // same address again, after mutation

	if buf.len() != 2 {
		t.Fatalf("buffer holds %d entries, want 2 (one per distinct address)", buf.len())
	}
	pending := buf.drain()
	if buf.len() != 0 {
		t.Fatal("drain left entries behind — a second flush would write the same rows twice")
	}
	byAddr := map[string]float64{}
	for _, a := range pending {
		byAddr[a.Address] = a.Balance.Float()
	}
	if byAddr["0xaaa"] != 30 {
		t.Errorf("collapsed entry carries balance %v, want the final 30", byAddr["0xaaa"])
	}
	if byAddr["0xbbb"] != 5 {
		t.Errorf("unrelated account got balance %v, want 5", byAddr["0xbbb"])
	}
}

// TestAccountWriteBuffer_SameAddressDifferentPointerReplacesEntry covers the
// case add() specifically guards: replay can hold two different
// *AccountState values for one address (a rollback restore installs a fresh
// copy). Writing both would hand the batch's optimistic-version check the
// same row twice, and one of the two would report a spurious conflict.
func TestAccountWriteBuffer_SameAddressDifferentPointerReplacesEntry(t *testing.T) {
	_, buf := withAccountWriteBuffer(context.Background(), 2)
	buf.add(&AccountState{Address: "0xaaa", Balance: NewDecimal(1), Version: 3})
	buf.add(&AccountState{Address: "0xaaa", Balance: NewDecimal(2), Version: 3})
	if buf.len() != 1 {
		t.Fatalf("buffer holds %d entries for one address, want 1", buf.len())
	}
	if got := buf.drain()[0].Balance.Float(); got != 2 {
		t.Errorf("kept balance %v, want the later 2", got)
	}
}

// TestAccountWriteBuffer_NoBufferInCtxWritesThrough proves the buffer is
// strictly opt-in: every caller outside replay still writes immediately, so
// this change cannot alter the behaviour of any other atomic operation.
func TestAccountWriteBuffer_NoBufferInCtxWritesThrough(t *testing.T) {
	if accountWriteBufferFrom(context.Background()) != nil {
		t.Fatal("a plain context reported a write buffer")
	}
	cs := newTestState()
	acc := &AccountState{Address: "0xaaa", Balance: NewDecimal(1)}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.saveAccountToDBCtx(context.Background(), acc); err != nil {
		t.Fatalf("write-through save: %v", err)
	}
	if acc.Version == 0 {
		t.Error("write-through save did not mark the account as saved — it was buffered instead")
	}
}

// TestReplayBuffered_StateRootAndRowsMatchUnbufferedApply is the consensus
// claim, and the reason this file exists. A block replayed through the
// buffered path must leave byte-identical state — every balance, and the
// StateRoot itself — to applying the same transfers one at a time through
// the ordinary, unbuffered delta path.
//
// Two independent ChainStates against the same real Postgres would collide
// on the shared table, so the comparison runs sequentially: replay the
// block, record the root and balances, reset, apply the same transfers
// through ApplyTransferDelta (which never sees a buffer), compare.
func TestReplayBuffered_StateRootAndRowsMatchUnbufferedApply(t *testing.T) {
	truncateDistTestTables(t)

	const pairs = 12
	senders := make([]string, pairs)
	recipients := make([]string, pairs)
	for i := range senders {
		senders[i] = distTestAddr(3_000_000 + 2*i)
		recipients[i] = distTestAddr(3_000_001 + 2*i)
	}
	txs := make([]Transaction, pairs)
	for i := range txs {
		txs[i] = Transaction{
			Type: "transfer", Wallet: senders[i], To: recipients[i], Amount: 7,
			TxHash: fmt.Sprintf("0xbufcmp-%d", i),
		}
	}

	seed := func(cs *ChainState) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		accs := make([]*AccountState, 0, pairs)
		for _, s := range senders {
			a := &AccountState{Address: s, Balance: NewDecimal(500)}
			cs.accounts.Set(s, a)
			accs = append(accs, a)
		}
		if err := cs.saveAccountsToDBBatch(accs); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// --- buffered: through replayTransactions ---
	csA := NewChainState("")
	if !csA.useDB {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	seed(csA)
	dag := newBufferTestDAG(t, csA)
	if !dag.replayTransactions(&Block{
		Height: 1, Hash: "0xbuffered-block", Timestamp: nowUnix(), Transactions: txs,
	}, false) {
		csA.db.Close()
		t.Fatal("buffered replay rejected the block")
	}
	csA.mu.Lock()
	bufferedRoot := csA.stateRootLocked(csA.getConfigValue("last_ubi_at"))
	bufferedBalances := map[string]float64{}
	for i := range senders {
		s, _ := csA.accounts.Get(senders[i])
		r, _ := csA.accounts.Get(recipients[i])
		bufferedBalances[senders[i]] = s.Balance.Float()
		bufferedBalances[recipients[i]] = r.Balance.Float()
	}
	// The rows really are in Postgres, not only in memory — the whole point
	// of deferring is that the write still happens.
	var dbRecipient float64
	if err := csA.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, recipients[0]).Scan(&dbRecipient); err != nil {
		csA.mu.Unlock()
		csA.db.Close()
		t.Fatalf("recipient row missing from Postgres after buffered replay: %v", err)
	}
	if dbRecipient != 7 {
		csA.mu.Unlock()
		csA.db.Close()
		t.Fatalf("recipient row in Postgres has balance %v, want 7 — the buffered write never landed", dbRecipient)
	}
	csA.mu.Unlock()
	csA.db.Close()

	// --- unbuffered: the same transfers, one ApplyTransferDelta at a time ---
	truncateDistTestTables(t)
	csB := NewChainState("")
	defer csB.db.Close()
	seed(csB)
	for i := range txs {
		if err := csB.ApplyTransferDelta(senders[i], recipients[i], 7, 0, 0); err != nil {
			t.Fatalf("unbuffered apply %d: %v", i, err)
		}
	}
	csB.mu.Lock()
	unbufferedRoot := csB.stateRootLocked(csB.getConfigValue("last_ubi_at"))
	for addr, want := range bufferedBalances {
		acc, ok := csB.accounts.Get(addr)
		if !ok {
			csB.mu.Unlock()
			t.Fatalf("%s missing after unbuffered apply", addr)
		}
		if got := acc.Balance.Float(); got != want {
			csB.mu.Unlock()
			t.Fatalf("%s: buffered replay produced %v, unbuffered produced %v", addr, want, got)
		}
	}
	csB.mu.Unlock()

	if bufferedRoot != unbufferedRoot {
		t.Fatalf("STATE ROOT DIVERGED between buffered replay and unbuffered apply.\n"+
			"  buffered:   %s\n  unbuffered: %s\n"+
			"Deferring account row writes must be invisible to consensus — a difference here\n"+
			"means nodes running the two paths would fork.", bufferedRoot, unbufferedRoot)
	}
}

// TestReplayBuffered_RolledBackBlockWritesNothing is the failure-path claim:
// a block that hard-fails partway through must leave the database exactly as
// it found it. With buffering this is a genuinely new risk — the rollback
// has to discard pending writes rather than undo committed ones.
func TestReplayBuffered_RolledBackBlockWritesNothing(t *testing.T) {
	truncateDistTestTables(t)
	cs := NewChainState("")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	defer cs.db.Close()

	sender := distTestAddr(3_100_000)
	recipient := distTestAddr(3_100_001)
	cs.mu.Lock()
	acc := &AccountState{Address: sender, Balance: NewDecimal(100)}
	cs.accounts.Set(sender, acc)
	err := cs.saveAccountsToDBBatch([]*AccountState{acc})
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	dag := newBufferTestDAG(t, cs)
	// A good transfer followed by a transaction type nothing recognises,
	// which replayTransactions treats as a genuine state-inconsistency
	// failure and rolls the whole block back.
	ok := dag.replayTransactions(&Block{
		Height: 1, Hash: "0xrollback-block", Timestamp: nowUnix(),
		Transactions: []Transaction{
			{Type: "transfer", Wallet: sender, To: recipient, Amount: 40, TxHash: "0xrb-good"},
			{Type: "definitely-not-a-real-tx-type", Wallet: sender, TxHash: "0xrb-bad"},
		},
	}, false)
	if ok {
		t.Fatal("replayTransactions accepted a block containing an unknown TX type")
	}

	var senderBalance float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, sender).Scan(&senderBalance); err != nil {
		t.Fatalf("sender row: %v", err)
	}
	if senderBalance != 100 {
		t.Errorf("sender row is %v after a rolled-back block, want the original 100 — a buffered "+
			"write from the rolled-back block reached Postgres", senderBalance)
	}
	var recipientRows int
	if err := cs.db.QueryRow(`SELECT count(*) FROM chain_accounts WHERE lower(address) = $1`, recipient).Scan(&recipientRows); err != nil {
		t.Fatalf("recipient count: %v", err)
	}
	if recipientRows != 0 {
		t.Errorf("the rolled-back block's recipient has %d row(s) in Postgres, want 0", recipientRows)
	}
	cs.mu.Lock()
	if acc, ok := cs.accounts.Get(sender); ok && acc.Balance.Float() != 100 {
		t.Errorf("in-memory sender balance is %v after rollback, want 100", acc.Balance.Float())
	}
	cs.mu.Unlock()
}

// TestReplayBuffered_FlushesBeforeTransactionsThatReadRowsBack covers the one
// case the buffer is NOT transparent for: a transaction type that reads
// account rows back out of SQL. register_human writes a row; ubi_distribution
// then enumerates humans with `WHERE is_human = true`. If the registration
// were still sitting in the buffer, the new human would be silently skipped
// from the distribution — a consensus divergence that no in-memory assertion
// would catch.
func TestReplayBuffered_FlushesBeforeTransactionsThatReadRowsBack(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := NewChainState("")
	defer cs.db.Close()

	human := distTestAddr(3_200_000)
	cs.mu.Lock()
	acc := &AccountState{Address: human, Balance: NewDecimal(50), IsHuman: true}
	cs.accounts.Set(human, acc)
	seedErr := cs.saveAccountsToDBBatch([]*AccountState{acc})
	cs.mu.Unlock()
	if seedErr != nil {
		t.Fatalf("seed: %v", seedErr)
	}

	// Buffer the human's row by mutating it through a transfer, then run a
	// transaction type that enumerates humans from SQL. The enumeration must
	// see the flushed row.
	ctx, _ := withAccountWriteBuffer(context.Background(), 4)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	target, _ := cs.accounts.Get(human)
	target.Balance = NewDecimal(77)
	if err := cs.saveAccountToDBCtx(ctx, target); err != nil {
		t.Fatalf("buffered save: %v", err)
	}
	var beforeFlush float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, human).Scan(&beforeFlush); err != nil {
		t.Fatalf("read before flush: %v", err)
	}
	if beforeFlush != 50 {
		t.Fatalf("row already at %v before any flush — the write was not deferred at all, so this "+
			"test cannot detect a missing flush", beforeFlush)
	}
	if err := cs.flushAccountWriteBuffer(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	var afterFlush float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, human).Scan(&afterFlush); err != nil {
		t.Fatalf("read after flush: %v", err)
	}
	if afterFlush != 77 {
		t.Errorf("row is %v after flush, want 77 — a SQL-level read (e.g. ubi_distribution's "+
			"`WHERE is_human = true` enumeration) would have seen stale state", afterFlush)
	}
	// Flushing again must be a no-op, not a second write.
	if err := cs.flushAccountWriteBuffer(ctx); err != nil {
		t.Errorf("second flush should be a no-op, got: %v", err)
	}
}
