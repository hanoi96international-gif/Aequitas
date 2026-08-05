"""Command line: backtest, scan, fetch, paper, live."""

from __future__ import annotations

import argparse
import csv
import logging
import sys
from datetime import datetime, timezone
from pathlib import Path

from .backtest import run_backtest, scan_signals
from .strategy import LsobStrategy
from .config import Config, load_config
from .data import audit_candles, cached_ohlcv, clean_candles, load_any, save_csv
from .model import Candle, timeframe_ms


def _iso(ts: int) -> str:
    return datetime.fromtimestamp(ts / 1000, tz=timezone.utc).strftime("%Y-%m-%d %H:%M")


def _load_candles(cfg: Config, refresh: bool = False) -> list[Candle]:
    if cfg.data.csv:
        candles = load_any(cfg.data.csv)
    else:
        candles = cached_ohlcv(
            cfg.data.cache_dir,
            cfg.market.exchange,
            cfg.market.symbol,
            cfg.market.timeframe,
            cfg.data.history_bars,
            refresh=refresh,
        )

    audit = audit_candles(candles, cfg.data.spike_ratio, cfg.data.jump_ratio)

    # A timeframe that disagrees with the data is silent and expensive.
    # Every higher-timeframe feature — the bias filter, the ladder's anchor —
    # multiplies `market.timeframe`, so a config saying 15m over hourly
    # candles turns "4h" into 1h without anything looking wrong.
    if audit.interval_ms:
        try:
            configured = timeframe_ms(cfg.market.timeframe)
        except ValueError:
            configured = 0
        if configured and configured != audit.interval_ms:
            print(
                f"WARNING: market.timeframe is {cfg.market.timeframe} but the candles are "
                f"{_describe_interval(audit.interval_ms)} apart. Higher-timeframe settings "
                f"multiply the configured value, so they are currently off by "
                f"{audit.interval_ms / configured:.3g}x.",
                file=sys.stderr,
            )

    if not audit.clean:
        print(f"Data integrity: {audit.format()}", file=sys.stderr)
        if cfg.data.clean:
            before = len(candles)
            candles = clean_candles(candles, cfg.data.spike_ratio, cfg.data.jump_ratio)
            print(f"  dropped {before - len(candles)} unusable bars", file=sys.stderr)

    if cfg.data.history_bars > 0:
        candles = candles[-cfg.data.history_bars :]
    return candles


def _describe_interval(ms: int) -> str:
    for unit, size in (("d", 86_400_000), ("h", 3_600_000), ("m", 60_000)):
        if ms % size == 0:
            return f"{ms // size}{unit}"
    return f"{ms}ms"


def _describe(cfg: Config, candles: list[Candle]) -> str:
    source = cfg.data.csv or f"{cfg.market.exchange}:{cfg.market.symbol}"
    if not candles:
        return f"{source} {cfg.market.timeframe} — no candles"
    return (
        f"{source} {cfg.market.timeframe} — {len(candles)} bars, "
        f"{_iso(candles[0].ts)} to {_iso(candles[-1].ts)} UTC"
    )


def cmd_backtest(cfg: Config, args: argparse.Namespace) -> int:
    candles = _load_candles(cfg, refresh=args.refresh)
    if not candles:
        print("No candles loaded.", file=sys.stderr)
        return 1
    print(_describe(cfg, candles))
    result = run_backtest(cfg, candles)
    print()
    print(result.format())

    if args.trades:
        print("\nTrades")
        print(f"{'dir':<6}{'entry':>14}{'exit':>14}{'R':>8}{'pnl':>12}  reason")
        for t in result.trades:
            print(
                f"{t.direction:<6}{t.entry_price:>14.6g}{t.exit_price:>14.6g}"
                f"{t.r_multiple:>8.2f}{t.pnl:>12.2f}  {t.exit_reason}"
            )
    if args.equity_csv:
        path = Path(args.equity_csv)
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["bar", "timestamp", "equity"])
            for bar, equity in result.equity_curve:
                writer.writerow([bar, candles[bar].ts, f"{equity:.8f}"])
        print(f"\nEquity curve written to {path}")
    return 0


def cmd_scan(cfg: Config, args: argparse.Namespace) -> int:
    candles = _load_candles(cfg, refresh=args.refresh)
    if not candles:
        print("No candles loaded.", file=sys.stderr)
        return 1
    print(_describe(cfg, candles))
    strategy = LsobStrategy(cfg)
    signals = []
    for candle in candles:
        signals.extend(strategy.on_bar(candle))
    if not signals:
        print("\nNo setups found.")
        if strategy.rejections:
            print("Setups were built and then dropped by:")
            for reason, count in sorted(strategy.rejections.items(), key=lambda kv: -kv[1]):
                print(f"  {reason}: {count}")
        else:
            print("Nothing even reached the order-block stage — loosen")
            print("orderblock.displacement_atr or liquidity.max_penetration_atr.")
        return 0
    print(f"\n{len(signals)} setups")
    if strategy.rejections:
        dropped = ", ".join(
            f"{k}={v}" for k, v in sorted(strategy.rejections.items(), key=lambda kv: -kv[1])
        )
        print(f"({dropped})")
    print()
    print(f"{'time (UTC)':<18}{'dir':<7}{'entry':>13}{'stop':>13}{'R':>7}{'disp':>7}  targets")
    for s in signals[-args.limit :]:
        targets = ", ".join(f"{t:.6g}" for t in s.targets)
        print(
            f"{_iso(s.ts):<18}{s.direction:<7}{s.entry:>13.6g}{s.stop:>13.6g}"
            f"{s.reward_risk:>7.2f}{s.displacement:>7.2f}  {targets}"
        )
    return 0


def cmd_chart(cfg: Config, args: argparse.Namespace) -> int:
    from .chart import fib_ladder, levels_for, render_svg, window_around

    candles = _load_candles(cfg, refresh=False)
    if not candles:
        print("No candles loaded.", file=sys.stderr)
        return 1
    signals = scan_signals(cfg, candles)
    if not signals:
        print("No setups to draw.", file=sys.stderr)
        return 1

    chosen = signals[args.index] if -len(signals) <= args.index < len(signals) else signals[-1]
    window = window_around(candles, chosen, args.before, args.after)
    title = (
        f"{cfg.market.symbol} {cfg.market.timeframe} — {chosen.direction.upper()} "
        f"{_iso(chosen.ts)} UTC · {chosen.reward_risk:.2f}R"
    )
    marks = levels_for(chosen)
    if args.fib:
        marks += fib_ladder(chosen, list(cfg.entry.fib_levels))
    svg = render_svg(window, chosen, title, marks, theme=args.theme)
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(svg, encoding="utf-8")
    print(f"{len(signals)} setups; drew #{args.index} -> {out}")
    return 0


def cmd_risk(cfg: Config, args: argparse.Namespace) -> int:
    from .sizing import format_sweep, sweep

    candles = _load_candles(cfg, refresh=False)
    if not candles:
        print("No candles loaded.", file=sys.stderr)
        return 1
    result = run_backtest(cfg, candles)
    if not result.trades:
        print("The backtest produced no trades, so there is nothing to resample.", file=sys.stderr)
        return 1

    rs = [t.r_multiple for t in result.trades]
    levels = [float(x) for x in args.levels.split(",")]
    print(_describe(cfg, candles))
    print()
    print(format_sweep(sweep(rs, levels, trades=args.trades, runs=args.runs), len(rs)))
    return 0


def cmd_walkforward(cfg: Config, args: argparse.Namespace) -> int:
    from .walkforward import expand_grid, walk_forward

    candles = _load_candles(cfg, refresh=args.refresh)
    if not candles:
        print("No candles loaded.", file=sys.stderr)
        return 1
    wf = cfg.walkforward
    if not wf.grid:
        print(
            "walkforward.grid is empty — add the parameters to search, e.g.\n"
            '  [walkforward.grid]\n'
            '  "orderblock.displacement_atr" = [0.5, 1.0, 1.5]',
            file=sys.stderr,
        )
        return 1

    needed = wf.train_bars + wf.test_bars
    if len(candles) < needed:
        print(
            f"Need at least {needed} bars for one fold, have {len(candles)}.", file=sys.stderr
        )
        return 1

    print(_describe(cfg, candles))
    runs = len(expand_grid(wf.grid)) * 2 * (1 + (len(candles) - needed) // wf.test_bars)
    print(f"Searching {len(expand_grid(wf.grid))} parameter sets ({runs} backtests)...\n")

    result = walk_forward(
        cfg,
        candles,
        wf.grid,
        train_bars=wf.train_bars,
        test_bars=wf.test_bars,
        min_trades=wf.min_trades,
        metric=wf.metric,
        selection=args.selection or wf.selection,
    )
    print(result.format())

    if args.params:
        print("\nWinning parameters per fold")
        for fold in result.folds:
            chosen = ", ".join(f"{k}={v}" for k, v in sorted(fold.params.items()))
            print(f"  fold {fold.index}: {chosen}")
    return 0


def cmd_fetch(cfg: Config, args: argparse.Namespace) -> int:
    from .data import fetch_ohlcv

    if args.months:
        return _fetch_archive(cfg, args)

    bars = args.bars or cfg.data.history_bars
    candles = fetch_ohlcv(cfg.market.exchange, cfg.market.symbol, cfg.market.timeframe, bars)
    if not candles:
        print("Exchange returned no candles.", file=sys.stderr)
        return 1
    out = Path(args.out) if args.out else Path(cfg.data.cache_dir) / (
        f"{cfg.market.exchange}_{cfg.market.symbol.replace('/', '-')}_{cfg.market.timeframe}.csv"
    )
    save_csv(out, candles)
    print(f"{len(candles)} bars ({_iso(candles[0].ts)} to {_iso(candles[-1].ts)} UTC) -> {out}")
    return 0


def _fetch_archive(cfg: Config, args: argparse.Namespace) -> int:
    """Pull whole months from Binance's public archive — no API key needed."""
    from .data import download_archive, load_klines, months_between

    start, _, end = args.months.partition(":")
    months = months_between(start, end or start)
    cache = Path(cfg.data.cache_dir)
    candles: list[Candle] = []
    for month in months:
        path = cache / Path(
            f"{cfg.market.symbol.replace('/', '')}-{cfg.market.timeframe}-{month}.zip"
        ).name
        if not path.exists():
            print(f"  downloading {month} ...")
            path = download_archive(
                cfg.market.symbol, cfg.market.timeframe, month, cache
            )
        candles.extend(load_klines(path))

    if not candles:
        print("Archive returned no candles.", file=sys.stderr)
        return 1
    candles.sort(key=lambda c: c.ts)
    deduped = {c.ts: c for c in candles}
    candles = [deduped[k] for k in sorted(deduped)]

    out = Path(args.out) if args.out else cache / (
        f"binance_{cfg.market.symbol.replace('/', '-')}_{cfg.market.timeframe}.csv"
    )
    save_csv(out, candles)
    print(
        f"{len(candles)} bars ({_iso(candles[0].ts)} to {_iso(candles[-1].ts)} UTC) -> {out}\n"
        f"Set data.csv = \"{out}\" in your config to backtest it."
    )
    return 0


def cmd_run(cfg: Config, args: argparse.Namespace) -> int:
    from .agent import Agent
    from .broker import LiveBroker, PaperBroker

    if not cfg.live.enabled:
        print("live.enabled is false in the config — nothing to run.", file=sys.stderr)
        return 1

    if args.live:
        broker = LiveBroker(cfg, confirmed=True)
        if not cfg.live.sandbox:
            print("!! REAL MONEY: live.sandbox is false.", file=sys.stderr)
            if input("Type 'yes, real money' to continue: ").strip() != "yes, real money":
                print("Aborted.", file=sys.stderr)
                return 1
    else:
        broker = PaperBroker(cfg)

    Agent(cfg, broker, warmup_bars=cfg.data.history_bars).run()
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="lsob", description="LSOB trade agent")
    parser.add_argument("-c", "--config", default="config.toml", help="path to the TOML config")
    parser.add_argument("-v", "--verbose", action="store_true", help="debug logging")
    sub = parser.add_subparsers(dest="command", required=True)

    bt = sub.add_parser("backtest", help="run the strategy over historical candles")
    bt.add_argument("--trades", action="store_true", help="print every trade")
    bt.add_argument("--equity-csv", help="write the equity curve to this file")
    bt.add_argument("--refresh", action="store_true", help="ignore the candle cache")
    bt.set_defaults(func=cmd_backtest)

    scan = sub.add_parser("scan", help="list detected setups without trading them")
    scan.add_argument("--limit", type=int, default=50, help="show at most N most recent setups")
    scan.add_argument("--refresh", action="store_true", help="ignore the candle cache")
    scan.set_defaults(func=cmd_scan)

    chart = sub.add_parser("chart", help="draw a setup and its levels as SVG")
    chart.add_argument("--index", type=int, default=-1, help="which setup (-1 = most recent)")
    chart.add_argument("--before", type=int, default=45, help="bars of context before the signal")
    chart.add_argument("--after", type=int, default=25, help="bars after it")
    chart.add_argument("--out", default="chart.svg", help="output SVG path")
    chart.add_argument("--theme", choices=["light", "dark"], default="light")
    chart.add_argument(
        "--fib", action="store_true", help="draw the full retracement ladder"
    )
    chart.set_defaults(func=cmd_chart)

    risk = sub.add_parser("risk", help="what each risk-per-trade level does to drawdown")
    risk.add_argument(
        "--levels", default="0.25,0.5,1.0,2.0,3.0", help="risk-per-trade percentages to compare"
    )
    risk.add_argument("--trades", type=int, default=200, help="trades per simulated run")
    risk.add_argument("--runs", type=int, default=5000, help="number of simulated runs")
    risk.set_defaults(func=cmd_risk)

    wf = sub.add_parser(
        "walkforward", help="optimise on past windows, score on the windows that follow"
    )
    wf.add_argument("--params", action="store_true", help="print the winner of each fold")
    wf.add_argument(
        "--selection",
        choices=["robust", "peak"],
        help="robust prefers a broad plateau; peak takes the single best score",
    )
    wf.add_argument("--refresh", action="store_true", help="ignore the candle cache")
    wf.set_defaults(func=cmd_walkforward)

    fetch = sub.add_parser("fetch", help="download candles to CSV")
    fetch.add_argument("--bars", type=int, help="how many bars (default data.history_bars)")
    fetch.add_argument("--out", help="output CSV path")
    fetch.add_argument(
        "--months",
        help="pull whole months from Binance's public archive instead of the REST "
        "API — no API key needed. Format: YYYY-MM or YYYY-MM:YYYY-MM",
    )
    fetch.set_defaults(func=cmd_fetch)

    run = sub.add_parser("run", help="run the agent (paper unless --live)")
    run.add_argument(
        "--live",
        action="store_true",
        help="send real orders; also requires live.enabled and live.mode='live'",
    )
    run.set_defaults(func=cmd_run)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)-7s %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    try:
        cfg = load_config(args.config)
    except FileNotFoundError:
        print(f"Config not found: {args.config}", file=sys.stderr)
        return 1
    except ValueError as exc:
        print(f"Invalid config: {exc}", file=sys.stderr)
        return 1

    try:
        return args.func(cfg, args)
    except KeyboardInterrupt:
        return 130
    except (RuntimeError, ValueError) as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
