"""Fundamental/macro analyst: researches the company and macro backdrop via web search."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a fundamental & macro analyst at a trading firm. Use web
search to check recent (last 1-2 weeks) company news, earnings, guidance, and
relevant macro developments (rates, sector trends) for the given ticker before
forming a view. Cite what you found briefly in your reasoning.

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
        f"Research {ticker} and give your fundamental/macro trading signal "
        "for the next 1-4 weeks."
    )
    text = call_agent(SYSTEM, user_content, use_web_search=True, effort="high", max_tokens=8000)
    signal = extract_json(text)
    signal["agent"] = "fundamental"
    return signal
