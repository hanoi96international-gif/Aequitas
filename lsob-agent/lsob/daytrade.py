"""Day-trading rules: the clock, not the chart.

A swing trader exits when the idea is finished. A day trader exits when the
day is finished, whether the idea is or not — that is the whole difference,
and it is a constraint the strategy never sees. Three rules encode it:

  * **flat_at** — every open position is closed at the last bar of the
    session, at that bar's close. No overnight gap risk, no funding, no
    weekend.
  * **no_entry_after** — a fill twenty minutes before the flatten is not a
    trade, it is a coin toss with a deadline. New fills stop earlier than
    the flatten does.
  * **max_bars_in_trade** — a setup that has neither paid nor failed within
    its expected window has been invalidated by time. The stop is a price;
    this is the other axis.

Both cutoffs are wall-clock times in **UTC**, and the trading day is the UTC
calendar day. If your session is CME or a broker day that rolls at 22:00,
express the window in UTC rather than expecting the agent to know about it.

One detail worth stating because it decides where the flatten lands: a bar is
stamped with its *open* time, so `flat_at = "22:00"` on hourly candles has to
fire on the bar opening at 21:00 — the one whose close *is* 22:00. The test
is therefore against the bar's close time, not its open. Choose a cutoff that
falls on a boundary of your timeframe; anything else rounds outward to the
first bar that ends at or after it, which is late by up to one bar and says
so here rather than surprising you in the trade list.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from .model import timeframe_ms

if TYPE_CHECKING:  # pragma: no cover - import cycle otherwise
    from .config import Config

_DAY_MS = 86_400_000
_DAY_MINUTES = 24 * 60


def parse_hhmm(text: str, label: str) -> int:
    """Turn "21:55" into minutes from midnight. "24:00" is end of day."""
    try:
        hours, minutes = (int(part) for part in text.split(":"))
    except ValueError as exc:
        raise ValueError(f"{label} must look like '21:55' (UTC), got {text!r}") from exc
    if not 0 <= minutes < 60:
        raise ValueError(f"{label} has no such minute: {text!r}")
    total = hours * 60 + minutes
    if not 0 <= total <= _DAY_MINUTES:
        raise ValueError(f"{label} is out of range: {text!r}")
    return total


class DayClock:
    """Answers the three time questions the executor has to ask each bar."""

    __slots__ = ("enabled", "_flat", "_cutoff", "_tf_minutes", "max_bars", "max_trades")

    def __init__(
        self,
        enabled: bool,
        flat_at: str,
        no_entry_after: str,
        max_bars_in_trade: int,
        max_trades_per_day: int,
        timeframe_ms: int,
    ) -> None:
        self.enabled = enabled
        self._flat = parse_hhmm(flat_at, "daytrade.flat_at") if flat_at else None
        self._cutoff = (
            parse_hhmm(no_entry_after, "daytrade.no_entry_after") if no_entry_after else None
        )
        self._tf_minutes = max(1, timeframe_ms // 60_000)
        self.max_bars = max_bars_in_trade
        self.max_trades = max_trades_per_day

    @staticmethod
    def day_key(ts_ms: int) -> int:
        """The UTC calendar day a bar belongs to."""
        return ts_ms // _DAY_MS

    def _minutes(self, ts_ms: int) -> int:
        return (ts_ms % _DAY_MS) // 60_000

    def must_flatten(self, ts_ms: int) -> bool:
        """Is this the last bar of the session — the one to be flat on?

        Measured against the bar's close, so a cutoff that sits on a bar
        boundary fires on the bar that ends there rather than the one after.
        """
        if not self.enabled or self._flat is None:
            return False
        return self._minutes(ts_ms) + self._tf_minutes >= self._flat

    def entries_closed(self, ts_ms: int) -> bool:
        """Too late in the day to start something new?"""
        if not self.enabled:
            return False
        if self._cutoff is not None and self._minutes(ts_ms) >= self._cutoff:
            return True
        # Without an explicit cutoff, the flatten itself is the deadline:
        # filling on the bar you are about to close out is not a trade.
        return self.must_flatten(ts_ms)

    def timed_out(self, bars_held: int) -> bool:
        return bool(self.enabled and self.max_bars and bars_held >= self.max_bars)

    def day_full(self, taken_today: int) -> bool:
        return bool(self.enabled and self.max_trades and taken_today >= self.max_trades)


def day_clock(cfg: Config) -> DayClock:
    """Build the clock the backtest and the live broker both run on.

    One factory, so a rule can never mean one thing in the backtest and
    another in the account.
    """
    try:
        tf_ms = timeframe_ms(cfg.market.timeframe)
    except ValueError:  # an unparsable timeframe only matters once the clock runs
        tf_ms = 60_000
    d = cfg.daytrade
    return DayClock(
        d.enabled, d.flat_at, d.no_entry_after, d.max_bars_in_trade, d.max_trades_per_day, tf_ms
    )
