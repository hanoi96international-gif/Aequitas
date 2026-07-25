"""Outreach writer: drafts a short, honest, personalized pitch for one lead.

Drafts only — nothing here sends anything. The message is written for a
human to read, edit if needed, and send themselves via their own email
client, so there's a real person accountable for what goes out and to whom.
"""

from agents.base import call_agent, extract_json

SYSTEM = """You are writing a first-contact outreach message on behalf of a
content agency, to a specific business a researcher identified as a good
fit. Rules:

- Reference the specific observation about their business — this must read
  as clearly personalized, not a template with their name swapped in.
- No hype, no guarantees ("we'll 10x your traffic"), no pressure tactics.
  State plainly what the agency offers and why you're reaching out.
- Keep it short (under 120 words for the message body) and end with one
  low-commitment ask (e.g. "would a free sample be useful?"), not a hard
  sell.
- Include a clear, easy way to say no / opt out.

Answer with a single JSON object and nothing else, matching this shape:
{
  "company": string,
  "subject": string,
  "message": string
}"""


def draft(lead: dict, agency_name: str, agency_offer: str, agency_contact: str) -> dict:
    user_content = (
        f"Agency name: {agency_name}\n"
        f"Agency offer: {agency_offer}\n"
        f"Agency contact for replies: {agency_contact}\n\n"
        f"Lead:\n{lead}\n\n"
        "Draft the outreach message."
    )
    text = call_agent(SYSTEM, user_content, effort="medium", max_tokens=2048)
    return extract_json(text)
