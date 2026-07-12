package main

// One-off local utility: SAFELY backfills chain_blocks rows that exist in
// OLD_DATABASE_URL (the frozen, pre-cutover DB) but are missing from
// NEW_DATABASE_URL (the live, now-primary DB) -- e.g. the small handful of
// blocks that landed in the old DB in the last moments before the
// DATABASE_URL cutover, after the last full-table sync ran. Unlike
// cmd/dbmigrate, this NEVER deletes or overwrites anything already in the
// destination -- it only INSERTs rows whose hash isn't already there, so it
// is safe to run against a DB that has since produced thousands of its own
// new blocks.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	_ "github.com/lib/pq"
)

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
	// The frozen old DB is external to this Railway project (not a tracked
	// service), and the Aequitas service's own DATABASE_URL now points at
	// the new DB post-cutover -- so its connection string can no longer be
	// fetched via `railway variables`. Passed explicitly instead.
	oldURL := os.Getenv("FROZEN_OLD_DATABASE_URL")
	if oldURL == "" {
		fmt.Println("set FROZEN_OLD_DATABASE_URL to the frozen pre-cutover connection string")
		os.Exit(1)
	}
	newURL, err := railwayServiceVar("Postgres", "DATABASE_PUBLIC_URL")
	if err != nil {
		fmt.Printf("✗ could not fetch destination DATABASE_PUBLIC_URL: %v\n", err)
		os.Exit(1)
	}

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

	var oldCount, newCountBefore int
	oldDB.QueryRow(`SELECT COUNT(*) FROM chain_blocks`).Scan(&oldCount)
	newDB.QueryRow(`SELECT COUNT(*) FROM chain_blocks`).Scan(&newCountBefore)
	fmt.Printf("Before: OLD chain_blocks=%d, NEW chain_blocks=%d\n", oldCount, newCountBefore)

	// Fetch every hash NEW already has in ONE bulk query -- checking 237k
	// hashes one round trip at a time (the previous version of this script)
	// would take hours over a ~250ms-latency link. A single SELECT + an
	// in-memory set lookup does the same job in one round trip total.
	fmt.Println("Fetching existing hashes from NEW (one bulk query)...")
	existing := make(map[string]bool, newCountBefore)
	newHashRows, err := newDB.Query(`SELECT hash FROM chain_blocks`)
	if err != nil {
		panic(err)
	}
	for newHashRows.Next() {
		var h string
		if err := newHashRows.Scan(&h); err != nil {
			panic(err)
		}
		existing[h] = true
	}
	newHashRows.Close()
	fmt.Printf("NEW already has %d distinct hashes.\n", len(existing))

	fmt.Println("Scanning OLD for rows missing from NEW...")
	rows, err := oldDB.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	type missingRow struct {
		hash, parentHashesRaw, proposer, stateRoot, signature, txsRaw, selectedParent, bluesRaw string
		height, timestamp, blueScore                                                            int64
		humans                                                                                   int
	}
	var missing []missingRow
	checked := 0
	for rows.Next() {
		var r missingRow
		if err := rows.Scan(&r.hash, &r.height, &r.parentHashesRaw, &r.proposer, &r.timestamp, &r.humans, &r.stateRoot,
			&r.signature, &r.txsRaw, &r.selectedParent, &r.blueScore, &r.bluesRaw); err != nil {
			fmt.Printf("  scan error: %v\n", err)
			continue
		}
		checked++
		if !existing[r.hash] {
			missing = append(missing, r)
		}
	}
	rows.Close()
	fmt.Printf("Checked %d source rows, %d are missing from NEW.\n", checked, len(missing))

	if len(missing) == 0 {
		fmt.Println("Nothing to backfill.")
		return
	}

	insertStmt, err := newDB.Prepare(`INSERT INTO chain_blocks
		(hash, height, parent_hashes, proposer, timestamp, humans, state_root, signature, transactions, selected_parent, blue_score, blues)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (hash) DO NOTHING`)
	if err != nil {
		panic(err)
	}
	defer insertStmt.Close()

	inserted := 0
	var missingHeights []int64
	for _, r := range missing {
		if _, err := insertStmt.Exec(r.hash, r.height, r.parentHashesRaw, r.proposer, r.timestamp, r.humans, r.stateRoot,
			r.signature, r.txsRaw, r.selectedParent, r.blueScore, r.bluesRaw); err != nil {
			fmt.Printf("  ✗ insert failed for hash %s height %d: %v\n", r.hash, r.height, err)
			continue
		}
		inserted++
		missingHeights = append(missingHeights, r.height)
	}

	var newCountAfter int
	newDB.QueryRow(`SELECT COUNT(*) FROM chain_blocks`).Scan(&newCountAfter)
	fmt.Printf("Inserted %d missing rows.\n", inserted)
	fmt.Printf("After: NEW chain_blocks=%d\n", newCountAfter)
	if len(missingHeights) > 0 {
		fmt.Printf("Backfilled heights: %v\n", strings.Trim(fmt.Sprint(missingHeights), "[]"))
	}
}
