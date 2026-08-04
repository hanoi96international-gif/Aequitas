"""Performance statistics for a completed run.

Deliberately reported in R multiples as well as cash. Cash results are a
function of the position sizing you happened to pick; R is a property of the
strategy, and it is the number that tells you whether the edge is real.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .execution import Trade


@dataclass(slots=True)
class Stats:
    trades: int = 0
    wins: int = 0
    losses: int = 0
    win_rate: float = 0.0
    net_pnl: float = 0.0
    fees: float = 0.0
    return_pct: float = 0.0
    expectancy_r: float = 0.0
    total_r: float = 0.0
    avg_win_r: float = 0.0
    avg_loss_r: float = 0.0
    profit_factor: float = 0.0
    max_drawdown_pct: float = 0.0
    max_drawdown_cash: float = 0.0
    longest_losing_streak: int = 0
    avg_bars_held: float = 0.0
    sharpe: float = 0.0
    longs: int = 0
    shorts: int = 0
    exit_reasons: dict[str, int] = field(default_factory=dict)
    starting_equity: float = 0.0
    ending_equity: float = 0.0

    def format(self) -> str:
        if self.trades == 0:
            return "No trades taken."
        lines = [
            f"Trades            {self.trades}  ({self.longs} long / {self.shorts} short)",
            f"Win rate          {self.win_rate:.1f}%  ({self.wins}W / {self.losses}L)",
            f"Expectancy        {self.expectancy_r:+.3f} R per trade",
            f"Total R           {self.total_r:+.2f} R",
            f"Avg win / loss    {self.avg_win_r:+.2f} R / {self.avg_loss_r:+.2f} R",
            f"Profit factor     {self.profit_factor:.2f}",
            f"Net P&L           {self.net_pnl:+,.2f}  ({self.return_pct:+.2f}%)",
            f"Fees paid         {self.fees:,.2f}",
            f"Equity            {self.starting_equity:,.2f} -> {self.ending_equity:,.2f}",
            f"Max drawdown      {self.max_drawdown_pct:.2f}%  ({self.max_drawdown_cash:,.2f})",
            f"Losing streak     {self.longest_losing_streak}",
            f"Avg bars held     {self.avg_bars_held:.1f}",
            f"Sharpe (per trade) {self.sharpe:.2f}",
        ]
        if self.exit_reasons:
            reasons = ", ".join(
                f"{k}={v}" for k, v in sorted(self.exit_reasons.items(), key=lambda kv: -kv[1])
            )
            lines.append(f"Exits             {reasons}")
        return "\n".join(lines)


def compute(
    trades: list[Trade], equity_curve: list[tuple[int, float]], starting_equity: float
) -> Stats:
    stats = Stats(starting_equity=starting_equity, ending_equity=starting_equity)
    if equity_curve:
        stats.ending_equity = equity_curve[-1][1]
    stats.max_drawdown_pct, stats.max_drawdown_cash = _drawdown(equity_curve)
    if not trades:
        return stats

    rs = [t.r_multiple for t in trades]
    wins = [t for t in trades if t.pnl > 0]
    losses = [t for t in trades if t.pnl <= 0]

    stats.trades = len(trades)
    stats.wins = len(wins)
    stats.losses = len(losses)
    stats.win_rate = 100.0 * len(wins) / len(trades)
    stats.net_pnl = sum(t.pnl for t in trades)
    stats.fees = sum(t.fees for t in trades)
    stats.return_pct = 100.0 * stats.net_pnl / starting_equity if starting_equity else 0.0
    stats.total_r = sum(rs)
    stats.expectancy_r = stats.total_r / len(trades)
    stats.avg_win_r = sum(t.r_multiple for t in wins) / len(wins) if wins else 0.0
    stats.avg_loss_r = sum(t.r_multiple for t in losses) / len(losses) if losses else 0.0
    gross_win = sum(t.pnl for t in wins)
    gross_loss = abs(sum(t.pnl for t in losses))
    stats.profit_factor = gross_win / gross_loss if gross_loss > 0 else math.inf
    stats.longest_losing_streak = _longest_losing_streak(trades)
    stats.avg_bars_held = sum(t.bars_held for t in trades) / len(trades)
    stats.sharpe = _sharpe(rs)
    stats.longs = sum(1 for t in trades if t.direction == "long")
    stats.shorts = len(trades) - stats.longs
    for t in trades:
        stats.exit_reasons[t.exit_reason] = stats.exit_reasons.get(t.exit_reason, 0) + 1
    return stats


def _drawdown(curve: list[tuple[int, float]]) -> tuple[float, float]:
    peak = -math.inf
    worst_pct = 0.0
    worst_cash = 0.0
    for _, equity in curve:
        peak = max(peak, equity)
        if peak <= 0:
            continue
        dd_cash = peak - equity
        dd_pct = 100.0 * dd_cash / peak
        worst_pct = max(worst_pct, dd_pct)
        worst_cash = max(worst_cash, dd_cash)
    return worst_pct, worst_cash


def _longest_losing_streak(trades: list[Trade]) -> int:
    worst = current = 0
    for t in trades:
        if t.pnl <= 0:
            current += 1
            worst = max(worst, current)
        else:
            current = 0
    return worst


def _sharpe(rs: list[float]) -> float:
    """Per-trade Sharpe on R multiples — not annualised, and not pretending to be."""
    if len(rs) < 2:
        return 0.0
    mean = sum(rs) / len(rs)
    var = sum((r - mean) ** 2 for r in rs) / (len(rs) - 1)
    sd = math.sqrt(var)
    return mean / sd if sd > 0 else 0.0
