package mpc

import (
	"fmt"
	"sort"
)

// Bucketing: the only thing that makes private de-duplication survive scale.
//
// # THE ARITHMETIC THAT FORCES THIS
//
// One secure comparison of a 512-bit template across three parties costs about
// 93 ms (measured, see TestSecureComparisonCostAndItsConsequence). Comparing a
// newcomer against every existing enrolment therefore costs:
//
//	         100 enrolments      9.3 seconds
//	      10,000 enrolments     15.5 minutes
//	   1,000,000 enrolments      1.1 days
//	8,000,000,000 enrolments    23.6 YEARS   — per single registration
//
// No optimisation closes that gap. A thousandfold faster protocol still needs
// eight days at population scale. The cost per comparison is not the problem;
// the NUMBER of comparisons is. Anything that scales must compare against a
// few dozen candidates instead of billions, and everything else is detail.
//
// # HOW THE CANDIDATES ARE FOUND
//
// The sketch this package already produces (see binarize.go) is a
// sign-random-projection code: similar faces produce similar codes, bit for
// bit. So a prefix of that code is itself a similarity signal — two templates
// that agree on the first k bits are far more likely to be close than two
// drawn at random. Grouping enrolments by that prefix turns "compare against
// everyone" into "compare against the handful who share a prefix".
//
// # WHAT THIS COSTS IN PRIVACY, STATED PLAINLY
//
// The bucket key is derived from the template, so publishing it leaks
// something: which of 2^k regions of sketch space a person falls into. With
// k=12 that is one part in 4096 — it does not identify anyone, and it cannot
// be inverted to a face, but it is not nothing, and an adversary who watches
// enrolments learns the population's distribution across buckets.
//
// This is a deliberate trade and the only known way to make the numbers work.
// It is stated here rather than buried because the honest version of "private
// biometric de-duplication at scale" always contains a leak of exactly this
// shape; systems that claim otherwise have usually just not written it down.
//
// # WHAT IT COSTS IN ACCURACY
//
// A prefix match is a heuristic. Two captures of one person can land in
// different buckets — noise flips a prefix bit — and that person is then not
// compared against their own earlier enrolment, so a duplicate is missed.
// MultiProbe exists for exactly this: probing neighbouring buckets recovers
// most of those cases at linear cost in the number of probes. Missing a
// duplicate is also the SAFE direction of error here: under this project's
// rule a false match against a stranger is the expensive mistake, because it
// keeps a real human out.
type BucketKey uint32

// BucketConfig describes how sketches are grouped.
type BucketConfig struct {
	// PrefixBits is how many leading sketch bits form the key. Larger means
	// smaller buckets (less work per registration) but more missed duplicates
	// from noise, and more leaked structure. It must be identical everywhere,
	// forever — regrouping an existing registry means re-bucketing all of it.
	PrefixBits int
}

// Validate rejects configurations that cannot work, rather than letting them
// produce a registry that looks fine and matches nobody.
func (c BucketConfig) Validate(sketchBits int) error {
	if c.PrefixBits <= 0 {
		return fmt.Errorf("mpc: PrefixBits must be positive, got %d", c.PrefixBits)
	}
	if c.PrefixBits > 32 {
		return fmt.Errorf("mpc: PrefixBits %d exceeds the 32 bits a BucketKey holds", c.PrefixBits)
	}
	if c.PrefixBits >= sketchBits {
		return fmt.Errorf("mpc: PrefixBits %d must be smaller than the %d-bit sketch — "+
			"a key as wide as the sketch buckets every capture separately and finds nobody",
			c.PrefixBits, sketchBits)
	}
	return nil
}

// KeyFor derives the bucket key from a PLAINTEXT sketch.
//
// This runs where the sketch is still in the clear — on the capture device or
// in the enrolling client — never on a party that holds only shares. That is
// the point: the key travels with the shares as public routing information,
// and the parties never reconstruct anything to compute it.
func (c BucketConfig) KeyFor(sketch []uint8) (BucketKey, error) {
	if err := c.Validate(len(sketch)); err != nil {
		return 0, err
	}
	var key BucketKey
	for i := 0; i < c.PrefixBits; i++ {
		if sketch[i] > 1 {
			return 0, fmt.Errorf("mpc: sketch bit %d is %d, expected 0 or 1", i, sketch[i])
		}
		key = key<<1 | BucketKey(sketch[i])
	}
	return key, nil
}

// MultiProbe returns the bucket to search plus the neighbours worth probing,
// nearest first.
//
// A single-bit flip in the prefix moves a person to a different bucket, and
// with k prefix bits there are k such neighbours. Probing them recovers the
// duplicates that noise would otherwise hide, at a cost of one extra bucket
// per probe. probes=0 searches only the exact bucket.
//
// Ordering matters: neighbours are returned by flipping the LAST prefix bit
// first. Sketch bits are independent, so no position is inherently more
// fragile — but a stable, documented order makes the search reproducible,
// which an arbitrary map iteration would not.
func (c BucketConfig) MultiProbe(key BucketKey, probes int) []BucketKey {
	if probes < 0 {
		probes = 0
	}
	if probes > c.PrefixBits {
		probes = c.PrefixBits
	}
	out := make([]BucketKey, 0, probes+1)
	out = append(out, key)
	for i := 0; i < probes; i++ {
		out = append(out, key^(1<<uint(i)))
	}
	return out
}

// BucketedRegistry is a Registry that only returns plausible candidates.
//
// It wraps the storage a validator actually keeps: enrolments indexed by their
// public bucket key. The shares themselves stay exactly as private as before —
// bucketing changes WHICH comparisons happen, never what a party can see.
type BucketedRegistry struct {
	cfg     BucketConfig
	probes  int
	buckets map[BucketKey][]Enrollment
	total   int
}

// NewBucketedRegistry creates an empty registry.
//
// probes is how many neighbouring buckets to search alongside the exact one.
// Zero is fastest and misses the most; PrefixBits probes every single-bit
// neighbour. Two or three is the usual compromise, and the right value is a
// measurement against real capture noise, not a guess.
func NewBucketedRegistry(cfg BucketConfig, probes int) *BucketedRegistry {
	return &BucketedRegistry{cfg: cfg, probes: probes, buckets: map[BucketKey][]Enrollment{}}
}

// BucketedEnrollment is an enrolment together with its public routing key.
type BucketedEnrollment struct {
	Enrollment
	Key BucketKey
}

// Add stores an enrolment under its bucket key.
func (r *BucketedRegistry) Add(e BucketedEnrollment) error {
	if e.Template == nil {
		return fmt.Errorf("mpc: enrollment %q has no template", e.ID)
	}
	for _, existing := range r.buckets[e.Key] {
		if existing.ID == e.ID {
			return fmt.Errorf("mpc: enrollment %q already exists", e.ID)
		}
	}
	r.buckets[e.Key] = append(r.buckets[e.Key], e.Enrollment)
	r.total++
	return nil
}

// Total is how many enrolments are stored across all buckets.
func (r *BucketedRegistry) Total() int { return r.total }

// Candidates returns the enrolments worth comparing a newcomer against.
//
// This is the whole point of the package: the caller then runs SecureMatch
// against THESE, not against the population.
func (r *BucketedRegistry) Candidates(key BucketKey) []Enrollment {
	var out []Enrollment
	for _, probe := range r.cfg.MultiProbe(key, r.probes) {
		out = append(out, r.buckets[probe]...)
	}
	return out
}

// BucketSizes reports the population of every non-empty bucket, largest first.
//
// Operationally this is the number to watch. Bucketing only helps while
// buckets stay small; if the sketch distribution is skewed, one bucket grows
// until searching it costs as much as a linear scan — and the failure is
// gradual and silent, showing up as registrations that slowly take longer
// rather than as an error. Alerting on the largest bucket catches that early.
func (r *BucketedRegistry) BucketSizes() []int {
	sizes := make([]int, 0, len(r.buckets))
	for _, v := range r.buckets {
		sizes = append(sizes, len(v))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
	return sizes
}

// EstimateComparisons predicts how many secure comparisons one registration
// costs against a registry of the given size, assuming sketches spread evenly.
//
// Even spread is optimistic: real biometric distributions are not uniform. Use
// it to see whether a configuration is in the right ballpark, and BucketSizes
// to see what is actually happening.
func (c BucketConfig) EstimateComparisons(population int, probes int) float64 {
	buckets := float64(uint64(1) << uint(c.PrefixBits))
	perBucket := float64(population) / buckets
	return perBucket * float64(probes+1)
}
