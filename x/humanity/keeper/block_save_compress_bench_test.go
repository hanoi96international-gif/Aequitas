package keeper

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// Does pre-compressing the block payload actually make the INSERT cheaper?
//
// TestSaveBlockCostAtScale established that the INSERT dominates the block save
// and scales with payload bytes — on this hardware 0.21MB/11ms, 2.07MB/66ms,
// 10.35MB/308ms. That save runs while dag.mu is held, so it is the binding
// constraint on block production: under load the chain wrote 9 blocks where 85
// were due.
//
// "Fewer bytes" is the obvious lever, and gzip on JSON this repetitive should
// give a large factor. But Postgres already TOAST-compresses a large text
// value server-side, so it is entirely possible that pre-compressing moves the
// same work around without saving anything, or even costs more. That is a
// measurement, not an argument — and building the storage change first and
// measuring afterwards is how this project has repeatedly wasted a day.
//
// Opt-in, same as its neighbour: needs a DISPOSABLE Postgres.
func TestCompressedPayloadInsertCost(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if os.Getenv("AEQUITAS_BLOCK_SAVE_BENCH") != "1" || dsn == "" {
		t.Skip("opt-in only: set AEQUITAS_BLOCK_SAVE_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS zz_payload_probe (id BIGSERIAL PRIMARY KEY, raw TEXT, z BYTEA)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	defer db.Exec(`DROP TABLE IF EXISTS zz_payload_probe`)

	for _, n := range []int{1000, 10000, 50000} {
		txs := make([]Transaction, n)
		for i := range txs {
			txs[i] = Transaction{
				Type:   "transfer",
				Wallet: fmt.Sprintf("0x%040x", i),
				To:     fmt.Sprintf("0x%040x", i+1),
				Amount: float64(i%1000) + 0.5,
			}
		}
		raw, err := json.Marshal(txs)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var zbuf bytes.Buffer
		zStart := time.Now()
		zw, _ := gzip.NewWriterLevel(&zbuf, gzip.BestSpeed)
		zw.Write(raw)
		zw.Close()
		zDur := time.Since(zStart)

		// Raw INSERT, exactly the shape SaveBlockWithPendingTxsAtomic uses.
		t0 := time.Now()
		if _, err := db.Exec(`INSERT INTO zz_payload_probe (raw) VALUES ($1)`, string(raw)); err != nil {
			t.Fatalf("insert raw: %v", err)
		}
		rawDur := time.Since(t0)

		t1 := time.Now()
		if _, err := db.Exec(`INSERT INTO zz_payload_probe (z) VALUES ($1)`, zbuf.Bytes()); err != nil {
			t.Fatalf("insert compressed: %v", err)
		}
		zInsDur := time.Since(t1)

		t.Logf("txs=%-6d raw=%.2fMB insert=%v || gzip=%.2fMB compress=%v insert=%v || total-compressed=%v (%.0f%% of raw)",
			n,
			float64(len(raw))/(1<<20), rawDur,
			float64(zbuf.Len())/(1<<20), zDur, zInsDur,
			zDur+zInsDur,
			100*float64((zDur+zInsDur).Nanoseconds())/float64(rawDur.Nanoseconds()))
	}
}
