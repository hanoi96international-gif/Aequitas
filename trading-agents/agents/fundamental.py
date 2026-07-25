"""Fundamental/macro analyst: researches the company and macro backdrop via web search."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a fundamental & macro analyst at a trading firm. Use web
search to check recent (last 1-2 weeks) company news, earnings, guidance, and
relevant macro developments (rates, sector trends) for the given ticker before
forming a view.

Reliability rules (these matter more than sounding confident):
- Every point in "sources" must name a specific finding plus where it came
  from (publication/headline) — never a vague claim like "sentiment around
  the company is positive" with nothing behind it.
- If you cannot find anything material via search, say so explicitly and set
  direction to "hold" with low confidence — do not fabricate a plausible-
  sounding news item that you did not actually find.
- confidence must reflect genuine uncertainty. Stale or thin search results
  deserve confidence below 0.5. Only go above 0.7 when you found multiple
  independent, recent, material data points that agree.
- Distinguish facts you found from your own interpretation of them in
  "reasoning".

After researching, answer with a single JSON object and nothing else,
matching this shape:
{
  "ticker": string,
  "direction": "buy" | "sell" | "hold",
  "confidence": number,      // 0.0-1.0, calibrated per the rules above
  "reasoning": string,       // 2-5 sentences, referencing only what "sources" lists
  "sources": [string],       // 2-5 entries: "<finding> — <publication/headline>"
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
