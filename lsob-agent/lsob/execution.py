"""Order and position management, shared by the backtest and the live agent.

Fill assumptions are deliberately pessimistic, because a backtest's job is
to talk you *out* of strategies. With only OHLC per bar we cannot know the
path price took inside it, so wherever the bar allows two readings this
picks the one that hurts:

  * a bar that could have hit both the stop and a target is scored as the
    stop, at every stage of the trade
  * a limit entry only fills on a genuine trade-through of its price; a bar
    that merely touches it fills at the limit, never better, except when the
    bar gapped past it and the fill is marked to the open
  * entry and exit both pay fees, and market exits pay slippage on top

If a result only survives when those are relaxed, it is not a result.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .config import Config
from .model import Candle
from .strategy import Signal


@dataclass(slots=True)
class PendingOrder:
    signal: Signal
    placed_index: int

    @property
    def direction(self) -> str:
        return self.signal.direction


@dataclass(slots=True)
class Fill:
    index: int
    ts: int
    price: float
    qty: float
    reason: str  # 'entry' | 'tp1'.. | 'stop' | 'expiry'


@dataclass(slots=True)
class Position:
    signal: Signal
    entry_price: float
    entry_index: int
    qty: float
    remaining: float
    stop: float
    targets: list[float]
    weights: list[float]
    filled_targets: int = 0
    fees_paid: float = 0.0
    realized: float = 0.0
    fills: list[Fill] = field(default_factory=list)

    @property
    def direction(self) -> str:
        return self.signal.direction

    @property
    def risk_per_unit(self) -> float:
        return abs(self.entry_price - self.signal.stop)


@dataclass(slots=True)
class Trade:
    direction: str
    entry_ts: int
    exit_ts: int
    entry_index: int
    exit_index: int
    entry_price: float
    exit_price: float
    qty: float
    pnl: float  # net of fees and slippage
    fees: float
    r_multiple: float
    exit_reason: str
    bars_held: int
    reward_risk: float
    displacement: float
    sweep_touches: int
    fills: list[Fill] = field(default_factory=list)


class Executor:
    """Turns signals into positions and positions into trades.

    Holds the equity curve, so it is also the thing that knows how big a
    position may be.
    """

    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.equity = cfg.risk.starting_equity
        self.pending: list[PendingOrder] = []
        self.positions: list[Position] = []
        self.trades: list[Trade] = []
        self.equity_curve: list[tuple[int, float]] = []
        self.rejected: dict[str, int] = {}

    # ── intake ───────────────────────────────────────────────────────────

    def on_signal(self, index: int, signal: Signal) -> bool:
        """Place a resting limit order, unless a risk rule says otherwise."""
        # A working order counts against the limit as much as an open
        # position does — otherwise `max_concurrent = 1` would still let a
        # queue of resting orders all fill on the same impulsive bar.
        if len(self.positions) + len(self.pending) >= self.cfg.risk.max_concurrent:
            self._reject("max_concurrent")
            return False
        if self.equity <= 0:
            self._reject("no_equity")
            return False
        self.pending.append(PendingOrder(signal=signal, placed_index=index))
        return True

    def _reject(self, reason: str) -> None:
        self.rejected[reason] = self.rejected.get(reason, 0) + 1

    # ── per-bar processing ───────────────────────────────────────────────

    def on_bar(self, index: int, candle: Candle) -> list[Trade]:
        """Advance every working order and open position by one bar.

        Open positions are handled *before* pending fills so that an order
        filling on this bar is not also exited on it — a same-bar round trip
        is an artefact of bar granularity, not a trade anyone could take.
        """
        closed = self._manage_positions(index, candle)
        self._manage_pending(index, candle)
        self.equity_curve.append((index, self.mark_to_market(candle)))
        return closed

    def mark_to_market(self, candle: Candle) -> float:
        unrealized = 0.0
        for pos in self.positions:
            sign = 1.0 if pos.direction == "long" else -1.0
            unrealized += sign * (candle.close - pos.entry_price) * pos.remaining
        return self.equity + unrealized

    def _manage_pending(self, index: int, candle: Candle) -> None:
        survivors: list[PendingOrder] = []
        for order in self.pending:
            sig = order.signal
            if index <= sig.index:
                survivors.append(order)
                continue

            # Invalidation outranks the fill: if the bar closed beyond the
            # stop, the setup was wrong regardless of whether price passed
            # through the entry on the way there.
            invalidated = (
                candle.close >= sig.stop if sig.direction == "short" else candle.close <= sig.stop
            )
            if invalidated:
                continue

            fill_price = self._limit_fill(candle, sig)
            if fill_price is not None:
                if len(self.positions) < self.cfg.risk.max_concurrent:
                    self._open(index, candle, sig, fill_price)
                else:
                    self._reject("filled_but_at_capacity")
                continue

            if index >= sig.expires_at:
                self._reject("expired")
                continue
            survivors.append(order)
        self.pending = survivors

    @staticmethod
    def _limit_fill(candle: Candle, sig: Signal) -> float | None:
        """Fill price for a resting limit order, or None if it did not fill."""
        if sig.direction == "short":
            if candle.open >= sig.entry:
                return candle.open  # gapped through: filled at the open, not better
            return sig.entry if candle.high >= sig.entry else None
        if candle.open <= sig.entry:
            return candle.open
        return sig.entry if candle.low <= sig.entry else None

    def _open(self, index: int, candle: Candle, sig: Signal, price: float) -> None:
        risk_per_unit = abs(price - sig.stop)
        if risk_per_unit <= 0:
            return
        risk_cash = self.equity * (self.cfg.risk.risk_pct / 100.0)
        qty = risk_cash / risk_per_unit
        cap = self.equity * (self.cfg.risk.max_position_pct / 100.0)
        if price > 0 and qty * price > cap:
            qty = cap / price
        if qty <= 0:
            self._reject("size_zero")
            return

        fee = qty * price * (self.cfg.costs.maker_fee_bps / 10_000.0)
        self.equity -= fee
        pos = Position(
            signal=sig,
            entry_price=price,
            entry_index=index,
            qty=qty,
            remaining=qty,
            stop=sig.stop,
            targets=list(sig.targets),
            weights=list(sig.weights),
            fees_paid=fee,
        )
        pos.fills.append(Fill(index, candle.ts, price, qty, "entry"))
        self.positions.append(pos)

    def _manage_positions(self, index: int, candle: Candle) -> list[Trade]:
        closed: list[Trade] = []
        survivors: list[Position] = []
        for pos in self.positions:
            if index <= pos.entry_index:
                survivors.append(pos)
                continue
            trade = self._step_position(index, candle, pos)
            if trade is not None:
                closed.append(trade)
                self.trades.append(trade)
            else:
                survivors.append(pos)
        self.positions = survivors
        return closed

    def _step_position(self, index: int, candle: Candle, pos: Position) -> Trade | None:
        long = pos.direction == "long"
        stop_hit = candle.low <= pos.stop if long else candle.high >= pos.stop

        # Worst case first: a bar that touched the stop is scored as stopped,
        # even if it also traded through a target.
        if stop_hit:
            self._exit(index, candle, pos, pos.stop, pos.remaining, "stop", taker=True)
            return self._close(index, candle, pos, "stop")

        while pos.filled_targets < len(pos.targets):
            target = pos.targets[pos.filled_targets]
            reached = candle.high >= target if long else candle.low <= target
            if not reached:
                break
            weight = pos.weights[pos.filled_targets]
            qty = pos.qty * weight
            qty = min(qty, pos.remaining)
            pos.filled_targets += 1
            self._exit(
                index, candle, pos, target, qty, f"tp{pos.filled_targets}", taker=False
            )
            be_after = self.cfg.entry.breakeven_after_tp
            if be_after and pos.filled_targets >= be_after and pos.remaining > 0:
                pos.stop = pos.entry_price
            if pos.remaining <= 1e-12:
                return self._close(index, candle, pos, f"tp{pos.filled_targets}")
        return None

    def _exit(
        self,
        index: int,
        candle: Candle,
        pos: Position,
        price: float,
        qty: float,
        reason: str,
        taker: bool,
    ) -> None:
        if taker:
            slip = price * (self.cfg.costs.slippage_bps / 10_000.0)
            price = price - slip if pos.direction == "long" else price + slip
        fee_bps = self.cfg.costs.taker_fee_bps if taker else self.cfg.costs.maker_fee_bps
        fee = qty * price * (fee_bps / 10_000.0)
        sign = 1.0 if pos.direction == "long" else -1.0
        gross = sign * (price - pos.entry_price) * qty

        pos.realized += gross
        pos.fees_paid += fee
        pos.remaining -= qty
        self.equity += gross - fee
        pos.fills.append(Fill(index, candle.ts, price, qty, reason))

    def _close(self, index: int, candle: Candle, pos: Position, reason: str) -> Trade:
        exits = [f for f in pos.fills if f.reason != "entry"]
        exit_qty = sum(f.qty for f in exits) or pos.qty
        avg_exit = sum(f.price * f.qty for f in exits) / exit_qty if exits else pos.entry_price
        net = pos.realized - pos.fees_paid
        risk_cash = pos.risk_per_unit * pos.qty
        return Trade(
            direction=pos.direction,
            entry_ts=pos.fills[0].ts,
            exit_ts=candle.ts,
            entry_index=pos.entry_index,
            exit_index=index,
            entry_price=pos.entry_price,
            exit_price=avg_exit,
            qty=pos.qty,
            pnl=net,
            fees=pos.fees_paid,
            r_multiple=net / risk_cash if risk_cash > 0 else 0.0,
            exit_reason=reason,
            bars_held=index - pos.entry_index,
            reward_risk=pos.signal.reward_risk,
            displacement=pos.signal.displacement,
            sweep_touches=pos.signal.sweep_touches,
            fills=list(pos.fills),
        )

    def force_close_all(self, index: int, candle: Candle) -> list[Trade]:
        """Flatten at the last close — used at the end of a backtest run."""
        closed: list[Trade] = []
        for pos in list(self.positions):
            self._exit(index, candle, pos, candle.close, pos.remaining, "eod", taker=True)
            closed.append(self._close(index, candle, pos, "end_of_data"))
        self.trades.extend(closed)
        self.positions = []
        self.pending = []
        return closed
