// Package binance is a minimal, dependency-free client for the public market
// data endpoints — enough to seed history and to poll for closed bars.
//
// It uses REST polling rather than the WebSocket stream, and that is a
// deliberate choice rather than a shortcut. Everything in this framework
// decides on BAR CLOSES: nothing reacts within a bar, so nothing benefits from
// tick-level delivery. What polling buys in exchange is the absence of an
// entire failure class — no frame parsing, no reconnect state machine, no
// silent half-open socket delivering nothing while the process looks healthy.
// For minute bars and slower, a poll a few times per bar is both sufficient
// and considerably harder to get wrong.
//
// No credentials are involved anywhere here. These are public endpoints, and
// this package cannot place an order because it never authenticates.
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

// Market selects the spot or USDⓈ-M futures API.
type Market string

const (
	Spot    Market = "spot"
	Futures Market = "futures"
)

func (m Market) base() (string, error) {
	switch m {
	case Spot:
		return "https://api.binance.com/api/v3", nil
	case Futures:
		return "https://fapi.binance.com/fapi/v1", nil
	}
	return "", fmt.Errorf("unknown market %q (want spot or futures)", m)
}

// Client talks to the public endpoints.
type Client struct {
	HTTP *http.Client
	// Pause is applied between paged requests to stay inside the rate limit.
	Pause time.Duration
}

// New returns a client with conservative defaults.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 60 * time.Second}, Pause: 250 * time.Millisecond}
}

// Klines fetches bars from start to now, paging through the 1000-per-request
// limit. The returned candles carry a real taker split: Binance reports the
// taker BUY base volume per bar, so the remainder is taker sell — an order-flow
// split from the venue rather than an inference from the candle's colour.
//
// The bar currently forming is EXCLUDED. Its high, low and close are still
// moving, and an agent handed it decides on numbers that have not settled.
func (c *Client) Klines(ctx context.Context, market Market, symbol, interval string, start time.Time) ([]ms.Candle, error) {
	base, err := market.base()
	if err != nil {
		return nil, err
	}
	return c.klinesFrom(ctx, base, symbol, interval, start)
}

// klinesFrom is Klines with the API root already resolved, so the parsing and
// the closed-bar rule can be exercised against a local server. It takes base
// and symbol separately rather than a pre-built endpoint: a caller assembling
// the query itself is one forgotten "?" away from a URL that fails to parse.
func (c *Client) klinesFrom(ctx context.Context, base, symbol, interval string, start time.Time) ([]ms.Candle, error) {
	dur, err := ParseInterval(interval)
	if err != nil {
		return nil, err
	}

	var out []ms.Candle
	seen := map[int64]bool{}
	cursor := start.UnixMilli()
	now := time.Now()

	for {
		url := fmt.Sprintf("%s/klines?symbol=%s&interval=%s&startTime=%d&limit=1000",
			base, symbol, interval, cursor)
		body, err := c.get(ctx, url)
		if err != nil {
			return nil, err
		}
		var raw [][]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("klines: %w (response: %.200s)", err, body)
		}
		if len(raw) == 0 {
			break
		}

		progressed := false
		for _, k := range raw {
			if len(k) < 10 {
				continue
			}
			openMs, err := AsInt(k[0])
			if err != nil {
				return nil, err
			}
			if seen[openMs] {
				continue
			}
			seen[openMs] = true
			progressed = true

			open := time.UnixMilli(openMs).UTC()
			if !now.After(open.Add(dur)) {
				continue // still forming
			}

			vals := make([]float64, 0, 5)
			for _, idx := range []int{1, 2, 3, 4, 5} {
				v, err := AsFloat(k[idx])
				if err != nil {
					return nil, err
				}
				vals = append(vals, v)
			}
			takerBuy, err := AsFloat(k[9])
			if err != nil {
				return nil, err
			}
			sell := vals[4] - takerBuy
			if sell < 0 {
				sell = 0
			}

			out = append(out, ms.Candle{
				Time: open, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3],
				Volume: vals[4], BuyVolume: takerBuy, SellVolume: sell,
			})
		}

		if !progressed || len(raw) < 1000 {
			break
		}
		last, err := AsInt(raw[len(raw)-1][0])
		if err != nil {
			return nil, err
		}
		cursor = last + 1
		time.Sleep(c.Pause)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// FundingHistory returns settlement rates keyed by their millisecond.
func (c *Client) FundingHistory(ctx context.Context, symbol string, start time.Time) (map[int64]float64, error) {
	out := map[int64]float64{}
	cursor := start.UnixMilli()

	for {
		url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/fundingRate?symbol=%s&startTime=%d&limit=1000",
			symbol, cursor)
		body, err := c.get(ctx, url)
		if err != nil {
			return nil, err
		}
		var page []struct {
			FundingTime int64  `json:"fundingTime"`
			FundingRate string `json:"fundingRate"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("funding: %w (response: %.200s)", err, body)
		}
		if len(page) == 0 {
			break
		}
		before := len(out)
		for _, p := range page {
			r, err := strconv.ParseFloat(p.FundingRate, 64)
			if err != nil {
				return nil, err
			}
			out[p.FundingTime] = r
		}
		if len(out) == before || len(page) < 1000 {
			break
		}
		cursor = page[len(page)-1].FundingTime + 1
		time.Sleep(c.Pause)
	}
	return out, nil
}

// PollFeed adapts the client to marketsignals.Feed, returning only the most
// recent CLOSED bars.
type PollFeed struct {
	Client   *Client
	Market   Market
	Symbol   string
	Interval string
	// Lookback is how many recent bars to request per poll. A handful is
	// plenty: the runner discards everything it already holds, and the small
	// overlap is what lets a poll that arrives late still catch up without a
	// gap.
	Lookback int
}

// Poll returns recent closed bars, oldest first.
func (f PollFeed) Poll(ctx context.Context) ([]ms.Candle, error) {
	dur, err := ParseInterval(f.Interval)
	if err != nil {
		return nil, err
	}
	n := f.Lookback
	if n <= 0 {
		n = 5
	}
	start := time.Now().Add(-time.Duration(n+1) * dur)
	return f.Client.Klines(ctx, f.Market, f.Symbol, f.Interval, start)
}

// ParseInterval converts Binance's interval notation to a duration.
func ParseInterval(s string) (time.Duration, error) {
	table := map[string]time.Duration{
		"1m": time.Minute, "3m": 3 * time.Minute, "5m": 5 * time.Minute,
		"15m": 15 * time.Minute, "30m": 30 * time.Minute,
		"1h": time.Hour, "2h": 2 * time.Hour, "4h": 4 * time.Hour,
		"6h": 6 * time.Hour, "8h": 8 * time.Hour, "12h": 12 * time.Hour,
		"1d": 24 * time.Hour, "3d": 72 * time.Hour, "1w": 168 * time.Hour,
	}
	if d, ok := table[s]; ok {
		return d, nil
	}
	return 0, fmt.Errorf("unsupported interval %q", s)
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "marketsignals/1.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %d: %.200s", url, resp.StatusCode, body)
	}
	return body, nil
}

// AsFloat reads a JSON value that Binance may quote as a string.
func AsFloat(r json.RawMessage) (float64, error) {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return strconv.ParseFloat(s, 64)
	}
	var f float64
	err := json.Unmarshal(r, &f)
	return f, err
}

// AsInt reads a JSON integer that Binance may quote as a string.
func AsInt(r json.RawMessage) (int64, error) {
	var i int64
	if err := json.Unmarshal(r, &i); err == nil {
		return i, nil
	}
	var s string
	if err := json.Unmarshal(r, &s); err != nil {
		return 0, err
	}
	return strconv.ParseInt(s, 10, 64)
}
