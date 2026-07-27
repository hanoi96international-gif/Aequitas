package marketsignals

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Portfolios.
//
// Everything above this file trades one instrument. That is a real limitation
// rather than a simplification, and the research literature makes it a costly
// one: the most consistently documented effect in crypto returns is
// CROSS-SECTIONAL — past winners outperform past losers across a universe —
// and it cannot be expressed at all by a system that looks at one symbol at a
// time. A single-instrument momentum agent and a cross-sectional one are
// different strategies, not the same strategy at different scales.
//
// The second reason is plainer. Diversification is the only genuinely free
// improvement available to a strategy, and this package had none of it.
//
// But crypto diversification is largely an illusion, and the mechanism here is
// built around admitting that. In a drawdown, correlations across the asset
// class go to nearly one: ten alts are not ten bets, they are roughly one bet
// held ten times, and precisely when it matters least. The allocator therefore
// does NOT target a number of positions or an equal weight. It targets
// portfolio VOLATILITY computed from the realised covariance, which means that
// when correlations rise the whole book shrinks automatically — no separate
// rule required, because rising correlation raises portfolio volatility and
// the target does the rest.

// Universe is a set of instruments with aligned price history.
type Universe struct {
	Instruments []Instrument
	Series      map[string]*Series
}

// Align verifies that every series covers exactly the same bar times, and
// trims them to the common set if not.
//
// Misalignment is the quiet killer of cross-sectional work: comparing one
// asset's Monday against another's Tuesday produces a ranking of nothing, and
// nothing about the output looks wrong. Exchanges differ on listing dates,
// halts, and which bars they emit when a market is dead, so this is the normal
// case rather than the exceptional one.
func (u *Universe) Align() error {
	if len(u.Series) < 2 {
		return fmt.Errorf("a universe needs at least two instruments, got %d", len(u.Series))
	}

	counts := map[time.Time]int{}
	var interval time.Duration
	for _, s := range u.Series {
		if err := s.Validate(); err != nil {
			return err
		}
		if interval == 0 {
			interval = s.Interval
		} else if s.Interval != interval {
			return fmt.Errorf("series %q has interval %s, others have %s — bars of different "+
				"lengths cannot be ranked against each other", s.Symbol, s.Interval, interval)
		}
		for _, c := range s.Candles {
			counts[c.Time]++
		}
	}

	var common []time.Time
	for t, n := range counts {
		if n == len(u.Series) {
			common = append(common, t)
		}
	}
	if len(common) == 0 {
		return fmt.Errorf("the %d series share no bar times at all", len(u.Series))
	}
	sort.Slice(common, func(i, j int) bool { return common[i].Before(common[j]) })

	keep := map[time.Time]bool{}
	for _, t := range common {
		keep[t] = true
	}
	for sym, s := range u.Series {
		out := &Series{Symbol: s.Symbol, Interval: s.Interval, Events: s.Events}
		for i, c := range s.Candles {
			if !keep[c.Time] {
				continue
			}
			out.Candles = append(out.Candles, c)
			if i < len(s.Funding) {
				out.Funding = append(out.Funding, s.Funding[i])
			}
			if i < len(s.Social) {
				out.Social = append(out.Social, s.Social[i])
			}
		}
		u.Series[sym] = out
	}
	return nil
}

// Symbols returns the universe's symbols in a stable order.
func (u *Universe) Symbols() []string {
	out := make([]string, 0, len(u.Series))
	for s := range u.Series {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Bars is the number of aligned bars, or zero if unaligned.
func (u *Universe) Bars() int {
	for _, s := range u.Series {
		return len(s.Candles)
	}
	return 0
}

// Allocation is a target weight per symbol, as a fraction of equity.
type Allocation struct {
	Weights map[string]float64
	// Gross is the sum of absolute weights, Net their signed sum.
	Gross float64
	Net   float64
	// PortfolioVol is the annualised volatility the covariance implies for
	// this book.
	PortfolioVol float64
	// DiversificationRatio is weighted-average asset volatility divided by
	// portfolio volatility. It is 1 when everything moves together and
	// sqrt(N) when nothing does — the number that says whether ten positions
	// are ten bets or one bet held ten times.
	DiversificationRatio float64
	Reason               string
}

// CrossSectional ranks a universe and allocates across it.
//
// Long the strongest, short the weakest, sized so the BOOK hits a volatility
// target rather than so each position hits one. That distinction is the whole
// mechanism: per-position sizing on ten correlated alts produces ten times the
// intended risk, and does so exactly during the correlated selloff when the
// mistake is least survivable.
type CrossSectional struct {
	// Lookback is the ranking window, in bars.
	Lookback int
	// SkipRecent excludes the most recent bars from the ranking. Cross-
	// sectional momentum is conventionally measured skipping the last period
	// because very recent returns reverse; including them mixes a momentum
	// signal with a reversal one and blunts both.
	SkipRecent int
	// LongFraction and ShortFraction are the shares of the universe held on
	// each side. ShortFraction zero makes it long-only.
	LongFraction  float64
	ShortFraction float64

	// TargetVol is the annualised volatility for the whole book.
	TargetVol float64
	// CovLookback is the window for the covariance estimate.
	CovLookback int
	// MaxGross and MaxNet cap total and directional exposure.
	MaxGross float64
	MaxNet   float64
	// MinNames is the smallest universe worth ranking. Ranking four assets
	// into top and bottom deciles is not a cross-section, it is a coin flip
	// with extra arithmetic.
	MinNames int
}

// NewCrossSectional returns conventional, untuned settings.
func NewCrossSectional() *CrossSectional {
	return &CrossSectional{
		Lookback:      168,
		SkipRecent:    12,
		LongFraction:  0.3,
		ShortFraction: 0.3,
		TargetVol:     0.30,
		CovLookback:   240,
		MaxGross:      1.0,
		MaxNet:        0.5,
		MinNames:      8,
	}
}

// Warmup is how many bars the allocator needs.
func (c *CrossSectional) Warmup() int {
	return maxInt(c.Lookback+c.SkipRecent, c.CovLookback) + 2
}

// Allocate produces target weights from the first n bars of each series.
func (c *CrossSectional) Allocate(u *Universe, n int) (Allocation, error) {
	symbols := u.Symbols()
	if len(symbols) < c.MinNames {
		return Allocation{Reason: fmt.Sprintf(
			"universe of %d is below the %d needed for a cross-section to mean anything",
			len(symbols), c.MinNames)}, nil
	}
	if n < c.Warmup() {
		return Allocation{Reason: fmt.Sprintf("warming up (%d/%d bars)", n, c.Warmup())}, nil
	}

	// Rank on trailing return, skipping the most recent bars.
	type scored struct {
		symbol string
		score  float64
	}
	ranked := make([]scored, 0, len(symbols))
	for _, sym := range symbols {
		cs := u.Series[sym].Candles[:n]
		end := len(cs) - 1 - c.SkipRecent
		start := end - c.Lookback
		if start < 0 {
			return Allocation{Reason: "insufficient history for the ranking window"}, nil
		}
		ranked = append(ranked, scored{sym, math.Log(cs[end].Close / cs[start].Close)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].symbol < ranked[j].symbol // deterministic ties
		}
		return ranked[i].score > ranked[j].score
	})

	nLong := int(math.Round(c.LongFraction * float64(len(ranked))))
	nShort := int(math.Round(c.ShortFraction * float64(len(ranked))))
	if nLong < 1 {
		nLong = 1
	}
	if c.ShortFraction > 0 && nShort < 1 {
		nShort = 1
	}
	if nLong+nShort > len(ranked) {
		return Allocation{Reason: "long and short sleeves overlap; reduce the fractions"}, nil
	}

	raw := map[string]float64{}
	for i := 0; i < nLong; i++ {
		raw[ranked[i].symbol] = 1 / float64(nLong)
	}
	for i := 0; i < nShort; i++ {
		raw[ranked[len(ranked)-1-i].symbol] = -1 / float64(nShort)
	}

	// Size the BOOK, not the positions.
	rets := u.returnMatrix(symbols, n, c.CovLookback)
	cov := Covariance(rets)
	barsPerYear := 365.0
	for _, s := range u.Series {
		barsPerYear = s.BarsPerYear()
		break
	}

	w := make([]float64, len(symbols))
	for i, sym := range symbols {
		w[i] = raw[sym]
	}
	vol := PortfolioVol(w, cov) * math.Sqrt(barsPerYear)
	div := DiversificationRatio(w, cov)

	scale := 1.0
	if vol > 0 {
		scale = c.TargetVol / vol
	}

	out := Allocation{
		Weights: map[string]float64{}, PortfolioVol: vol, DiversificationRatio: div,
	}
	for sym, v := range raw {
		out.Weights[sym] = v * scale
	}
	out.recompute()

	// Caps, applied after the volatility scaling so they bind only when the
	// covariance says the book could be larger than is prudent.
	if out.Gross > c.MaxGross && out.Gross > 0 {
		out.scaleAll(c.MaxGross / out.Gross)
	}
	if math.Abs(out.Net) > c.MaxNet && out.Net != 0 {
		// Shifting every weight equally removes net exposure without
		// disturbing the ranking's relative ordering.
		shift := (math.Abs(out.Net) - c.MaxNet) / float64(len(out.Weights))
		if out.Net > 0 {
			shift = -shift
		}
		for sym := range out.Weights {
			out.Weights[sym] += shift
		}
		out.recompute()
	}

	out.Reason = fmt.Sprintf(
		"%d long / %d short of %d; book vol %s against a %s target (scaled %s), "+
			"diversification ratio %s — %s",
		nLong, nShort, len(ranked), pct(vol), pct(c.TargetVol), f2(scale), f2(div),
		diversificationNote(div, len(raw)))
	return out, nil
}

func diversificationNote(ratio float64, names int) string {
	ideal := math.Sqrt(float64(names))
	switch {
	case ratio < 1.2:
		return "these positions are one bet held several times"
	case ratio < ideal/2:
		return "far less independent than the position count suggests"
	default:
		return "genuinely spread"
	}
}

func (a *Allocation) recompute() {
	a.Gross, a.Net = 0, 0
	for _, v := range a.Weights {
		a.Gross += math.Abs(v)
		a.Net += v
	}
}

func (a *Allocation) scaleAll(f float64) {
	for sym := range a.Weights {
		a.Weights[sym] *= f
	}
	a.recompute()
}

// returnMatrix builds per-symbol log returns over the last `window` bars.
func (u *Universe) returnMatrix(symbols []string, n, window int) [][]float64 {
	out := make([][]float64, len(symbols))
	for i, sym := range symbols {
		cs := u.Series[sym].Candles[:n]
		lo := len(cs) - window - 1
		if lo < 0 {
			lo = 0
		}
		closes := make([]float64, 0, len(cs)-lo)
		for _, c := range cs[lo:] {
			closes = append(closes, c.Close)
		}
		out[i] = LogReturns(closes)
	}
	return out
}

// Covariance returns the sample covariance matrix of per-series returns. Rows
// are series; all must be the same length.
func Covariance(rets [][]float64) [][]float64 {
	n := len(rets)
	cov := make([][]float64, n)
	for i := range cov {
		cov[i] = make([]float64, n)
	}
	if n == 0 || len(rets[0]) < 2 {
		return cov
	}
	m := len(rets[0])
	for _, r := range rets {
		if len(r) < m {
			m = len(r)
		}
	}

	means := make([]float64, n)
	for i := range rets {
		for _, v := range rets[i][:m] {
			means[i] += v
		}
		means[i] /= float64(m)
	}
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			s := 0.0
			for k := 0; k < m; k++ {
				s += (rets[i][k] - means[i]) * (rets[j][k] - means[j])
			}
			s /= float64(m - 1)
			cov[i][j], cov[j][i] = s, s
		}
	}
	return cov
}

// PortfolioVol is the per-bar standard deviation of a weighted book.
func PortfolioVol(w []float64, cov [][]float64) float64 {
	if len(w) != len(cov) {
		return 0
	}
	v := 0.0
	for i := range w {
		for j := range w {
			v += w[i] * w[j] * cov[i][j]
		}
	}
	if v <= 0 {
		return 0
	}
	return math.Sqrt(v)
}

// DiversificationRatio is the weighted average of individual volatilities
// divided by the portfolio's own volatility.
//
// It is 1 when every asset moves together and sqrt(N) when none of them do,
// and it is the number to look at before congratulating yourself on holding
// ten things. In a crypto drawdown it collapses toward 1, which is the
// quantitative form of the observation that the asset class has one factor
// and everything else is detail.
func DiversificationRatio(w []float64, cov [][]float64) float64 {
	pv := PortfolioVol(w, cov)
	if pv <= 0 {
		return 0
	}
	weighted := 0.0
	for i := range w {
		weighted += math.Abs(w[i]) * math.Sqrt(cov[i][i])
	}
	return weighted / pv
}

// CooldownGuard refuses to re-enter an instrument for a while after a losing
// exit — a protection worth borrowing from Freqtrade, whose CooldownPeriod and
// StoplossGuard exist for the same reason.
//
// The failure it prevents is specific: a trend agent stopped out by a spike
// sees the same conditions on the next bar and re-enters, repeatedly, paying
// costs each time. The market has not changed; only the account has.
type CooldownGuard struct {
	// Bars is how long to stand aside after a loss.
	Bars int
	// LossThreshold is how bad an exit must be to trigger the cooldown, as a
	// fraction of equity.
	LossThreshold float64

	remaining int
	last      float64
}

// NewCooldownGuard returns a guard standing aside for 12 bars after a loss of
// 1% of equity or worse.
func NewCooldownGuard() *CooldownGuard {
	return &CooldownGuard{Bars: 12, LossThreshold: 0.01}
}

// RecordEquity feeds the guard the account's current equity; it starts a
// cooldown when the change since the previous call is a loss past the
// threshold.
func (g *CooldownGuard) RecordEquity(equity float64) {
	if g.last > 0 {
		if loss := (g.last - equity) / g.last; loss >= g.LossThreshold {
			g.remaining = g.Bars
		}
	}
	g.last = equity
	if g.remaining > 0 {
		g.remaining--
	}
}

// ModulateRisk zeroes the position while the cooldown runs.
func (g *CooldownGuard) ModulateRisk(View) RiskAdjustment {
	if g.remaining <= 0 {
		return NoAdjustment("cooldown", "no recent loss")
	}
	return RiskAdjustment{
		Scale: 0, Veto: true, Source: "cooldown",
		Reason: fmt.Sprintf("standing aside for %d more bar(s) after a loss — re-entering the "+
			"setup that just stopped you out pays costs for a market that has not changed",
			g.remaining),
	}
}
