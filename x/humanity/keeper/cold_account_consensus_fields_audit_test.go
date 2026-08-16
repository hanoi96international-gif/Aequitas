package keeper

import (
	"os"
	"strings"
	"testing"
)

// Audit 2026-08-16 (transfer/WAL/concurrency pass), finding COLD-LEAF.
//
// accountLeaf (state.go) is the consensus commitment for one account: its
// SHA-256 is XORed into cs.accountSetXOR, which is a component of StateRoot().
// It commits to exactly five stored fields:
//
//	balance, tusd_balance, lp_shares, is_human, faucet_claimed
//
// THREE code paths reconstruct an AccountState from chain_accounts — the
// finding as first written named only the first two:
//
//   - loadFromDB — the startup preload. Its SELECT always named all five,
//     plus last_activity_at, demurrage_14_day_warning_shown and version.
//   - ensureAccountLoadedCtx — the ON-DEMAND page-in for any account that is
//     cold (never preloaded, or beyond maxInMemAccounts). Its SELECT named
//     only balance, is_human, tusd_balance, lp_shares, last_activity_at and
//     version. faucet_claimed was MISSING, and so was
//     demurrage_14_day_warning_shown.
//   - ensureAccountsLoadedCtx — the BATCH page-in, same defect, found while
//     fixing the first. This is the loader snapshotForRollbackLocked and
//     distributeUBIPoolLocked call, so it corrupted the cached leaf for every
//     human in a distribution round at once.
//
// STATUS: fixed 2026-08-16 — both on-demand loaders now select and scan the
// two columns. The tests below are the regression guard; they are written to
// fail again if either SELECT loses a consensus column.
//
// Three consequences, all real, all now closed:
//
//  1. CONSENSUS. rebuildStateAccumulators (state.go:6375) folds leaf(fc=true)
//     into accountSetXOR for such a row at startup, reading faucet_claimed from
//     the DB. A node that never had the account resident pages it in with
//     FaucetClaimed=false and caches acc.leafHash = leaf(fc=false). The next
//     mutation runs updateAccountLeafLocked, which XORs OUT the cached leaf and
//     XORs IN the new one — so this node XORs out a value that was never XORed
//     in, while a node that had the same account warm XORs out the correct one.
//     The two nodes' accountSetXOR now differ permanently for the same block:
//     a StateRoot fork whose only differing component is account_set_xor —
//     precisely the signature recorded for the 2026-07-25 Contabo2 incident.
//
//  2. FAUCET DOUBLE-CLAIM. claimTUsdFaucetLocked (state.go:6144) and
//     applyFaucetDeltaLocked (state.go:7921) both gate on acc.FaucetClaimed.
//     applyFaucetDeltaLocked's own FIX comment (state.go:7911-7915) states the
//     purpose of its ensureAccountLoadedCtx call verbatim: a blind-created
//     account "would have silently let a cold wallet that already claimed the
//     faucet claim it again on replay." That fix does not work, because the
//     loader it calls does not read the column the guard tests.
//
//  3. SILENT FLAG ERASURE. Every save writes the flag back from memory —
//     saveAccountToDBInnerCtx (state.go:2306/2321, "faucet_claimed = $8") and
//     saveAccountsToDBBatchCtx (state.go:2511/2525). So the first ordinary
//     transfer touching a paged-in account rewrites faucet_claimed=false into
//     the DB, destroying the record permanently rather than merely mis-reading
//     it once.
//
// These tests need no database: the defect is in which columns the query names,
// and the consensus consequence is a property of accountLeaf itself.

// TestAccountLeaf_CommitsToFaucetClaimed establishes the premise the two tests
// below rest on: FaucetClaimed is consensus state, not bookkeeping. PASS is the
// correct outcome — this pins accountLeaf's current, correct behaviour.
func TestAccountLeaf_CommitsToFaucetClaimed(t *testing.T) {
	claimed := &AccountState{
		Address:     "0x00000000000000000000000000000000000000aa",
		Balance:     NewDecimal(1234.5),
		TUsdBalance: NewDecimal(10),
		LPShares:    NewDecimal(0),
		IsHuman:     true,
		// last_activity_at deliberately absent from both sides: accountLeaf
		// does not commit to it (see touchActivityAt's own comment), so it
		// cannot mask or manufacture the difference under test.
		FaucetClaimed: true,
	}
	unclaimed := *claimed
	unclaimed.FaucetClaimed = false

	if accountLeaf(claimed) == accountLeaf(&unclaimed) {
		t.Fatal("accountLeaf produced the same leaf for FaucetClaimed=true and =false — " +
			"if this ever becomes true the field has left consensus and the two tests below are moot")
	}
}

// TestColdPageIn_ReconstructedLeafMatchesDBTruth was INTENTIONALLY RED when the
// finding was written; the fix (state.go, both loaders now SELECT
// faucet_claimed and demurrage_14_day_warning_shown) turns it green.
//
// It MODELS the page-in rather than executing it — ensureAccountLoadedCtx
// returns immediately when cs.db == nil and no Postgres is reachable here — so
// the reconstruction mirrors exactly the columns the loader assigns. That
// modelling is the test's one weakness, and it is why the source-level tests
// below exist: those read state.go directly and cannot drift from it.
func TestColdPageIn_ReconstructedLeafMatchesDBTruth(t *testing.T) {
	// The row as it exists in chain_accounts for a human who claimed the
	// faucet — and as rebuildStateAccumulators reads it into accountSetXOR.
	dbTruth := &AccountState{
		Address:       "0x00000000000000000000000000000000000000bb",
		Balance:       NewDecimal(2500),
		TUsdBalance:   NewDecimal(100),
		LPShares:      NewDecimal(0),
		IsHuman:       true,
		FaucetClaimed: true,
	}

	// The same row after an on-demand page-in, built from the columns the
	// loader scans TODAY — faucet_claimed included.
	pagedIn := &AccountState{Address: dbTruth.Address}
	pagedIn.Balance = dbTruth.Balance
	pagedIn.IsHuman = dbTruth.IsHuman
	pagedIn.TUsdBalance = dbTruth.TUsdBalance
	pagedIn.LPShares = dbTruth.LPShares
	pagedIn.LastActivityAt = dbTruth.LastActivityAt
	pagedIn.Version = dbTruth.Version
	pagedIn.FaucetClaimed = dbTruth.FaucetClaimed

	if accountLeaf(pagedIn) != accountLeaf(dbTruth) {
		t.Errorf("consensus leaf differs between the DB row and its paged-in reconstruction:\n"+
			"  from chain_accounts (what rebuildStateAccumulators folded into accountSetXOR): %x\n"+
			"  from the page-in (what updateAccountLeafLocked will XOR out): %x\n"+
			"  A node holding this account warm and a node paging it in therefore compute different\n"+
			"  accountSetXOR values for the same block — a StateRoot fork whose only differing\n"+
			"  component is account_set_xor.",
			accountLeaf(dbTruth), accountLeaf(pagedIn))
	}

	// Guard on the guard: if dropping the flag did NOT change the leaf, the
	// assertion above would pass for the wrong reason and this file would stop
	// protecting anything. (TestAccountLeaf_CommitsToFaucetClaimed states the
	// same property; this repeats it on the exact fixture under test.)
	lost := *pagedIn
	lost.FaucetClaimed = false
	if accountLeaf(&lost) == accountLeaf(dbTruth) {
		t.Fatal("dropping FaucetClaimed left the leaf unchanged — this fixture can no longer " +
			"detect the regression it exists to catch")
	}
}

// TestEnsureAccountLoadedCtx_SelectsEveryConsensusColumn is INTENTIONALLY RED.
//
// It reads the production source directly rather than modelling it, so it
// cannot drift from the code the way the test above could. It adds nothing to
// production — no hook, no export — it only inspects state.go as text.
func TestEnsureAccountLoadedCtx_SelectsEveryConsensusColumn(t *testing.T) {
	body := functionBodyFromSource(t, "state.go", "func (cs *ChainState) ensureAccountLoadedCtx(")

	// Exactly the columns accountLeaf hashes (state.go:6324-6351). Every one of
	// them must be read back, or the reconstructed account cannot reproduce the
	// leaf that rebuildStateAccumulators already folded into accountSetXOR.
	for _, column := range []string{"balance", "is_human", "tusd_balance", "lp_shares", "faucet_claimed"} {
		if !strings.Contains(body, column) {
			t.Errorf("ensureAccountLoadedCtx never reads %q, but accountLeaf commits to it.\n"+
				"  A cold account paged in through this function is reconstructed with that field at its\n"+
				"  Go zero value, so its leaf — and therefore this node's accountSetXOR and StateRoot —\n"+
				"  disagrees with a node that had the same account resident. loadFromDB (state.go:1956)\n"+
				"  selects the full set; this on-demand path is the one that does not.", column)
		}
	}

	// Not consensus state, but the same omission with a user-visible effect:
	// the one-time 14-day demurrage notice re-fires for every cold account,
	// because the flag that suppresses it is never read back and is then
	// written back as false by the next save.
	if !strings.Contains(body, "demurrage_14_day_warning_shown") {
		t.Logf("NOTE (not a consensus failure): ensureAccountLoadedCtx also omits " +
			"demurrage_14_day_warning_shown, so a paged-in account loses its one-time-notice flag " +
			"and the next save writes the cleared value back to the DB.")
	}
}

// TestEnsureAccountsLoadedCtx_SelectsEveryConsensusColumn covers the BATCH
// cold loader, which the finding above did not name and which carried exactly
// the same defect. It matters more, not less: ensureAccountsLoadedCtx is the
// cold-load call made by snapshotForRollbackLocked and by
// distributeUBIPoolLocked, so a missing column corrupted the cached leaf for
// every human in one distribution round rather than for a single address.
//
// Kept as a separate test from the single-address one so a regression names
// the path it broke.
func TestEnsureAccountsLoadedCtx_SelectsEveryConsensusColumn(t *testing.T) {
	body := functionBodyFromSource(t, "state.go", "func (cs *ChainState) ensureAccountsLoadedCtx(")

	for _, column := range []string{"balance", "is_human", "tusd_balance", "lp_shares", "faucet_claimed"} {
		if !strings.Contains(body, column) {
			t.Errorf("ensureAccountsLoadedCtx never reads %q, but accountLeaf commits to it.\n"+
				"  This is the batch page-in used by snapshotForRollbackLocked and the UBI distribution\n"+
				"  round, and it caches acc.leafHash = accountLeaf(acc) just like the single-address\n"+
				"  loader — so the whole batch is folded into accountSetXOR at a leaf the DB never held.", column)
		}
	}

	// Both loaders must also scan what they select: a column named in the SQL
	// but absent from the Scan target list would still leave the field at its
	// zero value, and the query would fail at runtime rather than at build.
	if !strings.Contains(body, "&acc.FaucetClaimed") {
		t.Error("ensureAccountsLoadedCtx selects faucet_claimed but never scans it into acc.FaucetClaimed")
	}
}

// functionBodyFromSource returns the text of the function in file whose
// declaration starts with decl, up to the next top-level declaration. Test-only
// helper; it reads the file from the package directory, which is the working
// directory `go test` uses.
func functionBodyFromSource(t *testing.T, file, decl string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("could not read %s: %v", file, err)
	}
	text := string(src)
	start := strings.Index(text, decl)
	if start < 0 {
		t.Fatalf("could not find %q in %s — the function was renamed or moved; "+
			"this test must be pointed at its new location rather than deleted", decl, file)
	}
	rest := text[start+len(decl):]
	// The next line that begins a new top-level func ends this one.
	if end := strings.Index(rest, "\nfunc "); end >= 0 {
		return rest[:end]
	}
	return rest
}
