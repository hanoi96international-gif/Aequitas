// Command bookd runs a whole book live: several instruments, one allocator,
// one set of weights emitted whenever the universe closes a bar together.
//
// It is the counterpart to signald, which runs one instrument. The difference
// is not scale but timing. An allocator ranks names against each other, so it
// cannot act the moment any single bar closes — it has to wait until every
// name has closed the SAME bar. Ranking one asset's finished hour against
// another's unfinished one produces weights that are all wrong, and the output
// looks entirely normal.
//
//	go run ./cmd/bookd -config config.json
//
// It places no orders. It holds no API key, never authenticates, and contains
// no code path that could reach a venue's trading endpoint.
package main

import (
	"context"
	"encoding/json"
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
	configPath := flag.String("config", "config.json", "path to the config file")
	allocator := flag.String("allocator", "experts", "cross or experts")
	seedBars := flag.Int("seed", 600, "historical bars to load per instrument before starting")
	out := flag.String("out", "", "optional JSONL file to append allocations to")
	pollEvery := flag.Duration("poll", 0, "poll interval (default: a tenth of the bar interval)")
	flag.Parse()

	if err := run(*configPath, *allocator, *out, *seedBars, *pollEvery); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(configPath, allocator, out string, seedBars int, pollEvery time.Duration) error {
	cfg, err := ms.LoadConfig(configPath)
	if err != nil {
		return err
	}
	barDur := cfg.Interval.Duration()
	if pollEvery <= 0 {
		pollEvery = barDur / 10
		if pollEvery < 5*time.Second {
			pollEvery = 5 * time.Second
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := binance.New()
	interval := binanceInterval(barDur)
	if interval == "" {
		return fmt.Errorf("interval %s has no Binance equivalent", barDur)
	}

	// Seed every instrument before deciding anything. A book that starts
	// empty spends its warmup ranking names on almost no history, and the
	// weights it produces meanwhile are not weak — they are arbitrary.
	fmt.Fprintf(os.Stderr, "seeding %d bars for %d instruments...\n", seedBars, len(cfg.Universe))
	start := time.Now().Add(-time.Duration(seedBars+5) * barDur)

	seeded := map[string]*ms.Series{}
	var instruments []ms.Instrument
	feeds := map[string]ms.Feed{}

	for _, ic := range cfg.Universe {
		mkt := binance.Spot
		if ic.HasPerpetual {
			mkt = binance.Futures
		}
		candles, err := client.Klines(ctx, mkt, ic.Symbol, interval, start)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v — dropped from the book\n", ic.Symbol, err)
			continue
		}
		if len(candles) < 2 {
			fmt.Fprintf(os.Stderr, "  %s: only %d bars — dropped\n", ic.Symbol, len(candles))
			continue
		}
		seeded[ic.Symbol] = &ms.Series{Symbol: ic.Symbol, Interval: barDur, Candles: candles}
		instruments = append(instruments, ic.Instrument())
		feeds[ic.Symbol] = binance.PollFeed{
			Client: client, Market: mkt, Symbol: ic.Symbol,
			Interval: interval, Lookback: 5,
		}
		fmt.Fprintf(os.Stderr, "  %-12s %d bars\n", ic.Symbol, len(candles))
	}

	if len(instruments) < 2 {
		return fmt.Errorf("only %d instrument(s) could be seeded; a book needs at least 2",
			len(instruments))
	}

	alloc, err := buildAllocator(allocator, instruments, seeded, cfg)
	if err != nil {
		return err
	}

	book := ms.NewLiveBook(instruments, barDur, alloc)
	for sym, s := range seeded {
		if err := book.Seed(sym, s); err != nil {
			return fmt.Errorf("seed %s: %w", sym, err)
		}
	}

	fmt.Fprintf(os.Stderr, "\n%s over %d instruments, polling every %s\n",
		alloc.Name(), len(instruments), pollEvery)
	fmt.Fprintf(os.Stderr, "allocator warmup: %d bars\n", alloc.Warmup())
	fmt.Fprintln(os.Stderr, "This process places no orders.")

	sinks := []ms.BookSink{ms.BookFuncSink(func(s ms.BookSignal) error {
		fmt.Println(ms.FormatBook(s))
		return nil
	})}
	if out != "" {
		f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		sinks = append(sinks, ms.BookFuncSink(func(s ms.BookSignal) error {
			b, err := json.Marshal(s)
			if err != nil {
				return err
			}
			_, err = f.Write(append(b, '\n'))
			return err
		}))
		fmt.Fprintf(os.Stderr, "appending allocations to %s\n", out)
	}
	fmt.Fprintln(os.Stderr)

	err = book.Run(ctx, feeds, fanout(sinks), pollEvery,
		func(msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format(time.RFC3339), msg) })
	if err == context.Canceled {
		fmt.Fprintln(os.Stderr, "\nstopped.")
		return nil
	}
	return err
}

func buildAllocator(which string, instruments []ms.Instrument,
	seeded map[string]*ms.Series, cfg ms.Config) (ms.Allocator, error) {

	switch which {
	case "cross":
		c := ms.NewCrossSectional()
		c.TargetVol = cfg.Risk.TargetVol
		c.MaxGross, c.MaxNet = cfg.Risk.MaxGross, cfg.Risk.MaxNet
		if len(instruments) < c.MinNames {
			// Lowered rather than refused, but said out loud: a cross-section
			// over a handful of names is a coin flip with extra arithmetic,
			// and the operator should know that is what they are running.
			fmt.Fprintf(os.Stderr,
				"WARNING: %d instruments is below the %d a cross-section needs to mean "+
					"anything; ranking this few is close to a coin flip\n",
				len(instruments), c.MinNames)
			c.MinNames = 2
		}
		return c, nil

	case "experts":
		u := &ms.Universe{Instruments: instruments, Series: seeded}
		p, err := ms.NewExpertPanel(u)
		if err != nil {
			return nil, err
		}
		p.TargetVol = cfg.Risk.TargetVol
		p.MaxGross, p.MaxNet = cfg.Risk.MaxGross, cfg.Risk.MaxNet
		return p, nil
	}
	return nil, fmt.Errorf("unknown allocator %q (want cross or experts)", which)
}

// binanceInterval maps a duration onto Binance's notation.
func binanceInterval(d time.Duration) string {
	for _, s := range []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h",
		"6h", "8h", "12h", "1d", "3d", "1w"} {
		if got, err := binance.ParseInterval(s); err == nil && got == d {
			return s
		}
	}
	return ""
}

func fanout(sinks []ms.BookSink) ms.BookSink {
	return ms.BookFuncSink(func(s ms.BookSignal) error {
		var firstErr error
		for _, sink := range sinks {
			if err := sink.EmitBook(s); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}
