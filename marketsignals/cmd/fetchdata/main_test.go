package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

// These tests cover the seam between the two halves — what fetchdata writes
// and what the library reads. Nothing else in either program checks it, and a
// mismatch here costs whoever runs this an hour of downloading before
// anything says so.
//
// The network is deliberately not involved: what needs proving is the file
// format and the funding alignment, and both are pure functions of data
// already in hand.

func syntheticBars(n int, start time.Time) []bar {
	out := make([]bar, 0, n)
	price := 100.0
	for i := 0; i < n; i++ {
		open := price
		price = open * 1.001
		out = append(out, bar{
			Time: start.Add(time.Duration(i) * time.Hour),
			Open: open, High: price * 1.002, Low: open * 0.998, Close: price,
			Volume: 1000, TakerBuyVolume: 600,
		})
	}
	return out
}

func TestWriteCSV_RoundTripsThroughLoadCSV(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := syntheticBars(50, start)
	path := filepath.Join(t.TempDir(), "bars.csv")

	if err := writeCSV(path, bars, nil); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	s, err := ms.LoadCSV(path, "TEST", time.Hour)
	if err != nil {
		t.Fatalf("the library refused the file this tool writes: %v", err)
	}

	if len(s.Candles) != len(bars) {
		t.Fatalf("wrote %d bars, read back %d", len(bars), len(s.Candles))
	}
	if !s.Candles[0].Time.Equal(start) {
		t.Fatalf("first bar at %s, want %s", s.Candles[0].Time, start)
	}
	if got, want := s.Candles[0].Close, bars[0].Close; got != want {
		t.Fatalf("close round-tripped as %.10g, want %.10g", got, want)
	}

	// The taker split is the whole reason to prefer Binance klines over a
	// plain OHLCV source: without it the flow agent stands down.
	c := s.Candles[0]
	if !c.HasFlow() {
		t.Fatal("no taker split survived the round trip; the flow agent would stand down")
	}
	if c.BuyVolume != 600 || c.SellVolume != 400 {
		t.Fatalf("split is %v/%v, want 600/400 (volume less taker buy)", c.BuyVolume, c.SellVolume)
	}
}

// TestWriteCSV_FundingIsForwardFilledNotBackfilled is the lookahead guard for
// the data layer. Funding settles every eight hours; the rate in effect over a
// bar is the most recent settlement AT OR BEFORE it. Carrying a later
// settlement backwards would hand the positioning agent a number that did not
// exist yet — lookahead entering through the file format, where none of the
// library's structural defences can see it.
func TestWriteCSV_FundingIsForwardFilledNotBackfilled(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := syntheticBars(24, start)

	// Settlements at t+0h and t+8h with very different values.
	funding := map[int64]float64{
		start.UnixMilli():                     0.0001,
		start.Add(8 * time.Hour).UnixMilli():  0.0009,
		start.Add(16 * time.Hour).UnixMilli(): -0.0005,
	}

	path := filepath.Join(t.TempDir(), "perp.csv")
	if err := writeCSV(path, bars, funding); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	s, err := ms.LoadCSV(path, "TEST", time.Hour)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Funding) != len(s.Candles) {
		t.Fatalf("%d funding points for %d bars", len(s.Funding), len(s.Candles))
	}

	for i, c := range s.Candles {
		want := 0.0001
		switch {
		case !c.Time.Before(start.Add(16 * time.Hour)):
			want = -0.0005
		case !c.Time.Before(start.Add(8 * time.Hour)):
			want = 0.0009
		}
		if s.Funding[i] != want {
			t.Fatalf("bar %d at %s carries funding %v, want %v — a bar must show the "+
				"settlement already in force, never a later one",
				i, c.Time.Format(time.RFC3339), s.Funding[i], want)
		}
	}
}

// TestWriteCSV_DropsBarsBeforeTheFirstSettlement: writing zero for a bar with
// no funding yet would read as "funding is neutral" when the truth is that it
// is unknown, and the positioning agent treats those very differently.
func TestWriteCSV_DropsBarsBeforeTheFirstSettlement(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := syntheticBars(12, start)
	funding := map[int64]float64{
		start.Add(5 * time.Hour).UnixMilli(): 0.0003,
	}

	path := filepath.Join(t.TempDir(), "perp.csv")
	if err := writeCSV(path, bars, funding); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	s, err := ms.LoadCSV(path, "TEST", time.Hour)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Candles) != 7 {
		t.Fatalf("kept %d bars, want the 7 at or after the first settlement", len(s.Candles))
	}
	if !s.Candles[0].Time.Equal(start.Add(5 * time.Hour)) {
		t.Fatalf("first kept bar is %s, want the settlement hour", s.Candles[0].Time)
	}
	for i, f := range s.Funding {
		if f != 0.0003 {
			t.Fatalf("bar %d carries %v, want the only settlement 0.0003", i, f)
		}
	}
}

func TestWriteCSV_SurvivesTheLibrarysValidation(t *testing.T) {
	// LoadCSV validates OHLC relationships and strict time ordering. A
	// formatting slip that produced, say, a high below a close would be caught
	// here rather than after a long download.
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "bars.csv")
	if err := writeCSV(path, syntheticBars(300, start), nil); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	s, err := ms.LoadCSV(path, "TEST", time.Hour)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// And the file must be long enough to actually search, which is the point
	// of fetching it.
	if _, err := ms.NewBacktester().Run(s, ms.AgentStrategy{Agent: ms.NewBreakoutAgent()}); err != nil {
		t.Fatalf("a 300-bar file could not be backtested: %v", err)
	}
}

func TestWriteCSV_RefusesAnUnwritablePath(t *testing.T) {
	if err := writeCSV(filepath.Join(t.TempDir(), "no", "such", "dir", "x.csv"),
		syntheticBars(3, time.Now()), nil); err == nil {
		t.Fatal("expected an error for an unwritable path")
	}
}

func TestAsFloatAndAsInt_HandleBinancesMixedTypes(t *testing.T) {
	// Binance returns times as JSON numbers and prices as JSON strings in the
	// same array, which is why these helpers exist.
	f, err := asFloat([]byte(`"12345.6789"`))
	if err != nil || f != 12345.6789 {
		t.Fatalf("asFloat on a quoted number = %v, %v", f, err)
	}
	if f, err := asFloat([]byte(`12345.6789`)); err != nil || f != 12345.6789 {
		t.Fatalf("asFloat on a bare number = %v, %v", f, err)
	}
	i, err := asInt([]byte(`1704067200000`))
	if err != nil || i != 1704067200000 {
		t.Fatalf("asInt on a bare number = %v, %v", i, err)
	}
	if i, err := asInt([]byte(`"1704067200000"`)); err != nil || i != 1704067200000 {
		t.Fatalf("asInt on a quoted number = %v, %v", i, err)
	}
	if _, err := asFloat([]byte(`"not a number"`)); err == nil {
		t.Fatal("expected an error rather than a silent zero price")
	}
}

func TestMain_UsageDoesNotPanic(t *testing.T) {
	// Cheap guard: the usage text is the first thing anyone sees.
	old := os.Stderr
	defer func() { os.Stderr = old }()
	f, _ := os.CreateTemp(t.TempDir(), "out")
	os.Stderr = f
	usage()
}
