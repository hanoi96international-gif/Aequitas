package keeper

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every table this package reads or writes must also be created by it.
//
// bio_hashes was not, and nothing caught it: SaveBioHash's INSERT failed on
// every registration and only logged a warning, CountChainBioHashes turned the
// same error into a plain 0 — indistinguishable from an empty table — and a
// comment in state.go asserted the table was "created lazily by SaveBioHash",
// which it never did. The result was a month of /api/health/combined reporting
// chain_bio_hashes: 0 against 15 humans and 15 nullifiers, with one of the
// three duplicate-biometric checks inert the whole time, on both validators.
//
// No database is needed to catch that: the CREATE and the use are both in this
// package's own source. This test reads it.

// tablesCreatedElsewhere are the tables this package legitimately uses without
// a literal CREATE TABLE of its own. Every entry needs a reason; an entry with
// no reason is how bio_hashes would come back.
var tablesCreatedElsewhere = map[string]string{
	// Created lazily, immediately before first use, by
	// SavePreUpgradeRelationshipSlots — via fmt.Sprintf with the table name as
	// a constant, so the CREATE is real but not a literal this test can see.
	"evm_upgrade_relationship_slots": "created in SavePreUpgradeRelationshipSlots via fmt.Sprintf",
}

func TestEverySQLTableUsedIsAlsoCreated(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("could not read package directory: %v", err)
	}
	var source strings.Builder
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(".", n))
		if err != nil {
			t.Fatalf("could not read %s: %v", n, err)
		}
		source.Write(b)
		source.WriteString("\n")
	}
	src := source.String()

	// Go string literals only — comments in this codebase are long and prose-y,
	// and matching them would drown the result in false positives.
	lits := regexp.MustCompile("`[^`]*`").FindAllString(src, -1)
	lits = append(lits, regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`).FindAllString(src, -1)...)

	sqlish := regexp.MustCompile(`(?i)\b(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|TRUNCATE)\b`)
	created := map[string]bool{}
	used := map[string][]string{} // table -> the statements that touch it
	cteNames := map[string]bool{} // WITH x AS (...) aliases look exactly like tables

	reCreate := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-z_][a-z0-9_]*)`)
	reCTE := regexp.MustCompile(`(?i)(?:WITH|,)\s+([a-z_][a-z0-9_]*)\s+AS\s*\(`)
	refPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)`),
		regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([a-z_][a-z0-9_]*)`),
		regexp.MustCompile(`(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET\b`),
		regexp.MustCompile(`(?i)\bTRUNCATE\s+(?:TABLE\s+)?([a-z_][a-z0-9_]*)`),
		regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+([a-z_][a-z0-9_]*)`),
		regexp.MustCompile(`(?i)\bFROM\s+([a-z_][a-z0-9_]*)`),
		regexp.MustCompile(`(?i)\bJOIN\s+([a-z_][a-z0-9_]*)`),
	}

	for _, lit := range lits {
		if !sqlish.MatchString(lit) {
			continue
		}
		for _, m := range reCreate.FindAllStringSubmatch(lit, -1) {
			created[strings.ToLower(m[1])] = true
		}
		for _, m := range reCTE.FindAllStringSubmatch(lit, -1) {
			cteNames[strings.ToLower(m[1])] = true
		}
		for _, re := range refPatterns {
			for _, m := range re.FindAllStringSubmatch(lit, -1) {
				name := strings.ToLower(m[1])
				snippet := strings.Join(strings.Fields(lit), " ")
				if len(snippet) > 90 {
					snippet = snippet[:90] + "…"
				}
				used[name] = append(used[name], snippet)
			}
		}
	}

	if len(created) < 10 {
		t.Fatalf("only found %d CREATE TABLE statements — the scan is broken, not the schema", len(created))
	}

	// SQL keywords that the (case-insensitive) patterns above can capture in
	// place of a name — e.g. `TRUNCATE TABLE %s`, where the real name is a
	// format verb and "TABLE" is what the name group ends up matching.
	sqlKeywords := map[string]bool{"table": true, "only": true, "select": true, "lateral": true}

	var missing []string
	for name := range used {
		if created[name] || cteNames[name] || tablesCreatedElsewhere[name] != "" || sqlKeywords[name] {
			continue
		}
		// A bare SELECT ... FROM (subquery) or a function call reads as a name
		// here; require the token to be used somewhere as a write target too,
		// which every real table in this package is.
		writes := false
		for _, s := range used[name] {
			u := strings.ToUpper(s)
			if strings.Contains(u, "INSERT INTO "+strings.ToUpper(name)) ||
				strings.Contains(u, "DELETE FROM "+strings.ToUpper(name)) ||
				strings.Contains(u, "UPDATE "+strings.ToUpper(name)) ||
				strings.Contains(u, "TRUNCATE "+strings.ToUpper(name)) ||
				strings.Contains(u, "ALTER TABLE "+strings.ToUpper(name)) {
				writes = true
				break
			}
		}
		if writes {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	for _, name := range missing {
		t.Errorf("table %q is written by this package but never created by it.\n"+
			"  Either add a CREATE TABLE IF NOT EXISTS to initDBTables, or add it to\n"+
			"  tablesCreatedElsewhere WITH A REASON. Statements touching it:\n    %s",
			name, strings.Join(uniqueStrings(used[name]), "\n    "))
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}
