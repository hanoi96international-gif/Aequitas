"""Liquidity pools and sweep detection — the "LS" half of LSOB.

Resting stops sit just beyond obvious swing highs (buy-side liquidity, BSL)
and swing lows (sell-side liquidity, SSL). A *sweep* is price reaching
through such a level and then failing to hold there. The hard part is
telling that apart from a genuine breakout, which looks identical for one
bar; the two knobs that do the separating are `max_penetration_atr` (how far
beyond the level price may travel and still count as a raid) and
`reclaim_bars` (how quickly it must close back inside).
"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Candle
from .structure import Swing


@dataclass(slots=True)
class LiquidityPool:
    kind: str  # 'bsl' (above swing highs) | 'ssl' (below swing lows)
    price: float
    created_index: int
    touches: int = 1
    pierced_index: int | None = None
    pierce_extreme: float | None = None
    swept_index: int | None = None
    broken: bool = False

    @property
    def alive(self) -> bool:
        return not self.broken and self.swept_index is None


@dataclass(frozen=True, slots=True)
class Sweep:
    """A liquidity raid that has already been rejected."""

    pool: LiquidityPool
    index: int  # bar on which the reclaim completed
    ts: int
    pierce_index: int  # bar that first reached through the level
    extreme: float  # furthest price reached beyond the level
    direction: str  # 'short' (BSL swept) | 'long' (SSL swept)
    penetration_atr: float
    touches: int


@dataclass(slots=True)
class LiquidityConfig:
    equal_level_atr: float = 0.15
    min_penetration_atr: float = 0.0
    max_penetration_atr: float = 1.5
    reclaim_bars: int = 2
    pool_max_age_bars: int = 500
    min_touches: int = 1


class LiquidityBook:
    """Maintains live liquidity pools and reports sweeps as they complete."""

    __slots__ = ("cfg", "pools")

    def __init__(self, cfg: LiquidityConfig) -> None:
        self.cfg = cfg
        self.pools: list[LiquidityPool] = []

    def add_swing(self, swing: Swing, atr: float) -> None:
        """Register a confirmed pivot, merging equal levels into one pool.

        Equal highs are not two pools, they are one pool with more stops
        behind it — `touches` is what makes that visible to the strategy.
        """
        kind = "bsl" if swing.kind == "high" else "ssl"
        tolerance = self.cfg.equal_level_atr * atr
        for pool in self.pools:
            if pool.kind != kind or not pool.alive:
                continue
            if abs(pool.price - swing.price) <= tolerance:
                pool.touches += 1
                # Keep the level at the extreme of the cluster: that is where
                # the stops actually rest.
                if kind == "bsl":
                    pool.price = max(pool.price, swing.price)
                else:
                    pool.price = min(pool.price, swing.price)
                return
        self.pools.append(LiquidityPool(kind, swing.price, swing.index))

    def update(self, index: int, candle: Candle, atr: float) -> list[Sweep]:
        """Feed one bar; return sweeps that completed on it."""
        sweeps: list[Sweep] = []
        min_pen = self.cfg.min_penetration_atr * atr
        max_pen = self.cfg.max_penetration_atr * atr

        for pool in self.pools:
            if not pool.alive:
                continue

            if pool.kind == "bsl":
                reached = candle.high > pool.price + min_pen
                reclaimed = candle.close < pool.price
                extreme_now = candle.high
            else:
                reached = candle.low < pool.price - min_pen
                reclaimed = candle.close > pool.price
                extreme_now = candle.low

            if reached:
                if pool.pierced_index is None:
                    pool.pierced_index = index
                    pool.pierce_extreme = extreme_now
                elif pool.kind == "bsl":
                    pool.pierce_extreme = max(pool.pierce_extreme, extreme_now)
                else:
                    pool.pierce_extreme = min(pool.pierce_extreme, extreme_now)

            if pool.pierced_index is None:
                continue

            penetration = abs(pool.pierce_extreme - pool.price)
            if penetration > max_pen:
                # Price did not raid the level, it left through it.
                pool.broken = True
                continue

            if reclaimed:
                pool.swept_index = index
                sweeps.append(
                    Sweep(
                        pool=pool,
                        index=index,
                        ts=candle.ts,
                        pierce_index=pool.pierced_index,
                        extreme=pool.pierce_extreme,
                        direction="short" if pool.kind == "bsl" else "long",
                        penetration_atr=penetration / atr if atr > 0 else 0.0,
                        touches=pool.touches,
                    )
                )
            elif index - pool.pierced_index >= self.cfg.reclaim_bars:
                # Reached through and stayed there: accepted, not rejected.
                pool.broken = True

        self._prune(index)
        return [s for s in sweeps if s.touches >= self.cfg.min_touches]

    def opposite_pools(self, direction: str) -> list[LiquidityPool]:
        """Live pools a trade in `direction` would be travelling *toward*."""
        want = "ssl" if direction == "short" else "bsl"
        return [p for p in self.pools if p.alive and p.kind == want]

    def _prune(self, index: int) -> None:
        max_age = self.cfg.pool_max_age_bars
        self.pools = [
            p
            for p in self.pools
            if p.alive and (max_age <= 0 or index - p.created_index <= max_age)
        ]
