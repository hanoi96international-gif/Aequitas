import os

from dotenv import load_dotenv

load_dotenv()

ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY")

MODEL = "claude-opus-5"

# Tickers analyzed every run. Override with --tickers on the CLI.
DEFAULT_WATCHLIST = ["SPY", "AAPL", "MSFT", "NVDA"]

# How much price history to pull for the technical analyst. 1y gives enough
# bars for a meaningful 200-day SMA on the most recent row.
PRICE_HISTORY_PERIOD = "1y"

REPORTS_DIR = os.path.join(os.path.dirname(__file__), "reports")

# --- Risk profile: "aggressive, but bounded" -------------------------------
# This is a sizing/labeling configuration for risk_management.py, NOT a
# promised or guaranteed return. Stop-loss and position size are computed
# deterministically in code from these numbers plus market volatility (ATR)
# — the LLM never invents price levels or position sizes. The monthly target
# is aspirational, used only to label backtest results as "on/off track";
# nothing in the code assumes it will actually be hit.
RISK_PROFILE = {
    "name": "aggressive_bounded",
    # Max % of trading capital committed to a single position, regardless
    # of how confident the signal is.
    "max_position_pct": 15.0,
    # % of total capital allowed to be lost if the stop-loss is hit on a
    # single trade. Position size is derived FROM this (fixed-fractional
    # risk sizing), not the other way around.
    "risk_per_trade_pct": 2.0,
    # Stop-loss / take-profit distance from entry, expressed as a multiple
    # of ATR(14) — i.e. volatility-scaled, not a fixed percentage.
    "stop_loss_atr_multiple": 1.5,
    "take_profit_atr_multiple": 3.0,  # 2:1 reward:risk
    # Aspirational, NOT guaranteed. Used only to annotate backtest results.
    "monthly_return_target_pct": 15.0,
    # Circuit breaker: if a backtest shows a max drawdown beyond this, the
    # backtest report flags the strategy as too risky for live use at this
    # profile — it does not silently keep trading through it.
    "max_drawdown_circuit_breaker_pct": 20.0,
}
