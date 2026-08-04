"""Candle sources: local CSV and (optionally) a ccxt exchange.

ccxt is imported lazily so the strategy, the backtester and the whole test
suite run on a bare Python 3.11 with nothing installed.
"""

from __future__ import annotations

import csv
import time
from datetime import datetime, timezone
from pathlib import Path

from .model import Candle, timeframe_ms

_TS_ALIASES = ("timestamp", "ts", "time", "date", "datetime", "open_time")


def load_csv(path: str | Path) -> list[Candle]:
    """Read OHLCV rows, tolerating the header names exchanges actually emit.

    Accepts epoch seconds, epoch milliseconds or ISO-8601 timestamps, and
    sorts by time — an out-of-order file would otherwise produce a backtest
    that quietly trades the future.
    """
    rows: list[Candle] = []
    with Path(path).open(newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None:
            raise ValueError(f"{path}: empty file")
        cols = {name.strip().lower(): name for name in reader.fieldnames}
        ts_col = next((cols[a] for a in _TS_ALIASES if a in cols), None)
        if ts_col is None and reader.fieldnames and not reader.fieldnames[0].strip():
            # A pandas `to_csv` of a time-indexed frame leaves the index column
            # unnamed. That first column is the timestamp.
            ts_col = reader.fieldnames[0]
        if ts_col is None:
            raise ValueError(f"{path}: no timestamp column (looked for {_TS_ALIASES})")
        for needed in ("open", "high", "low", "close"):
            if needed not in cols:
                raise ValueError(f"{path}: missing '{needed}' column")
        vol_col = cols.get("volume")

        for lineno, row in enumerate(reader, start=2):
            try:
                candle = Candle(
                    ts=_parse_ts(row[ts_col]),
                    open=float(row[cols["open"]]),
                    high=float(row[cols["high"]]),
                    low=float(row[cols["low"]]),
                    close=float(row[cols["close"]]),
                    volume=float(row[vol_col]) if vol_col and row[vol_col] else 0.0,
                )
            except (TypeError, ValueError) as exc:
                raise ValueError(f"{path}:{lineno}: {exc}") from exc
            rows.append(candle)
    rows.sort(key=lambda c: c.ts)
    return rows


def _parse_ts(raw: str) -> int:
    value = raw.strip()
    if not value:
        raise ValueError("empty timestamp")
    try:
        number = float(value)
    except ValueError:
        text = value.replace("Z", "+00:00")
        dt = datetime.fromisoformat(text)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp() * 1000)
    # Seconds and milliseconds are told apart by magnitude: anything below
    # ~Nov 2286 in ms would be an implausible date if read as seconds.
    return int(number if number > 1e11 else number * 1000)


def save_csv(path: str | Path, candles: list[Candle]) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["timestamp", "open", "high", "low", "close", "volume"])
        for c in candles:
            writer.writerow([c.ts, c.open, c.high, c.low, c.close, c.volume])


def fetch_ohlcv(
    exchange_id: str,
    symbol: str,
    timeframe: str,
    limit: int,
    until_ms: int | None = None,
) -> list[Candle]:
    """Page backwards through an exchange's public OHLCV endpoint.

    The final (still forming) candle is dropped: acting on a bar that has not
    closed is the single easiest way to make a backtest unreproducible.
    """
    try:
        import ccxt  # type: ignore[import-untyped]
    except ImportError as exc:  # pragma: no cover - depends on environment
        raise RuntimeError(
            "ccxt is not installed — `pip install ccxt`, or set data.csv to "
            "run against a local file instead"
        ) from exc

    if not hasattr(ccxt, exchange_id):
        raise ValueError(f"unknown exchange {exchange_id!r}")
    exchange = getattr(ccxt, exchange_id)({"enableRateLimit": True})
    step = timeframe_ms(timeframe)
    end = until_ms if until_ms is not None else int(time.time() * 1000)

    collected: dict[int, Candle] = {}
    cursor = end - limit * step
    while len(collected) < limit:
        batch = exchange.fetch_ohlcv(symbol, timeframe, since=cursor, limit=1000)
        if not batch:
            break
        for ts, o, h, l, c, v in batch:
            if ts < end:
                collected[ts] = Candle(int(ts), float(o), float(h), float(l), float(c), float(v))
        advanced = batch[-1][0] + step
        if advanced <= cursor:
            break
        cursor = advanced
        if cursor >= end:
            break

    candles = [collected[k] for k in sorted(collected)]
    # Guard against a venue returning an unclosed bar despite the `< end` filter.
    now = int(time.time() * 1000)
    while candles and candles[-1].ts + step > now:
        candles.pop()
    return candles[-limit:]


def cached_ohlcv(
    cache_dir: str | Path,
    exchange_id: str,
    symbol: str,
    timeframe: str,
    limit: int,
    refresh: bool = False,
) -> list[Candle]:
    """Fetch via `fetch_ohlcv`, reusing a local CSV when one is good enough."""
    safe_symbol = symbol.replace("/", "-").replace(":", "-")
    path = Path(cache_dir) / f"{exchange_id}_{safe_symbol}_{timeframe}.csv"
    if path.exists() and not refresh:
        cached = load_csv(path)
        if len(cached) >= limit:
            return cached[-limit:]
    candles = fetch_ohlcv(exchange_id, symbol, timeframe, limit)
    if candles:
        save_csv(path, candles)
    return candles
