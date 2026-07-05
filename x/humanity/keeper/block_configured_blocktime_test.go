package keeper

import (
	"testing"
	"time"
)

// TestConfiguredBlockTimeSeconds_ReflectsRealValue is the regression guard
// for the 2026-07-05 audit finding: /api/status's "block_time" field used
// to be a bare hand-typed literal in api.go, correct only by coincidence
// with whatever BLOCK_TIME happened to be at the time someone last edited
// it — BLOCK_TIME changed 4 separate times in one night, and nothing
// enforced the two stayed in sync. TuneProposerBreakerForBlockTime is
// already the established "call once at startup with the real BLOCK_TIME"
// entry point; this verifies it also updates the value the status endpoint
// reads, so the two can never drift apart again.
func TestConfiguredBlockTimeSeconds_ReflectsRealValue(t *testing.T) {
	original := configuredBlockTimeSeconds
	defer func() { configuredBlockTimeSeconds = original }()

	TuneProposerBreakerForBlockTime(1500 * time.Millisecond)
	if got := ConfiguredBlockTimeSeconds(); got != 1.5 {
		t.Fatalf("ConfiguredBlockTimeSeconds() = %v after tuning for 1.5s BLOCK_TIME, want 1.5", got)
	}
}
