"""Backtest driver: walk the candles once, in order, and score the result."""

from __future__ import annotations

from dataclasses import dataclass, field

from .config import Config
from .execution import Executor, Trade
from .metrics import Stats, compute
from .model import Candle
from .strategy import LsobStrategy, Signal


@dataclass(slots=True)
class BacktestResult:
    stats: Stats
    trades: list[Trade]
    signals: list[Signal]
    equity_curve: list[tuple[int, float]]
    rejected: dict[str, int] = field(default_factory=dict)
    bars: int = 0
    capped_entries: int = 0
    worst_risk_fraction: float = 1.0

    def format(self) -> str:
        head = f"Bars {self.bars}   Signals {len(self.signals)}   Filled {len(self.trades)}"
        body = self.stats.format()
        if self.rejected:
            skipped = ", ".join(f"{k}={v}" for k, v in sorted(self.rejected.items()))
            body += f"\nSignals not traded {skipped}"
        if self.capped_entries:
            body += (
                f"\n\nWARNING: risk.max_position_pct capped {self.capped_entries} of "
                f"{len(self.trades)} entries. Those trades risked as little as "
                f"{self.worst_risk_fraction * 100:.0f}% of risk_pct, so R multiples are "
                f"not comparable across trades and the expectancy above overstates the "
                f"cash result. Raise max_position_pct (leveraged markets) or lower "
                f"risk_pct until nothing is capped."
            )
        return f"{head}\n{'-' * len(head)}\n{body}"


def run_backtest(cfg: Config, candles: list[Candle]) -> BacktestResult:
    """Feed `candles` through the strategy and executor a single bar at a time.

    Order within a bar matters and is fixed here: the executor sees the bar
    first (so fills and exits are resolved against price action the strategy
    has not yet reacted to), then the strategy emits signals from the closed
    bar, which can only be filled from the *next* bar onwards.
    """
    strategy = LsobStrategy(cfg)
    executor = Executor(cfg)
    signals: list[Signal] = []

    for index, candle in enumerate(candles):
        executor.on_bar(index, candle)
        for signal in strategy.on_bar(candle):
            signals.append(signal)
            executor.on_signal(index, signal)

    if candles:
        executor.force_close_all(len(candles) - 1, candles[-1])

    stats = compute(executor.trades, executor.equity_curve, cfg.risk.starting_equity)
    return BacktestResult(
        stats=stats,
        trades=executor.trades,
        signals=signals,
        equity_curve=executor.equity_curve,
        rejected=dict(executor.rejected),
        bars=len(candles),
        capped_entries=executor.capped_entries,
        worst_risk_fraction=executor.worst_risk_fraction,
    )


def scan_signals(cfg: Config, candles: list[Candle]) -> list[Signal]:
    """Run the detector alone — no execution, no sizing. Useful for eyeballing."""
    strategy = LsobStrategy(cfg)
    out: list[Signal] = []
    for candle in candles:
        out.extend(strategy.on_bar(candle))
    return out
