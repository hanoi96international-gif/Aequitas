"""Price history fetching and indicator computation for the technical analyst."""

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


def _rsi(closes: pd.Series, period: int = 14) -> pd.Series:
    delta = closes.diff()
    gain = delta.clip(lower=0)
    loss = -delta.clip(upper=0)
    avg_gain = gain.rolling(window=period).mean()
    avg_loss = loss.rolling(window=period).mean()
    rs = avg_gain / avg_loss.replace(0, float("nan"))
    return 100 - (100 / (1 + rs))


def fetch_price_snapshot(ticker: str, period: str = PRICE_HISTORY_PERIOD) -> PriceSnapshot:
    history = yf.Ticker(ticker).history(period=period, auto_adjust=True)
    if history.empty:
        raise MarketDataError(f"No price data returned for {ticker!r}")

    closes = history["Close"]
    history["sma20"] = closes.rolling(window=20).mean()
    history["sma50"] = closes.rolling(window=50).mean()
    history["rsi14"] = _rsi(closes)
    history["daily_return_pct"] = closes.pct_change() * 100

    last = history.iloc[-1]
    period_start = closes.iloc[0]
    period_return_pct = (last["Close"] / period_start - 1) * 100
    volatility_pct = history["daily_return_pct"].std()

    summary_text = (
        f"Ticker: {ticker}\n"
        f"Last close: {last['Close']:.2f} (as of {history.index[-1].date()})\n"
        f"{period} return: {period_return_pct:+.2f}%\n"
        f"20-day SMA: {last['sma20']:.2f} | 50-day SMA: {last['sma50']:.2f}\n"
        f"RSI(14): {last['rsi14']:.1f}\n"
        f"Daily volatility (std dev of daily returns): {volatility_pct:.2f}%\n"
        f"Last 5 daily returns (%): "
        + ", ".join(f"{v:+.2f}" for v in history["daily_return_pct"].tail(5))
    )

    return PriceSnapshot(ticker=ticker, history=history, summary_text=summary_text)
