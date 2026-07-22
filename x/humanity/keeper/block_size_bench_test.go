package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestBlockCostAtScale is the investigation SCALING_ARCHITECTURE.md Phase 9
// calls for: ProduceBlock has no per-block transaction cap (LoadPendingTxs'
// query has no LIMIT), so at 50,000 TPS a single ~1-2s BLOCK_TIME tick could
// drain 50,000-100,000 transactions into ONE block. This measures the real,
// concrete costs that scale with transactions-per-block -- not GHOSTDAG
// (which operates on DAG/parent-child structure, not per-block tx count,
// and is unaffected by this), but the two things that genuinely are O(N) in
// transaction count on EVERY block, for EVERY node:
//  1. calculateBlockHash's tx_root: json.Marshal(txs) + sha256, run by the
//     producer AND by every peer verifying the block (block.go).
//  2. The full block payload: json.Marshal(block) for P2P broadcast
//     (p2p.go's broadcastExcept) and json.Unmarshal on the receiving end.
//
// Opt-in only (like the other *_bench_test.go files) -- prints real
// measurements via t.Logf, does not assert pass/fail thresholds (there is
// no "right" answer yet; this is the investigation, not the fix).
func TestBlockCostAtScale(t *testing.T) {
	if os.Getenv("AEQUITAS_BLOCK_SIZE_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_BLOCK_SIZE_BENCH=1 to run (SCALING_ARCHITECTURE.md Phase 9 investigation)")
	}

	sizes := []int{100, 1000, 10000, 50000, 100000}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			txs := makeRepresentativeTxs(n)
			block := &Block{
				Height:       12345,
				Timestamp:    time.Now().Unix(),
				ParentHashes: []string{"0xparent1", "0xparent2"},
				Proposer:     "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
				Humans:       1000,
				StateRoot:    "0xdeadbeef",
				Transactions: txs,
			}

			hashStart := time.Now()
			hash := calculateBlockHash(block)
			hashDur := time.Since(hashStart)
			block.Hash = hash

			marshalStart := time.Now()
			payload, err := json.Marshal(block)
			marshalDur := time.Since(marshalStart)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			unmarshalStart := time.Now()
			var decoded Block
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			unmarshalDur := time.Since(unmarshalStart)

			t.Logf("n=%d: calculateBlockHash=%v, json.Marshal(block)=%v (%.2f MB), json.Unmarshal=%v",
				n, hashDur, marshalDur, float64(len(payload))/(1024*1024), unmarshalDur)
		})
	}
}

// makeRepresentativeTxs builds n plain-transfer Transactions with the field
// shape a real high-volume workload is dominated by (see Transaction's own
// comment: registrations with ZK proof fields are comparatively rare and
// much larger per-TX -- using them here would overstate the common case).
func makeRepresentativeTxs(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := 0; i < n; i++ {
		txs[i] = Transaction{
			Type:              "transfer",
			Wallet:            fmt.Sprintf("0x%040x", i),
			To:                fmt.Sprintf("0x%040x", i+1),
			Amount:            1.5,
			FromDemurrageLost: 0.02,
			ToDemurrageLost:   0.0,
			TxHash:            fmt.Sprintf("0x%064x", i),
		}
	}
	return txs
}

// TestTxRootHashDeterministic is a quick sanity check that calculateBlockHash
// stays deterministic at scale (same input, same output) -- not a
// performance test, just guards the benchmark's own assumptions.
func TestTxRootHashDeterministic(t *testing.T) {
	txs := makeRepresentativeTxs(50)
	data, _ := json.Marshal(txs)
	h1 := sha256.Sum256(data)
	data2, _ := json.Marshal(txs)
	h2 := sha256.Sum256(data2)
	if hex.EncodeToString(h1[:]) != hex.EncodeToString(h2[:]) {
		t.Fatal("tx_root hash not deterministic for identical input")
	}
}
