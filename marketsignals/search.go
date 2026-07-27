package marketsignals

import (
	"fmt"
	"math"
	"sort"
)

// The search.
//
// Everything else in this package evaluates strategies that were handed to it.
// That is only half the job, and the missing half is the one that finds
// anything: testing six untuned defaults and concluding "no edge here" is not
// a search, it is a sample of six.
//
// The reason a large search is usually a bad idea is that trying enough
// configurations guarantees a good-looking backtest whether or not anything
// real is present. But that is exactly the failure DeflatedSharpe measures,
// and this package already counts trials. With that in place a wide search
// stops being reckless and becomes the correct thing to do: the machinery was
// built to make searching safe, and then never used to search.
//
// Two rules make the result mean something.
//
// FIRST: the selection at every point in time uses only data from before that
// point. This is anchored walk-forward — choose a configuration on everything
// known so far, then evaluate it on the next segment and nothing else, then
// re-choose. It is slower to converge than optimising over the whole history
// and it produces far worse-looking numbers, which is the entire point: the
// worse-looking number is the one that was actually available.
//
// SECOND, and less obvious: what gets scored is the PROCEDURE, not its
// winner. Reporting the best configuration's full-history performance answers
// "what would I have made had I known in advance which parameters to use",
// which is not a question anybody can act on. Concatenating each segment's
// out-of-sample returns answers "what would this search have made", and that
// is a question with a usable answer. The gap between those two numbers is
// usually enormous, and mistaking the first for the second is the single most
// expensive error in quantitative research.

// Variant is one named configuration of one agent. The constructor is a
// function so every fold gets a fresh, unshared instance.
type Variant struct {
	Name  string
	Build func() Agent
}

// Strategy adapts a variant for the backtester.
func (v Variant) Strategy() Strategy { return AgentStrategy{Agent: v.Build()} }

// BreakoutGrid enumerates channel lengths, break thresholds and trend filters.
func BreakoutGrid() []Variant {
	var out []Variant
	for _, ch := range []int{20, 55, 100} {
		for _, brk := range []float64{0.10, 0.25, 0.50} {
			for _, ema := range [][2]int{{10, 50}, {20, 100}, {50, 200}} {
				ch, brk, ema := ch, brk, ema
				out = append(out, Variant{
					Name: fmt.Sprintf("breakout/ch%d/atr%.2f/ema%d-%d", ch, brk, ema[0], ema[1]),
					Build: func() Agent {
						a := NewBreakoutAgent()
						a.Channel, a.MinATRBreak = ch, brk
						a.FastEMA, a.SlowEMA = ema[0], ema[1]
						return a
					},
				})
			}
		}
	}
	return out
}

// ReversionGrid enumerates stretch thresholds and rejection requirements.
func ReversionGrid() []Variant {
	var out []Variant
	for _, lb := range []int{30, 50} {
		for _, z := range []float64{1.5, 2.0, 2.5} {
			for _, wick := range []float64{0.3, 0.4, 0.5} {
				lb, z, wick := lb, z, wick
				out = append(out, Variant{
					Name: fmt.Sprintf("reversion/lb%d/z%.1f/wick%.1f", lb, z, wick),
					Build: func() Agent {
						a := NewReversionAgent()
						a.Lookback, a.MinZ, a.MinWickFrac = lb, z, wick
						return a
					},
				})
			}
		}
	}
	return out
}

// FlowGrid enumerates divergence windows and confirmation thresholds.
func FlowGrid() []Variant {
	var out []Variant
	for _, lb := range []int{20, 30, 50} {
		for _, gap := range []float64{0.5, 1.0, 1.5} {
			lb, gap := lb, gap
			out = append(out, Variant{
				Name: fmt.Sprintf("flow/lb%d/gap%.1f", lb, gap),
				Build: func() Agent {
					a := NewFlowAgent()
					a.Lookback, a.MinDeltaGapZ = lb, gap
					return a
				},
			})
		}
	}
	return out
}

// FibonacciGrid enumerates swing sensitivities and minimum leg sizes.
func FibonacciGrid() []Variant {
	var out []Variant
	for _, sw := range []int{3, 5, 8} {
		for _, leg := range []float64{2, 3, 4} {
			sw, leg := sw, leg
			out = append(out, Variant{
				Name: fmt.Sprintf("fibonacci/sw%d/leg%.0f", sw, leg),
				Build: func() Agent {
					a := NewFibonacciAgent()
					a.SwingStrength, a.MinLegATR = sw, leg
					return a
				},
			})
		}
	}
	return out
}

// PatternGrid enumerates swing sensitivities and break thresholds.
func PatternGrid() []Variant {
	var out []Variant
	for _, sw := range []int{3, 5, 8} {
		for _, brk := range []float64{0.10, 0.25, 0.50} {
			sw, brk := sw, brk
			out = append(out, Variant{
				Name: fmt.Sprintf("pattern/sw%d/brk%.2f", sw, brk),
				Build: func() Agent {
					a := NewPatternAgent()
					a.SwingStrength, a.MinBreakATR = sw, brk
					return a
				},
			})
		}
	}
	return out
}

// FundingGrid enumerates crowding thresholds and persistence requirements.
func FundingGrid() []Variant {
	var out []Variant
	for _, lb := range []int{100, 200} {
		for _, p := range []float64{0.90, 0.95} {
			for _, persist := range []int{2, 3} {
				lb, p, persist := lb, p, persist
				out = append(out, Variant{
					Name: fmt.Sprintf("funding/lb%d/pct%.2f/n%d", lb, p, persist),
					Build: func() Agent {
						a := NewFundingAgent()
						a.Lookback, a.MinPct, a.MinPersist = lb, p, persist
						return a
					},
				})
			}
		}
	}
	return out
}

// FullGrid is every variant of every price-driven agent — the search space.
func FullGrid() []Variant {
	var out []Variant
	out = append(out, BreakoutGrid()...)
	out = append(out, ReversionGrid()...)
	out = append(out, FlowGrid()...)
	out = append(out, FibonacciGrid()...)
	out = append(out, PatternGrid()...)
	out = append(out, FundingGrid()...)
	return out
}

// Searcher runs anchored walk-forward selection over a variant space.
type Searcher struct {
	Backtester *Backtester
	// Folds is how many segments the evaluable history is cut into.
	Folds int
	// MinTrainFolds is how many segments must pass before the first
	// selection. Choosing a configuration on one short segment is choosing on
	// noise, and the resulting out-of-sample record then measures that noise
	// rather than the method.
	MinTrainFolds int
}

// NewSearcher returns a searcher with sane defaults.
func NewSearcher() *Searcher {
	return &Searcher{Backtester: NewBacktester(), Folds: 8, MinTrainFolds: 3}
}

// FoldChoice records what the search picked at one point in time and what
// that pick then did on data it had never seen.
type FoldChoice struct {
	Fold           int
	FromBar        int
	ToBar          int
	Chosen         string
	InSampleSharpe float64
	OutOfSample    Metrics
}

// SearchResult is the outcome of the whole procedure.
type SearchResult struct {
	Trials int
	Folds  []FoldChoice

	// OutOfSample is the concatenation of every fold's out-of-sample returns
	// — the record of the search procedure itself.
	OutOfSample []float64
	Metrics     Metrics
	// DeflatedSharpe is that record's credibility, charged for every variant
	// the search examined.
	DeflatedSharpe float64

	// NullHurdle is the Sharpe the best of this many EDGELESS variants would
	// be expected to show, derived analytically: under no edge, a Sharpe
	// estimated over T observations has sampling error 1/sqrt(T). This is the
	// hurdle the verdict uses.
	NullHurdle float64
	// EmpiricalHurdle is the same figure computed instead from the observed
	// spread of variant Sharpes, as the deflated-Sharpe literature suggests.
	//
	// It is reported but NOT used to decide, because it has a failure mode
	// that matters exactly when the search succeeds: the observed spread
	// conflates noise with SHARED SIGNAL. When a real edge is present, most
	// variants of the right agent capture some of it, the spread widens
	// because the edge is real, and the hurdle rises until it rejects the
	// very thing it was meant to detect. On pure noise the two hurdles agree
	// closely — that agreement is what makes the divergence diagnostic rather
	// than arbitrary. A gap between them says "many variants agree here",
	// which is evidence for a finding, not against it.
	EmpiricalHurdle float64

	// BestInHindsight is the single variant with the best full-history
	// performance, reported ONLY so it can be compared against the procedure.
	// It is not a result: nobody could have known to pick it at the start.
	BestInHindsight string
	HindsightSharpe float64

	Verdict string
	// Stability is how many DISTINCT variants the search chose across folds.
	// A method that picks a different winner every fold has not found an
	// edge, it has found whatever led most recently.
	Stability int
}

// Run executes the search.
func (s *Searcher) Run(series *Series, variants []Variant) (SearchResult, error) {
	if len(variants) == 0 {
		return SearchResult{}, fmt.Errorf("no variants to search")
	}
	if s.Folds < s.MinTrainFolds+1 {
		return SearchResult{}, fmt.Errorf("need at least %d folds to leave one out of sample",
			s.MinTrainFolds+1)
	}

	// One full backtest per variant. Because no agent can see past its own
	// bar, a variant's decision at bar i is the same whether the run started
	// at bar 0 or at bar i-k, so slicing a full run by bar range gives the
	// same decisions a restricted run would have made. The equity path does
	// carry across fold boundaries, which is deliberate: a real account is
	// not reset every quarter, and the drawdown kill switch should see the
	// history it would really have seen.
	runs := make([]Result, 0, len(variants))
	kept := make([]Variant, 0, len(variants))
	for _, v := range variants {
		res, err := s.Backtester.Run(series, v.Strategy())
		if err != nil {
			continue // variant needs more history than this series has
		}
		if len(res.Steps) == 0 {
			continue
		}
		runs = append(runs, res)
		kept = append(kept, v)
	}
	if len(runs) == 0 {
		return SearchResult{}, fmt.Errorf("no variant could run on %d bars", len(series.Candles))
	}

	// Every variant must be judged over the same bars, or a short-warmup
	// variant would be credited with an easier stretch of history.
	start, end := runs[0].Steps[0].Index, runs[0].Steps[len(runs[0].Steps)-1].Index
	for _, r := range runs {
		start = maxInt(start, r.Steps[0].Index)
		if last := r.Steps[len(r.Steps)-1].Index; last < end {
			end = last
		}
	}
	if end-start < s.Folds*20 {
		return SearchResult{}, fmt.Errorf("only %d comparable bars for %d folds", end-start, s.Folds)
	}

	out := SearchResult{Trials: len(kept)}
	foldSize := (end - start + 1) / s.Folds
	chosenSet := map[string]bool{}
	// The span the procedure is actually judged over. Every comparison below
	// uses it too: measuring the hindsight winner over the FULL history while
	// the procedure only trades the later folds compares two different
	// periods, and on a losing stretch that can make hindsight look worse
	// than choosing as you go — which is not a finding, it is a mismatched
	// denominator.
	oosStart := start + s.MinTrainFolds*foldSize

	for k := s.MinTrainFolds; k < s.Folds; k++ {
		trainEnd := start + k*foldSize
		testEnd := trainEnd + foldSize
		if k == s.Folds-1 {
			testEnd = end + 1
		}

		// Choose on everything before this fold, and only on that.
		best, bestSharpe := -1, math.Inf(-1)
		for i := range runs {
			// Activity is counted from the steps, not read off Metrics:
			// ComputeMetrics works on a bare return stream and has no
			// positions to count, so its Trades field is always zero. The
			// backtester fills that in afterwards from its own steps.
			if tradesInRange(runs[i], start, trainEnd) == 0 {
				continue // never took a position in the training window
			}
			m := ComputeMetrics(returnsInRange(runs[i], start, trainEnd), runs[i].BarsPerYear)
			if m.Sharpe > bestSharpe {
				best, bestSharpe = i, m.Sharpe
			}
		}
		if best < 0 {
			continue // nothing traded in the training window
		}

		oos := returnsInRange(runs[best], trainEnd, testEnd)
		out.OutOfSample = append(out.OutOfSample, oos...)
		out.Folds = append(out.Folds, FoldChoice{
			Fold: k, FromBar: trainEnd, ToBar: testEnd - 1,
			Chosen: kept[best].Name, InSampleSharpe: bestSharpe,
			OutOfSample: ComputeMetrics(oos, runs[best].BarsPerYear),
		})
		chosenSet[kept[best].Name] = true
	}

	if len(out.OutOfSample) == 0 {
		return SearchResult{}, fmt.Errorf("search produced no out-of-sample bars")
	}

	barsPerYear := runs[0].BarsPerYear
	out.Metrics = ComputeMetrics(out.OutOfSample, barsPerYear)
	out.Stability = len(chosenSet)

	// Hindsight, for contrast only — over the SAME span the procedure was
	// judged on.
	for i := range runs {
		m := ComputeMetrics(returnsInRange(runs[i], oosStart, end+1), barsPerYear)
		if out.BestInHindsight == "" || m.Sharpe > out.HindsightSharpe {
			out.BestInHindsight, out.HindsightSharpe = kept[i].Name, m.Sharpe
		}
	}

	// The search examined this many variants, so the procedure's own record
	// is charged for all of them.
	perBar := make([]float64, 0, len(runs))
	for i := range runs {
		m := ComputeMetrics(returnsInRange(runs[i], oosStart, end+1), barsPerYear)
		perBar = append(perBar, m.Sharpe/math.Sqrt(barsPerYear))
	}
	trialSD := 0.0
	if sd := StdDev(perBar, len(perBar)); !math.IsNaN(sd) {
		trialSD = sd
	}
	out.EmpiricalHurdle = ExpectedMaxSharpe(len(kept), trialSD) * math.Sqrt(barsPerYear)

	// The null: how far apart this many genuinely edgeless variants' Sharpes
	// would scatter purely from estimation error over this many observations.
	nullSD := 1 / math.Sqrt(float64(out.Metrics.Bars))
	nullHurdle := ExpectedMaxSharpe(len(kept), nullSD)
	out.NullHurdle = nullHurdle * math.Sqrt(barsPerYear)

	out.DeflatedSharpe = DeflatedSharpe(
		out.Metrics.Sharpe/math.Sqrt(barsPerYear), nullHurdle,
		out.Metrics.Bars, out.Metrics.Skew, out.Metrics.ExcessKurt)

	switch {
	case out.Metrics.Sharpe <= 0:
		out.Verdict = "no edge: the procedure lost money out of sample"
	case out.DeflatedSharpe < 0.95:
		out.Verdict = "not proven: the out-of-sample record is within what this many trials " +
			"would produce from noise"
	case out.Stability > len(out.Folds)*2/3:
		out.Verdict = "unstable: profitable out of sample, but the search picked a different " +
			"winner almost every fold, which is chasing rather than finding"
	default:
		out.Verdict = "edge found: profitable out of sample, stable in what it selected, and " +
			"beyond what this many trials explains"
	}
	return out, nil
}

// tradesInRange counts position changes among the steps for bars in [lo, hi),
// which is how the search tells "this variant had no opinion here" apart from
// "this variant held a losing position here".
func tradesInRange(r Result, lo, hi int) int {
	var steps []Step
	for _, st := range r.Steps {
		if st.Index >= lo && st.Index < hi {
			steps = append(steps, st)
		}
	}
	return countFlips(steps)
}

// returnsInRange extracts a run's returns for bars in [lo, hi).
func returnsInRange(r Result, lo, hi int) []float64 {
	var out []float64
	for i, st := range r.Steps {
		if st.Index >= lo && st.Index < hi {
			out = append(out, r.Returns[i])
		}
	}
	return out
}

// CrossMarketResult aggregates a search run separately on several markets.
//
// This is the strongest evidence available without waiting for live results.
// A configuration that works on one symbol is a configuration fitted to one
// symbol's history; the same method surviving on several markets that share
// no idiosyncratic history is much harder to explain as luck. It is also the
// test most likely to kill a finding, which is why it is worth running.
type CrossMarketResult struct {
	PerMarket map[string]SearchResult
	Positive  int
	Total     int
	Verdict   string
}

// SearchAcross runs the same search independently on every series.
func (s *Searcher) SearchAcross(seriesSet []*Series, variants []Variant) (CrossMarketResult, error) {
	out := CrossMarketResult{PerMarket: map[string]SearchResult{}}
	for _, series := range seriesSet {
		res, err := s.Run(series, variants)
		if err != nil {
			return CrossMarketResult{}, fmt.Errorf("%s: %w", series.Symbol, err)
		}
		out.PerMarket[series.Symbol] = res
		out.Total++
		if res.Metrics.Sharpe > 0 {
			out.Positive++
		}
	}
	switch {
	case out.Total == 0:
		out.Verdict = "no markets searched"
	case out.Positive == out.Total:
		out.Verdict = "the method held on every market searched"
	case out.Positive*2 >= out.Total:
		out.Verdict = "mixed: the method held on some markets and not others, which is what a " +
			"fitted result looks like as often as a real one"
	default:
		out.Verdict = "the method failed on most markets — a win on the rest is selection"
	}
	return out, nil
}

// Report renders a search result.
func (r SearchResult) Report() string {
	out := fmt.Sprintf("Searched %d variants across %d out-of-sample folds.\n\n", r.Trials, len(r.Folds))
	out += "What the SEARCH PROCEDURE made on data it had not seen when it chose:\n"
	out += fmt.Sprintf("  Sharpe %.2f   maxDD %.1f%%   %d bars   P(edge real) %.3f\n",
		r.Metrics.Sharpe, r.Metrics.MaxDrawdown*100, r.Metrics.Bars, r.DeflatedSharpe)
	out += fmt.Sprintf("  hurdle (null, %d variants): annualised Sharpe %.2f\n", r.Trials, r.NullHurdle)
	out += fmt.Sprintf("  hurdle from observed variant spread: %.2f\n", r.EmpiricalHurdle)
	if r.EmpiricalHurdle > 1.5*r.NullHurdle {
		out += "  (the spread hurdle is far above the null one — many variants moved together,\n" +
			"   which is what a shared real effect looks like, so the null hurdle decides)\n"
	}
	out += "\n"

	out += "For contrast, the best variant judged with hindsight over the whole history:\n"
	out += fmt.Sprintf("  %s — Sharpe %.2f\n", r.BestInHindsight, r.HindsightSharpe)
	out += "  Nobody could have known to pick it at the start. The gap between these two\n" +
		"  numbers is what optimising over a full history quietly pays you in imagination.\n\n"

	out += "What it chose, fold by fold:\n"
	for _, f := range r.Folds {
		out += fmt.Sprintf("  bars %6d-%-6d  %-34s  in-sample %6.2f → out-of-sample %6.2f\n",
			f.FromBar, f.ToBar, f.Chosen, f.InSampleSharpe, f.OutOfSample.Sharpe)
	}
	out += fmt.Sprintf("\n  %d distinct variants chosen across %d folds\n", r.Stability, len(r.Folds))
	out += "\nVERDICT: " + r.Verdict + "\n"
	return out
}

// Report renders a cross-market result.
func (r CrossMarketResult) Report() string {
	out := fmt.Sprintf("Cross-market search: %d of %d markets profitable out of sample\n\n",
		r.Positive, r.Total)
	names := make([]string, 0, len(r.PerMarket))
	for k := range r.PerMarket {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		res := r.PerMarket[n]
		out += fmt.Sprintf("  %-12s Sharpe %6.2f  P(edge) %.3f  %s\n",
			n, res.Metrics.Sharpe, res.DeflatedSharpe, res.Verdict)
	}
	return out + "\nVERDICT: " + r.Verdict + "\n"
}
