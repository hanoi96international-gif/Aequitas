"""Candle sources: local CSV and (optionally) a ccxt exchange.

ccxt is imported lazily so the strategy, the backtester and the whole test
suite run on a bare Python 3.11 with nothing installed.
"""

from __future__ import annotations

import csv
import math
import time
from dataclasses import dataclass
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


@dataclass(slots=True)
class CandleAudit:
    """What is wrong with a candle series, counted rather than assumed."""

    total: int = 0
    non_finite: int = 0  # inf/NaN, or a sentinel standing in for missing data
    impossible: int = 0  # high below the body, or low above it
    non_positive: int = 0
    spikes: int = 0  # single bars with an implausible high/low ratio
    jumps: int = 0  # bars disconnected from the previous good close
    duplicate_ts: int = 0
    gaps: int = 0  # missing bars, at the series' own dominant interval

    @property
    def bad_bars(self) -> int:
        return self.non_finite + self.impossible + self.non_positive + self.spikes + self.jumps

    @property
    def clean(self) -> bool:
        return not (self.bad_bars or self.duplicate_ts)

    def format(self) -> str:
        if self.clean and not self.gaps:
            return f"{self.total} candles, no integrity problems found."
        parts = [f"{self.total} candles"]
        for count, label in (
            (self.non_finite, "non-finite or sentinel"),
            (self.impossible, "structurally impossible"),
            (self.non_positive, "non-positive prices"),
            (self.spikes, "intra-bar spikes"),
            (self.jumps, "disconnected from the previous close"),
            (self.duplicate_ts, "duplicate timestamps"),
            (self.gaps, "missing bars"),
        ):
            if count:
                parts.append(f"{count} {label}")
        return ", ".join(parts)


# Anything at or above this is not a price any market printed; the public
# BTC/USD series that motivated these checks fills its gaps with 1.7e308.
_ABSURD_PRICE = 1e12


def _defect(candle: Candle, previous_close: float | None, spike_ratio: float, jump_ratio: float) -> str | None:
    """Classify one bar, or return None if it is usable."""
    prices = (candle.open, candle.high, candle.low, candle.close)
    if not all(math.isfinite(p) for p in prices) or max(prices) >= _ABSURD_PRICE:
        return "non_finite"
    if min(prices) <= 0:
        return "non_positive"
    if candle.high < max(candle.open, candle.close) or candle.low > min(candle.open, candle.close):
        return "impossible"
    if candle.high / candle.low > spike_ratio:
        return "spikes"
    if previous_close is not None and previous_close > 0:
        move = candle.close / previous_close
        if move > jump_ratio or move < 1.0 / jump_ratio:
            return "jumps"
    return None


def audit_candles(
    candles: list[Candle], spike_ratio: float = 10.0, jump_ratio: float = 10.0
) -> CandleAudit:
    """Look for the defects real market data actually contains.

    Two kinds, and the second is why an intra-bar check is not enough. A
    public BTC/USD series used here held a bar with a low of 1.50 against a
    close of 581 — caught by the high/low ratio. It also filled 11% of its
    rows with 1.7e308 in *every* column, so open, high, low and close agreed
    perfectly and the ratio was exactly 1.00. Only comparison with the
    previous good close exposes those.

    Both matter because ATR is built from true range: one bar like either of
    these poisons every ATR-scaled threshold for `atr_period` bars after it,
    and the strategy then silently stops producing signals at all.
    """
    audit = CandleAudit(total=len(candles))
    previous_close: float | None = None
    for candle in candles:
        defect = _defect(candle, previous_close, spike_ratio, jump_ratio)
        if defect is None:
            previous_close = candle.close
        else:
            setattr(audit, defect, getattr(audit, defect) + 1)

    steps: dict[int, int] = {}
    deltas = [later.ts - earlier.ts for earlier, later in zip(candles, candles[1:], strict=False)]
    for delta in deltas:
        if delta == 0:
            audit.duplicate_ts += 1
        elif delta > 0:
            steps[delta] = steps.get(delta, 0) + 1
    if steps:
        interval = max(steps, key=lambda k: steps[k])
        audit.gaps = sum((d // interval) - 1 for d in deltas if d > interval)
    return audit


def clean_candles(
    candles: list[Candle], spike_ratio: float = 10.0, jump_ratio: float = 10.0
) -> list[Candle]:
    """Drop unusable bars and collapse duplicate timestamps.

    Gaps are left alone. A missing hour is information, and interpolating one
    would invent price action the market never printed.
    """
    kept: dict[int, Candle] = {}
    previous_close: float | None = None
    for candle in candles:
        if _defect(candle, previous_close, spike_ratio, jump_ratio) is not None:
            continue
        previous_close = candle.close
        kept[candle.ts] = candle  # a later duplicate wins
    return [kept[ts] for ts in sorted(kept)]


def load_klines(path: str | Path) -> list[Candle]:
    """Read Binance's public archive format — no API key, no rate limit.

    The monthly and daily dumps at data.binance.vision are plain CSV with no
    header, twelve columns per row, of which the first six are the candle.
    Newer files ship *with* a header, so both are accepted. `.zip` archives
    are read in place; there is no need to unpack them first.

    This exists because the REST endpoint is the awkward way to get years of
    history: it pages, it rate-limits, and it needs the network to stay up
    for the whole walk. The archive is one file per month.
    """
    target = Path(path)
    if target.suffix.lower() == ".zip":
        import zipfile

        with zipfile.ZipFile(target) as archive:
            names = [n for n in archive.namelist() if n.lower().endswith(".csv")]
            if not names:
                raise ValueError(f"{path}: archive contains no CSV")
            with archive.open(names[0]) as fh:
                text = fh.read().decode("utf-8")
    else:
        text = target.read_text(encoding="utf-8")

    candles: list[Candle] = []
    for lineno, line in enumerate(text.splitlines(), start=1):
        line = line.strip()
        if not line:
            continue
        parts = line.split(",")
        if len(parts) < 6:
            raise ValueError(f"{path}:{lineno}: expected at least 6 columns, got {len(parts)}")
        try:
            candles.append(
                Candle(
                    ts=_parse_ts(parts[0]),
                    open=float(parts[1]),
                    high=float(parts[2]),
                    low=float(parts[3]),
                    close=float(parts[4]),
                    volume=float(parts[5]),
                )
            )
        except ValueError:
            if lineno == 1:
                continue  # a header row on a newer dump
            raise ValueError(f"{path}:{lineno}: could not parse candle") from None
    candles.sort(key=lambda c: c.ts)
    return candles


def load_any(path: str | Path) -> list[Candle]:
    """Load candles from whichever of the supported layouts `path` holds.

    Tries the labelled-CSV reader first and falls back to the headerless
    Binance archive layout, so `data.csv` in the config accepts either
    without the operator having to say which one it is.
    """
    target = Path(path)
    if target.suffix.lower() == ".zip":
        return load_klines(target)
    try:
        return load_csv(target)
    except ValueError:
        return load_klines(target)


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
    # Units are told apart by magnitude. A contemporary date is ~1.7e9 in
    # seconds, ~1.7e12 in milliseconds and ~1.7e15 in microseconds, so the
    # gaps between them are three orders wide and unambiguous in practice.
    # Binance's archive switched from milliseconds to microseconds, which is
    # exactly the silent off-by-1000 this guards against.
    if number > 1e14:
        return int(number / 1000)
    if number > 1e11:
        return int(number)
    return int(number * 1000)


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


ARCHIVE_BASE = "https://data.binance.vision/data/spot/monthly/klines"


def archive_url(symbol: str, timeframe: str, month: str) -> str:
    """Build the public-archive URL for one month of candles.

    `symbol` is given in ccxt form ("BTC/USDT"); the archive uses the
    unslashed form. `month` is "YYYY-MM".
    """
    pair = symbol.replace("/", "").replace(":", "").upper()
    return f"{ARCHIVE_BASE}/{pair}/{timeframe}/{pair}-{timeframe}-{month}.zip"


def download_archive(symbol: str, timeframe: str, month: str, dest_dir: str | Path) -> Path:
    """Fetch one monthly archive to disk and return its path.

    Uses urllib rather than ccxt: this endpoint is a static file server, so
    it needs no API key, no signing and no rate limiting.
    """
    import urllib.error
    import urllib.request

    url = archive_url(symbol, timeframe, month)
    target = Path(dest_dir) / Path(url).name
    target.parent.mkdir(parents=True, exist_ok=True)
    try:
        with urllib.request.urlopen(url, timeout=120) as response:
            target.write_bytes(response.read())
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            raise RuntimeError(
                f"no archive for {symbol} {timeframe} {month} — check the symbol, "
                f"the timeframe, and that the month has finished ({url})"
            ) from exc
        raise RuntimeError(f"archive download failed ({exc.code}): {url}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"could not reach the Binance archive: {exc.reason}") from exc
    return target


def months_between(start: str, end: str) -> list[str]:
    """Every "YYYY-MM" from `start` to `end` inclusive."""
    try:
        start_year, start_month = (int(x) for x in start.split("-"))
        end_year, end_month = (int(x) for x in end.split("-"))
    except ValueError as exc:
        raise ValueError("months must look like 'YYYY-MM'") from exc
    if not (1 <= start_month <= 12 and 1 <= end_month <= 12):
        raise ValueError("month must be 01-12")
    if (start_year, start_month) > (end_year, end_month):
        raise ValueError(f"{start} is after {end}")

    out: list[str] = []
    year, month = start_year, start_month
    while (year, month) <= (end_year, end_month):
        out.append(f"{year:04d}-{month:02d}")
        month += 1
        if month > 12:
            year, month = year + 1, 1
    return out


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
