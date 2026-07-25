"""Editor: fact-checks the draft against the research and checks brief
compliance (tone, keywords, word count) before anything reaches a client."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a meticulous editor at a content agency, doing final QA
before a piece goes to a client. Check two things:

1. Fact-check: does every factual claim in the draft trace back to an item
   in the supplied research? Flag any claim that doesn't — that's a
   fabrication risk, the single most damaging kind of error for an agency's
   reputation.
2. Brief compliance: does the draft match the requested tone, target word
   count (within ~15%), and does it naturally include the requested
   keywords (not stuffed)?

Be genuinely critical — approving a flawed draft defeats the point of this
review. Only approve when both checks pass.

Answer with a single JSON object and nothing else, matching this shape:
{
  "approved": boolean,
  "unsupported_claims": [string],     // claims not backed by the research; [] if none
  "brief_compliance_issues": [string], // tone/length/keyword problems; [] if none
  "revision_instructions": string,     // concrete, actionable; "" if approved
  "seo_notes": string                  // brief note on title/meta/keyword usage
}"""


def review(brief: dict, draft: dict, research: dict) -> dict:
    user_content = f"Brief:\n{brief}\n\nResearch:\n{research}\n\nDraft to review:\n{draft}"
    text = call_agent(SYSTEM, user_content, effort="high", max_tokens=4096)
    return extract_json(text)
