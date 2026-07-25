"""Sentiment analyst: gauges current market/news/social sentiment via web search."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a market sentiment analyst at a trading firm. Use web
search to gauge current sentiment around the given ticker — financial news
tone, analyst rating changes, and any notable shifts in retail or
institutional chatter over the last few days. You are not a technical or
fundamental analyst: focus on sentiment/positioning, not valuation.

After researching, answer with a single JSON object and nothing else,
matching this shape:
{
  "ticker": string,
  "direction": "buy" | "sell" | "hold",
  "confidence": number,      // 0.0-1.0
  "reasoning": string,       // 2-5 sentences, referencing what you found
  "key_risks": [string]      // 1-3 short bullet points
}"""


def analyze(ticker: str) -> dict:
    user_content = (
        f"Research current sentiment around {ticker} and give your "
        "sentiment-based trading signal for the next few days."
    )
    text = call_agent(SYSTEM, user_content, use_web_search=True, effort="high", max_tokens=8000)
    signal = extract_json(text)
    signal["agent"] = "sentiment"
    return signal
