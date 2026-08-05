from __future__ import annotations

import pytest
from conftest import BASE_MS, candle
from test_strategy import SETUP, base_config, run

from lsob.filters import DealingRange, SessionFilter, dealing_range
from lsob.orderblock import OrderBlock, is_mitigated

DAY_MS = 86_400_000
HOUR_MS = 3_600_000


# ── premium / discount ───────────────────────────────────────────────────


def test_equilibrium_splits_the_range_in_half():
    rng = DealingRange(high=110.0, low=100.0)
    assert rng.equilibrium == 105.0
    assert rng.position_of(100.0) == 0.0
    assert rng.position_of(110.0) == 1.0
    assert rng.position_of(105.0) == 0.5


def test_shorts_want_premium_and_longs_want_discount():
    rng = DealingRange(high=110.0, low=100.0)
    assert rng.allows("short", 108.0) is True
    assert rng.allows("short", 102.0) is False
    assert rng.allows("long", 102.0) is True
    assert rng.allows("long", 108.0) is False


def test_a_higher_threshold_demands_a_deeper_premium():
    rng = DealingRange(high=110.0, low=100.0)
    assert rng.allows("short", 106.0, threshold=0.5) is True
    assert rng.allows("short", 106.0, threshold=0.8) is False
    assert rng.allows("long", 104.0, threshold=0.5) is True
    assert rng.allows("long", 104.0, threshold=0.8) is False


def test_a_degenerate_range_permits_nothing():
    flat_range = DealingRange(high=100.0, low=100.0)
    assert flat_range.valid is False
    assert flat_range.allows("short", 100.0) is False
    assert flat_range.allows("long", 100.0) is False


def test_the_range_spans_the_most_recent_swings_only():
    highs = [100.0, 120.0, 105.0, 106.0, 107.0, 108.0]
    lows = [90.0, 80.0, 95.0, 96.0, 97.0, 98.0]
    recent = dealing_range(highs, lows, count=3)
    assert recent == DealingRange(high=108.0, low=96.0), "the old 120/80 extremes are dropped"
    assert dealing_range([], [1.0], count=3) is None


def test_premium_discount_filter_vetoes_the_reference_setup():
    """The short's entry sits low in its range, so a premium rule rejects it."""
    cfg = base_config()
    assert len(run(cfg)) == 1

    cfg.filters.premium_discount = True
    cfg.filters.pd_threshold = 0.95  # demand an extreme premium
    assert run(cfg) == []


# ── mitigation ───────────────────────────────────────────────────────────


def test_a_block_price_left_and_returned_to_is_mitigated():
    ob = OrderBlock("short", index=10, ts=0, top=105.0, bottom=103.0)
    window = [
        (11, candle(11, 100.0, 101.0, 99.0, 100.0)),  # clearly below: departed
        (12, candle(12, 100.0, 103.5, 99.5, 101.0)),  # trades back into the zone
    ]
    assert is_mitigated(window, ob) is True


def test_the_displacement_candle_leaving_the_block_is_not_a_visit():
    """It opens inside the block by construction — that is departure, not return."""
    ob = OrderBlock("short", index=10, ts=0, top=107.0, bottom=103.8)
    window = [(11, candle(11, 104.5, 104.7, 101.0, 101.2))]
    assert is_mitigated(window, ob) is False


def test_a_block_price_stayed_away_from_is_not_mitigated():
    ob = OrderBlock("short", index=10, ts=0, top=105.0, bottom=103.0)
    window = [
        (11, candle(11, 100.0, 101.0, 99.0, 100.0)),
        (12, candle(12, 100.0, 102.9, 98.0, 99.0)),  # never reaches 103
    ]
    assert is_mitigated(window, ob) is False


def test_a_long_block_mirrors_the_same_rule():
    ob = OrderBlock("long", index=10, ts=0, top=103.0, bottom=101.0)
    left_only = [(11, candle(11, 104.0, 106.0, 103.5, 105.0))]
    assert is_mitigated(left_only, ob) is False
    came_back = left_only + [(12, candle(12, 105.0, 105.5, 102.5, 103.0))]
    assert is_mitigated(came_back, ob) is True


def test_the_blocks_own_candle_does_not_count_as_a_visit():
    ob = OrderBlock("short", index=10, ts=0, top=105.0, bottom=103.0)
    window = [(10, candle(10, 103.5, 105.0, 103.0, 104.0))]
    assert is_mitigated(window, ob) is False


def test_requiring_an_unmitigated_block_does_not_break_the_reference_setup():
    """Price displaced away from the block and never came back, so it stands."""
    cfg = base_config()
    cfg.filters.require_unmitigated = True
    assert len(run(cfg)) == 1


def test_a_block_revisited_before_the_signal_is_rejected():
    revisit = list(SETUP[:32]) + [
        # Bar 32 drops clear of the block (103.8-107): price has departed.
        candle(32, 103.0, 103.5, 101.0, 101.5),
        # Bar 33 rallies back into it before closing through the reference
        # low — so the block is filled by the time the signal would fire.
        candle(33, 101.5, 104.2, 98.5, 99.0),
        candle(34, 99.0, 99.5, 97.0, 97.5),
    ]
    cfg = base_config()
    assert len(run(cfg, revisit)) == 1, "without the filter the setup still fires"

    cfg.filters.require_unmitigated = True
    assert run(cfg, revisit) == []


# ── sessions ─────────────────────────────────────────────────────────────


def monday(hour: int, minute: int = 0) -> int:
    """2024-01-01 was a Monday."""
    return BASE_MS + hour * HOUR_MS + minute * 60_000


def test_a_disabled_session_filter_accepts_everything():
    assert SessionFilter(False, [], []).allows(monday(3)) is True


def test_only_bars_inside_a_window_are_accepted():
    session = SessionFilter(True, ["07:00-10:00"], [0, 1, 2, 3, 4])
    assert session.allows(monday(8)) is True
    assert session.allows(monday(7)) is True, "the window start is inclusive"
    assert session.allows(monday(10)) is False, "the window end is exclusive"
    assert session.allows(monday(3)) is False


def test_weekends_are_excluded_when_days_say_so():
    session = SessionFilter(True, ["07:00-10:00"], [0, 1, 2, 3, 4])
    saturday = monday(8) + 5 * DAY_MS
    sunday = monday(8) + 6 * DAY_MS
    assert session.allows(saturday) is False
    assert session.allows(sunday) is False


def test_multiple_windows_are_all_honoured():
    session = SessionFilter(True, ["07:00-10:00", "12:00-15:00"], [0, 1, 2, 3, 4])
    assert session.allows(monday(8)) is True
    assert session.allows(monday(13)) is True
    assert session.allows(monday(11)) is False


@pytest.mark.parametrize("window", ["7-10", "07:00", "25:00-26:00", "10:00-07:00", "abc"])
def test_a_malformed_window_is_rejected_at_construction(window):
    with pytest.raises(ValueError):
        SessionFilter(True, [window], [0])


def test_an_enabled_filter_needs_windows_and_days():
    with pytest.raises(ValueError, match="session_windows"):
        SessionFilter(True, [], [0])
    with pytest.raises(ValueError, match="session_days"):
        SessionFilter(True, ["07:00-10:00"], [])


def test_the_session_filter_can_veto_a_valid_setup():
    cfg = base_config()
    assert len(run(cfg)) == 1

    # The reference setup fires at 08:15 UTC on a Monday; exclude that hour.
    cfg.filters.session_enabled = True
    cfg.filters.session_windows = ["12:00-15:00"]
    cfg.filters.session_days = [0, 1, 2, 3, 4]
    assert run(cfg) == []

    cfg.filters.session_windows = ["07:00-10:00"]
    assert len(run(cfg)) == 1
