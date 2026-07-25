package keeper

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// schema_index_coverage_test.go pins the fix for what turned out to be the
// single largest cost on the block-replay path: every account lookup in this
// codebase is spelled `WHERE lower(address) = $1`, and a b-tree on the bare
// column cannot serve that predicate. Postgres has no way to know lower() is
// order-preserving, so it seq-scans the whole table for each lookup — a cost
// that grows linearly with the account count and was invisible in every
// existing benchmark because they all ran against small tables.
//
// Two indexes in initDB already used the expression form
// (idx_nullifiers_wallet, idx_bio_registrations_wallet), so the technique was
// known; the hot tables simply never got it. This test makes the omission
// impossible to repeat: it reads the package's own SQL, finds every
// `lower(col)` predicate, and requires initDB to declare a matching
// expression index.
//
// It is a pure source-level check — no database needed — so it runs on every
// ordinary `go test ./...`, including in CI where no Postgres exists. The
// real EXPLAIN-level proof that the indexes are actually USED lives in
// TestSchemaIndexes_AccountLookupUsesIndex below, which is opt-in on a real
// database.

// sqlLiteral extracts Go raw-string literals, which is how every SQL
// statement in this package is written. Matching against raw source instead
// would also match English prose in the comments (an early version of this
// test happily reported "a.address" and "go.address" as missing indexes),
// so the scan is deliberately restricted to actual query text.
var sqlLiteral = regexp.MustCompile("`([^`]*)`")

// lowerPredicate matches `... FROM <table> ... WHERE ... lower(<col>) =` —
// the only shape this codebase uses for case-insensitive lookups.
var lowerPredicate = regexp.MustCompile(`(?is)from\s+([a-z_]+)\s.*?where\s.*?lower\(\s*([a-z_]+)\s*\)\s*(=|in|any)`)

// lowerUpdatePredicate is the same for UPDATE statements.
var lowerUpdatePredicate = regexp.MustCompile(`(?is)update\s+([a-z_]+)\s+set\s.*?where\s.*?lower\(\s*([a-z_]+)\s*\)\s*(=|in|any)`)

// indexOnExpression matches an initDB CREATE INDEX whose column list
// contains lower(col) — anywhere in the list, since a composite index like
// (height, lower(proposer)) serves `height = $1 AND lower(proposer) = $2`
// perfectly well.
var indexOnExpression = regexp.MustCompile(`(?is)create\s+(?:unique\s+)?index\s+if\s+not\s+exists\s+\w+\s+on\s+([a-z_]+)\s*\(([^)]*\([^)]*\)[^)]*)\)`)
var indexedLowerCol = regexp.MustCompile(`(?i)lower\(\s*([a-z_]+)\s*\)`)

func TestSchemaIndexes_EveryLowerLookupHasAnExpressionIndex(t *testing.T) {
	sources := readKeeperSources(t)

	needed := map[string]string{} // "table.column" -> where it was found
	for file, src := range sources {
		for _, lit := range sqlLiteral.FindAllStringSubmatch(src, -1) {
			q := lit[1]
			if !strings.Contains(strings.ToLower(q), "lower(") {
				continue
			}
			for _, re := range []*regexp.Regexp{lowerPredicate, lowerUpdatePredicate} {
				for _, m := range re.FindAllStringSubmatch(q, -1) {
					needed[strings.ToLower(m[1]+"."+m[2])] = file
				}
			}
		}
	}
	// Guard against the gate quietly becoming a no-op: if a refactor changes
	// how SQL is written and the regexes stop matching, an empty `needed` set
	// would make this test pass forever while the seq scans came back.
	for _, mustFind := range []string{"chain_accounts.address", "evm_storage.address", "guardians.wallet_address"} {
		if _, ok := needed[mustFind]; !ok {
			t.Fatalf("the scan no longer finds the known `lower(%s)` lookup — the regexes have\n"+
				"drifted away from how this package writes SQL, which would turn this gate into a\n"+
				"no-op instead of failing loudly. Fix the extraction, do not delete this check.",
				strings.SplitN(mustFind, ".", 2)[1])
		}
	}

	have := map[string]bool{}
	for _, src := range sources {
		for _, lit := range sqlLiteral.FindAllStringSubmatch(src, -1) {
			for _, m := range indexOnExpression.FindAllStringSubmatch(lit[1], -1) {
				table := strings.ToLower(m[1])
				for _, c := range indexedLowerCol.FindAllStringSubmatch(m[2], -1) {
					have[table+"."+strings.ToLower(c[1])] = true
				}
			}
		}
	}

	var missing []string
	for key, file := range needed {
		if !have[key] {
			missing = append(missing, key+"  (used in "+file+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d case-insensitive lookup(s) have no matching expression index — each one is a\n"+
			"SEQUENTIAL SCAN of the whole table on every call, growing linearly with row count.\n"+
			"Add `CREATE INDEX IF NOT EXISTS ... ON <table>(lower(<col>))` to initDB:\n\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestSchemaIndexes_AccountLookupUsesIndex is the empirical half: it asks
// Postgres itself whether the hottest lookup on the replay path actually uses
// an index. A source-level check can be satisfied by an index that the
// planner then refuses to use (wrong collation, wrong operator class, a
// volatile expression) — only EXPLAIN can rule that out.
func TestSchemaIndexes_AccountLookupUsesIndex(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	cs := NewChainState("")
	if cs.db == nil {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	defer cs.db.Close()

	// Seed enough rows that the planner would genuinely prefer a seq scan if
	// no usable index existed — with a nearly empty table it picks one
	// anyway and the assertion would be vacuous.
	accs := make([]*AccountState, 0, 500)
	cs.mu.Lock()
	for i := 0; i < 500; i++ {
		a := &AccountState{Address: distTestAddr(2_000_000 + i), Balance: NewDecimal(1)}
		cs.accounts.Set(a.Address, a)
		accs = append(accs, a)
	}
	err := cs.saveAccountsToDBBatch(accs)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := cs.db.Exec(`ANALYZE chain_accounts`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	rows, err := cs.db.Query(
		`EXPLAIN SELECT balance, is_human, tusd_balance, lp_shares,
		        COALESCE(last_activity_at,0), COALESCE(version,0)
		 FROM chain_accounts WHERE lower(address) = $1`, distTestAddr(2_000_100))
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		fmt.Fprintln(&plan, line)
	}
	got := plan.String()
	if strings.Contains(got, "Seq Scan") {
		t.Errorf("the hottest lookup on the replay path is still a sequential scan — the\n"+
			"expression index on chain_accounts(lower(address)) is missing or unusable.\n"+
			"Plan was:\n%s", got)
	}
	t.Logf("ensureAccountLoaded plan:\n%s", got)
}

// readKeeperSources returns every non-test source file in this package,
// keyed by base name.
func readKeeperSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no sources read — is this test running outside the package dir?")
	}
	return out
}
