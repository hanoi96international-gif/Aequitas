package marketsignals

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Running a book live.
//
// A single instrument can act the moment its bar closes. A book cannot, and
// the difference is the whole design problem here.
//
// An allocator RANKS names against each other, or weighs their experts against
// each other, and both operations are meaningless unless every name is being
// measured at the same instant. If BTC's hourly bar has closed and SOL's has
// not yet arrived, ranking them compares one asset's finished hour against
// another's unfinished one — and the output looks entirely normal. Every
// weight in the book is then wrong, not just the late name's.
//
// So the book allocates only when every symbol shares a newest closed bar.
// Until then it holds its previous targets and says why. A symbol that falls
// permanently behind is reported rather than waited on for ever: a book frozen
// because one feed died looks exactly like a book with no signal.

// BookSignal is one allocation decision, emitted when the universe aligns.
type BookSignal struct {
	Time    time.Time          `json:"time"`
	BarTime time.Time          `json:"bar_time"`
	Weights map[string]float64 `json:"weights"`
	Prices  map[string]float64 `json:"prices"`

	Gross                float64 `json:"gross"`
	Net                  float64 `json:"net"`
	DiversificationRatio float64 `json:"diversification_ratio"`
	Reason               string  `json:"reason"`
}

// BookSink receives book-level allocations.
type BookSink interface {
	EmitBook(BookSignal) error
}

// BookFuncSink adapts a function into a BookSink.
type BookFuncSink func(BookSignal) error

// EmitBook calls the function.
func (f BookFuncSink) EmitBook(s BookSignal) error { return f(s) }

// LiveBook maintains a rolling universe and allocates on each aligned close.
type LiveBook struct {
	Allocator Allocator
	Interval  time.Duration

	// MaxBars caps each symbol's rolling window.
	MaxBars int

	// StaleAfter is how far a symbol may lag the leader before the book
	// reports it as stale. Expressed in bars.
	StaleAfter int

	Now func() time.Time

	mu      sync.Mutex
	uni     *Universe
	lastBar map[string]time.Time
	emitted time.Time
	healthy bool
	problem string
}

// NewLiveBook returns a book over the given instruments.
func NewLiveBook(instruments []Instrument, interval time.Duration, a Allocator) *LiveBook {
	uni := &Universe{Series: map[string]*Series{}, Instruments: instruments}
	for _, inst := range instruments {
		uni.Series[inst.Symbol] = &Series{Symbol: inst.Symbol, Interval: interval}
	}
	return &LiveBook{
		Allocator: a, Interval: interval, MaxBars: 5000, StaleAfter: 3,
		Now: time.Now, uni: uni, lastBar: map[string]time.Time{}, healthy: true,
	}
}

// Seed loads historical bars for one symbol.
func (b *LiveBook) Seed(symbol string, history *Series) error {
	if err := history.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.uni.Series[symbol]
	if !ok {
		return fmt.Errorf("%s is not in this book's universe", symbol)
	}
	s.Candles = append([]Candle(nil), history.Candles...)
	s.Funding = append([]float64(nil), history.Funding...)
	b.lastBar[symbol] = s.Candles[len(s.Candles)-1].Time
	b.trimLocked(symbol)
	return nil
}

// Health reports whether the book's signals currently mean anything.
func (b *LiveBook) Health() (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.healthy, b.problem
}

// Bars reports how many bars are held for a symbol.
func (b *LiveBook) Bars(symbol string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.uni.Series[symbol]; ok {
		return len(s.Candles)
	}
	return 0
}

// OnBars ingests closed bars for one symbol and allocates if that completes an
// aligned set.
//
// Bars are rejected on the same terms as the single-instrument runner: still
// forming, already known, or separated by a gap. A gap in ANY symbol marks the
// whole book unhealthy, because a covariance estimated across a hole in one
// series is wrong for every weight it produces, not just that name's.
func (b *LiveBook) OnBars(symbol string, bars []Candle) (*BookSignal, error) {
	b.mu.Lock()

	s, ok := b.uni.Series[symbol]
	if !ok {
		b.mu.Unlock()
		return nil, fmt.Errorf("%s is not in this book's universe", symbol)
	}
	now := b.Now()

	for _, c := range bars {
		if now.Before(c.Time.Add(b.Interval)) {
			continue // still forming
		}
		if last, seen := b.lastBar[symbol]; seen && !c.Time.After(last) {
			continue // already known, or stale
		}
		if last, seen := b.lastBar[symbol]; seen {
			if expected := last.Add(b.Interval); c.Time.After(expected) {
				missing := int(c.Time.Sub(expected)/b.Interval) + 1
				b.healthy = false
				b.problem = fmt.Sprintf(
					"%s: gap of %d bar(s) — a covariance estimated across a hole in one series "+
						"is wrong for every weight in the book, not only that name's; re-seed",
					symbol, missing)
				b.mu.Unlock()
				return nil, nil
			}
		}
		s.Candles = append(s.Candles, c)
		b.lastBar[symbol] = c.Time
		b.trimLocked(symbol)
	}

	if !b.healthy {
		b.mu.Unlock()
		return nil, nil
	}

	aligned, at, problem := b.alignmentLocked()
	if !aligned {
		b.problem = problem
		b.mu.Unlock()
		return nil, nil
	}
	if !b.emitted.IsZero() && !at.After(b.emitted) {
		b.mu.Unlock()
		return nil, nil // already allocated for this bar
	}

	// The universe is complete as of `at`. Allocate on exactly the bars every
	// name shares — never on a partial set.
	n := len(s.Candles)
	for _, other := range b.uni.Series {
		if len(other.Candles) < n {
			n = len(other.Candles)
		}
	}
	uni, snapshotAt := b.uni, at
	b.emitted = at
	b.problem = ""
	b.mu.Unlock()

	alloc, err := b.Allocator.Allocate(uni, n)
	if err != nil {
		return nil, err
	}
	if len(alloc.Weights) == 0 {
		return nil, nil
	}

	prices := map[string]float64{}
	for sym, series := range uni.Series {
		prices[sym] = series.Candles[n-1].Close
	}

	return &BookSignal{
		Time: now, BarTime: snapshotAt,
		Weights: alloc.Weights, Prices: prices,
		Gross: alloc.Gross, Net: alloc.Net,
		DiversificationRatio: alloc.DiversificationRatio,
		Reason:               alloc.Reason,
	}, nil
}

// alignmentLocked reports whether every symbol has reached the same newest bar.
func (b *LiveBook) alignmentLocked() (bool, time.Time, string) {
	if len(b.lastBar) < len(b.uni.Series) {
		var waiting []string
		for sym := range b.uni.Series {
			if _, ok := b.lastBar[sym]; !ok {
				waiting = append(waiting, sym)
			}
		}
		return false, time.Time{}, fmt.Sprintf("no bars yet for %v", waiting)
	}

	var newest time.Time
	for _, t := range b.lastBar {
		if t.After(newest) {
			newest = t
		}
	}

	var behind []string
	stale := false
	for sym, t := range b.lastBar {
		if t.Equal(newest) {
			continue
		}
		behind = append(behind, sym)
		if newest.Sub(t) > time.Duration(b.StaleAfter)*b.Interval {
			stale = true
		}
	}
	if len(behind) == 0 {
		return true, newest, ""
	}
	if stale {
		// Named rather than waited on: a book frozen because one feed died
		// looks exactly like a book with no signal.
		return false, time.Time{}, fmt.Sprintf(
			"STALE: %v are more than %d bars behind — the book is not allocating, and that "+
				"is a broken feed rather than an absence of opportunity", behind, b.StaleAfter)
	}
	return false, time.Time{}, fmt.Sprintf("waiting for %v to close the same bar", behind)
}

func (b *LiveBook) trimLocked(symbol string) {
	s := b.uni.Series[symbol]
	if b.MaxBars <= 0 || len(s.Candles) <= b.MaxBars {
		return
	}
	drop := len(s.Candles) - b.MaxBars
	s.Candles = append([]Candle(nil), s.Candles[drop:]...)
	if len(s.Funding) >= drop {
		s.Funding = append([]float64(nil), s.Funding[drop:]...)
	} else {
		s.Funding = nil
	}
}

// Run polls one feed per symbol until the context is cancelled.
func (b *LiveBook) Run(ctx context.Context, feeds map[string]Feed, sink BookSink,
	every time.Duration, heartbeat func(string)) error {

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		for sym, feed := range feeds {
			bars, err := feed.Poll(ctx)
			if err != nil {
				if heartbeat != nil {
					heartbeat(fmt.Sprintf("%s feed error: %v", sym, err))
				}
				continue
			}
			sig, err := b.OnBars(sym, bars)
			if err != nil {
				return err
			}
			if sig != nil {
				if err := sink.EmitBook(*sig); err != nil {
					return fmt.Errorf("sink: %w", err)
				}
			}
		}

		if ok, problem := b.Health(); !ok && heartbeat != nil {
			heartbeat("UNHEALTHY: " + problem)
		} else if problem != "" && heartbeat != nil {
			heartbeat(problem)
		}
	}
}

// FormatBook renders a book allocation as a short human message.
func FormatBook(s BookSignal) string {
	out := fmt.Sprintf("BOOK %s — gross %s, net %s, diversification %s\n",
		s.BarTime.Format("2006-01-02 15:04 MST"), pct(s.Gross), pct(s.Net),
		f2(s.DiversificationRatio))

	syms := make([]string, 0, len(s.Weights))
	for sym := range s.Weights {
		syms = append(syms, sym)
	}
	sortStrings(syms)
	for _, sym := range syms {
		w := s.Weights[sym]
		if w == 0 {
			continue
		}
		out += fmt.Sprintf("  %-12s %+7s of equity  @ %.6g\n", sym, pct(w), s.Prices[sym])
	}
	out += "\n" + s.Reason + "\nThis is an allocation, not an order. Nothing has been traded."
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
