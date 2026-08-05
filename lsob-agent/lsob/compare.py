"""Run one configuration across several markets without touching it.

A configuration that was arrived at on one market has been shaped by that
market, whether or not anyone tuned it deliberately — every rejected variant
along the way was a choice informed by the same data. The only check that
sees through this is running the result, unaltered, somewhere it has never
been.

The rule this module enforces is that nothing may be adjusted per market
except the two things that are properties of the venue rather than of the
strategy: the timeframe of the file, and the fees. Changing anything else
would make the comparison a second round of fitting.
"""

from __future__ import annotations

import copy
from dataclasses import dataclass

from .backtest import run_backtest
from .config import Config
from .data import clean_candles, load_any
from .metrics import Stats

# Below this many trades a market result describes its sample, not the
# strategy. The walk-forward uses the same floor, for the same reason.
MIN_TRADES = 30


@dataclass(slots=True)
class MarketRun:
    name: str
    bars: int
    signals: int
    stats: Stats

    @property
    def conclusive(self) -> bool:
        return self.stats.trades >= MIN_TRADES


@dataclass(slots=True)
class Comparison:
    runs: list[MarketRun]

    def format(self) -> str:
        if not self.runs:
            return "No markets to compare."
        lines = [
            f"{'market':<26}{'bars':>8}{'signals':>9}{'trades':>8}"
            f"{'exp R':>9}{'win%':>7}{'PF':>7}",
        ]
        for run in self.runs:
            s = run.stats
            flag = "" if run.conclusive else "  *"
            lines.append(
                f"{run.name[:26]:<26}{run.bars:>8}{run.signals:>9}{s.trades:>8}"
                f"{s.expectancy_r:>+9.3f}{s.win_rate:>7.1f}{s.profit_factor:>7.2f}{flag}"
            )

        thin = [r for r in self.runs if not r.conclusive]
        if thin:
            lines.append("")
            lines.append(
                f"  * fewer than {MIN_TRADES} trades — describes the sample, not the strategy"
            )
        lines.append("")
        lines.append(self._verdict())
        return "\n".join(lines)

    def _verdict(self) -> str:
        conclusive = [r for r in self.runs if r.conclusive]
        if len(conclusive) < 2:
            return (
                "Only one market carries enough trades to judge. That is the same "
                "evidence as before, not a second opinion — find more data before "
                "reading anything into the agreement."
            )
        positive = [r for r in conclusive if r.stats.expectancy_r > 0]
        if len(positive) == len(conclusive):
            return (
                "Positive on every market with a usable sample. That is what a "
                "transferable edge looks like; confirm it forward before sizing up."
            )
        if not positive:
            return "Negative on every market with a usable sample."
        names = ", ".join(r.name for r in positive)
        return (
            f"Positive only on {names}. A rule that works in one venue and not "
            f"others is describing that venue, and the first market is the one it "
            f"was built on."
        )


def compare(cfg: Config, markets: list[tuple[str, str, str, tuple[float, float, float]]]) -> Comparison:
    """Score `cfg` on each (name, csv, timeframe, (maker, taker, slippage)).

    Only the timeframe and the costs are allowed to vary — see the module
    docstring. Everything the strategy does is held fixed.
    """
    runs: list[MarketRun] = []
    for name, path, timeframe, (maker, taker, slippage) in markets:
        market_cfg = copy.deepcopy(cfg)
        market_cfg.data.csv = path
        market_cfg.market.timeframe = timeframe
        market_cfg.costs.maker_fee_bps = maker
        market_cfg.costs.taker_fee_bps = taker
        market_cfg.costs.slippage_bps = slippage

        candles = clean_candles(
            load_any(path), market_cfg.data.spike_ratio, market_cfg.data.jump_ratio
        )
        result = run_backtest(market_cfg, candles)
        runs.append(
            MarketRun(name=name, bars=len(candles), signals=len(result.signals), stats=result.stats)
        )
    return Comparison(runs=runs)
