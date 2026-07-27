package marketsignals

import (
	"math"
	"strings"
	"testing"
)

func TestCosts_LinearModelArithmetic(t *testing.T) {
	c := Costs{FeeRate: 0.0005, SlippageRate: 0.0010}
	if got := c.CostFraction(1.0); math.Abs(got-0.0015) > 1e-12 {
		t.Fatalf("full-size cost %.6f, want 0.0015", got)
	}
	// Linear: half the turnover, half the cost.
	if got := c.CostFraction(0.5); math.Abs(got-0.00075) > 1e-12 {
		t.Fatalf("half-size cost %.6f, want 0.00075", got)
	}
	// Direction does not matter; selling costs what buying costs.
	if c.CostFraction(-0.5) != c.CostFraction(0.5) {
		t.Fatal("a reduction was priced differently from an increase")
	}
	// The stress round moves slippage, not the contractual fee.
	s := c.Stressed(2).(Costs)
	if s.FeeRate != c.FeeRate {
		t.Fatalf("stress changed the fee from %v to %v", c.FeeRate, s.FeeRate)
	}
	if s.SlippageRate != 2*c.SlippageRate {
		t.Fatalf("stressed slippage %v, want %v", s.SlippageRate, 2*c.SlippageRate)
	}
}

// TestAMMCosts_ImpactIsExactForConstantProduct pins the arithmetic. Putting Δx
// into a reserve X returns Y·Δx/(X+Δx), so the effective price is (X+Δx)/Y
// against a spot of X/Y — an impact of exactly Δx/X.
func TestAMMCosts_ImpactIsExactForConstantProduct(t *testing.T) {
	// $1m pool means $500k on the side being traded into.
	a := AMMCosts{PoolFeeRate: 0, MEVRate: 0, LiquidityUSD: 1_000_000, AccountUSD: 100_000}

	// A full-size position is $100k against a $500k reserve: 20% impact.
	if got := a.CostFraction(1.0); math.Abs(got-0.20) > 1e-9 {
		t.Fatalf("full-size cost %.6f, want 0.20 ($100k into a $500k reserve)", got)
	}
	// A tenth of that is $10k: 2% impact, applied to a tenth of the position.
	if got := a.CostFraction(0.1); math.Abs(got-0.002) > 1e-9 {
		t.Fatalf("tenth-size cost %.6f, want 0.002", got)
	}
}

// TestAMMCosts_AreQuadraticInSize is the property that decides DEX strategies
// and that a flat rate cannot express: doubling the position quadruples what
// the round trip costs.
func TestAMMCosts_AreQuadraticInSize(t *testing.T) {
	a := AMMCosts{PoolFeeRate: 0, MEVRate: 0, LiquidityUSD: 1_000_000, AccountUSD: 100_000}

	small := a.CostFraction(0.1)
	double := a.CostFraction(0.2)
	if math.Abs(double/small-4) > 1e-6 {
		t.Fatalf("doubling the position multiplied cost by %.3f, want 4 — impact grows with "+
			"size, so total cost grows with its square", double/small)
	}

	// The linear model has no such property, which is exactly why it is the
	// wrong shape for an AMM.
	lin := Costs{FeeRate: 0, SlippageRate: 0.001}
	if math.Abs(lin.CostFraction(0.2)/lin.CostFraction(0.1)-2) > 1e-9 {
		t.Fatal("the linear model stopped being linear")
	}
}

// TestAMMCosts_AccountSizeDecidesTradeability is the finding that matters most
// about DEX trading and that no price series contains. The same signal, the
// same pool, the same everything — except how much money is being run.
func TestAMMCosts_AccountSizeDecidesTradeability(t *testing.T) {
	pool := 400_000.0

	small := DefaultAMMCosts(pool, 5_000)
	large := DefaultAMMCosts(pool, 500_000)

	at := 0.25 // a quarter of the account committed
	smallCost := small.CostFraction(at) / at
	largeCost := large.CostFraction(at) / at

	if !(largeCost > 20*smallCost) {
		t.Fatalf("per-unit cost at $500k is %.4f against %.4f at $5k — a hundredfold account "+
			"on the same pool must cost dramatically more, or the impact term is not working",
			largeCost, smallCost)
	}

	// The practical form of the same fact: how large a position stays under a
	// 1% one-way cost.
	smallMax := small.MaxViablePosition(0.01)
	largeMax := large.MaxViablePosition(0.01)
	if !(smallMax > largeMax) {
		t.Fatalf("max viable position is %.3f at $5k and %.3f at $500k", smallMax, largeMax)
	}
	if largeMax > 0.005 {
		t.Fatalf("a $500k account can supposedly commit %.2f%% of itself to a $400k pool "+
			"for under 1%% cost; that is the trader becoming the market", largeMax*100)
	}
	t.Logf("$400k pool — max position under 1%% cost: %.1f%% of a $5k account, %.2f%% of a $500k one",
		smallMax*100, largeMax*100)
}

// TestAMMCosts_RefuseToPriceWhatTheyCannotModel: a missing pool or account
// size does not make the model slightly less accurate, it removes the term
// that decides viability. Falling back to the linear part would understate
// exactly the case this model exists to catch.
func TestAMMCosts_RefuseToPriceWhatTheyCannotModel(t *testing.T) {
	for name, a := range map[string]AMMCosts{
		"no pool":    {PoolFeeRate: 0.003, LiquidityUSD: 0, AccountUSD: 10_000},
		"no account": {PoolFeeRate: 0.003, LiquidityUSD: 100_000, AccountUSD: 0},
	} {
		if got := a.CostFraction(0.1); got != 1 {
			t.Fatalf("%s: cost %.4f, want a prohibitive 1.0 rather than a flattering fallback",
				name, got)
		}
	}
	// Zero turnover still costs nothing.
	if got := (AMMCosts{}).CostFraction(0); got != 0 {
		t.Fatalf("not trading cost %.4f", got)
	}
}

func TestAMMCosts_StressMovesOnlyTheDiscretionaryTerm(t *testing.T) {
	a := DefaultAMMCosts(1_000_000, 50_000)
	s := a.Stressed(3).(AMMCosts)

	if s.PoolFeeRate != a.PoolFeeRate {
		t.Fatal("stress changed the pool fee, which is fixed by the contract")
	}
	if s.LiquidityUSD != a.LiquidityUSD || s.AccountUSD != a.AccountUSD {
		t.Fatal("stress changed the pool or account size rather than a cost assumption")
	}
	if s.MEVRate != 3*a.MEVRate {
		t.Fatalf("stressed MEV allowance %v, want %v", s.MEVRate, 3*a.MEVRate)
	}
}

func TestAMMCosts_DescribeNamesTheSizeDependence(t *testing.T) {
	got := DefaultAMMCosts(250_000, 20_000).Describe()
	for _, want := range []string{"pool", "account", "impact grows with size"} {
		if !strings.Contains(got, want) {
			t.Fatalf("description %q should mention %q", got, want)
		}
	}
}

// TestBacktester_AMMCostsShrinkAViableStrategy runs the same series and the
// same agent at two account sizes against one pool, through the full
// backtester. The point is that this difference is invisible in the price
// data: only the cost model knows the trader got bigger.
func TestBacktester_AMMCostsShrinkAViableStrategy(t *testing.T) {
	s := withFlow(trendSeries(1200, 17, 0.006))
	strat := AgentStrategy{Agent: NewBreakoutAgent()}

	small := NewBacktester()
	small.Costs = DefaultAMMCosts(400_000, 5_000)
	large := NewBacktester()
	large.Costs = DefaultAMMCosts(400_000, 2_000_000)

	a, err := small.Run(s, strat)
	if err != nil {
		t.Fatalf("small account: %v", err)
	}
	b, err := large.Run(s, strat)
	if err != nil {
		t.Fatalf("large account: %v", err)
	}
	if a.Metrics.Turnover == 0 {
		t.Fatal("the strategy never traded; the comparison proves nothing")
	}
	if !(b.Metrics.TotalReturn < a.Metrics.TotalReturn) {
		t.Fatalf("a $2m account returned %.4f against %.4f for a $5k one on the same $400k "+
			"pool — the impact term is not reaching the equity curve",
			b.Metrics.TotalReturn, a.Metrics.TotalReturn)
	}
	t.Logf("same signal, same pool: %.2f%% at $5k, %.2f%% at $2m",
		a.Metrics.TotalReturn*100, b.Metrics.TotalReturn*100)
}

func TestExpertFor_DEXInstrumentsUseTheAMMModel(t *testing.T) {
	dex := memecoin() // VenueDEX, pool and account set
	p, err := ExpertFor(dex)
	if err != nil {
		t.Fatalf("expert: %v", err)
	}
	if _, ok := p.Costs.(AMMCosts); !ok {
		t.Fatalf("a DEX instrument got cost model %T; an AMM's cost is a function of size "+
			"against pool depth, not a rate", p.Costs)
	}
	if !strings.Contains(p.Describe(), "AMM pool") {
		t.Fatalf("profile description should say it is pricing against a pool:\n%s", p.Describe())
	}

	// The same sector on a centralised venue keeps the linear model, because
	// there the flat approximation is reasonable.
	cex := Instrument{Symbol: "MEMEUSDT", Class: ClassCrypto, Sector: SectorMeme,
		Venue: VenueCEX, ContinuousTrading: true}
	cp, err := ExpertFor(cex)
	if err != nil {
		t.Fatalf("expert: %v", err)
	}
	if _, ok := cp.Costs.(Costs); !ok {
		t.Fatalf("a CEX instrument got cost model %T, want the linear one", cp.Costs)
	}
}

// TestAMMCosts_MaxViablePositionIsSelfConsistent checks the solver against the
// model it inverts.
func TestAMMCosts_MaxViablePositionIsSelfConsistent(t *testing.T) {
	for _, a := range []AMMCosts{
		DefaultAMMCosts(1_000_000, 10_000),
		DefaultAMMCosts(50_000, 25_000),
		{PoolFeeRate: 0.003, MEVRate: 0, LiquidityUSD: 2_000_000, AccountUSD: 100_000},
	} {
		const limit = 0.01
		t.Run(a.Describe()[:20], func(t *testing.T) {
			max := a.MaxViablePosition(limit)
			if max <= 0 || max > 1 {
				t.Fatalf("max viable position %.4f outside (0,1]", max)
			}
			// At that size the cost per unit should sit on the limit, or below
			// it when the answer was capped at full size.
			perUnit := a.CostFraction(max) / max
			if perUnit > limit+1e-6 {
				t.Fatalf("at the reported maximum %.4f the cost is %.4f, above the %.4f limit",
					max, perUnit, limit)
			}
			if max < 1 && perUnit < limit-1e-6 {
				t.Fatalf("at %.4f the cost is only %.4f; the solver stopped short of the limit",
					max, perUnit)
			}
		})
	}
}

// TestAMMCosts_MaxViablePositionIsZeroWhenFixedCostsExceedTheBudget: when the
// pool fee and MEV allowance alone are above the limit, no size is small
// enough, and reporting some small positive number would be worse than
// useless.
func TestAMMCosts_MaxViablePositionIsZeroWhenFixedCostsExceedTheBudget(t *testing.T) {
	a := AMMCosts{PoolFeeRate: 0.01, MEVRate: 0.01, LiquidityUSD: 1e9, AccountUSD: 1000}
	if got := a.MaxViablePosition(0.005); got != 0 {
		t.Fatalf("max viable position %.6f with 200bp of unavoidable cost against a 50bp "+
			"budget; want 0", got)
	}
}
