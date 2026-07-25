"""Runs the full daily pipeline for a watchlist: data -> 3 independent expert
signals -> peer review (each expert checked by a different expert, with an
unsupported-claims fact-check) -> synthesis into one final call per ticker ->
deterministic risk-managed position sizing."""

import concurrent.futures as cf
import logging
from dataclasses import asdict

from agents import fundamental, review, sentiment, synthesis, technical
from agents.base import AgentError
from data.market_data import MarketDataError, fetch_price_snapshot
from risk_management import size_position

log = logging.getLogger(__name__)

# reviewer_role -> which agent's signal they check. Round-robin so nobody
# reviews their own work and every signal gets exactly one independent check.
REVIEW_ASSIGNMENTS = {
    "sentiment analyst": "technical",
    "technical analyst": "fundamental",
    "fundamental analyst": "sentiment",
}


def _run_analysts(ticker: str, price_summary: str) -> list[dict]:
    with cf.ThreadPoolExecutor(max_workers=3) as pool:
        futures = {
            pool.submit(technical.analyze, ticker, price_summary): "technical",
            pool.submit(fundamental.analyze, ticker): "fundamental",
            pool.submit(sentiment.analyze, ticker): "sentiment",
        }
        signals = []
        for future in cf.as_completed(futures):
            name = futures[future]
            try:
                signals.append(future.result())
            except AgentError as exc:
                log.warning("%s analyst failed for %s: %s", name, ticker, exc)
    return signals


def _run_peer_reviews(signals: list[dict]) -> list[dict]:
    by_agent = {s["agent"]: s for s in signals}
    with cf.ThreadPoolExecutor(max_workers=3) as pool:
        futures = {}
        for reviewer_role, target_agent in REVIEW_ASSIGNMENTS.items():
            target = by_agent.get(target_agent)
            if target is None:
                continue
            futures[pool.submit(review.review, reviewer_role, target)] = reviewer_role
        reviews = []
        for future in cf.as_completed(futures):
            reviewer_role = futures[future]
            try:
                reviews.append(future.result())
            except AgentError as exc:
                log.warning("Review by %s failed: %s", reviewer_role, exc)
    return reviews


def run_ticker(ticker: str) -> dict:
    log.info("Analyzing %s", ticker)
    snapshot = fetch_price_snapshot(ticker)

    signals = _run_analysts(ticker, snapshot.summary_text)
    if not signals:
        raise AgentError(f"All analysts failed for {ticker}; skipping")

    reviews = _run_peer_reviews(signals)
    final = synthesis.synthesize(ticker, signals, reviews)

    # Deterministic, non-LLM step: turn the direction/confidence call into
    # concrete stop-loss/take-profit/position-size numbers from ATR.
    position = size_position(
        direction=final["final_direction"],
        entry_price=snapshot.last_close,
        atr=snapshot.atr14,
        confidence=final["final_confidence"],
    )
    final["risk_management"] = asdict(position)

    return {
        "ticker": ticker,
        "price_summary": snapshot.summary_text,
        "signals": signals,
        "reviews": reviews,
        "final": final,
    }


def run_watchlist(tickers: list[str]) -> list[dict]:
    results = []
    for ticker in tickers:
        try:
            results.append(run_ticker(ticker))
        except (AgentError, MarketDataError) as exc:
            log.error("Skipping %s: %s", ticker, exc)
    return results
