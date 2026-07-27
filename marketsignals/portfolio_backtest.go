package marketsignals

import (
	"fmt"
	"math"
)

// Running a book of experts.
//
// This is where the pieces meet. Each instrument gets the expert profile its
// class and sector call for — the agents that have valid inputs there, the
// cost model its venue implies, the risk caps its liquidity supports — and
// each of those produces an opinion about its own market, independently.
//
// Then the portfolio layer does the thing no single expert can do: it decides
// how much of the account those opinions are collectively worth.
//
// That last step matters more than it sounds, and it is the family rule from
// the ensemble one level up. When ten experts all say long, the naive reading
// is ten times the conviction. The correct reading depends entirely on whether
// their ten markets are ten markets — and in crypto they usually are not. Ten
// correlated longs sized independently is one bet at ten times the intended
// size, entered precisely when the correlation that makes it one bet is
// highest. Sizing the BOOK from the covariance handles this without anyone
// having to notice it is happening.

// Allocator turns a universe and a point in time into target weights.
type Allocator interface {
	Name() string
	Warmup() int
	Allocate(u *Universe, n int) (Allocation, error)
}

// Name identifies the cross-sectional allocator, including the parameters that
// distinguish one variant from another — a field of eighteen candidates all
// called "cross-sectional" is a report nobody can act on.
func (c *CrossSectional) Name() string {
	return fmt.Sprintf("xs/lb%d/skip%d/frac%.1f", c.Lookback, c.SkipRecent, c.LongFraction)
}

// ExpertPanel runs one expert ensemble per instrument and combines their
// targets into a book.
//
// It is the counterpart to CrossSectional rather than a variant of it. A
// cross-sectional allocator has no opinion about any single market — it only
// ranks them against each other, and will hold the least-bad name in a
// universe that is entirely falling. An expert panel has an opinion about each
// market on its own terms and can decline to hold anything at all. Which of
// those you want is a real choice, and neither answer is right for every
// regime.
type ExpertPanel struct {
	// Ensembles and Risk are per symbol, built from each instrument's expert
	// profile so a memecoin and BTC are never treated with one set of
	// assumptions.
	Ensembles map[string]*Ensemble
	Risk      map[string]*RiskManager

	// TargetVol is the annualised volatility for the whole book.
	TargetVol   float64
	CovLookback int
	MaxGross    float64
	MaxNet      float64
}

// NewExpertPanel builds a panel from a universe, giving every instrument the
// profile its class and sector call for.
func NewExpertPanel(u *Universe) (*ExpertPanel, error) {
	p := &ExpertPanel{
		Ensembles:   map[string]*Ensemble{},
		Risk:        map[string]*RiskManager{},
		TargetVol:   0.30,
		CovLookback: 240,
		MaxGross:    1.0,
		MaxNet:      0.7,
	}
	for _, inst := range u.Instruments {
		profile, err := ExpertFor(inst)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", inst.Symbol, err)
		}
		if !profile.TradeableAtAll {
			// Not an error: a universe may legitimately contain a stablecoin
			// or a fresh launch, and the right response is to hold none of it
			// rather than to refuse the whole book.
			continue
		}
		p.Ensembles[inst.Symbol] = profile.Ensemble()
		p.Risk[inst.Symbol] = profile.Backtester().Risk
	}
	if len(p.Ensembles) == 0 {
		return nil, fmt.Errorf("no instrument in this universe is tradeable by any profile")
	}
	return p, nil
}

// Name identifies the panel.
func (p *ExpertPanel) Name() string { return "expert-panel" }

// Warmup is the longest warmup among the members, plus the covariance window.
func (p *ExpertPanel) Warmup() int {
	w := p.CovLookback
	for _, e := range p.Ensembles {
		w = maxInt(w, e.Warmup())
	}
	return w + 2
}

// Allocate polls every expert and sizes the resulting book.
func (p *ExpertPanel) Allocate(u *Universe, n int) (Allocation, error) {
	if n < p.Warmup() {
		return Allocation{Reason: fmt.Sprintf("warming up (%d/%d bars)", n, p.Warmup())}, nil
	}

	symbols := u.Symbols()
	raw := map[string]float64{}
	voted := 0

	for _, sym := range symbols {
		e, ok := p.Ensembles[sym]
		if !ok {
			continue // no tradeable profile for this instrument
		}
		s, ok := u.Series[sym]
		if !ok || len(s.Candles) < n {
			continue
		}
		v := NewView(s, n)

		// Each expert sizes on its own terms first — its own volatility
		// target, its own leverage cap — because those encode what its market
		// can carry. The book-level scaling below then decides how much of
		// that collection of opinions the account should actually take.
		sized := p.Risk[sym].Size(e.Decide(v), v, 0)
		if m, ok := any(e).(RiskModulator); ok {
			adj := m.ModulateRisk(v)
			scale := clamp(adj.Scale, 0, 1)
			if adj.Veto {
				scale = 0
			}
			sized.Target *= scale
		}
		if sized.Target != 0 {
			raw[sym] = sized.Target
			voted++
		}
	}

	if voted == 0 {
		return Allocation{Weights: map[string]float64{},
			Reason: "no expert has a position; the book is flat"}, nil
	}

	// Book-level volatility targeting over the realised covariance. This is
	// what stops agreement among correlated experts from becoming a
	// concentrated bet: their agreement raises portfolio volatility, and the
	// target shrinks the book in response.
	w := make([]float64, len(symbols))
	for i, sym := range symbols {
		w[i] = raw[sym]
	}
	cov := Covariance(u.returnMatrix(symbols, n, p.CovLookback))

	barsPerYear := 365.0
	for _, s := range u.Series {
		barsPerYear = s.BarsPerYear()
		break
	}
	vol := PortfolioVol(w, cov) * math.Sqrt(barsPerYear)
	div := DiversificationRatio(w, cov)

	scale := 1.0
	if vol > 0 {
		scale = p.TargetVol / vol
	}

	out := Allocation{Weights: map[string]float64{}, PortfolioVol: vol, DiversificationRatio: div}
	for sym, v := range raw {
		out.Weights[sym] = v * scale
	}
	out.recompute()

	if out.Gross > p.MaxGross && out.Gross > 0 {
		out.scaleAll(p.MaxGross / out.Gross)
	}
	if math.Abs(out.Net) > p.MaxNet && out.Gross > 0 {
		out.scaleAll(p.MaxNet / math.Abs(out.Net))
	}

	out.Reason = fmt.Sprintf(
		"%d of %d experts hold a position; book vol %s against a %s target (scaled %s), "+
			"diversification ratio %s — %s",
		voted, len(p.Ensembles), pct(vol), pct(p.TargetVol), f2(scale), f2(div),
		diversificationNote(div, voted))
	return out, nil
}

// PortfolioBacktester runs an allocator over a universe.
//
// The execution model is the single-instrument one, applied per symbol: decide
// on a close, fill at the next open, realise over the following bar. Costs are
// charged PER INSTRUMENT from that instrument's own model, because a book
// holding BTC and a memecoin against one cost assumption is wrong about both.
type PortfolioBacktester struct {
	// Costs per symbol. Symbols absent fall back to Default.
	Costs   map[string]CostModel
	Default CostModel
}

// NewPortfolioBacktester returns a backtester with unkind default costs.
func NewPortfolioBacktester() *PortfolioBacktester {
	return &PortfolioBacktester{Costs: map[string]CostModel{}, Default: DefaultCosts()}
}

// CostsFromUniverse fills the per-symbol cost models from each instrument's
// expert profile, so a DEX name is priced against its pool and a liquid
// perpetual against its spread.
func (b *PortfolioBacktester) CostsFromUniverse(u *Universe) error {
	for _, inst := range u.Instruments {
		profile, err := ExpertFor(inst)
		if err != nil {
			return fmt.Errorf("%s: %w", inst.Symbol, err)
		}
		if profile.Costs != nil {
			b.Costs[inst.Symbol] = profile.Costs
		}
	}
	return nil
}

func (b *PortfolioBacktester) costFor(symbol string) CostModel {
	if c, ok := b.Costs[symbol]; ok {
		return c
	}
	return b.Default
}

// PortfolioResult is a book's record, plus what it held along the way.
type PortfolioResult struct {
	Result
	// Allocations is the target book at each decision point, retained so a
	// drawdown can be traced to what was actually held rather than guessed at.
	Allocations []Allocation
	// AvgGross and AvgDiversification summarise the book's shape over the run.
	AvgGross           float64
	AvgDiversification float64
}

// Run executes the allocator over the universe.
func (b *PortfolioBacktester) Run(u *Universe, a Allocator) (PortfolioResult, error) {
	if err := u.Align(); err != nil {
		return PortfolioResult{}, err
	}
	bars := u.Bars()
	symbols := u.Symbols()
	warmup := a.Warmup()

	if warmup-1 > bars-3 {
		return PortfolioResult{}, fmt.Errorf(
			"universe has %d aligned bars; %q needs at least %d", bars, a.Name(), warmup+2)
	}

	res := PortfolioResult{}
	res.Strategy = a.Name()
	res.Symbol = fmt.Sprintf("%d-name book", len(symbols))
	for _, s := range u.Series {
		res.BarsPerYear = s.BarsPerYear()
		break
	}

	equity := 1.0
	held := map[string]float64{}
	var grossSum, divSum, turnoverSum float64
	rebalances := 0

	for i := warmup - 1; i <= bars-3; i++ {
		alloc, err := a.Allocate(u, i+1)
		if err != nil {
			return PortfolioResult{}, err
		}

		barRet, barTurnover := 0.0, 0.0
		for _, sym := range symbols {
			target := alloc.Weights[sym]
			turnover := math.Abs(target - held[sym])
			barTurnover += turnover
			barRet -= b.costFor(sym).CostFraction(turnover)
			held[sym] = target

			cs := u.Series[sym].Candles
			entry, exit := cs[i+1].Open, cs[i+2].Open
			barRet += target * (exit/entry - 1)
		}

		equity *= 1 + barRet
		res.Returns = append(res.Returns, barRet)
		res.Equity = append(res.Equity, equity)
		res.Allocations = append(res.Allocations, alloc)
		res.Steps = append(res.Steps, Step{Index: i, Return: barRet, Equity: equity})

		grossSum += alloc.Gross
		divSum += alloc.DiversificationRatio
		turnoverSum += barTurnover
		if barTurnover > 0 {
			rebalances++
		}
	}

	res.Metrics = ComputeMetrics(res.Returns, res.BarsPerYear)
	// ComputeMetrics works on a bare return stream and cannot see positions,
	// so activity is filled in here. Leaving it at zero was not cosmetic: the
	// hiring panel's evidence criterion reads it, so a book would have failed
	// that criterion for ever regardless of how it performed.
	res.Metrics.Turnover = turnoverSum
	res.Metrics.Trades = rebalances
	if n := float64(len(res.Allocations)); n > 0 {
		res.AvgGross = grossSum / n
		res.AvgDiversification = divSum / n
	}
	return res, nil
}

// Report renders a portfolio result.
func (r PortfolioResult) Report() string {
	m := r.Metrics
	out := fmt.Sprintf("%s over a %s\n", r.Strategy, r.Symbol)
	out += fmt.Sprintf("  Sharpe %.2f   maxDD %.1f%%   total %.1f%%   %d bars\n",
		m.Sharpe, m.MaxDrawdown*100, m.TotalReturn*100, m.Bars)
	out += fmt.Sprintf("  average gross exposure %.2f, average diversification ratio %.2f\n",
		r.AvgGross, r.AvgDiversification)
	if r.AvgDiversification > 0 && r.AvgDiversification < 1.3 {
		out += "  (a ratio near 1 means the book was one bet held several times, whatever\n" +
			"   the position count said)\n"
	}
	return out
}
