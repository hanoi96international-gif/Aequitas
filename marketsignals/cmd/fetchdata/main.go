// Command fetchdata pulls real market history into the CSV and JSON files the
// rest of this module reads.
//
// It exists because the environment this package was developed in cannot
// reach any exchange: the egress policy answers 403 to every market-data
// host. Rather than pretend otherwise, the fetching is a separate, dependency-
// free program you run where the network is open.
//
//	go run ./cmd/fetchdata binance -symbol BTCUSDT -days 720 -out btc.csv
//	go run ./cmd/fetchdata binance -market futures -symbol BTCUSDT -days 720 -out btc_perp.csv
//	go run ./cmd/fetchdata dexscreener -chain solana -address <mint> -out launches.json
//
// Standard library only, so `go run` works on a bare Go install with no
// module downloads.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "binance":
		err = binanceCmd(os.Args[2:])
	case "dexscreener":
		err = dexscreenerCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `fetchdata — pull real market history for marketsignals

  fetchdata binance -symbol BTCUSDT [-market spot|futures] [-interval 1h] [-days 720] -out FILE.csv
        Download klines and write the CSV signalctl reads. Binance reports the
        taker buy volume per bar, so the order-flow agent gets a real buy/sell
        split rather than a guess. With -market futures the funding rate
        history is fetched too and aligned to the bars, which is what the
        positioning agent needs.

  fetchdata dexscreener -chain solana -address ADDR -out FILE.json
        Fetch what Dexscreener actually publishes about a token. This is NOT
        enough to clear the launch screener on its own — see the note it
        prints — and the missing checks are recorded as unperformed rather
        than filled in with reassuring defaults.

Then:
  go run ./cmd/signalctl search -csv btc.csv -csv eth.csv -grid full
`)
}

// ── Binance ──────────────────────────────────────────────────────────────

func binanceCmd(args []string) error {
	fs := flag.NewFlagSet("binance", flag.ExitOnError)
	symbol := fs.String("symbol", "", "trading pair, e.g. BTCUSDT (required)")
	market := fs.String("market", "spot", "spot or futures; futures adds a funding column")
	interval := fs.String("interval", "1h", "bar interval: 1m 5m 15m 1h 4h 1d")
	days := fs.Int("days", 720, "how many days of history to fetch")
	out := fs.String("out", "", "output CSV path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *symbol == "" || *out == "" {
		fs.Usage()
		return fmt.Errorf("-symbol and -out are required")
	}

	base := "https://api.binance.com/api/v3"
	if *market == "futures" {
		base = "https://fapi.binance.com/fapi/v1"
	} else if *market != "spot" {
		return fmt.Errorf("-market must be spot or futures, got %q", *market)
	}

	start := time.Now().AddDate(0, 0, -*days)
	fmt.Fprintf(os.Stderr, "fetching %s %s klines from %s...\n",
		*symbol, *interval, start.Format("2006-01-02"))

	bars, err := fetchKlines(base, *symbol, *interval, start)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return fmt.Errorf("no bars returned for %s — check the symbol spelling", *symbol)
	}
	fmt.Fprintf(os.Stderr, "got %d bars (%s to %s)\n", len(bars),
		bars[0].Time.Format("2006-01-02"), bars[len(bars)-1].Time.Format("2006-01-02"))

	var funding map[int64]float64
	if *market == "futures" {
		fmt.Fprintln(os.Stderr, "fetching funding history...")
		funding, err = fetchFunding(*symbol, start)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "got %d funding points\n", len(funding))
	}

	return writeCSV(*out, bars, funding)
}

type bar struct {
	Time                   time.Time
	Open, High, Low, Close float64
	Volume                 float64
	TakerBuyVolume         float64
}

// fetchKlines pages through Binance's 1000-bar-per-request limit.
func fetchKlines(base, symbol, interval string, start time.Time) ([]bar, error) {
	var out []bar
	cursor := start.UnixMilli()
	seen := map[int64]bool{}

	for {
		url := fmt.Sprintf("%s/klines?symbol=%s&interval=%s&startTime=%d&limit=1000",
			base, symbol, interval, cursor)
		body, err := get(url)
		if err != nil {
			return nil, err
		}

		// Each kline is a heterogeneous array; decode field by field.
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
			openMs, err := asInt(k[0])
			if err != nil {
				return nil, err
			}
			if seen[openMs] {
				continue
			}
			seen[openMs] = true
			progressed = true

			nums := make([]float64, 0, 5)
			for _, idx := range []int{1, 2, 3, 4, 5} {
				v, err := asFloat(k[idx])
				if err != nil {
					return nil, err
				}
				nums = append(nums, v)
			}
			// Field 9 is the taker BUY base-asset volume: the part of the
			// bar's volume that lifted the offer. Everything else was a taker
			// sell. This is a genuine order-flow split from the venue, not an
			// inference from whether the candle closed green.
			takerBuy, err := asFloat(k[9])
			if err != nil {
				return nil, err
			}

			out = append(out, bar{
				Time: time.UnixMilli(openMs).UTC(),
				Open: nums[0], High: nums[1], Low: nums[2], Close: nums[3],
				Volume: nums[4], TakerBuyVolume: takerBuy,
			})
		}

		if !progressed || len(raw) < 1000 {
			break
		}
		last, err := asInt(raw[len(raw)-1][0])
		if err != nil {
			return nil, err
		}
		cursor = last + 1
		fmt.Fprintf(os.Stderr, "  %d bars...\n", len(out))
		time.Sleep(250 * time.Millisecond) // stay well inside the rate limit
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// fetchFunding returns funding rates keyed by their effective millisecond.
func fetchFunding(symbol string, start time.Time) (map[int64]float64, error) {
	out := map[int64]float64{}
	cursor := start.UnixMilli()

	for {
		url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/fundingRate?symbol=%s&startTime=%d&limit=1000",
			symbol, cursor)
		body, err := get(url)
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
		time.Sleep(250 * time.Millisecond)
	}
	return out, nil
}

// writeCSV emits the exact column layout LoadCSV expects.
func writeCSV(path string, bars []bar, funding map[int64]float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := "time,open,high,low,close,volume,buy_volume,sell_volume"
	if funding != nil {
		header += ",funding"
	}
	fmt.Fprintln(f, header)

	// Funding settles every eight hours; the rate in effect over a bar is the
	// most recent settlement at or before it. Forward-filling is the honest
	// alignment — carrying a LATER settlement backwards onto an earlier bar
	// would hand the agent a number that did not exist yet.
	var stamps []int64
	for t := range funding {
		stamps = append(stamps, t)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })

	skipped := 0
	for _, b := range bars {
		sell := b.Volume - b.TakerBuyVolume
		if sell < 0 {
			sell = 0
		}
		line := fmt.Sprintf("%d,%.10g,%.10g,%.10g,%.10g,%.10g,%.10g,%.10g",
			b.Time.Unix(), b.Open, b.High, b.Low, b.Close, b.Volume, b.TakerBuyVolume, sell)

		if funding != nil {
			ms := b.Time.UnixMilli()
			idx := sort.Search(len(stamps), func(i int) bool { return stamps[i] > ms }) - 1
			if idx < 0 {
				// No settlement has happened yet at this bar. Writing 0 would
				// read as "funding is neutral" when the truth is that it is
				// unknown, so the bar is dropped instead.
				skipped++
				continue
			}
			line += fmt.Sprintf(",%.10g", funding[stamps[idx]])
		}
		fmt.Fprintln(f, line)
	}

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "dropped %d leading bars with no funding settlement yet\n", skipped)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

// ── Dexscreener ──────────────────────────────────────────────────────────

func dexscreenerCmd(args []string) error {
	fs := flag.NewFlagSet("dexscreener", flag.ExitOnError)
	chain := fs.String("chain", "", "chain id, e.g. solana, base, ethereum (required)")
	address := fs.String("address", "", "token address (required)")
	out := fs.String("out", "", "output JSON path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chain == "" || *address == "" || *out == "" {
		fs.Usage()
		return fmt.Errorf("-chain, -address and -out are required")
	}

	body, err := get("https://api.dexscreener.com/latest/dex/tokens/" + *address)
	if err != nil {
		return err
	}
	var resp struct {
		Pairs []struct {
			ChainID   string `json:"chainId"`
			BaseToken struct {
				Address string `json:"address"`
				Symbol  string `json:"symbol"`
			} `json:"baseToken"`
			Liquidity struct {
				USD float64 `json:"usd"`
			} `json:"liquidity"`
			Volume struct {
				H1 float64 `json:"h1"`
			} `json:"volume"`
			Txns struct {
				H1 struct {
					Buys  int `json:"buys"`
					Sells int `json:"sells"`
				} `json:"h1"`
			} `json:"txns"`
			PairCreatedAt int64 `json:"pairCreatedAt"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("dexscreener: %w (response: %.200s)", err, body)
	}
	if len(resp.Pairs) == 0 {
		return fmt.Errorf("dexscreener has no pairs for %s on %s", *address, *chain)
	}

	p := resp.Pairs[0]
	age := 0.0
	if p.PairCreatedAt > 0 {
		age = time.Since(time.UnixMilli(p.PairCreatedAt)).Minutes()
	}

	// Only the fields Dexscreener genuinely reports are populated. Everything
	// the screener vetoes on — authorities, LP lock, honeypot simulation,
	// holder concentration, deployer history — is left unset AND marked
	// unchecked, so the screener rejects for "not verified" instead of
	// mistaking a blank for a pass.
	launch := map[string]any{
		"symbol":           p.BaseToken.Symbol,
		"chain":            p.ChainID,
		"token_address":    p.BaseToken.Address,
		"age_minutes":      age,
		"liquidity_usd":    p.Liquidity.USD,
		"volume_usd_1h":    p.Volume.H1,
		"tx_count_1h":      p.Txns.H1.Buys + p.Txns.H1.Sells,
		"unique_buyers_1h": 0, // Dexscreener reports trade counts, not distinct wallets
		"checks": map[string]bool{
			"authorities":         false,
			"liquidity_lock":      false,
			"sell_simulation":     false,
			"holder_distribution": false,
			"deployer_history":    false,
			"source_verification": false,
		},
	}

	b, _ := json.MarshalIndent([]any{launch}, "", "  ")
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, `wrote %s

NOTE: Dexscreener publishes price, liquidity and trade counts. It does NOT
publish the things that decide whether a token can be held at all — mint and
freeze authority, LP lock or burn, whether a sell simulates successfully,
holder concentration, or the deployer's history. Those need an RPC node or a
dedicated safety API.

Every one of those is recorded as UNCHECKED, so the screener will reject this
token on the grounds that nobody looked. That is the correct outcome: an
unchecked authority is not an absent one, and the failure mode of assuming
otherwise is buying a token whose supply the deployer can multiply at will.
`, *out)
	return nil
}

// ── HTTP ─────────────────────────────────────────────────────────────────

func get(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "marketsignals-fetchdata/1.0")

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
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
