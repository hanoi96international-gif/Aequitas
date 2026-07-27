package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The client is tested against a local server rather than the exchange. What
// needs proving is the parsing and the closed-bar rule, and both are decided
// by the response bytes — a live endpoint would add flakiness without adding
// evidence.

func TestParseInterval(t *testing.T) {
	if d, err := ParseInterval("1h"); err != nil || d != time.Hour {
		t.Fatalf("1h = %v, %v", d, err)
	}
	if d, err := ParseInterval("15m"); err != nil || d != 15*time.Minute {
		t.Fatalf("15m = %v, %v", d, err)
	}
	if _, err := ParseInterval("7s"); err == nil {
		t.Fatal("expected an error for an unsupported interval")
	}
}

func TestAsFloatAndAsInt_HandleMixedTypes(t *testing.T) {
	// Binance returns times as JSON numbers and prices as JSON strings inside
	// the same array, which is the only reason these helpers exist.
	if f, err := AsFloat(json.RawMessage(`"12345.6789"`)); err != nil || f != 12345.6789 {
		t.Fatalf("quoted number = %v, %v", f, err)
	}
	if f, err := AsFloat(json.RawMessage(`12345.6789`)); err != nil || f != 12345.6789 {
		t.Fatalf("bare number = %v, %v", f, err)
	}
	if i, err := AsInt(json.RawMessage(`1704067200000`)); err != nil || i != 1704067200000 {
		t.Fatalf("bare int = %v, %v", i, err)
	}
	if i, err := AsInt(json.RawMessage(`"1704067200000"`)); err != nil || i != 1704067200000 {
		t.Fatalf("quoted int = %v, %v", i, err)
	}
	if _, err := AsFloat(json.RawMessage(`"not a number"`)); err == nil {
		t.Fatal("expected an error rather than a silent zero price")
	}
}

// klineJSON renders one kline in Binance's wire shape: open time as a number,
// prices as strings, and the taker buy base volume at index 9.
func klineJSON(openMs int64, o, h, l, c, vol, takerBuy float64) string {
	return fmt.Sprintf(`[%d,"%g","%g","%g","%g","%g",%d,"0",0,"%g","0","0"]`,
		openMs, o, h, l, c, vol, openMs+3599999, takerBuy)
}

func TestKlines_ParsesAndSplitsTakerVolume(t *testing.T) {
	// Two bars, both closed an hour or more ago.
	base := time.Now().Add(-5 * time.Hour).Truncate(time.Hour)
	body := "[" +
		klineJSON(base.UnixMilli(), 100, 105, 99, 104, 1000, 600) + "," +
		klineJSON(base.Add(time.Hour).UnixMilli(), 104, 110, 103, 108, 2000, 1500) +
		"]"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := New()
	c.Pause = 0
	got, err := c.klinesFrom(context.Background(), srv.URL, "TEST", "1h", base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d bars, want 2", len(got))
	}
	if got[0].Close != 104 || got[1].High != 110 {
		t.Fatalf("prices parsed wrong: %+v", got)
	}
	// Taker buy is reported; the remainder is taker sell. This is what gives
	// the flow agent a real split instead of an inference from candle colour.
	if got[0].BuyVolume != 600 || got[0].SellVolume != 400 {
		t.Fatalf("split %v/%v, want 600/400", got[0].BuyVolume, got[0].SellVolume)
	}
	if got[1].BuyVolume != 1500 || got[1].SellVolume != 500 {
		t.Fatalf("split %v/%v, want 1500/500", got[1].BuyVolume, got[1].SellVolume)
	}
}

// TestKlines_ExcludesTheFormingBar is the client's half of the rule the whole
// live path rests on. A poll returns the bar currently forming as its last
// element; its high, low and close are still moving, and an agent handed it
// decides on numbers that have not settled.
func TestKlines_ExcludesTheFormingBar(t *testing.T) {
	closed := time.Now().Add(-2 * time.Hour).Truncate(time.Hour)
	forming := time.Now().Truncate(time.Hour) // this hour, still running

	body := "[" +
		klineJSON(closed.UnixMilli(), 100, 105, 99, 104, 1000, 600) + "," +
		klineJSON(forming.UnixMilli(), 104, 110, 103, 108, 500, 300) +
		"]"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := New()
	c.Pause = 0
	got, err := c.klinesFrom(context.Background(), srv.URL, "TEST", "1h", closed.Add(-time.Hour))
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bars, want only the closed one — the forming bar leaked through", len(got))
	}
	if !got[0].Time.Equal(closed.UTC()) {
		t.Fatalf("kept the bar at %s, want the closed one at %s", got[0].Time, closed.UTC())
	}
}

func TestKlines_ReportsAnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"code":-1003,"msg":"Too many requests"}`)
	}))
	defer srv.Close()

	c := New()
	c.Pause = 0
	if _, err := c.klinesFrom(context.Background(), srv.URL, "TEST", "1h", time.Now().Add(-time.Hour)); err == nil {
		t.Fatal("expected a rate-limit response to surface as an error rather than as no bars")
	}
}

func TestKlines_RejectsAnUnknownMarket(t *testing.T) {
	if _, err := New().Klines(context.Background(), Market("dark-pool"), "X", "1h", time.Now()); err == nil {
		t.Fatal("expected an error for an unknown market")
	}
}
