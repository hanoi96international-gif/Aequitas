package marketsignals

import (
	"math"
	"strings"
	"testing"
	"time"
)

func btc() Instrument {
	return Instrument{
		Symbol: "BTCUSDT", Name: "Bitcoin", Class: ClassCrypto, Sector: SectorMajor,
		Venue: VenueCEX, HasPerpetual: true, ContinuousTrading: true,
		MedianDailyVolumeUSD: 2e10,
	}
}

func memecoin() Instrument {
	return Instrument{
		Symbol: "WIFHAT", Class: ClassCrypto, Sector: SectorMeme, Venue: VenueDEX,
		Chain: "solana", Address: "So111", ContinuousTrading: true,
		MedianDailyVolumeUSD: 3e6,
		PoolLiquidityUSD:     500_000,
		AccountUSD:           10_000,
	}
}

func spyETF() Instrument {
	return Instrument{Symbol: "SPY", Name: "S&P 500 ETF", Class: ClassETF, Venue: VenueExchange}
}

func TestInstrument_ValidateCatchesMisclassification(t *testing.T) {
	cases := map[string]Instrument{
		"crypto sector on an ETF": {Symbol: "SPY", Class: ClassETF, Sector: SectorMeme, Venue: VenueExchange},
		"crypto without a sector": {Symbol: "X", Class: ClassCrypto, Venue: VenueCEX},
		"crypto on an equity venue": {
			Symbol: "X", Class: ClassCrypto, Sector: SectorMajor, Venue: VenueExchange},
		"ETF trading continuously": {
			Symbol: "SPY", Class: ClassETF, Venue: VenueExchange, ContinuousTrading: true},
		"DEX token with no address": {
			Symbol: "X", Class: ClassCrypto, Sector: SectorMeme, Venue: VenueDEX, Chain: "solana"},
		"DEX token with no pool size": {
			Symbol: "X", Class: ClassCrypto, Sector: SectorMeme, Venue: VenueDEX,
			Chain: "solana", Address: "a", AccountUSD: 1000},
		"DEX token with no account size": {
			Symbol: "X", Class: ClassCrypto, Sector: SectorMeme, Venue: VenueDEX,
			Chain: "solana", Address: "a", PoolLiquidityUSD: 100_000},
		"unknown class": {Symbol: "X", Class: "commodity", Venue: VenueExchange},
	}
	for name, i := range cases {
		t.Run(name, func(t *testing.T) {
			if err := i.Validate(); err == nil {
				t.Fatal("expected a validation error rather than a silently wrong expert profile")
			}
		})
	}

	for _, ok := range []Instrument{btc(), memecoin(), spyETF()} {
		if err := ok.Validate(); err != nil {
			t.Fatalf("%s should be valid: %v", ok.Symbol, err)
		}
	}
}

// TestExpertFor_MemecoinsAreTreatedNothingLikeMajors is the point of the whole
// taxonomy. The same code applied with one set of assumptions to both would
// size a memecoin position roughly ten times too large and assume costs an
// order of magnitude too low.
func TestExpertFor_MemecoinsAreTreatedNothingLikeMajors(t *testing.T) {
	major, err := ExpertFor(btc())
	if err != nil {
		t.Fatalf("major: %v", err)
	}
	meme, err := ExpertFor(memecoin())
	if err != nil {
		t.Fatalf("meme: %v", err)
	}

	// Compared at a representative position size, because an AMM's cost is a
	// function of size and a single "per unit" number does not exist for it.
	memeCost := meme.Costs.CostFraction(0.1)
	majorCost := major.Costs.CostFraction(0.1)
	if !(memeCost > 5*majorCost) {
		t.Fatalf("memecoin cost %.4f is not materially above the major's %.4f at a 10%% "+
			"position — thin pools and MEV are the dominant term in a memecoin round trip",
			memeCost, majorCost)
	}
	if !(meme.MaxPositionFraction < major.MaxPositionFraction/2) {
		t.Fatalf("memecoin max position %.2f against the major's %.2f",
			meme.MaxPositionFraction, major.MaxPositionFraction)
	}
	if !(meme.Criteria.MinTrades > major.Criteria.MinTrades) {
		t.Fatal("the memecoin segment should demand more evidence, not less: its survivors " +
			"are a small and flattering sample")
	}
}

// TestExpertFor_ExcludesAgentsWithoutInputs: an agent fed zeros where its
// input should be does not produce a weak signal, it produces a confident one
// computed from nothing.
func TestExpertFor_ExcludesAgentsWithoutInputs(t *testing.T) {
	etf, err := ExpertFor(spyETF())
	if err != nil {
		t.Fatalf("etf: %v", err)
	}
	if hasAgent(etf, "funding") {
		t.Fatal("the funding agent is in the ETF profile, but an ETF has no perpetual and " +
			"therefore no funding rate to read")
	}

	withPerp, _ := ExpertFor(btc())
	if !hasAgent(withPerp, "funding") {
		t.Fatal("a major with a perpetual should carry the funding agent")
	}

	spot := btc()
	spot.HasPerpetual = false
	noPerp, _ := ExpertFor(spot)
	if hasAgent(noPerp, "funding") {
		t.Fatal("spot-only instrument still carries the funding agent")
	}
}

// TestExpertFor_ReversionIsConfinedToMarketsWithAMean encodes a judgement
// worth being able to argue with: fading extension requires something to
// revert TO. A basket has one. A memecoin does not.
func TestExpertFor_ReversionIsConfinedToMarketsWithAMean(t *testing.T) {
	etf, _ := ExpertFor(spyETF())
	if !hasFamily(etf, FamilyReversion) {
		t.Fatal("reversion should be available on a diversified basket")
	}

	for _, i := range []Instrument{
		memecoin(),
		{Symbol: "AI", Class: ClassCrypto, Sector: SectorNarrative, Venue: VenueCEX, ContinuousTrading: true},
		{Symbol: "ALT", Class: ClassCrypto, Sector: SectorLargeAlt, Venue: VenueCEX, ContinuousTrading: true},
	} {
		p, err := ExpertFor(i)
		if err != nil {
			t.Fatalf("%s: %v", i.Symbol, err)
		}
		if hasFamily(p, FamilyReversion) {
			t.Fatalf("%s (%s) carries a reversion agent; in a market that can trend 80%% in a "+
				"month, fading extension is a way of being repeatedly right and finally ruined",
				i.Symbol, p.Name)
		}
	}
}

// TestExpertFor_RefusesSegmentsWithNothingToMeasure. Saying "this framework
// does not apply here" is a more useful output than a confident number
// computed from nothing.
func TestExpertFor_RefusesSegmentsWithNothingToMeasure(t *testing.T) {
	for _, tc := range []struct {
		sector CryptoSector
		expect string
	}{
		{SectorNewLaunch, "no usable price history"},
		{SectorStablecoin, "peg-break risk"},
	} {
		i := Instrument{Symbol: "X", Class: ClassCrypto, Sector: tc.sector, Venue: VenueCEX,
			ContinuousTrading: true}
		p, err := ExpertFor(i)
		if err != nil {
			t.Fatalf("%s: %v", tc.sector, err)
		}
		if p.TradeableAtAll {
			t.Fatalf("%s was marked tradeable", tc.sector)
		}
		if !strings.Contains(p.NotTradeableReason, tc.expect) {
			t.Fatalf("%s reason %q should mention %q", tc.sector, p.NotTradeableReason, tc.expect)
		}
		if !strings.Contains(p.Describe(), "NOT TRADEABLE") {
			t.Fatalf("%s: Describe() must lead with the refusal", tc.sector)
		}
	}
}

func TestExpertProfile_ConfiguresItsBacktesterAndPanel(t *testing.T) {
	p, _ := ExpertFor(memecoin())
	bt := p.Backtester()
	if bt.Costs != p.Costs {
		t.Fatal("profile costs did not reach the backtester")
	}
	if bt.Risk.MaxLeverage != p.MaxPositionFraction {
		t.Fatalf("leverage cap %.2f, want the profile's %.2f", bt.Risk.MaxLeverage, p.MaxPositionFraction)
	}
	if got := p.Panel().Criteria; got != p.Criteria {
		t.Fatal("profile hiring criteria did not reach the panel")
	}
	if got := p.Ensemble(); len(got.Agents) != len(p.Agents) {
		t.Fatalf("ensemble has %d agents, profile has %d", len(got.Agents), len(p.Agents))
	}
}

func hasAgent(p ExpertProfile, name string) bool {
	for _, a := range p.Agents {
		if a.Name() == name {
			return true
		}
	}
	return false
}

func hasFamily(p ExpertProfile, f Family) bool {
	for _, a := range p.Agents {
		if a.Family() == f {
			return true
		}
	}
	return false
}

// ── Social ───────────────────────────────────────────────────────────────

func snapshot(posts, authors int, dup, ageDays, newRatio float64) SocialSnapshot {
	return SocialSnapshot{
		Posts: posts, UniqueAuthors: authors,
		DuplicateTextRatio: dup, MedianAuthorAgeDays: ageDays, NewAccountRatio: newRatio,
	}
}

func TestSocialSnapshot_CredibilityMultipliesItsComponents(t *testing.T) {
	good := snapshot(100, 80, 0.10, 360, 0.10)
	want := 0.8 * 0.9 * 1.0 * 0.9
	if got := good.Credibility(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("credibility %.4f, want %.4f", got, want)
	}
	if got := good.CredibleAttention(); math.Abs(got-100*want) > 1e-9 {
		t.Fatalf("credible attention %.4f, want %.4f", got, 100*want)
	}

	// Failing any single component must collapse the score, not be averaged
	// away by the others — each one is a complete way of faking attention.
	base := good
	for name, mutate := range map[string]func(*SocialSnapshot){
		"one account posting repeatedly": func(s *SocialSnapshot) { s.UniqueAuthors = 2 },
		"copy-paste campaign":            func(s *SocialSnapshot) { s.DuplicateTextRatio = 0.98 },
		"freshly created swarm":          func(s *SocialSnapshot) { s.NewAccountRatio = 0.98 },
		"brand-new accounts":             func(s *SocialSnapshot) { s.MedianAuthorAgeDays = 2 },
	} {
		s := base
		mutate(&s)
		if got := s.Credibility(); got > 0.15 {
			t.Fatalf("%s still scored %.3f credible", name, got)
		}
	}
}

// socialSeries builds a price series with attention attached. The last bar's
// posts are set explicitly; the history is basePosts with a spike every
// spikeEvery bars (0 for none). The spikes are periodic rather than clustered
// at the start so that they fall inside the agent's trailing window — a
// fixture whose "history" sits before the lookback window teaches the agent
// nothing and silently tests a different case than the one intended.
func socialSeries(bars, basePosts, spikePosts, spikeEvery, lastPosts int) *Series {
	s := &Series{Symbol: "SOC", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < bars; i++ {
		s.Candles = append(s.Candles, Candle{
			Time: t.Add(time.Duration(i) * time.Hour),
			Open: 100, High: 101, Low: 99, Close: 100, Volume: 1,
		})
		posts := basePosts
		if spikeEvery > 0 && i%spikeEvery == 0 {
			posts = spikePosts
		}
		if i == bars-1 {
			posts = lastPosts
		}
		snap := snapshot(posts, posts*8/10, 0.10, 360, 0.10)
		snap.Time = s.Candles[i].Time
		s.Social = append(s.Social, snap)
	}
	return s
}

func TestSocialAgent_FadesPeakAttention(t *testing.T) {
	// A quiet week, then the loudest bar in its own history.
	s := socialSeries(200, 10, 10, 0, 400)
	sig := NewSocialAgent().Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Short {
		t.Fatalf("agent = %s (%s); at peak credible attention the crowd is usually the exit "+
			"liquidity, not the buyer", sig.Dir, sig.Note)
	}
}

func TestSocialAgent_RefusesToReadBoughtAttention(t *testing.T) {
	s := socialSeries(200, 10, 10, 0, 400)
	// Same enormous post count, but from a handful of week-old accounts all
	// saying the same thing.
	last := &s.Social[len(s.Social)-1]
	last.UniqueAuthors = 6
	last.DuplicateTextRatio = 0.95
	last.MedianAuthorAgeDays = 4
	last.NewAccountRatio = 0.97

	sig := NewSocialAgent().Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Flat {
		t.Fatalf("agent took a %s on manufactured attention: %s", sig.Dir, sig.Note)
	}
	if !containsStr(sig.Note, "manufactured") {
		t.Fatalf("note %q should say the feed was not believed", sig.Note)
	}
}

// TestSocialAgent_ConstructiveBranchIsReachable guards the specific defect
// this agent shipped with in an earlier form: a constructive branch gated on
// conditions that could not both hold, so it never fired and its absence was
// invisible.
func TestSocialAgent_ConstructiveBranchIsReachable(t *testing.T) {
	// A mostly quiet history with a scattering of much louder bars, and a
	// current bar well above the baseline but below those earlier peaks.
	s := socialSeries(200, 4, 60, 7, 40)
	sig := NewSocialAgent().Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Long {
		t.Fatalf("agent = %s (%s); attention accelerating well above baseline but short of "+
			"its own extremes must be able to read constructively — otherwise the branch "+
			"is dead code that reads as working", sig.Dir, sig.Note)
	}
	if sig.Strength > 0.5 {
		t.Fatalf("strength %.2f; the constructive reading is the weakest claim this package "+
			"makes and must stay capped", sig.Strength)
	}
}

func TestSocialAgent_ModulatesRiskAtExtremes(t *testing.T) {
	a := NewSocialAgent()

	loud := socialSeries(200, 10, 10, 0, 400)
	if got := a.ModulateRisk(NewView(loud, 200)); got.Scale >= 1 {
		t.Fatalf("scale %.2f at peak attention; headline risk is elevated in both directions",
			got.Scale)
	}

	quiet := socialSeries(200, 10, 10, 0, 11)
	if got := a.ModulateRisk(NewView(quiet, 200)); got.Scale != 1 {
		t.Fatalf("scale %.2f in an unremarkable attention regime", got.Scale)
	}
}

func TestSocialAgent_StandsDownWithoutData(t *testing.T) {
	s := randomWalkSeries(300, 5, 0.01)
	sig := NewSocialAgent().Evaluate(NewView(s, 300))
	if sig.Dir != Flat || !containsStr(sig.Note, "no social data") {
		t.Fatalf("want a flat signal naming the missing data, got %s (%s)", sig.Dir, sig.Note)
	}
}

func TestRankByCredibleAttention_IgnoresBoughtVolume(t *testing.T) {
	bought := ProjectAttention{
		Instrument:        Instrument{Symbol: "PUMP", Class: ClassCrypto, Sector: SectorMeme, Venue: VenueDEX, Chain: "sol", Address: "a"},
		Current:           snapshot(50_000, 400, 0.97, 3, 0.98), // enormous and worthless
		BaselineAttention: 1,
	}
	genuine := ProjectAttention{
		Instrument:        Instrument{Symbol: "REAL", Class: ClassCrypto, Sector: SectorInfra, Venue: VenueCEX},
		Current:           snapshot(900, 780, 0.05, 400, 0.05),
		BaselineAttention: 100,
	}

	got := RankByCredibleAttention([]ProjectAttention{bought, genuine})
	if got[0].Instrument.Symbol != "REAL" {
		t.Fatalf("ranked %q first: 50,000 posts from 400 recycled accounts must not outrank "+
			"900 posts from 780 established ones", got[0].Instrument.Symbol)
	}
	if !containsStr(got[1].Note, "bought") {
		t.Fatalf("note %q should flag the purchased attention", got[1].Note)
	}
	if !containsStr(RankReport(got), "INVESTIGATE") {
		t.Fatal("the discovery report must say it produces a watchlist, not buy signals")
	}
}

func TestNormalizeDEXVolume_RemovesArbitrage(t *testing.T) {
	if got := NormalizeDEXVolume(1000, 0.7); math.Abs(got-300) > 1e-9 {
		t.Fatalf("normalized volume %.2f, want 300", got)
	}
	// Out-of-range inputs clamp rather than producing negative volume.
	if got := NormalizeDEXVolume(1000, 1.5); got != 0 {
		t.Fatalf("normalized volume %.2f, want 0", got)
	}
}

func TestCrossVenuePrice_FlagsMaterialDivergence(t *testing.T) {
	if _, material := CrossVenuePrice(100, 100.5); material {
		t.Fatal("a 50bp gap between venues is ordinary, not a data-quality alarm")
	}
	spread, material := CrossVenuePrice(100, 108)
	if !material || math.Abs(spread-0.08) > 1e-9 {
		t.Fatalf("spread %.4f material=%v, want 0.08 and true", spread, material)
	}
}
