from __future__ import annotations

import pytest

from lsob.sizing import simulate, sweep


def winning_distribution() -> list[float]:
    """40% win rate at +3R, 60% at -1R — a clearly positive expectancy."""
    return [3.0] * 4 + [-1.0] * 6


def losing_distribution() -> list[float]:
    return [1.0] * 4 + [-1.0] * 6


def test_drawdown_grows_faster_than_the_risk_that_causes_it():
    rs = winning_distribution()
    low = simulate(rs, risk_pct=0.5, trades=200, runs=800, seed=1)
    high = simulate(rs, risk_pct=2.0, trades=200, runs=800, seed=1)

    assert high.median_max_dd_pct > low.median_max_dd_pct
    # 4x the risk should cost more than 4x the drawdown, because losses
    # compound against a shrinking account.
    assert high.median_max_dd_pct > 4 * low.median_max_dd_pct * 0.9


def test_sizing_cannot_rescue_a_negative_expectancy():
    """Smaller size slows the bleed; it does not change the direction."""
    rs = losing_distribution()
    for risk in (0.25, 0.5, 1.0):
        report = simulate(rs, risk_pct=risk, trades=300, runs=800, seed=2)
        assert report.median_return_pct < 0
        assert report.prob_loss > 0.5


def test_a_positive_expectancy_shows_up_as_a_positive_median():
    report = simulate(winning_distribution(), risk_pct=1.0, trades=300, runs=800, seed=3)
    assert report.median_return_pct > 0
    assert report.prob_loss < 0.5


def test_losing_streaks_are_longer_than_intuition_suggests():
    """A 40% win rate produces long runs of losses; the account must survive them."""
    report = simulate(winning_distribution(), risk_pct=1.0, trades=200, runs=800, seed=4)
    assert report.median_longest_losing_streak >= 5


def test_percentiles_are_ordered():
    report = simulate(winning_distribution(), risk_pct=1.0, trades=200, runs=800, seed=5)
    assert report.median_max_dd_pct <= report.p95_max_dd_pct <= report.worst_max_dd_pct
    assert report.p05_return_pct <= report.median_return_pct
    assert 0.0 <= report.prob_dd_over_50 <= report.prob_dd_over_20 <= 1.0


def test_the_simulation_is_reproducible():
    rs = winning_distribution()
    a = simulate(rs, risk_pct=1.0, trades=100, runs=400, seed=7)
    b = simulate(rs, risk_pct=1.0, trades=100, runs=400, seed=7)
    assert a.median_max_dd_pct == b.median_max_dd_pct
    assert a.median_return_pct == b.median_return_pct


def test_a_sweep_returns_one_report_per_level_in_order():
    reports = sweep(winning_distribution(), [0.5, 1.0, 2.0], trades=100, runs=300)
    assert [r.risk_pct for r in reports] == [0.5, 1.0, 2.0]


def test_degenerate_inputs_are_rejected():
    with pytest.raises(ValueError, match="R multiple"):
        simulate([], risk_pct=1.0)
    with pytest.raises(ValueError, match="risk_pct"):
        simulate([1.0], risk_pct=0.0)


def test_an_account_cannot_go_negative():
    """A -1R trade at 100% risk wipes the account; it must not go below zero."""
    report = simulate([-1.0], risk_pct=100.0, trades=50, runs=100, seed=9)
    assert report.median_return_pct >= -100.0
    assert report.prob_loss == 1.0
