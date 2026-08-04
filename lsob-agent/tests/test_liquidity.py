from __future__ import annotations

from conftest import candle

from lsob.liquidity import LiquidityBook, LiquidityConfig
from lsob.structure import Swing


def book(**overrides) -> LiquidityBook:
    cfg = LiquidityConfig(**overrides)
    return LiquidityBook(cfg)


def bsl(price: float, index: int = 0) -> Swing:
    return Swing(index=index, ts=index * 1000, price=price, kind="high", confirmed_at=index)


def test_rejected_raid_is_a_sweep():
    lb = book()
    lb.add_swing(bsl(100.0), atr=1.0)
    sweeps = lb.update(10, candle(10, 99, 100.8, 98.5, 99.0), atr=1.0)
    assert len(sweeps) == 1
    s = sweeps[0]
    assert s.direction == "short"
    assert s.extreme == 100.8
    assert s.pierce_index == 10


def test_accepted_break_is_not_a_sweep():
    lb = book(reclaim_bars=1)
    lb.add_swing(bsl(100.0), atr=1.0)
    assert lb.update(10, candle(10, 99, 100.8, 98.5, 100.5), atr=1.0) == []
    # Still above the level a bar later: the market accepted the price.
    assert lb.update(11, candle(11, 100.5, 101.0, 100.2, 100.9), atr=1.0) == []
    assert lb.pools == []


def test_travelling_far_past_the_level_is_a_breakout_not_a_raid():
    lb = book(max_penetration_atr=1.5)
    lb.add_swing(bsl(100.0), atr=1.0)
    # 2 ATR beyond the level, then back inside — too deep to be a stop raid.
    assert lb.update(10, candle(10, 99, 102.0, 98.0, 99.0), atr=1.0) == []
    assert lb.pools == []


def test_late_reclaim_is_allowed_within_the_window():
    lb = book(reclaim_bars=2)
    lb.add_swing(bsl(100.0), atr=1.0)
    assert lb.update(10, candle(10, 99, 100.5, 99, 100.3), atr=1.0) == []
    assert lb.update(11, candle(11, 100.3, 100.6, 100.1, 100.2), atr=1.0) == []
    sweeps = lb.update(12, candle(12, 100.2, 100.4, 99.0, 99.2), atr=1.0)
    assert len(sweeps) == 1
    assert sweeps[0].extreme == 100.6, "the extreme spans every bar of the raid"


def test_equal_highs_merge_into_one_pool_with_more_touches():
    lb = book(equal_level_atr=0.2)
    lb.add_swing(bsl(100.0, index=1), atr=1.0)
    lb.add_swing(bsl(100.1, index=5), atr=1.0)
    lb.add_swing(bsl(103.0, index=9), atr=1.0)
    assert len(lb.pools) == 2
    merged = next(p for p in lb.pools if p.price < 101)
    assert merged.touches == 2
    assert merged.price == 100.1, "the pool sits at the extreme of the cluster"


def test_min_touches_filters_single_pivot_raids():
    lb = book(min_touches=2)
    lb.add_swing(bsl(100.0), atr=1.0)
    assert lb.update(10, candle(10, 99, 100.5, 98.5, 99.0), atr=1.0) == []


def test_sell_side_sweep_is_the_mirror_image():
    lb = book()
    lb.add_swing(Swing(0, 0, 100.0, "low", 0), atr=1.0)
    sweeps = lb.update(10, candle(10, 101, 101.5, 99.2, 100.9), atr=1.0)
    assert len(sweeps) == 1
    assert sweeps[0].direction == "long"
    assert sweeps[0].extreme == 99.2
