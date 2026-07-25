"""Price history fetching and indicator computation.

Provides a richer, multi-timeframe indicator set (trend, momentum,
volatility, volume) so the technical agent has enough grounded evidence to
cite specific numbers rather than making vague, unverifiable claims.
"""

from dataclasses import dataclass

import pandas as pd
import yfinance as yf

from config import PRICE_HISTORY_PERIOD


class MarketDataError(RuntimeError):
    pass


@dataclass
class PriceSnapshot:
    ticker: str
    history: pd.DataFrame
    summary_text: str
    last_close: float
    atr14: float


def _rsi(closes: pd.Series, period: int = 14) -> pd.Series:
    delta = closes.diff()
    gain = delta.clip(lower=0)
    loss = -delta.clip(upper=0)
    avg_gain = gain.rolling(window=period).mean()
    avg_loss = loss.rolling(window=period).mean()
    rs = avg_gain / avg_loss.replace(0, float("nan"))
    return 100 - (100 / (1 + rs))


def _atr(history: pd.DataFrame, period: int = 14) -> pd.Series:
    """Average True Range — used downstream for volatility-scaled stop-loss
    and take-profit levels (see risk_management.py), not just display."""
    high, low, close = history["High"], history["Low"], history["Close"]
    prev_close = close.shift(1)
    true_range = pd.concat(
        [(high - low), (high - prev_close).abs(), (low - prev_close).abs()], axis=1
    ).max(axis=1)
    return true_range.rolling(window=period).mean()


def _macd(closes: pd.Series):
    ema12 = closes.ewm(span=12, adjust=False).mean()
    ema26 = closes.ewm(span=26, adjust=False).mean()
    macd_line = ema12 - ema26
    signal_line = macd_line.ewm(span=9, adjust=False).mean()
    histogram = macd_line - signal_line
    return macd_line, signal_line, histogram


def _bollinger(closes: pd.Series, period: int = 20, num_std: float = 2.0):
    mid = closes.rolling(window=period).mean()
    std = closes.rolling(window=period).std()
    return mid + num_std * std, mid, mid - num_std * std


def compute_indicators(history: pd.DataFrame) -> pd.DataFrame:
    """Adds all indicator columns in place-equivalent fashion and returns the
    same (extended) DataFrame. Requires History, Low, Close, Volume columns."""
    closes = history["Close"]

    history["sma20"] = closes.rolling(window=20).mean()
    history["sma50"] = closes.rolling(window=50).mean()
    history["sma200"] = closes.rolling(window=200).mean()
    history["rsi14"] = _rsi(closes)
    history["atr14"] = _atr(history)
    history["macd"], history["macd_signal"], history["macd_hist"] = _macd(closes)
    history["bb_upper"], history["bb_mid"], history["bb_lower"] = _bollinger(closes)
    history["volume_sma20"] = history["Volume"].rolling(window=20).mean()
    history["daily_return_pct"] = closes.pct_change() * 100

    return history


def fetch_price_snapshot(ticker: str, period: str = PRICE_HISTORY_PERIOD) -> PriceSnapshot:
    history = yf.Ticker(ticker).history(period=period, auto_adjust=True)
    if history.empty:
        raise MarketDataError(f"No price data returned for {ticker!r}")

    history = compute_indicators(history)
    last = history.iloc[-1]

    closes = history["Close"]
    period_return_pct = (last["Close"] / closes.iloc[0] - 1) * 100
    volatility_pct = history["daily_return_pct"].std()

    trend = "above" if last["Close"] > last["sma200"] else "below"
    volume_ratio = last["Volume"] / last["volume_sma20"] if last["volume_sma20"] else float("nan")

    # sma200 needs ~200 bars of history; note if we don't have enough yet
    # instead of silently reporting NaN as if it were a real value.
    sma200_text = f"{last['sma200']:.2f} (price is {trend} it)" if pd.notna(last["sma200"]) else "n/a (not enough history for a 200-day average yet)"

    summary_text = (
        f"Ticker: {ticker}\n"
        f"Last close: {last['Close']:.2f} (as of {history.index[-1].date()})\n"
        f"{period} return: {period_return_pct:+.2f}%\n"
        f"Trend — SMA20: {last['sma20']:.2f} | SMA50: {last['sma50']:.2f} | SMA200: {sma200_text}\n"
        f"Momentum — RSI(14): {last['rsi14']:.1f} | "
        f"MACD: {last['macd']:.3f} (signal {last['macd_signal']:.3f}, histogram {last['macd_hist']:+.3f})\n"
        f"Volatility — ATR(14): {last['atr14']:.2f} "
        f"({last['atr14'] / last['Close'] * 100:.1f}% of price) | "
        f"Daily return std dev: {volatility_pct:.2f}%\n"
        f"Bollinger Bands(20,2): upper {last['bb_upper']:.2f} | mid {last['bb_mid']:.2f} | lower {last['bb_lower']:.2f}\n"
        f"Volume: {last['Volume']:,.0f} vs 20-day avg {last['volume_sma20']:,.0f} "
        f"(ratio {volume_ratio:.2f}x)\n"
        f"Last 5 daily returns (%): "
        + ", ".join(f"{v:+.2f}" for v in history["daily_return_pct"].tail(5))
    )

    return PriceSnapshot(
        ticker=ticker,
        history=history,
        summary_text=summary_text,
        last_close=float(last["Close"]),
        atr14=float(last["atr14"]) if pd.notna(last["atr14"]) else float(volatility_pct / 100 * last["Close"]),
    )
