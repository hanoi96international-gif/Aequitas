from __future__ import annotations

import pytest
from conftest import BASE_MS, STEP_MS, candle
from test_execution import short_signal, zero_cost_config

from lsob.config import Config, validate
from lsob.daytrade import DayClock
from lsob.execution import Executor

HOUR_MS = 60 * 60 * 1000


def clock(flat: str = "", cutoff: str = "", bars: int = 0, per_day: int = 0, tf: int = HOUR_MS):
    return DayClock(True, flat, cutoff, bars, per_day, tf)


def at(hour: int, minute: int = 0) -> int:
    return BASE_MS + hour * HOUR_MS + minute * 60_000


def daytrade_config(**rules) -> Config:
    cfg = zero_cost_config()
    cfg.daytrade.enabled = True
    for key, value in rules.items():
        setattr(cfg.daytrade, key, value)
    return cfg


# ── the clock ────────────────────────────────────────────────────────────


def test_flatten_fires_on_the_bar_that_ends_at_the_cutoff():
    """A bar carries its *open* time, so 22:00 belongs to the 21:00 bar."""
    c = clock(flat="22:00")
    assert not c.must_flatten(at(20)), "20:00-21:00 ends before the cutoff"
    assert c.must_flatten(at(21)), "21:00-22:00 ends exactly on it"
    assert c.must_flatten(at(23)), "and anything later is later"


def test_a_cutoff_off_the_bar_grid_rounds_outward():
    c = clock(flat="21:55")
    assert c.must_flatten(at(21)), "the bar containing 21:55 is the last one"
    assert not c.must_flatten(at(20))


def test_a_bar_ending_at_midnight_does_not_wrap_into_the_next_day():
    """23:00 + 1h is minute 1440, not minute 0 — or the last bar never flattens."""
    assert clock(flat="22:00").must_flatten(at(23))
    assert clock(flat="24:00").must_flatten(at(23))


def test_without_an_explicit_cutoff_the_flatten_is_the_entry_deadline():
    c = clock(flat="22:00")
    assert c.entries_closed(at(21)), "filling into the bar you close out is not a trade"
    assert not c.entries_closed(at(20))


def test_the_entry_cutoff_can_come_earlier_than_the_flatten():
    c = clock(flat="22:00", cutoff="20:00")
    assert c.entries_closed(at(20))
    assert not c.entries_closed(at(19))
    assert not c.must_flatten(at(20)), "no new entries, but the open trade runs on"


def test_a_disabled_clock_answers_no_to_everything():
    c = DayClock(False, "22:00", "20:00", 4, 1, HOUR_MS)
    assert not c.must_flatten(at(23))
    assert not c.entries_closed(at(23))
    assert not c.timed_out(99)
    assert not c.day_full(99)


def test_the_day_key_is_the_utc_calendar_day():
    assert DayClock.day_key(at(23)) == DayClock.day_key(at(0))
    assert DayClock.day_key(at(24)) == DayClock.day_key(at(0)) + 1


# ── exits driven by the clock ────────────────────────────────────────────


def test_an_open_position_is_flat_at_the_end_of_the_session():
    # 15m bars from midnight: the bar ending at 01:00 is the one opening 00:45.
    ex = Executor(daytrade_config(flat_at="01:00"))
    ex.on_signal(0, short_signal(index=0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.5, 99.0))
    assert len(ex.positions) == 1, "filled at the limit"

    ex.on_bar(2, candle(2, 99.0, 99.5, 98.5, 99.0))
    assert len(ex.positions) == 1, "session still open"

    closed = ex.on_bar(3, candle(3, 99.0, 99.5, 98.0, 98.5))
    assert len(closed) == 1
    assert closed[0].exit_reason == "session_end"
    assert closed[0].exit_price == pytest.approx(98.5), "flattened at that bar's close"
    assert ex.positions == []


def test_a_stop_on_the_flatten_bar_is_a_stop_not_a_flatten():
    """Price resolves the bar before the clock does — the loss really happened."""
    ex = Executor(daytrade_config(flat_at="01:00"))
    ex.on_signal(0, short_signal(index=0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.5, 99.0))

    closed = ex.on_bar(3, candle(3, 99.0, 103.0, 98.5, 99.0))
    assert [t.exit_reason for t in closed] == ["stop"]


def test_the_time_stop_closes_a_trade_that_never_resolved():
    ex = Executor(daytrade_config(max_bars_in_trade=2))
    ex.on_signal(0, short_signal(index=0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.5, 99.0))
    assert ex.on_bar(2, candle(2, 99.0, 99.5, 98.5, 99.0)) == [], "one bar held"

    closed = ex.on_bar(3, candle(3, 99.0, 99.5, 98.5, 99.2))
    assert len(closed) == 1
    assert closed[0].exit_reason == "time_stop"
    assert closed[0].bars_held == 2


def test_no_working_order_survives_the_cutoff():
    """"Flat overnight" has to include the orders, not just the positions."""
    ex = Executor(daytrade_config(flat_at="02:00", no_entry_after="01:00"))
    ex.on_signal(0, short_signal(index=0))
    for i in range(1, 4):  # 00:15 .. 00:45, price nowhere near the entry
        ex.on_bar(i, candle(i, 95.0, 95.5, 94.5, 95.0))
    assert len(ex.pending) == 1

    ex.on_bar(4, candle(4, 95.0, 95.5, 94.5, 95.0))  # 01:00
    assert ex.pending == []
    assert ex.rejected.get("after_cutoff") == 1
    assert ex.positions == [], "and nothing filled on the way out"


def test_a_daily_trade_cap_stops_the_second_entry_and_resets_tomorrow():
    ex = Executor(daytrade_config(max_trades_per_day=1))

    ex.on_signal(0, short_signal(index=0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.5, 99.0))  # fills
    ex.on_bar(2, candle(2, 99.0, 99.0, 95.0, 96.0))  # target, trade done
    assert len(ex.trades) == 1

    ex.on_signal(2, short_signal(index=2))
    ex.on_bar(3, candle(3, 99.0, 100.5, 98.5, 99.0))
    assert ex.positions == [], "the day's allowance is spent"
    assert ex.rejected.get("day_limit") == 1

    ex.on_signal(96, short_signal(index=96))  # 96 x 15m = the next UTC day
    ex.on_bar(97, candle(97, 99.0, 100.5, 98.5, 99.0))
    assert len(ex.positions) == 1, "a new day is a new allowance"


def test_none_of_it_happens_while_daytrade_is_off():
    cfg = zero_cost_config()
    cfg.daytrade.enabled = False
    cfg.daytrade.flat_at = "01:00"
    cfg.daytrade.max_bars_in_trade = 1
    ex = Executor(cfg)
    ex.on_signal(0, short_signal(index=0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.5, 99.0))
    for i in range(2, 8):
        ex.on_bar(i, candle(i, 99.0, 99.5, 98.5, 99.0))
    assert len(ex.positions) == 1, "the switch is the switch"


# ── what a round trip costs, in units of risk ────────────────────────────


def test_cost_is_measured_in_the_trade_s_own_risk():
    cfg = Config()  # 2 maker + 5 taker + 1 slippage = 8 bps
    ex = Executor(cfg)
    # 100 * 0.0008 = 0.08 in price, against 2.00 of risk.
    assert ex.cost_r(short_signal(entry=100.0, stop=102.0)) == pytest.approx(0.04)
    # Same fees, quarter of the stop distance: four times the toll.
    assert ex.cost_r(short_signal(entry=100.0, stop=100.5)) == pytest.approx(0.16)


def test_the_cost_guard_refuses_setups_it_cannot_pay_for():
    cfg = Config()
    cfg.costs.max_cost_r = 0.10
    ex = Executor(cfg)
    assert ex.on_signal(0, short_signal(entry=100.0, stop=102.0)), "0.04 R is affordable"
    assert not ex.on_signal(0, short_signal(entry=100.0, stop=100.5)), "0.16 R is not"
    assert ex.rejected.get("cost_vs_risk") == 1


def test_the_cost_is_recorded_even_when_the_guard_is_off():
    ex = Executor(Config())
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0))
    assert ex.cost_r_values == [pytest.approx(0.04)]


# ── configuration ────────────────────────────────────────────────────────


def test_enabling_daytrade_without_a_rule_is_an_error():
    cfg = Config()
    cfg.daytrade.enabled = True
    with pytest.raises(ValueError, match="no rule is set"):
        validate(cfg)


def test_an_entry_cutoff_after_the_flatten_is_an_error():
    cfg = Config()
    cfg.daytrade.enabled = True
    cfg.daytrade.flat_at = "20:00"
    cfg.daytrade.no_entry_after = "21:00"
    with pytest.raises(ValueError, match="not be later"):
        validate(cfg)


def test_an_entry_cutoff_without_a_flatten_is_an_error():
    cfg = Config()
    cfg.daytrade.enabled = True
    cfg.daytrade.max_bars_in_trade = 8
    cfg.daytrade.no_entry_after = "21:00"
    with pytest.raises(ValueError, match="needs daytrade.flat_at"):
        validate(cfg)


@pytest.mark.parametrize("bad", ["2200", "22:00:00", "25:00", "22:61", ""])
def test_a_malformed_time_is_rejected(bad: str):
    cfg = Config()
    cfg.daytrade.enabled = True
    cfg.daytrade.flat_at = bad
    with pytest.raises(ValueError):
        validate(cfg)


def test_a_negative_cost_guard_is_rejected():
    cfg = Config()
    cfg.costs.max_cost_r = -1.0
    with pytest.raises(ValueError, match="max_cost_r"):
        validate(cfg)


def test_the_backtest_counts_what_was_held_overnight():
    from lsob.backtest import BacktestResult
    from lsob.metrics import Stats

    result = BacktestResult(
        stats=Stats(),
        trades=[],
        signals=[],
        equity_curve=[],
        cost_r=[0.3, 0.3, 0.9],
        overnight=2,
    )
    text = result.format()
    assert "0.30 R median" in text
    assert "eat 30% of the risk" in text


def test_the_step_is_what_the_fixtures_assume():
    assert STEP_MS == 15 * 60 * 1000
