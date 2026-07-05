package keeper

import "testing"

// Regression tests for the beta-launch audit (2026-07-05) safety net: the
// hardcoded V7 storage-slot persistence lists (v7SimpleSlots/
// v7AddressMappingSlots/v7ArrayBaseSlots) are contract-specific knowledge
// that silently goes stale if AequitasV7.sol's storage layout ever changes
// without a matching update here. v7SlotsVerifiedFor turns that into a
// loud, checkable mismatch instead of quietly-wrong behavior.

func TestV7SlotsVerifiedFor_MatchesCurrentlyDeployedVersion(t *testing.T) {
	if !v7SlotsVerifiedFor(V7ContractVersion) {
		t.Fatalf("v7SlotsVerifiedForVersion (%q) has drifted from the currently deployed V7ContractVersion (%q) — "+
			"update the slot lists in evm_engine.go for this version's storage layout, then bump v7SlotsVerifiedForVersion to match",
			v7SlotsVerifiedForVersion, V7ContractVersion)
	}
}

func TestV7SlotsVerifiedFor_DetectsMismatch(t *testing.T) {
	if v7SlotsVerifiedFor("some-future-version-nobody-verified-yet") {
		t.Fatal("expected a mismatch for an unverified version string")
	}
}
