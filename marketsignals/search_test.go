package marketsignals

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// momentumSeries has a REAL, tradeable edge built into it: log returns follow
// an AR(1) process, so a move genuinely predicts the next one.
//
// phi here is far larger than anything a liquid market exhibits — this is a
// positive control, not a claim about reality. Its job is to answer the one
// question a framework built around rejection has to answer about itself: when
// an edge is actually present, does the machinery find it, or does it reject
// everything unconditionally? A system that says no to real edges and to noise
// alike is not conservative, it is broken, and nothing else it reports can be
// trusted either.
func momentumSeries(n int, seed int64, phi, vol float64) *Series {
	rng := rand.New(rand.NewSource(seed))
	s := &Series{Symbol: "MOMENTUM", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	price, r := 100.0, 0.0
	for i := 0; i < n; i++ {
		open := price
		r = phi*r + rng.NormFloat64()*vol
		price = open * math.Exp(r)
		wick := math.Abs(rng.NormFloat64()) * open * vol * 0.3
		buy, sell := 1000.0, 1000.0
		if r > 0 {
			buy *= 1.5
		} else {
			sell *= 1.5
		}
		s.Candles = append(s.Candles, Candle{
			Time:   t,
			Open:   open,
			High:   math.Max(open, price) + wick,
			Low:    math.Min(open, price) - wick,
			Close:  price,
			Volume: buy + sell, BuyVolume: buy, SellVolume: sell,
		})
		t = t.Add(time.Hour)
	}
	return s
}

// TestSearch_FindsAnEdgeThatIsReallyThere is the positive control. Without it,
// every "no edge" verdict this package produces is unfalsifiable.
func TestSearch_FindsAnEdgeThatIsReallyThere(t *testing.T) {
	s := momentumSeries(2600, 31337, 0.40, 0.010)

	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 6, 2
	res, err := se.Run(s, BreakoutGrid())
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Log("\n" + res.Report())

	if res.Metrics.Sharpe <= 0 {
		t.Fatalf("the search lost money out of sample on a series with a genuine %0.2f "+
			"return autocorrelation built into it — the machinery cannot find an edge that "+
			"is unmistakably present, so its rejections mean nothing either", 0.40)
	}
	if res.DeflatedSharpe < 0.95 {
		t.Fatalf("out-of-sample Sharpe %.2f was not convincing (P=%.3f) despite a real edge; "+
			"the hurdle is rejecting signal along with noise",
			res.Metrics.Sharpe, res.DeflatedSharpe)
	}
}

// TestSearch_FindsNothingInNoise is the negative control, and the two must
// hold together: either one alone is easy to pass with broken code.
func TestSearch_FindsNothingInNoise(t *testing.T) {
	s := withFlow(randomWalkSeries(2600, 4711, 0.010))

	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 6, 2
	res, err := se.Run(s, BreakoutGrid())
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Log("\n" + res.Report())

	if res.DeflatedSharpe >= 0.95 && res.Metrics.Sharpe > 0 {
		t.Fatalf("the search claims an edge (Sharpe %.2f, P=%.3f) on a pure random walk",
			res.Metrics.Sharpe, res.DeflatedSharpe)
	}
}

// TestSearch_HindsightFlattersTheProcedure measures the gap the whole design
// exists to expose: what the best variant made when picked with hindsight,
// against what the search actually made choosing as it went. Reporting the
// first as if it were the second is the most expensive error in this field.
func TestSearch_HindsightFlattersTheProcedure(t *testing.T) {
	s := withFlow(randomWalkSeries(2600, 99, 0.012))

	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 6, 2
	res, err := se.Run(s, BreakoutGrid())
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.BestInHindsight == "" {
		t.Fatal("no hindsight winner recorded; the comparison is missing")
	}
	if !(res.HindsightSharpe > res.Metrics.Sharpe) {
		t.Fatalf("hindsight Sharpe %.2f did not exceed the procedure's %.2f — on noise the "+
			"best-of-27 must look better than choosing as you go, or the selection is "+
			"not actually happening", res.HindsightSharpe, res.Metrics.Sharpe)
	}
}

// TestSearch_SelectsOnlyOnThePast rewrites the tail of a series and confirms
// that the choices made in earlier folds are unchanged. A search that lets
// later data influence an earlier selection is optimising with hindsight while
// reporting an out-of-sample number.
func TestSearch_SelectsOnlyOnThePast(t *testing.T) {
	base := withFlow(trendSeries(2400, 21, 0.004))
	rewritten := &Series{
		Symbol:   base.Symbol,
		Interval: base.Interval,
		Candles:  append([]Candle(nil), base.Candles...),
	}
	// Replace the final third with a violent crash.
	cut := 1600
	price := rewritten.Candles[cut].Close
	for i := cut + 1; i < len(rewritten.Candles); i++ {
		open := price
		price = open * 0.97
		c := &rewritten.Candles[i]
		c.Open, c.Close = open, price
		c.High, c.Low = open*1.002, price*0.98
	}

	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 6, 2

	a, err := se.Run(base, BreakoutGrid())
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	b, err := se.Run(rewritten, BreakoutGrid())
	if err != nil {
		t.Fatalf("rewritten: %v", err)
	}

	compared := 0
	for i := range a.Folds {
		if i >= len(b.Folds) || a.Folds[i].ToBar > cut {
			break
		}
		if a.Folds[i].Chosen != b.Folds[i].Chosen {
			t.Fatalf("fold %d (bars %d-%d) chose %q originally and %q after the LATER data "+
				"was rewritten — the search is selecting with hindsight",
				i, a.Folds[i].FromBar, a.Folds[i].ToBar, a.Folds[i].Chosen, b.Folds[i].Chosen)
		}
		compared++
	}
	if compared == 0 {
		t.Fatal("no folds fell entirely before the cut; the test proves nothing")
	}
}

func TestSearch_RejectsAnImpossibleConfiguration(t *testing.T) {
	s := randomWalkSeries(1200, 7, 0.01)
	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 3, 3 // no fold left out of sample
	if _, err := se.Run(s, BreakoutGrid()); err == nil {
		t.Fatal("expected an error when no fold remains out of sample")
	}
	if _, err := NewSearcher().Run(s, nil); err == nil {
		t.Fatal("expected an error for an empty variant space")
	}
}

func TestSearchAcross_RequiresMoreThanOneMarket(t *testing.T) {
	// Four independent random walks: the same method should not survive all
	// of them, and the verdict should say so rather than celebrating whichever
	// one happened to work.
	var set []*Series
	for i := 0; i < 4; i++ {
		s := withFlow(randomWalkSeries(2000, int64(500+i), 0.012))
		s.Symbol = "NOISE" + string(rune('A'+i))
		set = append(set, s)
	}

	se := NewSearcher()
	se.Folds, se.MinTrainFolds = 5, 2
	res, err := se.SearchAcross(set, BreakoutGrid())
	if err != nil {
		t.Fatalf("cross-market: %v", err)
	}
	if res.Total != 4 {
		t.Fatalf("searched %d markets, want 4", res.Total)
	}
	if res.Positive == 4 {
		t.Fatalf("the method was profitable on all four independent random walks, which "+
			"should be very unlikely: %s", res.Report())
	}
	t.Log("\n" + res.Report())
}
