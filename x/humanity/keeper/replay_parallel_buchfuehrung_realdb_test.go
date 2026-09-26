package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// Gegenstueck zu TestParallelesNachspielen_FuehrtBuchWieSeriell gegen ein
// echtes Postgres: der parallele Pfad schreibt die Buchkonten ab der
// Aktivierung gesammelt in dieselbe Transaktion wie die Konten
// (replay_parallel.go, Phase 2b). Nach dem Block muss wirtschaft_buch genau
// dem Stand im Speicher entsprechen -- sonst kennt ein Knoten nach einem
// Neustart andere Freibetraege als vorher.
func TestParallelesNachspielen_BuchfuehrungInDB_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-replay-parallel-buch-realdb-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}
	for _, q := range []string{`TRUNCATE wirtschaft_buch`, `TRUNCATE wirtschaft_unternehmen`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	cs.wirt().mu.Lock()
	cs.wirt().buch = map[string]*buchKonto{}
	cs.wirt().unternehmen = map[string]*unternehmenEintrag{}
	cs.wirt().mu.Unlock()

	const paare = 12
	var txs []Transaction
	var alle []string
	ctx := context.Background()
	cs.mu.Lock()
	for i := 0; i < paare; i++ {
		mensch := distTestAddr(700 + 2*i)
		firma := distTestAddr(701 + 2*i)
		inhaber := distTestAddr(900 + i)
		for _, a := range []*AccountState{
			{Address: mensch, Balance: NewDecimal(2000), IsHuman: true},
			{Address: inhaber, Balance: NewDecimal(2000), IsHuman: true},
			{Address: firma, Balance: NewDecimal(3000)},
		} {
			cs.accounts.Set(a.Address, a)
			if err := cs.saveAccountToDB(a); err != nil {
				cs.mu.Unlock()
				t.Fatalf("seed %s: %v", a.Address, err)
			}
		}
		if err := cs.applyUnternehmenEroeffnenLocked(ctx, firma, inhaber, fmt.Sprintf("Firma %d", i), "handel", jetzt); err != nil {
			cs.mu.Unlock()
			t.Fatalf("eroeffnen %s: %v", firma, err)
		}
		// Einkauf: zaehlt als Umsatz der Firma und als Ausgabe des Menschen.
		txs = append(txs, Transaction{Type: "transfer", Wallet: mensch, To: firma, Amount: 120 + float64(i), BuchAt: jetzt})
		alle = append(alle, mensch, firma)
	}
	cs.mu.Unlock()

	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != len(txs) {
		t.Fatalf("nur %d von %d Ueberweisungen buendelbar -- der Test prueft dann nicht den parallelen Pfad", len(batch), len(txs))
	}

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	block := &Block{Height: 1, Hash: "replay-parallel-buch-realdb-1", Timestamp: jetzt, Transactions: txs}
	if ok := dag.replayTransactions(block, false); !ok {
		t.Fatal("replayTransactions lehnte einen gueltigen Block ab")
	}

	speicher := buchSchnappschuss(t, cs, alle)
	if len(speicher) != len(alle) {
		t.Fatalf("Buchfuehrung im Speicher: %d von %d Konten -- der parallele Pfad hat nicht gebucht", len(speicher), len(alle))
	}
	for _, a := range alle {
		var daten string
		if err := cs.db.QueryRow(`SELECT daten FROM wirtschaft_buch WHERE address = $1`, a).Scan(&daten); err != nil {
			t.Fatalf("wirtschaft_buch %s: %v", a, err)
		}
		// Ueber json normalisieren: Postgres gibt TEXT unveraendert zurueck,
		// aber so bleibt der Vergleich unabhaengig von der Feldreihenfolge.
		var db, mem map[string]interface{}
		if err := json.Unmarshal([]byte(daten), &db); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(speicher[a]), &mem); err != nil {
			t.Fatal(err)
		}
		dbJ, _ := json.Marshal(db)
		memJ, _ := json.Marshal(mem)
		if string(dbJ) != string(memJ) {
			t.Fatalf("Buchkonto %s: Datenbank und Speicher weichen ab\n  db:       %s\n  speicher: %s", a, dbJ, memJ)
		}
	}
}
