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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
	"github.com/hanoi96international-gif/marketsignals/binance"
	gecko "github.com/hanoi96international-gif/marketsignals/geckoterminal"
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
	case "dex":
		err = dexCmd(os.Args[2:])
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

  fetchdata dex -chain eth -address 0xTOKEN [-timeframe hour] [-bars 1000] [-account 10000] -out FILE.csv
        Download DEX pool bars via GeckoTerminal, which is the only free
        public source of OHLCV history for a pool. Picks the DEEPEST pool for
        the token, reports the pool's reserve, and prints how large a position
        that reserve can actually absorb at your account size — on an AMM that
        number decides viability before any Sharpe ratio does.

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

	mkt := binance.Spot
	switch *market {
	case "spot":
	case "futures":
		mkt = binance.Futures
	default:
		return fmt.Errorf("-market must be spot or futures, got %q", *market)
	}

	ctx := context.Background()
	client := binance.New()
	start := time.Now().AddDate(0, 0, -*days)

	fmt.Fprintf(os.Stderr, "fetching %s %s klines from %s...\n",
		*symbol, *interval, start.Format("2006-01-02"))
	bars, err := client.Klines(ctx, mkt, *symbol, *interval, start)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return fmt.Errorf("no bars returned for %s — check the symbol spelling", *symbol)
	}
	fmt.Fprintf(os.Stderr, "got %d bars (%s to %s)\n", len(bars),
		bars[0].Time.Format("2006-01-02"), bars[len(bars)-1].Time.Format("2006-01-02"))

	var funding map[int64]float64
	if mkt == binance.Futures {
		fmt.Fprintln(os.Stderr, "fetching funding history...")
		funding, err = client.FundingHistory(ctx, *symbol, start)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "got %d funding points\n", len(funding))
	}

	return writeCSV(*out, bars, funding)
}

// writeCSV emits the exact column layout LoadCSV expects.
func writeCSV(path string, bars []ms.Candle, funding map[int64]float64) error {
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
		line := fmt.Sprintf("%d,%.10g,%.10g,%.10g,%.10g,%.10g,%.10g,%.10g",
			b.Time.Unix(), b.Open, b.High, b.Low, b.Close, b.Volume, b.BuyVolume, b.SellVolume)

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

// ── DEX bars ─────────────────────────────────────────────────────────────

func dexCmd(args []string) error {
	fs := flag.NewFlagSet("dex", flag.ExitOnError)
	chain := fs.String("chain", "", "network id, e.g. eth, base, solana, bsc (required)")
	address := fs.String("address", "", "token address (required)")
	pool := fs.String("pool", "", "pool address; default is the deepest pool for the token")
	timeframe := fs.String("timeframe", "hour", "minute, hour or day")
	aggregate := fs.Int("aggregate", 1, "multiplier on the timeframe (hour + 4 = 4h bars)")
	bars := fs.Int("bars", 1000, "how many bars to fetch")
	account := fs.Float64("account", 10000, "account size in USD, for the cost analysis")
	out := fs.String("out", "", "output CSV path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chain == "" || *out == "" || (*address == "" && *pool == "") {
		fs.Usage()
		return fmt.Errorf("-chain, -out and one of -address or -pool are required")
	}

	ctx := context.Background()
	client := gecko.New()

	poolAddr, liquidity := *pool, 0.0
	if poolAddr == "" {
		fmt.Fprintf(os.Stderr, "finding the deepest pool for %s on %s...\n", *address, *chain)
		p, err := client.TopPool(ctx, *chain, *address)
		if err != nil {
			return err
		}
		poolAddr, liquidity = p.Address, p.LiquidityUSD
		fmt.Fprintf(os.Stderr, "using %s (%s) — reserve $%.0f, 24h volume $%.0f\n",
			p.Name, p.Address, p.LiquidityUSD, p.Volume24hUSD)
	}

	fmt.Fprintf(os.Stderr, "fetching %d %s bars...\n", *bars, *timeframe)
	candles, err := client.OHLCV(ctx, *chain, poolAddr, *timeframe, *aggregate, *bars)
	if err != nil {
		return err
	}
	if len(candles) == 0 {
		return fmt.Errorf("no bars returned for pool %s", poolAddr)
	}
	fmt.Fprintf(os.Stderr, "got %d bars (%s to %s)\n", len(candles),
		candles[0].Time.Format("2006-01-02"), candles[len(candles)-1].Time.Format("2006-01-02"))

	if err := writeCSV(*out, candles, nil); err != nil {
		return err
	}

	fmt.Fprint(os.Stderr, `
NOTE: this source reports total volume only, with no taker buy/sell split, so
the order-flow agent will stand down on this series. Inferring a split from
whether a candle closed green is not order flow — it is the price series
wearing a disguise, and an agent fed that would duplicate what the price
agents already said.
`)

	if liquidity > 0 {
		costs := ms.DefaultAMMCosts(liquidity, *account)
		fmt.Fprintf(os.Stderr, `
COST ANALYSIS — read this before the Sharpe ratio.
  %s

  Largest position whose cost stays under 1%% per unit traded: %.2f%% of a
  $%.0f account.

On an AMM your own trade moves the price before you are filled, so cost grows
with the SQUARE of position size. A strategy is not profitable or unprofitable
in the abstract here — it is profitable up to a size. Run the search with:

  go run ./cmd/signalctl search -csv %s -grid full

but treat any result above that position size as describing a trade you cannot
actually make.
`, costs.Describe(), costs.MaxViablePosition(0.01)*100, *account, *out)
	}
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
