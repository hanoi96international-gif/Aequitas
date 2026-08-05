from __future__ import annotations

import pytest
from conftest import BASE_MS
from test_execution import short_signal, zero_cost_config

from lsob.execution import Executor
from lsob.intrabar import IntrabarIndex, first_of, touched_after_fill
from lsob.model import Candle

HOUR = 3_600_000
MINUTE = 60_000


def minute(offset: int, o: float, h: float, l: float, c: float) -> Candle:
    return Candle(ts=BASE_MS + offset * MINUTE, open=o, high=h, low=l, close=c, volume=1.0)


def hour(offset: int, o: float, h: float, l: float, c: float) -> Candle:
    return Candle(ts=BASE_MS + offset * HOUR, open=o, high=h, low=l, close=c, volume=1.0)


# ── the index ────────────────────────────────────────────────────────────


def test_fine_candles_are_grouped_by_the_bar_that_contains_them():
    fine = [minute(i, 100, 101, 99, 100) for i in range(120)]
    index = IntrabarIndex(fine, HOUR)
    assert len(index) == 2
    assert len(index.within(BASE_MS)) == 60
    assert len(index.within(BASE_MS + HOUR)) == 60
    assert index.within(BASE_MS + 2 * HOUR) == []


def test_coverage_is_reported_not_assumed():
    index = IntrabarIndex([minute(0, 100, 101, 99, 100)], HOUR)
    assert index.covers(BASE_MS) is True
    assert index.covers(BASE_MS + HOUR) is False


def test_a_non_positive_bucket_is_rejected():
    with pytest.raises(ValueError, match="bucket_ms"):
        IntrabarIndex([], 0)


# ── ordering ─────────────────────────────────────────────────────────────


def test_the_first_event_wins():
    fine = [minute(0, 100, 100.5, 99.5, 100), minute(1, 100, 103, 100, 102)]
    events = [("up", lambda c: c.high >= 103), ("down", lambda c: c.low <= 99)]
    assert first_of(fine, events, pessimistic="down") == "up"


def test_two_events_inside_one_fine_candle_fall_back_to_pessimism():
    """Resolution improves the answer; it never invents one."""
    fine = [minute(0, 100, 103, 98, 100)]
    events = [("up", lambda c: c.high >= 103), ("down", lambda c: c.low <= 99)]
    assert first_of(fine, events, pessimistic="down") == "down"


def test_no_event_returns_nothing():
    fine = [minute(0, 100, 100.5, 99.5, 100)]
    events = [("up", lambda c: c.high >= 103)]
    assert first_of(fine, events, pessimistic="up") is None


# ── entry then stop ──────────────────────────────────────────────────────


def test_a_short_entry_is_always_crossed_before_its_stop():
    """The geometry, not an assumption: for a resting limit the entry lies
    between the bar's open and the stop, so price reaches it first. Verified
    on real data — 42 of 42 such bars. Finer candles cannot overturn this.
    """
    fine = [minute(0, 99, 102.5, 99, 102)]  # one push through entry then stop
    assert touched_after_fill(fine, entry=100.0, level=102.0, direction="short") is None

    stepped = [
        minute(0, 99, 100.5, 99, 100.4),   # crosses the entry
        minute(1, 100.4, 102.5, 100.4, 102),  # then the stop
    ]
    assert touched_after_fill(stepped, entry=100.0, level=102.0, direction="short") is True


def test_a_stop_reached_after_the_fill_did_hit_the_position():
    fine = [
        minute(0, 99, 100.2, 98.8, 100),   # entry fills
        minute(1, 100, 102.5, 100, 102),   # then the stop
    ]
    assert touched_after_fill(fine, entry=100.0, level=102.0, direction="short") is True


def test_a_fill_and_stop_in_the_same_minute_stays_unresolved():
    fine = [minute(0, 99, 102.5, 98.8, 102)]
    assert touched_after_fill(fine, entry=100.0, level=102.0, direction="short") is None


def test_missing_fine_data_stays_unresolved():
    assert touched_after_fill([], entry=100.0, level=102.0, direction="short") is None


def test_a_long_is_the_mirror_image():
    fine = [minute(0, 101, 101, 99.8, 100), minute(1, 100, 100, 97.5, 98)]
    assert touched_after_fill(fine, entry=100.0, level=98.0, direction="long") is True


# ── through the executor ─────────────────────────────────────────────────


def coarse_bar_spanning_entry_and_stop() -> Candle:
    return hour(0, 99.0, 102.5, 98.0, 99.5)


def test_without_fine_data_the_ambiguous_bar_is_scored_as_a_stop():
    ex = Executor(zero_cost_config())
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
    ex.on_bar(1, coarse_bar_spanning_entry_and_stop())
    assert len(ex.trades) == 1 and ex.trades[0].exit_reason == "stop"
    assert ex.resolved_by_intrabar == 0


def test_fine_data_confirms_rather_than_overturns_the_entry_bar_stop():
    """There is no rescue here, and the test says so rather than pretending."""
    fine = [
        minute(0, 99, 100.5, 99, 100.4),
        minute(30, 100.4, 102.5, 100.4, 102),
    ]
    ex = Executor(zero_cost_config(), IntrabarIndex(fine, HOUR))
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
    ex.on_bar(1, coarse_bar_spanning_entry_and_stop())

    assert len(ex.trades) == 1 and ex.trades[0].exit_reason == "stop"
    assert ex.resolved_by_intrabar == 1


def test_fine_data_confirms_a_stop_that_really_did_come_after_the_fill():
    fine = [
        minute(0, 99, 100.2, 98.5, 100),   # fill
        minute(30, 100, 102.5, 100, 102),  # then the stop
    ]
    ex = Executor(zero_cost_config(), IntrabarIndex(fine, HOUR))
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
    ex.on_bar(1, coarse_bar_spanning_entry_and_stop())

    assert len(ex.trades) == 1 and ex.trades[0].exit_reason == "stop"
    assert ex.resolved_by_intrabar == 1


def test_a_target_reached_before_the_stop_is_scored_as_a_win():
    """The classic ambiguity: one bar spans both. The minutes decide."""
    entry_bar = hour(0, 100.0, 100.2, 99.0, 99.5)
    both_bar = hour(1, 99.5, 102.5, 95.0, 99.0)
    fine = [
        Candle(ts=BASE_MS + HOUR + i * MINUTE, open=99, high=99.5, low=95.0, close=96)
        if i == 0
        else Candle(ts=BASE_MS + HOUR + i * MINUTE, open=96, high=102.5, low=96, close=99)
        for i in range(2)
    ]
    ex = Executor(zero_cost_config(), IntrabarIndex(fine, HOUR))
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
    ex.on_bar(1, entry_bar)
    trades = ex.on_bar(2, both_bar)

    assert len(trades) == 1
    assert trades[0].exit_reason == "tp1", "the target was reached first"
    assert trades[0].r_multiple > 0
    assert ex.resolved_by_intrabar == 1
