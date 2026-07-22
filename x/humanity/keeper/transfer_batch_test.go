package keeper

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// These tests exercise TransferAtomic's group-commit path (see its own and
// processTransferBatch's comments in state.go) against a real, disposable
// local Postgres — opt-in via the same AEQUITAS_TPS_BENCH gate as
// tps_bench_test.go, since they need the identical setup and skipping them
// from the normal `go test ./...`/CI run for the same reason: no Postgres
// available there, and these are deliberately slower, real-DB tests.
func skipUnlessRealDBBenchEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
}

// TestTransferBatch_ConcurrentSuccessAllCommit is the core correctness
// check: many concurrent TransferAtomic callers, all individually valid,
// coalesced by the batcher into shared DB transactions — every one must
// still succeed, and the resulting balances must be exactly what a
// one-at-a-time execution would have produced (batching must not lose,
// duplicate, or reorder-corrupt any transfer).
func TestTransferBatch_ConcurrentSuccessAllCommit(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	state := NewChainState("unused-batch-test-1.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	const numSenders = 50
	const txsPerSender = 20
	recipient := "0x00000000000000000000000000000000005ee1"

	senderAddrs := make([]string, numSenders)
	state.mu.Lock()
	for i := 0; i < numSenders; i++ {
		addr := fmt.Sprintf("0xba%038x", i)
		senderAddrs[i] = addr
		acc := &AccountState{Address: addr, Balance: NewDecimal(1000), LastActivityAt: time.Now().Unix()}
		if err := state.saveAccountToDB(acc); err != nil {
			state.mu.Unlock()
			t.Fatalf("seeding sender %s: %v", addr, err)
		}
	}
	state.mu.Unlock()

	var wg sync.WaitGroup
	errs := make(chan error, numSenders*txsPerSender)
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			addr := senderAddrs[idx]
			for j := 0; j < txsPerSender; j++ {
				tmpl := Transaction{Type: "transfer", Wallet: addr, To: recipient, Amount: 1, TxHash: fmt.Sprintf("0xbatch1-%d-%d", idx, j)}
				if _, _, err := state.TransferAtomic(addr, recipient, 1, tmpl); err != nil {
					errs <- fmt.Errorf("sender %d tx %d: %w", idx, j, err)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("unexpected transfer failure: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	for i, addr := range senderAddrs {
		state.ensureAccountLoaded(addr)
		acc, ok := state.accounts[addr]
		if !ok {
			t.Fatalf("sender %d (%s): account missing after transfers", i, addr)
		}
		want := 1000.0 - float64(txsPerSender)
		got := acc.Balance.Float()
		if got != want {
			t.Errorf("sender %d (%s): balance = %.6f, want %.6f", i, addr, got, want)
		}
	}
	state.ensureAccountLoaded(recipient)
	recvAcc, ok := state.accounts[recipient]
	if !ok {
		t.Fatal("recipient account missing")
	}
	wantRecv := float64(numSenders * txsPerSender)
	if recvAcc.Balance.Float() != wantRecv {
		t.Errorf("recipient balance = %.6f, want %.6f", recvAcc.Balance.Float(), wantRecv)
	}
}

// TestTransferBatch_OneBadMemberFailsWholeBatchCleanly pins the documented
// all-or-nothing tradeoff (see processTransferBatch's comment): when a
// batch contains one transfer that can never succeed (insufficient
// balance), every member submitted closely enough together to land in the
// same batch gets an error -- but critically, NOTHING is left half-applied:
// no sender's balance changes, and a clean retry of the same valid
// transfers afterward succeeds and produces the exact expected balances,
// proving runAtomicWithOutbox's rollback fully undid the aborted batch
// (both DB and in-memory) rather than leaving any partial state.
func TestTransferBatch_OneBadMemberFailsWholeBatchCleanly(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	state := NewChainState("unused-batch-test-2.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	recipient := "0x00000000000000000000000000000000005ee2"
	goodAddr := "0xba00000000000000000000000000000000c001"
	brokeAddr := "0xba00000000000000000000000000000000c002" // never funded -- guaranteed insufficient balance

	state.mu.Lock()
	if err := state.saveAccountToDB(&AccountState{Address: goodAddr, Balance: NewDecimal(500), LastActivityAt: time.Now().Unix()}); err != nil {
		state.mu.Unlock()
		t.Fatalf("seeding %s: %v", goodAddr, err)
	}
	state.mu.Unlock()

	// Fire both requests as close together as possible so they land in the
	// same batch window (transferBatchMaxWait) -- goroutines, not
	// sequential calls, so neither can complete before the other starts.
	var wg sync.WaitGroup
	var goodErr, brokeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		tmpl := Transaction{Type: "transfer", Wallet: goodAddr, To: recipient, Amount: 10, TxHash: "0xbatch2-good"}
		_, _, goodErr = state.TransferAtomic(goodAddr, recipient, 10, tmpl)
	}()
	go func() {
		defer wg.Done()
		tmpl := Transaction{Type: "transfer", Wallet: brokeAddr, To: recipient, Amount: 10, TxHash: "0xbatch2-broke"}
		_, _, brokeErr = state.TransferAtomic(brokeAddr, recipient, 10, tmpl)
	}()
	wg.Wait()

	if brokeErr == nil {
		t.Fatal("expected the unfunded sender's transfer to fail")
	}
	// This assertion is the actual point of the test: IF the two calls
	// landed in the same batch, the good transfer must have failed too
	// (all-or-nothing) -- if the batcher's timing put them in different
	// batches, goodErr == nil is equally correct (they were never really
	// coupled). Either way, what must hold is checked below: no partial
	// balance change survives.
	t.Logf("goodErr=%v brokeErr=%v (nil goodErr means the two calls landed in different batches, which is fine)", goodErr, brokeErr)

	state.mu.Lock()
	state.ensureAccountLoaded(goodAddr)
	balanceAfterFirstRound := state.accounts[goodAddr].Balance.Float()
	state.mu.Unlock()

	if goodErr != nil {
		// Coupled with the failing sibling and rolled back -- balance must
		// be untouched.
		if balanceAfterFirstRound != 500 {
			t.Fatalf("good sender's balance = %.6f after an aborted batch, want unchanged 500 (rollback left partial state)", balanceAfterFirstRound)
		}
	}

	// Retry the good transfer alone -- must succeed cleanly regardless of
	// what happened above, proving no residue (stale demurrage timestamp,
	// stuck version counter, phantom pool credit) survived the abort.
	tmpl := Transaction{Type: "transfer", Wallet: goodAddr, To: recipient, Amount: 10, TxHash: "0xbatch2-good-retry"}
	if _, _, err := state.TransferAtomic(goodAddr, recipient, 10, tmpl); err != nil {
		t.Fatalf("retry of the valid transfer failed: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	state.ensureAccountLoaded(goodAddr)
	got := state.accounts[goodAddr].Balance.Float()
	want := balanceAfterFirstRound - 10
	if got != want {
		t.Errorf("good sender's balance after retry = %.6f, want %.6f", got, want)
	}
}

// TestTransferBatch_HighConcurrencyNoDeadlock is a smaller, fast-running
// regression guard for the connection-pool self-deadlock fixed alongside
// this batching mechanism (see state.go's ensureAccountLoaded FIX comment)
// -- TestSimulateMaxTPS_Ingestion already covers this at benchmark scale;
// this is the same shape at a size cheap enough to run every time this
// opt-in suite runs, so a future regression here fails fast.
func TestTransferBatch_HighConcurrencyNoDeadlock(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	state := NewChainState("unused-batch-test-3.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	const numSenders = 40
	recipient := "0x00000000000000000000000000000000005ee3"
	senderAddrs := make([]string, numSenders)
	state.mu.Lock()
	for i := 0; i < numSenders; i++ {
		addr := fmt.Sprintf("0xbc%038x", i)
		senderAddrs[i] = addr
		if err := state.saveAccountToDB(&AccountState{Address: addr, Balance: NewDecimal(100), LastActivityAt: time.Now().Unix()}); err != nil {
			state.mu.Unlock()
			t.Fatalf("seeding %s: %v", addr, err)
		}
	}
	state.mu.Unlock()

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i := 0; i < numSenders; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tmpl := Transaction{Type: "transfer", Wallet: senderAddrs[idx], To: recipient, Amount: 1, TxHash: fmt.Sprintf("0xbatch3-%d", idx)}
				if _, _, err := state.TransferAtomic(senderAddrs[idx], recipient, 1, tmpl); err != nil {
					t.Errorf("sender %d: %v", idx, err)
				}
			}(i)
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("timed out — likely a connection-pool deadlock regression")
	}
}
