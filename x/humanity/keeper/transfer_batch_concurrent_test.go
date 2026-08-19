package keeper

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// newBatchRequest builds a transferBatchRequest the way TransferAtomic's own
// group-commit path does, for tests that call processTransferBatchConcurrent
// directly instead of going through TransferAtomic — lets these tests target
// specific batch shapes (e.g. forcing a demurrage-eligible member) that would
// be hard to guarantee land in the same real batch via timing alone.
func newBatchRequest(from, to string, amount float64, txHash string) *transferBatchRequest {
	return &transferBatchRequest{
		from: from, to: to, amount: amount,
		pendingTxTemplate: Transaction{Type: "transfer", Wallet: from, To: to, Amount: amount, TxHash: txHash},
		result:            make(chan transferBatchResult, 1),
	}
}

// TestProcessTransferBatchConcurrent_EligibleBatchSucceeds is the basic
// happy path: a batch of several members, all warm, cap-safe, decay-free,
// mutually disjoint addresses — must be handled entirely by the shard-locked
// parallel path (handled=true), with correct balances and outbox rows.
func TestProcessTransferBatchConcurrent_EligibleBatchSucceeds(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 5
	froms := make([]string, n)
	tos := make([]string, n)
	for i := 0; i < n; i++ {
		froms[i] = distTestAddr(500 + i*2)
		tos[i] = distTestAddr(501 + i*2)
		seedConcurrentTestAccount(t, cs, froms[i], 100, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, tos[i], 0, time.Now().Unix())
	}

	batch := make([]*transferBatchRequest, n)
	for i := 0; i < n; i++ {
		batch[i] = newBatchRequest(froms[i], tos[i], 30, fmt.Sprintf("0xpbc-%d", i))
	}

	if !cs.processTransferBatchConcurrent(batch) {
		t.Fatal("want handled=true for a fully eligible, mutually disjoint batch")
	}
	for i, req := range batch {
		res := <-req.result
		if res.err != nil {
			t.Fatalf("member %d: unexpected error: %v", i, res.err)
		}
	}

	cs.mu.RLock()
	for i := 0; i < n; i++ {
		fromAcc, _ := cs.accounts.Get(froms[i])
		toAcc, _ := cs.accounts.Get(tos[i])
		if fromAcc.Balance.Float() != 70 {
			t.Errorf("member %d sender balance = %v, want 70", i, fromAcc.Balance.Float())
		}
		if toAcc.Balance.Float() != 30 {
			t.Errorf("member %d recipient balance = %v, want 30", i, toAcc.Balance.Float())
		}
	}
	gotXOR := cs.accountSetXOR
	want := referenceAccountXOR(cs)
	cs.mu.RUnlock()
	if gotXOR != want {
		t.Fatal("accountSetXOR diverged from a full recompute after a successful parallel batch")
	}

	for i := 0; i < n; i++ {
		var queued int
		if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, fmt.Sprintf("0xpbc-%d", i)).Scan(&queued); err != nil {
			t.Fatalf("checking outbox for member %d: %v", i, err)
		}
		if queued != 1 {
			t.Errorf("member %d: want exactly 1 outbox row, got %d", i, queued)
		}
	}
}

// TestProcessTransferBatchConcurrent_DisjointBatchesRunConcurrently proves
// the actual point of this mechanism: two batches with completely disjoint
// touched-address sets, submitted to processTransferBatchConcurrent from two
// goroutines at the same time, both succeed — neither's TryLockAddrs call
// ever needs to bail because of the other, since they never touch the same
// shard.
func TestProcessTransferBatchConcurrent_DisjointBatchesRunConcurrently(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 20 // members per batch
	batchA := make([]*transferBatchRequest, n)
	batchB := make([]*transferBatchRequest, n)
	for i := 0; i < n; i++ {
		fa, ta := distTestAddr(600+i*2), distTestAddr(601+i*2)
		fb, tb := distTestAddr(700+i*2), distTestAddr(701+i*2)
		seedConcurrentTestAccount(t, cs, fa, 100, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, ta, 0, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, fb, 100, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, tb, 0, time.Now().Unix())
		batchA[i] = newBatchRequest(fa, ta, 10, fmt.Sprintf("0xpbc-a-%d", i))
		batchB[i] = newBatchRequest(fb, tb, 10, fmt.Sprintf("0xpbc-b-%d", i))
	}

	var wg sync.WaitGroup
	var handledA, handledB bool
	wg.Add(2)
	go func() { defer wg.Done(); handledA = cs.processTransferBatchConcurrent(batchA) }()
	go func() { defer wg.Done(); handledB = cs.processTransferBatchConcurrent(batchB) }()
	wg.Wait()

	if !handledA || !handledB {
		t.Fatalf("want both disjoint batches handled=true, got A=%v B=%v", handledA, handledB)
	}
	for _, batch := range [][]*transferBatchRequest{batchA, batchB} {
		for i, req := range batch {
			if res := <-req.result; res.err != nil {
				t.Fatalf("member %d: unexpected error: %v", i, res.err)
			}
		}
	}

	cs.mu.RLock()
	gotXOR := cs.accountSetXOR
	want := referenceAccountXOR(cs)
	cs.mu.RUnlock()
	if gotXOR != want {
		t.Fatal("accountSetXOR diverged from a full recompute after two concurrent disjoint parallel batches")
	}
}

// TestProcessTransferBatchConcurrent_PoolTouchingMemberFallsBackWhole pins
// the deliberate scope limit documented in processTransferBatchConcurrent's
// own comment: if even ONE member would settle demurrage (and so would need
// to touch a pool address, which this path never locks), the WHOLE batch
// bails — handled=false, nothing touched, not even the OTHER members that
// were individually eligible.
func TestProcessTransferBatchConcurrent_PoolTouchingMemberFallsBackWhole(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	eligibleFrom := distTestAddr(510)
	eligibleTo := distTestAddr(511)
	staleFrom := distTestAddr(512)
	staleTo := distTestAddr(513)
	seedConcurrentTestAccount(t, cs, eligibleFrom, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, eligibleTo, 0, time.Now().Unix())
	// A LastActivityAt far enough in the past that effectiveBalance()
	// differs from the stored Balance -- the same demurrage-eligibility
	// trigger TestTransferConcurrent_DemurrageEligibleFallsBack uses.
	//
	// FIX (Audit 2026-08-18): the balance here used to be 100 AEQ, and an idle
	// clock alone is not enough. Demurrage decays only the EXCESS above the
	// fair share (1000 AEQ — see effectiveBalance), so 100 AEQ decays by
	// nothing no matter how long it sits, effectiveBalance equalled Balance,
	// the batch was genuinely eligible, and this test was asserting a fallback
	// that correctly did not happen. It inherited the mistake from
	// TestTransferConcurrent_DemurrageEligibleFallsBack, which had the same
	// wrong fixture — and neither ever failed, because both are gated on
	// AEQUITAS_TPS_BENCH + DATABASE_URL and nothing in CI supplied either.
	seedConcurrentTestAccount(t, cs, staleFrom, 5000, time.Now().Unix()-int64(demurrageGracePeriodSeconds)-int64(60*60*24*40))
	seedConcurrentTestAccount(t, cs, staleTo, 0, time.Now().Unix())

	batch := []*transferBatchRequest{
		newBatchRequest(eligibleFrom, eligibleTo, 10, "0xpbc-fallback-eligible"),
		newBatchRequest(staleFrom, staleTo, 10, "0xpbc-fallback-stale"),
	}

	if cs.processTransferBatchConcurrent(batch) {
		t.Fatal("want handled=false: one member needs pool-address access, so the whole batch must fall back")
	}

	cs.mu.RLock()
	eligibleFromAcc, _ := cs.accounts.Get(eligibleFrom)
	cs.mu.RUnlock()
	if eligibleFromAcc.Balance.Float() != 100 {
		t.Errorf("eligible member's sender balance = %v, want unchanged 100 (whole batch must bail, not partially apply)", eligibleFromAcc.Balance.Float())
	}
}

// TestProcessTransferBatchConcurrent_OneBadMemberFailsWholeBatchCleanly
// mirrors TestTransferBatch_OneBadMemberFailsWholeBatchCleanly for the
// parallel path specifically: an insufficient-balance member fails every
// member in the same batch, and nothing (DB or in-memory) is left
// half-applied.
func TestProcessTransferBatchConcurrent_OneBadMemberFailsWholeBatchCleanly(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	goodFrom := distTestAddr(520)
	goodTo := distTestAddr(521)
	brokeFrom := distTestAddr(522) // never funded -- guaranteed insufficient balance
	brokeTo := distTestAddr(523)
	seedConcurrentTestAccount(t, cs, goodFrom, 500, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, goodTo, 0, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, brokeFrom, 0, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, brokeTo, 0, time.Now().Unix())

	batch := []*transferBatchRequest{
		newBatchRequest(goodFrom, goodTo, 10, "0xpbc-badmember-good"),
		newBatchRequest(brokeFrom, brokeTo, 10, "0xpbc-badmember-broke"),
	}

	if !cs.processTransferBatchConcurrent(batch) {
		t.Fatal("want handled=true: this is a real, decided failure (insufficient balance), not an eligibility bail-out")
	}
	if res := <-batch[0].result; res.err == nil {
		t.Error("want the good member to ALSO fail (all-or-nothing), got nil error")
	}
	if res := <-batch[1].result; res.err == nil {
		t.Error("want an error for the broke member")
	}

	cs.mu.RLock()
	goodFromAcc, _ := cs.accounts.Get(goodFrom)
	cs.mu.RUnlock()
	if goodFromAcc.Balance.Float() != 500 {
		t.Errorf("good member's sender balance = %v, want unchanged 500 (rollback must leave nothing half-applied)", goodFromAcc.Balance.Float())
	}
}

// TestProcessTransferBatchConcurrent_AsymmetricVersionConflictRevertsAll is
// TestTransferConcurrent_AsymmetricVersionConflictRevertsBothAccounts's
// counterpart for a real batch (more than 2 accounts): forces a version
// conflict for exactly ONE address out of several, so
// saveAccountsToDBBatchCtx's internal loop folds SOME accounts' leaves into
// accountSetXOR before discovering the conflict — accountSetXOR must still
// end up exactly as if the whole batch had never been attempted.
func TestProcessTransferBatchConcurrent_AsymmetricVersionConflictRevertsAll(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 4
	froms := make([]string, n)
	tos := make([]string, n)
	for i := 0; i < n; i++ {
		froms[i] = distTestAddr(530 + i*2)
		tos[i] = distTestAddr(531 + i*2)
		seedConcurrentTestAccount(t, cs, froms[i], 100, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, tos[i], 0, time.Now().Unix())
	}

	cs.mu.RLock()
	wantXORBefore := referenceAccountXOR(cs)
	cs.mu.RUnlock()

	// Bump exactly one recipient's DB-row version out from under the
	// upcoming call -- its in-memory scratch copy will still expect the
	// old version, so saveAccountsToDBBatchCtx's UPDATE won't match it,
	// while every OTHER account's row (untouched) matches fine.
	conflictAddr := tos[2]
	if _, err := cs.db.Exec(`UPDATE chain_accounts SET version = version + 1 WHERE lower(address) = $1`, conflictAddr); err != nil {
		t.Fatalf("forcing version conflict for %s: %v", conflictAddr, err)
	}

	batch := make([]*transferBatchRequest, n)
	for i := 0; i < n; i++ {
		batch[i] = newBatchRequest(froms[i], tos[i], 20, fmt.Sprintf("0xpbc-conflict-%d", i))
	}

	if !cs.processTransferBatchConcurrent(batch) {
		t.Fatal("want handled=true: this is a real DB-level failure, not an eligibility bail-out")
	}
	for i, req := range batch {
		if res := <-req.result; res.err == nil {
			t.Errorf("member %d: want an error (one account in this batch had a forced version conflict)", i)
		}
	}

	cs.mu.RLock()
	for i := 0; i < n; i++ {
		fromAcc, _ := cs.accounts.Get(froms[i])
		toAcc, _ := cs.accounts.Get(tos[i])
		if fromAcc.Balance.Float() != 100 {
			t.Errorf("member %d sender balance = %v, want unchanged 100", i, fromAcc.Balance.Float())
		}
		if toAcc.Balance.Float() != 0 {
			t.Errorf("member %d recipient balance = %v, want unchanged 0", i, toAcc.Balance.Float())
		}
	}
	gotXOR := cs.accountSetXOR
	wantXORAfter := referenceAccountXOR(cs)
	cs.mu.RUnlock()
	if gotXOR != wantXORAfter {
		t.Fatal("accountSetXOR diverged from a full recompute after an asymmetric version conflict in a parallel batch")
	}
	if gotXOR != wantXORBefore {
		t.Fatal("accountSetXOR was not fully reverted to its pre-call value after an asymmetric version conflict in a parallel batch")
	}
}

// TestTransferAtomic_MixedContendedAndDisjointTrafficNoDeadlockNoCorruption
// is the chaos test for the whole dispatch mechanism wired into
// TransferAtomic (runTransferBatcher's own parallel-dispatch + fallback
// logic, not processTransferBatchConcurrent called directly): a real mix of
// contended (many senders paying a handful of shared, semi-hot recipients --
// exactly the shape that must always fall back to serialized processing,
// since a hot address can never be parallelized against itself) and
// disjoint (ring topology, the shape this whole mechanism exists to speed
// up) transfers, all fired through TransferAtomic concurrently. What must
// hold regardless of which path any given transfer took: -race clean, never
// deadlock, exact balance conservation.
func TestTransferAtomic_MixedContendedAndDisjointTrafficNoDeadlockNoCorruption(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const numHotRecipients = 3
	const sendersPerHotRecipient = 15
	const numRingSenders = 30
	const txsPerSender = 8

	hotRecipients := make([]string, numHotRecipients)
	for h := 0; h < numHotRecipients; h++ {
		hotRecipients[h] = distTestAddr(800 + h)
		seedConcurrentTestAccount(t, cs, hotRecipients[h], 0, time.Now().Unix())
	}
	hotSenders := make([]string, numHotRecipients*sendersPerHotRecipient)
	for i := range hotSenders {
		hotSenders[i] = distTestAddr(900 + i)
		seedConcurrentTestAccount(t, cs, hotSenders[i], 1000, time.Now().Unix())
	}
	ringAddrs := make([]string, numRingSenders)
	for i := range ringAddrs {
		ringAddrs[i] = distTestAddr(1100 + i)
		seedConcurrentTestAccount(t, cs, ringAddrs[i], 1000, time.Now().Unix())
	}

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i, sender := range hotSenders {
			wg.Add(1)
			go func(idx int, from string) {
				defer wg.Done()
				to := hotRecipients[idx%numHotRecipients]
				for j := 0; j < txsPerSender; j++ {
					tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: fmt.Sprintf("0xchaos-hot-%d-%d", idx, j)}
					if _, _, err := cs.TransferAtomic(from, to, 1, tmpl); err != nil {
						t.Errorf("hot sender %d tx %d: %v", idx, j, err)
					}
				}
			}(i, sender)
		}
		for i, from := range ringAddrs {
			wg.Add(1)
			go func(idx int, from string) {
				defer wg.Done()
				to := ringAddrs[(idx+1)%numRingSenders]
				for j := 0; j < txsPerSender; j++ {
					tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: fmt.Sprintf("0xchaos-ring-%d-%d", idx, j)}
					if _, _, err := cs.TransferAtomic(from, to, 1, tmpl); err != nil {
						t.Errorf("ring sender %d tx %d: %v", idx, j, err)
					}
				}
			}(i, from)
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out — likely a deadlock in the parallel batch dispatch mechanism")
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for h, addr := range hotRecipients {
		acc, _ := cs.accounts.Get(addr)
		want := float64(sendersPerHotRecipient * txsPerSender)
		if acc.Balance.Float() != want {
			t.Errorf("hot recipient %d balance = %v, want %v", h, acc.Balance.Float(), want)
		}
	}
	for _, addr := range hotSenders {
		acc, _ := cs.accounts.Get(addr)
		want := 1000.0 - float64(txsPerSender)
		if acc.Balance.Float() != want {
			t.Errorf("hot sender %s balance = %v, want %v", addr, acc.Balance.Float(), want)
		}
	}
	// Ring topology is net-zero per address regardless of exact per-sender
	// values (each sends txsPerSender out and receives however many its
	// own predecessor sent it) -- total across the ring must still equal
	// the seeded total, proving no value was created or destroyed.
	var total float64
	for _, addr := range ringAddrs {
		acc, _ := cs.accounts.Get(addr)
		total += acc.Balance.Float()
	}
	wantTotal := 1000.0 * float64(numRingSenders)
	if total != wantTotal {
		t.Errorf("ring total balance = %v, want %v (value created or destroyed)", total, wantTotal)
	}

	gotXOR := cs.accountSetXOR
	want := referenceAccountXOR(cs)
	if gotXOR != want {
		t.Fatal("accountSetXOR diverged from a full recompute after mixed contended/disjoint chaos traffic")
	}
}
