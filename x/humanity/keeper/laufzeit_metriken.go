package keeper

import (
	"math"
	"runtime/metrics"
)

// Laufzeit-Kennzahlen ohne Stop-the-world (runtime/metrics statt
// ReadMemStats).
//
// Pruefstand Lauf 13 (02.10.2026): einzelne Bloecke brauchen 0,5-1,5 s, und
// zwar in Schritten, die nichts miteinander teilen -- eine reine
// CPU-Rechnung (JSON + Komprimierung: Mittel 18 ms, Spitze 728 ms) ebenso
// wie eine Postgres-Abfrage auf eine Zeile (Mittel 6 ms, Spitze 556 ms). Die
// Sperren sind es nicht (cs.mu: hoechstens 7 ms). Bleibt: der Prozess als
// Ganzes steht -- Scheduler (lauffaehig, aber kein Kern) oder Garbage
// Collector (Pausen, Mithilfe beim Markieren). Diese Zahlen trennen beides.

var laufzeitProben = []metrics.Sample{
	{Name: "/sched/latencies:seconds"},
	{Name: "/gc/pauses:seconds"},
	{Name: "/cpu/classes/gc/total:cpu-seconds"},
	{Name: "/cpu/classes/total:cpu-seconds"},
	{Name: "/gc/cycles/total:gc-cycles"},
	{Name: "/sched/goroutines:goroutines"},
}

// histoQuantile: oberer Rand des Eimers, in dem das Quantil q liegt (s).
func histoQuantile(h *metrics.Float64Histogram, q float64) float64 {
	if h == nil {
		return 0
	}
	var total uint64
	for _, c := range h.Counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	ziel := uint64(math.Ceil(q * float64(total)))
	var lauf uint64
	for i, c := range h.Counts {
		lauf += c
		if lauf >= ziel {
			o := h.Buckets[i+1]
			if math.IsInf(o, 1) {
				o = h.Buckets[i]
			}
			return o
		}
	}
	return 0
}

// LaufzeitStand fuer /api/health/combined. Werte seit Prozessstart.
func LaufzeitStand() map[string]interface{} {
	proben := make([]metrics.Sample, len(laufzeitProben))
	copy(proben, laufzeitProben)
	metrics.Read(proben)
	aus := map[string]interface{}{}
	ms := func(s float64) float64 { return math.Round(s*1e6) / 1e3 }
	for _, p := range proben {
		switch p.Name {
		case "/sched/latencies:seconds":
			if p.Value.Kind() == metrics.KindFloat64Histogram {
				h := p.Value.Float64Histogram()
				aus["sched_p50_ms"] = ms(histoQuantile(h, 0.50))
				aus["sched_p99_ms"] = ms(histoQuantile(h, 0.99))
				aus["sched_p999_ms"] = ms(histoQuantile(h, 0.999))
			}
		case "/gc/pauses:seconds":
			if p.Value.Kind() == metrics.KindFloat64Histogram {
				h := p.Value.Float64Histogram()
				aus["gc_pause_p99_ms"] = ms(histoQuantile(h, 0.99))
				aus["gc_pause_max_ms"] = ms(histoQuantile(h, 1))
			}
		case "/cpu/classes/gc/total:cpu-seconds":
			if p.Value.Kind() == metrics.KindFloat64 {
				aus["gc_cpu_s"] = math.Round(p.Value.Float64()*10) / 10
			}
		case "/cpu/classes/total:cpu-seconds":
			if p.Value.Kind() == metrics.KindFloat64 {
				aus["cpu_gesamt_s"] = math.Round(p.Value.Float64()*10) / 10
			}
		case "/gc/cycles/total:gc-cycles":
			if p.Value.Kind() == metrics.KindUint64 {
				aus["gc_zyklen"] = p.Value.Uint64()
			}
		case "/sched/goroutines:goroutines":
			if p.Value.Kind() == metrics.KindUint64 {
				aus["goroutinen"] = p.Value.Uint64()
			}
		}
	}
	return aus
}
