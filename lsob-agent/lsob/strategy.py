"""The LSOB engine: liquidity sweep → displacement → order block → signal.

The engine is strictly incremental. `on_bar` is called once per closed
candle and may only look at bars it has already been given, which is why the
backtest and the live agent can share it verbatim — there is no second code
path that could quietly drift.

The sequence it is looking for, in short form:

  1. a swing high/low leaves resting liquidity behind
  2. price raids that level and closes back inside it        → Sweep
  3. price then moves away hard, breaking structure          → displacement
  4. the last opposing candle before that move               → order block
  5. a limit order at the edge of that block, stop beyond
     the raid's extreme                                      → Signal

Every step is a config knob, because everyone's variant of this differs.
"""

from __future__ import annotations

from collections import deque
from dataclasses import dataclass, replace

from .bias import BiasFilter
from .config import Config
from .filters import SessionFilter, dealing_range
from .liquidity import LiquidityBook, Sweep
from .model import ATR, Candle
from .orderblock import (
    OrderBlock,
    displacement_atr,
    find_order_block,
    has_fvg,
    is_mitigated,
    retracement_level,
)
from .structure import MarketStructure, SwingDetector


@dataclass(frozen=True, slots=True)
class Signal:
    """A ready-to-place order: limit entry, hard invalidation, staged targets."""

    direction: str  # 'long' | 'short'
    index: int  # bar the signal was emitted on
    ts: int
    entry: float
    stop: float
    targets: list[float]
    weights: list[float]
    expires_at: int  # bar index after which the resting order is pulled
    risk: float  # per-unit distance from entry to stop
    reward_risk: float  # R at the final target
    order_block: OrderBlock
    sweep_level: float
    sweep_extreme: float
    sweep_touches: int
    displacement: float  # in ATR units
    atr: float
    # The far end of the displacement leg. With the raid extreme it defines
    # the span every retracement ratio is measured across, so a chart can
    # redraw the whole ladder without recomputing the setup.
    leg_extreme: float = 0.0

    @property
    def label(self) -> str:
        return (
            f"{self.direction.upper()} @ {self.entry:.6g} "
            f"SL {self.stop:.6g} TP {', '.join(f'{t:.6g}' for t in self.targets)} "
            f"({self.reward_risk:.2f}R)"
        )


@dataclass(slots=True)
class _Armed:
    """A built setup held back until inducement has been taken.

    The order block is not the first thing price reaches on its way back.
    A minor swing forms in front of it, and the stops behind that swing are
    the inducement — the liquidity that funds the move into the block. A tap
    that arrives without collecting it is early, and the block is far more
    likely to be run straight through.

    Held here rather than in the executor because the inducement forms
    *after* the setup is built and *before* the entry is offered, so nothing
    downstream is in a position to see it.
    """

    signal: Signal
    detector: SwingDetector
    inducement: float | None = None


@dataclass(slots=True)
class _Watch:
    """A completed sweep waiting to see whether displacement follows."""

    sweep: Sweep
    reference_level: float | None  # opposing swing whose break confirms the move
    bos_confirmed: bool = False


class LsobStrategy:
    """Stateful LSOB detector. Feed closed candles; collect signals."""

    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.atr = ATR(cfg.structure.atr_period)
        self.structure = MarketStructure(
            cfg.structure.swing_left,
            cfg.structure.swing_right,
            use_close=cfg.structure.bos_use_close,
        )
        self.book = LiquidityBook(cfg.liquidity)
        self.bias = BiasFilter(cfg)
        self.session = SessionFilter(
            cfg.filters.session_enabled,
            list(cfg.filters.session_windows),
            [int(d) for d in cfg.filters.session_days],
        )
        self.index = -1
        # Why setups were dropped, so `scan` can explain an empty result
        # instead of leaving you guessing which filter did it.
        self.rejections: dict[str, int] = {}

        # Bars are kept only as far back as the order-block search can reach,
        # plus the displacement window it may have to scan for an FVG.
        span = cfg.orderblock.ob_max_lookback + cfg.orderblock.displacement_max_bars + 4
        self._recent: deque[tuple[int, Candle]] = deque(maxlen=max(span, 8))
        self._watching: list[_Watch] = []
        self._armed: list[_Armed] = []
        self._last_signal_index: int | None = None

    def on_bar(self, candle: Candle) -> list[Signal]:
        """Process one closed candle and return any signals it produced."""
        self.index += 1
        i = self.index
        self._recent.append((i, candle))

        atr = self.atr.update(candle)
        self.bias.update(candle)

        swings, brk = self.structure.update(i, candle)
        if atr is None or atr <= 0:
            # No ATR yet: every threshold below is denominated in it, so there
            # is nothing meaningful to measure against.
            return []

        for swing in swings:
            self.book.add_swing(swing, atr)

        # A break of structure confirms any watch whose reference level it took.
        if brk is not None:
            for watch in self._watching:
                if watch.reference_level is None:
                    continue
                if watch.sweep.direction == "short" and brk.direction == "down":
                    if brk.level <= watch.reference_level:
                        watch.bos_confirmed = True
                elif watch.sweep.direction == "long" and brk.direction == "up":
                    if brk.level >= watch.reference_level:
                        watch.bos_confirmed = True

        for sweep in self.book.update(i, candle, atr):
            self._start_watch(sweep)

        signals = self._resolve_armed(i, candle)
        signals.extend(self._resolve_watches(i, candle, atr))
        return signals

    # ── internals ────────────────────────────────────────────────────────

    def _start_watch(self, sweep: Sweep) -> None:
        if sweep.direction == "long" and not self.cfg.risk.allow_long:
            return
        if sweep.direction == "short" and not self.cfg.risk.allow_short:
            return
        if sweep.direction == "short":
            ref = self.structure.last_low()
        else:
            ref = self.structure.last_high()
        self._watching.append(_Watch(sweep=sweep, reference_level=ref.price if ref else None))

    def _resolve_watches(self, i: int, candle: Candle, atr: float) -> list[Signal]:
        cfg = self.cfg
        signals: list[Signal] = []
        still_watching: list[_Watch] = []

        for watch in self._watching:
            sweep = watch.sweep
            age = i - sweep.index

            # The raid's extreme is the setup's premise. Closing beyond it says
            # the premise was wrong — this was continuation, not a rejection.
            if sweep.direction == "short" and candle.close > sweep.extreme:
                continue
            if sweep.direction == "long" and candle.close < sweep.extreme:
                continue
            if age > cfg.orderblock.displacement_max_bars:
                continue

            disp = displacement_atr(sweep.extreme, candle.close, sweep.direction, atr)
            if disp < cfg.orderblock.displacement_atr:
                still_watching.append(watch)
                continue
            if cfg.orderblock.require_bos and not watch.bos_confirmed:
                still_watching.append(watch)
                continue

            leg = [(idx, c) for idx, c in self._recent if idx >= sweep.pierce_index]
            if cfg.orderblock.require_fvg and not has_fvg(leg, sweep.direction):
                still_watching.append(watch)
                continue

            signal = self._build_signal(i, candle, atr, sweep, disp)
            if signal is None:
                still_watching.append(watch)
                continue
            if cfg.filters.require_inducement:
                self._armed.append(
                    _Armed(
                        signal=signal,
                        detector=SwingDetector(
                            cfg.filters.inducement_swing, cfg.filters.inducement_swing
                        ),
                    )
                )
                continue
            signals.append(signal)
            self._last_signal_index = i

        self._watching = still_watching
        return signals

    def _resolve_armed(self, i: int, candle: Candle) -> list[Signal]:
        """Offer armed setups once price has taken the inducement in front of them.

        Order within a bar matters: inducement is checked before the block is,
        because the ideal bar does both — it runs the minor swing and taps the
        block in one move, and that is a valid entry, not a voided one.
        """
        emitted: list[Signal] = []
        surviving: list[_Armed] = []

        for armed in self._armed:
            sig = armed.signal
            ob = sig.order_block
            short = sig.direction == "short"

            if i > sig.expires_at:
                self._drop("inducement_never_taken")
                continue

            for swing in armed.detector.update(candle):
                # Only swings between price and the block can be inducement;
                # anything past the block is on the far side of the entry.
                if short and swing.kind == "high" and swing.price < ob.bottom:
                    armed.inducement = swing.price
                elif not short and swing.kind == "low" and swing.price > ob.top:
                    armed.inducement = swing.price

            taken = armed.inducement is not None and (
                candle.high > armed.inducement if short else candle.low < armed.inducement
            )
            if taken:
                emitted.append(replace(sig, index=i, ts=candle.ts, expires_at=i + self.cfg.entry.valid_bars))
                self._last_signal_index = i
                continue

            reached_block = candle.high >= ob.bottom if short else candle.low <= ob.top
            if reached_block:
                self._drop("block_tapped_before_inducement")
                continue

            beyond_stop = candle.close >= sig.stop if short else candle.close <= sig.stop
            if beyond_stop:
                self._drop("invalidated_while_arming")
                continue

            surviving.append(armed)

        self._armed = surviving
        return emitted

    def _build_signal(
        self, i: int, candle: Candle, atr: float, sweep: Sweep, disp: float
    ) -> Signal | None:
        cfg = self.cfg
        if not self.bias.allows(sweep.direction):
            return self._drop("bias")
        if not self.session.allows(candle.ts):
            return self._drop("session")
        if self._last_signal_index is not None:
            if i - self._last_signal_index < cfg.risk.cooldown_bars:
                return self._drop("cooldown")

        first = sweep.pierce_index if cfg.orderblock.ob_include_sweep_candle else sweep.pierce_index + 1
        window = [
            (idx, c)
            for idx, c in self._recent
            if idx <= i and idx >= max(first, i - cfg.orderblock.ob_max_lookback)
        ]
        if not window:
            return None

        ob = find_order_block(window, sweep.direction, cfg.orderblock.zone_mode, candle.close)
        if ob is None:
            return self._drop("no_order_block")

        if cfg.filters.require_unmitigated:
            seen = [(idx, c) for idx, c in self._recent if idx <= i]
            if is_mitigated(seen, ob):
                return self._drop("mitigated")

        if cfg.filters.premium_discount:
            rng = dealing_range(
                [s.price for s in self.structure.highs],
                [s.price for s in self.structure.lows],
                cfg.filters.range_swings,
            )
            if rng is None or not rng.allows(
                sweep.direction, ob.edge(cfg.entry.edge), cfg.filters.pd_threshold
            ):
                return self._drop("premium_discount")

        leg = [c for idx, c in self._recent if sweep.pierce_index <= idx <= i]
        leg_extreme = (
            min(c.low for c in leg) if sweep.direction == "short"
            else max(c.high for c in leg)
        )
        if cfg.entry.edge == "retracement":
            entry = retracement_level(
                sweep.extreme, leg_extreme, sweep.direction, cfg.entry.retracement
            )
            if cfg.entry.retracement_in_block and not (ob.bottom <= entry <= ob.top):
                return self._drop("retracement_outside_block")
            # The limit has to sit on the far side of price, or it is not a
            # retracement entry — price is already past it.
            if sweep.direction == "short" and entry <= candle.close:
                return self._drop("retracement_behind_price")
            if sweep.direction == "long" and entry >= candle.close:
                return self._drop("retracement_behind_price")
        else:
            entry = ob.edge(cfg.entry.edge)
        buffer = cfg.entry.sl_buffer_atr * atr
        if sweep.direction == "short":
            anchor = sweep.extreme if cfg.entry.sl_anchor == "sweep_extreme" else ob.top
            stop = anchor + buffer
        else:
            anchor = sweep.extreme if cfg.entry.sl_anchor == "sweep_extreme" else ob.bottom
            stop = anchor - buffer

        risk = abs(entry - stop)
        if risk <= 0:
            return self._drop("degenerate_risk")

        targets = self._targets(sweep.direction, entry, risk)
        if not targets:
            return self._drop("no_targets")
        if any(t <= 0 for t in targets):
            # A short whose stop is far enough away can put an R-multiple
            # target below zero. Price cannot go there, so the setup cannot
            # pay what it promises and is not tradeable.
            return self._drop("target_below_zero")
        final_rr = abs(targets[-1] - entry) / risk
        if final_rr < cfg.entry.min_rr:
            return self._drop("below_min_rr")

        return Signal(
            direction=sweep.direction,
            index=i,
            ts=candle.ts,
            entry=entry,
            stop=stop,
            targets=targets,
            weights=list(cfg.entry.tp_weights),
            expires_at=i + cfg.entry.valid_bars,
            risk=risk,
            reward_risk=final_rr,
            order_block=ob,
            sweep_level=sweep.pool.price,
            sweep_extreme=sweep.extreme,
            sweep_touches=sweep.touches,
            displacement=disp,
            atr=atr,
            leg_extreme=leg_extreme,
        )

    def _drop(self, reason: str) -> None:
        """Record why a setup was discarded, and return None for the caller."""
        self.rejections[reason] = self.rejections.get(reason, 0) + 1
        return None

    def _targets(self, direction: str, entry: float, risk: float) -> list[float]:
        cfg = self.cfg.entry
        sign = -1.0 if direction == "short" else 1.0
        rr_targets = [entry + sign * r * risk for r in cfg.tp_rr]
        if cfg.tp_mode == "rr":
            return rr_targets

        # 'liquidity': aim the final target at the nearest untouched pool on
        # the other side, and fall back to the R target when there is none.
        pools = self.book.opposite_pools(direction)
        candidates = [
            p.price for p in pools if (p.price < entry if direction == "short" else p.price > entry)
        ]
        if not candidates:
            return rr_targets
        chosen = max(candidates) if direction == "short" else min(candidates)
        if abs(chosen - entry) / risk < cfg.min_rr:
            return rr_targets
        rr_targets[-1] = chosen
        return sorted(rr_targets, reverse=direction == "short")
