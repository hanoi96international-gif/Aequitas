"""Core value types shared by every module: candles, ATR, timeframe maths.

Everything downstream consumes candles one at a time. There is deliberately
no pandas/numpy here: the engine has to run identically in a backtest loop
and in a live poll loop, and the cheapest way to guarantee that is to make
the incremental path the only path.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class Candle:
    """A single OHLCV bar. `ts` is the bar's *open* time in epoch milliseconds."""

    ts: int
    open: float
    high: float
    low: float
    close: float
    volume: float = 0.0

    @property
    def is_bullish(self) -> bool:
        return self.close > self.open

    @property
    def is_bearish(self) -> bool:
        return self.close < self.open

    @property
    def body_top(self) -> float:
        return max(self.open, self.close)

    @property
    def body_bottom(self) -> float:
        return min(self.open, self.close)

    @property
    def body(self) -> float:
        return abs(self.close - self.open)

    @property
    def span(self) -> float:
        return self.high - self.low


class ATR:
    """Wilder's Average True Range, fed one candle at a time.

    Returns None until `period` candles have been seen, so callers can tell
    "no reading yet" from "a reading of zero" — a distinction that matters
    because every threshold in the strategy is expressed in ATR units and a
    zero would silently make them all trivially satisfiable.
    """

    __slots__ = ("period", "_prev_close", "_seed", "_value")

    def __init__(self, period: int) -> None:
        if period < 1:
            raise ValueError("ATR period must be >= 1")
        self.period = period
        self._prev_close: float | None = None
        self._seed: list[float] = []
        self._value: float | None = None

    def update(self, candle: Candle) -> float | None:
        if self._prev_close is None:
            tr = candle.span
        else:
            tr = max(
                candle.span,
                abs(candle.high - self._prev_close),
                abs(candle.low - self._prev_close),
            )
        self._prev_close = candle.close

        if self._value is None:
            self._seed.append(tr)
            if len(self._seed) == self.period:
                self._value = sum(self._seed) / self.period
                self._seed = []
        else:
            self._value = (self._value * (self.period - 1) + tr) / self.period
        return self._value

    @property
    def value(self) -> float | None:
        return self._value


_TF_UNITS = {"s": 1_000, "m": 60_000, "h": 3_600_000, "d": 86_400_000, "w": 604_800_000}


def timeframe_ms(timeframe: str) -> int:
    """Convert an exchange-style timeframe ("15m", "4h", "1d") to milliseconds."""
    tf = timeframe.strip().lower()
    if len(tf) < 2 or tf[-1] not in _TF_UNITS:
        raise ValueError(f"unsupported timeframe {timeframe!r}")
    try:
        amount = int(tf[:-1])
    except ValueError as exc:
        raise ValueError(f"unsupported timeframe {timeframe!r}") from exc
    if amount < 1:
        raise ValueError(f"unsupported timeframe {timeframe!r}")
    return amount * _TF_UNITS[tf[-1]]
