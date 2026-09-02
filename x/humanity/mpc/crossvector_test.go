package mpc

import (
	"fmt"
	"testing"
)

// The vector the Python capture pipeline is pinned against.
//
// The matching service has to produce bit-identical sketches, or two captures
// of the same person land in different buckets, nobody is ever compared, and
// the duplicate check approves everyone while looking perfectly healthy.
//
// Both sides assert these same literals. Pinning only the Python side would let
// a change here silently move the sketch space and break the pipeline; pinning
// both means either side drifting is a failing test rather than a system that
// quietly stops recognising anyone.
//
// Its counterpart is matching-service/tests/test_mpc_client.py.
const (
	crossSeed = "cross-language-v1"
	crossBits = 64
	crossDim  = 8

	crossSketch = "0001001010100000101001011110111101111000010001000011001101111101"
)

var (
	crossPositions = [][]int{
		{55, 13, 6, 18, 14, 7},
		{35, 37, 45, 50, 63, 12},
		{49, 29, 1, 40, 21, 30},
	}
	crossKeys = []BucketKey{44, 46, 19}
)

func crossEmbedding() []float64 {
	emb := make([]float64, crossDim)
	for i := range emb {
		emb[i] = float64(i+1) * 0.125
		if i%2 == 1 {
			emb[i] = -emb[i]
		}
	}
	return emb
}

func TestCrossLanguageVectorIsStable(t *testing.T) {
	p, err := NewProjections(crossSeed, crossBits, crossDim)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := p.Sketch(crossEmbedding())
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, b := range sk {
		got += fmt.Sprintf("%d", b)
	}
	if got != crossSketch {
		t.Errorf("the sketch changed.\n  was: %s\n  now: %s\n"+
			"Every enrolment already stored was filed under the old sketch space and becomes "+
			"unfindable. If this change is intended, both this constant and the matching "+
			"service's copy must move together, and existing enrolments must be re-indexed.",
			crossSketch, got)
	}

	cfg, err := NewMultiTableConfig(len(crossPositions), len(crossPositions[0]), crossSeed, crossBits)
	if err != nil {
		t.Fatal(err)
	}
	for tbl := range crossPositions {
		for i, want := range crossPositions[tbl] {
			if cfg.positions[tbl][i] != want {
				t.Fatalf("table %d position %d is %d, want %d — the index would file enrolments "+
					"under one key and search for them under another",
					tbl, i, cfg.positions[tbl][i], want)
			}
		}
	}

	keys, err := cfg.KeysFor(sk)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range crossKeys {
		if keys[i] != want {
			t.Errorf("bucket key %d is %d, want %d", i, keys[i], want)
		}
	}
}
