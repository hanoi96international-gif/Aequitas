"""Order blocks, displacement and fair-value gaps — the "OB" half of LSOB.

An order block is the last candle *against* the direction of the impulsive
move that follows it: the last up-candle before a sell-off, the last
down-candle before a rally. The zone it leaves behind is where unfilled
orders sit, and it is where this agent wants its entry.

A sweep alone is not a setup. What promotes it to one is *displacement* —
the move away from the swept level has to be violent enough to be someone
actually repricing the market rather than noise drifting back.
"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Candle


@dataclass(frozen=True, slots=True)
class OrderBlock:
    direction: str  # 'short' (bearish OB / supply) | 'long' (bullish OB / demand)
    index: int
    ts: int
    top: float
    bottom: float

    @property
    def mid(self) -> float:
        return (self.top + self.bottom) / 2.0

    def edge(self, which: str) -> float:
        """`proximal` = the side price reaches first on the retrace."""
        if which == "mid":
            return self.mid
        if which == "proximal":
            return self.bottom if self.direction == "short" else self.top
        if which == "distal":
            return self.top if self.direction == "short" else self.bottom
        raise ValueError(f"unknown entry edge {which!r}")


def zone_of(candle: Candle, direction: str, mode: str) -> tuple[float, float]:
    """Return (top, bottom) of the order block zone under the chosen mode."""
    if mode == "full":
        return candle.high, candle.low
    if mode == "body":
        return candle.body_top, candle.body_bottom
    if mode == "body_to_extreme":
        # The classic reading: body plus the wick that points into the move.
        if direction == "short":
            return candle.high, candle.body_bottom
        return candle.body_top, candle.low
    raise ValueError(f"unknown zone mode {mode!r}")


def find_order_block(
    window: list[tuple[int, Candle]],
    direction: str,
    zone_mode: str,
    reference_close: float,
) -> OrderBlock | None:
    """Find the last opposing candle in `window`, scanning backwards.

    `window` is (absolute_index, candle) pairs in chronological order, already
    trimmed to the caller's lookback. `reference_close` is the current price:
    a zone that does not sit on the far side of it is not a retracement
    target, it is price we have already left behind, and is rejected.
    """
    for index, candle in reversed(window):
        opposing = candle.is_bullish if direction == "short" else candle.is_bearish
        if not opposing:
            continue
        top, bottom = zone_of(candle, direction, zone_mode)
        if top <= bottom:
            continue
        if direction == "short" and bottom <= reference_close:
            continue
        if direction == "long" and top >= reference_close:
            continue
        return OrderBlock(direction, index, candle.ts, top, bottom)
    return None


def has_fvg(window: list[tuple[int, Candle]], direction: str) -> bool:
    """True if the leg contains a fair-value gap in the traded direction.

    A bearish FVG is a three-bar formation where bar 3's high never reaches
    bar 1's low — price moved so fast it left an untraded band behind.
    """
    candles = [c for _, c in window]
    for i in range(2, len(candles)):
        first, third = candles[i - 2], candles[i]
        if direction == "short" and third.high < first.low:
            return True
        if direction == "long" and third.low > first.high:
            return True
    return False


def displacement_atr(sweep_extreme: float, close: float, direction: str, atr: float) -> float:
    """How far price has travelled away from the raided level, in ATR units."""
    if atr <= 0:
        return 0.0
    move = sweep_extreme - close if direction == "short" else close - sweep_extreme
    return move / atr
