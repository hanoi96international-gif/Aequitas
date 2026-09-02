package mpc

import (
	"fmt"
	"math/rand"
	"testing"
)

// TestMultiTableRecallBeatsSingleTable is the measurement this file exists for.
// A single wide prefix loses most returning people to noise; several
// independent tables recover them.
func TestMultiTableRecallBeatsSingleTable(t *testing.T) {
	const dim, bits = 512, 512
	p, err := NewProjections("multitable-recall", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(202))

	measure := func(cfg *MultiTableConfig, theta float64, trials int) float64 {
		found := 0
		for i := 0; i < trials; i++ {
			base := unitVector(rng, dim)
			other := unitVector(rng, dim)
			re := rotateToward(base, other, theta)

			bs, _ := p.Sketch(base)
			rs, _ := p.Sketch(re)
			bk, _ := cfg.KeysFor(bs)
			rk, _ := cfg.KeysFor(rs)
			for tbl := range bk {
				if bk[tbl] == rk[tbl] {
					found++
					break
				}
			}
		}
		return float64(found) / float64(trials)
	}

	const theta = 0.25 // a realistic same-person spread
	single, err := NewMultiTableConfig(1, 27, "study", bits)
	if err != nil {
		t.Fatal(err)
	}
	many, err := NewMultiTableConfig(20, 27, "study", bits)
	if err != nil {
		t.Fatal(err)
	}

	singleRecall := measure(single, theta, 300)
	manyRecall := measure(many, theta, 300)

	t.Logf("27-bit keys, same-person angle %.2f rad", theta)
	t.Logf("  1 table  : %.1f%% recall, %.0f candidates at 8e9", singleRecall*100, single.ExpectedCandidates(8e9))
	t.Logf(" 20 tables : %.1f%% recall, %.0f candidates at 8e9", manyRecall*100, many.ExpectedCandidates(8e9))

	if manyRecall <= singleRecall {
		t.Errorf("20 tables (%.1f%%) did not beat 1 table (%.1f%%) — the tables are not "+
			"independent, so a flip that breaks one breaks them all", manyRecall*100, singleRecall*100)
	}
	if manyRecall < 0.80 {
		t.Errorf("recall with 20 tables is %.1f%%, which still loses one duplicate in five. "+
			"More tables or a narrower key is needed before this is usable.", manyRecall*100)
	}
}

// TestExpectedRecallTracksReality keeps the planning formula honest: if it
// predicts something the measurement contradicts, capacity planning built on it
// would be fiction.
func TestExpectedRecallTracksReality(t *testing.T) {
	const dim, bits = 256, 256
	p, err := NewProjections("recall-formula", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(77))

	cfg, err := NewMultiTableConfig(8, 12, "formula", bits)
	if err != nil {
		t.Fatal(err)
	}

	const theta = 0.25
	flip := theta / 3.14159265358979
	predicted := cfg.ExpectedRecall(flip)

	found, trials := 0, 400
	for i := 0; i < trials; i++ {
		base := unitVector(rng, dim)
		other := unitVector(rng, dim)
		re := rotateToward(base, other, theta)
		bs, _ := p.Sketch(base)
		rs, _ := p.Sketch(re)
		bk, _ := cfg.KeysFor(bs)
		rk, _ := cfg.KeysFor(rs)
		for tbl := range bk {
			if bk[tbl] == rk[tbl] {
				found++
				break
			}
		}
	}
	actual := float64(found) / float64(trials)

	t.Logf("predicted recall %.1f%%, measured %.1f%%", predicted*100, actual*100)
	if actual < predicted-0.15 || actual > predicted+0.15 {
		t.Errorf("the recall formula is off by more than 15 points (predicted %.1f%%, measured %.1f%%); "+
			"capacity planning based on it would be wrong", predicted*100, actual*100)
	}
}

func TestMultiTableRegistryReturnsUnionWithoutDuplicates(t *testing.T) {
	cfg, err := NewMultiTableConfig(3, 8, "reg", 64)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewMultiTableRegistry(cfg)

	tmpl, err := NewSharedTemplate([]uint8{1, 0, 1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Same enrolment matches in two of the three tables: it must appear once.
	if err := reg.Add(Enrollment{ID: "twice", Template: tmpl}, []BucketKey{5, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(Enrollment{ID: "once", Template: tmpl}, []BucketKey{7, 7, 5}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(Enrollment{ID: "never", Template: tmpl}, []BucketKey{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Candidates([]BucketKey{5, 5, 5})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for _, e := range got {
		ids[e.ID]++
	}
	if ids["twice"] != 1 {
		t.Errorf("enrolment matching two tables appeared %d times, want exactly 1", ids["twice"])
	}
	if ids["once"] != 1 {
		t.Errorf("enrolment matching one table appeared %d times, want 1", ids["once"])
	}
	if ids["never"] != 0 {
		t.Error("an enrolment sharing no bucket was returned")
	}
}

func TestMultiTableRemoveClearsEveryTable(t *testing.T) {
	cfg, err := NewMultiTableConfig(3, 8, "rm", 64)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewMultiTableRegistry(cfg)
	tmpl, _ := NewSharedTemplate([]uint8{1, 1}, 3)
	keys := []BucketKey{4, 4, 4}
	if err := reg.Add(Enrollment{ID: "gone", Template: tmpl}, keys); err != nil {
		t.Fatal(err)
	}
	if err := reg.Remove("gone"); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Candidates(keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a removed enrolment is still findable in %d table(s) — withdrawal of consent "+
			"could not be honoured", len(got))
	}
	if reg.Size() != 0 {
		t.Errorf("Size() = %d after removal, want 0", reg.Size())
	}
}

func TestMultiTableConfigRejectsUnusableSettings(t *testing.T) {
	if _, err := NewMultiTableConfig(0, 8, "s", 64); err == nil {
		t.Error("zero tables must be refused")
	}
	if _, err := NewMultiTableConfig(2, 0, "s", 64); err == nil {
		t.Error("zero key bits must be refused")
	}
	if _, err := NewMultiTableConfig(2, 64, "s", 64); err == nil {
		t.Error("a key as wide as the sketch must be refused")
	}
	if _, err := NewMultiTableConfig(2, 8, "", 64); err == nil {
		t.Error("an empty seed must be refused")
	}
}

// TestPlanningTable prints the configuration space so a deployment choice is
// made from numbers rather than intuition.
func TestPlanningTable(t *testing.T) {
	const population = 8e9
	fmt.Printf("\n=== choosing an index for 8e9 people (bit-flip 8%%, realistic) ===\n")
	fmt.Printf("%-8s %-8s %-12s %-14s\n", "tables", "keybits", "recall", "candidates")
	for _, tables := range []int{8, 20, 40} {
		for _, bits := range []int{24, 27, 30} {
			cfg, err := NewMultiTableConfig(tables, bits, "planning", 512)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Printf("%-8d %-8d %-12.1f%% %-14.0f\n",
				tables, bits, cfg.ExpectedRecall(0.08)*100, cfg.ExpectedCandidates(int(population)))
		}
	}
}
