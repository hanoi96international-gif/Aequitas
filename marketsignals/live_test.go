package marketsignals

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// liveFixture builds a runner already seeded with enough history to be past
// warmup, plus the clock it is standing at.
func liveFixture(t *testing.T) (*LiveRunner, time.Time) {
	t.Helper()
	hist := withFlow(trendSeries(400, 3, 0.004))
	// Re-time the history so it ends at a known instant.
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := range hist.Candles {
		hist.Candles[i].Time = end.Add(time.Duration(i-len(hist.Candles)+1) * time.Hour)
	}

	r := NewLiveRunner("TEST", time.Hour, AgentStrategy{Agent: NewBreakoutAgent()})
	now := end.Add(time.Hour) // the last seeded bar has just closed
	r.Now = func() time.Time { return now }
	if err := r.Seed(hist); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return r, now
}

func nextBar(after time.Time, close float64) Candle {
	return Candle{
		Time: after, Open: close * 0.999, High: close * 1.002, Low: close * 0.997,
		Close: close, Volume: 1000, BuyVolume: 600, SellVolume: 400,
	}
}

// TestLive_RefusesAFormingBar is the single most important test on the live
// path. A REST poll returns the bar currently forming as its last element; an
// agent handed that bar sees a close that is still moving, and its signal
// moves with it — flipping sides inside one bar and entering on spikes that
// retrace before the bar even ends. Every property the backtest established
// is void, and nothing errors.
func TestLive_RefusesAFormingBar(t *testing.T) {
	r, now := liveFixture(t)
	before := r.Bars()

	// This bar opened at `now` and closes an hour later. It is still forming.
	forming := nextBar(now, 150)

	got, err := r.OnBars([]Candle{forming})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("emitted %d signal(s) from a bar that has not closed: %+v", len(got), got)
	}
	if r.Bars() != before {
		t.Fatalf("a forming bar was appended to the series (%d → %d)", before, r.Bars())
	}

	// Once its closing instant passes, the very same bar is accepted.
	r.Now = func() time.Time { return now.Add(time.Hour) }
	got, err = r.OnBars([]Candle{forming})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("emitted %d signals for one closed bar", len(got))
	}
	if !got[0].BarTime.Equal(forming.Time) {
		t.Fatalf("signal is for bar %s, want %s", got[0].BarTime, forming.Time)
	}
}

// TestLive_RefusesToRunAcrossAGap: a dropped connection leaves a hole, and
// every indicator here reads consecutive bars. An ATR computed across a hole
// is a number about a market that did not happen.
func TestLive_RefusesToRunAcrossAGap(t *testing.T) {
	r, now := liveFixture(t)

	// Skip three hours, then deliver a bar as though nothing happened.
	gapped := nextBar(now.Add(3*time.Hour), 150)
	r.Now = func() time.Time { return now.Add(5 * time.Hour) }

	got, err := r.OnBars([]Candle{gapped})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("emitted %d signal(s) across a gap", len(got))
	}
	healthy, problem := r.Health()
	if healthy {
		t.Fatal("runner still reports healthy after a gap")
	}
	if !strings.Contains(problem, "gap") || !strings.Contains(problem, "re-seed") {
		t.Fatalf("problem %q should name the gap and say what fixes it", problem)
	}

	// And it stays silent until re-seeded, rather than quietly resuming.
	r.Now = func() time.Time { return now.Add(9 * time.Hour) }
	got, _ = r.OnBars([]Candle{nextBar(now.Add(4*time.Hour), 151)})
	if len(got) != 0 {
		t.Fatal("resumed signalling while a gap was still outstanding")
	}
}

func TestLive_IgnoresRepeatedAndStaleBars(t *testing.T) {
	r, now := liveFixture(t)
	r.Now = func() time.Time { return now.Add(2 * time.Hour) }

	fresh := nextBar(now, 150)
	got, _ := r.OnBars([]Candle{fresh})
	if len(got) != 1 {
		t.Fatalf("first delivery produced %d signals, want 1", len(got))
	}
	bars := r.Bars()

	// The same bar again — a poll overlapping the previous one.
	got, _ = r.OnBars([]Candle{fresh})
	if len(got) != 0 {
		t.Fatalf("re-delivering the same bar produced %d signal(s)", len(got))
	}
	// And an older one.
	got, _ = r.OnBars([]Candle{nextBar(now.Add(-5*time.Hour), 140)})
	if len(got) != 0 {
		t.Fatalf("a stale bar produced %d signal(s)", len(got))
	}
	if r.Bars() != bars {
		t.Fatalf("duplicate or stale bars changed the series length (%d → %d)", bars, r.Bars())
	}
}

// TestLive_MatchesTheBacktestOnTheSameBars is the equivalence that makes the
// backtest's evidence transferable. If the live path decided differently from
// the historical one on identical data, every number the search produced would
// describe a system other than the one running.
func TestLive_MatchesTheBacktestOnTheSameBars(t *testing.T) {
	full := withFlow(trendSeries(500, 11, 0.004))
	split := 420

	// The backtester's view of the decision at bar `split-1`.
	agent := NewBreakoutAgent()
	wantSig := agent.Evaluate(NewView(full, split))

	hist := &Series{Symbol: full.Symbol, Interval: full.Interval,
		Candles: append([]Candle(nil), full.Candles[:split-1]...)}

	r := NewLiveRunner(full.Symbol, full.Interval, AgentStrategy{Agent: NewBreakoutAgent()})
	last := hist.Candles[len(hist.Candles)-1].Time
	r.Now = func() time.Time { return last.Add(3 * time.Hour) }
	if err := r.Seed(hist); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := r.OnBars([]Candle{full.Candles[split-1]})
	if err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d signals, want 1", len(got))
	}
	if got[0].Direction != wantSig.Dir.String() {
		t.Fatalf("live decided %q where the backtester decided %q on identical bars",
			got[0].Direction, wantSig.Dir.String())
	}
	if got[0].Strength != wantSig.Strength {
		t.Fatalf("live strength %v, backtest %v", got[0].Strength, wantSig.Strength)
	}
}

// TestLive_WarnsWhenTheKillSwitchCannotTrip. In a backtest the drawdown stop
// reads a simulated equity curve. Live, that curve does not exist, and a
// silent default of zero would leave the switch permanently un-tripped —
// removing the one control that stops a strategy which has started losing.
func TestLive_WarnsWhenTheKillSwitchCannotTrip(t *testing.T) {
	r, now := liveFixture(t)
	r.Now = func() time.Time { return now.Add(2 * time.Hour) }

	got, _ := r.OnBars([]Candle{nextBar(now, 150)})
	if len(got) != 1 {
		t.Fatalf("got %d signals", len(got))
	}
	if !strings.Contains(got[0].Reason, "kill switch cannot trip") {
		t.Fatalf("reason %q must say the drawdown stop is inert without real equity",
			got[0].Reason)
	}

	// Wired to a real account, the warning goes and the switch works.
	r2, now2 := liveFixture(t)
	r2.Now = func() time.Time { return now2.Add(2 * time.Hour) }
	r2.Drawdown = func() float64 { return 0.5 } // deep drawdown

	got2, _ := r2.OnBars([]Candle{nextBar(now2, 150)})
	if len(got2) != 1 {
		t.Fatalf("got %d signals", len(got2))
	}
	if strings.Contains(got2[0].Reason, "kill switch cannot trip") {
		t.Fatal("warning still present with a drawdown source wired in")
	}
	if got2[0].Target != 0 {
		t.Fatalf("target %v at a 50%% drawdown; the kill switch must flatten", got2[0].Target)
	}
}

func TestLive_TrimsTheRollingWindow(t *testing.T) {
	r, now := liveFixture(t)
	r.MaxBars = 200
	r.Now = func() time.Time { return now.Add(100 * time.Hour) }

	var bars []Candle
	for i := 0; i < 50; i++ {
		bars = append(bars, nextBar(now.Add(time.Duration(i)*time.Hour), 150+float64(i)))
	}
	if _, err := r.OnBars(bars); err != nil {
		t.Fatalf("OnBars: %v", err)
	}
	if got := r.Bars(); got != 200 {
		t.Fatalf("held %d bars, want the %d-bar cap", got, 200)
	}
}

func TestLive_EnsembleSignalCarriesEveryMember(t *testing.T) {
	hist := withFlow(trendSeries(500, 7, 0.004))
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := range hist.Candles {
		hist.Candles[i].Time = end.Add(time.Duration(i-len(hist.Candles)+1) * time.Hour)
	}

	e := NewEnsemble()
	r := NewLiveRunner("TEST", time.Hour, e)
	r.Now = func() time.Time { return end.Add(3 * time.Hour) }
	if err := r.Seed(hist); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, _ := r.OnBars([]Candle{nextBar(end.Add(time.Hour), 150)})
	if len(got) != 1 {
		t.Fatalf("got %d signals", len(got))
	}
	if len(got[0].Members) != len(e.Agents) {
		t.Fatalf("%d member signals for %d agents — an unexplained live position is one you "+
			"cannot audit afterwards", len(got[0].Members), len(e.Agents))
	}
}

func TestJSONLSink_WritesOneReplayableLinePerSignal(t *testing.T) {
	var buf bytes.Buffer
	sink := &JSONLSink{W: &buf}

	for i := 0; i < 3; i++ {
		if err := sink.Emit(LiveSignal{
			Symbol: "TEST", Direction: "long", Target: 0.4, Close: float64(100 + i),
		}); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, l := range lines {
		var got LiveSignal
		if err := json.Unmarshal([]byte(l), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
		if got.Close != float64(100+i) {
			t.Fatalf("line %d round-tripped close %v", i, got.Close)
		}
	}
}

// fakeFeed replays scripted polls.
type fakeFeed struct {
	polls [][]Candle
	n     int
}

func (f *fakeFeed) Poll(context.Context) ([]Candle, error) {
	if f.n >= len(f.polls) {
		return nil, nil
	}
	out := f.polls[f.n]
	f.n++
	return out, nil
}

func TestLive_RunEmitsThroughTheSinkAndStopsOnContext(t *testing.T) {
	r, now := liveFixture(t)
	clock := now
	r.Now = func() time.Time { return clock }

	feed := &fakeFeed{polls: [][]Candle{
		{nextBar(now, 150)},
		{nextBar(now.Add(time.Hour), 151)},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	var emitted []LiveSignal
	sink := FuncSink(func(s LiveSignal) error {
		emitted = append(emitted, s)
		clock = clock.Add(time.Hour)
		if len(emitted) == 2 {
			cancel()
		}
		return nil
	})

	// Advance the clock enough that the scripted bars have closed.
	clock = now.Add(3 * time.Hour)
	err := r.Run(ctx, feed, sink, time.Millisecond, nil)
	if err != context.Canceled {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted %d signals, want 2", len(emitted))
	}
}

func TestLive_SeedRejectsBrokenHistory(t *testing.T) {
	r := NewLiveRunner("TEST", time.Hour, AgentStrategy{Agent: NewBreakoutAgent()})
	broken := trendSeries(100, 1, 0.01)
	broken.Candles[50].Time = broken.Candles[49].Time // duplicate timestamp
	if err := r.Seed(broken); err == nil {
		t.Fatal("expected Seed to reject a history that fails validation")
	}
}
