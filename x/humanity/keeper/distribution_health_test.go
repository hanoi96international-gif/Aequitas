package keeper

import (
	"testing"
	"time"
)

// The trap this metric exists to avoid: reading last_ubi_at as "when the round
// last ran". It is not that. RunDailyDistributionAtomic stamps it only
// `if ubiTotal > 0`, so on a quiet chain a perfectly healthy scheduler leaves it
// untouched for weeks. Reported on 2026-08-15 as nineteen days old, which meant
// only that no fees had accumulated — the four pools held 0.0000/0.0000/0.0000/
// 0.2227 AEQ.
func TestDistributionHealth_OldPayoutAloneIsNotAFault(t *testing.T) {
	resetDistributionOutcomeForTest()
	cs := newTestState()
	// A round that ran and found nothing to pay is a healthy round.
	RecordDistributionOutcome("ran", "round executed; nothing to distribute")

	h := cs.DistributionHealth()
	if h["healthy"] != true {
		t.Errorf("a scheduler that ran is healthy even with no payout, got %v (problem: %v)",
			h["healthy"], h["problem"])
	}
	if h["last_payout_note"] == nil {
		t.Error("the payout timestamp must carry the note explaining what it does and does not mean")
	}
}

// The condition that IS a fault: rounds are reached and every one of them is
// declined. That is the shape the peer-sync gate produced.
func TestDistributionHealth_EveryRoundDeclinedIsAFault(t *testing.T) {
	resetDistributionOutcomeForTest()
	cs := newTestState()
	for i := 0; i < 3; i++ {
		RecordDistributionOutcome("skipped",
			"peers are configured but this node has never successfully synced a block from any of them")
	}
	h := cs.DistributionHealth()
	if h["healthy"] != false {
		t.Fatalf("three declined rounds and no execution must not read as healthy, got %v", h["healthy"])
	}
	problem, _ := h["problem"].(string)
	if problem == "" {
		t.Fatal("want a problem string naming what was declined and why")
	}
	att, ok := h["last_attempt"].(map[string]interface{})
	if !ok || att["outcome"] != "skipped" {
		t.Fatalf("last_attempt must carry the outcome, got %v", h["last_attempt"])
	}
	if att["reason"] == "" {
		t.Error("the reason is the whole point — it was previously only a log line")
	}
}

// A node that has not yet reached a slot is not broken.
func TestDistributionHealth_BeforeFirstAttempt(t *testing.T) {
	resetDistributionOutcomeForTest()
	cs := newTestState()
	h := cs.DistributionHealth()
	if h["healthy"] != true {
		t.Errorf("a node that has not reached a round yet is healthy, got %v", h["healthy"])
	}
	if h["last_attempt"] != nil {
		t.Errorf("last_attempt = %v, want nil before the first attempt", h["last_attempt"])
	}
}

// A round that executed long ago and nothing since is the other fault shape.
func TestDistributionHealth_StaleAfterALastSuccessfulRound(t *testing.T) {
	resetDistributionOutcomeForTest()
	cs := newTestState()
	RecordDistributionOutcome("ran", "")
	lastDistribution.mu.Lock()
	lastDistribution.lastRan = time.Now().Add(-30 * time.Hour)
	lastDistribution.mu.Unlock()
	RecordDistributionOutcome("skipped", "still degraded")

	h := cs.DistributionHealth()
	if h["healthy"] != false {
		t.Fatalf("no execution in 30h must not read as healthy, got %v", h["healthy"])
	}
}

func resetDistributionOutcomeForTest() {
	lastDistribution.mu.Lock()
	defer lastDistribution.mu.Unlock()
	lastDistribution.at = time.Time{}
	lastDistribution.outcome = ""
	lastDistribution.reason = ""
	lastDistribution.attempts = 0
	lastDistribution.lastRan = time.Time{}
}
