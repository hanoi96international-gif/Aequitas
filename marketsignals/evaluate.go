package marketsignals

import (
	"fmt"
	"math"
	"sort"
)

// One bar for everything.
//
// The hiring panel and the deflated-Sharpe machinery were built around a
// single instrument, and the portfolio layer arrived without them. That left
// the package in an awkward position: every single-symbol strategy had to
// clear a bar before deployment, while the book — the more complex object,
// with more ways to be wrong — had none.
//
// Runnable closes that. Anything able to produce a Result under a stated cost
// model can be interviewed, so a cross-sectional allocator faces exactly the
// criteria a breakout agent does, computed the same way.
//
// The generalisation also fixed a waste: the old path re-ran a full backtest
// to compute walk-forward folds, when the folds are slices of the run it had
// already done.

// Runnable is anything that can be evaluated: a strategy on a series, or an
// allocator over a universe.
//
// It takes the cost model as an argument rather than holding one, because the
// hiring panel needs to re-run the same thing under harsher assumptions and a
// candidate that silently kept its own costs would sail through the round
// designed to catch it.
type Runnable interface {
	Name() string
	RunWith(costs CostModel) (Result, error)
	// Caveats are limitations no statistical test can detect, stated by the
	// candidate itself. See PortfolioRunnable for why this exists.
	Caveats() []string
}

// StrategyRunnable evaluates a Strategy on one series.
type StrategyRunnable struct {
	Series   *Series
	Strategy Strategy
	Risk     *RiskManager
}

// Name identifies the strategy.
func (r StrategyRunnable) Name() string { return r.Strategy.Name() }

// Caveats: a single instrument carries no universe-selection problem.
func (r StrategyRunnable) Caveats() []string { return nil }

// RunWith backtests under the given costs.
func (r StrategyRunnable) RunWith(costs CostModel) (Result, error) {
	risk := r.Risk
	if risk == nil {
		risk = NewRiskManager()
	}
	return (&Backtester{Costs: costs, Risk: risk}).Run(r.Series, r.Strategy)
}

// PortfolioRunnable evaluates an allocator over a universe.
type PortfolioRunnable struct {
	Universe  *Universe
	Allocator Allocator
	// PerSymbolCosts overrides the panel's cost model per instrument. The
	// stress round scales whatever is here as well, so a book cannot hide an
	// optimistic assumption in one name.
	PerSymbolCosts map[string]CostModel
}

// Name identifies the allocator.
func (r PortfolioRunnable) Name() string { return r.Allocator.Name() }

// Caveats states the limitation that no amount of deflated Sharpe can reach.
//
// Every statistical control in this package addresses the same question: given
// THIS data, how much of the result is selection? None of them can see a
// problem in the data itself, and a universe has one that a single instrument
// does not.
//
// A universe assembled today contains the coins that still exist. Everything
// that went to zero, got delisted, or quietly stopped trading is absent, and
// its absence is invisible — the backtest never sees the position it would
// have held into a delisting, so the returns it reports are the returns of a
// strategy that only ever traded survivors. In crypto the effect is large:
// most tokens ever listed no longer trade meaningfully.
//
// The deflated Sharpe cannot detect this, because it is not a selection
// problem within the data; it is a selection problem about which data exists.
// The only fix is a universe built from what was listed AT THE TIME, rebuilt
// at each rebalance, and no public dataset makes that easy. Stating it is the
// honest alternative to appearing to have handled it.
func (r PortfolioRunnable) Caveats() []string {
	return []string{
		fmt.Sprintf("universe of %d names: if it was assembled from instruments trading "+
			"TODAY, the result describes a strategy that only ever held survivors, and no "+
			"statistical control in this package can detect that", len(r.Universe.Series)),
		"universe choice is itself a parameter — trying several and reporting the best is " +
			"selection that the trial count must include",
	}
}

// RunWith backtests the book under the given costs.
func (r PortfolioRunnable) RunWith(costs CostModel) (Result, error) {
	bt := &PortfolioBacktester{Costs: map[string]CostModel{}, Default: costs}
	for sym, c := range r.PerSymbolCosts {
		bt.Costs[sym] = c
	}
	res, err := bt.Run(r.Universe, r.Allocator)
	if err != nil {
		return Result{}, err
	}
	return res.Result, nil
}

// FoldsOf slices an already-computed run into n contiguous out-of-sample
// segments.
//
// Slicing rather than re-running is not only cheaper, it is more correct: a
// re-run can differ from the original in path-dependent state — the drawdown
// stop reads an equity curve that starts at the fold boundary rather than at
// the beginning — so the folds would not sum to the run they claim to
// decompose.
func FoldsOf(res Result, n int) ([]Fold, error) {
	if n < 2 {
		return nil, fmt.Errorf("need at least 2 folds, got %d", n)
	}
	if len(res.Returns) < n*2 {
		return nil, fmt.Errorf("%d realised bars cannot support %d folds", len(res.Returns), n)
	}
	size := len(res.Returns) / n
	out := make([]Fold, 0, n)
	for k := 0; k < n; k++ {
		lo, hi := k*size, (k+1)*size
		if k == n-1 {
			hi = len(res.Returns)
		}
		out = append(out, Fold{
			Index:   k,
			FromBar: res.Steps[lo].Index,
			ToBar:   res.Steps[hi-1].Index,
			Metrics: ComputeMetrics(res.Returns[lo:hi], res.BarsPerYear),
		})
	}
	return out, nil
}

// ConductRunnables interviews any mix of candidates against the same bar.
//
// Single-instrument strategies and whole books compete in one field, which is
// the point: the trial count that the deflated Sharpe charges against is the
// number of things tried, and a researcher who tries five agents and three
// allocators has tried eight.
func (p *Panel) ConductRunnables(cands ...Runnable) ([]Interview, error) {
	if len(cands) == 0 {
		return nil, fmt.Errorf("no candidates to interview")
	}
	baseCosts := p.Backtester.Costs
	stressCosts := baseCosts.Stressed(p.Criteria.CostStressMultiple)

	type run struct {
		cand   Runnable
		result Result
		folds  []Fold
		stress Result
	}

	runs := make([]run, 0, len(cands))
	perBar := make([]float64, 0, len(cands))

	for _, c := range cands {
		res, err := c.RunWith(baseCosts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.Name(), err)
		}
		folds, _ := FoldsOf(res, p.Folds)
		stress, serr := c.RunWith(stressCosts)
		if serr != nil {
			return nil, fmt.Errorf("%s under stress: %w", c.Name(), serr)
		}
		runs = append(runs, run{c, res, folds, stress})
		perBar = append(perBar, res.Metrics.Sharpe/math.Sqrt(res.BarsPerYear))
	}

	// The hurdle: what the best of this many EDGELESS candidates would show
	// from estimation error alone. Derived analytically rather than from the
	// observed spread, because the observed spread conflates noise with any
	// shared real effect and would reject the finding it was meant to detect.
	bars := runs[0].result.Metrics.Bars
	for _, r := range runs {
		if r.result.Metrics.Bars < bars {
			bars = r.result.Metrics.Bars
		}
	}
	nullSD := 0.0
	if bars > 1 {
		nullSD = 1 / math.Sqrt(float64(bars))
	}
	hurdle := ExpectedMaxSharpe(len(cands), nullSD)

	out := make([]Interview, 0, len(runs))
	for _, r := range runs {
		m := r.result.Metrics
		iv := Interview{Candidate: r.cand.Name(), Result: r.result, Stressed: r.stress}

		add := func(name string, passed bool, format string, args ...any) {
			iv.Assessments = append(iv.Assessments, Assessment{
				Criterion: name, Passed: passed, Detail: fmt.Sprintf(format, args...),
			})
		}

		deflated := DeflatedSharpe(m.Sharpe/math.Sqrt(r.result.BarsPerYear), hurdle,
			m.Bars, m.Skew, m.ExcessKurt)
		add("edge survives selection", deflated >= p.Criteria.MinDeflatedSharpe,
			"P(edge real) %.3f against a required %.3f, with %d candidates in the field",
			deflated, p.Criteria.MinDeflatedSharpe, len(cands))

		positive, foldFrac := 0, 0.0
		for _, f := range r.folds {
			if f.Metrics.Sharpe > 0 {
				positive++
			}
		}
		if len(r.folds) > 0 {
			foldFrac = float64(positive) / float64(len(r.folds))
		}
		add("holds across regimes", foldFrac >= p.Criteria.MinPositiveFoldFraction,
			"%d of %d out-of-sample folds positive (%.0f%%, need %.0f%%)",
			positive, len(r.folds), foldFrac*100, p.Criteria.MinPositiveFoldFraction*100)

		add("drawdown is holdable", m.MaxDrawdown <= p.Criteria.MaxDrawdown,
			"deepest decline %.1f%% against a %.1f%% limit",
			m.MaxDrawdown*100, p.Criteria.MaxDrawdown*100)

		// A book's activity is turnover rather than a count of round trips:
		// it rebalances continuously and may never "close a trade" at all.
		activity := m.Trades
		if activity == 0 && m.Turnover > 0 {
			activity = int(m.Turnover)
		}
		add("enough evidence", activity >= p.Criteria.MinTrades,
			"%d trades or units of turnover against a %d minimum", activity, p.Criteria.MinTrades)

		add("survives realistic costs",
			r.stress.Metrics.Sharpe >= p.Criteria.MinStressedSharpe,
			"Sharpe %.2f at %.0fx costs (was %.2f) — needs %.2f",
			r.stress.Metrics.Sharpe, p.Criteria.CostStressMultiple, m.Sharpe,
			p.Criteria.MinStressedSharpe)

		iv.Hired = true
		for _, a := range iv.Assessments {
			if !a.Passed {
				iv.Hired = false
			}
		}
		iv.Caveats = r.cand.Caveats()
		out = append(out, iv)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hired != out[j].Hired {
			return out[i].Hired
		}
		return out[i].Result.Metrics.Sharpe > out[j].Result.Metrics.Sharpe
	})
	return out, nil
}

// AllocatorGrid enumerates cross-sectional configurations, so an allocator can
// be searched the way an agent can — and so the trial count reflects it.
func AllocatorGrid() []*CrossSectional {
	var out []*CrossSectional
	for _, lookback := range []int{72, 168, 336} {
		for _, skip := range []int{0, 12, 24} {
			for _, frac := range []float64{0.2, 0.3} {
				c := NewCrossSectional()
				c.Lookback, c.SkipRecent = lookback, skip
				c.LongFraction, c.ShortFraction = frac, frac
				out = append(out, c)
			}
		}
	}
	return out
}
