// Command signalctl runs the marketsignals agents from the command line.
//
// It does three things and refuses to do a fourth: it evaluates agents over
// historical bars, it screens new token launches, and it prints what the
// agents currently think. It does not place orders, hold keys, or talk to an
// exchange. Turning a signal into a trade is a separate decision that should
// be made by a person who has read the evaluation output.
package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "backtest":
		err = backtestCmd(os.Args[2:])
	case "screen":
		err = screenCmd(os.Args[2:])
	case "signal":
		err = signalCmd(os.Args[2:])
	case "demo":
		err = demoCmd(os.Args[2:])
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
	fmt.Fprint(os.Stderr, `signalctl — market signal agents

  signalctl backtest -csv BARS.csv [-interval 1h] [-folds 5] [-fee 0.0005] [-slip 0.0010]
        Evaluate every agent and the ensemble over historical bars, ranked by
        the probability that the result is an edge rather than the best of
        several draws from noise.

  signalctl signal -csv BARS.csv [-interval 1h]
        Print what each agent thinks about the most recent bar.

  signalctl screen -json LAUNCHES.json
        Screen new token launches. Expect most of them to be rejected.

  signalctl demo
        Run the full evaluation on a synthetic random walk, to show what an
        honest "no edge here" result looks like.

CSV columns: time,open,high,low,close,volume[,buy_volume,sell_volume][,funding]
This tool never places an order.
`)
}

func backtestCmd(args []string) error {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	path := fs.String("csv", "", "path to a bars CSV (required)")
	symbol := fs.String("symbol", "SERIES", "symbol label for reporting")
	interval := fs.Duration("interval", time.Hour, "bar duration")
	folds := fs.Int("folds", 5, "walk-forward folds")
	fee := fs.Float64("fee", ms.DefaultCosts().FeeRate, "taker fee as a fraction of notional")
	slip := fs.Float64("slip", ms.DefaultCosts().SlippageRate, "assumed slippage as a fraction")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		fs.Usage()
		return fmt.Errorf("-csv is required")
	}

	s, err := ms.LoadCSV(*path, *symbol, *interval)
	if err != nil {
		return err
	}

	bt := ms.NewBacktester()
	bt.Costs = ms.Costs{FeeRate: *fee, SlippageRate: *slip}

	sel, err := bt.SelectBest(s, *folds, candidates()...)
	if err != nil {
		return err
	}

	fmt.Printf("%s — %d bars at %s\n\n", s.Symbol, len(s.Candles), *interval)
	fmt.Print(sel.Report())
	fmt.Print(`
Read this as follows. "P(edge real)" already accounts for how many candidates
were compared: with enough attempts, the best backtest looks good whether or
not anything real is there. Below 0.95, the honest conclusion is that nothing
has been demonstrated — not that the strategy is nearly there.

Costs assumed: `)
	fmt.Printf("%.1fbp fee + %.1fbp slippage per unit traded. If the ranking changes\n",
		*fee*10_000, *slip*10_000)
	fmt.Print("when you double the slippage, the result was a fee assumption, not an edge.\n")
	return nil
}

func signalCmd(args []string) error {
	fs := flag.NewFlagSet("signal", flag.ExitOnError)
	path := fs.String("csv", "", "path to a bars CSV (required)")
	symbol := fs.String("symbol", "SERIES", "symbol label")
	interval := fs.Duration("interval", time.Hour, "bar duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		fs.Usage()
		return fmt.Errorf("-csv is required")
	}

	s, err := ms.LoadCSV(*path, *symbol, *interval)
	if err != nil {
		return err
	}
	e := ms.NewEnsemble()
	res := e.Evaluate(ms.NewView(s, len(s.Candles)))

	last := s.Candles[len(s.Candles)-1]
	fmt.Printf("%s as of %s (close %.6f)\n\n", s.Symbol, last.Time.Format(time.RFC3339), last.Close)
	for _, m := range res.Members {
		fmt.Printf("  %-10s %-5s  %s\n", m.Agent, m.Dir, m.Note)
	}
	fmt.Printf("\n  %-10s %-5s  %s\n", "ENSEMBLE", res.Dir, res.Note)
	if res.Dir != ms.Flat {
		sized := ms.NewRiskManager().Size(res.Signal, ms.NewView(s, len(s.Candles)), 0)
		fmt.Printf("  %-10s %-5s  %s\n", "size", "", sized.Reason)
	}
	fmt.Print("\nA signal is not a trade. Nothing here has been executed.\n")
	return nil
}

func screenCmd(args []string) error {
	fs := flag.NewFlagSet("screen", flag.ExitOnError)
	path := fs.String("json", "", "path to a JSON array of launches (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		fs.Usage()
		return fmt.Errorf("-json is required")
	}

	launches, err := ms.LoadLaunches(*path)
	if err != nil {
		return err
	}
	a := ms.NewLaunchAgent()

	counts := map[ms.Verdict]int{}
	for _, l := range launches {
		screen := a.Screen(l)
		counts[screen.Verdict]++
		fmt.Print(screen.Summary(), "\n")
	}
	fmt.Printf("%d screened: %d rejected, %d watch, %d accepted\n",
		len(launches), counts[ms.Reject], counts[ms.Watch], counts[ms.Accept])
	if counts[ms.Accept] > 0 {
		fmt.Print("\nAn accepted launch is still a high-variance bet on an asset days old.\n" +
			"The screener removes engineered losses; it does not find winners.\n")
	}
	return nil
}

func demoCmd(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	bars := fs.Int("bars", 4000, "how many synthetic bars to generate")
	seed := fs.Int64("seed", 2024, "random seed")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s := syntheticRandomWalk(*bars, *seed)
	sel, err := ms.NewBacktester().SelectBest(s, 5, candidates()...)
	if err != nil {
		return err
	}

	fmt.Print(`This is a pure random walk. There is no edge in it, by construction.
Every agent below is being run on data that cannot be predicted, so this is
what the evaluation should say when a strategy has found nothing:

`)
	fmt.Print(sel.Report())
	fmt.Print(`
Note that several agents show a NEGATIVE Sharpe. That is correct and expected:
trading costs are charged on a market with no edge, so the honest outcome of
trading noise is to lose the fees. A framework that showed profits here would
be broken.
`)
	return nil
}

func candidates() []ms.Strategy {
	return []ms.Strategy{
		ms.AgentStrategy{Agent: ms.NewBreakoutAgent()},
		ms.AgentStrategy{Agent: ms.NewReversionAgent()},
		ms.AgentStrategy{Agent: ms.NewFlowAgent()},
		ms.AgentStrategy{Agent: ms.NewFundingAgent()},
		ms.NewEnsemble(),
	}
}

// syntheticRandomWalk builds a series with no predictable structure, carrying
// a full taker split and funding so that every agent has its inputs.
func syntheticRandomWalk(n int, seed int64) *ms.Series {
	rng := rand.New(rand.NewSource(seed))
	s := &ms.Series{Symbol: "SYNTH", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	price := 100.0
	for i := 0; i < n; i++ {
		open := price
		price = open * math.Exp(rng.NormFloat64()*0.015)
		wick := math.Abs(rng.NormFloat64()) * open * 0.005
		buy := 1000 * (0.5 + rng.Float64())
		sell := 1000 * (0.5 + rng.Float64())
		s.Candles = append(s.Candles, ms.Candle{
			Time:   t,
			Open:   open,
			High:   math.Max(open, price) + wick,
			Low:    math.Min(open, price) - wick,
			Close:  price,
			Volume: buy + sell, BuyVolume: buy, SellVolume: sell,
		})
		s.Funding = append(s.Funding, 0.0001*rng.NormFloat64())
		t = t.Add(time.Hour)
	}
	return s
}
