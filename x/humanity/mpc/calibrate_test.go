package mpc

import (
	"math/rand"
	"testing"
)

// makePairs builds a labelled set with a KNOWN separation, so the harness can
// be checked against ground truth it cannot see.
func makePairs(t *testing.T, bits, genuine, impostor int, genuineFlips, impostorFlips int) []LabelledPair {
	t.Helper()
	rng := rand.New(rand.NewSource(1234))
	var out []LabelledPair
	mk := func(flips int, same bool) LabelledPair {
		a := make([]uint8, bits)
		for i := range a {
			a[i] = uint8(rng.Intn(2))
		}
		b := make([]uint8, bits)
		copy(b, a)
		perm := rng.Perm(bits)
		for i := 0; i < flips; i++ {
			b[perm[i]] ^= 1
		}
		return LabelledPair{A: a, B: b, SamePerson: same}
	}
	for i := 0; i < genuine; i++ {
		out = append(out, mk(genuineFlips, true))
	}
	for i := 0; i < impostor; i++ {
		out = append(out, mk(impostorFlips, false))
	}
	return out
}

// TestCalibrateRecoversKnownSeparation: same-person pairs 10 bits apart,
// strangers 60 apart. Any threshold between them must be perfect, and the
// harness must say so.
func TestCalibrateRecoversKnownSeparation(t *testing.T) {
	const bits = 128
	pairs := makePairs(t, bits, 1200, 1200, 10, 60)

	points, err := Calibrate(pairs, bits)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != bits+1 {
		t.Fatalf("got %d points, want %d", len(points), bits+1)
	}

	mid := points[30] // between 10 and 60
	if mid.FAR != 0 || mid.FRR != 0 {
		t.Errorf("at threshold 30 with a clean 10/60 separation: FAR=%v FRR=%v, want both 0",
			mid.FAR, mid.FRR)
	}
	// Below every genuine distance, nothing is flagged: no lockouts, no catches.
	if points[0].FAR != 0 || points[0].FRR != 1 {
		t.Errorf("at threshold 0: FAR=%v FRR=%v, want 0 and 1", points[0].FAR, points[0].FRR)
	}
	// Above every distance, everything is flagged: everyone is a duplicate.
	last := points[bits]
	if last.FAR != 1 || last.FRR != 0 {
		t.Errorf("at threshold %d: FAR=%v FRR=%v, want 1 and 0", bits, last.FAR, last.FRR)
	}
}

// TestFARRisesAndFRRFallsWithThreshold pins the direction of the trade, so a
// sign error cannot silently invert the recommendation.
func TestFARRisesAndFRRFallsWithThreshold(t *testing.T) {
	const bits = 64
	pairs := makePairs(t, bits, 1100, 1100, 8, 26)
	points, err := Calibrate(pairs, bits)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(points); i++ {
		if points[i].FAR < points[i-1].FAR {
			t.Fatalf("FAR fell from %v to %v as the threshold rose to %d",
				points[i-1].FAR, points[i].FAR, points[i].Threshold)
		}
		if points[i].FRR > points[i-1].FRR {
			t.Fatalf("FRR rose from %v to %v as the threshold rose to %d",
				points[i-1].FRR, points[i].FRR, points[i].Threshold)
		}
	}
}

// TestRecommendationRespectsTheLockoutBudget: the recommendation exists to
// bound how many real people are refused.
func TestRecommendationRespectsTheLockoutBudget(t *testing.T) {
	const bits = 128
	pairs := makePairs(t, bits, 1500, 1500, 12, 45)
	points, err := Calibrate(pairs, bits)
	if err != nil {
		t.Fatal(err)
	}

	for _, budget := range []float64{0.001, 0.01, 0.05} {
		got, err := RecommendThreshold(points, budget)
		if err != nil {
			t.Fatalf("budget %v: %v", budget, err)
		}
		if got.FAR > budget {
			t.Errorf("budget %v: recommended threshold %d has FAR %v, over budget",
				budget, got.Threshold, got.FAR)
		}
		// It must also be the LARGEST such threshold, or duplicates are given
		// away for nothing.
		if got.Threshold+1 <= bits && points[got.Threshold+1].FAR <= budget {
			t.Errorf("budget %v: recommended %d but %d also fits the budget and catches more "+
				"duplicates", budget, got.Threshold, got.Threshold+1)
		}
		t.Logf("budget FAR<=%.3f -> threshold %d (FAR %.4f, FRR %.4f)",
			budget, got.Threshold, got.FAR, got.FRR)
	}
}

// TestTooSmallASampleIsRefused is the point of the whole file: a threshold
// backed by thirty pairs would be quoted as fact for years.
func TestTooSmallASampleIsRefused(t *testing.T) {
	const bits = 64
	pairs := makePairs(t, bits, 15, 15, 5, 30)
	points, err := Calibrate(pairs, bits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecommendThreshold(points, 0.001); err == nil {
		t.Error("a threshold was recommended from 15 pairs per class — a FAR of zero there means " +
			"nothing was observed, not that nobody is locked out")
	}
}

// TestOverlappingDistributionsAreReportedNotHidden: when the sketch cannot
// separate the captures, the harness must say so rather than return the least
// bad threshold as if it were fine.
func TestOverlappingDistributionsAreReportedNotHidden(t *testing.T) {
	const bits = 64
	// Genuine and impostor pairs at the same distance: no separation at all.
	pairs := makePairs(t, bits, 1200, 1200, 20, 20)
	points, err := Calibrate(pairs, bits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RecommendThreshold(points, 0.001)
	if err != nil {
		t.Logf("refused, as it should: %v", err)
		return
	}
	if got.FRR < 0.99 {
		t.Errorf("with completely overlapping distributions the recommendation was threshold %d "+
			"at FRR %.3f, which implies duplicates are being caught. They are not — the sketch "+
			"carries no signal here", got.Threshold, got.FRR)
	}
}

func TestCalibrateRejectsUnusableInput(t *testing.T) {
	if _, err := Calibrate(nil, 64); err == nil {
		t.Error("an empty pair set was accepted")
	}
	onlyGenuine := []LabelledPair{{A: []uint8{0, 1}, B: []uint8{0, 1}, SamePerson: true}}
	if _, err := Calibrate(onlyGenuine, 2); err == nil {
		t.Error("a set with no impostor pairs was accepted — FAR would be unmeasured")
	}
	onlyImpostor := []LabelledPair{{A: []uint8{0, 1}, B: []uint8{1, 0}, SamePerson: false}}
	if _, err := Calibrate(onlyImpostor, 2); err == nil {
		t.Error("a set with no genuine pairs was accepted — FRR would be unmeasured")
	}
	mixed := []LabelledPair{{A: []uint8{0, 1}, B: []uint8{0}, SamePerson: true}}
	if _, err := Calibrate(mixed, 2); err == nil {
		t.Error("mismatched sketch lengths were accepted")
	}
}

// TestIndexRecallIsPartOfTheAnswer: a good threshold in front of a lossy index
// still misses duplicates, and the two must be reported together.
func TestEffectiveCatchRateCombinesIndexAndThreshold(t *testing.T) {
	p := CalibrationPoint{Threshold: 40, FRR: 0.10}
	got := EffectiveDuplicateCatchRate(0.92, p)
	want := 0.92 * 0.90
	if got < want-1e-9 || got > want+1e-9 {
		t.Errorf("EffectiveDuplicateCatchRate = %v, want %v", got, want)
	}
	if got > 0.92 {
		t.Error("the combined catch rate exceeded the index recall, which is impossible: a pair " +
			"never compared cannot be caught by any threshold")
	}
}
