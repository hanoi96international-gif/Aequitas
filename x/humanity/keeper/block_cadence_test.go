package keeper

import "testing"

func fullTxSet() []Transaction {
	return make([]Transaction, maxTxsPerBlock)
}

func notFullTxSet() []Transaction {
	return make([]Transaction, maxTxsPerBlock-1)
}

func TestProduceBlocksForTick_NilFirstBlockReturnsEmpty(t *testing.T) {
	calls := 0
	produce := func() *Block {
		calls++
		return nil
	}
	got := produceBlocksForTick(produce)
	if len(got) != 0 {
		t.Errorf("want 0 blocks, got %d", len(got))
	}
	if calls != 1 {
		t.Errorf("want produce called exactly once (the catch-up-gate case), got %d", calls)
	}
}

func TestProduceBlocksForTick_NotFullSingleBlockStopsAfterOne(t *testing.T) {
	calls := 0
	produce := func() *Block {
		calls++
		return &Block{Height: int64(calls), Transactions: notFullTxSet()}
	}
	got := produceBlocksForTick(produce)
	if len(got) != 1 {
		t.Fatalf("want 1 block (the common, non-backlogged case), got %d", len(got))
	}
	if calls != 1 {
		t.Errorf("want produce called exactly once -- a not-full first block must not trigger any extra production, got %d calls", calls)
	}
}

func TestProduceBlocksForTick_FullBlocksProduceUpToCap(t *testing.T) {
	calls := 0
	produce := func() *Block {
		calls++
		return &Block{Height: int64(calls), Transactions: fullTxSet()} // always full -- unbounded backlog
	}
	got := produceBlocksForTick(produce)
	want := 1 + maxExtraBlocksPerTick
	if len(got) != want {
		t.Fatalf("want %d blocks (1 normal + %d extra cap), got %d", want, maxExtraBlocksPerTick, len(got))
	}
	if calls != want {
		t.Errorf("want produce called exactly %d times, got %d -- the cap must bound real calls, not just the returned slice", want, calls)
	}
	// Heights must be sequential 1..want, in order -- each extra block's
	// parent-chaining correctness depends on ProduceBlock (untouched by
	// this function) always picking up the CURRENT tips, but this at least
	// proves this function returns them in the order produced, not
	// reordered or duplicated.
	for i, b := range got {
		if b.Height != int64(i+1) {
			t.Errorf("block %d: Height = %d, want %d (returned out of order)", i, b.Height, i+1)
		}
	}
}

func TestProduceBlocksForTick_StopsWhenBacklogDrains(t *testing.T) {
	calls := 0
	produce := func() *Block {
		calls++
		if calls <= 2 {
			return &Block{Height: int64(calls), Transactions: fullTxSet()}
		}
		return &Block{Height: int64(calls), Transactions: notFullTxSet()} // backlog now drained
	}
	got := produceBlocksForTick(produce)
	if len(got) != 3 {
		t.Fatalf("want 3 blocks (2 full + 1 not-full that stops the loop), got %d", len(got))
	}
	if calls != 3 {
		t.Errorf("want produce called exactly 3 times -- must not keep going past a drained backlog, got %d", calls)
	}
}

func TestProduceBlocksForTick_StopsCleanlyOnNilDuringExtraProduction(t *testing.T) {
	calls := 0
	produce := func() *Block {
		calls++
		if calls == 1 {
			return &Block{Height: 1, Transactions: fullTxSet()} // looks backlogged...
		}
		return nil // ...but the second call hits some other gate (catch-up, degraded, etc.)
	}
	got := produceBlocksForTick(produce)
	if len(got) != 1 {
		t.Fatalf("want 1 block (the first, successful one), got %d", len(got))
	}
	if calls != 2 {
		t.Errorf("want produce called exactly twice (first success, second nil that stops the loop), got %d", calls)
	}
}
