"""Walk-forward analysis: optimise on the past, score on the future, roll on.

A grid search over the whole history tells you which settings *described*
that history best. That number is not an expectation for tomorrow, and the
gap between the two is where most strategies quietly die.

So this never scores a parameter set on data it was chosen on. The history
is cut into consecutive train/test windows; the winner of each train window
is applied — unchanged — to the test window that follows it, and only those
out-of-sample stretches are added up.

The two numbers to read are:

  * aggregate out-of-sample expectancy — what the *process* of optimising
    and then trading would actually have earned
  * the rank correlation between in-sample and out-of-sample scores — how
    much the ranking transfers at all. Near zero means the data cannot tell
    good settings from lucky ones, and picking a "best" set is self-deception
"""

from __future__ import annotations

import statistics
from dataclasses import dataclass, field
from itertools import product
from typing import Any

from .backtest import run_backtest
from .config import Config, validate
from .metrics import Stats
from .model import Candle


@dataclass(slots=True)
class Fold:
    index: int
    train_range: tuple[int, int]
    test_range: tuple[int, int]
    params: dict[str, Any]
    in_sample: Stats
    out_of_sample: Stats


@dataclass(slots=True)
class WalkForwardResult:
    folds: list[Fold] = field(default_factory=list)
    oos_expectancy: float = 0.0
    oos_trades: int = 0
    oos_total_r: float = 0.0
    rank_correlation: float = 0.0
    grid_size: int = 0
    skipped_folds: int = 0

    def format(self) -> str:
        if not self.folds:
            return (
                "No fold produced enough trades to score.\n"
                "Widen the windows, loosen the filters, or lower min_trades."
            )
        lines = [
            f"Folds scored        {len(self.folds)}"
            + (f"  ({self.skipped_folds} skipped)" if self.skipped_folds else ""),
            f"Grid size           {self.grid_size} parameter sets per fold",
            "",
            f"{'fold':<6}{'train':>14}{'test':>14}{'IS exp':>9}{'OOS exp':>10}{'OOS n':>7}",
        ]
        for f in self.folds:
            lines.append(
                f"{f.index:<6}{f'{f.train_range[0]}-{f.train_range[1]}':>14}"
                f"{f'{f.test_range[0]}-{f.test_range[1]}':>14}"
                f"{f.in_sample.expectancy_r:>+9.3f}{f.out_of_sample.expectancy_r:>+10.3f}"
                f"{f.out_of_sample.trades:>7}"
            )
        lines += [
            "",
            f"Out-of-sample expectancy  {self.oos_expectancy:+.3f} R over {self.oos_trades} trades",
            f"Out-of-sample total       {self.oos_total_r:+.2f} R",
            f"IS/OOS rank correlation   {self.rank_correlation:+.3f}",
            "",
            self._verdict(),
        ]
        return "\n".join(lines)

    def _verdict(self) -> str:
        if self.oos_trades < 30:
            return (
                f"Too few out-of-sample trades ({self.oos_trades}) to conclude anything. "
                "Any number above is within noise."
            )
        if abs(self.rank_correlation) < 0.2:
            return (
                "The in-sample ranking barely transfers (|rho| < 0.2): on this data, "
                "optimising these parameters is picking noise. Prefer settings that "
                "are broadly reasonable over settings that scored best."
            )
        if self.oos_expectancy <= 0:
            return "Optimising transfers somewhat, but the result is still not profitable."
        return (
            "The ranking transfers and out-of-sample expectancy is positive. "
            "Confirm on a different market before risking anything."
        )


def apply_overrides(cfg: Config, params: dict[str, Any]) -> Config:
    """Return a copy of `cfg` with dotted-path overrides applied.

    Keys look like "orderblock.displacement_atr" so a grid can reach any
    setting without this module needing to know what the settings are.
    """
    import copy

    out = copy.deepcopy(cfg)
    for path, value in params.items():
        section, _, field_name = path.partition(".")
        if not field_name:
            raise ValueError(f"grid key {path!r} must be 'section.field'")
        if not hasattr(out, section):
            raise ValueError(f"grid key {path!r}: no config section {section!r}")
        target = getattr(out, section)
        if not hasattr(target, field_name):
            raise ValueError(f"grid key {path!r}: no field {field_name!r} in [{section}]")
        current = getattr(target, field_name)
        # TOML writes `1` for a float threshold; coerce rather than silently
        # storing an int where every comparison expects a float.
        if isinstance(current, float) and isinstance(value, int) and not isinstance(value, bool):
            value = float(value)
        setattr(target, field_name, value)
    validate(out)
    return out


def expand_grid(grid: dict[str, list[Any]]) -> list[dict[str, Any]]:
    if not grid:
        return [{}]
    keys = list(grid)
    return [dict(zip(keys, values, strict=True)) for values in product(*(grid[k] for k in keys))]


def spearman(xs: list[float], ys: list[float]) -> float:
    """Rank correlation, computed here to keep the package dependency-free."""
    if len(xs) < 2:
        return 0.0

    def ranks(values: list[float]) -> list[float]:
        """Average ranks for ties.

        Ties are not an edge case here: a grid search routinely produces
        parameter sets that score identically, and ranking them 1-2-3 by
        their position in the list would manufacture a correlation out of
        nothing more than the order the grid happened to be expanded in.
        """
        order = sorted(range(len(values)), key=lambda i: values[i])
        out = [0.0] * len(values)
        position = 0
        while position < len(order):
            end = position
            while end + 1 < len(order) and values[order[end + 1]] == values[order[position]]:
                end += 1
            shared = (position + end) / 2.0
            for k in range(position, end + 1):
                out[order[k]] = shared
            position = end + 1
        return out

    rx, ry = ranks(xs), ranks(ys)
    mx, my = statistics.mean(rx), statistics.mean(ry)
    num = sum((a - mx) * (b - my) for a, b in zip(rx, ry, strict=True))
    den = (
        sum((a - mx) ** 2 for a in rx) * sum((b - my) ** 2 for b in ry)
    ) ** 0.5
    return num / den if den else 0.0


def _score(stats: Stats, metric: str) -> float:
    if metric == "expectancy_r":
        return stats.expectancy_r
    if metric == "total_r":
        return stats.total_r
    if metric == "profit_factor":
        return stats.profit_factor if stats.profit_factor != float("inf") else 999.0
    raise ValueError(f"unknown metric {metric!r}")


def walk_forward(
    cfg: Config,
    candles: list[Candle],
    grid: dict[str, list[Any]],
    train_bars: int,
    test_bars: int,
    min_trades: int = 10,
    metric: str = "expectancy_r",
) -> WalkForwardResult:
    combos = expand_grid(grid)
    configs = [(params, apply_overrides(cfg, params)) for params in combos]

    result = WalkForwardResult(grid_size=len(combos))
    all_is: list[float] = []
    all_oos: list[float] = []

    start = 0
    fold_index = 0
    while start + train_bars + test_bars <= len(candles):
        train = candles[start : start + train_bars]
        test = candles[start + train_bars : start + train_bars + test_bars]

        scored: list[tuple[float, dict[str, Any], Stats, Stats]] = []
        for params, candidate in configs:
            is_stats = run_backtest(candidate, train).stats
            oos_stats = run_backtest(candidate, test).stats
            # Both halves feed the correlation, but only the training score
            # is ever allowed to pick a winner.
            all_is.append(_score(is_stats, metric))
            all_oos.append(_score(oos_stats, metric))
            if is_stats.trades >= min_trades:
                scored.append((_score(is_stats, metric), params, is_stats, oos_stats))

        if scored:
            scored.sort(key=lambda row: -row[0])
            _, params, is_stats, oos_stats = scored[0]
            result.folds.append(
                Fold(
                    index=fold_index,
                    train_range=(start, start + train_bars),
                    test_range=(start + train_bars, start + train_bars + test_bars),
                    params=params,
                    in_sample=is_stats,
                    out_of_sample=oos_stats,
                )
            )
            result.oos_trades += oos_stats.trades
            result.oos_total_r += oos_stats.total_r
        else:
            result.skipped_folds += 1

        fold_index += 1
        start += test_bars

    if result.oos_trades:
        result.oos_expectancy = result.oos_total_r / result.oos_trades
    result.rank_correlation = spearman(all_is, all_oos)
    return result
