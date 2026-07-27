package marketsignals

import (
	"fmt"
	"math"
	"sort"
)

// Strategy is anything that can turn a View into a position intent. Both a
// single Agent (via AgentStrategy) and an Ensemble satisfy it, so the
// backtester never needs to know which it is running.
type Strategy interface {
	Name() string
	Warmup() int
	Decide(v View) Signal
}

// AgentStrategy adapts a single Agent to Strategy, for evaluating agents in
// isolation before letting them into an ensemble.
type AgentStrategy struct{ Agent Agent }

func (a AgentStrategy) Name() string         { return a.Agent.Name() }
func (a AgentStrategy) Warmup() int          { return a.Agent.Warmup() }
func (a AgentStrategy) Decide(v View) Signal { return a.Agent.Evaluate(v) }

// Decide lets an Ensemble be backtested directly.
func (e *Ensemble) Decide(v View) Signal { return e.Evaluate(v).Signal }

// Name identifies the ensemble in results.
func (e *Ensemble) Name() string { return "ensemble" }

// Costs are what separates a backtest from a fantasy. Both are charged on
// every unit of position CHANGE, so a strategy that flips daily pays for it.
type Costs struct {
	// FeeRate is the taker fee as a fraction of notional traded (0.0005 = 5bp).
	FeeRate float64
	// SlippageRate is the expected adverse fill relative to the reference
	// price, as a fraction. Set it pessimistically: it is the single easiest
	// number to under-assume, and a strategy whose edge does not survive
	// doubling it does not have an edge, it has a fee rebate.
	SlippageRate float64
}

// DefaultCosts are deliberately unkind: 5bp fee plus 10bp slippage per side,
// roughly a liquid perpetual taken with market orders in normal conditions.
func DefaultCosts() Costs { return Costs{FeeRate: 0.0005, SlippageRate: 0.0010} }

// PerUnit is the total cost charged per unit of position change.
func (c Costs) PerUnit() float64 { return c.FeeRate + c.SlippageRate }

// Backtester runs a Strategy over a Series.
//
// The execution model is the part that matters, and it is deliberately
// pessimistic at every point where a choice exists:
//
//	bar i closes  →  the strategy sees bars 0..i and decides
//	bar i+1 opens →  the position change is filled here, with costs
//	bar i+2 opens →  the return on that position is realised
//
// Deciding on a close and filling on the SAME bar's close is the single most
// common way a backtest reports profits nobody could have taken: at the
// moment the close is known, the opportunity to trade at it is gone. Filling
// at the next open costs the strategy the overnight gap, which is real money
// and belongs in the result.
type Backtester struct {
	Costs Costs
	Risk  *RiskManager
}

// NewBacktester returns a backtester with unkind default costs and
// conservative risk settings.
func NewBacktester() *Backtester {
	return &Backtester{Costs: DefaultCosts(), Risk: NewRiskManager()}
}

// Step is one bar of the simulation, retained so a result can be audited
// rather than trusted.
type Step struct {
	Index    int
	Signal   Signal
	Target   float64
	Position float64
	Cost     float64
	Return   float64
	Equity   float64
}

// Result is the outcome of a backtest.
type Result struct {
	Strategy    string
	Symbol      string
	Metrics     Metrics
	Returns     []float64
	Equity      []float64
	Steps       []Step
	BarsPerYear float64
}

// Run executes the strategy over the whole series.
func (b *Backtester) Run(s *Series, strat Strategy) (Result, error) {
	if err := s.Validate(); err != nil {
		return Result{}, err
	}
	cs := s.Candles
	warmup := maxInt(strat.Warmup(), b.Risk.VolLookback+2)

	// Need bars i+1 (fill) and i+2 (realisation) to exist beyond the decision
	// bar i, so the last decision possible is at len-3.
	if warmup-1 > len(cs)-3 {
		return Result{}, fmt.Errorf("series %q has %d bars; %q needs at least %d",
			s.Symbol, len(cs), strat.Name(), warmup+2)
	}

	res := Result{
		Strategy:    strat.Name(),
		Symbol:      s.Symbol,
		BarsPerYear: s.BarsPerYear(),
	}

	equity, peak, pos := 1.0, 1.0, 0.0

	for i := warmup - 1; i <= len(cs)-3; i++ {
		v := NewView(s, i+1) // bars 0..i inclusive — nothing later exists here

		drawdown := (peak - equity) / peak
		sized := b.Risk.Size(strat.Decide(v), v, drawdown)

		// Risk modulation is applied AFTER sizing and can only ever shrink the
		// position. Folding it into the signal's strength instead would let a
		// calendar warning be traded off against conviction, which is exactly
		// backwards: the whole point of a blackout is that it does not care
		// how good the setup looks.
		if m, ok := strat.(RiskModulator); ok {
			adj := m.ModulateRisk(v)
			scale := clamp(adj.Scale, 0, 1)
			if adj.Veto {
				scale = 0
			}
			if scale < 1 {
				sized.Target *= scale
				sized.Reason += fmt.Sprintf(" | %s cut size to %s: %s", adj.Source, pct(scale), adj.Reason)
			}
		}

		// Fill the change at the next bar's open.
		turnover := math.Abs(sized.Target - pos)
		cost := turnover * b.Costs.PerUnit()
		pos = sized.Target

		// Realise the position over the following bar.
		entry, exit := cs[i+1].Open, cs[i+2].Open
		barRet := pos*(exit/entry-1) - cost

		equity *= 1 + barRet
		peak = math.Max(peak, equity)

		res.Returns = append(res.Returns, barRet)
		res.Equity = append(res.Equity, equity)
		res.Steps = append(res.Steps, Step{
			Index: i, Signal: sized.Signal, Target: sized.Target,
			Position: pos, Cost: cost, Return: barRet, Equity: equity,
		})
	}

	res.Metrics = ComputeMetrics(res.Returns, res.BarsPerYear)
	res.Metrics.Turnover = totalTurnover(res.Steps)
	res.Metrics.Trades = countFlips(res.Steps)
	return res, nil
}

func totalTurnover(steps []Step) float64 {
	t, prev := 0.0, 0.0
	for _, s := range steps {
		t += math.Abs(s.Position - prev)
		prev = s.Position
	}
	return t
}

func countFlips(steps []Step) int {
	n, prev := 0, 0.0
	for _, s := range steps {
		if (prev == 0 && s.Position != 0) || (prev > 0 && s.Position <= 0) || (prev < 0 && s.Position >= 0) {
			if s.Position != 0 {
				n++
			}
		}
		prev = s.Position
	}
	return n
}

// Fold is one out-of-sample segment of a walk-forward evaluation.
type Fold struct {
	Index   int
	FromBar int
	ToBar   int
	Metrics Metrics
}

// WalkForward splits the series into n contiguous segments and evaluates the
// strategy separately on each.
//
// This is the cheapest defence against the most expensive mistake. A single
// full-history Sharpe hides everything: a strategy that made all of its money
// in one quarter of 2021 and bled for three years shows the same headline
// number as one that worked throughout. The per-fold spread tells you which
// of those you have, and the spread is also the trialSD that DeflatedSharpe
// needs in order to mean anything.
func (b *Backtester) WalkForward(s *Series, strat Strategy, n int) ([]Fold, error) {
	if n < 2 {
		return nil, fmt.Errorf("walk-forward needs at least 2 folds, got %d", n)
	}
	full, err := b.Run(s, strat)
	if err != nil {
		return nil, err
	}
	if len(full.Returns) < n*2 {
		return nil, fmt.Errorf("%d realised bars cannot support %d folds", len(full.Returns), n)
	}

	size := len(full.Returns) / n
	folds := make([]Fold, 0, n)
	for k := 0; k < n; k++ {
		lo := k * size
		hi := lo + size
		if k == n-1 {
			hi = len(full.Returns)
		}
		folds = append(folds, Fold{
			Index:   k,
			FromBar: full.Steps[lo].Index,
			ToBar:   full.Steps[hi-1].Index,
			Metrics: ComputeMetrics(full.Returns[lo:hi], full.BarsPerYear),
		})
	}
	return folds, nil
}

// Candidate is one strategy's evaluation within a selection run.
type Candidate struct {
	Strategy Strategy
	Result   Result
	Folds    []Fold
	// FoldSharpeSD is the dispersion of annualised Sharpe across folds — the
	// instability that a single headline number conceals.
	FoldSharpeSD float64
	// PositiveFolds counts folds with a positive Sharpe. A strategy that only
	// works in some regimes shows up here before it shows up in your account.
	PositiveFolds int
	// DeflatedSharpe is the probability the edge is real AFTER accounting for
	// how many candidates were tried. This, not Sharpe, is the ranking key.
	DeflatedSharpe float64
	Verdict        string
}

// Selection is the outcome of comparing candidates honestly.
type Selection struct {
	Candidates []Candidate
	// SelectionHurdle is the Sharpe the best of these candidates would be
	// expected to show even if every one of them were pure noise.
	SelectionHurdle float64
	Trials          int
}

// SelectBest evaluates every candidate and ranks them by deflated Sharpe.
//
// This is the function that answers "which agents are actually the best
// ones", and its whole design is a refusal to answer it the easy way. Picking
// the top Sharpe out of a list is not selection, it is sampling: with enough
// candidates the winner's backtest looks good no matter what, because
// somebody has to come first. So the winner is measured against the Sharpe
// that the WINNER OF THIS MANY COIN FLIPS would be expected to show, using
// the across-fold dispersion as the noise scale.
//
// A candidate that cannot clear that hurdle is reported as unproven, however
// pretty its equity curve is. Most will not clear it. That is the correct
// result, not a bug in the ranking.
func (b *Backtester) SelectBest(s *Series, folds int, cands ...Strategy) (Selection, error) {
	if len(cands) == 0 {
		return Selection{}, fmt.Errorf("no candidates to select from")
	}
	sel := Selection{Trials: len(cands)}

	var perBarSharpes []float64
	for _, st := range cands {
		res, err := b.Run(s, st)
		if err != nil {
			return Selection{}, fmt.Errorf("%s: %w", st.Name(), err)
		}
		c := Candidate{Strategy: st, Result: res}

		if f, err := b.WalkForward(s, st, folds); err == nil {
			c.Folds = f
			sharpes := make([]float64, 0, len(f))
			for _, fold := range f {
				sharpes = append(sharpes, fold.Metrics.Sharpe)
				if fold.Metrics.Sharpe > 0 {
					c.PositiveFolds++
				}
			}
			if sd := StdDev(sharpes, len(sharpes)); !math.IsNaN(sd) {
				c.FoldSharpeSD = sd
			}
		}

		sel.Candidates = append(sel.Candidates, c)
		perBarSharpes = append(perBarSharpes, res.Metrics.Sharpe/math.Sqrt(res.BarsPerYear))
	}

	// The noise scale for the selection hurdle: how much the candidates'
	// Sharpes differ from one another. When only one candidate was tried
	// there is no selection effect, and the hurdle is correctly zero.
	trialSD := 0.0
	if len(perBarSharpes) > 1 {
		if sd := StdDev(perBarSharpes, len(perBarSharpes)); !math.IsNaN(sd) {
			trialSD = sd
		}
	}
	hurdle := ExpectedMaxSharpe(len(cands), trialSD)
	sel.SelectionHurdle = hurdle * math.Sqrt(sel.Candidates[0].Result.BarsPerYear)

	for i := range sel.Candidates {
		c := &sel.Candidates[i]
		m := c.Result.Metrics
		perBar := m.Sharpe / math.Sqrt(c.Result.BarsPerYear)
		c.DeflatedSharpe = DeflatedSharpe(perBar, hurdle, m.Bars, m.Skew, m.ExcessKurt)

		switch {
		case m.Bars == 0:
			c.Verdict = "no trades"
		case c.DeflatedSharpe >= 0.95 && c.PositiveFolds*2 >= len(c.Folds):
			c.Verdict = "edge survives selection and holds across folds"
		case c.DeflatedSharpe >= 0.95:
			c.Verdict = "clears the hurdle but is concentrated in a minority of folds"
		default:
			c.Verdict = "unproven — indistinguishable from the best of this many noise draws"
		}
	}

	sort.SliceStable(sel.Candidates, func(i, j int) bool {
		return sel.Candidates[i].DeflatedSharpe > sel.Candidates[j].DeflatedSharpe
	})
	return sel, nil
}

// Report renders a selection as a readable table.
func (s Selection) Report() string {
	out := fmt.Sprintf("Evaluated %d candidate(s). Selection hurdle: annualised Sharpe %.2f\n",
		s.Trials, s.SelectionHurdle)
	out += "(the Sharpe the best of these would be expected to show with no edge at all)\n\n"
	for _, c := range s.Candidates {
		m := c.Result.Metrics
		out += fmt.Sprintf("%-12s Sharpe %6.2f  maxDD %5.1f%%  trades %4d  P(edge real) %.3f\n",
			c.Strategy.Name(), m.Sharpe, m.MaxDrawdown*100, m.Trades, c.DeflatedSharpe)
		out += fmt.Sprintf("             %d/%d folds positive — %s\n",
			c.PositiveFolds, len(c.Folds), c.Verdict)
	}
	return out
}
