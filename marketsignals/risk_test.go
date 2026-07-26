package marketsignals

import (
	"math"
	"strings"
	"testing"
)

func riskView() View {
	s := randomWalkSeries(200, 23, 0.01)
	return NewView(s, len(s.Candles))
}

func TestRisk_SizesInverselyToVolatility(t *testing.T) {
	r := NewRiskManager()
	sig := Signal{Dir: Long, Strength: 1}

	calm := NewView(randomWalkSeries(200, 31, 0.005), 200)
	wild := NewView(randomWalkSeries(200, 31, 0.05), 200)

	quiet := r.Size(sig, calm, 0)
	loud := r.Size(sig, wild, 0)

	if !(quiet.Target > loud.Target) {
		t.Fatalf("calm market sized %.4f, volatile market sized %.4f — volatility targeting "+
			"must shrink the position when the market gets violent", quiet.Target, loud.Target)
	}
}

func TestRisk_RespectsLeverageCap(t *testing.T) {
	r := NewRiskManager()
	r.MaxLeverage = 0.5
	// A market so quiet that naive vol targeting would demand enormous size.
	flat := NewView(flatSeries(200, 0.001), 200)

	got := r.Size(Signal{Dir: Long, Strength: 1}, flat, 0)
	if math.Abs(got.Target) > r.MaxLeverage+1e-12 {
		t.Fatalf("position %.4f exceeds the %.2f leverage cap", got.Target, r.MaxLeverage)
	}
}

// TestRisk_FloorsVolatilityEstimate guards the specific way vol targeting
// blows up: a quiet stretch drives the denominator toward zero and the
// position toward infinity, immediately before the move that ends the quiet
// stretch.
func TestRisk_FloorsVolatilityEstimate(t *testing.T) {
	r := NewRiskManager()
	r.MaxLeverage = 1000 // remove the cap so only the vol floor is under test

	dead := NewView(flatSeries(200, 0), 200) // literally zero realised volatility
	got := r.Size(Signal{Dir: Long, Strength: 1}, dead, 0)

	want := (r.TargetVol / r.MinVol) * r.KellyFraction
	if math.Abs(got.Target-want) > 1e-9 {
		t.Fatalf("zero-volatility market sized %.4f, want %.4f from the MinVol floor", got.Target, want)
	}
}

func TestRisk_ShortsAreNegative(t *testing.T) {
	r := NewRiskManager()
	got := r.Size(Signal{Dir: Short, Strength: 1}, riskView(), 0)
	if got.Target >= 0 {
		t.Fatalf("short signal produced a non-negative position %.4f", got.Target)
	}
}

func TestRisk_KillSwitchOverridesConviction(t *testing.T) {
	r := NewRiskManager()
	sig := Signal{Dir: Long, Strength: 1} // maximum conviction

	if got := r.Size(sig, riskView(), r.MaxDrawdown-0.001); got.Target == 0 {
		t.Fatal("kill switch fired before the drawdown limit was reached")
	}
	got := r.Size(sig, riskView(), r.MaxDrawdown)
	if got.Target != 0 {
		t.Fatalf("position %.4f at the drawdown limit — the kill switch must outrank "+
			"every signal, especially a confident one", got.Target)
	}
	if !strings.Contains(got.Reason, "kill switch") {
		t.Fatalf("reason %q should name the kill switch", got.Reason)
	}
}

func TestRisk_FlatSignalTakesNoPosition(t *testing.T) {
	r := NewRiskManager()
	if got := r.Size(Signal{Dir: Flat, Strength: 1}, riskView(), 0); got.Target != 0 {
		t.Fatalf("flat signal produced position %.4f", got.Target)
	}
}

func TestRisk_ScalesWithConviction(t *testing.T) {
	r := NewRiskManager()
	v := riskView()
	weak := r.Size(Signal{Dir: Long, Strength: 0.25}, v, 0)
	strong := r.Size(Signal{Dir: Long, Strength: 1.0}, v, 0)
	if !(strong.Target > weak.Target) {
		t.Fatalf("conviction 1.0 sized %.4f but conviction 0.25 sized %.4f", strong.Target, weak.Target)
	}
}
