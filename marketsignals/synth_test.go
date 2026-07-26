package marketsignals

import (
	"math"
	"math/rand"
	"time"
)

// Deterministic synthetic series, shared by the tests. Every generator is
// seeded explicitly so that a failing test fails the same way twice — a
// randomised backtest test that passes four times out of five is worse than
// no test, because it teaches you to re-run instead of to look.

func mkCandle(t time.Time, open, close, wick float64) Candle {
	high := math.Max(open, close) + wick
	low := math.Min(open, close) - wick
	return Candle{Time: t, Open: open, High: high, Low: low, Close: close, Volume: 1000}
}

// randomWalkSeries has no edge in it by construction: log returns are iid
// normal. Any strategy that appears profitable here is measuring noise, which
// is exactly what several tests below assert.
func randomWalkSeries(n int, seed int64, vol float64) *Series {
	rng := rand.New(rand.NewSource(seed))
	s := &Series{Symbol: "NOISE", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	price := 100.0
	for i := 0; i < n; i++ {
		open := price
		price = open * math.Exp(rng.NormFloat64()*vol)
		s.Candles = append(s.Candles, mkCandle(t, open, price, math.Abs(rng.NormFloat64())*vol*open*0.5))
		t = t.Add(time.Hour)
	}
	return s
}

// trendSeries drifts steadily upward with light noise — the regime a breakout
// agent is supposed to catch.
func trendSeries(n int, seed int64, drift float64) *Series {
	rng := rand.New(rand.NewSource(seed))
	s := &Series{Symbol: "TREND", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	price := 100.0
	for i := 0; i < n; i++ {
		open := price
		price = open * math.Exp(drift+rng.NormFloat64()*drift*0.3)
		s.Candles = append(s.Candles, mkCandle(t, open, price, open*drift*0.5))
		t = t.Add(time.Hour)
	}
	return s
}

// flatSeries oscillates in a tight band — the regime in which a reversion
// agent is allowed to speak and a breakout agent must stay quiet.
func flatSeries(n int, amplitude float64) *Series {
	s := &Series{Symbol: "FLAT", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < n; i++ {
		open := 100 + amplitude*math.Sin(float64(i)/7)
		close := 100 + amplitude*math.Sin(float64(i+1)/7)
		s.Candles = append(s.Candles, mkCandle(t, open, close, amplitude*0.2))
		t = t.Add(time.Hour)
	}
	return s
}

// withFlow attaches a taker split to every bar, splitting volume in the
// direction the bar closed.
func withFlow(s *Series) *Series {
	for i := range s.Candles {
		c := &s.Candles[i]
		if c.Close >= c.Open {
			c.BuyVolume, c.SellVolume = c.Volume*0.6, c.Volume*0.4
		} else {
			c.BuyVolume, c.SellVolume = c.Volume*0.4, c.Volume*0.6
		}
	}
	return s
}

// appendCandle grows a series by one bar built from explicit OHLC.
func appendCandle(s *Series, open, high, low, close, buyVol, sellVol float64) {
	last := s.Candles[len(s.Candles)-1]
	s.Candles = append(s.Candles, Candle{
		Time:       last.Time.Add(s.Interval),
		Open:       open,
		High:       high,
		Low:        low,
		Close:      close,
		Volume:     buyVol + sellVol,
		BuyVolume:  buyVol,
		SellVolume: sellVol,
	})
}
