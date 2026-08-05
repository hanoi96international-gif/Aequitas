"""Brokers: a paper broker that reuses the backtest executor, and a live one.

The paper broker is not a simulation *of* the backtester — it is the same
`Executor` class, driven by live candles instead of historical ones. If paper
and backtest disagree, the cause is the data, not two drifting code paths.

The live broker is off unless three separate things all say yes: the config
sets `live.enabled` and `live.mode = "live"`, API credentials are present in
the environment, and the operator passes `--live` on the command line. It
defaults to the exchange's sandbox even then.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass

from .config import Config
from .daytrade import day_clock
from .execution import Executor, Trade
from .model import Candle
from .strategy import Signal

log = logging.getLogger("lsob.broker")


class PaperBroker:
    """Executes against closed candles with the backtest's fill assumptions."""

    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.executor = Executor(cfg)

    @property
    def equity(self) -> float:
        return self.executor.equity

    def on_bar(self, index: int, candle: Candle) -> list[Trade]:
        return self.executor.on_bar(index, candle)

    def on_signal(self, index: int, signal: Signal) -> bool:
        return self.executor.on_signal(index, signal)

    def describe(self) -> str:
        return f"paper (equity {self.executor.equity:,.2f})"


@dataclass(slots=True)
class LiveOrderSet:
    """The exchange-side ids belonging to one signal."""

    entry_id: str
    signal: Signal
    stop_id: str | None = None
    target_ids: list[str] | None = None
    filled: bool = False
    filled_qty: float = 0.0
    filled_index: int | None = None


class LiveBroker:
    """Places real orders through ccxt.

    Scope, stated plainly so it is not mistaken for more than it is: this
    places a limit entry, then a reduce-only stop and take-profit once the
    entry reports as filled, cancels working entries that expire, and — when
    `[daytrade]` is on — closes out by the clock. It does not attempt
    partial-fill reconciliation across restarts, and it will not manage
    positions it did not open. Run it on a testnet until you have watched it
    handle a full trade, twice.

    The day-trading rules are duplicated here rather than borrowed from the
    `Executor`, because this class does not own the position — the exchange
    does. What is shared is the `DayClock` that decides *when*; only the
    resulting orders differ.
    """

    def __init__(self, cfg: Config, confirmed: bool) -> None:
        if not cfg.live.enabled or cfg.live.mode != "live":
            raise RuntimeError("live trading requires live.enabled = true and live.mode = 'live'")
        if not confirmed:
            raise RuntimeError("live trading requires the explicit --live flag")

        try:
            import ccxt  # type: ignore[import-untyped]
        except ImportError as exc:  # pragma: no cover - depends on environment
            raise RuntimeError("live trading requires ccxt — `pip install ccxt`") from exc

        key = os.environ.get(cfg.live.api_key_env, "").strip()
        secret = os.environ.get(cfg.live.api_secret_env, "").strip()
        if not key or not secret:
            raise RuntimeError(
                f"missing credentials: set {cfg.live.api_key_env} and {cfg.live.api_secret_env}"
            )

        self.cfg = cfg
        self.exchange = getattr(ccxt, cfg.market.exchange)(
            {"apiKey": key, "secret": secret, "enableRateLimit": True}
        )
        if cfg.live.sandbox:
            self.exchange.set_sandbox_mode(True)
        self.symbol = cfg.market.symbol
        self.working: list[LiveOrderSet] = []
        self.clock = day_clock(cfg)
        self.entries_by_day: dict[int, int] = {}

    def describe(self) -> str:
        venue = "SANDBOX" if self.cfg.live.sandbox else "REAL MONEY"
        return f"live {self.cfg.market.exchange} {self.symbol} [{venue}]"

    @property
    def equity(self) -> float:
        try:
            balance = self.exchange.fetch_balance()
        except Exception as exc:  # noqa: BLE001 - any venue error must not kill the loop
            log.warning("could not fetch balance: %s", exc)
            return 0.0
        quote = self.symbol.split("/")[-1].split(":")[0]
        return float(balance.get("total", {}).get(quote, 0.0) or 0.0)

    def on_signal(self, index: int, signal: Signal) -> bool:
        if self.clock.entries_closed(signal.ts):
            log.info("signal skipped: past the day's entry cutoff")
            return False
        day = self.clock.day_key(signal.ts)
        if self.clock.day_full(self.entries_by_day.get(day, 0)):
            log.info("signal skipped: daytrade.max_trades_per_day reached")
            return False
        if len(self.working) >= self.cfg.risk.max_concurrent:
            log.info("signal skipped: already at max_concurrent")
            return False
        qty = self._size(signal)
        if qty <= 0:
            log.warning("signal skipped: computed size is zero")
            return False
        side = "sell" if signal.direction == "short" else "buy"
        try:
            order = self.exchange.create_limit_order(self.symbol, side, qty, signal.entry)
        except Exception as exc:  # noqa: BLE001
            log.error("entry order rejected: %s", exc)
            return False
        log.info("placed %s limit %s @ %s", side, qty, signal.entry)
        self.working.append(LiveOrderSet(entry_id=str(order["id"]), signal=signal))
        return True

    def on_bar(self, index: int, candle: Candle) -> list[Trade]:
        """Poll working orders, arm protection on fills, cancel on expiry."""
        self._clock_rules(index, candle)
        for order_set in list(self.working):
            try:
                status = self.exchange.fetch_order(order_set.entry_id, self.symbol)
            except Exception as exc:  # noqa: BLE001
                log.warning("could not fetch order %s: %s", order_set.entry_id, exc)
                continue

            state = status.get("status")
            if state == "closed" and not order_set.filled:
                order_set.filled = True
                order_set.filled_index = index
                order_set.filled_qty = float(status.get("filled") or 0.0)
                day = self.clock.day_key(candle.ts)
                self.entries_by_day[day] = self.entries_by_day.get(day, 0) + 1
                self._arm_protection(order_set, order_set.filled_qty)
            elif state == "canceled":
                self.working.remove(order_set)
            elif not order_set.filled and index >= order_set.signal.expires_at:
                self._cancel(order_set)
        return []

    def _clock_rules(self, index: int, candle: Candle) -> None:
        """Apply the day-trading rules to real orders, before polling them."""
        if not self.clock.enabled:
            return
        flatten = self.clock.must_flatten(candle.ts)
        for order_set in list(self.working):
            if not order_set.filled:
                if flatten and self.cfg.daytrade.cancel_orders_at_cutoff:
                    self._cancel(order_set, "session end")
                continue
            held = index - (order_set.filled_index if order_set.filled_index is not None else index)
            if flatten:
                self._flatten(order_set, "session end")
            elif self.clock.timed_out(held):
                self._flatten(order_set, "time stop")

    def _flatten(self, order_set: LiveOrderSet, why: str) -> None:
        """Cancel the protective orders and close the position at market.

        The closing order is reduce-only and sized to the recorded fill. If a
        take-profit already took part of the position, reduce-only is what
        stops the remainder from flipping into a new position in the other
        direction — the exchange caps it at what is actually open. Sizing this
        from a stale local number without that flag would be how a flatten
        turns into an unintended trade.
        """
        sig = order_set.signal
        for order_id in [order_set.stop_id, *(order_set.target_ids or [])]:
            if not order_id:
                continue
            try:
                self.exchange.cancel_order(order_id, self.symbol)
            except Exception as exc:  # noqa: BLE001
                log.warning("could not cancel protective order %s: %s", order_id, exc)

        exit_side = "buy" if sig.direction == "short" else "sell"
        try:
            self.exchange.create_order(
                self.symbol, "market", exit_side, order_set.filled_qty, None,
                {"reduceOnly": True},
            )
            log.info("flattened %s position (%s)", sig.direction, why)
        except Exception as exc:  # noqa: BLE001
            log.error("FLATTEN FAILED — position may still be open: %s", exc)
            return
        if order_set in self.working:
            self.working.remove(order_set)

    def _arm_protection(self, order_set: LiveOrderSet, filled_qty: float) -> None:
        sig = order_set.signal
        exit_side = "buy" if sig.direction == "short" else "sell"
        if filled_qty <= 0:
            return
        try:
            stop = self.exchange.create_order(
                self.symbol,
                "stop_market",
                exit_side,
                filled_qty,
                None,
                {"stopPrice": sig.stop, "reduceOnly": True},
            )
            order_set.stop_id = str(stop["id"])
        except Exception as exc:  # noqa: BLE001
            log.error("STOP ORDER FAILED — position is unprotected: %s", exc)

        ids: list[str] = []
        for target, weight in zip(sig.targets, sig.weights, strict=True):
            try:
                tp = self.exchange.create_limit_order(
                    self.symbol, exit_side, filled_qty * weight, target, {"reduceOnly": True}
                )
                ids.append(str(tp["id"]))
            except Exception as exc:  # noqa: BLE001
                log.error("take-profit at %s rejected: %s", target, exc)
        order_set.target_ids = ids

    def _cancel(self, order_set: LiveOrderSet, why: str = "expired") -> None:
        try:
            self.exchange.cancel_order(order_set.entry_id, self.symbol)
            log.info("entry %s cancelled (%s)", order_set.entry_id, why)
        except Exception as exc:  # noqa: BLE001
            log.warning("could not cancel %s: %s", order_set.entry_id, exc)
        finally:
            if order_set in self.working:
                self.working.remove(order_set)

    def _size(self, signal: Signal) -> float:
        equity = self.equity
        risk_cash = equity * (self.cfg.risk.risk_pct / 100.0)
        per_unit = abs(signal.entry - signal.stop)
        if per_unit <= 0:
            return 0.0
        qty = risk_cash / per_unit
        try:
            return float(self.exchange.amount_to_precision(self.symbol, qty))
        except Exception:  # noqa: BLE001
            return qty
