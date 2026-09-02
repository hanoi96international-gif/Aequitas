package keeper

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The two writes that move AEQ between an account and the pool are not atomic,
// so their ORDER decides which way a failure between them falls: whichever side
// is losing AEQ must be persisted first, so a crack destroys supply instead of
// creating it.
//
// The rule was applied to the primary paths and not to their replay
// counterparts, which left the same operation failing safely on a producing
// node and unsafely on a replaying one — invisible, because both nodes agree
// whenever nothing fails.
//
// This reads the source rather than exercising the paths: the failure only
// appears when a database write fails midway, which no unit test here can
// stage. Reading is enough to catch a reordering, which is the realistic
// regression.
func TestPoolWriteOrderMatchesBetweenPrimaryAndReplay(t *testing.T) {
	src, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatalf("could not read state.go: %v", err)
	}
	content := strings.ReplaceAll(string(src), "\r\n", "\n")

	// Which side each function takes AEQ from. "account" means the account is
	// debited, so the account must be saved first; "pool" is the reverse.
	// swapLocked and applySwapDeltaLocked are direction-aware and checked
	// separately below.
	losesAEQ := map[string]string{
		"addLiquidityLocked":         "account",
		"addLiquidityDeltaLocked":    "account",
		"removeLiquidityLocked":      "pool",
		"removeLiquidityDeltaLocked": "pool",
	}

	fnRe := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)`)
	locs := fnRe.FindAllStringSubmatchIndex(content, -1)

	for name, side := range losesAEQ {
		body := ""
		for i, loc := range locs {
			if content[loc[2]:loc[3]] != name {
				continue
			}
			end := len(content)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			body = content[loc[0]:end]
			break
		}
		if body == "" {
			t.Errorf("%s not found in state.go — if it was renamed, re-point this test rather "+
				"than deleting it", name)
			continue
		}

		acct := strings.Index(body, "saveAccountToDBCtx(ctx, acc)")
		pool := strings.Index(body, "savePoolToDBCtx(ctx)")
		if acct < 0 || pool < 0 {
			continue // not a function that does both
		}

		first := "account"
		if pool < acct {
			first = "pool"
		}
		if first != side {
			t.Errorf("%s persists the %s first, but the %s is the side losing AEQ.\n"+
				"  A failure between the two writes then leaves the receiving side credited "+
				"durably while the giving side never records the loss — that is new money.\n"+
				"  Whichever side is LOSING AEQ must be saved first.", name, first, side)
		}
	}
}

// TestSwapWriteOrderIsDirectionAware: a swap moves value BOTH ways, so no fixed
// order is safe. Both the primary and the replay path must branch on direction.
func TestSwapWriteOrderIsDirectionAware(t *testing.T) {
	f, err := os.Open("state.go")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")

	for _, fn := range []string{"swapLocked", "applySwapDeltaLocked"} {
		i := strings.Index(content, "func (cs *ChainState) "+fn+"(")
		if i < 0 {
			t.Errorf("%s not found", fn)
			continue
		}
		end := strings.Index(content[i+10:], "\nfunc ")
		body := content[i:]
		if end > 0 {
			body = content[i : i+10+end]
		}

		// The tell: the save order must depend on aeqToTusd somewhere between
		// the mutation and the writes.
		hasBranch := strings.Contains(body, "aeqToTusd") &&
			strings.Count(body, "saveAccountToDBCtx") >= 1 &&
			strings.Count(body, "savePoolToDBCtx") >= 1
		if !hasBranch {
			t.Errorf("%s does not appear to order its two writes by swap direction", fn)
			continue
		}

		// A single unconditional account-then-pool sequence with no direction
		// branch between them is the shape that was wrong.
		if strings.Count(body, "savePoolFirst") == 0 && strings.Count(body, "saveAccountFirst") == 0 {
			t.Errorf("%s has no direction-aware ordering helper. A swap moves AEQ both ways: "+
				"saving the account first is right only when the account is the side giving "+
				"AEQ up, and wrong in the other direction, where it creates money", fn)
		}
	}
}

// TestEveryPoolWritingPathIsAccountedFor keeps the sweep from decaying into a
// one-off.
//
// A systematic pass on 2026-08-20 compared every *DeltaLocked path against its
// primary and found three divergences, every one of them invisible because both
// sides agree whenever nothing fails. That pass is worth nothing a month from
// now unless a NEW path that writes both an account and the pool is made to
// declare which side loses AEQ.
//
// So any function that saves both must appear in the table in
// TestPoolWriteOrderMatchesBetweenPrimaryAndReplay, or here as a deliberate
// exemption with a reason.
func TestEveryPoolWritingPathIsAccountedFor(t *testing.T) {
	src, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(string(src), "\r\n", "\n")

	accounted := map[string]string{
		"addLiquidityLocked":         "declared: the account loses AEQ",
		"addLiquidityDeltaLocked":    "declared: the account loses AEQ",
		"removeLiquidityLocked":      "declared: the pool loses AEQ",
		"removeLiquidityDeltaLocked": "declared: the pool loses AEQ",
		"swapLocked":                 "direction-aware, see TestSwapWriteOrderIsDirectionAware",
		"applySwapDeltaLocked":       "direction-aware, see TestSwapWriteOrderIsDirectionAware",
		// Sichtbar geworden, als der Scanner unten auch die Mantel-Namen
		// erfasste. Die Reihenfolge ist korrekt: der Topf gibt AEQ ab, also
		// wird er zuerst geschrieben, und ein Fehlschlag dazwischen setzt die
		// Reserven ausdruecklich zurueck.
		"MigrateStrandedPoolTUsdFeesV1": "declared: the pool loses AEQ, and it restores the reserves on a failed pool write",
		// Kein Uebertrag zwischen Konto und Topf, sondern das Zurueckschreiben
		// zuvor gesicherter Werte -- eine Seite, die AEQ verliert, gibt es hier
		// nicht. Ein Fehlschlag dazwischen wird vom Aufrufer bereits als
		// CRITICAL gemeldet ("rollback persistence failed"), weil dann Speicher
		// und Datenbank auseinanderlaufen; das ist der bekannte, gemeldete
		// Zustand und keine stille Fehlbuchung.
		"restoreFromRollbackLockedCtx": "restore of snapshot values, not a transfer; a partial failure is already reported as CRITICAL",
	}

	fnRe := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)`)
	locs := fnRe.FindAllStringSubmatchIndex(content, -1)

	for i, loc := range locs {
		name := content[loc[2]:loc[3]]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		body := content[loc[0]:end]

		// Auch die Mantel-Namen erfassen. Vorher sah der Scanner NUR die
		// Ctx-Fassungen -- MigrateStrandedPoolTUsdFeesV1 schreibt beides ueber
		// saveAccountToDB/savePoolToDB und war dadurch unsichtbar. Waehrend die
		// activeTx-Migration laeuft, existieren beide Schreibweisen
		// nebeneinander; ein Pruefer, der nur eine kennt, meldet zu wenig.
		schreibtKonto := strings.Contains(body, "saveAccountToDBCtx(ctx, acc)") ||
			strings.Contains(body, "saveAccountToDB(acc)")
		schreibtTopf := strings.Contains(body, "savePoolToDBCtx(ctx)") ||
			strings.Contains(body, "savePoolToDB()")
		if !schreibtKonto || !schreibtTopf {
			continue
		}
		if _, ok := accounted[name]; ok {
			continue
		}
		t.Errorf("%s writes both an account and the pool but is not accounted for.\n"+
			"  The two writes are not atomic, so their order decides whether a failure between\n"+
			"  them destroys supply or creates it. Add it to the table in\n"+
			"  TestPoolWriteOrderMatchesBetweenPrimaryAndReplay with the side that loses AEQ,\n"+
			"  or here with a reason it is exempt.", name)
	}
}
