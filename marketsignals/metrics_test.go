package marketsignals

import (
	"math"
	"testing"
)

func approx(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.6f, want %.6f (±%g)", label, got, want, tol)
	}
}

func TestIndicators_SMAAndEMA(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	approx(t, "SMA(5)", SMA(xs, 5), 3, 1e-12)
	approx(t, "SMA(2)", SMA(xs, 2), 4.5, 1e-12)
	if !math.IsNaN(SMA(xs, 6)) {
		t.Fatal("SMA over more points than exist must be NaN, not a partial average")
	}

	// EMA seeded with the SMA of the first n: with n == len(xs) it degenerates
	// to exactly that mean.
	approx(t, "EMA(5)", EMA(xs, 5), 3, 1e-12)
}

func TestIndicators_StdDevIsSampleNotPopulation(t *testing.T) {
	// Sample standard deviation of {2,4,4,4,5,5,7,9} is 2.13809..., the
	// population figure is 2. Using the wrong one biases every Sharpe and
	// every volatility target in the package.
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	approx(t, "StdDev", StdDev(xs, len(xs)), 2.138089935, 1e-9)
}

func TestIndicators_ZScoreAndPercentileRank(t *testing.T) {
	xs := []float64{10, 10, 10, 10, 20}
	if z := ZScore(xs, 5); !(z > 1.5) {
		t.Fatalf("ZScore = %.4f, want a clearly positive outlier", z)
	}
	if r := PercentileRank(xs, 5); r != 1 {
		t.Fatalf("PercentileRank of the largest value = %.4f, want 1", r)
	}

	flat := []float64{5, 5, 5, 5}
	if !math.IsNaN(ZScore(flat, 4)) {
		t.Fatal("ZScore over a zero-dispersion window must be NaN — a zero there reads as " +
			"'perfectly average' and silently enables signals")
	}
}

func TestIndicators_ATRIncludesGaps(t *testing.T) {
	cs := []Candle{
		{Open: 10, High: 10, Low: 10, Close: 10},
		// A bar whose own range is 1 but which gapped 5 above the prior close:
		// true range is 6, not 1.
		{Open: 15, High: 16, Low: 15, Close: 16},
	}
	approx(t, "ATR(1)", ATR(cs, 1), 6, 1e-12)
}

func TestMetrics_KnownReturnStream(t *testing.T) {
	// Alternating +10% / -5%, 100 bars: compounding to a known figure.
	rets := make([]float64, 100)
	for i := range rets {
		if i%2 == 0 {
			rets[i] = 0.10
		} else {
			rets[i] = -0.05
		}
	}
	m := ComputeMetrics(rets, 365)

	want := math.Pow(1.10*0.95, 50) - 1
	approx(t, "TotalReturn", m.TotalReturn, want, 1e-9)
	approx(t, "HitRate", m.HitRate, 0.5, 1e-12)
	approx(t, "ProfitFactor", m.ProfitFactor, 0.10*50/(0.05*50), 1e-12)
	if m.MaxDrawdown <= 0 {
		t.Fatal("a stream containing losses must show a drawdown")
	}
}

func TestMetrics_MaxDrawdownIsMeasuredFromThePeak(t *testing.T) {
	// Up 100%, then down 50% — back to the starting equity, but the drawdown
	// from the peak was half the account.
	m := ComputeMetrics([]float64{1.0, -0.5}, 365)
	approx(t, "TotalReturn", m.TotalReturn, 0, 1e-12)
	approx(t, "MaxDrawdown", m.MaxDrawdown, 0.5, 1e-12)
}

func TestMetrics_SharpeIsAnnualised(t *testing.T) {
	rets := make([]float64, 1000)
	for i := range rets {
		if i%2 == 0 {
			rets[i] = 0.01
		} else {
			rets[i] = -0.005
		}
	}
	hourly := ComputeMetrics(rets, 24*365)
	daily := ComputeMetrics(rets, 365)
	ratio := hourly.Sharpe / daily.Sharpe
	approx(t, "annualisation ratio", ratio, math.Sqrt(24), 1e-9)
}

func TestNormal_CDFAndInverseRoundTrip(t *testing.T) {
	approx(t, "normCDF(0)", normCDF(0), 0.5, 1e-12)
	approx(t, "normCDF(1.96)", normCDF(1.96), 0.975, 1e-4)
	approx(t, "normInv(0.975)", normInv(0.975), 1.959964, 1e-5)

	for _, p := range []float64{0.001, 0.01, 0.25, 0.5, 0.75, 0.99, 0.999} {
		approx(t, "round trip", normCDF(normInv(p)), p, 1e-8)
	}
}

// TestDeflatedSharpe_PunishesSelection encodes the arithmetic that makes
// SelectBest honest: the same observed Sharpe means much less once you admit
// how many strategies were tried to find it.
func TestDeflatedSharpe_PunishesSelection(t *testing.T) {
	const perBarSharpe = 0.05
	const n = 1000

	naive := DeflatedSharpe(perBarSharpe, 0, n, 0, 0)
	honest := DeflatedSharpe(perBarSharpe, ExpectedMaxSharpe(200, 0.02), n, 0, 0)

	if !(naive > honest) {
		t.Fatalf("confidence against a zero benchmark (%.4f) must exceed confidence against "+
			"a selection hurdle (%.4f)", naive, honest)
	}
	if naive < 0.9 {
		t.Fatalf("a Sharpe of %.2f over %d observations should be convincing against zero, got %.4f",
			perBarSharpe, n, naive)
	}
}

// TestDeflatedSharpe_PunishesFatLeftTails: two strategies with identical
// Sharpe are not equally trustworthy if one of them earns its returns by
// selling insurance against a crash.
func TestDeflatedSharpe_PunishesFatLeftTails(t *testing.T) {
	symmetric := DeflatedSharpe(0.05, 0, 1000, 0, 0)
	fatTailed := DeflatedSharpe(0.05, 0, 1000, -2.0, 8.0)
	if !(fatTailed < symmetric) {
		t.Fatalf("negatively skewed, fat-tailed returns (%.4f) must be discounted relative to "+
			"symmetric ones (%.4f) at the same Sharpe", fatTailed, symmetric)
	}
}

func TestMetrics_EmptyStreamIsZeroNotNaN(t *testing.T) {
	m := ComputeMetrics(nil, 365)
	if m.Bars != 0 || m.Sharpe != 0 || m.MaxDrawdown != 0 {
		t.Fatalf("metrics over no returns should be zero-valued, got %+v", m)
	}
}
