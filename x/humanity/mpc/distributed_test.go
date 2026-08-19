package mpc

import (
	"context"
	"math/rand"
	"sync"
	"testing"
)

// runParties executes one comparison with each party in its OWN goroutine,
// holding only its own shares. That is the point: if any party needed another
// party's row, this would deadlock or produce a wrong answer rather than
// quietly working the way an all-shares-in-one-process test does.
func runParties(t *testing.T, parties int, candidate []uint8, enrolled [][]uint8, threshold int) []MatchResult {
	t.Helper()

	candRows, err := SplitTemplateForParties(candidate, parties)
	if err != nil {
		t.Fatal(err)
	}
	enrolledRows := make([][]PartyTemplate, len(enrolled))
	for i, e := range enrolled {
		rows, err := SplitTemplateForParties(e, parties)
		if err != nil {
			t.Fatal(err)
		}
		enrolledRows[i] = rows
	}

	transports, err := NewLocalTransports(parties)
	if err != nil {
		t.Fatal(err)
	}
	budget := TriplesForManyComparison(len(candidate), len(enrolled)) + 4096
	triples, err := GenerateTriples(budget, parties)
	if err != nil {
		t.Fatal(err)
	}

	results := make([][]MatchResult, parties)
	errs := make([]error, parties)
	var wg sync.WaitGroup
	for p := 0; p < parties; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			mine := make([]PartyTemplate, len(enrolledRows))
			for i := range enrolledRows {
				mine[i] = enrolledRows[i][p]
			}
			m := &DistributedMatcher{
				Session:   NewSession(transports[p], NewTripleStore(triples[p])),
				Threshold: threshold,
			}
			results[p], errs[p] = m.CompareMany(context.Background(), candRows[p], mine)
		}(p)
	}
	wg.Wait()

	for p, err := range errs {
		if err != nil {
			t.Fatalf("party %d: %v", p, err)
		}
	}
	// Every party must reach the SAME public decision.
	for p := 1; p < parties; p++ {
		if len(results[p]) != len(results[0]) {
			t.Fatalf("party %d returned %d results, party 0 returned %d", p, len(results[p]), len(results[0]))
		}
		for k := range results[0] {
			if results[p][k].Similar != results[0][k].Similar {
				t.Fatalf("parties disagree on candidate %d: party0=%v party%d=%v",
					k, results[0][k].Similar, p, results[p][k].Similar)
			}
		}
	}
	return results[0]
}

// TestDistributedAgreesWithPlaintext is the end-to-end claim: parties holding
// nothing but random-looking rows reach the decision the plaintext distance
// implies.
func TestDistributedAgreesWithPlaintext(t *testing.T) {
	const length = 16
	rng := rand.New(rand.NewSource(313))

	for _, parties := range []int{2, 3} {
		cand := make([]uint8, length)
		for i := range cand {
			cand[i] = uint8(rng.Intn(2))
		}
		var enrolled [][]uint8
		for k := 0; k < 4; k++ {
			v := make([]uint8, length)
			copy(v, cand)
			for f := 0; f < k*3; f++ {
				v[f%length] ^= 1
			}
			enrolled = append(enrolled, v)
		}

		const threshold = 4
		got := runParties(t, parties, cand, enrolled, threshold)
		for k, e := range enrolled {
			want := plainHamming(cand, e) < threshold
			if got[k].Similar != want {
				t.Errorf("parties=%d candidate=%d: got %v, plaintext distance %d vs threshold %d says %v",
					parties, k, got[k].Similar, plainHamming(cand, e), threshold, want)
			}
		}
	}
}

// TestDistributedAgreesWithLocal pins that the distributed path and the
// in-process reference decide identically. Two implementations of the rule that
// decides who counts as a person must never diverge.
func TestDistributedAgreesWithLocal(t *testing.T) {
	const length, parties = 24, 2
	rng := rand.New(rand.NewSource(414))

	for trial := 0; trial < 5; trial++ {
		cand := make([]uint8, length)
		for i := range cand {
			cand[i] = uint8(rng.Intn(2))
		}
		other := make([]uint8, length)
		for i := range other {
			other[i] = uint8(rng.Intn(2))
		}
		threshold := rng.Intn(length + 1)

		dist := runParties(t, parties, cand, [][]uint8{other}, threshold)

		ta, err := NewSharedTemplate(cand, parties)
		if err != nil {
			t.Fatal(err)
		}
		tb, err := NewSharedTemplate(other, parties)
		if err != nil {
			t.Fatal(err)
		}
		local, err := SecureMatch(ta, tb, threshold, storesFor2(t, TriplesPerComparison(length)+64, parties))
		if err != nil {
			t.Fatal(err)
		}
		if dist[0].Similar != local.Similar {
			t.Fatalf("threshold=%d: distributed says %v, in-process reference says %v",
				threshold, dist[0].Similar, local.Similar)
		}
	}
}

// TestOnePartyLearnsNothing is the security claim, checked as directly as a
// test can: one party's row must not resemble the template it came from.
func TestOnePartyLearnsNothing(t *testing.T) {
	sketch := make([]uint8, 64)
	for i := range sketch {
		sketch[i] = 1 // an extreme input: every bit set
	}
	rows, err := SplitTemplateForParties(sketch, 2)
	if err != nil {
		t.Fatal(err)
	}
	for p, row := range rows {
		binaryLooking := 0
		for _, v := range row {
			if v == 0 || v == 1 {
				binaryLooking++
			}
		}
		if binaryLooking > len(row)/2 {
			t.Errorf("party %d's row has %d/%d values that look like plaintext bits — "+
				"the row should be uniform field elements, not a recognisable template",
				p, binaryLooking, len(row))
		}
	}
	// And the rows must still reconstruct to the original.
	for j := range sketch {
		var acc Element
		for _, row := range rows {
			acc = Add(acc, row[j])
		}
		if acc != Element(sketch[j]) {
			t.Fatalf("feature %d reconstructs to %d, want %d", j, acc, sketch[j])
		}
	}
}

// TestRoundCountIsIndependentOfCandidates is the scaling property, asserted
// rather than assumed: comparing against ten enrolments must not cost ten times
// the round trips of comparing against one.
func TestRoundCountIsIndependentOfCandidates(t *testing.T) {
	const length, parties = 16, 2

	roundsFor := func(candidates int) int {
		cand := make([]uint8, length)
		enrolled := make([][]uint8, candidates)
		for i := range enrolled {
			enrolled[i] = make([]uint8, length)
		}
		candRows, _ := SplitTemplateForParties(cand, parties)
		rows := make([][]PartyTemplate, candidates)
		for i := range enrolled {
			rows[i], _ = SplitTemplateForParties(enrolled[i], parties)
		}
		transports, _ := NewLocalTransports(parties)
		triples, _ := GenerateTriples(TriplesForManyComparison(length, candidates)+4096, parties)

		sessions := make([]*Session, parties)
		var wg sync.WaitGroup
		for p := 0; p < parties; p++ {
			sessions[p] = NewSession(transports[p], NewTripleStore(triples[p]))
			wg.Add(1)
			go func(p int) {
				defer wg.Done()
				mine := make([]PartyTemplate, candidates)
				for i := range rows {
					mine[i] = rows[i][p]
				}
				m := &DistributedMatcher{Session: sessions[p], Threshold: 4}
				if _, err := m.CompareMany(context.Background(), candRows[p], mine); err != nil {
					t.Errorf("party %d: %v", p, err)
				}
			}(p)
		}
		wg.Wait()
		return sessions[0].Rounds()
	}

	one := roundsFor(1)
	ten := roundsFor(10)
	t.Logf("rounds: 1 candidate = %d, 10 candidates = %d", one, ten)
	if ten != one {
		t.Errorf("round count grew from %d to %d with the candidate count — batching across "+
			"candidates is not working, and a crowded bucket becomes a slow registration", one, ten)
	}
}
