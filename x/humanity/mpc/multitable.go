package mpc

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/sha3"
)

// Multi-table LSH: the difference between a bucket index that works on paper
// and one that finds returning people.
//
// THE MEASUREMENT THAT FORCES THIS
//
// A single prefix has to be wide to keep buckets small at population scale —
// and the wider it is, the more likely capture noise flips one of its bits and
// sends a returning person to a different bucket, where nobody compares them.
// Measured on 512-bit sketches with a single table:
//
//	prefix 27, 2 probes, 4.8% bit-flip rate -> 35% of recaptures found
//	prefix 27, 2 probes, 8.0% bit-flip rate -> 12% of recaptures found
//
// Twelve percent recall means almost every duplicate walks through. Widening
// the probe count does not rescue it: at 27 probes recall is still 39%, and the
// candidate count has grown tenfold.
//
// The standard fix is not a better single hash — it is SEVERAL independent
// ones. A person is compared if ANY table puts them in the searched bucket, so
// with L tables the failure probability is (1-p)^L instead of (1-p):
//
//	p = 0.12, L =  8  -> 64% recall
//	p = 0.12, L = 20  -> 92% recall
//	p = 0.35, L =  8  -> 97% recall
//	p = 0.35, L = 20  -> 99.98% recall
//
// The cost is linear in L — L times the candidates — but candidate count is
// exactly what SecureMatchMany made cheap: it compares them all in the same
// ~21 network rounds. That is why these two pieces belong together; either one
// alone is impractical.
//
// WHY THE TABLES ARE INDEPENDENT
//
// Each table reads a DIFFERENT subset of sketch bits. The sketch comes from
// independent random hyperplanes (see binarize.go), so distinct bit positions
// are independent signals, and a noise flip that breaks one table's key leaves
// the others intact. Deriving the subsets from a seed keeps them stable across
// restarts and machines — the same requirement, and the same reason, as the
// projections themselves.

// MultiTableConfig describes L hash tables of k bits each.
type MultiTableConfig struct {
	Tables     int    // L: how many independent tables
	BitsPerKey int    // k: prefix width within each table
	Seed       string // fixes which bits each table reads; must never change
	SketchBits int    // total sketch length the tables draw from

	positions [][]int // [table][bit index into the sketch]
}

// NewMultiTableConfig derives the bit positions each table reads.
//
// Positions are drawn without replacement inside a table (reading one bit twice
// wastes key space) but independently across tables, so tables may overlap.
// Overlap costs a little independence and is unavoidable once L*k exceeds the
// sketch length; the validation below keeps that from becoming extreme.
func NewMultiTableConfig(tables, bitsPerKey int, seed string, sketchBits int) (*MultiTableConfig, error) {
	if tables <= 0 {
		return nil, fmt.Errorf("mpc: need at least one table, got %d", tables)
	}
	if bitsPerKey <= 0 || bitsPerKey > 32 {
		return nil, fmt.Errorf("mpc: bitsPerKey must be in 1..32, got %d", bitsPerKey)
	}
	if bitsPerKey >= sketchBits {
		return nil, fmt.Errorf("mpc: bitsPerKey %d must be smaller than the %d-bit sketch",
			bitsPerKey, sketchBits)
	}
	if seed == "" {
		return nil, fmt.Errorf("mpc: refusing an empty seed — it defines which bits every table " +
			"reads, and an empty default would silently make two deployments share an index layout")
	}

	cfg := &MultiTableConfig{Tables: tables, BitsPerKey: bitsPerKey, Seed: seed, SketchBits: sketchBits}
	shake := sha3.NewShake256()
	if _, err := shake.Write([]byte("aequitas-mpc-multitable-v1|" + seed)); err != nil {
		return nil, err
	}

	buf := make([]byte, 4)
	cfg.positions = make([][]int, tables)
	for tbl := 0; tbl < tables; tbl++ {
		used := make(map[int]bool, bitsPerKey)
		pos := make([]int, 0, bitsPerKey)
		for len(pos) < bitsPerKey {
			if _, err := shake.Read(buf); err != nil {
				return nil, err
			}
			p := int(binary.BigEndian.Uint32(buf) % uint32(sketchBits))
			if used[p] {
				continue
			}
			used[p] = true
			pos = append(pos, p)
		}
		cfg.positions[tbl] = pos
	}
	return cfg, nil
}

// KeysFor returns one bucket key per table for a plaintext sketch.
//
// As with the single-table key, this runs where the sketch is still in the
// clear. The keys travel with the shares as public routing information; no
// party ever reconstructs anything to compute them.
func (c *MultiTableConfig) KeysFor(sketch []uint8) ([]BucketKey, error) {
	if len(sketch) != c.SketchBits {
		return nil, fmt.Errorf("mpc: sketch has %d bits, this index is built for %d — "+
			"keys from different sketch lengths are not comparable", len(sketch), c.SketchBits)
	}
	keys := make([]BucketKey, c.Tables)
	for tbl, positions := range c.positions {
		var key BucketKey
		for _, p := range positions {
			if sketch[p] > 1 {
				return nil, fmt.Errorf("mpc: sketch bit %d is %d, expected 0 or 1", p, sketch[p])
			}
			key = key<<1 | BucketKey(sketch[p])
		}
		keys[tbl] = key
	}
	return keys, nil
}

// ExpectedRecall predicts the fraction of same-person recaptures this index
// finds, given the per-bit flip rate of the capture pipeline.
//
// Single-table recall is the chance that none of the k key bits flip,
// (1-flip)^k; across L independent tables the chance of failing in all of them
// is that raised to L. It is an estimate under an independence assumption the
// real world only approximates — measure against captures before trusting it
// for a threshold decision.
func (c *MultiTableConfig) ExpectedRecall(bitFlipRate float64) float64 {
	if bitFlipRate <= 0 {
		return 1
	}
	single := 1.0
	for i := 0; i < c.BitsPerKey; i++ {
		single *= 1 - bitFlipRate
	}
	missAll := 1.0
	for i := 0; i < c.Tables; i++ {
		missAll *= 1 - single
	}
	return 1 - missAll
}

// ExpectedCandidates predicts how many enrolments one lookup returns.
//
// L tables each contribute population / 2^k. Overlap between tables means the
// true count after de-duplication is somewhat lower, so this is a safe upper
// bound — which is the direction a capacity estimate should err in.
func (c *MultiTableConfig) ExpectedCandidates(population int) float64 {
	perTable := float64(population) / float64(uint64(1)<<uint(c.BitsPerKey))
	return perTable * float64(c.Tables)
}

// MultiTableRegistry stores enrolments in every table at once and returns the
// union of the buckets a candidate lands in.
type MultiTableRegistry struct {
	cfg    *MultiTableConfig
	tables []map[BucketKey][]string     // table -> key -> enrolment ids
	byID   map[string]Enrollment        // id -> enrolment
	keysOf map[string][]BucketKey       // id -> its key per table, for removal
}

// NewMultiTableRegistry creates an empty index.
func NewMultiTableRegistry(cfg *MultiTableConfig) *MultiTableRegistry {
	r := &MultiTableRegistry{
		cfg:    cfg,
		tables: make([]map[BucketKey][]string, cfg.Tables),
		byID:   map[string]Enrollment{},
		keysOf: map[string][]BucketKey{},
	}
	for i := range r.tables {
		r.tables[i] = map[BucketKey][]string{}
	}
	return r
}

// Add indexes an enrolment under its key in every table.
func (r *MultiTableRegistry) Add(e Enrollment, keys []BucketKey) error {
	if e.Template == nil {
		return fmt.Errorf("mpc: enrolment %q has no template", e.ID)
	}
	if len(keys) != r.cfg.Tables {
		return fmt.Errorf("mpc: got %d keys for %d tables — the enrolment was indexed against a "+
			"different configuration and would be findable only by accident", len(keys), r.cfg.Tables)
	}
	if _, exists := r.byID[e.ID]; exists {
		return fmt.Errorf("mpc: enrolment %q already exists", e.ID)
	}
	r.byID[e.ID] = e
	r.keysOf[e.ID] = keys
	for tbl, key := range keys {
		r.tables[tbl][key] = append(r.tables[tbl][key], e.ID)
	}
	return nil
}

// Candidates returns every enrolment sharing a bucket with these keys in at
// least one table, each listed once.
func (r *MultiTableRegistry) Candidates(keys []BucketKey) ([]Enrollment, error) {
	if len(keys) != r.cfg.Tables {
		return nil, fmt.Errorf("mpc: got %d keys for %d tables", len(keys), r.cfg.Tables)
	}
	seen := make(map[string]bool)
	var out []Enrollment
	for tbl, key := range keys {
		for _, id := range r.tables[tbl][key] {
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, r.byID[id])
		}
	}
	return out, nil
}

// Size is the number of distinct enrolments indexed.
func (r *MultiTableRegistry) Size() int { return len(r.byID) }

// Remove deletes an enrolment from every table.
//
// Present because deletion is not optional for biometric data: a person who
// withdraws consent, or whose enrolment is found to be wrong, must be
// removable — and an index that can only grow makes that impossible to honour.
func (r *MultiTableRegistry) Remove(id string) error {
	keys, ok := r.keysOf[id]
	if !ok {
		return fmt.Errorf("mpc: enrolment %q is not indexed", id)
	}
	for tbl, key := range keys {
		ids := r.tables[tbl][key]
		for i, existing := range ids {
			if existing == id {
				r.tables[tbl][key] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
		if len(r.tables[tbl][key]) == 0 {
			delete(r.tables[tbl], key)
		}
	}
	delete(r.byID, id)
	delete(r.keysOf, id)
	return nil
}
