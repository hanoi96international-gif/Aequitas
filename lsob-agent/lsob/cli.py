"""Command line: backtest, scan, fetch, paper, live."""

from __future__ import annotations

import argparse
import csv
import logging
import sys
from datetime import datetime, timezone
from pathlib import Path

from .backtest import run_backtest, scan_signals
from .config import Config, load_config
from .data import cached_ohlcv, load_csv, save_csv
from .model import Candle


def _iso(ts: int) -> str:
    return datetime.fromtimestamp(ts / 1000, tz=timezone.utc).strftime("%Y-%m-%d %H:%M")


def _load_candles(cfg: Config, refresh: bool = False) -> list[Candle]:
    if cfg.data.csv:
        candles = load_csv(cfg.data.csv)
        return candles[-cfg.data.history_bars :] if cfg.data.history_bars > 0 else candles
    return cached_ohlcv(
        cfg.data.cache_dir,
        cfg.market.exchange,
        cfg.market.symbol,
        cfg.market.timeframe,
        cfg.data.history_bars,
        refresh=refresh,
    )


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
    signals = scan_signals(cfg, candles)
    if not signals:
        print("\nNo setups found. Loosen orderblock.displacement_atr or entry.min_rr.")
        return 0
    print(f"\n{len(signals)} setups\n")
    print(f"{'time (UTC)':<18}{'dir':<7}{'entry':>13}{'stop':>13}{'R':>7}{'disp':>7}  targets")
    for s in signals[-args.limit :]:
        targets = ", ".join(f"{t:.6g}" for t in s.targets)
        print(
            f"{_iso(s.ts):<18}{s.direction:<7}{s.entry:>13.6g}{s.stop:>13.6g}"
            f"{s.reward_risk:>7.2f}{s.displacement:>7.2f}  {targets}"
        )
    return 0


def cmd_fetch(cfg: Config, args: argparse.Namespace) -> int:
    from .data import fetch_ohlcv

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

    fetch = sub.add_parser("fetch", help="download candles to CSV")
    fetch.add_argument("--bars", type=int, help="how many bars (default data.history_bars)")
    fetch.add_argument("--out", help="output CSV path")
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
