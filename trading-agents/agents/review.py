"""Peer review: one expert agent critiques another expert's signal.

This is the "experts check each other" step: every signal produced by an
analyst is reviewed by a different analyst persona before the synthesis step
sees it, so a single agent's blind spot, overconfidence, or an unsupported
claim doesn't pass through unchallenged.
"""

from agents.base import call_agent, extract_json

SYSTEM_TEMPLATE = """You are a {reviewer_role} at a trading firm, acting as a
peer reviewer. A colleague ({target_role}) produced the trading signal below,
including the specific evidence/sources they say it rests on. Your job is to
fact-check the argument, not just react to the conclusion:

1. Go through "reasoning" sentence by sentence. For each claim, check whether
   it is actually backed by an item in "cited evidence/sources" below. Any
   claim that is NOT backed by a listed item goes into "unsupported_claims"
   verbatim.
2. Judge whether the confidence level is justified given how strong (or
   thin) the cited evidence actually is — thin evidence with high stated
   confidence is itself a finding worth flagging in "critique".
3. Note any risks they missed or overstated.

Answer with a single JSON object and nothing else, matching this shape:
{{
  "reviewer": "{reviewer_role}",
  "target_agent": "{target_role}",
  "agreement": "agree" | "partial" | "disagree",
  "unsupported_claims": [string],      // claims in their reasoning not backed by their own cited evidence; [] if none
  "critique": string,                  // 2-4 sentences
  "confidence_adjustment": number      // -0.3 to +0.3: how much to shift their confidence (lower it if unsupported_claims is non-empty or evidence is thin)
}}"""


def review(reviewer_role: str, target_signal: dict) -> dict:
    target_role = target_signal.get("agent", "analyst")
    system = SYSTEM_TEMPLATE.format(reviewer_role=reviewer_role, target_role=target_role)
    cited = target_signal.get("evidence") or target_signal.get("sources") or []
    user_content = (
        f"Colleague's signal for {target_signal.get('ticker')}:\n"
        f"Direction: {target_signal.get('direction')}\n"
        f"Confidence: {target_signal.get('confidence')}\n"
        f"Reasoning: {target_signal.get('reasoning')}\n"
        f"Cited evidence/sources: {cited}\n"
        f"Key risks noted: {target_signal.get('key_risks')}\n"
    )
    text = call_agent(system, user_content, effort="medium", max_tokens=2048)
    return extract_json(text)
