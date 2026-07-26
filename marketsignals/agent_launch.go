package marketsignals

import (
	"fmt"
	"math"
	"sort"
)

// Launch is the observable state of a freshly deployed token. Every field is
// something an indexer, an RPC node or a simulated trade can establish before
// risking money — nothing here requires trusting the project's own claims,
// which are worth nothing at this stage of a token's life.
type Launch struct {
	Symbol       string  `json:"symbol"`
	Chain        string  `json:"chain"`
	TokenAddress string  `json:"token_address"`
	AgeMinutes   float64 `json:"age_minutes"`

	// Liquidity and its permanence. LiquidityLockedPct and LPBurnedPct are
	// the difference between a pool and a promise.
	LiquidityUSD       float64 `json:"liquidity_usd"`
	LiquidityLockedPct float64 `json:"liquidity_locked_pct"`
	LockRemainingDays  float64 `json:"lock_remaining_days"`
	LPBurnedPct        float64 `json:"lp_burned_pct"`

	// Authorities still held by the deployer. Any of these live means the
	// token's rules can change after you buy it.
	MintAuthorityActive   bool `json:"mint_authority_active"`
	FreezeAuthorityActive bool `json:"freeze_authority_active"`
	UpgradeableProxy      bool `json:"upgradeable_proxy"`
	ContractVerified      bool `json:"contract_verified"`

	// Distribution. TopHolderPct and Top10Pct should already EXCLUDE the
	// liquidity pool and burn addresses; a screener that counts the pool as a
	// holder concludes every healthy token is dangerously concentrated.
	TopHolderPct float64 `json:"top_holder_pct"`
	Top10Pct     float64 `json:"top10_pct"`
	HolderCount  int     `json:"holder_count"`
	SniperPct    float64 `json:"sniper_pct"` // supply taken by wallets that bought in the first blocks

	// Round-trip reality. Can you actually sell, and what does it cost?
	SellSimulationPassed bool    `json:"sell_simulation_passed"`
	BuyTaxPct            float64 `json:"buy_tax_pct"`
	SellTaxPct           float64 `json:"sell_tax_pct"`

	// Early activity. UniqueBuyers1h against TxCount1h is the wash-trading
	// tell: volume is trivially fabricated, distinct counterparties are not.
	VolumeUSD1h    float64 `json:"volume_usd_1h"`
	TxCount1h      int     `json:"tx_count_1h"`
	UniqueBuyers1h int     `json:"unique_buyers_1h"`

	// Deployer history — the single most predictive field in this struct.
	DeployerPriorRugs     int  `json:"deployer_prior_rugs"`
	DeployerPriorTokens   int  `json:"deployer_prior_tokens"`
	DeployerFundedByMixer bool `json:"deployer_funded_by_mixer"`
}

// Verdict is the screener's decision.
type Verdict string

const (
	// Reject means at least one disqualifying condition holds. No score can
	// overturn it, and no combination of good properties compensates: these
	// are conditions under which the downside is "the position goes to zero
	// and cannot be sold", which is not a risk you offset with upside.
	Reject Verdict = "reject"
	// Watch means nothing disqualifying was found, but the positive evidence
	// is not strong enough to justify a position yet.
	Watch Verdict = "watch"
	// Accept means no hard failure and enough independent positive evidence.
	// It is still a small-size, high-variance bet — the screener removes the
	// obviously engineered losses, it does not manufacture an edge.
	Accept Verdict = "accept"
)

// LaunchScreen is the full reasoning behind a verdict, kept as text so that a
// rejection can be argued with rather than merely obeyed.
type LaunchScreen struct {
	Symbol    string
	Verdict   Verdict
	Score     float64 // 0..100, only meaningful when HardFails is empty
	HardFails []string
	Positives []string
	Concerns  []string
}

// LaunchPolicy holds the thresholds. They are strict on purpose: for new
// launches the base rate of "this was engineered to take your money" is high
// enough that a screener tuned to avoid missing winners will mostly find
// losses.
type LaunchPolicy struct {
	MinLiquidityUSD    float64
	MinLockedOrBurnPct float64
	MinLockDays        float64
	MaxSellTaxPct      float64
	MaxTopHolderPct    float64
	MaxTop10Pct        float64
	MaxSniperPct       float64
	MinHolders         int
	MinUniqueBuyers    int
	MinBuyerTxRatio    float64 // unique buyers / transactions, wash-trading floor
	MinAgeMinutes      float64
	AcceptScore        float64
}

// DefaultLaunchPolicy is the strict baseline described above.
func DefaultLaunchPolicy() LaunchPolicy {
	return LaunchPolicy{
		MinLiquidityUSD:    25_000,
		MinLockedOrBurnPct: 90,
		MinLockDays:        30,
		MaxSellTaxPct:      5,
		MaxTopHolderPct:    15,
		MaxTop10Pct:        35,
		MaxSniperPct:       25,
		MinHolders:         250,
		MinUniqueBuyers:    100,
		MinBuyerTxRatio:    0.25,
		MinAgeMinutes:      30,
		AcceptScore:        70,
	}
}

// LaunchAgent screens new deployments. It is not a market agent — it never
// sees candles and never joins the ensemble's directional vote, because the
// question it answers is categorically different: not "which way does this
// go" but "is this instrument safe to hold at all".
type LaunchAgent struct {
	Policy LaunchPolicy
}

// NewLaunchAgent returns a screener on the strict default policy.
func NewLaunchAgent() *LaunchAgent { return &LaunchAgent{Policy: DefaultLaunchPolicy()} }

func (a *LaunchAgent) Name() string   { return "launch" }
func (a *LaunchAgent) Family() Family { return FamilyScreen }

// Screen applies the hard filters first and only then scores. The ordering is
// the design: a scoring model that lets a great liquidity number outweigh a
// live mint authority will eventually buy a token whose supply the deployer
// can multiply at will. Those conditions get a veto, not a weight.
func (a *LaunchAgent) Screen(l Launch) LaunchScreen {
	p := a.Policy
	s := LaunchScreen{Symbol: l.Symbol}

	fail := func(format string, args ...any) {
		s.HardFails = append(s.HardFails, fmt.Sprintf(format, args...))
	}

	// ── Vetoes: can the position be exited, and can the rules change? ──

	if !l.SellSimulationPassed {
		fail("honeypot: a simulated sell does not succeed — the position cannot be exited")
	}
	if l.SellTaxPct > p.MaxSellTaxPct {
		fail("sell tax %.1f%% exceeds %.1f%% — the exit is priced to punish you",
			l.SellTaxPct, p.MaxSellTaxPct)
	}
	if l.MintAuthorityActive {
		fail("mint authority still live — supply can be diluted after you buy")
	}
	if l.FreezeAuthorityActive {
		fail("freeze authority still live — your balance can be frozen in place")
	}
	if l.UpgradeableProxy {
		fail("upgradeable proxy — the contract you audited is not the one you will sell through")
	}
	if !l.ContractVerified {
		fail("unverified source — the contract's behaviour cannot be read before trading it")
	}

	// Liquidity that can be withdrawn is not liquidity; it is the exit the
	// deployer takes and you do not.
	secured := math.Max(l.LiquidityLockedPct, l.LPBurnedPct)
	if secured < p.MinLockedOrBurnPct {
		fail("only %.0f%% of LP is locked or burned (need %.0f%%) — a rug needs no exploit, just a withdrawal",
			secured, p.MinLockedOrBurnPct)
	} else if l.LPBurnedPct < p.MinLockedOrBurnPct && l.LockRemainingDays < p.MinLockDays {
		fail("LP lock expires in %.0f days (need %.0f) — a lock that ends is a scheduled rug",
			l.LockRemainingDays, p.MinLockDays)
	}

	if l.LiquidityUSD < p.MinLiquidityUSD {
		fail("liquidity $%.0f below $%.0f — your own exit moves the price against you",
			l.LiquidityUSD, p.MinLiquidityUSD)
	}

	// ── Vetoes: who is on the other side? ──

	if l.DeployerPriorRugs > 0 {
		fail("deployer has %d prior rug(s) — the most predictive fact available about this token",
			l.DeployerPriorRugs)
	}
	if l.DeployerFundedByMixer {
		fail("deployer wallet funded through a mixer — deliberate un-attribution before launch")
	}
	if l.TopHolderPct > p.MaxTopHolderPct {
		fail("top holder controls %.1f%% (max %.1f%%) — one wallet can end the price",
			l.TopHolderPct, p.MaxTopHolderPct)
	}
	if l.Top10Pct > p.MaxTop10Pct {
		fail("top 10 hold %.1f%% (max %.1f%%)", l.Top10Pct, p.MaxTop10Pct)
	}
	if l.SniperPct > p.MaxSniperPct {
		fail("snipers took %.1f%% in the opening blocks (max %.1f%%) — that supply exits into your buy",
			l.SniperPct, p.MaxSniperPct)
	}

	// Too young to have any evidence at all. Not a moral judgement: there is
	// simply no distribution or flow data yet to distinguish this from a scam.
	if l.AgeMinutes < p.MinAgeMinutes {
		fail("only %.0f minutes old (need %.0f) — no observable history to judge",
			l.AgeMinutes, p.MinAgeMinutes)
	}

	// Fabricated activity. Volume is free to manufacture; distinct
	// counterparties are not.
	if l.TxCount1h > 0 {
		ratio := float64(l.UniqueBuyers1h) / float64(l.TxCount1h)
		if ratio < p.MinBuyerTxRatio {
			fail("%d buyers across %d trades (ratio %.2f, need %.2f) — consistent with wash trading",
				l.UniqueBuyers1h, l.TxCount1h, ratio, p.MinBuyerTxRatio)
		}
	}
	if l.HolderCount < p.MinHolders {
		fail("%d holders below %d — no distribution to sell into", l.HolderCount, p.MinHolders)
	}
	if l.UniqueBuyers1h < p.MinUniqueBuyers {
		fail("%d unique buyers in the first hour below %d", l.UniqueBuyers1h, p.MinUniqueBuyers)
	}

	sort.Strings(s.HardFails)
	if len(s.HardFails) > 0 {
		s.Verdict = Reject
		s.Score = 0
		return s
	}

	// ── Scoring, reached only by launches that failed nothing above ──
	//
	// Five independent components, capped so no single one can carry a
	// verdict. Each is evidence of organic demand or of the deployer having
	// given up control; passing all five is the minimum for Accept.

	type component struct {
		label  string
		points float64
		max    float64
	}
	comps := []component{
		{"LP permanence", 20 * clamp((secured-p.MinLockedOrBurnPct)/(100-p.MinLockedOrBurnPct), 0, 1), 20},
		{"liquidity depth", 20 * clamp(math.Log10(l.LiquidityUSD/p.MinLiquidityUSD)/math.Log10(10), 0, 1), 20},
		{"holder distribution", 20 * clamp((p.MaxTop10Pct-l.Top10Pct)/p.MaxTop10Pct, 0, 1), 20},
		{"organic buyer breadth", 20 * clamp(float64(l.UniqueBuyers1h)/float64(4*p.MinUniqueBuyers), 0, 1), 20},
		{"survived time", 20 * clamp(l.AgeMinutes/(24*60), 0, 1), 20},
	}
	for _, c := range comps {
		s.Score += c.points
		switch {
		case c.points >= 0.75*c.max:
			s.Positives = append(s.Positives, fmt.Sprintf("%s: strong (%.0f/%.0f)", c.label, c.points, c.max))
		case c.points <= 0.25*c.max:
			s.Concerns = append(s.Concerns, fmt.Sprintf("%s: weak (%.0f/%.0f)", c.label, c.points, c.max))
		}
	}

	// A deployer with no track record is not a positive, only the absence of
	// a negative — worth flagging so nobody reads a clean screen as a
	// reference.
	if l.DeployerPriorTokens == 0 {
		s.Concerns = append(s.Concerns, "deployer has no prior tokens — no history either way")
	}
	if l.BuyTaxPct > 0 || l.SellTaxPct > 0 {
		s.Concerns = append(s.Concerns,
			fmt.Sprintf("taxed transfers (%.1f%% buy / %.1f%% sell) erode any edge on every round trip",
				l.BuyTaxPct, l.SellTaxPct))
	}

	if s.Score >= p.AcceptScore {
		s.Verdict = Accept
	} else {
		s.Verdict = Watch
	}
	return s
}

// Summary renders the screen as a short human-readable report.
func (s LaunchScreen) Summary() string {
	out := fmt.Sprintf("%s: %s (score %.0f/100)\n", s.Symbol, s.Verdict, s.Score)
	for _, f := range s.HardFails {
		out += "  DISQUALIFYING  " + f + "\n"
	}
	for _, p := range s.Positives {
		out += "  +  " + p + "\n"
	}
	for _, c := range s.Concerns {
		out += "  -  " + c + "\n"
	}
	return out
}
