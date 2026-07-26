package marketsignals

import "math"

// FundingAgent measures crowding rather than price. Perpetual funding is what
// one side is willing to PAY to keep its position, so a sustained extreme is
// a direct read on how leveraged and one-sided the market has become — and a
// crowded side is the side that gets liquidated.
//
// It is contrarian by construction and it is the slowest agent here, which is
// exactly why it belongs in the ensemble: its errors are uncorrelated with
// the price-shaped agents', so when it agrees with one of them the agreement
// carries real information.
//
// The gate is a percentile rank, not a fixed basis-point threshold. Funding
// regimes shift by orders of magnitude between a bull market and a quiet one;
// a hardcoded "0.05% is extreme" is right for one year of history and wrong
// for every other.
type FundingAgent struct {
	Lookback   int     // window defining what "extreme" means for this market
	MinPct     float64 // percentile rank required, e.g. 0.95
	MinPersist int     // consecutive bars the extreme must hold
}

// NewFundingAgent returns the agent with untuned round-number defaults.
func NewFundingAgent() *FundingAgent {
	return &FundingAgent{
		Lookback:   200,
		MinPct:     0.95,
		MinPersist: 3,
	}
}

func (a *FundingAgent) Name() string   { return "funding" }
func (a *FundingAgent) Family() Family { return FamilyPositioning }

func (a *FundingAgent) Warmup() int { return a.Lookback + a.MinPersist }

func (a *FundingAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	f := v.Funding()
	if f == nil {
		return Flatf(a.Name(), a.Family(), "series carries no funding data")
	}

	// Every one of the last MinPersist bars must independently rank as
	// extreme within its own trailing window. A single spike is a print; a
	// week of longs paying is a position the market has to get out of.
	longsPaying, shortsPaying := 0, 0
	for k := 0; k < a.MinPersist; k++ {
		end := len(f) - k
		window := f[:end]
		rank := PercentileRank(window, a.Lookback)
		if math.IsNaN(rank) {
			return Flatf(a.Name(), a.Family(), "funding distribution undefined")
		}
		if rank >= a.MinPct && window[len(window)-1] > 0 {
			longsPaying++
		}
		if rank <= 1-a.MinPct && window[len(window)-1] < 0 {
			shortsPaying++
		}
	}

	cur := f[len(f)-1]
	switch {
	case longsPaying == a.MinPersist:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Short,
			Strength: clamp(float64(longsPaying)/float64(a.MinPersist), 0, 1),
			Note:     "longs have paid " + bps(cur) + " for " + itoa(a.MinPersist) + " bars — crowded long",
		}
	case shortsPaying == a.MinPersist:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Long,
			Strength: clamp(float64(shortsPaying)/float64(a.MinPersist), 0, 1),
			Note:     "shorts have paid " + bps(cur) + " for " + itoa(a.MinPersist) + " bars — crowded short",
		}
	default:
		return Flatf(a.Name(), a.Family(), "funding unremarkable (%s)", bps(cur))
	}
}
