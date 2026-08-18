package keeper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
)

// TestNegativeBigIntCannotBeRLPEncoded pins the constraint the clamp exists
// for. Without this test the guard in aeqToWei looks like defensive noise
// somebody could tidy away; with it, removing the guard fails here first and
// explains itself.
//
// This is not hypothetical. Contabo2, 2026-08-18: the V7 contract deploy
// aborted at every single boot with
//
//	[DEPLOY] ERROR: failed to deploy V7 contract: EVM panic: rlp: cannot
//	         encode negative big.Int — restoring previous V7 state
//
// so the EVM mirror was never rebuilt, every block from then on produced a
// StateRoot the primary disagreed with, and the node's own auto-heal spent
// the day resyncing against a cause no resync could reach.
func TestNegativeBigIntCannotBeRLPEncoded(t *testing.T) {
	if _, err := rlp.EncodeToBytes(big.NewInt(-1)); err == nil {
		t.Fatal("expected RLP to refuse a negative big.Int; if this ever " +
			"starts succeeding, the clamp in aeqToWei can be reconsidered — " +
			"until then it is the only thing keeping one bad account from " +
			"making the whole EVM state uncommittable")
	}
	if _, err := rlp.EncodeToBytes(big.NewInt(0)); err != nil {
		t.Fatalf("zero must encode; got %v", err)
	}
}

// TestAeqToWeiNeverReturnsNegative covers the last-resort guard: whatever
// reaches aeqToWei, what comes out must be RLP-encodable.
func TestAeqToWeiNeverReturnsNegative(t *testing.T) {
	for _, aeq := range []float64{-0.000001, -1, -1000, -1e9} {
		got := aeqToWei(aeq)
		if got.Sign() < 0 {
			t.Errorf("aeqToWei(%v) = %s — negative values must be clamped", aeq, got)
		}
		if _, err := rlp.EncodeToBytes(got); err != nil {
			t.Errorf("aeqToWei(%v) produced something RLP cannot encode: %v", aeq, err)
		}
	}

	// The clamp must not touch anything legitimate: a positive balance still
	// converts at 1e18 wei per AEQ.
	if got, want := aeqToWei(1), new(big.Int).Set(weiPerAEQ); got.Cmp(want) != 0 {
		t.Errorf("aeqToWei(1) = %s, want %s", got, want)
	}
	if got := aeqToWei(0); got.Sign() != 0 {
		t.Errorf("aeqToWei(0) = %s, want 0", got)
	}
}

// TestEvmBalanceWeiClampsAndKeeps checks the mirror-loading wrapper, which is
// where the account address is still known and therefore the only place that
// can name the offender in the log.
func TestEvmBalanceWeiClampsAndKeeps(t *testing.T) {
	if got := evmBalanceWei("0xdeadbeef", -42); got.Sign() != 0 {
		t.Errorf("evmBalanceWei(negative) = %s, want 0", got)
	}
	if got, want := evmBalanceWei("0xdeadbeef", 2), new(big.Int).Mul(big.NewInt(2), weiPerAEQ); got.Cmp(want) != 0 {
		t.Errorf("evmBalanceWei(2) = %s, want %s", got, want)
	}
}
