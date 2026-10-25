package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// BenchmarkTagesrundeDB misst eine ganze Tagesrunde mit echter Datenbank, mit
// den heutigen Regeln und mit dem Gleichgewicht (wirtschaft3.go):
//
//   - erzeugen: RunDailyDistributionAtomic auf dem Leiter. Die Runde haelt
//     cs.mu die ganze Zeit -- so lange steht der Knoten.
//   - nachspielen: dieselben Transaktionen auf einem zweiten Knoten mit
//     eigener Datenbank, in Bloecken zu maxTxsPerBlock wie im Betrieb.
//
// Am Ende muessen beide Knoten dieselben Kontostaende haben.
//
//	AEQUITAS_TPS_BENCH=1 DATABASE_URL=postgres://.../irgendeine \
//	  AEQUITAS_RUNDE_KONTEN=20000 \
//	  go test ./x/humanity/keeper/ -run '^$' -bench BenchmarkTagesrundeDB -benchtime 1x
func BenchmarkTagesrundeDB(b *testing.B) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		b.Skip("opt-in: AEQUITAS_TPS_BENCH=1 und DATABASE_URL (wegwerfbarer Postgres-Server)")
	}
	n := 20_000
	if v, err := strconv.Atoi(os.Getenv("AEQUITAS_RUNDE_KONTEN")); err == nil && v > 0 {
		n = v
	}
	for _, gg := range []bool{false, true} {
		name := "heute"
		if gg {
			name = "gleichgewicht"
		}
		b.Run(fmt.Sprintf("%s/%d", name, n), func(b *testing.B) { tagesrundeMessen(b, n, gg) })
	}
}

// rundeKnoten: ein Knoten mit eigener frischer Datenbank und n Menschen
// (1.000 AEQ, jeder zehnte 8.000 -- heute zahlen nur die Umlauf).
func rundeKnoten(b *testing.B, basis, name string, n int) *ChainState {
	b.Helper()
	b.Setenv("DATABASE_URL", frischeDatenbank(b, basis, name))
	cs := testKnoten(b, "unused-tagesrunde-bench.json")
	if !cs.useDB {
		b.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()
	konten := make([]*AccountState, 0, 1000)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := 0; i < n; i++ {
		bal := 1000.0
		if i%10 == 0 {
			bal = 8000
		}
		acc := &AccountState{Address: distTestAddr(500_000 + i), IsHuman: true, Balance: NewDecimal(bal)}
		cs.accounts.Set(acc.Address, acc)
		cs.humanCount++
		konten = append(konten, acc)
		if len(konten) == cap(konten) || i == n-1 {
			if err := cs.saveAccountsToDBBatchCtx(ctx, konten); err != nil {
				b.Fatal(err)
			}
			konten = konten[:0]
		}
	}
	cs.rebuildStateAccumulators()
	return cs
}

func tagesrundeMessen(b *testing.B, n int, gg bool) {
	basis := os.Getenv("DATABASE_URL")
	wirtschaftAktivOverride.Store(1)
	b.Cleanup(func() { wirtschaftAktivOverride.Store(1<<63 - 1) })
	if gg {
		gleichgewichtAktivOverride.Store(1)
		b.Cleanup(func() { gleichgewichtAktivOverride.Store(0) })
	}
	lauf := time.Now().UnixNano() % 1_000_000_000
	erzeuger := rundeKnoten(b, basis, fmt.Sprintf("tagesrunde_%d_a", lauf), n)
	nach := rundeKnoten(b, basis, fmt.Sprintf("tagesrunde_%d_b", lauf), n)
	b.Setenv("DATABASE_URL", basis)
	dag := newOrphanTestDAG()
	dag.state = nach
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	at := nowUnix()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rundeAt := at + int64(i)*86400
		start := time.Now()
		if err := erzeuger.RunDailyDistributionAtomic(rundeAt); err != nil {
			b.Fatal(err)
		}
		erzeugen := time.Since(start)

		rows, err := erzeuger.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY wal_seq, id`)
		if err != nil {
			b.Fatal(err)
		}
		var txs []Transaction
		for rows.Next() {
			var roh string
			var tx Transaction
			if err := rows.Scan(&roh); err != nil {
				b.Fatal(err)
			}
			if err := json.Unmarshal([]byte(roh), &tx); err != nil {
				b.Fatal(err)
			}
			txs = append(txs, tx)
		}
		rows.Close()
		if _, err := erzeuger.db.Exec(`UPDATE pending_txs SET included_at = 1 WHERE included_at = 0`); err != nil {
			b.Fatal(err)
		}

		start = time.Now()
		for j, h := 0, 0; j < len(txs); j += maxTxsPerBlock {
			ende := j + maxTxsPerBlock
			if ende > len(txs) {
				ende = len(txs)
			}
			h++
			blk := erhaltungBlock(i*1000+h, rundeAt+5, txs[j:ende]...)
			if !dag.replayTransactions(blk, true) {
				b.Fatalf("Block %d der Runde abgelehnt", h)
			}
		}
		nachspielen := time.Since(start)
		b.ReportMetric(erzeugen.Seconds(), "s/erzeugen")
		b.ReportMetric(nachspielen.Seconds(), "s/nachspielen")
		b.ReportMetric(float64(len(txs)), "txs")
	}
	b.StopTimer()
	for i := 0; i < n; i += 997 {
		a := distTestAddr(500_000 + i)
		if acct(nach, a).Balance != acct(erzeuger, a).Balance {
			b.Fatalf("%s: nachgespielt %v, Erzeuger %v", a, acct(nach, a).Balance, acct(erzeuger, a).Balance)
		}
	}
	if acct(nach, ubiPoolAddr).Balance != acct(erzeuger, ubiPoolAddr).Balance {
		b.Fatalf("Topf: nachgespielt %v, Erzeuger %v", acct(nach, ubiPoolAddr).Balance, acct(erzeuger, ubiPoolAddr).Balance)
	}
}

// Die Reihenfolge im Ausgang ist die im Block. savePendingTxsGeordnetExec
// muss sie ueber mehrere Stuecke hinweg genau einhalten.
func TestAusgangGeordnet_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-ausgang-geordnet-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	n := 2*ausgangStueck + 7
	txs := make([]Transaction, n)
	for i := range txs {
		// Absichtlich nicht alphabetisch sortiert: die Reihenfolge der
		// Eingabe zaehlt, nicht der Inhalt.
		txs[i] = Transaction{Type: "umlauf", Wallet: distTestAddr((i * 7919) % n), Amount: float64(i + 1)}
	}
	dbTx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := savePendingTxsGeordnetExec(dbTx, txs); err != nil {
		dbTx.Rollback()
		t.Fatal(err)
	}
	if err := dbTx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY wal_seq, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var roh string
		var tx Transaction
		if err := rows.Scan(&roh); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(roh), &tx); err != nil {
			t.Fatal(err)
		}
		if i >= n || tx.Amount != txs[i].Amount || tx.Wallet != txs[i].Wallet {
			t.Fatalf("Zeile %d: %+v, erwartet %+v", i, tx, txs[min(i, n-1)])
		}
		i++
	}
	if i != n {
		t.Fatalf("%d Zeilen, erwartet %d", i, n)
	}
}
