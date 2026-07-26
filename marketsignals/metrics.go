package marketsignals

import "math"

// Metrics summarises a return stream. Everything here is computed from the
// realised bar-by-bar returns of the strategy after costs — there is no path
// in this package that reports a metric on gross returns, because a gross
// Sharpe is a description of a strategy nobody can trade.
type Metrics struct {
	Bars         int
	TotalReturn  float64
	CAGR         float64
	Sharpe       float64 // annualised
	Sortino      float64 // annualised, downside deviation only
	MaxDrawdown  float64
	ProfitFactor float64
	HitRate      float64
	Turnover     float64 // total absolute position change, in units of equity
	Trades       int
	Skew         float64
	ExcessKurt   float64
}

// ComputeMetrics derives the summary from per-bar returns.
func ComputeMetrics(returns []float64, barsPerYear float64) Metrics {
	m := Metrics{Bars: len(returns)}
	if len(returns) == 0 {
		return m
	}

	equity := 1.0
	peak := 1.0
	gains, losses := 0.0, 0.0
	wins := 0
	mean := 0.0

	for _, r := range returns {
		equity *= 1 + r
		peak = math.Max(peak, equity)
		if dd := (peak - equity) / peak; dd > m.MaxDrawdown {
			m.MaxDrawdown = dd
		}
		if r > 0 {
			gains += r
			wins++
		} else {
			losses += -r
		}
		mean += r
	}
	mean /= float64(len(returns))

	m.TotalReturn = equity - 1
	m.HitRate = float64(wins) / float64(len(returns))
	if losses > 0 {
		m.ProfitFactor = gains / losses
	} else if gains > 0 {
		m.ProfitFactor = math.Inf(1)
	}

	years := float64(len(returns)) / barsPerYear
	if years > 0 && equity > 0 {
		m.CAGR = math.Pow(equity, 1/years) - 1
	}

	// Moments. Skew and kurtosis are not decoration: they feed the deflated
	// Sharpe below, and a strategy with a great Sharpe and a fat left tail is
	// a different animal from one with the same Sharpe and symmetric returns.
	var m2, m3, m4 float64
	for _, r := range returns {
		d := r - mean
		m2 += d * d
		m3 += d * d * d
		m4 += d * d * d * d
	}
	n := float64(len(returns))
	m2 /= n
	m3 /= n
	m4 /= n
	sd := math.Sqrt(m2)
	if sd > 0 {
		m.Sharpe = mean / sd * math.Sqrt(barsPerYear)
		m.Skew = m3 / (sd * sd * sd)
		m.ExcessKurt = m4/(m2*m2) - 3
	}

	// Sortino uses downside deviation, which is the honest denominator when
	// a strategy's upside volatility is the thing you are trying to buy.
	var down float64
	var downN int
	for _, r := range returns {
		if r < 0 {
			down += r * r
			downN++
		}
	}
	if downN > 0 {
		dd := math.Sqrt(down / float64(len(returns)))
		if dd > 0 {
			m.Sortino = mean / dd * math.Sqrt(barsPerYear)
		}
	} else if mean > 0 {
		m.Sortino = math.Inf(1)
	}

	return m
}

// normCDF is the standard normal cumulative distribution function.
func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

// normInv is the inverse standard normal CDF (Acklam's rational
// approximation, |error| < 1.15e-9 over the open unit interval), needed for
// the expected-maximum-Sharpe term below.
func normInv(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	a := [6]float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02,
		1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := [5]float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02,
		6.680131188771972e+01, -1.328068155288572e+01}
	c := [6]float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00,
		-2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := [4]float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00,
		3.754408661907416e+00}
	const plow, phigh = 0.02425, 1 - 0.02425

	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p > phigh:
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	default:
		q := p - 0.5
		r := q * q
		return (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q /
			(((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	}
}

// ExpectedMaxSharpe is the Sharpe ratio you should expect to observe from the
// BEST of nTrials strategies that all have zero true edge, given that the
// trial Sharpes vary with standard deviation trialSD.
//
// This number is the reason most backtests are worthless. Try 100 variations
// of a strategy on the same data and the best one will show a respectable
// Sharpe purely from selection — no edge required. Comparing your winner
// against zero is the wrong test; comparing it against this is the right one.
//
// Bailey & López de Prado, "The Deflated Sharpe Ratio" (2014).
func ExpectedMaxSharpe(nTrials int, trialSD float64) float64 {
	if nTrials <= 1 || trialSD <= 0 {
		return 0
	}
	const euler = 0.5772156649015329
	n := float64(nTrials)
	return trialSD * ((1-euler)*normInv(1-1/n) + euler*normInv(1-1/(n*math.E)))
}

// DeflatedSharpe is the probability that the observed Sharpe reflects a real
// edge rather than the best draw from nTrials attempts on noise.
//
// sharpe and benchmark are PER-BAR (not annualised) Sharpe ratios; pass
// Metrics.Sharpe/sqrt(barsPerYear) to convert. The skew and kurtosis terms
// matter: fat-tailed, negatively skewed returns make a given Sharpe far less
// trustworthy, and a formula that ignores them flatters exactly the
// strategies that blow up.
//
// Read the output as a confidence level. Below ~0.95, the honest conclusion
// is that the backtest has not demonstrated an edge.
func DeflatedSharpe(sharpe, benchmark float64, nObs int, skew, excessKurt float64) float64 {
	if nObs < 2 {
		return 0
	}
	denom := 1 - skew*sharpe + (excessKurt/4)*sharpe*sharpe
	if denom <= 0 {
		return 0
	}
	z := (sharpe - benchmark) * math.Sqrt(float64(nObs-1)) / math.Sqrt(denom)
	return normCDF(z)
}
