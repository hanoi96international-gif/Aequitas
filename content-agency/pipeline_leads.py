"""Orchestrates lead generation: find prospects in a niche, draft a
personalized outreach message for each. Writes everything to the outbox for
human review — this module never sends anything itself, and never invents
contact details that weren't actually found.
"""

import json
import logging
import os
import re
from datetime import datetime

from agents import outreach_writer, prospect_finder
from agents.base import AgentError
from config import AGENCY_CONTACT, AGENCY_NAME, AGENCY_OFFER, MAX_LEADS_PER_RUN, OUTBOX_DIR

log = logging.getLogger(__name__)


def _slug(text: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")[:60]


def run_lead_generation(niche: str, max_leads: int = MAX_LEADS_PER_RUN) -> dict:
    log.info("Finding prospects for niche: %s", niche)
    found = prospect_finder.find(niche, max_leads)
    leads = found.get("leads", [])

    drafted = []
    for lead in leads:
        try:
            message = outreach_writer.draft(lead, AGENCY_NAME, AGENCY_OFFER, AGENCY_CONTACT)
        except AgentError as exc:
            log.warning("Outreach draft failed for %s: %s", lead.get("company"), exc)
            continue
        drafted.append({"lead": lead, "outreach": message})

    result = {"niche": niche, "leads": drafted}
    _save_to_outbox(niche, result)
    return result


def _save_to_outbox(niche: str, result: dict) -> None:
    os.makedirs(OUTBOX_DIR, exist_ok=True)
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    base_name = f"leads_{_slug(niche)}_{timestamp}"

    md_path = os.path.join(OUTBOX_DIR, f"{base_name}.md")
    with open(md_path, "w") as f:
        f.write(f"# Lead-Liste: {niche}\n\n")
        f.write(
            "**⚠️ Nichts wurde verschickt.** Jede Nachricht unten ist ein Entwurf. "
            "Vor dem Versand: pruefen, ob die Ansprache stimmt, ob rechtliche "
            "Anforderungen an Werbe-E-Mails (in Deutschland u.a. UWG/DSGVO) "
            "eingehalten sind, und manuell ueber dein eigenes E-Mail-Postfach "
            "versenden.\n\n"
            "---\n\n"
        )
        for item in result["leads"]:
            lead = item["lead"]
            outreach = item["outreach"]
            f.write(f"## {lead.get('company')}\n\n")
            f.write(f"- Website: {lead.get('website')}\n")
            f.write(f"- Beobachtung: {lead.get('observation')}\n")
            f.write(f"- Warum passend: {lead.get('why_good_fit')}\n")
            f.write(f"- Öffentliche Kontaktseite: {lead.get('public_contact_page') or 'nicht gefunden'}\n\n")
            f.write(f"**Betreff:** {outreach.get('subject')}\n\n")
            f.write(f"> {outreach.get('message')}\n\n")
            f.write("---\n\n")

    json_path = os.path.join(OUTBOX_DIR, f"{base_name}.json")
    with open(json_path, "w") as f:
        json.dump(result, f, indent=2)

    log.info("Saved %s", md_path)
