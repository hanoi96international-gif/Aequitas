package geckoterminal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(handler http.HandlerFunc) (*Client, func()) {
	srv := httptest.NewServer(handler)
	c := New()
	c.BaseURL = srv.URL
	c.Pause = 0
	return c, srv.Close
}

func TestBarDuration(t *testing.T) {
	for _, tc := range []struct {
		tf   string
		agg  int
		want time.Duration
	}{
		{"minute", 1, time.Minute},
		{"minute", 15, 15 * time.Minute},
		{"hour", 4, 4 * time.Hour},
		{"day", 1, 24 * time.Hour},
		{"hour", 0, time.Hour}, // an aggregate below 1 means 1
	} {
		got, err := BarDuration(tc.tf, tc.agg)
		if err != nil || got != tc.want {
			t.Fatalf("BarDuration(%q,%d) = %v, %v; want %v", tc.tf, tc.agg, got, err, tc.want)
		}
	}
	if _, err := BarDuration("fortnight", 1); err == nil {
		t.Fatal("expected an error for an unsupported timeframe")
	}
}

// TestTopPool_RanksByDepthNotVolume encodes a judgement worth being able to
// argue with. Volume on a thin pool is largely arbitrage bots correcting it
// against a deeper venue, so ranking by volume reliably selects the pool you
// least want to trade against.
func TestTopPool_RanksByDepthNotVolume(t *testing.T) {
	body := `{"data":[
	  {"attributes":{"address":"0xthin","name":"THIN/WETH","reserve_in_usd":"50000","volume_usd":{"h24":"9000000"}}},
	  {"attributes":{"address":"0xdeep","name":"DEEP/WETH","reserve_in_usd":"4000000","volume_usd":{"h24":"120000"}}}
	]}`
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
	defer done()

	got, err := c.TopPool(context.Background(), "eth", "0xtoken")
	if err != nil {
		t.Fatalf("TopPool: %v", err)
	}
	if got.Address != "0xdeep" {
		t.Fatalf("chose %q; the $50k pool has 75x the volume but is the one whose volume is "+
			"mostly bots correcting it", got.Address)
	}
	if got.LiquidityUSD != 4_000_000 {
		t.Fatalf("liquidity %v, want 4000000", got.LiquidityUSD)
	}
	if got.Volume24hUSD != 120_000 {
		t.Fatalf("volume %v, want 120000", got.Volume24hUSD)
	}
}

func TestTopPool_ErrorsWhenNothingIsListed(t *testing.T) {
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	defer done()
	if _, err := c.TopPool(context.Background(), "eth", "0xtoken"); err == nil {
		t.Fatal("expected an error rather than an empty pool")
	}
}

func ohlcvBody(rows ...string) string {
	return `{"data":{"attributes":{"ohlcv_list":[` + strings.Join(rows, ",") + `]}}}`
}

func row(ts int64, o, h, l, c, v float64) string {
	return fmt.Sprintf("[%d,%g,%g,%g,%g,%g]", ts, o, h, l, c, v)
}

func TestOHLCV_ParsesAndOrdersBars(t *testing.T) {
	base := time.Now().Add(-10 * time.Hour).Truncate(time.Hour)
	// The API returns newest first; the client must hand back oldest first.
	body := ohlcvBody(
		row(base.Add(2*time.Hour).Unix(), 102, 108, 101, 107, 3000),
		row(base.Add(time.Hour).Unix(), 101, 104, 100, 102, 2000),
		row(base.Unix(), 100, 103, 99, 101, 1000),
	)
	calls := 0
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, body)
			return
		}
		fmt.Fprint(w, ohlcvBody()) // history exhausted
	})
	defer done()

	got, err := c.OHLCV(context.Background(), "eth", "0xpool", "hour", 1, 10)
	if err != nil {
		t.Fatalf("OHLCV: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d bars, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].Time.After(got[i-1].Time) {
			t.Fatalf("bars are not in ascending time order: %v then %v", got[i-1].Time, got[i].Time)
		}
	}
	if got[0].Close != 101 || got[2].High != 108 {
		t.Fatalf("prices parsed wrong: %+v", got)
	}
}

// TestOHLCV_LeavesTheTakerSplitEmpty. This source reports total volume only. A
// fabricated split would hand the flow agent a signal derived from the very
// price series it is supposed to be independent of — the agent standing down
// is the correct behaviour, and it can only do that if the split is honestly
// absent.
func TestOHLCV_LeavesTheTakerSplitEmpty(t *testing.T) {
	base := time.Now().Add(-5 * time.Hour).Truncate(time.Hour)
	calls := 0
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, ohlcvBody(row(base.Unix(), 100, 103, 99, 101, 5000)))
			return
		}
		fmt.Fprint(w, ohlcvBody())
	})
	defer done()

	got, err := c.OHLCV(context.Background(), "eth", "0xpool", "hour", 1, 5)
	if err != nil {
		t.Fatalf("OHLCV: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bars", len(got))
	}
	if got[0].Volume != 5000 {
		t.Fatalf("volume %v, want 5000", got[0].Volume)
	}
	if got[0].HasFlow() {
		t.Fatalf("a taker split was invented: buy %v sell %v",
			got[0].BuyVolume, got[0].SellVolume)
	}
}

// TestOHLCV_ExcludesTheFormingBar keeps the live path's central rule intact on
// the DEX side too.
func TestOHLCV_ExcludesTheFormingBar(t *testing.T) {
	closed := time.Now().Add(-2 * time.Hour).Truncate(time.Hour)
	forming := time.Now().Truncate(time.Hour)

	calls := 0
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, ohlcvBody(
				row(forming.Unix(), 105, 112, 104, 110, 900),
				row(closed.Unix(), 100, 103, 99, 101, 1000),
			))
			return
		}
		fmt.Fprint(w, ohlcvBody())
	})
	defer done()

	got, err := c.OHLCV(context.Background(), "eth", "0xpool", "hour", 1, 10)
	if err != nil {
		t.Fatalf("OHLCV: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bars, want only the closed one — the forming bar leaked through", len(got))
	}
	if !got[0].Time.Equal(closed.UTC()) {
		t.Fatalf("kept the bar at %s, want %s", got[0].Time, closed.UTC())
	}
}

func TestOHLCV_ReportsAnHTTPError(t *testing.T) {
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"errors":[{"status":"429"}]}`)
	})
	defer done()

	if _, err := c.OHLCV(context.Background(), "eth", "0xpool", "hour", 1, 10); err == nil {
		t.Fatal("expected a rate-limit response to surface as an error rather than as no bars")
	}
}

func TestOHLCV_StopsWhenHistoryRunsOut(t *testing.T) {
	base := time.Now().Add(-3 * time.Hour).Truncate(time.Hour)
	calls := 0
	c, done := testClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, ohlcvBody(row(base.Unix(), 100, 103, 99, 101, 1000)))
			return
		}
		fmt.Fprint(w, ohlcvBody())
	})
	defer done()

	// Asking for a thousand bars from a pool with one must terminate rather
	// than paging for ever.
	got, err := c.OHLCV(context.Background(), "eth", "0xpool", "hour", 1, 1000)
	if err != nil {
		t.Fatalf("OHLCV: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bars, want 1", len(got))
	}
	if calls > 3 {
		t.Fatalf("made %d requests for a one-bar pool; the paging loop is not terminating", calls)
	}
}
