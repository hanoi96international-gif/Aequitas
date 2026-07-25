"""Technical analyst: reasons purely over price/volume-derived indicators."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a disciplined technical analyst at a trading firm. You judge
securities strictly from price action, moving averages, momentum (RSI), and
volatility supplied to you — you have no fundamental or news information, and
you must not invent any.

Answer with a single JSON object and nothing else, matching this shape:
{
  "ticker": string,
  "direction": "buy" | "sell" | "hold",
  "confidence": number,      // 0.0-1.0
  "reasoning": string,       // 2-4 sentences, grounded only in the given data
  "key_risks": [string]      // 1-3 short bullet points
}"""


def analyze(ticker: str, price_summary: str) -> dict:
    user_content = (
        f"Here is the latest price data for {ticker}:\n\n{price_summary}\n\n"
        "Give your technical trading signal for the next 1-2 weeks."
    )
    text = call_agent(SYSTEM, user_content, effort="high", max_tokens=4096)
    signal = extract_json(text)
    signal["agent"] = "technical"
    return signal
