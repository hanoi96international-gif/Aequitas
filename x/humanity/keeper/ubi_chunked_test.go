package keeper

import (
	"context"
	"fmt"
	"testing"
)

// Chunking a distribution is a consensus change: it decides which block
// carries which credit. Everything below is about the one property that
// matters for that — two nodes running the same code must produce byte-identical
// results — plus the money invariants, because this credits real balances.

// withChunkSize shrinks the chunk for the duration of one test, so the
// multi-chunk path is actually exercised instead of everything fitting in the
// first chunk.
func withChunkSize(t *testing.T, n int64) {
	t.Helper()
	prev := ubiChunkSize
	ubiChunkSize = n
	t.Cleanup(func() { ubiChunkSize = prev })
}

func newUBITestState(humans int, poolBalance float64) (*ChainState, []string) {
	cs := newTestState()
	addrs := make([]string, 0, humans)
	for i := 0; i < humans; i++ {
		// Deliberately NOT in sorted order: the code must impose the ordering
		// itself, not inherit it from how accounts happened to be created.
		addr := fmt.Sprintf("0x%04x", (i*7919)%65536)
		addHuman(cs, addr, 0)
		addrs = append(addrs, addr)
	}
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(poolBalance)})
	return cs, addrs
}

// The whole point: no single chunk may do work proportional to the account
// count, and the epoch must still finish having paid everyone exactly once.
func TestUBIChunking_CreditsEveryHumanExactlyOnceAcrossChunks(t *testing.T) {
	const humans = 12
	withChunkSize(t, 5) // three chunks: 5, 5, 2
	cs, addrs := newUBITestState(humans, 120)
	ctx := context.Background()

	credited := map[string]int{}
	var rounds int
	for {
		rounds++
		if rounds > humans+5 {
			t.Fatal("epoch never completed — the cursor is not advancing")
		}
		shares, done, err := cs.distributeUBIChunkLocked(ctx)
		if err != nil {
			t.Fatalf("chunk %d failed: %v", rounds, err)
		}
		for _, s := range shares {
			credited[s.Wallet]++
		}
		if done {
			break
		}
	}

	if len(credited) != humans {
		t.Fatalf("credited %d of %d humans — a chunked distribution must still reach everyone", len(credited), humans)
	}
	for _, a := range addrs {
		if credited[a] != 1 {
			t.Fatalf("%s was credited %d times; exactly once is the only correct answer", a, credited[a])
		}
	}
}

// Two nodes must walk the accounts in the same order, or the same chunk index
// credits different people on different nodes and the chain forks. The order
// must come from the code, not from map iteration.
func TestUBIChunking_OrdersAccountsIdentically(t *testing.T) {
	ctx := context.Background()
	first := ""
	for run := 0; run < 5; run++ {
		cs, _ := newUBITestState(20, 200)
		addrs, err := cs.humanAccountChunkLocked(ctx, 0, 5)
		if err != nil {
			t.Fatalf("chunk enumeration failed: %v", err)
		}
		got := fmt.Sprint(addrs)
		if run == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("chunk 0 differs between runs:\n  %s\n  %s\nTwo nodes would credit different accounts for the same chunk and the chain would fork.", first, got)
		}
	}
}

// The share is fixed when the epoch opens and must not drift afterwards. If a
// later chunk recomputed it from the pool — which keeps receiving demurrage
// while the epoch runs — accounts would be paid different amounts depending
// only on which chunk they landed in.
func TestUBIChunking_ShareIsFixedWhenTheEpochOpens(t *testing.T) {
	withChunkSize(t, 3)
	cs, _ := newUBITestState(10, 100)
	ctx := context.Background()

	shares1, done, err := cs.distributeUBIChunkLocked(ctx)
	if err != nil || done {
		t.Fatalf("expected an open epoch after the first chunk (err=%v done=%v)", err, done)
	}
	if len(shares1) == 0 {
		t.Fatal("first chunk credited nobody")
	}
	want := shares1[0].Amount

	// Simulate demurrage arriving mid-epoch, which is exactly what happens in
	// production while a distribution is running.
	if pool, ok := cs.accounts.Get(ubiPoolAddr); ok {
		pool.Balance = pool.Balance.Add(NewDecimal(500))
	}

	for {
		shares, complete, err := cs.distributeUBIChunkLocked(ctx)
		if err != nil {
			t.Fatalf("later chunk failed: %v", err)
		}
		for _, s := range shares {
			if s.Amount != want {
				t.Fatalf("a later chunk paid %.6f where the epoch opened at %.6f — the share must be fixed for the whole epoch, or what someone receives depends on which chunk they fell into", s.Amount, want)
			}
		}
		if complete {
			break
		}
	}
}

// The pool must be debited by exactly what was paid, never zeroed. Demurrage
// arriving mid-epoch belongs to the NEXT epoch; zeroing would destroy it.
func TestUBIChunking_PoolKeepsMidEpochDemurrage(t *testing.T) {
	withChunkSize(t, 2)
	cs, _ := newUBITestState(6, 60)
	ctx := context.Background()

	if _, done, err := cs.distributeUBIChunkLocked(ctx); err != nil || done {
		t.Fatalf("expected an open epoch (err=%v done=%v)", err, done)
	}
	const arriving = 33.0
	pool, _ := cs.accounts.Get(ubiPoolAddr)
	pool.Balance = pool.Balance.Add(NewDecimal(arriving))

	for {
		_, complete, err := cs.distributeUBIChunkLocked(ctx)
		if err != nil {
			t.Fatalf("chunk failed: %v", err)
		}
		if complete {
			break
		}
	}

	pool, _ = cs.accounts.Get(ubiPoolAddr)
	got := pool.Balance.Float()
	if got < arriving-0.001 || got > arriving+0.001 {
		t.Fatalf("pool holds %.6f, expected the %.2f that arrived mid-epoch — zeroing the pool would silently destroy demurrage collected while the distribution ran", got, arriving)
	}
}

// An empty pool or no humans must finish immediately rather than opening an
// epoch that never closes and blocks every later distribution.
func TestUBIChunking_NothingToDoCompletesImmediately(t *testing.T) {
	ctx := context.Background()

	empty, _ := newUBITestState(5, 0)
	if shares, done, err := empty.distributeUBIChunkLocked(ctx); err != nil || !done || len(shares) != 0 {
		t.Fatalf("an empty pool must complete at once with no credits (shares=%d done=%v err=%v)", len(shares), done, err)
	}

	noHumans := newTestState()
	noHumans.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100)})
	if shares, done, err := noHumans.distributeUBIChunkLocked(ctx); err != nil || !done || len(shares) != 0 {
		t.Fatalf("no humans must complete at once with no credits (shares=%d done=%v err=%v)", len(shares), done, err)
	}
}

// Chunking stays off no matter what the activation height says, and this test
// pins that rather than the "activates at height N" behaviour it replaced.
//
// The reason is not in this file. Every distribution round emits a
// distribution_round_marker carrying the round's timestamp, and
// distributionRoundToSkip (block.go) discards an ENTIRE round whose marker
// falls within 24h of the last one already applied — the guard added on
// 2026-08-16 after a proven double-credit.
//
// Chunks of one round share that timestamp. TestUBIChunking_ChunkedRoundsAre
// SkippedByTheReplayGuard below measures what follows: the first chunk sets
// last_distribution_round_at, and every later chunk is then seen by every
// secondary as a duplicate of it and dropped whole. The distributing node
// would credit everyone; the others would credit the first chunk only.
//
// So this switch may not do what its name promises until the idempotency
// anchor identifies the CHUNK rather than the round. Until then, setting it
// says so — which is already an improvement on what it did before, which was
// nothing at all: the chunked functions in this file were never called from
// anywhere in the tree.
func TestUBIChunking_StaysOffEvenWhenAnActivationHeightIsSet(t *testing.T) {
	prev := ubiChunkingActivationHeight
	t.Cleanup(func() { ubiChunkingActivationHeight = prev })

	ubiChunkingActivationHeight = 0
	for _, h := range []int64{0, 1, 1 << 40} {
		if UBIChunkingActive(h) {
			t.Fatalf("chunking must stay off at height %d while no activation height is configured", h)
		}
	}

	ubiChunkingActivationHeight = 2_000_000
	for _, h := range []int64{1_999_999, 2_000_000, 1 << 40} {
		if UBIChunkingActive(h) {
			t.Fatalf("chunking activated at height %d — every chunk after the first would be "+
				"dropped on every other node, and the ledgers would part company", h)
		}
	}
}

// The measurement the comment above rests on. Kept as a test so the day
// somebody fixes the anchor, this fails and tells them the guard can be lifted.
func TestUBIChunking_ChunkedRoundsAreSkippedByTheReplayGuard(t *testing.T) {
	const roundAt = int64(1_700_000_000)

	// A second chunk of the same round, arriving after the first was applied.
	secondChunk := []Transaction{
		{Type: "ubi_distribution", Wallet: "0xabc", Amount: 5},
		{Type: "distribution_round_marker", DistributionAt: roundAt},
	}

	skip := distributionRoundToSkip(secondChunk, roundAt, 0)
	if skip == 0 {
		t.Fatal("the replay guard no longer drops a second chunk of the same round — " +
			"if that is because the anchor now identifies the chunk, UBIChunkingActive's " +
			"hard-coded false can go, and this test with it")
	}
	if skip != roundAt {
		t.Fatalf("expected the guard to skip round %d, got %d", roundAt, skip)
	}
}
