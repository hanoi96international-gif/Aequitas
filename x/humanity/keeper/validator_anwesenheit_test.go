package keeper

import (
	"context"
	"os"
	"testing"
	"time"
)

// Das fairste Geld: Validator-Anteile nach Anwesenheit, nicht nach Bloecken,
// nicht nach Hardware, nicht nach Dienstalter -- und nur an Menschen.
func TestValidatorVerteilung_GleichNachAnwesenheit_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-anwesenheit-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	a, b, c := distTestAddr(40), distTestAddr(41), distTestAddr(42) // c ist kein Mensch
	schluesselA, schluesselC := distTestAddr(50), distTestAddr(52)
	bis := time.Now().Unix()
	seit := bis - anwesenheitsZeitraum
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'anw-%'`) })
	cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'anw-%'`)

	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		// A leitet: drei Bloecke in JEDER Minute des Tages, unter seinem Schluessel.
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'anw-a-' || g, g, '[]', $1, $2 + (g / 3) * 60 FROM generate_series(0, 1440*3 - 1) g`, []interface{}{schluesselA, seit}},
		// B: ein Block je Minute, aber nur den halben Tag; aeltere Eintraege nennen die Wallet.
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'anw-b-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 719) g`, []interface{}{b, seit}},
		// B war gestern schon da -- zaehlt nicht fuer heute.
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'anw-b-alt-' || g, g, '[]', $1, $2 - 3600 + g * 60 FROM generate_series(0, 59) g`, []interface{}{b, seit}},
		// C (kein Mensch) war den ganzen Tag da.
		{`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp)
		  SELECT 'anw-c-' || g, g, '[]', $1, $2 + g * 60 FROM generate_series(0, 1439) g`, []interface{}{schluesselC, seit}},
		{`INSERT INTO registered_nodes (wallet_address, signing_address, blocks_produced) VALUES
		  ($1, $2, 0), ($3, '', 1000000), ($4, $5, 0)`, []interface{}{a, schluesselA, b, c, schluesselC}},
	} {
		if _, err := cs.db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%v", err)
		}
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, acc := range []*AccountState{
		{Address: a, IsHuman: true, LastActivityAt: bis},
		{Address: b, IsHuman: true, LastActivityAt: bis},
		{Address: c, LastActivityAt: bis},
		{Address: validatorsPoolAddr, Balance: NewDecimal(90)},
	} {
		if err := cs.saveAccountToDB(acc); err != nil {
			t.Fatal(err)
		}
		cs.accounts.Set(acc.Address, acc)
	}

	anw := cs.validatorAnwesenheitCtx(context.Background(), []string{a, b, c}, seit, bis)
	if anw[a] != 1440 || anw[b] != 720 || anw[c] != 1440 {
		t.Fatalf("Anwesenheit %v -- erwartet a=1440 (drei Bloecke je Minute zaehlen einmal), b=720 (gestern zaehlt nicht), c=1440", anw)
	}

	shares, err := cs.distributeValidatorsPoolLocked(context.Background(), bis)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, s := range shares {
		got[s.Wallet] = s.Amount
	}
	// A 1440 : B 720 -> 60 : 30. B's Million alter Bloecke zaehlt nicht,
	// A's dreifache Bloecke als Leiter auch nicht; C ist kein Mensch.
	if got[a] != 60 || got[b] != 30 || got[c] != 0 {
		t.Fatalf("Anteile %v -- erwartet a=60, b=30, c=0", got)
	}
}
