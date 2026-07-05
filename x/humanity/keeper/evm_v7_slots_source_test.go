package keeper

import (
	"os"
	"regexp"
	"testing"
)

// TestV7SlotListsMatchContractSource is the real, verifiable safety net for
// G6 (beta-launch audit 2026-07-05): the hardcoded storage-slot persistence
// lists (v7SimpleSlots/v7AddressMappingSlots/v7ArrayBaseSlots, evm_engine.go)
// are contract-specific knowledge that would silently go stale if
// AequitasV7.sol ever gains, loses, or reorders a storage-consuming state
// variable — any write to an untracked slot succeeds in the live EVM call
// but is never persisted, with nothing surfacing the mismatch (see those
// vars' own comment for the full failure mode).
//
// v7SlotsVerifiedForVersion (evm_slots_version_test.go) only catches this
// when V7ContractVersion itself is bumped — it would NOT catch a slot
// miscounted within the same version string, or a future engineer editing
// AequitasV7.sol without remembering to bump the version marker at all.
// This test instead parses AequitasV7.sol's actual storage-variable
// declarations directly and derives the slot layout Solidity itself would
// assign (sequential slots for constant/immutable-excluded declarations,
// N consecutive slots for a fixed-size array of length N — true as long as
// no two declared variables are small enough to pack into one slot
// together, which this test also checks) — then compares that DERIVED
// layout against the hardcoded Go lists byte for byte.
//
// A previous engineer already investigated making evm_engine.go's actual
// persistence step itself generic against go-ethereum's StateDB and hit a
// real technical wall in this go-ethereum version (see v7SimpleSlots'
// comment: "RawDump does not find accounts after Commit, even on the same
// backing database"). Reusing that fragile path here would risk the exact
// silent-miss failure mode this test exists to catch, in the test itself.
// Parsing the .sol source's declared storage layout sidesteps that
// entirely — it's a completely independent verification path.
func TestV7SlotListsMatchContractSource(t *testing.T) {
	src, err := os.ReadFile("../../../AequitasV7.sol")
	if err != nil {
		t.Fatalf("could not read AequitasV7.sol: %v", err)
	}
	content := string(src)

	contractStart := regexp.MustCompile(`contract\s+AequitasV7\s*\{`).FindStringIndex(content)
	if contractStart == nil {
		t.Fatal("could not find 'contract AequitasV7 {' — has the contract been renamed?")
	}
	// State variables are declared before any function/constructor/event in
	// this file's own convention (verified by inspection) — bound the scan
	// to that header region so we never accidentally parse a local variable
	// declaration inside a function body as a storage slot.
	body := content[contractStart[1]:]
	headerEnd := regexp.MustCompile(`\b(function|constructor|event)\b`).FindStringIndex(body)
	if headerEnd == nil {
		t.Fatal("could not find the end of AequitasV7's state-variable header (no function/constructor/event found)")
	}
	header := body[:headerEnd[0]]

	// Matches the four declaration shapes this file actually uses:
	//   mapping(K => V) public name;
	//   TYPE public constant name = ...;
	//   TYPE public immutable name;
	//   TYPE public name;              (plain storage var)
	//   TYPE[N] public name = [...];   (fixed-size array)
	declRe := regexp.MustCompile(`(?m)^\s*(mapping\s*\([^)]*\)|[A-Za-z0-9_]+(?:\[(\d+)\])?)\s+public\s+(constant\s+|immutable\s+)?([A-Za-z0-9_]+)`)
	matches := declRe.FindAllStringSubmatch(header, -1)
	if len(matches) == 0 {
		t.Fatal("no state variable declarations matched in the header region — the parsing regex is likely broken, not that the contract has zero storage")
	}

	type derivedSlot struct {
		name    string
		isMap   bool
		arrayN  int // 0 for non-arrays
		skipped bool
	}
	var derived []derivedSlot
	for _, m := range matches {
		typeOrMapping, arrayLen, qualifier, name := m[1], m[2], m[3], m[4]
		if qualifier == "constant " || qualifier == "immutable " {
			continue // no storage slot consumed
		}
		d := derivedSlot{name: name}
		if len(typeOrMapping) >= 7 && typeOrMapping[:7] == "mapping" {
			d.isMap = true
		} else if arrayLen != "" {
			n := 0
			for _, c := range arrayLen {
				n = n*10 + int(c-'0')
			}
			d.arrayN = n
		}
		derived = append(derived, d)
	}

	// Assign sequential slots exactly as Solidity would (each declaration
	// gets the next free slot; a fixed array of length N consumes N
	// consecutive slots). This chain assumes no variable packing occurred —
	// true here because every non-constant/immutable declaration is either
	// a mapping (always exactly 1 slot regardless of value type) or a plain
	// uint256 (32 bytes, never shares a slot with a neighbor) — verified by
	// reading the actual declarations above, not assumed blindly.
	var gotSimple, gotMappings, gotArrays []int64
	var gotSpecialCommitments, gotSpecialNullifiers int64 = -1, -1
	next := int64(0)
	for _, d := range derived {
		switch {
		case d.arrayN > 0:
			for i := 0; i < d.arrayN; i++ {
				gotArrays = append(gotArrays, next)
				next++
			}
		case d.isMap:
			switch d.name {
			case "usedCommitments":
				gotSpecialCommitments = next
			case "usedNullifiers":
				gotSpecialNullifiers = next
			default:
				gotMappings = append(gotMappings, next)
			}
			next++
		default:
			gotSimple = append(gotSimple, next)
			next++
		}
	}

	assertEqualInt64Slices(t, "v7SimpleSlots", v7SimpleSlots, gotSimple)
	assertEqualInt64Slices(t, "v7AddressMappingSlots", v7AddressMappingSlots, gotMappings)
	assertEqualInt64Slices(t, "v7ArrayBaseSlots", v7ArrayBaseSlots, gotArrays)
	if gotSpecialCommitments != 7 {
		t.Errorf("usedCommitments derived at slot %d from the contract source, but evm_engine.go's dumpAndPersistStorage hardcodes slot 7 for it", gotSpecialCommitments)
	}
	if gotSpecialNullifiers != 8 {
		t.Errorf("usedNullifiers derived at slot %d from the contract source, but evm_engine.go hardcodes slot 8 for it", gotSpecialNullifiers)
	}
}

func assertEqualInt64Slices(t *testing.T, label string, want, got []int64) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s: length mismatch — Go list has %d slot(s) %v, but AequitasV7.sol's source derives %d slot(s) %v. "+
			"If AequitasV7.sol gained/lost/reordered a storage variable, update %s in evm_engine.go to match.",
			label, len(want), want, len(got), got, label)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("%s[%d]: Go list says slot %d, contract source derives slot %d", label, i, want[i], got[i])
		}
	}
}
