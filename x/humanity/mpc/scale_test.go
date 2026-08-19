package mpc

import (
	"fmt"
	"testing"
	"time"
)

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
