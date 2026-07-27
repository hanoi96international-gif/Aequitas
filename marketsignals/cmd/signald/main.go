// Command signald runs the agents against a live feed and emits a signal on
// every bar close.
//
// It does not trade. It holds no API key, it never authenticates, and there is
// no code path in this binary or in anything it imports that can place an
// order. What it produces is a target position as a fraction of equity, on
// stdout and optionally as a JSONL file. Acting on that is a separate decision
// made by a person or by a system that person wrote.
//
//	go run ./cmd/signald -symbol BTCUSDT -interval 1h -market futures
//
// On start it seeds several hundred bars of history so the agents are not in
// warmup, then polls for closed bars. If the feed drops long enough to leave a
// hole in the series, it stops signalling and says so rather than computing
// indicators across the gap.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
	"github.com/hanoi96international-gif/marketsignals/binance"
)

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "trading pair")
	interval := flag.String("interval", "1h", "bar interval: 1m 5m 15m 1h 4h 1d")
	market := flag.String("market", "spot", "spot or futures")
	seedBars := flag.Int("seed", 600, "how many historical bars to load before starting")
	out := flag.String("out", "", "optional JSONL file to append signals to")
	sector := flag.String("sector", "major", "crypto sector, selecting the expert profile")
	notify := flag.String("notify", "", "webhook URL for notifications (env NOTIFY_URL is safer)")
	notifyField := flag.String("notify-field", "text", "JSON field for the message: text (Telegram/Slack), content (Discord), empty for raw JSON")
	notifyChat := flag.String("notify-chat", "", "Telegram chat_id, if the endpoint needs one")
	minChange := flag.Float64("notify-min-change", 0.20, "position change, as a fraction of equity, worth a message")
	pollEvery := flag.Duration("poll", 0, "poll interval (default: a tenth of the bar interval)")
	trade := flag.Bool("trade", false, "run the execution engine (paper by default — see -broker)")
	equity := flag.Float64("equity", 1000, "paper account size in USD")
	maxPos := flag.Float64("max-position", 0.25, "hard cap on |position| as a fraction of equity")
	maxOrder := flag.Float64("max-order", 0.10, "hard cap on any single order")
	maxDD := flag.Float64("max-drawdown", 0.15, "flatten and halt at this drawdown from peak")
	flag.Parse()

	// Preferring the environment keeps the token out of shell history and out
	// of any `ps` listing on a shared box.
	url := *notify
	if fromEnv := os.Getenv("NOTIFY_URL"); fromEnv != "" {
		url = fromEnv
	}
	cfg := notifyConfig{URL: url, Field: *notifyField, ChatID: *notifyChat, MinChange: *minChange}
	exec := execConfig{
		Enabled: *trade, EquityUSD: *equity,
		Rails: ms.Rails{
			MaxPositionFraction:  *maxPos,
			MaxOrderFraction:     *maxOrder,
			MaxDailyLossFraction: 0.05,
			MaxDrawdownFraction:  *maxDD,
			// Never armed from a flag. Trading real money is a decision that
			// should require editing code and supplying a broker, not
			// remembering which flag was set in a shell history.
			DryRun: false,
		},
	}

	if err := run(*symbol, *interval, *market, *sector, *out, *seedBars, *pollEvery, cfg, exec); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// execConfig configures the execution engine.
//
// There is no flag anywhere in this binary that points it at a real venue.
// The Broker interface is satisfied here only by PaperBroker, and swapping in
// something that trades money is a deliberate code change made by whoever
// holds the credentials — not a flag they might set by accident, and not a
// flag a copied command line might carry.
type execConfig struct {
	Enabled   bool
	EquityUSD float64
	Rails     ms.Rails
}

type notifyConfig struct {
	URL       string
	Field     string
	ChatID    string
	MinChange float64
}

func run(symbol, interval, market, sector, out string, seedBars int, pollEvery time.Duration, notify notifyConfig, exec execConfig) error {
	barDur, err := binance.ParseInterval(interval)
	if err != nil {
		return err
	}
	if pollEvery <= 0 {
		// A few polls per bar is enough: nothing here reacts within a bar, so
		// polling faster only adds requests.
		pollEvery = barDur / 10
		if pollEvery < 5*time.Second {
			pollEvery = 5 * time.Second
		}
	}

	inst := ms.Instrument{
		Symbol: symbol, Class: ms.ClassCrypto, Sector: ms.CryptoSector(sector),
		Venue: ms.VenueCEX, ContinuousTrading: true,
		HasPerpetual: market == "futures",
	}
	profile, err := ms.ExpertFor(inst)
	if err != nil {
		return err
	}
	if !profile.TradeableAtAll {
		return fmt.Errorf("%s: %s", profile.Name, profile.NotTradeableReason)
	}

	fmt.Printf("── %s ──\n%s\n", inst, profile.Describe())

	client := binance.New()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mkt := binance.Spot
	if market == "futures" {
		mkt = binance.Futures
	}

	fmt.Fprintf(os.Stderr, "seeding %d bars of history...\n", seedBars)
	start := time.Now().Add(-time.Duration(seedBars+5) * barDur)
	candles, err := client.Klines(ctx, mkt, symbol, interval, start)
	if err != nil {
		return fmt.Errorf("seeding: %w", err)
	}
	if len(candles) == 0 {
		return fmt.Errorf("no history returned for %s", symbol)
	}

	hist := &ms.Series{Symbol: symbol, Interval: barDur, Candles: candles}
	if mkt == binance.Futures {
		funding, err := client.FundingHistory(ctx, symbol, start)
		if err != nil {
			return fmt.Errorf("funding: %w", err)
		}
		hist = attachFunding(hist, funding)
	}

	ensemble := profile.Ensemble()
	runner := ms.NewLiveRunner(symbol, barDur, ensemble)
	runner.Risk = profile.Backtester().Risk
	if err := runner.Seed(hist); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "seeded %d bars (%s to %s); warmup needs %d\n",
		runner.Bars(),
		hist.Candles[0].Time.Format(time.RFC3339),
		hist.Candles[len(hist.Candles)-1].Time.Format(time.RFC3339),
		runner.Warmup())
	if runner.Bars() < runner.Warmup() {
		fmt.Fprintf(os.Stderr, "WARNING: %d bars is short of the %d this profile needs; "+
			"no signal will be emitted until enough have accumulated\n",
			runner.Bars(), runner.Warmup())
	}

	sinks := []ms.Sink{ms.FuncSink(printSignal)}
	if out != "" {
		f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		sinks = append(sinks, &ms.JSONLSink{W: f})
		fmt.Fprintf(os.Stderr, "appending signals to %s\n", out)
	}

	if notify.URL != "" {
		hook := ms.NewWebhookSink(notify.URL, notify.Field)
		if notify.ChatID != "" {
			hook.Extra = map[string]any{"chat_id": notify.ChatID}
		}
		filter := ms.NewChangeNotifier(hook)
		filter.MinChange = notify.MinChange
		sinks = append(sinks, filter)
		// The URL is never echoed: for most providers it contains the token.
		fmt.Fprintf(os.Stderr,
			"notifying on a position change of %.0f%% or more, and on every side change\n",
			notify.MinChange*100)
	}

	var engine *ms.Engine
	var paper *ms.PaperBroker
	if exec.Enabled {
		paper = ms.NewPaperBroker(exec.EquityUSD)
		engine = ms.NewEngine(paper)
		engine.Rails = exec.Rails

		// This is what makes the drawdown stop real rather than decorative:
		// the runner asks the account, and the paper account has an equity
		// curve because every closed bar revalues it.
		runner.Drawdown = engine.Drawdown

		sinks = append(sinks, ms.FuncSink(func(sig ms.LiveSignal) error {
			paper.Mark(sig.Symbol, sig.Close)
			act, err := engine.OnSignal(context.Background(), sig)
			printAction(act, paper)
			if err != nil && err != ms.ErrHalted {
				return err
			}
			return nil
		}))
		fmt.Fprintf(os.Stderr,
			"PAPER trading a $%.0f account: max position %.0f%%, max order %.0f%%, "+
				"flatten at %.0f%% drawdown\n",
			exec.EquityUSD, exec.Rails.MaxPositionFraction*100,
			exec.Rails.MaxOrderFraction*100, exec.Rails.MaxDrawdownFraction*100)
	}

	fmt.Fprintf(os.Stderr, "polling every %s. This process places no real orders.\n\n", pollEvery)

	feed := binance.PollFeed{Client: client, Market: mkt, Symbol: symbol,
		Interval: interval, Lookback: 5}

	err = runner.Run(ctx, feed, fanout(sinks), pollEvery,
		func(msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format(time.RFC3339), msg) })
	if err == context.Canceled {
		fmt.Fprintln(os.Stderr, "\nstopped.")
		return nil
	}
	return err
}

// attachFunding aligns settlements to bars, forward-filled: a bar carries the
// settlement already in force, never a later one. Bars before the first
// settlement are dropped rather than given a zero, which would read as
// "funding is neutral" when the truth is that it is unknown.
func attachFunding(s *ms.Series, funding map[int64]float64) *ms.Series {
	var stamps []int64
	for t := range funding {
		stamps = append(stamps, t)
	}
	if len(stamps) == 0 {
		return s
	}
	for i := 0; i < len(stamps); i++ {
		for j := i + 1; j < len(stamps); j++ {
			if stamps[j] < stamps[i] {
				stamps[i], stamps[j] = stamps[j], stamps[i]
			}
		}
	}

	out := &ms.Series{Symbol: s.Symbol, Interval: s.Interval}
	for _, c := range s.Candles {
		ms64 := c.Time.UnixMilli()
		idx := -1
		for i, t := range stamps {
			if t > ms64 {
				break
			}
			idx = i
		}
		if idx < 0 {
			continue
		}
		out.Candles = append(out.Candles, c)
		out.Funding = append(out.Funding, funding[stamps[idx]])
	}
	return out
}

func printAction(a ms.Action, p *ms.PaperBroker) {
	switch {
	case a.Halted:
		fmt.Printf("    EXECUTION HALTED: %s\n", a.Reason)
	case a.Fill != nil:
		fmt.Printf("    filled %s %.8f at %.2f (fee %.4f) — paper equity now $%.2f\n",
			a.Order.Side, a.Fill.Quantity, a.Fill.Price, a.Fill.Fee, p.Equity())
	case a.Skipped != "":
		fmt.Printf("    no order: %s\n", a.Skipped)
	}
}

func printSignal(s ms.LiveSignal) error {
	fmt.Printf("%s  %-9s close %-12.6g  target %+.4f of equity\n",
		s.BarTime.Format("2006-01-02 15:04"), s.Direction, s.Close, s.Target)
	fmt.Printf("    %s\n", s.Reason)
	if s.RiskReason != "" {
		fmt.Printf("    risk: %s\n", s.RiskReason)
	}
	for _, m := range s.Members {
		fmt.Printf("      %-11s %-5s %s\n", m.Agent, m.Dir, m.Note)
	}
	fmt.Println()
	return nil
}

// fanout writes each signal to every sink, so a file sink failing does not
// silently stop the console output or vice versa.
func fanout(sinks []ms.Sink) ms.Sink {
	return ms.FuncSink(func(s ms.LiveSignal) error {
		var firstErr error
		for _, sink := range sinks {
			if err := sink.Emit(s); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}
