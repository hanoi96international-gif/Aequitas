package marketsignals

import (
	"math"
	"strings"
	"testing"
)

// TestPortfolioBacktester_ChargesEachInstrumentItsOwnCosts is the property a
// single cost number cannot express. A book holding BTC and a memecoin against
// one assumption is wrong about both, and wrong in the direction that flatters
// the expensive one.
func TestPortfolioBacktester_ChargesEachInstrumentItsOwnCosts(t *testing.T) {
	u := correlatedUniverse(10, 700, 0.3, 21)
	cs := NewCrossSectional()
	cs.MinNames = 8

	cheap := NewPortfolioBacktester()
	expensive := NewPortfolioBacktester()
	// One name in the book trades against a thin AMM pool; everything else is
	// a liquid book.
	expensive.Costs["A"] = DefaultAMMCosts(200_000, 250_000)

	a, err := cheap.Run(u, cs)
	if err != nil {
		t.Fatalf("cheap: %v", err)
	}
	b, err := expensive.Run(u, cs)
	if err != nil {
		t.Fatalf("expensive: %v", err)
	}
	if !(b.Metrics.TotalReturn < a.Metrics.TotalReturn) {
		t.Fatalf("the book returned %.4f with one expensive name against %.4f with none — "+
			"per-instrument costs are not reaching the result",
			b.Metrics.TotalReturn, a.Metrics.TotalReturn)
	}
}

// TestExpertPanel_ShrinksWhenExpertsAgreeOnCorrelatedMarkets is the family
// rule one level up. Ten experts saying long is ten times the conviction only
// if their ten markets are ten markets, and in crypto they usually are not.
func TestExpertPanel_ShrinksWhenExpertsAgreeOnCorrelatedMarkets(t *testing.T) {
	independent := correlatedUniverse(10, 900, 0.0, 33)
	together := correlatedUniverse(10, 900, 0.97, 33)

	build := func(u *Universe) *ExpertPanel {
		p, err := NewExpertPanel(u)
		if err != nil {
			t.Fatalf("panel: %v", err)
		}
		p.MaxGross, p.MaxNet = 10, 10 // let volatility targeting bind
		return p
	}

	// Force every expert to the same view, so only the covariance differs.
	forceLong := func(p *ExpertPanel) {
		for sym := range p.Ensembles {
			p.Ensembles[sym] = &Ensemble{
				Agents: []Agent{
					stub("t", FamilyTrend, Long, 1),
					stub("f", FamilyFlow, Long, 1),
				},
				MinAgents: 2, MinFamilies: 2, MinStrength: 0.1,
			}
			_ = sym
		}
	}

	pi, pt := build(independent), build(together)
	forceLong(pi)
	forceLong(pt)

	ai, err := pi.Allocate(independent, independent.Bars())
	if err != nil {
		t.Fatalf("independent: %v", err)
	}
	at, err := pt.Allocate(together, together.Bars())
	if err != nil {
		t.Fatalf("correlated: %v", err)
	}
	if ai.Gross == 0 || at.Gross == 0 {
		t.Fatalf("no book: %q / %q", ai.Reason, at.Reason)
	}

	if !(ai.Gross > 1.5*at.Gross) {
		t.Fatalf("gross is %.3f when the experts agree about independent markets and %.3f "+
			"when they agree about one market wearing ten names; unanimity among correlated "+
			"experts is one opinion, not ten", ai.Gross, at.Gross)
	}
	t.Logf("unanimous experts — independent markets: gross %.2f (div %.2f); "+
		"one-factor market: gross %.2f (div %.2f)",
		ai.Gross, ai.DiversificationRatio, at.Gross, at.DiversificationRatio)
}

func TestExpertPanel_HoldsNothingWhenNoExpertHasAView(t *testing.T) {
	u := correlatedUniverse(10, 900, 0.3, 44)
	p, err := NewExpertPanel(u)
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	for sym := range p.Ensembles {
		p.Ensembles[sym] = &Ensemble{
			Agents:    []Agent{stub("t", FamilyTrend, Flat, 0)},
			MinAgents: 2, MinFamilies: 2,
		}
	}

	got, err := p.Allocate(u, u.Bars())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Gross != 0 {
		t.Fatalf("gross %.4f with no expert holding anything", got.Gross)
	}
	if !strings.Contains(got.Reason, "flat") {
		t.Fatalf("reason %q should say the book is flat", got.Reason)
	}
}

// TestExpertPanel_SkipsInstrumentsNoProfileWillTrade. A universe may
// legitimately contain a stablecoin or a days-old launch; the right response is
// to hold none of it rather than to refuse the whole book.
func TestExpertPanel_SkipsInstrumentsNoProfileWillTrade(t *testing.T) {
	u := correlatedUniverse(10, 400, 0.3, 55)
	syms := u.Symbols()
	for i := range u.Instruments {
		if u.Instruments[i].Symbol == syms[0] {
			u.Instruments[i].Sector = SectorStablecoin
		}
	}

	p, err := NewExpertPanel(u)
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	if _, ok := p.Ensembles[syms[0]]; ok {
		t.Fatalf("%s was given an ensemble despite its profile refusing to trade it", syms[0])
	}
	if len(p.Ensembles) != len(syms)-1 {
		t.Fatalf("panel covers %d of %d instruments", len(p.Ensembles), len(syms)-1)
	}
}

func TestExpertPanel_RefusesAUniverseNothingCanTrade(t *testing.T) {
	u := correlatedUniverse(3, 400, 0.3, 66)
	for i := range u.Instruments {
		u.Instruments[i].Sector = SectorNewLaunch
	}
	if _, err := NewExpertPanel(u); err == nil {
		t.Fatal("expected an error when no instrument is tradeable by any profile")
	}
}

// TestPortfolioBacktester_DecidesOnlyOnThePast applies the package's central
// guarantee to the portfolio path, which has its own way of leaking: an
// allocator ranks across symbols, so a single symbol's future bar could
// contaminate every weight rather than just one.
func TestPortfolioBacktester_DecidesOnlyOnThePast(t *testing.T) {
	base := correlatedUniverse(10, 800, 0.3, 77)

	rewritten := &Universe{Instruments: base.Instruments, Series: map[string]*Series{}}
	for sym, s := range base.Series {
		cp := &Series{Symbol: s.Symbol, Interval: s.Interval,
			Candles: append([]Candle(nil), s.Candles...)}
		rewritten.Series[sym] = cp
	}
	// Rewrite the tail of ONE name into a violent rally.
	target := base.Symbols()[0]
	cs := rewritten.Series[target].Candles
	price := cs[600].Close
	for i := 601; i < len(cs); i++ {
		open := price
		price = open * 1.05
		cs[i].Open, cs[i].Close = open, price
		cs[i].High, cs[i].Low = price*1.001, open*0.999
	}

	alloc := NewCrossSectional()
	alloc.MinNames = 8
	bt := NewPortfolioBacktester()

	a, err := bt.Run(base, alloc)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	b, err := bt.Run(rewritten, alloc)
	if err != nil {
		t.Fatalf("rewritten: %v", err)
	}

	compared := 0
	for i := range a.Allocations {
		if a.Steps[i].Index > 590 {
			break
		}
		for sym, w := range a.Allocations[i].Weights {
			if math.Abs(w-b.Allocations[i].Weights[sym]) > 1e-12 {
				t.Fatalf("bar %d weighted %s at %.6f originally and %.6f after a LATER stretch "+
					"of a different symbol was rewritten — the allocator is ranking with hindsight",
					a.Steps[i].Index, sym, w, b.Allocations[i].Weights[sym])
			}
		}
		compared++
	}
	if compared < 100 {
		t.Fatalf("only %d allocations compared; the test is not exercising enough of the run",
			compared)
	}
}

// TestPortfolioBacktester_FindsNoEdgeInAnIndependentNoiseUniverse is the
// portfolio path's negative control. Ten random walks contain no
// cross-sectional signal, and a ranking of noise must not become a return.
func TestPortfolioBacktester_FindsNoEdgeInAnIndependentNoiseUniverse(t *testing.T) {
	u := correlatedUniverse(12, 1200, 0.2, 88)

	alloc := NewCrossSectional()
	alloc.MinNames = 8
	res, err := NewPortfolioBacktester().Run(u, alloc)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Log("\n" + res.Report())

	perBar := res.Metrics.Sharpe / math.Sqrt(res.BarsPerYear)
	null := 1 / math.Sqrt(float64(maxInt(res.Metrics.Bars, 2)))
	if p := DeflatedSharpe(perBar, ExpectedMaxSharpe(2, null), res.Metrics.Bars,
		res.Metrics.Skew, res.Metrics.ExcessKurt); p >= 0.95 && res.Metrics.Sharpe > 0 {
		t.Fatalf("the book claims an edge (Sharpe %.2f, P=%.3f) ranking pure noise",
			res.Metrics.Sharpe, p)
	}
}

// TestPortfolioBacktester_CapturesAnImplantedCrossSectionalEffect is the
// positive control, and it has to hold together with the negative one: a
// framework that reports nothing on noise AND nothing on a real effect is not
// conservative, it is broken.
func TestPortfolioBacktester_CapturesAnImplantedCrossSectionalEffect(t *testing.T) {
	// Persistent dispersion: half the universe drifts up, half drifts down,
	// so past winners really do keep winning.
	u := correlatedUniverse(12, 1200, 0.3, 99)
	syms := u.Symbols()
	for i, sym := range syms {
		drift := 0.0015
		if i%2 == 1 {
			drift = -0.0015
		}
		cs := u.Series[sym].Candles
		for j := range cs {
			f := math.Exp(drift * float64(j))
			cs[j].Open, cs[j].High, cs[j].Low, cs[j].Close =
				cs[j].Open*f, cs[j].High*f, cs[j].Low*f, cs[j].Close*f
		}
	}

	alloc := NewCrossSectional()
	alloc.MinNames = 8
	res, err := NewPortfolioBacktester().Run(u, alloc)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Log("\n" + res.Report())

	if res.Metrics.Sharpe <= 0 {
		t.Fatalf("Sharpe %.2f on a universe where winners genuinely keep winning — the "+
			"allocator cannot capture an effect that is unmistakably present, so its "+
			"silence on real data would mean nothing", res.Metrics.Sharpe)
	}
}

func TestPortfolioBacktester_RejectsATooShortUniverse(t *testing.T) {
	u := correlatedUniverse(10, 120, 0.3, 12)
	alloc := NewCrossSectional()
	alloc.MinNames = 8
	if _, err := NewPortfolioBacktester().Run(u, alloc); err == nil {
		t.Fatal("expected an error rather than a backtest over warmup bars")
	}
}

func TestPortfolioBacktester_CostsFromUniverseUsesEachProfile(t *testing.T) {
	u := correlatedUniverse(10, 400, 0.3, 13)
	syms := u.Symbols()
	for i := range u.Instruments {
		if u.Instruments[i].Symbol == syms[0] {
			u.Instruments[i].Sector = SectorMeme
			u.Instruments[i].Venue = VenueDEX
			u.Instruments[i].Chain = "solana"
			u.Instruments[i].Address = "So111"
			u.Instruments[i].PoolLiquidityUSD = 300_000
			u.Instruments[i].AccountUSD = 20_000
		}
	}

	bt := NewPortfolioBacktester()
	if err := bt.CostsFromUniverse(u); err != nil {
		t.Fatalf("costs: %v", err)
	}
	if _, ok := bt.Costs[syms[0]].(AMMCosts); !ok {
		t.Fatalf("the DEX name got cost model %T, want the AMM one", bt.Costs[syms[0]])
	}
	if _, ok := bt.Costs[syms[1]].(Costs); !ok {
		t.Fatalf("a CEX name got cost model %T, want the linear one", bt.Costs[syms[1]])
	}
}
