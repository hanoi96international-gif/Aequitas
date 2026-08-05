"""Position sizing: the one part of this that is arithmetic, not prediction.

Whether a setup has an edge is a forecast, and the walk-forward tooling
exists because forecasts of that kind are mostly wishful. How deep a
drawdown a given risk-per-trade produces is *not* a forecast — given a
distribution of outcomes, it follows. That makes sizing the only lever here
that can be set safely without first being right about the market.

Two things this makes visible, both of which are counter-intuitive until
you see the numbers:

  * drawdown scales faster than risk. Doubling risk-per-trade more than
    doubles the drawdown you should expect to sit through
  * losing streaks are longer than intuition says. A 40%-win-rate strategy
    will hand you eight losses in a row often enough to matter, and the
    account has to survive that without the position size collapsing

Outcomes are resampled from R multiples you actually observed, so the
simulation inherits the real shape of the results — including the fat left
tail that a win-rate-and-payoff summary throws away.
"""

from __future__ import annotations

import random
import statistics
from dataclasses import dataclass


@dataclass(slots=True)
class SizingReport:
    risk_pct: float
    trades: int
    runs: int
    median_return_pct: float
    p05_return_pct: float
    median_max_dd_pct: float
    p95_max_dd_pct: float
    worst_max_dd_pct: float
    prob_dd_over_20: float
    prob_dd_over_50: float
    prob_loss: float
    median_longest_losing_streak: int

    def row(self) -> str:
        return (
            f"{self.risk_pct:>6.2f}%{self.median_return_pct:>12.1f}%"
            f"{self.median_max_dd_pct:>11.1f}%{self.p95_max_dd_pct:>11.1f}%"
            f"{self.prob_dd_over_20 * 100:>10.0f}%{self.prob_dd_over_50 * 100:>10.0f}%"
            f"{self.prob_loss * 100:>10.0f}%"
        )


def simulate(
    r_multiples: list[float],
    risk_pct: float,
    trades: int = 200,
    runs: int = 5_000,
    seed: int = 1,
) -> SizingReport:
    """Bootstrap `runs` alternative histories of `trades` trades.

    Each trade risks `risk_pct` of *current* equity, which is what a
    percent-risk rule actually does — the compounding is the point, and it
    is also why the downside is not symmetric with the upside.
    """
    if not r_multiples:
        raise ValueError("need at least one observed R multiple to resample from")
    if risk_pct <= 0:
        raise ValueError("risk_pct must be positive")

    rng = random.Random(seed)
    fraction = risk_pct / 100.0

    returns: list[float] = []
    max_dds: list[float] = []
    streaks: list[int] = []
    losses = 0
    dd_over_20 = 0
    dd_over_50 = 0

    for _ in range(runs):
        equity = 1.0
        peak = 1.0
        worst_dd = 0.0
        streak = longest_streak = 0

        for _ in range(trades):
            r = r_multiples[rng.randrange(len(r_multiples))]
            equity *= 1.0 + fraction * r
            if equity <= 0:
                equity = 1e-12
                worst_dd = 100.0
                break
            peak = max(peak, equity)
            worst_dd = max(worst_dd, 100.0 * (peak - equity) / peak)
            if r <= 0:
                streak += 1
                longest_streak = max(longest_streak, streak)
            else:
                streak = 0

        returns.append(100.0 * (equity - 1.0))
        max_dds.append(worst_dd)
        streaks.append(longest_streak)
        if equity < 1.0:
            losses += 1
        if worst_dd > 20.0:
            dd_over_20 += 1
        if worst_dd > 50.0:
            dd_over_50 += 1

    ordered_dd = sorted(max_dds)
    ordered_ret = sorted(returns)
    return SizingReport(
        risk_pct=risk_pct,
        trades=trades,
        runs=runs,
        median_return_pct=statistics.median(returns),
        p05_return_pct=ordered_ret[int(0.05 * len(ordered_ret))],
        median_max_dd_pct=statistics.median(max_dds),
        p95_max_dd_pct=ordered_dd[int(0.95 * len(ordered_dd))],
        worst_max_dd_pct=ordered_dd[-1],
        prob_dd_over_20=dd_over_20 / runs,
        prob_dd_over_50=dd_over_50 / runs,
        prob_loss=losses / runs,
        median_longest_losing_streak=int(statistics.median(streaks)),
    )


def sweep(
    r_multiples: list[float],
    risk_levels: list[float],
    trades: int = 200,
    runs: int = 5_000,
    seed: int = 1,
) -> list[SizingReport]:
    return [simulate(r_multiples, r, trades, runs, seed) for r in risk_levels]


def format_sweep(reports: list[SizingReport], sample_size: int) -> str:
    if not reports:
        return "Nothing to simulate."
    head = reports[0]
    lines = [
        f"Resampled from {sample_size} observed trades, "
        f"{head.trades} trades per run, {head.runs:,} runs",
        "",
        f"{'risk':>7}{'median ret':>12}{'med DD':>11}{'95th DD':>11}"
        f"{'DD>20%':>10}{'DD>50%':>10}{'P(loss)':>10}",
    ]
    lines.extend(r.row() for r in reports)
    lines.append("")
    lines.append(
        f"Median longest losing streak: {head.median_longest_losing_streak} trades in a row"
    )
    if sample_size < 30:
        lines.append(
            f"\nWARNING: {sample_size} trades is too small a sample to describe the "
            "distribution. Treat these numbers as illustrative of the mechanism, "
            "not as a forecast for this strategy."
        )
    return "\n".join(lines)
