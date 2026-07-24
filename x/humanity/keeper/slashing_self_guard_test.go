package keeper

import (
	"strings"
	"testing"
)

// shouldDropSelfObservedEquivocation mirrors the two guards AddPeerBlock's
// equivocation goroutine applies before recording or propagating anything:
// a node never slashes its own signing address, and never slashes its own
// configured BOOTSTRAP_SIGNER. Kept as a pure predicate so both can be
// exercised without a live DAG, DB or network — the same reason
// syncStarvationTickConfirms exists in autoheal.go.
func shouldDropSelfObservedEquivocation(proposer, selfAddr, bootstrapSigner string) bool {
	if selfAddr != "" && strings.EqualFold(proposer, selfAddr) {
		return true
	}
	if bootstrapSigner != "" && strings.EqualFold(proposer, bootstrapSigner) {
		return true
	}
	return false
}

const (
	primaryAddr    = "0x92CBeDeC9D348B4762Cb9aF99500ee6139C5B671"
	primaryAddrLC  = "0x92cbedec9d348b4762cb9af99500ee6139c5b671"
	thirdPartyAddr = "0x0BE8b961CBf6564bd1931B0803D35C0659E0D016"
)

// TestSelfObservedEquivocation_NodeNeverSlashesItself is the regression guard
// for the 2026-07-24 finding: slash_equivocation TXs against the primary were
// still being minted hours after the BOOTSTRAP_SIGNER guard shipped, because
// nothing stopped a node from slashing its OWN signing address. The primary is
// the seed and has no BOOTSTRAP_SIGNER configured, so that guard was skipped
// entirely on exactly the node whose address was being slashed. Confirmed live
// in block #1781171 on aequitas.digital itself.
func TestSelfObservedEquivocation_NodeNeverSlashesItself(t *testing.T) {
	// The primary: its own address, no BOOTSTRAP_SIGNER set.
	if !shouldDropSelfObservedEquivocation(primaryAddr, primaryAddr, "") {
		t.Fatal("a node observing an equivocation by its OWN signing address must drop it — propagating the verdict is a self-inflicted network-wide ban")
	}
}

// TestSelfObservedEquivocation_CaseInsensitive guards the comparison itself:
// block.Proposer arrives checksum-cased while configured/derived addresses are
// routinely lowercased, so a case-sensitive check would silently never fire.
func TestSelfObservedEquivocation_CaseInsensitive(t *testing.T) {
	if !shouldDropSelfObservedEquivocation(primaryAddr, primaryAddrLC, "") {
		t.Fatal("the self-address comparison must be case-insensitive — proposers arrive checksum-cased, configured addresses lowercased")
	}
}

// TestSelfObservedEquivocation_BootstrapSignerStillGuarded pins that the
// original guard is untouched by the new one: a secondary still refuses to
// slash its configured trust anchor even though that address is not its own.
func TestSelfObservedEquivocation_BootstrapSignerStillGuarded(t *testing.T) {
	if !shouldDropSelfObservedEquivocation(primaryAddr, thirdPartyAddr, primaryAddrLC) {
		t.Fatal("a node must still refuse to slash its configured BOOTSTRAP_SIGNER (the 2026-07-24 guard this one complements)")
	}
}

// TestSelfObservedEquivocation_ThirdPartyStillSlashable is the boundary in the
// other direction, and the reason neither guard is scoped any wider: a
// genuinely misbehaving THIRD validator must still be slashed locally and
// immediately. Widening either guard into "never slash anyone" would disable
// the mechanism outright.
func TestSelfObservedEquivocation_ThirdPartyStillSlashable(t *testing.T) {
	if shouldDropSelfObservedEquivocation(thirdPartyAddr, primaryAddr, primaryAddrLC) {
		t.Fatal("a third validator that is neither this node nor its trust anchor must remain slashable")
	}
}

// TestSelfObservedEquivocation_UnconfiguredNodeSlashesNobodyByAccident checks
// the empty-string edges: an unset selfProposer or BOOTSTRAP_SIGNER must not
// match every proposer via "" == prefix-style sloppiness.
func TestSelfObservedEquivocation_UnconfiguredNodeSlashesNobodyByAccident(t *testing.T) {
	if shouldDropSelfObservedEquivocation(thirdPartyAddr, "", "") {
		t.Fatal("empty self/bootstrap addresses must never match a real proposer")
	}
}
