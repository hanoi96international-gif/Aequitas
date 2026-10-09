package keeper

import (
	"context"
	"strings"
	"testing"
)

// Pruefung von #327, INFO-4: Abfragen auf chain_accounts ueber
// lower(address) lasen die ganze Tabelle -- der Primaerschluessel auf address
// dient dafuer nicht. Der Ausdrucksindex muss sie tragen (mit abgeschaltetem
// Seq Scan zeigt EXPLAIN sonst trotzdem einen).
func TestChainAccounts_IndexAufLowerAddress_RealDB(t *testing.T) {
	cs := testKnoten(t, "unused-konten-index-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	ctx := context.Background()
	conn, err := cs.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, `RESET enable_seqscan`) //nolint:errcheck
	for _, q := range []string{
		`SELECT is_human FROM chain_accounts WHERE lower(address) = '0xabc'`,
		`SELECT address, balance FROM chain_accounts WHERE lower(address) = ANY(ARRAY['0xabc', '0xdef'])`,
		`UPDATE chain_accounts SET version = 1 WHERE lower(address) = '0xabc' AND (version IS NULL OR version = 0)`,
	} {
		rows, err := conn.QueryContext(ctx, `EXPLAIN `+q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		var plan strings.Builder
		for rows.Next() {
			var zeile string
			if err := rows.Scan(&zeile); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(zeile + "\n")
		}
		rows.Close()
		if !strings.Contains(plan.String(), "idx_chain_accounts_lower_address") {
			t.Fatalf("%s nutzt den Index auf lower(address) nicht:\n%s", q, plan.String())
		}
	}
}
