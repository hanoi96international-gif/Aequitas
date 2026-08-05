from __future__ import annotations

import pytest
from test_backtest import WITH_RETRACE
from test_strategy import base_config

from lsob.compare import MIN_TRADES, Comparison, MarketRun, compare
from lsob.data import save_csv
from lsob.metrics import Stats


def run_with(trades: int, expectancy: float, name: str = "market") -> MarketRun:
    return MarketRun(
        name=name,
        bars=1000,
        signals=trades * 2,
        stats=Stats(trades=trades, expectancy_r=expectancy, win_rate=50.0, profit_factor=1.0),
    )


def test_a_thin_sample_is_marked_and_never_carries_the_verdict():
    thin = run_with(MIN_TRADES - 1, +5.0, "lucky")
    solid = run_with(MIN_TRADES, -0.2, "real")
    assert thin.conclusive is False
    assert solid.conclusive is True

    text = Comparison([thin, solid]).format()
    assert "*" in text
    assert "describes the sample" in text


def test_one_usable_market_is_not_a_second_opinion():
    """The whole point of the command is refusing to double-count evidence."""
    text = Comparison([run_with(50, +0.4, "origin"), run_with(3, +9.9, "thin")]).format()
    assert "not a second opinion" in text


def test_positive_everywhere_is_stated_plainly():
    text = Comparison([run_with(40, +0.3, "a"), run_with(60, +0.5, "b")]).format()
    assert "every market with a usable sample" in text


def test_negative_everywhere_is_stated_plainly():
    text = Comparison([run_with(40, -0.3, "a"), run_with(60, -0.5, "b")]).format()
    assert "Negative on every market" in text


def test_working_in_one_venue_only_is_named_for_what_it_is():
    text = Comparison([run_with(40, +0.6, "origin"), run_with(60, -0.4, "elsewhere")]).format()
    assert "describing that venue" in text
    assert "origin" in text


def test_nothing_to_compare_says_so():
    assert Comparison([]).format() == "No markets to compare."


def test_only_venue_properties_may_differ_per_market(tmp_path):
    """Timeframe and costs belong to the venue; everything else is the strategy."""
    path = tmp_path / "m.csv"
    save_csv(path, WITH_RETRACE)

    cfg = base_config()
    cfg.entry.retracement = 0.777  # a strategy setting that must survive untouched
    result = compare(
        cfg,
        [
            ("cheap", str(path), "15m", (0.0, 0.0, 0.0)),
            ("dear", str(path), "15m", (25.0, 25.0, 25.0)),
        ],
    )
    assert [r.name for r in result.runs] == ["cheap", "dear"]
    assert cfg.entry.retracement == 0.777, "the caller's config must not be mutated"
    assert cfg.data.csv == "", "nor its data path"
    # Same market, same signals — only the costs differ.
    assert result.runs[0].signals == result.runs[1].signals


def test_a_malformed_market_row_is_rejected_at_config_time():
    from lsob.config import validate

    cfg = base_config()
    cfg.compare.markets = [["too", "few", "fields"]]
    with pytest.raises(ValueError, match="maker_bps"):
        validate(cfg)
