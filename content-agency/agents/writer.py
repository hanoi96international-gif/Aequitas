"""Writer: drafts (and revises) the actual deliverable from a brief + research."""

from agents.base import call_agent, extract_json

SYSTEM = """You are a professional copywriter at a content agency. Write
copy that matches the brief exactly (tone, audience, word count range,
keywords to include). Ground every factual claim in the supplied research
facts — do not invent statistics, examples, or claims that aren't in the
research. If you want to make a point the research doesn't cover, phrase it
as opinion/framing, not as fact.

Answer with a single JSON object and nothing else, matching this shape:
{
  "title": string,
  "meta_description": string,   // <=160 chars, for SEO
  "body_markdown": string,      // the full piece, in markdown
  "word_count": number
}"""

REVISE_SYSTEM = SYSTEM + """

You are revising a previous draft based on editor feedback. Keep what
already works; fix exactly what the feedback calls out. Do not introduce new
unsupported claims while revising."""


def write(brief: dict, research: dict) -> dict:
    user_content = (
        f"Brief:\n{brief}\n\n"
        f"Research (use only these facts for anything factual):\n{research}\n\n"
        "Write the piece."
    )
    text = call_agent(SYSTEM, user_content, effort="high", max_tokens=8000)
    return extract_json(text)


def revise(brief: dict, draft: dict, revision_instructions: str, research: dict) -> dict:
    user_content = (
        f"Brief:\n{brief}\n\n"
        f"Research (use only these facts for anything factual):\n{research}\n\n"
        f"Previous draft:\n{draft}\n\n"
        f"Editor feedback to address:\n{revision_instructions}\n\n"
        "Produce the revised piece."
    )
    text = call_agent(REVISE_SYSTEM, user_content, effort="high", max_tokens=8000)
    return extract_json(text)
