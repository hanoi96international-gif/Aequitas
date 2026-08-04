from __future__ import annotations

from conftest import candle, flat

from lsob.model import ATR, timeframe_ms
from lsob.structure import MarketStructure, SwingDetector


def test_swing_high_is_reported_only_after_the_confirmation_window():
    det = SwingDetector(left=2, right=2)
    bars = [
        candle(0, 100, 101, 99, 100),
        candle(1, 100, 102, 99, 101),
        candle(2, 101, 110, 100, 105),  # the pivot
        candle(3, 105, 106, 103, 104),
        candle(4, 104, 105, 102, 103),
    ]
    seen = [det.update(b) for b in bars]
    assert all(not s for s in seen[:4]), "a pivot must not be known before its right window closes"

    swings = seen[4]
    highs = [s for s in swings if s.kind == "high"]
    assert len(highs) == 1
    assert highs[0].index == 2
    assert highs[0].price == 110
    assert highs[0].confirmed_at == 4


def test_equal_highs_yield_one_pivot_at_the_earlier_bar():
    det = SwingDetector(left=1, right=1)
    bars = [
        candle(0, 100, 101, 99, 100),
        candle(1, 100, 110, 99, 105),
        candle(2, 105, 110, 104, 106),
        candle(3, 106, 107, 105, 106),
    ]
    highs = [s for b in bars for s in det.update(b) if s.kind == "high"]
    assert [h.index for h in highs] == [1]


def test_break_of_structure_fires_once_not_on_every_stale_pivot():
    ms = MarketStructure(left=1, right=1)
    for i, bar in enumerate(
        [
            candle(0, 100, 101, 99, 100),
            candle(1, 100, 105, 99, 104),  # pivot high at 105
            candle(2, 104, 104, 100, 101),
            candle(3, 101, 108, 100, 103),  # pivot high at 108
            candle(4, 103, 104, 101, 102),
        ]
    ):
        ms.update(i, bar)
    assert [s.price for s in ms.highs] == [105, 108]

    _, brk = ms.update(5, candle(5, 102, 112, 102, 111))
    assert brk is not None and brk.direction == "up" and brk.level == 108

    # 105 is still below price but already spent — it must not report again.
    _, brk2 = ms.update(6, candle(6, 111, 112, 106, 107))
    assert brk2 is None


def test_atr_withholds_a_reading_until_the_period_is_complete():
    atr = ATR(3)
    bars = flat(2, 100)
    assert all(atr.update(b) is None for b in bars)
    assert atr.update(candle(2, 100, 102, 98, 100)) is not None
    assert atr.value > 0


def test_timeframe_parsing():
    assert timeframe_ms("15m") == 900_000
    assert timeframe_ms("4h") == 14_400_000
    assert timeframe_ms("1d") == 86_400_000
    for bad in ("", "m", "0m", "15x", "abc"):
        try:
            timeframe_ms(bad)
        except ValueError:
            continue
        raise AssertionError(f"{bad!r} should be rejected")
