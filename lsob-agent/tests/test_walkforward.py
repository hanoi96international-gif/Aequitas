from __future__ import annotations

import pytest
from test_no_lookahead import random_walk

from lsob.config import Config
from lsob.walkforward import apply_overrides, expand_grid, spearman, walk_forward


def test_overrides_do_not_mutate_the_original_config():
    cfg = Config()
    changed = apply_overrides(cfg, {"orderblock.displacement_atr": 3.0})
    assert changed.orderblock.displacement_atr == 3.0
    assert cfg.orderblock.displacement_atr == 1.0


def test_integer_grid_values_become_floats_where_the_field_is_a_float():
    changed = apply_overrides(Config(), {"orderblock.displacement_atr": 3})
    assert isinstance(changed.orderblock.displacement_atr, float)


def test_an_override_that_breaks_the_config_is_rejected():
    with pytest.raises(ValueError, match="min_rr|edge"):
        apply_overrides(Config(), {"entry.edge": "sideways"})


@pytest.mark.parametrize(
    "key, message",
    [
        ("nodot", "section.field"),
        ("bogus.field", "no config section"),
        ("entry.bogus", "no field"),
    ],
)
def test_bad_grid_keys_are_named_in_the_error(key, message):
    with pytest.raises(ValueError, match=message):
        apply_overrides(Config(), {key: 1})


def test_grid_expansion_is_the_full_product():
    assert len(expand_grid({"a.b": [1, 2], "c.d": [3, 4, 5]})) == 6
    assert expand_grid({}) == [{}]


def test_spearman_handles_the_extremes():
    xs = [1.0, 2.0, 3.0, 4.0]
    assert spearman(xs, xs) == pytest.approx(1.0)
    assert spearman(xs, xs[::-1]) == pytest.approx(-1.0)
    assert spearman([1.0, 1.0, 1.0], [1.0, 2.0, 3.0]) == pytest.approx(0.0, abs=1e-9)


def test_each_fold_is_scored_on_bars_it_did_not_choose_on():
    """The test window must start exactly where the train window ends."""
    candles = random_walk(6_000, seed=3)
    result = walk_forward(
        Config(),
        candles,
        {"orderblock.displacement_atr": [0.5, 1.5]},
        train_bars=2_000,
        test_bars=1_000,
        min_trades=1,
    )
    assert result.folds, "the fixture should produce at least one scored fold"
    for fold in result.folds:
        assert fold.train_range[1] == fold.test_range[0]
        assert fold.test_range[1] - fold.test_range[0] == 1_000
        assert fold.train_range[1] - fold.train_range[0] == 2_000
    assert result.grid_size == 2


def test_a_verdict_refuses_to_conclude_from_too_few_trades():
    candles = random_walk(3_500, seed=5)
    result = walk_forward(
        Config(),
        candles,
        {"orderblock.displacement_atr": [1.0]},
        train_bars=2_000,
        test_bars=1_000,
        min_trades=1,
    )
    if result.oos_trades < 30:
        assert "Too few out-of-sample trades" in result.format()


def test_folds_without_enough_training_trades_are_skipped_not_scored():
    candles = random_walk(4_000, seed=9)
    result = walk_forward(
        Config(),
        candles,
        {"orderblock.displacement_atr": [50.0]},  # nothing can meet this
        train_bars=2_000,
        test_bars=1_000,
        min_trades=5,
    )
    assert result.folds == []
    assert result.skipped_folds > 0
    assert "No fold produced enough trades" in result.format()


def test_tied_scores_get_averaged_ranks_not_arbitrary_ones():
    """A grid routinely scores sets identically; order must not fake a signal."""
    tied = [5.0, 5.0, 5.0, 5.0]
    increasing = [1.0, 2.0, 3.0, 4.0]
    assert spearman(tied, increasing) == pytest.approx(0.0, abs=1e-9)
    # Half tied, half separated: the correlation is real but not perfect.
    partial = spearman([1.0, 1.0, 2.0, 3.0], [1.0, 2.0, 3.0, 4.0])
    assert 0.0 < partial < 1.0
