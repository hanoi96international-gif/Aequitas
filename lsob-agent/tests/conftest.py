from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from lsob.model import Candle

STEP_MS = 15 * 60 * 1000
# 2024-01-01T00:00:00Z. Fixtures use a real epoch rather than counting from
# zero: timestamps that small are ambiguous between seconds and milliseconds,
# and no exchange would ever emit them.
BASE_MS = 1_704_067_200_000


def candle(i: int, o: float, h: float, l: float, c: float, v: float = 1.0) -> Candle:
    """Build a bar at slot `i`, asserting the OHLC is internally consistent.

    Test fixtures with a high below the close are the classic way to "prove"
    a strategy works on candles no exchange could print.
    """
    assert h >= max(o, c) and l <= min(o, c), f"bar {i}: impossible OHLC"
    return Candle(ts=BASE_MS + i * STEP_MS, open=o, high=h, low=l, close=c, volume=v)


def flat(count: int, price: float, spread: float = 1.0, start: int = 0) -> list[Candle]:
    """A run of quiet bars, used to build ATR before the interesting part."""
    return [
        candle(start + i, price, price + spread, price - spread, price)
        for i in range(count)
    ]
