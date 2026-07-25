#!/usr/bin/env python3
"""CLI entrypoint: runs the daily multi-agent trading-signal pipeline.

Usage:
    python main.py [--tickers SPY AAPL MSFT]

Writes reports/<date>.md and reports/<date>.json. This produces research
signals only -- it never places trades or talks to a broker.
"""

import argparse
import json
import logging
import os
from datetime import date, datetime

from config import DEFAULT_WATCHLIST, REPORTS_DIR
from orchestrator import run_watchlist
from report import render_markdown


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--tickers",
        nargs="+",
        default=DEFAULT_WATCHLIST,
        help="Ticker symbols to analyze (default: %(default)s)",
    )
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.INFO if args.verbose else logging.WARNING,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    if not os.environ.get("ANTHROPIC_API_KEY"):
        raise SystemExit(
            "ANTHROPIC_API_KEY is not set. Copy .env.example to .env and fill it in, "
            "or export it in your shell."
        )

    results = run_watchlist(args.tickers)
    if not results:
        raise SystemExit("No tickers produced a result; nothing to report.")

    run_date = date.today()
    os.makedirs(REPORTS_DIR, exist_ok=True)

    md_path = os.path.join(REPORTS_DIR, f"{run_date.isoformat()}.md")
    json_path = os.path.join(REPORTS_DIR, f"{run_date.isoformat()}.json")

    with open(md_path, "w") as f:
        f.write(render_markdown(results, run_date))

    with open(json_path, "w") as f:
        json.dump(
            {"date": run_date.isoformat(), "generated_at": datetime.now().isoformat(), "results": results},
            f,
            indent=2,
        )

    print(f"Wrote {md_path}")
    print(f"Wrote {json_path}")
    for result in results:
        final = result["final"]
        print(f"  {result['ticker']}: {final['final_direction'].upper()} ({final['final_confidence']:.2f})")


if __name__ == "__main__":
    main()
