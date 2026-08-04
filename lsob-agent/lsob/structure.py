"""Swing points and market structure — the skeleton the LSOB setup hangs on.

A swing high at bar p is only *knowable* at bar p + right. Detectors here
never report a swing before that bar, which is what keeps the backtest
honest: no rule can consult a pivot the live agent would not yet have seen.
"""

from __future__ import annotations

from collections import deque
from dataclasses import dataclass

from .model import Candle


@dataclass(frozen=True, slots=True)
class Swing:
    index: int  # bar the pivot sits on
    ts: int
    price: float
    kind: str  # 'high' | 'low'
    confirmed_at: int  # bar at which the pivot became knowable


class SwingDetector:
    """Fractal pivot detector with `left`/`right` confirmation windows.

    Ties are resolved in favour of the earliest bar (strict on the left,
    non-strict on the right), so a plateau of equal highs yields exactly one
    pivot instead of one per bar.
    """

    __slots__ = ("left", "right", "_buf", "_seen")

    def __init__(self, left: int, right: int) -> None:
        if left < 0 or right < 1:
            raise ValueError("swing_left must be >= 0 and swing_right >= 1")
        self.left = left
        self.right = right
        self._buf: deque[Candle] = deque(maxlen=left + right + 1)
        self._seen = 0

    def update(self, candle: Candle) -> list[Swing]:
        """Feed one bar; return the swings confirmed *by* this bar (0, 1 or 2)."""
        self._buf.append(candle)
        self._seen += 1
        if len(self._buf) < self._buf.maxlen:
            return []

        bars = list(self._buf)
        pivot = bars[self.left]
        left_bars = bars[: self.left]
        right_bars = bars[self.left + 1 :]
        pivot_index = self._seen - 1 - self.right
        confirmed_at = self._seen - 1

        out: list[Swing] = []
        if all(pivot.high > b.high for b in left_bars) and all(
            pivot.high >= b.high for b in right_bars
        ):
            out.append(Swing(pivot_index, pivot.ts, pivot.high, "high", confirmed_at))
        if all(pivot.low < b.low for b in left_bars) and all(
            pivot.low <= b.low for b in right_bars
        ):
            out.append(Swing(pivot_index, pivot.ts, pivot.low, "low", confirmed_at))
        return out


@dataclass(frozen=True, slots=True)
class StructureBreak:
    index: int
    ts: int
    direction: str  # 'up' | 'down'
    level: float  # the swing level that was broken
    choch: bool  # True when the break flipped the prevailing trend


class MarketStructure:
    """Tracks confirmed swings and reports breaks of structure.

    `history` is bounded — a live agent runs for weeks and an unbounded list
    of every pivot it ever saw is a slow leak with no upside.
    """

    __slots__ = ("_det", "_use_close", "highs", "lows", "trend", "_max_history")

    def __init__(self, left: int, right: int, use_close: bool = True, max_history: int = 64) -> None:
        self._det = SwingDetector(left, right)
        self._use_close = use_close
        self.highs: deque[Swing] = deque(maxlen=max_history)
        self.lows: deque[Swing] = deque(maxlen=max_history)
        self.trend: str | None = None
        self._max_history = max_history

    def update(self, index: int, candle: Candle) -> tuple[list[Swing], StructureBreak | None]:
        """Feed one bar; return (newly confirmed swings, structure break or None).

        Breaks are checked against the swings known *before* this bar, then the
        bar's own pivots are recorded — a pivot confirmed by this very bar sits
        `right` bars in the past and cannot be what the bar just broke.
        """
        top = candle.close if self._use_close else candle.high
        bottom = candle.close if self._use_close else candle.low

        brk: StructureBreak | None = None
        if self.highs and top > self.highs[-1].price:
            level = self.highs[-1].price
            choch = self.trend == "down"
            self.trend = "up"
            # Every stored high now under price is spent, not just the newest.
            # Dropping only the last one would let an older, lower pivot report
            # a second "break" on the very next bar.
            self.highs = deque(
                (s for s in self.highs if s.price >= top), maxlen=self._max_history
            )
            brk = StructureBreak(index, candle.ts, "up", level, choch)
        elif self.lows and bottom < self.lows[-1].price:
            level = self.lows[-1].price
            choch = self.trend == "up"
            self.trend = "down"
            self.lows = deque(
                (s for s in self.lows if s.price <= bottom), maxlen=self._max_history
            )
            brk = StructureBreak(index, candle.ts, "down", level, choch)

        swings = self._det.update(candle)
        for swing in swings:
            (self.highs if swing.kind == "high" else self.lows).append(swing)
        return swings, brk

    def last_low(self) -> Swing | None:
        return self.lows[-1] if self.lows else None

    def last_high(self) -> Swing | None:
        return self.highs[-1] if self.highs else None
