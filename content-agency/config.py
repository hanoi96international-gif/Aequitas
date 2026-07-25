import os

from dotenv import load_dotenv

load_dotenv()

ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY")

MODEL = "claude-opus-5"

BASE_DIR = os.path.dirname(__file__)
OUTBOX_DIR = os.path.join(BASE_DIR, "outbox")
ORDERS_FILE = os.path.join(BASE_DIR, "orders.json")

# How the agency describes itself in outreach drafts. Edit these before
# generating outreach — the outreach_writer agent uses them verbatim.
AGENCY_NAME = "Deine Content-Agentur"
AGENCY_OFFER = (
    "Wir schreiben recherchierte, SEO-taugliche Blogartikel und "
    "Produktbeschreibungen für kleine Unternehmen, schnell und zu einem "
    "festen Preis pro Artikel."
)
AGENCY_CONTACT = "kontakt@example.com"  # replace with a real address before use

# Rough pricing anchors for orders.py / your own reference — not enforced
# anywhere, just a starting point. Actual pricing depends on your market,
# niche, and how much editing/revision you're willing to include.
PRICING_GUIDE_EUR = {
    "blog_post_short": (40, 80),    # ~500-800 words
    "blog_post_long": (80, 150),    # ~1200-2000 words
    "product_description": (10, 25),  # per product, short-form
}

MAX_CONTENT_REVISIONS = 2
MAX_LEADS_PER_RUN = 10
