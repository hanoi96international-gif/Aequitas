"""Orchestrates one content order: research -> draft -> QA -> revise (bounded)
-> save to outbox for human review. Nothing here delivers to a client
automatically — the output is a file you read and send yourself."""

import json
import logging
import os
import re
from datetime import datetime

from agents import editor, researcher, writer
from agents.base import AgentError
from config import MAX_CONTENT_REVISIONS, OUTBOX_DIR

log = logging.getLogger(__name__)


def _slug(text: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")[:60]


def run_content_order(brief: dict) -> dict:
    """brief needs: topic, audience, tone, target_word_count, keywords (list)."""
    log.info("Researching: %s", brief["topic"])
    facts = researcher.research(brief["topic"], brief["audience"])

    log.info("Drafting")
    draft = writer.write(brief, facts)

    review = None
    for attempt in range(1, MAX_CONTENT_REVISIONS + 1):
        log.info("Editorial review, attempt %d", attempt)
        review = editor.review(brief, draft, facts)
        if review.get("approved"):
            break
        try:
            draft = writer.revise(brief, draft, review.get("revision_instructions", ""), facts)
        except AgentError as exc:
            log.warning("Revision failed: %s", exc)
            break

    result = {
        "brief": brief,
        "research": facts,
        "draft": draft,
        "final_review": review,
        "approved": bool(review and review.get("approved")),
    }

    _save_to_outbox(brief, result)
    return result


def _save_to_outbox(brief: dict, result: dict) -> None:
    os.makedirs(OUTBOX_DIR, exist_ok=True)
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    slug = _slug(brief["topic"])
    base_name = f"content_{slug}_{timestamp}"

    draft = result["draft"]
    review = result["final_review"] or {}

    status_banner = (
        "✅ APPROVED by editor QA — still read it yourself before sending to a client."
        if result["approved"]
        else "⚠️ NOT approved by editor QA after all revision attempts — needs a human look "
        "before this goes anywhere near a client."
    )

    md_path = os.path.join(OUTBOX_DIR, f"{base_name}.md")
    with open(md_path, "w") as f:
        f.write(f"<!-- {status_banner} -->\n\n")
        f.write(f"# {draft.get('title', brief['topic'])}\n\n")
        f.write(f"_Meta description: {draft.get('meta_description', '')}_\n\n")
        f.write(draft.get("body_markdown", ""))
        f.write("\n\n---\n\n")
        f.write(f"**Word count:** {draft.get('word_count', 'n/a')}\n\n")
        if review.get("unsupported_claims"):
            f.write(f"**⚠️ Unsupported claims flagged:** {review['unsupported_claims']}\n\n")
        if review.get("brief_compliance_issues"):
            f.write(f"**Brief compliance issues:** {review['brief_compliance_issues']}\n\n")
        f.write(f"**SEO notes:** {review.get('seo_notes', '')}\n")

    json_path = os.path.join(OUTBOX_DIR, f"{base_name}.json")
    with open(json_path, "w") as f:
        json.dump(result, f, indent=2)

    log.info("Saved %s", md_path)
