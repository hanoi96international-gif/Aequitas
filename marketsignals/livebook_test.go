package marketsignals

import (
	"strings"
	"testing"
	"time"
)

// bookFixture returns a live book seeded with aligned history for n names, and
// the instant its last seeded bar closed.
func bookFixture(t *testing.T, n, bars int) (*LiveBook, []string, time.Time) {
	t.Helper()
	u := correlatedUniverse(n, bars, 0.3, 1234)
	syms := u.Symbols()

	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, sym := range syms {
		cs := u.Series[sym].Candles
		for i := range cs {
			cs[i].Time = end.Add(time.Duration(i-len(cs)+1) * time.Hour)
		}
	}

	alloc := NewCrossSectional()
	alloc.MinNames = 2
	b := NewLiveBook(u.Instruments, time.Hour, alloc)
	now := end.Add(time.Hour)
	b.Now = func() time.Time { return now }

	for _, sym := range syms {
		if err := b.Seed(sym, u.Series[sym]); err != nil {
			t.Fatalf("seed %s: %v", sym, err)
		}
	}
	return b, syms, now
}

func bookBar(at time.Time, close float64) Candle {
	return Candle{
		Time: at, Open: close * 0.999, High: close * 1.002, Low: close * 0.997,
		Close: close, Volume: 1000,
	}
}

// TestLiveBook_WillNotAllocateOnAPartialUniverse is the property the whole
// file exists for. An allocator ranks names against each other; ranking one
// asset's finished hour against another's unfinished one produces weights that
// are all wrong, not just the late name's — and the output looks entirely
// normal.
func TestLiveBook_WillNotAllocateOnAPartialUniverse(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	next := now
	b.Now = func() time.Time { return next.Add(2 * time.Hour) }

	// Only the first name reports its new bar.
	sig, err := b.OnBars(syms[0], []Candle{bookBar(next, 120)})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if sig != nil {
		t.Fatal("the book allocated with five of six names still on the previous bar")
	}
	_, problem := b.Health()
	if !strings.Contains(problem, "waiting for") {
		t.Fatalf("status %q should say which names it is waiting for", problem)
	}

	// The rest arrive; the last one completes the set and triggers one
	// allocation.
	for _, sym := range syms[1 : len(syms)-1] {
		if sig, _ = b.OnBars(sym, []Candle{bookBar(next, 120)}); sig != nil {
			t.Fatalf("%s completed the set early", sym)
		}
	}
	sig, err = b.OnBars(syms[len(syms)-1], []Candle{bookBar(next, 120)})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if sig == nil {
		t.Fatal("the book did not allocate once every name shared a bar")
	}
	if !sig.BarTime.Equal(next) {
		t.Fatalf("allocated for bar %s, want %s", sig.BarTime, next)
	}
	if len(sig.Weights) == 0 {
		t.Fatalf("empty book: %s", sig.Reason)
	}
}

func TestLiveBook_AllocatesOncePerBar(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	next := now
	b.Now = func() time.Time { return next.Add(2 * time.Hour) }

	emitted := 0
	for _, sym := range syms {
		if sig, _ := b.OnBars(sym, []Candle{bookBar(next, 120)}); sig != nil {
			emitted++
		}
	}
	// A late duplicate poll must not produce a second allocation for the same
	// bar; a book that re-allocates on repeated polls churns turnover for
	// nothing.
	for _, sym := range syms {
		if sig, _ := b.OnBars(sym, []Candle{bookBar(next, 120)}); sig != nil {
			emitted++
		}
	}
	if emitted != 1 {
		t.Fatalf("emitted %d allocations for one bar, want 1", emitted)
	}
}

// TestLiveBook_NamesAStaleFeed. A book frozen because one feed died looks
// exactly like a book with no signal, and the two call for opposite responses.
func TestLiveBook_NamesAStaleFeed(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	b.StaleAfter = 2

	// Five names advance five hours; one never reports.
	for h := 1; h <= 5; h++ {
		at := now.Add(time.Duration(h-1) * time.Hour)
		b.Now = func() time.Time { return at.Add(2 * time.Hour) }
		for _, sym := range syms[:len(syms)-1] {
			if _, err := b.OnBars(sym, []Candle{bookBar(at, 120)}); err != nil {
				t.Fatalf("OnBars: %v", err)
			}
		}
	}

	_, problem := b.Health()
	if !strings.Contains(problem, "STALE") {
		t.Fatalf("status %q should flag the silent feed rather than waiting quietly", problem)
	}
	if !strings.Contains(problem, syms[len(syms)-1]) {
		t.Fatalf("status %q should name the lagging symbol", problem)
	}
	if !strings.Contains(problem, "broken feed") {
		t.Fatalf("status %q should distinguish a dead feed from an absence of opportunity",
			problem)
	}
}

// TestLiveBook_AGapInOneNamePoisonsTheWholeBook. A covariance estimated across
// a hole in one series is wrong for every weight it produces.
func TestLiveBook_AGapInOneNamePoisonsTheWholeBook(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	b.Now = func() time.Time { return now.Add(10 * time.Hour) }

	// One name skips three hours.
	sig, err := b.OnBars(syms[0], []Candle{bookBar(now.Add(3*time.Hour), 120)})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if sig != nil {
		t.Fatal("allocated across a gap")
	}
	healthy, problem := b.Health()
	if healthy {
		t.Fatal("the book still reports healthy after a gap in one of its names")
	}
	if !strings.Contains(problem, "every weight in the book") {
		t.Fatalf("problem %q should say the damage is not confined to that name", problem)
	}

	// And it stays silent for the other names too.
	for _, sym := range syms[1:] {
		if s, _ := b.OnBars(sym, []Candle{bookBar(now, 120)}); s != nil {
			t.Fatalf("%s produced an allocation while a gap was outstanding", sym)
		}
	}
}

func TestLiveBook_RefusesAFormingBar(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	before := b.Bars(syms[0])

	// This bar opened at `now` and closes an hour later.
	b.Now = func() time.Time { return now }
	if _, err := b.OnBars(syms[0], []Candle{bookBar(now, 120)}); err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if got := b.Bars(syms[0]); got != before {
		t.Fatalf("a forming bar was appended (%d → %d)", before, got)
	}
}

func TestLiveBook_RejectsASymbolOutsideItsUniverse(t *testing.T) {
	b, _, now := bookFixture(t, 6, 700)
	if _, err := b.OnBars("NOTHERE", []Candle{bookBar(now, 1)}); err == nil {
		t.Fatal("expected an error for a symbol the book does not hold")
	}
	if err := b.Seed("NOTHERE", trendSeries(100, 1, 0.01)); err == nil {
		t.Fatal("expected an error seeding a symbol the book does not hold")
	}
}

func TestLiveBook_TrimsEachSymbolsWindow(t *testing.T) {
	b, syms, now := bookFixture(t, 6, 700)
	b.MaxBars = 300
	next := now
	b.Now = func() time.Time { return next.Add(2 * time.Hour) }
	for _, sym := range syms {
		if _, err := b.OnBars(sym, []Candle{bookBar(next, 120)}); err != nil {
			t.Fatalf("OnBars: %v", err)
		}
	}
	for _, sym := range syms {
		if got := b.Bars(sym); got != 300 {
			t.Fatalf("%s holds %d bars, want the 300-bar cap", sym, got)
		}
	}
}

func TestFormatBook_LeadsWithTheBookShape(t *testing.T) {
	got := FormatBook(BookSignal{
		BarTime: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		Weights: map[string]float64{"BTCUSDT": 0.25, "ETHUSDT": -0.15, "SOLUSDT": 0},
		Prices:  map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3400, "SOLUSDT": 150},
		Gross:   0.40, Net: 0.10, DiversificationRatio: 1.9,
		Reason: "3 long / 3 short of 10",
	})

	first := strings.SplitN(got, "\n", 2)[0]
	for _, want := range []string{"gross", "net", "diversification"} {
		if !strings.Contains(first, want) {
			t.Fatalf("first line %q should carry the book's shape", first)
		}
	}
	if !strings.Contains(got, "BTCUSDT") || !strings.Contains(got, "ETHUSDT") {
		t.Fatalf("held names missing from the message:\n%s", got)
	}
	if strings.Contains(got, "SOLUSDT") {
		t.Fatalf("a zero weight was listed; that is noise in a phone message:\n%s", got)
	}
	if !strings.Contains(got, "not an order") {
		t.Fatalf("message must say it is not an order:\n%s", got)
	}
}

// TestLiveBook_MatchesTheBacktestOnTheSameBars is the equivalence that makes
// the portfolio backtest's evidence transferable to the live path.
func TestLiveBook_MatchesTheBacktestOnTheSameBars(t *testing.T) {
	u := correlatedUniverse(8, 800, 0.3, 4321)
	syms := u.Symbols()
	const split = 700

	alloc := NewCrossSectional()
	alloc.MinNames = 2

	// What the allocator would produce over the first `split` bars.
	want, err := alloc.Allocate(u, split)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	b := NewLiveBook(u.Instruments, time.Hour, alloc)
	last := u.Series[syms[0]].Candles[split-1].Time
	b.Now = func() time.Time { return last.Add(3 * time.Hour) }

	for _, sym := range syms {
		hist := &Series{Symbol: sym, Interval: time.Hour,
			Candles: append([]Candle(nil), u.Series[sym].Candles[:split-1]...)}
		if err := b.Seed(sym, hist); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var got *BookSignal
	for _, sym := range syms {
		s, err := b.OnBars(sym, []Candle{u.Series[sym].Candles[split-1]})
		if err != nil {
			t.Fatalf("OnBars: %v", err)
		}
		if s != nil {
			got = s
		}
	}
	if got == nil {
		t.Fatal("the live book produced no allocation")
	}
	for sym, w := range want.Weights {
		if diff := got.Weights[sym] - w; diff > 1e-12 || diff < -1e-12 {
			t.Fatalf("live weighted %s at %.9f where the backtester weighted it %.9f — "+
				"without this equivalence every portfolio number describes a different system",
				sym, got.Weights[sym], w)
		}
	}
}
