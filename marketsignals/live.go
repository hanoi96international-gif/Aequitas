package marketsignals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Running live.
//
// The gap between a backtest and a live runner is smaller than it looks and
// more dangerous than it looks, because almost all of it sits in one rule:
// THE BAR MUST BE CLOSED.
//
// A REST poll of any exchange's klines returns the bar currently forming as
// its last element. An agent handed that bar sees a high, low and close that
// are still changing, so its signal changes with them — flipping long and
// short several times inside a single bar, entering on a spike that retraces
// before the bar even ends. Every property the backtest established is void,
// because the backtest only ever evaluated finished bars. The failure is also
// invisible: the code runs, the signals look plausible, and the losses arrive
// as "the strategy stopped working" rather than as an error.
//
// So the runner refuses any bar whose closing instant has not passed, and it
// refuses to run at all across a gap in the series.

// LiveSignal is one decision, emitted after a bar closes.
type LiveSignal struct {
	Time    time.Time `json:"time"`
	Symbol  string    `json:"symbol"`
	BarTime time.Time `json:"bar_time"`
	Close   float64   `json:"close"`

	Direction string   `json:"direction"`
	Strength  float64  `json:"strength"`
	Target    float64  `json:"target_fraction_of_equity"`
	Reason    string   `json:"reason"`
	Members   []Signal `json:"members,omitempty"`

	// RiskScale is what the modulators did to the position, and RiskReason
	// why. Kept separate from Reason because "we are flat because nobody has
	// a view" and "we are flat because an election lands in two hours" are
	// different states that look identical in a position report.
	RiskScale  float64 `json:"risk_scale"`
	RiskReason string  `json:"risk_reason,omitempty"`
}

// Feed supplies recent CLOSED bars, oldest first.
//
// An implementation MUST NOT return the bar currently forming. The runner
// checks anyway — belt and braces on the one rule that matters — but a feed
// that leaks a forming bar will have its newest bar silently ignored until it
// closes, which looks like a stalled feed. Better to drop it at the source.
type Feed interface {
	Poll(ctx context.Context) ([]Candle, error)
}

// Sink receives emitted signals.
type Sink interface {
	Emit(LiveSignal) error
}

// LiveRunner maintains a rolling series and evaluates it on each bar close.
type LiveRunner struct {
	Symbol   string
	Interval time.Duration
	Strategy Strategy
	Risk     *RiskManager

	// MaxBars caps the rolling window. Zero keeps everything, which grows
	// without bound in a long-running process.
	MaxBars int

	// Drawdown reports the CURRENT drawdown of the real account, for the risk
	// manager's kill switch.
	//
	// It must be supplied. In a backtest the kill switch reads a simulated
	// equity curve; live, that curve does not exist, and defaulting to zero
	// would leave the switch permanently un-tripped — silently removing the
	// one control that exists to stop a strategy that has started losing. A
	// nil Drawdown is therefore treated as unknown, and the runner reports it
	// rather than assuming health.
	Drawdown func() float64

	// Now is injectable for tests.
	Now func() time.Time

	mu      sync.Mutex
	series  *Series
	lastBar time.Time
	healthy bool
	problem string
}

// NewLiveRunner returns a runner for a strategy on one instrument.
func NewLiveRunner(symbol string, interval time.Duration, strat Strategy) *LiveRunner {
	return &LiveRunner{
		Symbol:   symbol,
		Interval: interval,
		Strategy: strat,
		Risk:     NewRiskManager(),
		MaxBars:  5000,
		Now:      time.Now,
		series:   &Series{Symbol: symbol, Interval: interval},
	}
}

// Seed loads historical bars so the agents are not in warmup for days after a
// restart. The history must be contiguous and already closed.
func (r *LiveRunner) Seed(history *Series) error {
	if err := history.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.series = &Series{
		Symbol:   r.Symbol,
		Interval: r.Interval,
		Candles:  append([]Candle(nil), history.Candles...),
		Funding:  append([]float64(nil), history.Funding...),
	}
	r.lastBar = r.series.Candles[len(r.series.Candles)-1].Time
	r.healthy = true
	r.problem = ""
	r.trimLocked()
	return nil
}

// Warmup is how many bars the strategy needs before it can speak.
func (r *LiveRunner) Warmup() int { return maxInt(r.Strategy.Warmup(), r.Risk.VolLookback+2) }

// Health reports whether the runner is in a state where its signals mean
// anything, and why not when they do not.
func (r *LiveRunner) Health() (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.healthy, r.problem
}

// Bars is how many bars are currently held.
func (r *LiveRunner) Bars() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.series.Candles)
}

// OnBars ingests candidate bars and returns a signal for each one that
// genuinely closed. It is pure with respect to the clock (via Now) and does
// no I/O, which is what makes the live path testable.
//
// Bars are rejected, silently and deliberately, when they are:
//
//   - still forming (closing instant not yet passed),
//   - already known, or older than the newest bar held,
//   - separated from the newest bar by a gap.
//
// The gap case is the one worth dwelling on. A dropped connection leaves a
// hole, and every indicator here reads a window of consecutive bars — an ATR
// or a channel computed across a hole is a number about a market that did not
// happen. The runner marks itself unhealthy and emits nothing until it is
// re-seeded with the missing history. Continuing with a hole is the failure
// that produces confident, wrong signals.
func (r *LiveRunner) OnBars(bars []Candle) ([]LiveSignal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.Now()
	var out []LiveSignal

	for _, c := range bars {
		// Rule one: the bar must have finished.
		if !now.After(c.Time.Add(r.Interval)) && !now.Equal(c.Time.Add(r.Interval)) {
			continue
		}
		// Nothing new.
		if !r.lastBar.IsZero() && !c.Time.After(r.lastBar) {
			continue
		}
		// A hole between the last bar held and this one.
		if !r.lastBar.IsZero() {
			expected := r.lastBar.Add(r.Interval)
			if c.Time.After(expected) {
				missing := int(c.Time.Sub(expected)/r.Interval) + 1
				r.healthy = false
				r.problem = fmt.Sprintf(
					"gap of %d bar(s) between %s and %s — indicators computed across a hole "+
						"describe a market that did not happen; re-seed before trusting anything",
					missing, r.lastBar.Format(time.RFC3339), c.Time.Format(time.RFC3339))
				return out, nil
			}
		}

		r.series.Candles = append(r.series.Candles, c)
		r.lastBar = c.Time
		r.trimLocked()

		if !r.healthy {
			continue // a gap is outstanding; nothing here is trustworthy yet
		}
		if len(r.series.Candles) < r.Warmup() {
			continue
		}
		out = append(out, r.evaluateLocked(c, now))
	}
	return out, nil
}

func (r *LiveRunner) evaluateLocked(bar Candle, now time.Time) LiveSignal {
	v := NewView(r.series, len(r.series.Candles))

	dd, ddKnown := 0.0, false
	if r.Drawdown != nil {
		dd, ddKnown = r.Drawdown(), true
	}

	sig := r.Strategy.Decide(v)
	sized := r.Risk.Size(sig, v, dd)

	scale, riskReason := 1.0, ""
	if m, ok := r.Strategy.(RiskModulator); ok {
		adj := m.ModulateRisk(v)
		scale = clamp(adj.Scale, 0, 1)
		if adj.Veto {
			scale = 0
		}
		if scale < 1 {
			sized.Target *= scale
			riskReason = adj.Source + ": " + adj.Reason
		}
	}

	reason := sized.Reason
	if !ddKnown {
		reason += " | WARNING: no account drawdown wired in, so the kill switch cannot trip"
	}

	out := LiveSignal{
		Time: now, Symbol: r.Symbol, BarTime: bar.Time, Close: bar.Close,
		Direction: sig.Dir.String(), Strength: sig.Strength,
		Target: sized.Target, Reason: reason,
		RiskScale: scale, RiskReason: riskReason,
	}
	if e, ok := r.Strategy.(*Ensemble); ok {
		out.Members = e.Evaluate(v).Members
	}
	return out
}

func (r *LiveRunner) trimLocked() {
	if r.MaxBars <= 0 || len(r.series.Candles) <= r.MaxBars {
		return
	}
	drop := len(r.series.Candles) - r.MaxBars
	r.series.Candles = append([]Candle(nil), r.series.Candles[drop:]...)
	if len(r.series.Funding) >= drop {
		r.series.Funding = append([]float64(nil), r.series.Funding[drop:]...)
	} else {
		r.series.Funding = nil
	}
}

// Run polls the feed until the context is cancelled, emitting to the sink.
//
// A heartbeat is deliberate: a runner receiving nothing looks exactly like a
// quiet market from the outside, and "the feed died three hours ago" should
// not be something you discover from a position report.
func (r *LiveRunner) Run(ctx context.Context, feed Feed, sink Sink, every time.Duration, heartbeat func(string)) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	lastSignal := r.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		bars, err := feed.Poll(ctx)
		if err != nil {
			if heartbeat != nil {
				heartbeat("feed error: " + err.Error())
			}
			continue
		}

		signals, err := r.OnBars(bars)
		if err != nil {
			return err
		}
		if ok, problem := r.Health(); !ok {
			if heartbeat != nil {
				heartbeat("UNHEALTHY: " + problem)
			}
			continue
		}

		for _, s := range signals {
			if err := sink.Emit(s); err != nil {
				return fmt.Errorf("sink: %w", err)
			}
			lastSignal = r.Now()
		}
		if heartbeat != nil && r.Now().Sub(lastSignal) > 3*r.Interval {
			heartbeat(fmt.Sprintf("no closed bar in %s — feed may be stalled",
				r.Now().Sub(lastSignal).Round(time.Second)))
		}
	}
}

// JSONLSink writes one JSON object per line — appendable, greppable, and
// trivially replayable against the backtester to check that the live path
// produced what the historical one would have.
type JSONLSink struct {
	W  io.Writer
	mu sync.Mutex
}

// Emit writes the signal as a JSON line.
func (s *JSONLSink) Emit(sig LiveSignal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(sig)
	if err != nil {
		return err
	}
	_, err = s.W.Write(append(b, '\n'))
	return err
}

// FuncSink adapts a function into a Sink.
type FuncSink func(LiveSignal) error

// Emit calls the function.
func (f FuncSink) Emit(s LiveSignal) error { return f(s) }
