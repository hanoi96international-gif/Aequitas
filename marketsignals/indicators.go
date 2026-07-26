package marketsignals

import "math"

// Indicators here all take an already-truncated slice — the View has removed
// the future before they ever run — and all return a single value for the
// most recent point rather than a full series. That shape is deliberate: an
// indicator that returns a whole array invites a caller to index it at i+1.

// SMA is the mean of the last n values. Returns NaN if there are fewer than n.
func SMA(xs []float64, n int) float64 {
	if n <= 0 || len(xs) < n {
		return math.NaN()
	}
	sum := 0.0
	for _, x := range xs[len(xs)-n:] {
		sum += x
	}
	return sum / float64(n)
}

// EMA is the exponentially weighted mean with the conventional 2/(n+1) decay,
// seeded with the SMA of the first n points so the value does not depend on
// how much unrelated history happens to precede the window.
func EMA(xs []float64, n int) float64 {
	if n <= 0 || len(xs) < n {
		return math.NaN()
	}
	k := 2.0 / float64(n+1)
	ema := SMA(xs[:n], n)
	for _, x := range xs[n:] {
		ema = x*k + ema*(1-k)
	}
	return ema
}

// StdDev is the sample standard deviation of the last n values.
func StdDev(xs []float64, n int) float64 {
	if n <= 1 || len(xs) < n {
		return math.NaN()
	}
	w := xs[len(xs)-n:]
	mean := 0.0
	for _, x := range w {
		mean += x
	}
	mean /= float64(n)
	ss := 0.0
	for _, x := range w {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(n-1))
}

// LogReturns converts a price path into log returns, which are what every
// volatility and Sharpe calculation in this package expects: they add across
// time, so a bar-level standard deviation scales by sqrt(bars) into an
// annualised number without the compounding bias simple returns carry.
func LogReturns(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}
	out := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		out[i-1] = math.Log(prices[i] / prices[i-1])
	}
	return out
}

// RealizedVol is the annualised standard deviation of the last n log returns.
func RealizedVol(prices []float64, n int, barsPerYear float64) float64 {
	rets := LogReturns(prices)
	sd := StdDev(rets, n)
	if math.IsNaN(sd) {
		return math.NaN()
	}
	return sd * math.Sqrt(barsPerYear)
}

// ZScore expresses the final value of xs in standard deviations from the mean
// of the preceding n-value window. NaN when the window is degenerate (zero
// dispersion), which callers must treat as "no signal" — not as zero.
func ZScore(xs []float64, n int) float64 {
	if len(xs) < n || n <= 1 {
		return math.NaN()
	}
	mean := SMA(xs, n)
	sd := StdDev(xs, n)
	if sd == 0 || math.IsNaN(sd) {
		return math.NaN()
	}
	return (xs[len(xs)-1] - mean) / sd
}

// TrueRange is the classic Wilder measure: the bar's own range, extended to
// include any gap away from the previous close.
func TrueRange(prev, cur Candle) float64 {
	hl := cur.High - cur.Low
	hc := math.Abs(cur.High - prev.Close)
	lc := math.Abs(cur.Low - prev.Close)
	return math.Max(hl, math.Max(hc, lc))
}

// ATR is the simple average true range over the last n bars. Used throughout
// as the unit of "how big is a normal move here", so that thresholds mean the
// same thing on a 3% asset and a 30% one.
func ATR(cs []Candle, n int) float64 {
	if n <= 0 || len(cs) < n+1 {
		return math.NaN()
	}
	sum := 0.0
	for i := len(cs) - n; i < len(cs); i++ {
		sum += TrueRange(cs[i-1], cs[i])
	}
	return sum / float64(n)
}

// Donchian returns the highest high and lowest low over the n bars ENDING
// BEFORE the most recent one. Excluding the current bar is the whole point:
// a channel that includes the bar you are testing can never be broken by it,
// which is the classic off-by-one that turns a breakout backtest into a
// tautology.
func Donchian(cs []Candle, n int) (high, low float64) {
	if n <= 0 || len(cs) < n+1 {
		return math.NaN(), math.NaN()
	}
	w := cs[len(cs)-1-n : len(cs)-1]
	high, low = w[0].High, w[0].Low
	for _, c := range w[1:] {
		high = math.Max(high, c.High)
		low = math.Min(low, c.Low)
	}
	return high, low
}

// PercentileRank is the fraction of the last n values that the final value
// exceeds — a distribution-free way to ask "is this extreme by this market's
// own recent standards", which holds up where a z-score does not because
// crypto returns are far from normal.
func PercentileRank(xs []float64, n int) float64 {
	if n <= 1 || len(xs) < n {
		return math.NaN()
	}
	w := xs[len(xs)-n:]
	cur := w[len(w)-1]
	below := 0
	for _, x := range w[:len(w)-1] {
		if x < cur {
			below++
		}
	}
	return float64(below) / float64(len(w)-1)
}

// CumulativeDelta is the running sum of taker buy minus taker sell volume
// over the visible bars — the order-flow agent's raw material. Bars without a
// reported split contribute nothing rather than a guess.
func CumulativeDelta(cs []Candle) []float64 {
	out := make([]float64, len(cs))
	run := 0.0
	for i, c := range cs {
		if c.HasFlow() {
			run += c.BuyVolume - c.SellVolume
		}
		out[i] = run
	}
	return out
}

// ArgMin returns the index of the smallest value in xs, or -1 when empty.
func ArgMin(xs []float64) int {
	if len(xs) == 0 {
		return -1
	}
	best := 0
	for i, x := range xs {
		if x < xs[best] {
			best = i
		}
	}
	return best
}

// ArgMax returns the index of the largest value in xs, or -1 when empty.
func ArgMax(xs []float64) int {
	if len(xs) == 0 {
		return -1
	}
	best := 0
	for i, x := range xs {
		if x > xs[best] {
			best = i
		}
	}
	return best
}

// clamp confines x to [lo, hi].
func clamp(x, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, x))
}
