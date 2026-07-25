"""Chief strategist: combines the three expert signals and their peer reviews
into one final daily recommendation per ticker.

Deliberately does NOT decide price levels or position size — that's
computed deterministically in risk_management.py from ATR and this signal's
confidence, so those numbers are reproducible rather than LLM-guessed.
"""

from agents.base import call_agent, extract_json

SYSTEM = """You are the chief strategist at a trading firm. Three analysts
(technical, fundamental, sentiment) each produced an independent trading
signal for a ticker, and each signal was peer-reviewed by another analyst,
including a fact-check for unsupported claims. Weigh the original signals
against their critiques — a confident signal that got "disagree" or has
unsupported_claims flagged should carry less weight than one that was
corroborated and fully backed by evidence. Do not simply average the three
directions; use judgment, and say so explicitly when the analysts genuinely
disagree rather than papering over it.

final_confidence must reflect the actual strength of the combined case, not
enthusiasm. If reviewers found unsupported claims or disagreed, that must
pull final_confidence down, even if all three raw signals happened to point
the same direction.

Answer with a single JSON object and nothing else, matching this shape:
{
  "ticker": string,
  "final_direction": "buy" | "sell" | "hold",
  "final_confidence": number,     // 0.0-1.0
  "summary": string,              // 3-6 sentences explaining the call, and how the peer reviews affected it
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
    lines.append("Peer reviews (including fact-check results):")
    for r in reviews:
        lines.append(
            f"- {r.get('reviewer')} on {r.get('target_agent')}: {r.get('agreement')} "
            f"(adj {r.get('confidence_adjustment')}) — {r.get('critique')} "
            f"| unsupported_claims: {r.get('unsupported_claims') or []}"
        )

    text = call_agent(SYSTEM, "\n".join(lines), effort="high", max_tokens=4096)
    return extract_json(text)
