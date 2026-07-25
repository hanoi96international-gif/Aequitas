"""Peer review: one expert agent critiques another expert's signal.

This is the "experts check each other" step: every signal produced by an
analyst is reviewed by a different analyst persona before the synthesis step
sees it, so a single agent's blind spot or overconfidence doesn't pass
through unchallenged.
"""

from agents.base import call_agent, extract_json

SYSTEM_TEMPLATE = """You are a {reviewer_role} at a trading firm, acting as a
peer reviewer. A colleague ({target_role}) produced the trading signal below.
Critique it on its own terms: is the reasoning sound and well-supported by
what they actually cited, is the confidence level justified, and are there
risks they missed or overstated? You do not have their tools or data
access — judge the argument, not just the conclusion.

Answer with a single JSON object and nothing else, matching this shape:
{{
  "reviewer": "{reviewer_role}",
  "target_agent": "{target_role}",
  "agreement": "agree" | "partial" | "disagree",
  "critique": string,                  // 2-4 sentences
  "confidence_adjustment": number      // -0.3 to +0.3: how much to shift their confidence
}}"""


def review(reviewer_role: str, target_signal: dict) -> dict:
    target_role = target_signal.get("agent", "analyst")
    system = SYSTEM_TEMPLATE.format(reviewer_role=reviewer_role, target_role=target_role)
    user_content = (
        f"Colleague's signal for {target_signal.get('ticker')}:\n"
        f"Direction: {target_signal.get('direction')}\n"
        f"Confidence: {target_signal.get('confidence')}\n"
        f"Reasoning: {target_signal.get('reasoning')}\n"
        f"Key risks noted: {target_signal.get('key_risks')}\n"
    )
    text = call_agent(system, user_content, effort="medium", max_tokens=2048)
    return extract_json(text)
