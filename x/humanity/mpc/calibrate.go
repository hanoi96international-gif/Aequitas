package mpc

import (
	"fmt"
	"sort"
)

// Threshold calibration: turning real captures into the number that decides
// who gets refused.
//
// # WHY THIS FILE EXISTS
//
// Every threshold in this package so far was chosen against synthetic vectors.
// Synthetic vectors are drawn independently, so two of them are as far apart as
// random chance allows — real faces are not. Two strangers who look alike, or
// one person under different lighting, sit in the region that decides
// everything, and no amount of uniform sampling produces that region.
//
// A threshold picked without real captures is a guess about who gets locked out
// of their own registration. This file computes the number instead, and refuses
// to produce one from a sample too small to mean anything.
//
// # THE TWO ERRORS ARE NOT SYMMETRIC
//
// FAR — two DIFFERENT people measured as the same. The second one is told they
// are already registered. They have done nothing wrong and, with no document to
// appeal to, they have no way to prove otherwise. This is the error that locks
// a real person out of the system permanently.
//
// FRR — the SAME person measured as different. They register twice. That is a
// Sybil, and it dilutes everyone else's share.
//
// Both are real costs and the trade is genuine. The Aequitas principle decides
// the direction: a reversible error is preferable to an irreversible one, and a
// duplicate account can be found and removed later while a person wrongly
// refused has no recourse at all. So RecommendThreshold optimises against a FAR
// budget and reports the FRR that results — never the reverse.

// LabelledPair is two captures with the truth attached.
//
// Sketches, not embeddings: the comparison runs on sketches, so the calibration
// has to measure what the system actually computes, including whatever the
// binarisation loses.
type LabelledPair struct {
	A, B       []uint8
	SamePerson bool
}

// CalibrationPoint is the behaviour at one threshold.
type CalibrationPoint struct {
	Threshold int

	// FAR: different people the system would call the same. The lockout rate.
	FAR float64
	// FRR: same person the system would call different. The Sybil rate.
	FRR float64

	FalseAccepts, ImpostorPairs int
	FalseRejects, GenuinePairs  int
}

// Calibrate measures FAR and FRR at every threshold from 0 to sketchBits.
//
// Returns points in ascending threshold order. A larger threshold flags more
// pairs as the same person, so FAR rises and FRR falls with it.
func Calibrate(pairs []LabelledPair, sketchBits int) ([]CalibrationPoint, error) {
	if sketchBits <= 0 {
		return nil, fmt.Errorf("mpc: sketchBits must be positive, got %d", sketchBits)
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("mpc: no pairs to calibrate against")
	}

	var genuine, impostor []int
	for i, p := range pairs {
		if len(p.A) != sketchBits || len(p.B) != sketchBits {
			return nil, fmt.Errorf("mpc: pair %d has sketches of %d and %d bits, expected %d — "+
				"a calibration mixing sketch lengths measures nothing",
				i, len(p.A), len(p.B), sketchBits)
		}
		d := 0
		for j := 0; j < sketchBits; j++ {
			if p.A[j] != p.B[j] {
				d++
			}
		}
		if p.SamePerson {
			genuine = append(genuine, d)
		} else {
			impostor = append(impostor, d)
		}
	}
	if len(genuine) == 0 {
		return nil, fmt.Errorf("mpc: no same-person pairs — without them FRR cannot be measured, " +
			"and a threshold chosen on impostors alone rejects everyone safely and uselessly")
	}
	if len(impostor) == 0 {
		return nil, fmt.Errorf("mpc: no different-person pairs — without them FAR cannot be " +
			"measured, which is the error that locks people out")
	}

	sort.Ints(genuine)
	sort.Ints(impostor)

	points := make([]CalibrationPoint, 0, sketchBits+1)
	for th := 0; th <= sketchBits; th++ {
		// The comparison flags a pair when distance < threshold.
		fa := sort.SearchInts(impostor, th) // impostor distances below th
		fr := len(genuine) - sort.SearchInts(genuine, th)
		points = append(points, CalibrationPoint{
			Threshold:     th,
			FAR:           float64(fa) / float64(len(impostor)),
			FRR:           float64(fr) / float64(len(genuine)),
			FalseAccepts:  fa,
			ImpostorPairs: len(impostor),
			FalseRejects:  fr,
			GenuinePairs:  len(genuine),
		})
	}
	return points, nil
}

// MinPairsForCalibration is the smallest sample this package will recommend a
// threshold from.
//
// To claim a FAR of 1 in 1000 you must have seen at least about 1000 impostor
// pairs; below that the measured rate is zero because nothing was observed, not
// because nothing happens. Refusing is better than reporting a confident zero
// from thirty pairs — that number would be quoted for years.
const MinPairsForCalibration = 1000

// RecommendThreshold picks the largest threshold whose FAR stays within budget.
//
// Largest, because among thresholds that meet the lockout budget the largest
// catches the most duplicates. maxFAR is a budget for locking real people out,
// so it should be set from how many wrongly-refused registrations the project
// can actually absorb — not from a number that looks small.
//
// The returned point carries the FRR that comes with it: the Sybil rate being
// accepted in exchange. Read it before deploying the threshold.
func RecommendThreshold(points []CalibrationPoint, maxFAR float64) (CalibrationPoint, error) {
	if len(points) == 0 {
		return CalibrationPoint{}, fmt.Errorf("mpc: no calibration points")
	}
	if maxFAR <= 0 || maxFAR >= 1 {
		return CalibrationPoint{}, fmt.Errorf("mpc: maxFAR must be in (0,1), got %v", maxFAR)
	}
	if n := points[0].ImpostorPairs; n < MinPairsForCalibration {
		return CalibrationPoint{}, fmt.Errorf("mpc: %d impostor pairs is too small a sample to set "+
			"a threshold from (need %d) — a FAR measured as zero here means nothing was observed, "+
			"not that nobody gets locked out", n, MinPairsForCalibration)
	}
	if n := points[0].GenuinePairs; n < MinPairsForCalibration {
		return CalibrationPoint{}, fmt.Errorf("mpc: %d same-person pairs is too small a sample "+
			"(need %d) — the duplicate-catch rate would be unmeasured",
			n, MinPairsForCalibration)
	}

	best := -1
	for i, p := range points {
		if p.FAR <= maxFAR {
			best = i
		}
	}
	if best < 0 {
		return CalibrationPoint{}, fmt.Errorf("mpc: no threshold meets a FAR budget of %v; the "+
			"lowest measured FAR is %v at threshold %d. The sketch is not separating these "+
			"captures — widen it or fix the capture pipeline rather than accepting the budget",
			maxFAR, points[0].FAR, points[0].Threshold)
	}
	chosen := points[best]
	if chosen.FRR >= 1 {
		return chosen, fmt.Errorf("mpc: threshold %d meets the FAR budget but has FRR %.3f — it "+
			"catches no duplicates at all, so the check would be decorative",
			chosen.Threshold, chosen.FRR)
	}
	return chosen, nil
}

// EffectiveDuplicateCatchRate combines the two independent ways a duplicate
// escapes: the index never compares the pair, or the comparison misses it.
//
// Reported together because either alone is misleading. A 99% recall index in
// front of a comparison that misses a third of duplicates catches two thirds,
// not 99%.
func EffectiveDuplicateCatchRate(indexRecall float64, p CalibrationPoint) float64 {
	return indexRecall * (1 - p.FRR)
}
