// Package geckoterminal is a dependency-free client for DEX pool history.
//
// It exists because the obvious source does not serve this need. Dexscreener
// publishes a pool's CURRENT state — price, liquidity, trade counts over
// trailing windows — which is what a screener wants and what a backtest
// cannot use. GeckoTerminal publishes OHLCV per pool, free and without a key,
// which is the only public source of DEX bar history that does not require
// either credentials or reconstructing bars from swap logs yourself.
//
// One thing it does NOT publish is the taker split. A centralised venue tells
// you how much volume lifted the offer; a pool aggregator does not. The flow
// agent therefore stands down on DEX series, and that is the correct outcome:
// inferring buy and sell volume from whether a candle closed green is not
// order flow, it is the price series wearing a disguise.
//
// The pool's reserve is fetched alongside the bars, because on an AMM the
// reserve is not context — it is what decides whether any of the bars are
// tradeable at the size you intend. See AMMCosts.
package geckoterminal

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

// Client talks to the public API.
type Client struct {
	// BaseURL is the API root; overridable for tests.
	BaseURL string
	HTTP    *http.Client
	// Pause is applied between requests. The free tier is rate limited at
	// roughly 30 calls a minute, so this is deliberately unhurried.
	Pause time.Duration
}

// New returns a client with conservative defaults.
func New() *Client {
	return &Client{
		BaseURL: "https://api.geckoterminal.com/api/v2",
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Pause:   2200 * time.Millisecond,
	}
}

// Pool describes one liquidity pool.
type Pool struct {
	Address      string
	Name         string
	Network      string
	LiquidityUSD float64
	Volume24hUSD float64
	BaseSymbol   string
}

// TopPool returns the deepest pool trading a token.
//
// Depth rather than volume decides which pool to use, and the distinction is
// not cosmetic: volume on a thin pool is mostly arbitrage bots correcting it
// against a deeper venue, so ranking by volume reliably selects the pool you
// least want to trade against.
func (c *Client) TopPool(ctx context.Context, network, tokenAddress string) (Pool, error) {
	url := fmt.Sprintf("%s/networks/%s/tokens/%s/pools", c.BaseURL, network, tokenAddress)
	body, err := c.get(ctx, url)
	if err != nil {
		return Pool{}, err
	}

	var resp struct {
		Data []struct {
			Attributes struct {
				Address      string `json:"address"`
				Name         string `json:"name"`
				ReserveInUSD string `json:"reserve_in_usd"`
				VolumeUSD    struct {
					H24 string `json:"h24"`
				} `json:"volume_usd"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Pool{}, fmt.Errorf("pools: %w (response: %.200s)", err, body)
	}
	if len(resp.Data) == 0 {
		return Pool{}, fmt.Errorf("no pools listed for %s on %s", tokenAddress, network)
	}

	best := Pool{Network: network}
	for _, d := range resp.Data {
		liq, _ := strconv.ParseFloat(d.Attributes.ReserveInUSD, 64)
		if liq <= best.LiquidityUSD {
			continue
		}
		vol, _ := strconv.ParseFloat(d.Attributes.VolumeUSD.H24, 64)
		best = Pool{
			Address: d.Attributes.Address, Name: d.Attributes.Name, Network: network,
			LiquidityUSD: liq, Volume24hUSD: vol,
		}
	}
	if best.Address == "" {
		return Pool{}, fmt.Errorf("no pool for %s reported a usable reserve", tokenAddress)
	}
	return best, nil
}

// OHLCV fetches bars for a pool.
//
// timeframe is "minute", "hour" or "day"; aggregate multiplies it (hour with
// aggregate 4 gives four-hour bars). The API caps a request at 1000 bars and
// pages backwards from a timestamp, so this walks back until it has enough or
// the pool's history runs out.
//
// The bar currently forming is excluded, on the same grounds as everywhere
// else: its high, low and close are still moving.
func (c *Client) OHLCV(ctx context.Context, network, poolAddress, timeframe string, aggregate, want int) ([]ms.Candle, error) {
	dur, err := BarDuration(timeframe, aggregate)
	if err != nil {
		return nil, err
	}

	byTime := map[int64]ms.Candle{}
	before := time.Now().Unix()
	now := time.Now()

	for len(byTime) < want {
		url := fmt.Sprintf("%s/networks/%s/pools/%s/ohlcv/%s?aggregate=%d&limit=1000&before_timestamp=%d",
			c.BaseURL, network, poolAddress, timeframe, aggregate, before)
		body, err := c.get(ctx, url)
		if err != nil {
			return nil, err
		}

		var resp struct {
			Data struct {
				Attributes struct {
					List [][]json.RawMessage `json:"ohlcv_list"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("ohlcv: %w (response: %.200s)", err, body)
		}
		list := resp.Data.Attributes.List
		if len(list) == 0 {
			break
		}

		oldest := before
		added := 0
		for _, row := range list {
			if len(row) < 6 {
				continue
			}
			ts, err := asInt(row[0])
			if err != nil {
				return nil, err
			}
			if ts < oldest {
				oldest = ts
			}
			if _, seen := byTime[ts]; seen {
				continue
			}
			open := time.Unix(ts, 0).UTC()
			if !now.After(open.Add(dur)) {
				continue // still forming
			}

			vals := make([]float64, 0, 5)
			for i := 1; i <= 5; i++ {
				v, err := asFloat(row[i])
				if err != nil {
					return nil, err
				}
				vals = append(vals, v)
			}
			// BuyVolume and SellVolume are deliberately left at zero: this
			// source reports total volume only, and a fabricated split would
			// hand the flow agent a signal derived from the price it is meant
			// to be independent of.
			byTime[ts] = ms.Candle{
				Time: open, Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3],
				Volume: vals[4],
			}
			added++
		}

		if added == 0 || oldest >= before {
			break
		}
		before = oldest - 1
		time.Sleep(c.Pause)
	}

	out := make([]ms.Candle, 0, len(byTime))
	for _, c := range byTime {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	if len(out) > want {
		out = out[len(out)-want:]
	}
	return out, nil
}

// BarDuration converts a timeframe and aggregate into a duration.
func BarDuration(timeframe string, aggregate int) (time.Duration, error) {
	if aggregate < 1 {
		aggregate = 1
	}
	switch timeframe {
	case "minute":
		return time.Duration(aggregate) * time.Minute, nil
	case "hour":
		return time.Duration(aggregate) * time.Hour, nil
	case "day":
		return time.Duration(aggregate) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("unsupported timeframe %q (want minute, hour or day)", timeframe)
}

// PollFeed adapts the client to marketsignals.Feed for live DEX running.
type PollFeed struct {
	Client      *Client
	Network     string
	PoolAddress string
	Timeframe   string
	Aggregate   int
	Lookback    int
}

// Poll returns recent closed bars, oldest first.
func (f PollFeed) Poll(ctx context.Context) ([]ms.Candle, error) {
	n := f.Lookback
	if n <= 0 {
		n = 5
	}
	return f.Client.OHLCV(ctx, f.Network, f.PoolAddress, f.Timeframe, f.Aggregate, n)
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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

func asFloat(r json.RawMessage) (float64, error) {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return strconv.ParseFloat(s, 64)
	}
	var f float64
	err := json.Unmarshal(r, &f)
	return f, err
}

func asInt(r json.RawMessage) (int64, error) {
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
