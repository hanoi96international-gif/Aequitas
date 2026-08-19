package mpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorldScaleRegistrationOverTheNetwork is the measurement that decides
// whether any of this is deployable.
//
// Every earlier number in this package was taken in one process, where a
// "round" costs a mutex. A round in production costs a round trip between two
// machines, and the whole design — batching, the power tree, multi-table LSH —
// exists to keep the NUMBER of those round trips constant while the candidate
// count grows. This test puts the real transport under the real candidate load
// and reports what a person actually waits for.
//
// The candidate count comes from the index configuration, not from optimism:
// 20 tables with a 27-bit prefix over eight billion people yields roughly
// 8e9 / 2^27 * 20 ≈ 1200 candidates per lookup.
func TestWorldScaleRegistrationOverTheNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates and exchanges ~10 MB per round; skipped under -short")
	}

	const (
		sketchBits = 512
		candidates = 1200
		threshold  = 100
	)

	ta, tb, wire := twoValidators(t, "scale-test-token")

	cand := make([]uint8, sketchBits)
	for i := range cand {
		cand[i] = uint8(i % 2)
	}
	candRows, err := SplitTemplateForParties(cand, 2)
	if err != nil {
		t.Fatal(err)
	}

	// One enrolment template, reused for every candidate slot: the cost is in
	// the number of comparisons, not in their content.
	enrolRows, err := SplitTemplateForParties(cand, 2)
	if err != nil {
		t.Fatal(err)
	}

	needed := TriplesForManyComparison(sketchBits, candidates) + 8192
	offlineStart := time.Now()
	triples, err := GenerateTriples(needed, 2)
	if err != nil {
		t.Fatal(err)
	}
	offline := time.Since(offlineStart)

	transports := []Transport{ta, tb}
	sessions := make([]*Session, 2)
	errs := make([]error, 2)

	onlineStart := time.Now()
	var wg sync.WaitGroup
	for p := 0; p < 2; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			mine := make([]PartyTemplate, candidates)
			for i := range mine {
				mine[i] = enrolRows[p]
			}
			sessions[p] = NewSession(transports[p], NewTripleStore(triples[p]))
			m := &DistributedMatcher{Session: sessions[p], Threshold: threshold}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, errs[p] = m.CompareMany(ctx, candRows[p], mine)
		}(p)
	}
	wg.Wait()
	online := time.Since(onlineStart)

	for p, err := range errs {
		if err != nil {
			t.Fatalf("validator %d: %v", p, err)
		}
	}

	mb := float64(atomic.LoadInt64(wire)) / (1 << 20)
	t.Logf("candidates compared:      %d", candidates)
	t.Logf("features per comparison:  %d", sketchBits)
	t.Logf("multiplications:          %d", candidates*sketchBits)
	t.Logf("network rounds:           %d", sessions[0].Rounds())
	t.Logf("bytes on the wire:        %.1f MB", mb)
	t.Logf("offline (triples, precomputed, NOT part of the wait): %v", offline)
	t.Logf("ONLINE — what a person waits for:                     %v", online)

	// The round count is the property that survives a real network: on a link
	// with 50 ms of latency, 9 rounds is half a second and 9000 rounds is an
	// abandoned registration.
	if rounds := sessions[0].Rounds(); rounds > 32 {
		t.Errorf("%d network rounds for one registration. On a 50 ms link that is %.1f s of "+
			"pure latency before any computation — a batching path has stopped batching",
			rounds, float64(rounds)*0.05)
	}

	// Loopback has no real latency, so this bound is about compute and
	// serialisation only. It is deliberately loose; the point is to catch an
	// order-of-magnitude regression, not to pin a wall-clock figure that varies
	// with the machine.
	if online > 60*time.Second {
		t.Errorf("one registration took %v on loopback — too slow to put in front of a person", online)
	}
}

// TestRoundsDoNotGrowWithPopulation states the scaling property directly: ten
// times the candidates must not mean ten times the round trips, because round
// trips are the only cost a network actually charges for.
func TestRoundsDoNotGrowWithPopulation(t *testing.T) {
	const sketchBits = 128

	roundsFor := func(candidates int) int {
		ta, tb, _ := twoValidators(t, "rounds-token")
		cand := make([]uint8, sketchBits)
		rows, err := SplitTemplateForParties(cand, 2)
		if err != nil {
			t.Fatal(err)
		}
		triples, err := GenerateTriples(TriplesForManyComparison(sketchBits, candidates)+8192, 2)
		if err != nil {
			t.Fatal(err)
		}
		transports := []Transport{ta, tb}
		sessions := make([]*Session, 2)

		var wg sync.WaitGroup
		for p := 0; p < 2; p++ {
			wg.Add(1)
			go func(p int) {
				defer wg.Done()
				mine := make([]PartyTemplate, candidates)
				for i := range mine {
					mine[i] = rows[p]
				}
				sessions[p] = NewSession(transports[p], NewTripleStore(triples[p]))
				m := &DistributedMatcher{Session: sessions[p], Threshold: 20}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if _, err := m.CompareMany(ctx, rows[p], mine); err != nil {
					t.Errorf("validator %d: %v", p, err)
				}
			}(p)
		}
		wg.Wait()
		return sessions[0].Rounds()
	}

	few, many := roundsFor(10), roundsFor(500)
	t.Logf("rounds: 10 candidates = %d, 500 candidates = %d", few, many)
	if few != many {
		t.Errorf("round count went from %d to %d as candidates grew 50x — the cost that a real "+
			"network charges for is growing with the population", few, many)
	}
}

// TestFinalScaleNumbers is the in-process projection kept from the batching
// work: it reports what the round count means at several link latencies. The
// networked measurement above supersedes its timing figure but not its table —
// loopback cannot show what 50 ms per round costs.
func TestFinalScaleNumbers(t *testing.T) {
	const bits, parties, candidates = 512, 3, 179
	cand, _ := NewSharedTemplate(make([]uint8, bits), parties)
	enrolled := make([]*SharedTemplate, candidates)
	for i := range enrolled {
		enrolled[i], _ = NewSharedTemplate(make([]uint8, bits), parties)
	}

	// warm the polynomial cache
	_, _ = SecureMatchBatched(cand, cand, 100, storesFor2(t, TriplesForBatchedComparison(bits)+512, parties))

	st := storesFor2(t, TriplesForManyComparison(bits, candidates)+8192, parties)
	t0 := time.Now()
	res, err := SecureMatchMany(cand, enrolled, 100, st)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(t0)
	if len(res) != candidates {
		t.Fatalf("got %d results", len(res))
	}

	seqRounds := candidates * TriplesPerComparison(bits)
	perPairRounds := candidates * 21
	togetherRounds := 21

	fmt.Printf("\n=== ONE REGISTRATION at 8e9 people (27-bit prefix, 2 probes) ===\n")
	fmt.Printf("candidates compared        : %d\n", candidates)
	fmt.Printf("local online compute       : %v\n", elapsed)
	fmt.Printf("\nnetwork rounds\n")
	fmt.Printf("  sequential protocol      : %d\n", seqRounds)
	fmt.Printf("  batched per pair         : %d\n", perPairRounds)
	fmt.Printf("  batched across candidates: %d\n", togetherRounds)
	fmt.Printf("\ntotal wait, by link latency\n")
	for _, lat := range []float64{0.5, 5, 20, 50} {
		fmt.Printf("  %4.1f ms: %6.2f s\n", lat, elapsed.Seconds()+float64(togetherRounds)*lat/1000)
	}
}
