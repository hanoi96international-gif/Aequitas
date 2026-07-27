package marketsignals

import (
	"fmt"
	"sort"
)

// The hiring panel.
//
// SelectBest ranks candidates. Ranking is not hiring: the best of a bad field
// is still bad, and a ranking always has a winner. An interview asks a
// different question — does THIS candidate meet the bar, independent of who
// else applied — and the honest answer for most trading strategies is no.
//
// Every criterion below exists because a strategy can pass all the others
// while failing it, and each failure mode has cost somebody real money:
//
//	deflated Sharpe   — the result is the best of many noise draws
//	fold consistency  — the edge existed in one regime and has since died
//	cost robustness   — the "edge" was an optimistic slippage assumption
//	drawdown          — the return is real but unholdable in practice
//	activity          — the record rests on a handful of lucky trades
//
// A candidate is hired only if it passes ALL of them. Criteria are not
// averaged into a score, because averaging lets a spectacular Sharpe buy
// forgiveness for a strategy that stops working the moment fees are realistic.

// HiringCriteria is the bar a candidate must clear to be hired.
type HiringCriteria struct {
	// MinDeflatedSharpe is the probability the edge is real after accounting
	// for how many candidates were tried.
	MinDeflatedSharpe float64
	// MinPositiveFoldFraction is how much of the out-of-sample timeline must
	// be profitable. An edge concentrated in one fold is a regime, not a
	// method.
	MinPositiveFoldFraction float64
	// MaxDrawdown is the deepest peak-to-trough decline tolerated. A
	// strategy nobody can hold through is not deployable however good its
	// arithmetic.
	MaxDrawdown float64
	// MinTrades guards against a record built on a handful of outcomes.
	MinTrades int
	// CostStressMultiple re-runs the candidate with slippage multiplied by
	// this factor. Surviving it is the single most informative test here:
	// most published edges are a fee assumption in disguise, and doubling
	// slippage is a cheap way to find out before the market does it for you.
	CostStressMultiple float64
	// MinStressedSharpe is the annualised Sharpe required under that stress.
	MinStressedSharpe float64
}

// DefaultHiringCriteria is deliberately strict. Applied to real market data,
// it will reject most of what it sees, including strategies with attractive
// headline numbers. That is the intended yield.
func DefaultHiringCriteria() HiringCriteria {
	return HiringCriteria{
		MinDeflatedSharpe:       0.95,
		MinPositiveFoldFraction: 0.6,
		MaxDrawdown:             0.35,
		MinTrades:               30,
		CostStressMultiple:      2.0,
		MinStressedSharpe:       0.0,
	}
}

// Assessment is one criterion's verdict on one candidate.
type Assessment struct {
	Criterion string
	Passed    bool
	Detail    string
}

// Interview is the full record for one candidate — kept as a list of
// individually pass/fail findings rather than a single number, so that a
// rejection can be read and argued with.
type Interview struct {
	Candidate   string
	Hired       bool
	Assessments []Assessment
	Result      Result
	Stressed    Result
	// Caveats are limitations no criterion above can test for — chiefly
	// survivorship in a universe. They are printed alongside a hire rather
	// than counted against it, because the honest response to something a
	// test cannot see is to say so, not to pretend a number covers it.
	Caveats []string
}

// FailureReasons lists the criteria this candidate failed.
func (iv Interview) FailureReasons() []string {
	var out []string
	for _, a := range iv.Assessments {
		if !a.Passed {
			out = append(out, a.Criterion+": "+a.Detail)
		}
	}
	return out
}

// Panel runs interviews and reports who was hired.
type Panel struct {
	Backtester *Backtester
	Criteria   HiringCriteria
	Folds      int
}

// NewPanel returns a panel with the strict default bar.
func NewPanel() *Panel {
	return &Panel{Backtester: NewBacktester(), Criteria: DefaultHiringCriteria(), Folds: 5}
}

// Conduct interviews every candidate against the same data and the same bar.
//
// The selection hurdle is computed once across the whole field and applied to
// each candidate, so interviewing more candidates makes the bar HARDER for
// all of them. That is the correct direction and the opposite of what
// searching harder usually does to a researcher's standards.
func (p *Panel) Conduct(s *Series, cands ...Strategy) ([]Interview, error) {
	if len(cands) == 0 {
		return nil, fmt.Errorf("no candidates to interview")
	}
	sel, err := p.Backtester.SelectBest(s, p.Folds, cands...)
	if err != nil {
		return nil, err
	}

	// The stress round, with each model's discretionary costs multiplied. What
	// "harsher" means is the model's business: for an order book it is the
	// slippage assumption, for an AMM the MEV allowance, since pool fees are
	// contractual and price impact is arithmetic.
	stress := &Backtester{
		Costs: p.Backtester.Costs.Stressed(p.Criteria.CostStressMultiple),
		Risk:  p.Backtester.Risk,
	}

	out := make([]Interview, 0, len(sel.Candidates))
	for _, c := range sel.Candidates {
		iv := Interview{Candidate: c.Strategy.Name(), Result: c.Result}
		m := c.Result.Metrics

		add := func(name string, passed bool, format string, args ...any) {
			iv.Assessments = append(iv.Assessments, Assessment{
				Criterion: name, Passed: passed, Detail: fmt.Sprintf(format, args...),
			})
		}

		add("edge survives selection",
			c.DeflatedSharpe >= p.Criteria.MinDeflatedSharpe,
			"P(edge real) %.3f against a required %.3f, with %d candidates in the field",
			c.DeflatedSharpe, p.Criteria.MinDeflatedSharpe, len(cands))

		foldFrac := 0.0
		if len(c.Folds) > 0 {
			foldFrac = float64(c.PositiveFolds) / float64(len(c.Folds))
		}
		add("holds across regimes",
			foldFrac >= p.Criteria.MinPositiveFoldFraction,
			"%d of %d out-of-sample folds positive (%.0f%%, need %.0f%%)",
			c.PositiveFolds, len(c.Folds), foldFrac*100, p.Criteria.MinPositiveFoldFraction*100)

		add("drawdown is holdable",
			m.MaxDrawdown <= p.Criteria.MaxDrawdown,
			"deepest decline %.1f%% against a %.1f%% limit", m.MaxDrawdown*100, p.Criteria.MaxDrawdown*100)

		add("enough evidence",
			m.Trades >= p.Criteria.MinTrades,
			"%d trades against a %d minimum", m.Trades, p.Criteria.MinTrades)

		stressed, serr := stress.Run(s, c.Strategy)
		if serr != nil {
			add("survives realistic costs", false, "stress run failed: %v", serr)
		} else {
			iv.Stressed = stressed
			add("survives realistic costs",
				stressed.Metrics.Sharpe >= p.Criteria.MinStressedSharpe,
				"Sharpe %.2f at %.0fx slippage (was %.2f) — needs %.2f",
				stressed.Metrics.Sharpe, p.Criteria.CostStressMultiple, m.Sharpe,
				p.Criteria.MinStressedSharpe)
		}

		iv.Hired = true
		for _, a := range iv.Assessments {
			if !a.Passed {
				iv.Hired = false
			}
		}
		out = append(out, iv)
	}

	// Hired first, then by how convincingly each cleared the field.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hired != out[j].Hired {
			return out[i].Hired
		}
		return out[i].Result.Metrics.Sharpe > out[j].Result.Metrics.Sharpe
	})
	return out, nil
}

// Hired filters a set of interviews down to the strategies that passed.
func Hired(interviews []Interview) []string {
	var out []string
	for _, iv := range interviews {
		if iv.Hired {
			out = append(out, iv.Candidate)
		}
	}
	return out
}

// Report renders the panel's decisions.
func (p *Panel) Report(interviews []Interview) string {
	out := ""
	hired := 0
	for _, iv := range interviews {
		verdict := "NOT HIRED"
		if iv.Hired {
			verdict = "HIRED"
			hired++
		}
		out += fmt.Sprintf("%-12s %-9s  Sharpe %6.2f  maxDD %5.1f%%  trades %4d\n",
			iv.Candidate, verdict, iv.Result.Metrics.Sharpe, iv.Result.Metrics.MaxDrawdown*100,
			iv.Result.Metrics.Trades)
		for _, a := range iv.Assessments {
			mark := "fail"
			if a.Passed {
				mark = "pass"
			}
			out += fmt.Sprintf("    [%s] %-26s %s\n", mark, a.Criterion, a.Detail)
		}
		for _, c := range iv.Caveats {
			out += "    [!!!!] " + c + "\n"
		}
	}
	out += fmt.Sprintf("\n%d of %d candidates hired.\n", hired, len(interviews))
	if hired == 0 {
		out += "Nobody cleared the bar. On real data that is the usual outcome, and it is\n" +
			"a finding rather than a failure — deploying the least-bad candidate because\n" +
			"the search produced no good one is how a research process turns into a loss.\n"
	}
	return out
}
