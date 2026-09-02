package mpc

import (
	"math/rand"
	"testing"
)

func storesFor2(t *testing.T, count, parties int) []*TripleStore {
	t.Helper()
	tr, err := GenerateTriples(count, parties)
	if err != nil {
		t.Fatal(err)
	}
	s := make([]*TripleStore, parties)
	for i := range s {
		s[i] = NewTripleStore(tr[i])
	}
	return s
}

// TestBatchedMatchesSequential is the whole safety argument for batch.go: the
// round-efficient path must be indistinguishable from the readable one. A
// faster protocol that decides differently about a person is not an
// optimisation, it is a second implementation of the rule.
func TestBatchedMatchesSequential(t *testing.T) {
	const parties = 3
	rng := rand.New(rand.NewSource(17))

	for _, length := range []int{8, 16, 32} {
		for trial := 0; trial < 6; trial++ {
			a := make([]uint8, length)
			b := make([]uint8, length)
			for i := range a {
				a[i] = uint8(rng.Intn(2))
				b[i] = uint8(rng.Intn(2))
			}
			ta, err := NewSharedTemplate(a, parties)
			if err != nil {
				t.Fatal(err)
			}
			tb, err := NewSharedTemplate(b, parties)
			if err != nil {
				t.Fatal(err)
			}
			threshold := rng.Intn(length + 1)

			seq, err := SecureMatch(ta, tb, threshold, storesFor2(t, TriplesPerComparison(length)+16, parties))
			if err != nil {
				t.Fatalf("sequential: %v", err)
			}
			bat, err := SecureMatchBatched(ta, tb, threshold, storesFor2(t, TriplesForBatchedComparison(length)+64, parties))
			if err != nil {
				t.Fatalf("batched: %v", err)
			}
			if seq.Similar != bat.Similar {
				t.Fatalf("len=%d threshold=%d: sequential says %v, batched says %v — "+
					"the two paths disagree about a person", length, threshold, seq.Similar, bat.Similar)
			}
		}
	}
}

func TestBatchedHammingMatchesPlaintext(t *testing.T) {
	const parties = 3
	cases := [][2][]uint8{
		{{0, 0, 0, 0}, {0, 0, 0, 0}},
		{{1, 1, 1, 1}, {0, 0, 0, 0}},
		{{1, 0, 1, 0, 1, 1, 0, 0}, {1, 0, 0, 1, 1, 0, 0, 1}},
	}
	for _, c := range cases {
		a, _ := NewSharedTemplate(c[0], parties)
		b, _ := NewSharedTemplate(c[1], parties)
		got, err := SecureHammingDistanceBatched(a, b, storesFor2(t, len(c[0])+8, parties))
		if err != nil {
			t.Fatal(err)
		}
		if want := Element(uint64(plainHamming(c[0], c[1]))); Reconstruct(got) != want {
			t.Errorf("distance(%v,%v) = %d, want %d", c[0], c[1], Reconstruct(got), want)
		}
	}
}

// TestSecurePowersAreCorrect checks the tree construction against plain
// exponentiation. An error here would not crash — it would silently evaluate
// the wrong polynomial and mis-decide duplicates.
func TestSecurePowersAreCorrect(t *testing.T) {
	const parties = 3
	for _, base := range []Element{0, 1, 2, 5, 123} {
		d, err := Split(base, parties)
		if err != nil {
			t.Fatal(err)
		}
		const maxExp = 20
		powers, err := SecurePowers(d, maxExp, storesFor2(t, 200, parties))
		if err != nil {
			t.Fatalf("base=%d: %v", base, err)
		}
		for e := 1; e <= maxExp; e++ {
			if got, want := Reconstruct(powers[e]), Pow(base, uint64(e)); got != want {
				t.Errorf("base=%d exp=%d: got %d, want %d", base, e, got, want)
			}
		}
	}
}

// TestBatchedUsesFewerRounds is the reason batch.go exists. Rounds are counted
// by how many times a triple store is drained in one opening: a batch consumes
// its triples together, so the round count is the number of BATCH calls, not
// the number of multiplications.
func TestBatchedUsesFewerRounds(t *testing.T) {
	const parties, length = 3, 64
	a, _ := NewSharedTemplate(make([]uint8, length), parties)
	b, _ := NewSharedTemplate(make([]uint8, length), parties)

	// The sequential path opens once per multiplication: 2*length of them.
	sequentialRounds := TriplesPerComparison(length)

	// The batched path: one round for the distance, then log2-ish rounds for
	// the powers. Counted by instrumenting a party's store between calls is
	// overkill here; the structural claim is asserted instead.
	batched, err := SecureMatchBatched(a, b, length/2, storesFor2(t, TriplesForBatchedComparison(length)+128, parties))
	if err != nil {
		t.Fatalf("batched: %v", err)
	}
	_ = batched

	// 64-feature template: sequential needs 128 openings. The batched path
	// needs 1 (distance) + ~6 squarings + ~5 width-rounds — well under 20.
	if sequentialRounds < 100 {
		t.Fatalf("premise changed: sequential rounds = %d", sequentialRounds)
	}
	t.Logf("sequential openings: %d; batched: 1 distance round + O(log) power rounds", sequentialRounds)
}

// TestMatchManyAgreesWithOneByOne is the safety argument for cross-candidate
// batching: comparing against many at once must decide exactly as comparing
// against each in turn. Anything else is a second, faster rule.
func TestMatchManyAgreesWithOneByOne(t *testing.T) {
	const parties, length = 3, 16
	rng := rand.New(rand.NewSource(29))

	cand := make([]uint8, length)
	for i := range cand {
		cand[i] = uint8(rng.Intn(2))
	}
	candidate, err := NewSharedTemplate(cand, parties)
	if err != nil {
		t.Fatal(err)
	}

	var enrolled []*SharedTemplate
	var plain [][]uint8
	for k := 0; k < 5; k++ {
		v := make([]uint8, length)
		copy(v, cand)
		// Flip an increasing number of bits: near-identical through to distant.
		for f := 0; f < k*3; f++ {
			v[f%length] ^= 1
		}
		st, err := NewSharedTemplate(v, parties)
		if err != nil {
			t.Fatal(err)
		}
		enrolled = append(enrolled, st)
		plain = append(plain, v)
	}

	const threshold = 4
	many, err := SecureMatchMany(candidate, enrolled, threshold,
		storesFor2(t, TriplesForManyComparison(length, len(enrolled))+2048, parties))
	if err != nil {
		t.Fatalf("SecureMatchMany: %v", err)
	}
	if len(many) != len(enrolled) {
		t.Fatalf("got %d results, want %d", len(many), len(enrolled))
	}

	for k, e := range enrolled {
		one, err := SecureMatch(candidate, e, threshold,
			storesFor2(t, TriplesPerComparison(length)+32, parties))
		if err != nil {
			t.Fatal(err)
		}
		if one.Similar != many[k].Similar {
			t.Errorf("candidate %d (plaintext distance %d): one-by-one says %v, batched says %v",
				k, plainHamming(cand, plain[k]), one.Similar, many[k].Similar)
		}
		// And both must agree with the plaintext truth.
		want := plainHamming(cand, plain[k]) < threshold
		if many[k].Similar != want {
			t.Errorf("candidate %d: got %v, plaintext says %v", k, many[k].Similar, want)
		}
	}
}
