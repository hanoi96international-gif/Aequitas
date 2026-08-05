"""Context filters: where in the range price is, and what time it is.

A sweep and an order block describe *what* happened. These describe *where*
and *when* it happened, which in this methodology matters as much:

  * **Premium / discount.** A dealing range runs from a swing low to a swing
    high, and its midpoint is equilibrium. Selling is meant to happen in the
    upper half and buying in the lower half. A short taken in discount is
    selling into the part of the range that is already cheap — the same
    pattern, at the wrong price.

  * **Sessions.** Liquidity raids cluster around the hours when volume
    arrives. A sweep at 03:00 UTC on a Sunday is usually thin-book drift
    wearing the same shape as the real thing.

Both are off by default. They cut trade counts hard, and a filter that
removes losses on the sample you tuned it on has proven nothing.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone


@dataclass(frozen=True, slots=True)
class DealingRange:
    high: float
    low: float

    @property
    def equilibrium(self) -> float:
        return (self.high + self.low) / 2.0

    @property
    def valid(self) -> bool:
        return self.high > self.low

    def position_of(self, price: float) -> float:
        """Where `price` sits in the range: 0.0 at the low, 1.0 at the high."""
        if not self.valid:
            return 0.5
        return (price - self.low) / (self.high - self.low)

    def allows(self, direction: str, price: float, threshold: float = 0.5) -> bool:
        """Shorts want premium, longs want discount.

        `threshold` is equilibrium by default; raising it demands a deeper
        premium before a short is allowed (and a deeper discount for a long).
        """
        if not self.valid:
            return False
        position = self.position_of(price)
        if direction == "short":
            return position >= threshold
        return position <= 1.0 - threshold


def dealing_range(highs: list[float], lows: list[float], count: int) -> DealingRange | None:
    """Build the range from the most recent `count` confirmed swings each side."""
    if not highs or not lows:
        return None
    recent_highs = highs[-count:]
    recent_lows = lows[-count:]
    return DealingRange(high=max(recent_highs), low=min(recent_lows))


class SessionFilter:
    """Accepts bars whose open time falls inside a configured UTC window."""

    __slots__ = ("_windows", "_days", "enabled")

    def __init__(self, enabled: bool, windows: list[str], days: list[int]) -> None:
        self.enabled = enabled
        self._windows = [self._parse(w) for w in windows]
        self._days = set(days)
        if enabled and not self._windows:
            raise ValueError("filters.session_windows must not be empty when sessions are on")
        if enabled and not self._days:
            raise ValueError("filters.session_days must not be empty when sessions are on")

    @staticmethod
    def _parse(window: str) -> tuple[int, int]:
        """Turn "07:00-10:00" into minutes-from-midnight bounds."""
        try:
            start_text, end_text = window.split("-")
            start_h, start_m = (int(x) for x in start_text.split(":"))
            end_h, end_m = (int(x) for x in end_text.split(":"))
        except ValueError as exc:
            raise ValueError(f"session window {window!r} must look like '07:00-10:00'") from exc
        start = start_h * 60 + start_m
        end = end_h * 60 + end_m
        if not (0 <= start < 24 * 60) or not (0 < end <= 24 * 60):
            raise ValueError(f"session window {window!r} is out of range")
        if start >= end:
            raise ValueError(f"session window {window!r} must start before it ends")
        return start, end

    def allows(self, ts_ms: int) -> bool:
        if not self.enabled:
            return True
        moment = datetime.fromtimestamp(ts_ms / 1000, tz=timezone.utc)
        if moment.weekday() not in self._days:
            return False
        minutes = moment.hour * 60 + moment.minute
        return any(start <= minutes < end for start, end in self._windows)
