package marketsignals

import (
	"fmt"
	"time"
)

// Expert profiles: what "a specialist agent for this market" actually means.
//
// It does not mean a different algorithm per sector. A breakout is a breakout.
// What genuinely differs between a Treasury ETF and a two-day-old memecoin is
// everything AROUND the algorithm — which agents have valid inputs at all,
// what a round trip costs, how much of the account may be at stake, and how
// much evidence is required before anything is believed.
//
// Encoding those as a profile keeps the specialism where it belongs. The
// alternative, one agent internally branching on asset class, hides the most
// important decisions inside the least readable part of the code, and makes
// it impossible to state plainly what the system assumes about a market.

// ExpertProfile is the complete treatment of one market segment.
type ExpertProfile struct {
	Name string
	// Rationale explains, in prose, why this profile differs from the others.
	// It exists so that a disagreement about how a market should be traded is
	// a conversation about a stated claim rather than about a magic number.
	Rationale string

	// Costs for this segment. These are the numbers most likely to be wrong
	// by an order of magnitude, and being wrong here invalidates everything
	// downstream more thoroughly than any signal error.
	Costs Costs

	// Risk caps for this segment.
	MaxPositionFraction float64
	TargetVol           float64
	MaxDrawdown         float64

	// Agents applicable here, and the ensemble gating to apply.
	Agents         []Agent
	Modulators     []RiskModulator
	MinAgents      int
	MinFamilies    int
	MinStrength    float64
	VetoOnConflict bool

	// Hiring bar for this segment.
	Criteria HiringCriteria

	// TradeableAtAll is false for segments where the honest answer is that no
	// price-history strategy applies. Saying so explicitly is more useful than
	// shipping agents that will produce confident nonsense.
	TradeableAtAll bool
	// NotTradeableReason explains that refusal.
	NotTradeableReason string
}

// Ensemble assembles the profile's agents into a configured ensemble.
func (p ExpertProfile) Ensemble() *Ensemble {
	return &Ensemble{
		Agents:         p.Agents,
		Modulators:     p.Modulators,
		MinAgents:      p.MinAgents,
		MinFamilies:    p.MinFamilies,
		MinStrength:    p.MinStrength,
		VetoOnConflict: p.VetoOnConflict,
	}
}

// Backtester assembles the profile's cost and risk assumptions.
func (p ExpertProfile) Backtester() *Backtester {
	r := NewRiskManager()
	r.MaxLeverage = p.MaxPositionFraction
	r.TargetVol = p.TargetVol
	r.MaxDrawdown = p.MaxDrawdown
	return &Backtester{Costs: p.Costs, Risk: r}
}

// Panel assembles the profile's hiring bar.
func (p ExpertProfile) Panel() *Panel {
	return &Panel{Backtester: p.Backtester(), Criteria: p.Criteria, Folds: 5}
}

// ExpertFor selects the profile for an instrument.
func ExpertFor(i Instrument) (ExpertProfile, error) {
	if err := i.Validate(); err != nil {
		return ExpertProfile{}, err
	}
	switch i.Class {
	case ClassETF:
		return etfProfile(i), nil
	case ClassEquity:
		return equityProfile(i), nil
	case ClassCrypto:
		return cryptoProfile(i), nil
	}
	return ExpertProfile{}, fmt.Errorf("no expert profile for %s", i)
}

// ── Equities and ETFs ────────────────────────────────────────────────────

func etfProfile(i Instrument) ExpertProfile {
	p := ExpertProfile{
		Name: "etf",
		Rationale: "A broad ETF is a basket, so single-name idiosyncrasy is diversified " +
			"away and what is left is macro. That makes the calendar the dominant input " +
			"and mean reversion comparatively safe: a basket has a level to revert to in " +
			"a way that a single speculative asset does not. There is no perpetual, so " +
			"the positioning agent has no input and is excluded rather than fed zeros. " +
			"Costs are low and the market closes, which moves risk from slippage to " +
			"overnight gaps — a distinction that changes what a stop is worth.",
		Costs:               Costs{FeeRate: 0.0001, SlippageRate: 0.0002},
		MaxPositionFraction: 1.0,
		TargetVol:           0.12,
		MaxDrawdown:         0.20,
		MinAgents:           2,
		MinFamilies:         2,
		MinStrength:         0.3,
		VetoOnConflict:      true,
		Criteria:            DefaultHiringCriteria(),
		TradeableAtAll:      true,
	}
	macro := NewMacroAgent()
	// A closed market cannot be exited during a release, so the blackout runs
	// long: the position must be right-sized before the previous close.
	macro.BlackoutBefore = 24 * time.Hour
	p.Agents = []Agent{
		NewBreakoutAgent(),
		NewReversionAgent(),
		NewFibonacciAgent(),
		NewPatternAgent(),
		macro,
	}
	p.Modulators = []RiskModulator{macro}
	return p
}

func equityProfile(i Instrument) ExpertProfile {
	p := etfProfile(i)
	p.Name = "equity"
	p.Rationale = "A single equity carries company-specific risk an ETF does not: an " +
		"earnings miss, a fraud, a halt. Mean reversion is therefore demoted — a stock " +
		"making new lows on news has no level to revert to, and fading it is how a " +
		"drawdown becomes permanent. Position size is capped below full equity for the " +
		"same reason, and the drawdown limit is tighter."
	p.MaxPositionFraction = 0.5
	p.TargetVol = 0.18
	p.MaxDrawdown = 0.15
	macro := NewMacroAgent()
	macro.BlackoutBefore = 24 * time.Hour
	p.Agents = []Agent{
		NewBreakoutAgent(),
		NewFibonacciAgent(),
		NewPatternAgent(),
		macro,
	}
	p.Modulators = []RiskModulator{macro}
	return p
}

// ── Crypto ───────────────────────────────────────────────────────────────

func cryptoProfile(i Instrument) ExpertProfile {
	switch i.Sector {
	case SectorNewLaunch:
		return newLaunchProfile()
	case SectorStablecoin:
		return stablecoinProfile()
	case SectorMeme:
		return memeProfile(i)
	case SectorMajor:
		return majorProfile(i)
	case SectorLargeAlt, SectorDeFi, SectorInfra:
		return liquidAltProfile(i)
	case SectorNarrative:
		return narrativeProfile(i)
	}
	return liquidAltProfile(i)
}

func majorProfile(i Instrument) ExpertProfile {
	macro := NewMacroAgent()
	p := ExpertProfile{
		Name: "crypto-major",
		Rationale: "BTC and ETH are the only crypto assets with enough depth and history " +
			"for the full agent set to have valid inputs simultaneously. They trade " +
			"continuously, so there is no overnight gap, and they carry deep perpetual " +
			"markets — which makes funding a genuine read on crowding rather than a " +
			"thinly-traded number. This is the segment where the framework is on its " +
			"firmest ground, and it is still the segment where most apparent edges are " +
			"noise, because it is the most heavily arbitraged.",
		Costs:               Costs{FeeRate: 0.0004, SlippageRate: 0.0005},
		MaxPositionFraction: 1.0,
		TargetVol:           0.35,
		MaxDrawdown:         0.25,
		MinAgents:           2,
		MinFamilies:         2,
		MinStrength:         0.3,
		VetoOnConflict:      true,
		Criteria:            DefaultHiringCriteria(),
		TradeableAtAll:      true,
		Modulators:          []RiskModulator{macro},
	}
	p.Agents = []Agent{
		NewBreakoutAgent(), NewReversionAgent(), NewFlowAgent(),
		NewFibonacciAgent(), NewPatternAgent(), macro,
	}
	if i.HasPerpetual {
		p.Agents = append(p.Agents, NewFundingAgent())
	}
	return p
}

func liquidAltProfile(i Instrument) ExpertProfile {
	p := majorProfile(i)
	p.Name = "crypto-liquid-alt"
	p.Rationale = "Liquid alts trend harder than the majors and revert less reliably: they " +
		"are higher beta on the same macro impulse, so a move that would be an extreme " +
		"in BTC is an ordinary day here. Mean reversion is dropped — in a market that " +
		"can trend 80% in a month, fading extension is a way of being repeatedly right " +
		"and finally ruined. Costs roughly double and size is halved."
	p.Costs = Costs{FeeRate: 0.0006, SlippageRate: 0.0015}
	p.MaxPositionFraction = 0.5
	p.TargetVol = 0.45
	var agents []Agent
	for _, a := range p.Agents {
		if a.Family() == FamilyReversion {
			continue
		}
		agents = append(agents, a)
	}
	p.Agents = agents
	return p
}

func narrativeProfile(i Instrument) ExpertProfile {
	p := liquidAltProfile(i)
	p.Name = "crypto-narrative"
	p.Rationale = "Theme-driven assets (AI, RWA, gaming) move on attention rather than on " +
		"anything measurable about themselves, and attention rotates without warning. " +
		"The practical consequence is that history is a poor guide across a rotation: " +
		"a strategy fitted during a theme's ascent describes a crowd that has since " +
		"left. The hiring bar therefore demands consistency across MORE of the " +
		"timeline than elsewhere, which most candidates in this segment will fail."
	p.Costs = Costs{FeeRate: 0.0008, SlippageRate: 0.0025}
	p.MaxPositionFraction = 0.3
	p.Criteria.MinPositiveFoldFraction = 0.8
	// A theme IS attention, so the social agent has a genuine input here.
	social := NewSocialAgent()
	p.Agents = append(p.Agents, social)
	p.Modulators = append(append([]RiskModulator{}, p.Modulators...), social)
	return p
}

func memeProfile(i Instrument) ExpertProfile {
	p := ExpertProfile{
		Name: "crypto-meme",
		Rationale: "Memecoins have no cash flow, no protocol revenue and no level a price " +
			"could be wrong relative to — so mean reversion is not merely unreliable " +
			"here, it is undefined, and it is excluded. What remains is momentum and " +
			"structure, on a market whose real cost of trading is an order of magnitude " +
			"above the majors once thin books and MEV are counted. The cost assumption " +
			"below is the single most important number in this profile: at 300bp a round " +
			"trip, a strategy needs a very large edge merely to break even, and most " +
			"apparent memecoin edges disappear entirely at honest costs.",
		Costs:               Costs{FeeRate: 0.0010, SlippageRate: 0.0200},
		MaxPositionFraction: 0.10,
		TargetVol:           0.60,
		MaxDrawdown:         0.30,
		MinAgents:           2,
		MinFamilies:         2,
		MinStrength:         0.45,
		VetoOnConflict:      true,
		Criteria:            DefaultHiringCriteria(),
		TradeableAtAll:      true,
	}
	// A higher bar to be believed, because the survivorship problem here is
	// extreme: the memecoins with usable history are the ones that did not go
	// to zero, and they are a small and flattering sample.
	p.Criteria.MinPositiveFoldFraction = 0.8
	p.Criteria.MinTrades = 50
	// Attention is the closest thing a memecoin has to a fundamental, so the
	// social agent belongs here more than anywhere else — and it earns its
	// place mostly by fading euphoria and cutting size into it, not by
	// chasing what is trending.
	social := NewSocialAgent()
	p.Agents = []Agent{NewBreakoutAgent(), NewFlowAgent(), NewPatternAgent(), social}
	p.Modulators = []RiskModulator{social}
	return p
}

func newLaunchProfile() ExpertProfile {
	return ExpertProfile{
		Name: "crypto-new-launch",
		Rationale: "A token days old has no price history worth the name, and every " +
			"technical agent here needs dozens to hundreds of bars before its indicators " +
			"mean anything. Running them anyway does not produce a weak signal; it " +
			"produces a confident one computed from noise, which is worse.",
		TradeableAtAll: false,
		NotTradeableReason: "no usable price history — use the launch screener, which asks " +
			"the question this segment actually poses (can this be held at all?) rather " +
			"than the one the chart agents ask (which way is it going?)",
		Costs:               Costs{FeeRate: 0.0010, SlippageRate: 0.0300},
		MaxPositionFraction: 0.02,
		TargetVol:           0.80,
		MaxDrawdown:         0.30,
		Criteria:            DefaultHiringCriteria(),
	}
}

func stablecoinProfile() ExpertProfile {
	return ExpertProfile{
		Name: "crypto-stablecoin",
		Rationale: "A stablecoin's return distribution is a spike at zero with a rare, " +
			"catastrophic left tail. Every metric in this package assumes returns with " +
			"some dispersion, and applied here they produce spectacular Sharpe ratios " +
			"describing a strategy that collects pennies until the peg breaks. That is " +
			"not an edge, it is a short volatility position with the risk hidden in the " +
			"one observation the sample has not seen yet.",
		TradeableAtAll: false,
		NotTradeableReason: "peg-break risk is not represented in a price history that has " +
			"not yet contained one; a high Sharpe here measures the absence of the event " +
			"rather than its improbability",
		Costs:               Costs{FeeRate: 0.0002, SlippageRate: 0.0003},
		MaxPositionFraction: 0.05,
		TargetVol:           0.05,
		MaxDrawdown:         0.05,
		Criteria:            DefaultHiringCriteria(),
	}
}

// Describe renders a profile as a readable briefing.
func (p ExpertProfile) Describe() string {
	out := fmt.Sprintf("%s\n%s\n\n", p.Name, p.Rationale)
	if !p.TradeableAtAll {
		return out + "NOT TRADEABLE BY THIS FRAMEWORK: " + p.NotTradeableReason + "\n"
	}
	out += fmt.Sprintf("  costs        %.0fbp fee + %.0fbp slippage per unit traded\n",
		p.Costs.FeeRate*10_000, p.Costs.SlippageRate*10_000)
	out += fmt.Sprintf("  max position %.0f%% of equity, target vol %.0f%%, drawdown stop %.0f%%\n",
		p.MaxPositionFraction*100, p.TargetVol*100, p.MaxDrawdown*100)
	out += fmt.Sprintf("  hiring bar   P(edge) %.2f, %.0f%% of folds positive, %d trades minimum\n",
		p.Criteria.MinDeflatedSharpe, p.Criteria.MinPositiveFoldFraction*100, p.Criteria.MinTrades)
	out += "  agents       "
	for i, a := range p.Agents {
		if i > 0 {
			out += ", "
		}
		out += a.Name()
	}
	return out + "\n"
}
