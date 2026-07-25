"""Shared Claude API plumbing used by every agent."""

import json
import re

import anthropic

from config import ANTHROPIC_API_KEY, MODEL

_client = None


class AgentError(RuntimeError):
    pass


def get_client() -> anthropic.Anthropic:
    global _client
    if _client is None:
        # Falls back to ANTHROPIC_API_KEY / an `ant auth login` profile when
        # api_key is None, so this also works without .env in some setups.
        _client = anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)
    return _client


def call_agent(
    system: str,
    user_content: str,
    *,
    use_web_search: bool = False,
    effort: str = "high",
    max_tokens: int = 8000,
) -> str:
    """Sends one request to Claude and returns the final assistant text.

    Raises AgentError if the model declines to answer (stop_reason == "refusal")
    or produces no text content.
    """
    client = get_client()
    tools = [{"type": "web_search_20260209", "name": "web_search", "max_uses": 5}] if use_web_search else None

    kwargs = dict(
        model=MODEL,
        max_tokens=max_tokens,
        system=system,
        output_config={"effort": effort},
        messages=[{"role": "user", "content": user_content}],
    )
    if tools:
        kwargs["tools"] = tools

    response = client.messages.create(**kwargs)

    if response.stop_reason == "refusal":
        category = response.stop_details.category if response.stop_details else None
        raise AgentError(f"Model declined to respond (category={category})")

    text_parts = [block.text for block in response.content if block.type == "text"]
    if not text_parts:
        raise AgentError("Model returned no text content")
    return "\n".join(text_parts)


def extract_json(text: str) -> dict:
    """Pulls the first top-level JSON object out of a model response.

    Agents are instructed to answer with pure JSON, but web-search-using
    agents sometimes wrap it in a sentence or a fenced code block.
    """
    fenced = re.search(r"```(?:json)?\s*(\{.*?\})\s*```", text, re.DOTALL)
    candidate = fenced.group(1) if fenced else None

    if candidate is None:
        start = text.find("{")
        end = text.rfind("}")
        if start == -1 or end == -1 or end <= start:
            raise AgentError(f"No JSON object found in model response:\n{text}")
        candidate = text[start : end + 1]

    try:
        return json.loads(candidate)
    except json.JSONDecodeError as exc:
        raise AgentError(f"Could not parse JSON from model response: {exc}\n{text}") from exc
