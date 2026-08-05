"""Backtest driver: walk the candles once, in order, and score the result."""

from __future__ import annotations

from dataclasses import dataclass, field

from .config import Config
from .daytrade import DayClock
from .execution import Executor, Trade
from .intrabar import IntrabarIndex
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
    stopped_on_entry_bar: int = 0
    resolved_by_intrabar: int = 0
    worst_risk_fraction: float = 1.0
    cost_r: list[float] = field(default_factory=list)
    overnight: int = 0
    daytrade: bool = False

    def format(self) -> str:
        head = f"Bars {self.bars}   Signals {len(self.signals)}   Filled {len(self.trades)}"
        body = self.stats.format()
        if self.rejected:
            skipped = ", ".join(f"{k}={v}" for k, v in sorted(self.rejected.items()))
            body += f"\nSignals not traded {skipped}"
        if self.stopped_on_entry_bar and self.trades:
            share = 100.0 * self.stopped_on_entry_bar / len(self.trades)
            body += (
                f"\nStopped on the fill bar {self.stopped_on_entry_bar} "
                f"({share:.0f}% of trades)"
            )
            if share > 25.0:
                body += (
                    " — these are real stops, not artefacts: the entry sits between "
                    "the bar's open and the stop, so price had to cross it first. "
                    "A high share means the stop is tight relative to bar range, "
                    "which is a fact about the strategy rather than the backtest."
                )
        if self.cost_r:
            ranked = sorted(self.cost_r)
            median = ranked[len(ranked) // 2]
            body += (
                f"\nCost per round trip {median:.2f} R median "
                f"({ranked[0]:.2f}-{ranked[-1]:.2f})"
            )
            if median >= 0.25:
                body += (
                    f" — fees and slippage eat {median * 100:.0f}% of the risk before the "
                    f"market has an opinion. On this timeframe the strategy is paying a "
                    f"toll it has to out-earn; either the stop is too tight for the venue "
                    f"or the venue is too expensive for the stop. `costs.max_cost_r` "
                    f"rejects the worst of them."
                )
        if self.trades:
            share = 100.0 * self.overnight / len(self.trades)
            body += f"\nHeld overnight     {self.overnight} ({share:.0f}% of trades)"
            if self.overnight and not self.daytrade:
                body += " — [daytrade] is off, so gap risk is in these numbers"
        if self.resolved_by_intrabar:
            body += (
                f"\nResolved with finer candles {self.resolved_by_intrabar} "
                f"ambiguous bar(s) — decided by evidence rather than assumption"
            )
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


def run_backtest(
    cfg: Config, candles: list[Candle], intrabar: IntrabarIndex | None = None
) -> BacktestResult:
    """Feed `candles` through the strategy and executor a single bar at a time.

    Order within a bar matters and is fixed here: the executor sees the bar
    first (so fills and exits are resolved against price action the strategy
    has not yet reacted to), then the strategy emits signals from the closed
    bar, which can only be filled from the *next* bar onwards.
    """
    strategy = LsobStrategy(cfg)
    executor = Executor(cfg, intrabar)
    signals: list[Signal] = []

    for index, candle in enumerate(candles):
        executor.on_bar(index, candle)
        for signal in strategy.on_bar(candle):
            signals.append(signal)
            executor.on_signal(index, signal)

    if candles:
        executor.force_close_all(len(candles) - 1, candles[-1])

    stats = compute(executor.trades, executor.equity_curve, cfg.risk.starting_equity)
    day = DayClock.day_key
    overnight = sum(1 for t in executor.trades if day(t.entry_ts) != day(t.exit_ts))
    return BacktestResult(
        stats=stats,
        trades=executor.trades,
        signals=signals,
        equity_curve=executor.equity_curve,
        rejected=dict(executor.rejected),
        bars=len(candles),
        capped_entries=executor.capped_entries,
        stopped_on_entry_bar=executor.stopped_on_entry_bar,
        resolved_by_intrabar=executor.resolved_by_intrabar,
        worst_risk_fraction=executor.worst_risk_fraction,
        cost_r=[c for c in executor.cost_r_values if c != float("inf")],
        overnight=overnight,
        daytrade=cfg.daytrade.enabled,
    )


def scan_signals(cfg: Config, candles: list[Candle]) -> list[Signal]:
    """Run the detector alone — no execution, no sizing. Useful for eyeballing."""
    strategy = LsobStrategy(cfg)
    out: list[Signal] = []
    for candle in candles:
        out.extend(strategy.on_bar(candle))
    return out
