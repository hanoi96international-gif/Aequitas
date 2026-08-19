package keeper

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestSaveBlockCostAtScale measures where the wall-clock time actually goes
// when ProduceBlock saves a FULL block (maxTxsPerBlock transactions) against
// a real Postgres — the single dominant cost behind STAGING_RUNBOOK.md's
// "~1.8s for ProduceBlock, SaveBlockWithPendingTxsAtomic 822ms of it"
// finding, which is currently the binding constraint on the 50,000 TPS
// target (one full block per ~2s is ~25,000 TPS, not 50,000).
//
// Unlike TestBlockCostAtScale (block_size_bench_test.go), which measures the
// pure CPU serialization costs (calculateBlockHash / json.Marshal /
// json.Unmarshal) with no database at all, this measures the DB round trips
// that dominate at 50,000 rows, and breaks them into the three statements
// SaveBlockWithPendingTxsAtomic actually issues so the expensive one is
// identifiable rather than guessed at:
//
//  1. UPDATE pending_txs SET included_block_hash = ... WHERE id = ANY(...)
//  2. INSERT INTO chain_blocks (... an ~11.6 MB transactions JSON blob ...)
//  3. DELETE FROM pending_txs WHERE id = ANY(...)
//
// Statements 1 and 3 touch the SAME 50,000 rows inside the SAME transaction,
// and in Postgres an UPDATE writes an entirely new row version (MVCC) — so
// statement 1 rewrites 50,000 rows that statement 3 then deletes before any
// other transaction could ever observe them.
//
// Opt-in, same convention as every other real-DB benchmark in this package:
// AEQUITAS_BLOCK_SAVE_BENCH=1 plus AEQUITAS_TPS_BENCH=1 and a disposable
// DATABASE_URL.
func TestSaveBlockCostAtScale(t *testing.T) {
	if os.Getenv("AEQUITAS_BLOCK_SAVE_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in only: set AEQUITAS_BLOCK_SAVE_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	// Deliberately NOT truncateDistTestTables: that helper runs before
	// NewChainState by design (see its own comment), but this benchmark only
	// touches pending_txs/chain_blocks and needs the schema to exist first —
	// NewChainState is what creates it. Clearing pending_txs per sub-test
	// below is the only isolation this benchmark actually needs.
	cs := NewChainState("unused-block-save-bench.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}
	// The three GHOSTDAG columns live behind their own one-time migration
	// (ensureGHOSTDAGColumns), not NewChainState's base table creation, and the
	// INSERT below writes all three — so on a fresh database the migration has
	// to run first, exactly as it does in production before the first block
	// save. Idempotent (ALTER ... IF NOT EXISTS + a sync.Once).
	cs.ensureGHOSTDAGColumns()

	for _, n := range []int{1000, 10000, maxTxsPerBlock} {
		t.Run(fmt.Sprintf("txs=%d", n), func(t *testing.T) {
			if _, err := cs.db.Exec(`DELETE FROM pending_txs`); err != nil {
				t.Fatalf("clear pending_txs: %v", err)
			}

			// Seed n pending rows exactly as the real ingestion path would,
			// then read them back through LoadPendingTxs so the measured ids
			// and Transactions are the genuine article, not synthetic stand-ins.
			seedStart := time.Now()
			for i := 0; i < n; i++ {
				txJSON, _ := json.Marshal(Transaction{
					Type:   "transfer",
					Wallet: fmt.Sprintf("0x%040x", i),
					To:     fmt.Sprintf("0x%040x", i+1_000_000),
					Amount: 1.25,
					TxHash: fmt.Sprintf("0x%064x", i),
				})
				if _, err := cs.db.Exec(
					`INSERT INTO pending_txs (tx_json, included_at) VALUES ($1, 0)`, string(txJSON),
				); err != nil {
					t.Fatalf("seed pending_tx %d: %v", i, err)
				}
			}
			seedDur := time.Since(seedStart)

			loadStart := time.Now()
			txs, ids := cs.LoadPendingTxs()
			loadDur := time.Since(loadStart)
			if len(ids) != n {
				t.Fatalf("LoadPendingTxs returned %d ids, want %d", len(ids), n)
			}

			block := &Block{
				Hash:         fmt.Sprintf("bench-%d-%d", n, time.Now().UnixNano()),
				Height:       int64(900000 + n),
				ParentHashes: []string{"bench-parent"},
				Proposer:     "0xbenchproposer",
				Timestamp:    time.Now().Unix(),
				Transactions: txs,
			}

			// Per-statement breakdown, issued against the same DB in the same
			// order and shape SaveBlockWithPendingTxsAtomic uses, so the
			// numbers attribute directly to its three statements.
			txsJSON, _ := json.Marshal(block.Transactions)
			parentsJSON, _ := json.Marshal(block.ParentHashes)

			dbtx, err := cs.db.Begin()
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			// The included_block_hash UPDATE is measured under an opt-in flag of
			// its own so this benchmark can show BOTH the pre-fix cost (set
			// AEQUITAS_BLOCK_SAVE_BENCH_LEGACY_UPDATE=1) and the shipped path
			// (default) from the same code, instead of the "after" number
			// relying on a since-deleted statement nobody can re-run. See
			// SaveBlockWithPendingTxsAtomic's own comment for why the statement
			// was removable at all.
			var updDur time.Duration
			if os.Getenv("AEQUITAS_BLOCK_SAVE_BENCH_LEGACY_UPDATE") == "1" {
				updStart := time.Now()
				if _, err := dbtx.Exec(
					`UPDATE pending_txs SET included_block_hash = $1 WHERE id = ANY($2)`, block.Hash, ids,
				); err != nil {
					dbtx.Rollback()
					t.Fatalf("update: %v", err)
				}
				updDur = time.Since(updStart)
			}

			insStart := time.Now()
			if _, err := dbtx.Exec(
				`INSERT INTO chain_blocks
				   (hash, height, parent_hashes, proposer, timestamp, humans, state_root,
				    signature, transactions, selected_parent, blue_score, blues)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				 ON CONFLICT (hash) DO NOTHING`,
				block.Hash, block.Height, string(parentsJSON), block.Proposer, block.Timestamp,
				0, "", "", string(txsJSON), "", 0, "[]",
			); err != nil {
				dbtx.Rollback()
				t.Fatalf("insert: %v", err)
			}
			insDur := time.Since(insStart)

			delStart := time.Now()
			if _, err := dbtx.Exec(`DELETE FROM pending_txs WHERE id = ANY($1)`, ids); err != nil {
				dbtx.Rollback()
				t.Fatalf("delete: %v", err)
			}
			delDur := time.Since(delStart)

			commitStart := time.Now()
			if err := dbtx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			commitDur := time.Since(commitStart)

			total := updDur + insDur + delDur + commitDur
			mode := "shipped (no included_block_hash UPDATE)"
			if updDur > 0 {
				mode = fmt.Sprintf("legacy (UPDATE %.1f%% of save)", 100*float64(updDur)/float64(total))
			}
			t.Logf("txs=%-6d payload=%.2fMB | seed=%s load=%s || UPDATE=%s INSERT=%s DELETE=%s COMMIT=%s | save-total=%s | %s",
				n, float64(len(txsJSON))/(1024*1024), seedDur.Round(time.Millisecond), loadDur.Round(time.Millisecond),
				updDur.Round(time.Millisecond), insDur.Round(time.Millisecond),
				delDur.Round(time.Millisecond), commitDur.Round(time.Millisecond),
				total.Round(time.Millisecond), mode)
		})
	}
}
