"""Optional directional filter: only take setups that agree with the trend.

Off by default. A bias filter roughly halves the trade count, which is
exactly the sort of change that looks like an edge in a backtest because it
removed the losers you happened to have — turn it on only after you have
seen it survive on data you did not tune it against.

Two modes:
  ema            — close above/below an EMA of the trading timeframe
  htf_structure  — market structure on candles aggregated `htf_multiplier`×
                   higher, which is the honest version of "check the 4H"
"""

from __future__ import annotations

from .config import Config
from .model import Candle, timeframe_ms
from .structure import MarketStructure


class _Ema:
    __slots__ = ("_k", "value", "_seed", "_period")

    def __init__(self, period: int) -> None:
        self._period = period
        self._k = 2.0 / (period + 1.0)
        self._seed: list[float] = []
        self.value: float | None = None

    def update(self, price: float) -> float | None:
        if self.value is None:
            self._seed.append(price)
            if len(self._seed) == self._period:
                self.value = sum(self._seed) / self._period
                self._seed = []
        else:
            self.value = price * self._k + self.value * (1 - self._k)
        return self.value


class _HtfAggregator:
    """Folds trading-timeframe candles into higher-timeframe ones.

    Only emits a bar once its bucket is complete, so the bias never reflects
    a partially formed HTF candle the live agent could not have known.
    """

    __slots__ = ("_bucket_ms", "_open", "_cur")

    def __init__(self, bucket_ms: int) -> None:
        self._bucket_ms = bucket_ms
        self._open: int | None = None
        self._cur: Candle | None = None

    def update(self, candle: Candle) -> Candle | None:
        bucket = candle.ts - (candle.ts % self._bucket_ms)
        closed: Candle | None = None
        if self._open is None:
            self._open, self._cur = bucket, candle
            return None
        if bucket != self._open:
            closed, self._open, self._cur = self._cur, bucket, candle
            return closed
        cur = self._cur
        assert cur is not None
        self._cur = Candle(
            ts=cur.ts,
            open=cur.open,
            high=max(cur.high, candle.high),
            low=min(cur.low, candle.low),
            close=candle.close,
            volume=cur.volume + candle.volume,
        )
        return None


class BiasFilter:
    __slots__ = ("mode", "_ema", "_agg", "_htf", "_last_close")

    def __init__(self, cfg: Config) -> None:
        self.mode = cfg.bias.mode
        self._ema: _Ema | None = None
        self._agg: _HtfAggregator | None = None
        self._htf: MarketStructure | None = None
        self._last_close: float | None = None

        if self.mode == "ema":
            self._ema = _Ema(cfg.bias.ema_period)
        elif self.mode == "htf_structure":
            bucket = timeframe_ms(cfg.market.timeframe) * cfg.bias.htf_multiplier
            self._agg = _HtfAggregator(bucket)
            self._htf = MarketStructure(
                cfg.structure.swing_left,
                cfg.structure.swing_right,
                use_close=cfg.structure.bos_use_close,
            )

    def update(self, candle: Candle) -> None:
        self._last_close = candle.close
        if self._ema is not None:
            self._ema.update(candle.close)
        elif self._agg is not None and self._htf is not None:
            closed = self._agg.update(candle)
            if closed is not None:
                self._htf.update(0, closed)

    def allows(self, direction: str) -> bool:
        if self.mode == "off":
            return True
        if self._ema is not None:
            if self._ema.value is None or self._last_close is None:
                return False
            return (
                self._last_close > self._ema.value
                if direction == "long"
                else self._last_close < self._ema.value
            )
        if self._htf is not None:
            trend = self._htf.trend
            if trend is None:
                return False
            return trend == ("up" if direction == "long" else "down")
        return True

    @property
    def state(self) -> str:
        if self.mode == "off":
            return "off"
        if self._ema is not None:
            if self._ema.value is None:
                return "warming up"
            return "bullish" if (self._last_close or 0) > self._ema.value else "bearish"
        if self._htf is not None:
            return self._htf.trend or "undecided"
        return "off"
