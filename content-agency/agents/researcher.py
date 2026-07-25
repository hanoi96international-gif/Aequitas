"""Researcher: gathers verifiable facts for a content brief via web search.

Kept separate from the writer so factual grounding happens before any
prose is written — the writer is instructed to only state facts that came
from this step, not to invent supporting details on the fly.
"""

from agents.base import call_agent, extract_json

SYSTEM = """You are a research assistant for a content agency. Given a topic
and target audience, use web search to gather concrete, verifiable, recent
facts that a writer can use — statistics, examples, current best practices,
common questions people have. Every fact must be something you actually
found, with a source. Do not pad the list with generic statements that
don't need a source.

If web search turns up little of substance, say so honestly rather than
inventing plausible-sounding facts.

Answer with a single JSON object and nothing else, matching this shape:
{
  "topic": string,
  "key_facts": [ { "fact": string, "source": string } ],  // 3-8 items
  "suggested_angle": string,       // one sentence: the most interesting way to frame this piece
  "notes_for_writer": string       // anything the writer should know (audience gaps, common misconceptions, etc.)
}"""


def research(topic: str, audience: str) -> dict:
    user_content = f"Topic: {topic}\nTarget audience: {audience}\n\nGather facts for this piece."
    text = call_agent(SYSTEM, user_content, use_web_search=True, effort="high", max_tokens=8000)
    return extract_json(text)
