package marketsignals

import "fmt"

// Cost models.
//
// On a centralised order book, assuming a constant cost per unit traded is a
// reasonable simplification: the book is deep relative to a retail order, so
// the marginal fill price barely moves and a flat basis-point figure is close
// enough.
//
// On an automated market maker it is simply wrong, and wrong in the direction
// that matters. An AMM has no book — the price is a function of the reserves,
// and your own trade moves it before you are filled. For a constant-product
// pool the arithmetic is exact rather than approximate: putting an amount Δx
// into a reserve X returns Y·Δx/(X+Δx), so the effective price is (X+Δx)/Y
// against a spot price of X/Y. The cost is therefore
//
//	impact = trade size / reserve
//
// which is LINEAR in size, making the total cost QUADRATIC in it. Doubling
// the position quadruples what the round trip costs.
//
// That single fact decides most DEX strategies, and a flat-rate model hides
// it completely. The same signal can be comfortably profitable with a few
// thousand at risk and untradeable with a few hundred thousand — not because
// the signal changed, but because the trader became the market. A backtest
// that does not know how much money it is trading cannot tell you which case
// you are in.

// CostModel converts a change in position into what that change costs.
type CostModel interface {
	// CostFraction returns the cost of changing position by `turnover`,
	// where turnover is expressed as a fraction of equity and the result is
	// also a fraction of equity.
	CostFraction(turnover float64) float64
	// Stressed returns the same model with its variable costs multiplied,
	// for the hiring panel's robustness round.
	Stressed(multiple float64) CostModel
	// Describe renders the model for a report.
	Describe() string
}

// Costs is the linear model: a fixed charge per unit traded. Appropriate for
// a liquid order book, and the default everywhere outside DEX profiles.
type Costs struct {
	// FeeRate is the taker fee as a fraction of notional (0.0005 = 5bp).
	FeeRate float64
	// SlippageRate is the assumed adverse fill, as a fraction. It is the
	// single easiest number to under-assume, and a strategy whose edge does
	// not survive doubling it does not have an edge, it has a fee rebate.
	SlippageRate float64
}

// DefaultCosts are deliberately unkind: 5bp fee plus 10bp slippage per side,
// roughly a liquid perpetual taken with market orders in normal conditions.
func DefaultCosts() Costs { return Costs{FeeRate: 0.0005, SlippageRate: 0.0010} }

// PerUnit is the total charged per unit of position change.
func (c Costs) PerUnit() float64 { return c.FeeRate + c.SlippageRate }

// CostFraction is linear in turnover.
func (c Costs) CostFraction(turnover float64) float64 {
	return absf(turnover) * c.PerUnit()
}

// Stressed multiplies the slippage assumption, leaving the fee — which is
// contractual and does not vary with conditions — alone.
func (c Costs) Stressed(multiple float64) CostModel {
	return Costs{FeeRate: c.FeeRate, SlippageRate: c.SlippageRate * multiple}
}

// Describe renders the model.
func (c Costs) Describe() string {
	return fmt.Sprintf("%.1fbp fee + %.1fbp slippage per unit traded",
		c.FeeRate*10_000, c.SlippageRate*10_000)
}

// AMMCosts models a constant-product pool, where the cost of a trade depends
// on its size relative to the pool.
//
// This is the model that tells you whether a DEX strategy is one you can
// actually run at the size you intend to run it, which a flat rate cannot.
type AMMCosts struct {
	// PoolFeeRate is the pool's own fee: 0.003 for a Uniswap v2-style pool,
	// 0.0025 or 0.0001 depending on the tier elsewhere.
	PoolFeeRate float64

	// LiquidityUSD is the pool's total value locked. Only half of it sits on
	// the side being traded into, which is what the impact is measured
	// against.
	LiquidityUSD float64

	// AccountUSD is how much money the strategy is actually running. Without
	// it the impact term has no scale, and the model quietly degenerates back
	// into a flat rate — so a zero here is treated as an error rather than as
	// "very small".
	AccountUSD float64

	// MEVRate is a flat allowance on top for being visible in a public
	// mempool before settling: sandwiching, priority-fee competition, failed
	// transactions that still burn gas. It is an estimate and should be
	// pessimistic, because the participants it represents are optimising
	// against you specifically.
	MEVRate float64
}

// DefaultAMMCosts describes a Uniswap v2-style pool with a pessimistic MEV
// allowance. Liquidity and account size have no sane defaults and must be set.
func DefaultAMMCosts(liquidityUSD, accountUSD float64) AMMCosts {
	return AMMCosts{
		PoolFeeRate:  0.003,
		LiquidityUSD: liquidityUSD,
		AccountUSD:   accountUSD,
		MEVRate:      0.005,
	}
}

// CostFraction is quadratic in turnover: the pool fee and MEV allowance scale
// linearly, while price impact scales with the trade's size relative to the
// reserve — and the trade's size is itself proportional to turnover.
func (a AMMCosts) CostFraction(turnover float64) float64 {
	t := absf(turnover)
	if t == 0 {
		return 0
	}
	linear := t * (a.PoolFeeRate + a.MEVRate)

	// A pool with no liquidity, or an unstated account size, cannot be
	// modelled. Returning the linear part alone would understate the cost of
	// the exact situation this model exists to catch, so the trade is priced
	// as impossible instead.
	reserve := a.LiquidityUSD / 2
	if reserve <= 0 || a.AccountUSD <= 0 {
		return 1
	}

	notional := t * a.AccountUSD
	impact := notional / reserve // exact for a constant-product pool
	return linear + t*impact
}

// Stressed multiplies the discretionary terms. Pool fees are fixed by the
// contract and impact is arithmetic, so only the MEV allowance moves.
func (a AMMCosts) Stressed(multiple float64) CostModel {
	a.MEVRate *= multiple
	return a
}

// Describe renders the model, including the round-trip cost at a
// representative position size — the number that actually decides viability.
func (a AMMCosts) Describe() string {
	full := a.CostFraction(1.0) * 10_000
	tenth := a.CostFraction(0.1) / 0.1 * 10_000
	return fmt.Sprintf(
		"AMM pool $%.0f, account $%.0f: %.0fbp per unit at a 10%% position, %.0fbp at full size "+
			"(%.0fbp pool fee + %.0fbp MEV, impact grows with size)",
		a.LiquidityUSD, a.AccountUSD, tenth, full,
		a.PoolFeeRate*10_000, a.MEVRate*10_000)
}

// MaxViablePosition returns the largest position, as a fraction of equity,
// whose cost PER UNIT TRADED stays under maxCostPerUnit.
//
// Per unit rather than in total, because that is the quantity a slippage
// budget is actually expressed in: "I will not pay more than 1% to get in or
// out". The cost per unit of an AMM trade is
//
//	fee + MEV + (position · account) / reserve
//
// which is linear in position, so the answer is a straightforward solve
// rather than the quadratic one it looks like from the total.
//
// This is the number to look at before a Sharpe ratio. A DEX strategy is not
// profitable or unprofitable in the abstract — it is profitable up to a size,
// and that size is routinely far smaller than people assume. Against a
// $400k pool a $5k account can commit around 8% of itself for under 1%; a
// $500k account can commit under a tenth of one percent.
func (a AMMCosts) MaxViablePosition(maxCostPerUnit float64) float64 {
	if a.AccountUSD <= 0 || a.LiquidityUSD <= 0 {
		return 0
	}
	reserve := a.LiquidityUSD / 2
	fixed := a.PoolFeeRate + a.MEVRate
	if fixed >= maxCostPerUnit {
		// The fee and MEV allowance alone already exceed the budget, so no
		// size is small enough.
		return 0
	}
	perUnitSlope := a.AccountUSD / reserve
	if perUnitSlope <= 0 {
		return 1
	}
	return clamp((maxCostPerUnit-fixed)/perUnitSlope, 0, 1)
}
