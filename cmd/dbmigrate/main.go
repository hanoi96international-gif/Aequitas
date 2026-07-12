package main

// One-off local utility: copies all data from OLD_DATABASE_URL to
// NEW_DATABASE_URL (both real Postgres instances), reusing the app's own
// schema-init logic (keeper.NewChainState) so the destination gets the
// identical, already-tested table definitions instead of a hand-reconstructed
// DDL guess. Not part of the running node; run once via `go run`, then delete.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
	"github.com/lib/pq"
)

// railwayServiceVar shells out to the already-authenticated `railway` CLI to
// fetch a single variable for a service, so the caller never has to type,
// paste, or persist the raw credential anywhere itself -- it flows straight
// from the CLI's own output into this process's memory only.
func railwayServiceVar(service, key string) (string, error) {
	cmd := exec.Command("railway", "variables", "--service", service, "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("railway variables --service %s: %w", service, err)
	}
	var vars map[string]string
	if err := json.Unmarshal(out, &vars); err != nil {
		return "", fmt.Errorf("parsing railway variables JSON for %s: %w", service, err)
	}
	v, ok := vars[key]
	if !ok || v == "" {
		return "", fmt.Errorf("%s not set on service %s", key, service)
	}
	return v, nil
}

func main() {
	oldURL, err := railwayServiceVar("Aequitas", "DATABASE_URL")
	if err != nil {
		fmt.Printf("✗ could not fetch source DATABASE_URL: %v\n", err)
		os.Exit(1)
	}
	newURL, err := railwayServiceVar("Postgres", "DATABASE_PUBLIC_URL")
	if err != nil {
		fmt.Printf("✗ could not fetch destination DATABASE_PUBLIC_URL: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Step 1: creating schema in destination DB via keeper.NewChainState ===")
	os.Setenv("DATABASE_URL", newURL)
	cs := keeper.NewChainState("unused.json")
	if cs == nil {
		fmt.Println("✗ NewChainState returned nil")
		os.Exit(1)
	}
	// initDB() only runs the base CREATE TABLE statements; the GHOSTDAG
	// columns (selected_parent/blue_score/blues) on chain_blocks are added
	// lazily by ensureGHOSTDAGColumns(), which is unexported and only
	// triggered from inside LoadBlockFromDBByHash and similar DB-read
	// methods. Calling it once (even for a hash that doesn't exist) forces
	// that ALTER TABLE to run before the bulk copy below needs the columns.
	cs.LoadBlockFromDBByHash("0000000000000000000000000000000000000000000000000000000000000000")
	// InitV6StateTables is normally called from evm_engine.go's init path,
	// which NewChainState alone doesn't run -- without this, v6_state/
	// v6_humans/v6_balances/v6_commitments never get created here and every
	// row in them silently fails to copy below.
	cs.InitV6StateTables()
	fmt.Println("✓ schema ready in destination DB (including lazy GHOSTDAG + v6 tables)")

	oldDB, err := sql.Open("postgres", oldURL)
	if err != nil {
		panic(err)
	}
	defer oldDB.Close()
	newDB, err := sql.Open("postgres", newURL)
	if err != nil {
		panic(err)
	}
	defer newDB.Close()

	if err := oldDB.Ping(); err != nil {
		fmt.Printf("✗ cannot reach OLD db: %v\n", err)
		os.Exit(1)
	}
	if err := newDB.Ping(); err != nil {
		fmt.Printf("✗ cannot reach NEW db: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Step 2: copying data table by table ===")
	// Disable FK/trigger checks during bulk load so table insert order
	// doesn't matter, then restore before returning.
	if _, err := newDB.Exec(`SET session_replication_role = replica`); err != nil {
		fmt.Printf("⚠ could not disable trigger/FK checks: %v (continuing anyway)\n", err)
	}
	defer newDB.Exec(`SET session_replication_role = origin`)

	rows, err := oldDB.Query(`SELECT tablename FROM pg_tables WHERE schemaname='public'`)
	if err != nil {
		panic(err)
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			panic(err)
		}
		tables = append(tables, t)
	}
	rows.Close()

	totalRows := 0
	for _, t := range tables {
		n, err := copyTable(oldDB, newDB, t)
		if err != nil {
			fmt.Printf("✗ %-40s FAILED: %v\n", t, err)
			continue
		}
		fmt.Printf("✓ %-40s %d rows\n", t, n)
		totalRows += n
	}
	fmt.Printf("=== Done: %d rows copied across %d tables ===\n", totalRows, len(tables))

	fmt.Println("=== Step 3: verifying critical tables ===")
	for _, t := range []string{"chain_blocks", "chain_accounts", "bio_registrations", "nullifiers", "chain_config", "liquidity_pool"} {
		verifyTable(oldDB, newDB, t)
	}
}

func verifyTable(oldDB, newDB *sql.DB, table string) {
	var oldCount, newCount int
	if err := oldDB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)).Scan(&oldCount); err != nil {
		fmt.Printf("  ✗ %s: could not count source rows: %v\n", table, err)
		return
	}
	if err := newDB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)).Scan(&newCount); err != nil {
		fmt.Printf("  ✗ %s: could not count destination rows: %v\n", table, err)
		return
	}
	mark := "✓"
	if oldCount != newCount {
		mark = "⚠ MISMATCH"
	}
	fmt.Printf("  %s %-20s source=%d destination=%d\n", mark, table, oldCount, newCount)
}

// destTableColumns returns table's column names as they exist on db, or an
// error if the table doesn't exist there at all.
func destTableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table does not exist on destination")
	}
	return cols, rows.Err()
}

// copyTable uses Postgres's COPY protocol (via pq.CopyIn) instead of one
// INSERT per row: a per-row round trip over a ~250ms-latency proxy link
// would take hours for a 160k-row table, whereas COPY streams all rows in a
// single continuous exchange.
//
// Only copies columns that exist on BOTH sides (intersection) -- the source
// (long-lived production) DB can have columns added out-of-band over time
// that the current code's CREATE TABLE statement never learned about (e.g.
// gini_snapshots' legacy "ts" column), and a table the destination schema
// doesn't create at all (e.g. a different service's own tables sharing this
// DB) is reported as a clean "does not exist on destination" skip instead of
// a raw SQL error.
func copyTable(oldDB, newDB *sql.DB, table string) (int, error) {
	destCols, err := destTableColumns(newDB, table)
	if err != nil {
		return 0, err
	}
	destColSet := make(map[string]bool, len(destCols))
	for _, c := range destCols {
		destColSet[c] = true
	}

	allRows, err := oldDB.Query(fmt.Sprintf(`SELECT * FROM %q LIMIT 0`, table))
	if err != nil {
		return 0, fmt.Errorf("probe columns: %w", err)
	}
	allCols, err := allRows.Columns()
	allRows.Close()
	if err != nil {
		return 0, fmt.Errorf("probe columns: %w", err)
	}
	var cols []string
	for _, c := range allCols {
		if destColSet[c] {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("no matching columns between source and destination")
	}
	if len(cols) != len(allCols) {
		fmt.Printf("  ⚠ %s: copying %d/%d columns (source has extra columns the current schema doesn't)\n", table, len(cols), len(allCols))
	}

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = fmt.Sprintf("%q", c)
	}
	rows, err := oldDB.Query(fmt.Sprintf(`SELECT %s FROM %q`, strings.Join(quoted, ","), table))
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	defer rows.Close()

	// Destination is a brand-new DB, but make this safely re-runnable.
	if _, err := newDB.Exec(fmt.Sprintf(`DELETE FROM %q`, table)); err != nil {
		return 0, fmt.Errorf("pre-delete: %w", err)
	}

	tx, err := newDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.Prepare(pq.CopyIn(table, cols...))
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("prepare COPY: %w", err)
	}

	vals := make([]interface{}, len(cols))
	valPtrs := make([]interface{}, len(cols))
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	count := 0
	for rows.Next() {
		if err := rows.Scan(valPtrs...); err != nil {
			stmt.Close()
			tx.Rollback()
			return count, fmt.Errorf("scan: %w", err)
		}
		// database/sql has no native decimal type, so NUMERIC/DECIMAL columns
		// scan as []byte holding the plain-text digits (e.g. "0.331094").
		// pq.CopyIn's COPY-protocol encoder treats a raw []byte as bytea and
		// hex-escapes it ("\x30..."), which Postgres then rejects as invalid
		// input for a numeric column -- converting to string first makes it
		// write the text as-is, exactly like a NUMERIC column expects.
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		if _, err := stmt.Exec(vals...); err != nil {
			stmt.Close()
			tx.Rollback()
			return count, fmt.Errorf("COPY row (after %d ok): %w", count, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		stmt.Close()
		tx.Rollback()
		return count, fmt.Errorf("row iteration: %w", err)
	}
	if _, err := stmt.Exec(); err != nil { // flush
		stmt.Close()
		tx.Rollback()
		return count, fmt.Errorf("COPY flush: %w", err)
	}
	if err := stmt.Close(); err != nil {
		tx.Rollback()
		return count, fmt.Errorf("COPY close: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return count, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}
