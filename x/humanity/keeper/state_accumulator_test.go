package keeper

import (
	"context"
	"testing"
)

// newAccumTestState builds a no-DB ChainState with the nullifier map
// initialized (newTestState omits it). In no-DB mode the maps are the
// authoritative source, so the incremental accumulators must equal a full
// recompute over those maps at every step.
func newAccumTestState() *ChainState {
	return &ChainState{
		accounts:   newShardedAccounts(),
		pool:       &PoolState{},
		nullifiers: make(map[string]string),
		useDB:      false,
	}
}

// referenceAccountXOR recomputes the account accumulator from scratch by
// XORing every current account's leaf — the O(N) definition the incremental
// accountSetXOR must always equal.
func referenceAccountXOR(cs *ChainState) [32]byte {
	var x [32]byte
	cs.accounts.Range(func(_ string, a *AccountState) bool {
		xorInto(&x, accountLeaf(a))
		return true
	})
	return x
}

func referenceNullifierXOR(cs *ChainState) [32]byte {
	var x [32]byte
	for k := range cs.nullifiers {
		xorInto(&x, nullifierLeaf(k))
	}
	return x
}

// TestStateRootAccumulator_MatchesFullRecompute drives a sequence of account
// and nullifier mutations through the same choke points production uses
// (saveAccountToDB, tryClaimNullifierLocked, SaveNullifier,
// releaseNullifierLocked) and asserts the O(1) accumulators equal the O(N)
// full recompute after every step — including inclusion toggling (an account
// dropping to zero balance leaves the root) and nullifier release.
func TestStateRootAccumulator_MatchesFullRecompute(t *testing.T) {
	cs := newAccumTestState()

	assertMatch := func(label string) {
		t.Helper()
		if cs.accountSetXOR != referenceAccountXOR(cs) {
			t.Fatalf("%s: accountSetXOR diverged from full recompute", label)
		}
		if cs.nullifierSetXOR != referenceNullifierXOR(cs) {
			t.Fatalf("%s: nullifierSetXOR diverged from full recompute", label)
		}
	}

	assertMatch("empty")

	// Add a human.
	a := &AccountState{Address: "0xaaa", Balance: NewDecimal(1000), IsHuman: true}
	cs.accounts.Set("0xaaa", a)
	if err := cs.saveAccountToDB(a); err != nil {
		t.Fatal(err)
	}
	assertMatch("add 0xaaa")

	// Mutate its balance repeatedly (each save must swap leaves cleanly).
	for _, bal := range []float64{500, 750, 12.345678, 1000} {
		a.Balance = NewDecimal(bal)
		if err := cs.saveAccountToDB(a); err != nil {
			t.Fatal(err)
		}
		assertMatch("mutate 0xaaa balance")
	}

	// Add a non-human with only tUSD + LP (still included via the !=0 rule).
	b := &AccountState{Address: "0xbbb", TUsdBalance: NewDecimal(10), LPShares: NewDecimal(5)}
	cs.accounts.Set("0xbbb", b)
	if err := cs.saveAccountToDB(b); err != nil {
		t.Fatal(err)
	}
	assertMatch("add 0xbbb")

	// Toggle 0xbbb OUT of the root (all balances zero, not human).
	b.TUsdBalance = NewDecimal(0)
	b.LPShares = NewDecimal(0)
	if err := cs.saveAccountToDB(b); err != nil {
		t.Fatal(err)
	}
	assertMatch("0xbbb excluded")
	// Its leaf must contribute nothing now.
	if b.leafHash != ([32]byte{}) {
		t.Fatal("excluded account should cache the zero leaf")
	}

	// Toggle it back IN.
	b.Balance = NewDecimal(42)
	if err := cs.saveAccountToDB(b); err != nil {
		t.Fatal(err)
	}
	assertMatch("0xbbb re-included")

	// Nullifiers: claim, save (distinct key), release.
	if _, err := cs.tryClaimNullifierLocked("null-1", "0xaaa"); err != nil {
		t.Fatal(err)
	}
	assertMatch("claim null-1")
	if err := cs.SaveNullifier(context.Background(), "null-2", "0xbbb"); err != nil {
		t.Fatal(err)
	}
	assertMatch("save null-2")
	// Claiming an already-present key must be a no-op for the accumulator.
	if _, err := cs.tryClaimNullifierLocked("null-1", "0xaaa"); err != nil {
		t.Fatal(err)
	}
	assertMatch("re-claim null-1 (no-op)")
	cs.releaseNullifierLocked("null-1")
	assertMatch("release null-1")

	// StateRoot must be deterministic and non-empty.
	r1 := cs.stateRootLocked("123")
	r2 := cs.stateRootLocked("123")
	if r1 != r2 || r1 == "" {
		t.Fatalf("StateRoot not deterministic/non-empty: %q vs %q", r1, r2)
	}
	// Changing a balance must change the root.
	a.Balance = NewDecimal(999)
	_ = cs.saveAccountToDB(a)
	if cs.stateRootLocked("123") == r1 {
		t.Fatal("StateRoot unchanged after a balance change")
	}
}

// TestStateRootAccumulator_OrderIndependence proves two states that reach the
// same account/nullifier SET by different mutation orders produce the same
// accumulators (hence the same root) — the property that lets independent
// nodes agree.
func TestStateRootAccumulator_OrderIndependence(t *testing.T) {
	build := func(order []string) *ChainState {
		cs := newAccumTestState()
		for _, addr := range order {
			acc := &AccountState{Address: addr, Balance: NewDecimal(100), IsHuman: true}
			cs.accounts.Set(addr, acc)
			_ = cs.saveAccountToDB(acc)
			_, _ = cs.tryClaimNullifierLocked("n-"+addr, addr)
		}
		return cs
	}
	a := build([]string{"0x01", "0x02", "0x03"})
	b := build([]string{"0x03", "0x01", "0x02"})
	if a.accountSetXOR != b.accountSetXOR {
		t.Fatal("accountSetXOR depends on insertion order")
	}
	if a.nullifierSetXOR != b.nullifierSetXOR {
		t.Fatal("nullifierSetXOR depends on insertion order")
	}
	if a.stateRootLocked("0") != b.stateRootLocked("0") {
		t.Fatal("StateRoot depends on insertion order")
	}
}

// TestStateRootAccumulator_Rollback verifies a block rollback restores both
// accumulators to their pre-block values, so a hard-failed block leaves the
// root exactly as it was.
func TestStateRootAccumulator_Rollback(t *testing.T) {
	cs := newAccumTestState()
	a := &AccountState{Address: "0xaaa", Balance: NewDecimal(1000), IsHuman: true}
	cs.accounts.Set("0xaaa", a)
	_ = cs.saveAccountToDB(a)
	_, _ = cs.tryClaimNullifierLocked("n1", "0xaaa")

	preAcc := cs.accountSetXOR
	preNull := cs.nullifierSetXOR
	preRoot := cs.stateRootLocked("0")

	// Snapshot as replayTransactions would: 0xaaa exists, 0xbbb will be created.
	snap := cs.snapshotForRollbackLocked([]string{"0xaaa", "0xbbb"}, false, map[string]configValueSnapshot{})

	// Mutate inside the "block": change a balance, create a new account, claim a
	// nullifier.
	a.Balance = NewDecimal(1)
	_ = cs.saveAccountToDB(a)
	b := &AccountState{Address: "0xbbb", Balance: NewDecimal(777), IsHuman: true}
	cs.accounts.Set("0xbbb", b)
	_ = cs.saveAccountToDB(b)
	_, _ = cs.tryClaimNullifierLocked("n2", "0xbbb")

	if cs.accountSetXOR == preAcc {
		t.Fatal("mutations did not change accountSetXOR (test is not exercising anything)")
	}

	// Roll the block back.
	if err := cs.restoreFromRollbackLocked(snap); err != nil {
		t.Fatal(err)
	}

	if cs.accountSetXOR != preAcc {
		t.Fatal("accountSetXOR not restored after rollback")
	}
	if cs.nullifierSetXOR != preNull {
		t.Fatal("nullifierSetXOR not restored after rollback")
	}
	// Accounts map IS restored, so the account accumulator also equals a fresh
	// recompute (0xbbb removed, 0xaaa back to 1000).
	if cs.accountSetXOR != referenceAccountXOR(cs) {
		t.Fatal("accountSetXOR != full recompute of restored accounts")
	}
	if cs.stateRootLocked("0") != preRoot {
		t.Fatal("StateRoot not restored after rollback")
	}
}
