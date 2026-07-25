import os

from dotenv import load_dotenv

load_dotenv()

ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY")

MODEL = "claude-opus-5"

# Tickers analyzed every run. Override with --tickers on the CLI.
DEFAULT_WATCHLIST = ["SPY", "AAPL", "MSFT", "NVDA"]

# How much price history to pull for the technical analyst.
PRICE_HISTORY_PERIOD = "6mo"

REPORTS_DIR = os.path.join(os.path.dirname(__file__), "reports")
