// Package marketsignals is a self-contained signal-research toolkit: a small
// set of specialised market agents, an ensemble that only speaks when they
// agree for uncorrelated reasons, a risk manager that sizes positions, and a
// backtester built to be hard to fool.
//
// It deliberately does NOT place orders. Everything here produces a target
// position and an evaluation of whether that target has ever been worth
// anything. Connecting it to a real venue is a separate, deliberate act that
// needs credentials this package never touches.
package marketsignals

import (
	"fmt"
	"time"
)

// Candle is one OHLCV bar. BuyVolume/SellVolume hold the taker-side split
// when the venue reports it (Binance aggTrades, Coinbase matches, most DEX
// indexers); both zero means "this feed did not tell us", which is a
// meaningful state — the order-flow agent stands down rather than inventing
// a split from the candle body, which is a well-known way to manufacture a
// signal out of nothing.
type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64

	BuyVolume  float64
	SellVolume float64
}

// HasFlow reports whether this bar carries a real taker split.
func (c Candle) HasFlow() bool { return c.BuyVolume > 0 || c.SellVolume > 0 }

// Range is the bar's full high-low extent.
func (c Candle) Range() float64 { return c.High - c.Low }

// LowerWick is the distance from the bar's low up to the bottom of its body:
// how far price was pushed down and then rejected within the bar.
func (c Candle) LowerWick() float64 {
	body := c.Open
	if c.Close < body {
		body = c.Close
	}
	return body - c.Low
}

// UpperWick mirrors LowerWick on the top side.
func (c Candle) UpperWick() float64 {
	body := c.Open
	if c.Close > body {
		body = c.Close
	}
	return c.High - body
}

// Series is one instrument's bar history, optionally carrying per-bar
// perpetual funding rates aligned index-for-index with Candles. Funding may
// be nil or shorter than Candles; the positioning agent checks before use.
type Series struct {
	Symbol   string
	Interval time.Duration
	Candles  []Candle

	// Funding is the funding rate in effect over each bar, as a decimal per
	// funding period (0.0001 = 1bp), aligned with Candles by index.
	Funding []float64
}

// Validate catches the input problems that silently corrupt a backtest:
// out-of-order or duplicate timestamps (which make "the next bar" meaningless),
// impossible OHLC relationships, and a Funding slice longer than the bars it
// claims to annotate.
func (s *Series) Validate() error {
	if len(s.Candles) == 0 {
		return fmt.Errorf("series %q: no candles", s.Symbol)
	}
	for i, c := range s.Candles {
		if c.High < c.Low {
			return fmt.Errorf("series %q bar %d: high %g < low %g", s.Symbol, i, c.High, c.Low)
		}
		if c.Open > c.High || c.Open < c.Low || c.Close > c.High || c.Close < c.Low {
			return fmt.Errorf("series %q bar %d: open/close outside high-low range", s.Symbol, i)
		}
		if c.Open <= 0 || c.Close <= 0 {
			return fmt.Errorf("series %q bar %d: non-positive price", s.Symbol, i)
		}
		if i > 0 && !c.Time.After(s.Candles[i-1].Time) {
			return fmt.Errorf("series %q bar %d: timestamp %s not after previous %s",
				s.Symbol, i, c.Time, s.Candles[i-1].Time)
		}
	}
	if len(s.Funding) > len(s.Candles) {
		return fmt.Errorf("series %q: %d funding points for %d candles",
			s.Symbol, len(s.Funding), len(s.Candles))
	}
	return nil
}

// BarsPerYear is how many bars of this series fit in a year — the
// annualisation factor for Sharpe and volatility targeting.
func (s *Series) BarsPerYear() float64 {
	if s.Interval <= 0 {
		return 365
	}
	return float64(365*24*time.Hour) / float64(s.Interval)
}

// View is the ONLY way an Agent sees the market, and it is the mechanism that
// makes lookahead structurally impossible rather than merely discouraged: a
// View constructed at bar n exposes bars [0, n) and offers no method that
// reaches past n. An agent cannot accidentally peek at the bar it is about to
// be filled on, because there is no expression in this API that names it.
type View struct {
	series *Series
	n      int
}

// NewView returns a view of the first n bars of s. Panics on an n outside
// [0, len(s.Candles)] because that is a programming error in the caller's
// loop bounds, not a runtime condition worth threading an error through.
func NewView(s *Series, n int) View {
	if n < 0 || n > len(s.Candles) {
		panic(fmt.Sprintf("marketsignals: view of %d bars over a %d-bar series", n, len(s.Candles)))
	}
	return View{series: s, n: n}
}

// Len is the number of bars visible — the last one being the bar that has
// just closed.
func (v View) Len() int { return v.n }

// Symbol identifies the instrument under view.
func (v View) Symbol() string { return v.series.Symbol }

// Interval is the bar duration.
func (v View) Interval() time.Duration { return v.series.Interval }

// BarsPerYear is the annualisation factor for this view's series.
func (v View) BarsPerYear() float64 { return v.series.BarsPerYear() }

// At returns the i-th visible bar. Panics for i outside [0, Len).
func (v View) At(i int) Candle {
	if i < 0 || i >= v.n {
		panic(fmt.Sprintf("marketsignals: bar %d outside the %d visible bars", i, v.n))
	}
	return v.series.Candles[i]
}

// Last is the bar that just closed — the newest information an agent has.
func (v View) Last() Candle { return v.At(v.n - 1) }

// Candles returns the visible bars. The returned slice is capped at n, so
// even a re-slice by the caller cannot reach a future bar.
func (v View) Candles() []Candle { return v.series.Candles[:v.n:v.n] }

// Closes, Highs, Lows and Volumes extract one field across the visible bars.
// They allocate; agents call them once per evaluation, which at backtest
// scale is immaterial next to the clarity of not threading strided accessors
// through every indicator.
func (v View) Closes() []float64  { return v.field(func(c Candle) float64 { return c.Close }) }
func (v View) Highs() []float64   { return v.field(func(c Candle) float64 { return c.High }) }
func (v View) Lows() []float64    { return v.field(func(c Candle) float64 { return c.Low }) }
func (v View) Volumes() []float64 { return v.field(func(c Candle) float64 { return c.Volume }) }

func (v View) field(pick func(Candle) float64) []float64 {
	out := make([]float64, v.n)
	for i := 0; i < v.n; i++ {
		out[i] = pick(v.series.Candles[i])
	}
	return out
}

// Funding returns the visible funding rates, or nil when the series carries
// none (or has not accumulated enough to cover the visible bars).
func (v View) Funding() []float64 {
	if len(v.series.Funding) < v.n {
		return nil
	}
	return v.series.Funding[:v.n:v.n]
}
