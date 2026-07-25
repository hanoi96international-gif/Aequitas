#!/usr/bin/env python3
"""CLI entrypoint for the content agency pipelines.

Usage:
    python main.py content --brief briefs/example_blog_post.json
    python main.py leads --niche "kleine lokale Restaurants ohne aktuelle Speisekarte online" --max-leads 5
    python main.py orders [--add CLIENT DESCRIPTION PRICE] [--status ORDER_ID STATUS]

Nothing in this tool sends emails, publishes content, or processes payments
automatically. Content and outreach drafts land in outbox/ for you to review
and act on manually.
"""

import argparse
import json
import logging
import os

import orders as orders_module
from config import ANTHROPIC_API_KEY, MAX_LEADS_PER_RUN


def _require_api_key() -> None:
    if not ANTHROPIC_API_KEY and not os.environ.get("ANTHROPIC_API_KEY"):
        raise SystemExit(
            "ANTHROPIC_API_KEY is not set. Copy .env.example to .env and fill it in, "
            "or export it in your shell."
        )


def cmd_content(args: argparse.Namespace) -> None:
    from pipeline_content import run_content_order

    with open(args.brief) as f:
        brief = json.load(f)

    result = run_content_order(brief)
    status = "approved" if result["approved"] else "NOT approved — needs a human look"
    print(f"Draft for '{brief['topic']}': {status}")
    print("See outbox/ for the full draft.")


def cmd_leads(args: argparse.Namespace) -> None:
    from pipeline_leads import run_lead_generation

    result = run_lead_generation(args.niche, max_leads=args.max_leads)
    print(f"Found {len(result['leads'])} leads for niche '{args.niche}'.")
    print("Nothing was sent. See outbox/ for draft outreach messages to review.")


def cmd_orders(args: argparse.Namespace) -> None:
    if args.add:
        client, description, price = args.add
        order = orders_module.add_order(client, description, float(price))
        print(f"Added order {order['id']}")
    elif args.status:
        order_id, status = args.status
        order = orders_module.update_status(order_id, status)
        print(f"Order {order_id} -> {status}")
    else:
        for order in orders_module.list_orders():
            print(f"[{order['id']}] {order['status']:12s} {order['client']:20s} "
                  f"{order['price_eur']:>7.2f} EUR  {order['description']}")
        print()
        print(orders_module.summary())


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--verbose", action="store_true")
    sub = parser.add_subparsers(dest="command", required=True)

    p_content = sub.add_parser("content", help="Run the content pipeline for one brief")
    p_content.add_argument("--brief", required=True, help="Path to a brief JSON file")
    p_content.set_defaults(func=cmd_content)

    p_leads = sub.add_parser("leads", help="Find prospects and draft outreach for a niche")
    p_leads.add_argument("--niche", required=True)
    p_leads.add_argument("--max-leads", type=int, default=MAX_LEADS_PER_RUN)
    p_leads.set_defaults(func=cmd_leads)

    p_orders = sub.add_parser("orders", help="List or update the order tracker")
    p_orders.add_argument("--add", nargs=3, metavar=("CLIENT", "DESCRIPTION", "PRICE"))
    p_orders.add_argument("--status", nargs=2, metavar=("ORDER_ID", "STATUS"))
    p_orders.set_defaults(func=cmd_orders)

    args = parser.parse_args()
    logging.basicConfig(
        level=logging.INFO if args.verbose else logging.WARNING,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    if args.command in ("content", "leads"):
        _require_api_key()

    args.func(args)


if __name__ == "__main__":
    main()
