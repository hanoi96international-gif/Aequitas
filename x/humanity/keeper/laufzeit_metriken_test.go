package keeper

import (
	"math"
	"runtime/metrics"
	"testing"
)

func TestHistoQuantile(t *testing.T) {
	h := &metrics.Float64Histogram{
		Counts:  []uint64{90, 9, 1},
		Buckets: []float64{0, 0.001, 0.01, math.Inf(1)},
	}
	if got := histoQuantile(h, 0.5); got != 0.001 {
		t.Fatalf("p50 = %v", got)
	}
	if got := histoQuantile(h, 0.99); got != 0.01 {
		t.Fatalf("p99 = %v", got)
	}
	// Letzter Eimer offen: unterer Rand statt +Inf.
	if got := histoQuantile(h, 1); got != 0.01 {
		t.Fatalf("max = %v", got)
	}
	if got := histoQuantile(&metrics.Float64Histogram{Counts: []uint64{0}, Buckets: []float64{0, 1}}, 0.5); got != 0 {
		t.Fatalf("leer = %v", got)
	}
}

func TestLaufzeitStand_HatKennzahlen(t *testing.T) {
	s := LaufzeitStand()
	for _, k := range []string{"sched_p99_ms", "gc_pause_max_ms", "gc_cpu_s", "goroutinen"} {
		if _, ok := s[k]; !ok {
			t.Fatalf("%s fehlt: %v", k, s)
		}
	}
}
