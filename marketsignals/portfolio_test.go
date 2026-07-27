package marketsignals

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// correlatedUniverse builds n series driven by a shared factor plus
// idiosyncratic noise. rho near 1 makes everything one bet; rho near 0 makes
// them independent.
func correlatedUniverse(n, bars int, rho float64, seed int64) *Universe {
	rng := rand.New(rand.NewSource(seed))
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	u := &Universe{Series: map[string]*Series{}}
	prices := make([]float64, n)
	series := make([]*Series, n)
	for i := range prices {
		prices[i] = 100
		sym := string(rune('A' + i))
		series[i] = &Series{Symbol: sym, Interval: time.Hour}
		u.Series[sym] = series[i]
		u.Instruments = append(u.Instruments, Instrument{
			Symbol: sym, Class: ClassCrypto, Sector: SectorLargeAlt,
			Venue: VenueCEX, ContinuousTrading: true,
		})
	}

	for b := 0; b < bars; b++ {
		factor := rng.NormFloat64() * 0.01
		for i := range prices {
			idio := rng.NormFloat64() * 0.01
			r := math.Sqrt(rho)*factor + math.Sqrt(1-rho)*idio
			open := prices[i]
			prices[i] = open * math.Exp(r)
			series[i].Candles = append(series[i].Candles, Candle{
				Time: start.Add(time.Duration(b) * time.Hour),
				Open: open, High: math.Max(open, prices[i]) * 1.001,
				Low: math.Min(open, prices[i]) * 0.999, Close: prices[i], Volume: 1000,
			})
		}
	}
	return u
}

// TestCrossSectional_LongOnlyBookShrinksWhenCorrelationsRise is the finding
// this file exists for. Ten LONG positions in a market with one factor are not
// ten bets. Sizing each position separately would produce roughly ten times
// the intended risk, and would do so exactly during the correlated selloff
// when that mistake is least survivable.
func TestCrossSectional_LongOnlyBookShrinksWhenCorrelationsRise(t *testing.T) {
	// Genuinely independent against almost perfectly correlated. With three
	// names in the long sleeve the expected gap is about sqrt(3).
	independent := correlatedUniverse(10, 600, 0.0, 7)
	together := correlatedUniverse(10, 600, 0.97, 7)

	cs := NewCrossSectional()
	cs.MinNames = 8
	cs.ShortFraction = 0 // long-only: this is where the claim holds
	// Both caps have to be lifted: in a long-only book net exposure equals
	// gross, so MaxNet binds at the same point MaxGross does, and either one
	// left at its default would mask what volatility targeting is doing.
	cs.MaxGross, cs.MaxNet = 5, 5

	a, err := cs.Allocate(independent, independent.Bars())
	if err != nil {
		t.Fatalf("independent: %v", err)
	}
	b, err := cs.Allocate(together, together.Bars())
	if err != nil {
		t.Fatalf("correlated: %v", err)
	}

	if a.Gross == 0 || b.Gross == 0 {
		t.Fatalf("no book allocated: %s / %s", a.Reason, b.Reason)
	}
	if !(a.Gross > 1.5*b.Gross) {
		t.Fatalf("gross exposure is %.3f on independent assets against %.3f on assets that "+
			"move as one — the book must shrink as diversification disappears",
			a.Gross, b.Gross)
	}
	t.Logf("long-only — independent: gross %.2f, div ratio %.2f\n"+
		"long-only — correlated:  gross %.2f, div ratio %.2f",
		a.Gross, a.DiversificationRatio, b.Gross, b.DiversificationRatio)
}

// TestCrossSectional_LongShortNetsOutTheCommonFactor records the property that
// corrected the test above, because it is genuinely counterintuitive and worth
// having written down.
//
// For a LONG-ONLY book, rising correlation is pure harm: diversification
// vanishes and the book must shrink. For a long/short book it is largely
// help, because the factor every asset shares appears on both sides and
// cancels. A market-neutral book's risk lives in what is LEFT once the common
// move is removed, so a one-factor market is a quieter place for it, not a
// more dangerous one.
//
// The same volatility targeting handles both without a special case, which is
// the argument for sizing from a covariance matrix rather than from a rule
// about how many positions to hold.
func TestCrossSectional_LongShortNetsOutTheCommonFactor(t *testing.T) {
	together := correlatedUniverse(10, 600, 0.97, 7)

	longOnly := NewCrossSectional()
	longOnly.MinNames, longOnly.ShortFraction, longOnly.MaxGross = 8, 0, 20

	neutral := NewCrossSectional()
	neutral.MinNames, neutral.MaxGross = 8, 20

	lo, err := longOnly.Allocate(together, together.Bars())
	if err != nil {
		t.Fatalf("long-only: %v", err)
	}
	ls, err := neutral.Allocate(together, together.Bars())
	if err != nil {
		t.Fatalf("long/short: %v", err)
	}

	if !(ls.PortfolioVol < lo.PortfolioVol) {
		t.Fatalf("book volatility is %.4f long/short against %.4f long-only in a one-factor "+
			"market — the shared move should cancel across the two sleeves",
			ls.PortfolioVol, lo.PortfolioVol)
	}
	t.Logf("one-factor market: long-only book vol %.1f%%, market-neutral book vol %.1f%%",
		lo.PortfolioVol*100, ls.PortfolioVol*100)
}

// TestDiversificationRatio_ExposesFakeDiversification puts a number on the
// difference between holding ten things and having ten bets.
func TestDiversificationRatio_ExposesFakeDiversification(t *testing.T) {
	const n = 9
	w := make([]float64, n)
	for i := range w {
		w[i] = 1.0 / n
	}

	// Independent assets: the ratio approaches sqrt(N).
	indep := make([][]float64, n)
	rng := rand.New(rand.NewSource(3))
	for i := range indep {
		indep[i] = make([]float64, 2000)
		for k := range indep[i] {
			indep[i][k] = rng.NormFloat64() * 0.01
		}
	}
	got := DiversificationRatio(w, Covariance(indep))
	if math.Abs(got-math.Sqrt(n)) > 0.5 {
		t.Fatalf("ratio %.3f on independent assets, want about %.3f", got, math.Sqrt(n))
	}

	// One factor: every asset is the same asset, and the ratio collapses to 1.
	same := make([][]float64, n)
	shared := make([]float64, 2000)
	for k := range shared {
		shared[k] = rng.NormFloat64() * 0.01
	}
	for i := range same {
		same[i] = shared
	}
	if got := DiversificationRatio(w, Covariance(same)); math.Abs(got-1) > 0.01 {
		t.Fatalf("ratio %.3f when every asset is identical, want 1 — this is the number that "+
			"says ten positions are one bet", got)
	}
}

func TestCrossSectional_RanksWinnersLongAndLosersShort(t *testing.T) {
	// A universe where one name has clearly trended up and one clearly down.
	u := correlatedUniverse(10, 600, 0.05, 11)
	syms := u.Symbols()
	winner, loser := syms[0], syms[1]

	for i := range u.Series[winner].Candles {
		f := 1 + 0.002*float64(i)
		c := &u.Series[winner].Candles[i]
		c.Open, c.High, c.Low, c.Close = c.Open*f, c.High*f, c.Low*f, c.Close*f
	}
	for i := range u.Series[loser].Candles {
		f := 1 / (1 + 0.002*float64(i))
		c := &u.Series[loser].Candles[i]
		c.Open, c.High, c.Low, c.Close = c.Open*f, c.High*f, c.Low*f, c.Close*f
	}

	cs := NewCrossSectional()
	cs.MinNames = 8
	got, err := cs.Allocate(u, u.Bars())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Weights[winner] <= 0 {
		t.Fatalf("weight %.4f on the strongest name; cross-sectional momentum holds winners long",
			got.Weights[winner])
	}
	if got.Weights[loser] >= 0 {
		t.Fatalf("weight %.4f on the weakest name; it belongs in the short sleeve",
			got.Weights[loser])
	}
}

func TestCrossSectional_RespectsGrossAndNetCaps(t *testing.T) {
	u := correlatedUniverse(10, 600, 0.02, 5)
	cs := NewCrossSectional()
	cs.MinNames = 8
	cs.TargetVol = 100 // absurd, so only the caps can bind
	cs.MaxGross = 0.6
	cs.MaxNet = 0.1

	got, err := cs.Allocate(u, u.Bars())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Gross > cs.MaxGross+1e-9 {
		t.Fatalf("gross %.4f above the %.2f cap", got.Gross, cs.MaxGross)
	}
	if math.Abs(got.Net) > cs.MaxNet+1e-9 {
		t.Fatalf("net %.4f above the %.2f cap", got.Net, cs.MaxNet)
	}
}

func TestCrossSectional_RefusesATinyUniverse(t *testing.T) {
	u := correlatedUniverse(4, 600, 0.3, 9)
	got, err := NewCrossSectional().Allocate(u, u.Bars())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if len(got.Weights) != 0 {
		t.Fatalf("allocated across %d names; ranking four assets into deciles is a coin flip "+
			"with extra arithmetic", len(got.Weights))
	}
	if !strings.Contains(got.Reason, "below the") {
		t.Fatalf("reason %q should say the universe is too small", got.Reason)
	}
}

// TestUniverse_AlignTrimsToCommonBars. Comparing one asset's Monday against
// another's Tuesday produces a ranking of nothing, and nothing about the
// output looks wrong.
func TestUniverse_AlignTrimsToCommonBars(t *testing.T) {
	u := correlatedUniverse(3, 100, 0.3, 4)
	syms := u.Symbols()

	// One asset was listed late; another halted at the end.
	u.Series[syms[0]].Candles = u.Series[syms[0]].Candles[10:]
	u.Series[syms[1]].Candles = u.Series[syms[1]].Candles[:90]

	if err := u.Align(); err != nil {
		t.Fatalf("align: %v", err)
	}
	want := 80 // bars 10..89
	for _, s := range u.Symbols() {
		if got := len(u.Series[s].Candles); got != want {
			t.Fatalf("%s has %d bars after alignment, want %d", s, got, want)
		}
	}
	first := u.Series[syms[0]].Candles[0].Time
	for _, s := range u.Symbols() {
		if !u.Series[s].Candles[0].Time.Equal(first) {
			t.Fatalf("%s starts at %s, %s starts at %s", s, u.Series[s].Candles[0].Time, syms[0], first)
		}
	}
}

func TestUniverse_AlignRejectsMixedIntervals(t *testing.T) {
	u := correlatedUniverse(3, 100, 0.3, 6)
	for _, s := range u.Symbols() {
		u.Series[s].Interval = 4 * time.Hour
		break
	}
	if err := u.Align(); err == nil {
		t.Fatal("expected an error for series with different bar lengths")
	}
}

func TestCovariance_MatchesVarianceOnTheDiagonal(t *testing.T) {
	rets := [][]float64{
		{0.01, -0.02, 0.03, -0.01, 0.02},
		{0.02, -0.01, 0.01, -0.03, 0.01},
	}
	cov := Covariance(rets)
	for i, r := range rets {
		want := StdDev(r, len(r))
		if got := math.Sqrt(cov[i][i]); math.Abs(got-want) > 1e-12 {
			t.Fatalf("series %d: diagonal gives sd %.9f, StdDev gives %.9f", i, got, want)
		}
	}
	if math.Abs(cov[0][1]-cov[1][0]) > 1e-15 {
		t.Fatal("covariance matrix is not symmetric")
	}
}

// TestCooldownGuard_StandsAsideAfterALoss covers the protection borrowed from
// Freqtrade: a trend agent stopped out by a spike sees the same setup on the
// next bar and re-enters, paying costs for a market that has not changed.
func TestCooldownGuard_StandsAsideAfterALoss(t *testing.T) {
	g := NewCooldownGuard()
	g.Bars = 3
	g.LossThreshold = 0.01

	v := NewView(trendSeries(100, 1, 0.01), 100)

	g.RecordEquity(10_000)
	if adj := g.ModulateRisk(v); adj.Scale != 1 {
		t.Fatalf("scale %.2f before any loss", adj.Scale)
	}

	g.RecordEquity(9_800) // a 2% loss
	adj := g.ModulateRisk(v)
	if adj.Scale != 0 || !adj.Veto {
		t.Fatalf("scale %.2f after a 2%% loss; want a full stand-aside", adj.Scale)
	}
	if !strings.Contains(adj.Reason, "has not changed") {
		t.Fatalf("reason %q should explain why re-entering is wrong", adj.Reason)
	}

	// It expires rather than latching for ever.
	for i := 0; i < 5; i++ {
		g.RecordEquity(9_800)
	}
	if adj := g.ModulateRisk(v); adj.Scale != 1 {
		t.Fatalf("scale %.2f long after the cooldown should have expired", adj.Scale)
	}
}

func TestCooldownGuard_IgnoresSmallMoves(t *testing.T) {
	g := NewCooldownGuard()
	g.LossThreshold = 0.05

	v := NewView(trendSeries(100, 1, 0.01), 100)
	g.RecordEquity(10_000)
	g.RecordEquity(9_950) // 0.5%, well under the threshold

	if adj := g.ModulateRisk(v); adj.Scale != 1 {
		t.Fatalf("scale %.2f after ordinary noise; the guard is for stop-outs, not for drift",
			adj.Scale)
	}
}
