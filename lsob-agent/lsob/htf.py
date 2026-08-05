"""The higher-timeframe swing a ladder is anchored to.

A retracement ladder drawn by hand is not measured across the little leg
that follows a sweep. It is dragged between two swing points on a slower
chart — the 4h low to the 4h high — and then left alone while price works
inside it. Every rung keeps the same price for days, which is the whole
reason those levels are worth watching: everyone looking at that chart sees
the same number.

Measuring across the local leg instead gives levels that move with every new
setup. Both are defensible, they are simply not the same tool, and the
difference in where 0.882 lands is large.

Candles are folded into higher-timeframe bars by the same aggregator the
bias filter uses, and swings are only reported once their confirmation
window closes — so a 4h pivot is knowable no earlier here than it would be
on a 4h chart.
"""

from __future__ import annotations

from .bias import HtfAggregator
from .model import Candle, timeframe_ms
from .structure import Swing, SwingDetector


class HtfSwings:
    """Tracks the most recent confirmed swing high and low on a slower chart."""

    __slots__ = ("_agg", "_detector", "_seen", "high", "low")

    def __init__(self, timeframe: str, multiplier: int, left: int, right: int) -> None:
        if multiplier < 1:
            raise ValueError("htf multiplier must be >= 1")
        self._agg = HtfAggregator(timeframe_ms(timeframe) * multiplier)
        self._detector = SwingDetector(left, right)
        self._seen = 0
        self.high: Swing | None = None
        self.low: Swing | None = None

    def update(self, candle: Candle) -> None:
        closed = self._agg.update(candle)
        if closed is None:
            return
        for swing in self._detector.update(closed):
            if swing.kind == "high":
                self.high = swing
            else:
                self.low = swing
        self._seen += 1

    @property
    def bars_seen(self) -> int:
        return self._seen

    def leg(self, direction: str) -> tuple[float, float] | None:
        """Return (raid end, far end) of the anchoring swing, or None.

        Oriented so a ratio of 1.0 lands on the end price would be shorted
        into and 0.0 on the opposite extreme — the same orientation a chart
        tool produces when the ladder is dragged low-to-high.
        """
        if self.high is None or self.low is None:
            return None
        if self.high.price <= self.low.price:
            return None
        if direction == "short":
            return self.high.price, self.low.price
        return self.low.price, self.high.price
