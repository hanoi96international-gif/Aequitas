"""The live agent loop: poll closed candles, feed the engine, act on signals.

Only *closed* candles reach the strategy. The forming bar is discarded on
every poll, so a signal can never appear and then vanish because the bar it
was based on went on to close somewhere else.
"""

from __future__ import annotations

import json
import logging
import signal as os_signal
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path

from .config import Config
from .data import fetch_ohlcv
from .execution import Trade
from .model import Candle, timeframe_ms
from .notify import Notifier
from .strategy import LsobStrategy, Signal

log = logging.getLogger("lsob.agent")


@dataclass
class AgentState:
    """What the agent writes to disk so a restart is legible to a human."""

    last_bar_ts: int = 0
    bars_seen: int = 0
    signals: int = 0
    trades: list[dict] = field(default_factory=list)
    equity: float = 0.0
    started_at: float = 0.0


class Agent:
    def __init__(self, cfg: Config, broker, warmup_bars: int = 1500) -> None:
        self.cfg = cfg
        self.broker = broker
        self.strategy = LsobStrategy(cfg)
        self.notifier = Notifier(cfg.notify)
        self.warmup_bars = warmup_bars
        self.state = AgentState(started_at=time.time())
        self.state_path = Path(cfg.live.state_file)
        self._index = -1
        self._stop = False

    def request_stop(self, *_: object) -> None:
        log.info("stop requested — finishing the current poll")
        self._stop = True

    def run(self) -> None:
        os_signal.signal(os_signal.SIGINT, self.request_stop)
        os_signal.signal(os_signal.SIGTERM, self.request_stop)

        log.info(
            "starting: %s %s %s via %s | bias %s",
            self.cfg.market.exchange,
            self.cfg.market.symbol,
            self.cfg.market.timeframe,
            self.broker.describe(),
            self.strategy.bias.state,
        )
        self._warm_up()
        step_ms = timeframe_ms(self.cfg.market.timeframe)

        while not self._stop:
            try:
                self._poll()
            except Exception as exc:  # noqa: BLE001 - a bad poll must not end the run
                log.warning("poll failed, retrying: %s", exc)
            self._save()
            self._sleep_until_next(step_ms)
        self._save()
        log.info("stopped after %d bars, %d signals", self.state.bars_seen, self.state.signals)

    def _warm_up(self) -> None:
        """Replay recent history so the detector has structure before it trades.

        Signals produced during warm-up are discarded: they refer to bars that
        already closed, and acting on them would be trading the past.
        """
        candles = fetch_ohlcv(
            self.cfg.market.exchange,
            self.cfg.market.symbol,
            self.cfg.market.timeframe,
            self.warmup_bars,
        )
        for candle in candles:
            self._index += 1
            self.strategy.on_bar(candle)
            self.state.last_bar_ts = candle.ts
        self.state.bars_seen = len(candles)
        log.info("warmed up on %d historical bars", len(candles))

    def _poll(self) -> None:
        candles = fetch_ohlcv(
            self.cfg.market.exchange,
            self.cfg.market.symbol,
            self.cfg.market.timeframe,
            120,
        )
        fresh = [c for c in candles if c.ts > self.state.last_bar_ts]
        for candle in fresh:
            self._on_closed_bar(candle)

    def _on_closed_bar(self, candle: Candle) -> None:
        self._index += 1
        self.state.last_bar_ts = candle.ts
        self.state.bars_seen += 1

        for trade in self.broker.on_bar(self._index, candle):
            self._on_trade(trade)
        for sig in self.strategy.on_bar(candle):
            self.state.signals += 1
            accepted = self.broker.on_signal(self._index, sig)
            self._on_signal(sig, accepted)

    def _on_signal(self, sig: Signal, accepted: bool) -> None:
        verdict = "placed" if accepted else "skipped (risk limits)"
        self.notifier.send(
            f"[LSOB] {self.cfg.market.symbol} {self.cfg.market.timeframe} — {verdict}\n"
            f"{sig.label}\n"
            f"swept {sig.sweep_level:.6g} (x{sig.sweep_touches}), "
            f"displacement {sig.displacement:.2f} ATR"
        )

    def _on_trade(self, trade: Trade) -> None:
        self.state.trades.append(
            {
                "direction": trade.direction,
                "entry": trade.entry_price,
                "exit": trade.exit_price,
                "pnl": trade.pnl,
                "r": trade.r_multiple,
                "reason": trade.exit_reason,
                "exit_ts": trade.exit_ts,
            }
        )
        self.notifier.send(
            f"[LSOB] closed {trade.direction} {trade.exit_reason} "
            f"{trade.r_multiple:+.2f}R ({trade.pnl:+,.2f})"
        )

    def _sleep_until_next(self, step_ms: int) -> None:
        """Wake shortly after the next bar closes, not on a fixed tick."""
        poll = max(1, self.cfg.live.poll_seconds)
        now_ms = int(time.time() * 1000)
        next_close = ((now_ms // step_ms) + 1) * step_ms
        wait = max(0.0, (next_close - now_ms) / 1000.0) + 2.0
        deadline = time.time() + min(wait, float(poll))
        while time.time() < deadline and not self._stop:
            time.sleep(0.5)

    def _save(self) -> None:
        try:
            self.state.equity = float(getattr(self.broker, "equity", 0.0))
            self.state_path.parent.mkdir(parents=True, exist_ok=True)
            self.state_path.write_text(json.dumps(asdict(self.state), indent=2), encoding="utf-8")
        except OSError as exc:
            log.warning("could not write state file: %s", exc)
