"""Chief strategist: combines the three expert signals and their peer reviews
into one final daily recommendation per ticker."""

from agents.base import call_agent, extract_json

SYSTEM = """You are the chief strategist at a trading firm. Three analysts
(technical, fundamental, sentiment) each produced an independent trading
signal for a ticker, and each signal was peer-reviewed by another analyst.
Weigh the original signals against the critiques they received — a
confident signal that got a "disagree" from its reviewer should carry less
weight than one that was corroborated. Do not simply average the three
directions; use judgment, and say so when the analysts genuinely disagree.

Answer with a single JSON object and nothing else, matching this shape:
{
  "ticker": string,
  "final_direction": "buy" | "sell" | "hold",
  "final_confidence": number,     // 0.0-1.0
  "summary": string,              // 3-6 sentences explaining the call
  "dissenting_views": [string]    // notable disagreements analysts/reviewers raised, [] if none
}"""


def synthesize(ticker: str, signals: list[dict], reviews: list[dict]) -> dict:
    lines = [f"Ticker: {ticker}", "", "Analyst signals:"]
    for s in signals:
        lines.append(
            f"- [{s.get('agent')}] {s.get('direction')} "
            f"(confidence {s.get('confidence')}): {s.get('reasoning')}"
        )
    lines.append("")
    lines.append("Peer reviews:")
    for r in reviews:
        lines.append(
            f"- {r.get('reviewer')} on {r.get('target_agent')}: {r.get('agreement')} "
            f"(adj {r.get('confidence_adjustment')}) — {r.get('critique')}"
        )

    text = call_agent(SYSTEM, "\n".join(lines), effort="high", max_tokens=4096)
    return extract_json(text)
