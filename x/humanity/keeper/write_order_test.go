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
