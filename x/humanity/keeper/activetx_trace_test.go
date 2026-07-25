package keeper

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestActiveTx_AtomicOperationsTakeNoImplicitFallback is the runtime half of
// the Roadmap step 5 gate. activetx_static_test.go proves, from the source,
// that no code path from an atomic root reaches the database without
// threading ctx. This proves the same thing the other way round — by
// actually running the atomic operations against a real Postgres with the
// fallback recorder armed, and asserting it never fires.
//
// The two are complementary and neither is redundant: the static walk can
// only see calls it can resolve by name, while this one only sees paths the
// test actually executes. A regression would have to evade both.
//
// Opt-in on a real database, like every other _db_test.go here: the whole
// point is the cs.db != nil branches, which are the only ones that can have
// a transaction to fall back to in the first place.
func TestActiveTx_AtomicOperationsTakeNoImplicitFallback(t *testing.T) {
	truncateDistTestTables(t) // skips unless AEQUITAS_TPS_BENCH=1 + DATABASE_URL

	cs := NewChainState("")
	if !cs.useDB || cs.db == nil {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	defer cs.db.Close()

	prev := SetActiveTxTraceForced(true)
	defer SetActiveTxTraceForced(prev)
	ResetActiveTxFallbackSites()

	alice, bob := distTestAddr(0x9001), distTestAddr(0x9002)

	// Registration: runAtomicWithOutbox, the plain single-operation root.
	if err := cs.RegisterHumanAtomic(alice, Transaction{Type: "register_human", Wallet: alice, TxHash: "0xactivetx-reg-a"}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if err := cs.RegisterHumanAtomic(bob, Transaction{Type: "register_human", Wallet: bob, TxHash: "0xactivetx-reg-b"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	// A transfer, which reaches the demurrage/wealth-cap/pool-credit paths
	// that used to be the densest cluster of implicit cs.activeTx pickups.
	if _, _, err := cs.TransferAtomic(alice, bob, 10, Transaction{
		Type: "transfer", Wallet: alice, To: bob, Amount: 10, TxHash: "0xactivetx-transfer",
	}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// The daily distribution round: runAtomicDistributionWithOutbox, the
	// multi-sub-step root (UBI, validators, LP, escrow) — a different and
	// much wider slice of the call graph than the single-operation root.
	if err := cs.RunDailyDistributionAtomic(nowUnix()); err != nil {
		t.Fatalf("daily distribution: %v", err)
	}

	// Block replay: the third root, and the one that historically depended
	// on cs.activeTx most heavily (see replayTransactions' replayCtx).
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	blk := &Block{
		Height:    1,
		Hash:      "0xactivetx-replay-block",
		Timestamp: nowUnix(),
		Proposer:  alice,
		Transactions: []Transaction{{
			Type: "transfer", Wallet: alice, To: bob, Amount: 1, TxHash: "0xactivetx-replay-tx",
		}},
	}
	if !dag.replayTransactions(blk, false) {
		t.Fatal("replayTransactions rejected the block — the fallback assertion below would be vacuous")
	}

	if sites := ActiveTxFallbackSites(); len(sites) > 0 {
		t.Fatalf("%d call path(s) still found their transaction in cs.activeTx instead of ctx.\n"+
			"Each one blocks two atomic operations from running concurrently (Roadmap step 5);\n"+
			"thread the operation's ctx through instead — see dbExecCtx's comment:\n  %s",
			len(sites), strings.Join(sites, "\n  "))
	}
}

// TestActiveTx_ConcurrentAtomicOperationsUseSeparateTransactions is what the
// migration was FOR. Two atomic operations running at the same time must
// each see their own *sql.Tx — never each other's, and never the pool by
// accident. Before ctx threading this was structurally impossible to check:
// there was one cs.activeTx field, so "which transaction is this write
// joining" had exactly one possible answer at any instant.
//
// This drives both operations through the real dbExecCtx resolution path and
// asserts each got back precisely the transaction its own ctx carried.
func TestActiveTx_ConcurrentAtomicOperationsUseSeparateTransactions(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	cs := NewChainState("")
	if cs.db == nil {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	defer cs.db.Close()

	txA, err := cs.db.Begin()
	if err != nil {
		t.Fatalf("begin A: %v", err)
	}
	defer txA.Rollback()
	txB, err := cs.db.Begin()
	if err != nil {
		t.Fatalf("begin B: %v", err)
	}
	defer txB.Rollback()

	ctxA, ctxB := withTx(context.Background(), txA), withTx(context.Background(), txB)

	// Simulate operation A having parked its transaction in the legacy
	// field. B must be entirely unaffected by that: its ctx is the only
	// thing that decides where its writes go.
	cs.setActiveTx(txA)
	defer cs.setActiveTx(nil)

	if got := cs.dbExecCtx(ctxA); got != sqlExecutor(txA) {
		t.Errorf("operation A resolved to %T, want its own *sql.Tx", got)
	}
	if got := cs.dbExecCtx(ctxB); got != sqlExecutor(txB) {
		t.Errorf("operation B resolved to %T, want its own *sql.Tx — it must not\n"+
			"inherit A's transaction from cs.activeTx, and must not silently fall to the pool", got)
	}
	if cs.activeTxCtx(ctxB) != txB {
		t.Error("activeTxCtx(B) did not return B's own transaction")
	}

	// And the writes really are isolated: A's uncommitted row must be
	// invisible to B, which is the property that makes two concurrent
	// atomic operations safe at all.
	key := "activetx_isolation_probe"
	if _, err := cs.dbExecCtx(ctxA).Exec(
		`INSERT INTO chain_config (key, value) VALUES ($1,$2)
         ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, "written-by-A"); err != nil {
		t.Fatalf("A insert: %v", err)
	}
	var seenByB string
	err = cs.dbExecCtx(ctxB).QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&seenByB)
	if err == nil {
		t.Errorf("operation B read %q from A's uncommitted transaction — the two are not isolated", seenByB)
	} else if err != sql.ErrNoRows {
		t.Fatalf("B read: unexpected error %v", err)
	}
	fmt.Fprintln(os.Stderr, "[activetx] two concurrent atomic operations resolved to separate, isolated transactions")
}
