"""Deterministic position sizing, stop-loss and take-profit calculation.

Deliberately NOT delegated to the LLM: price levels and position sizes are
exactly the kind of concrete numbers a language model can plausibly
hallucinate. Keeping this as plain arithmetic over ATR (a measured value)
means every number in the final report is reproducible and auditable —
the LLM only ever decides direction and confidence.
"""

from dataclasses import dataclass

from config import RISK_PROFILE


@dataclass
class RiskManagedPosition:
    direction: str
    entry_price: float
    stop_loss_price: float | None
    take_profit_price: float | None
    position_size_pct: float
    risk_reward_ratio: float | None
    note: str


def size_position(
    direction: str,
    entry_price: float,
    atr: float,
    confidence: float,
    risk_profile: dict = RISK_PROFILE,
) -> RiskManagedPosition:
    """Computes stop-loss/take-profit from ATR and a fixed-fractional
    position size from the stop distance — the standard "risk N% of capital
    per trade" method. Position size never exceeds max_position_pct, and
    scales down with confidence so a shaky signal risks less capital."""

    if direction == "hold" or atr <= 0 or entry_price <= 0:
        return RiskManagedPosition(
            direction=direction,
            entry_price=entry_price,
            stop_loss_price=None,
            take_profit_price=None,
            position_size_pct=0.0,
            risk_reward_ratio=None,
            note="No position sized for a 'hold' signal.",
        )

    stop_distance = atr * risk_profile["stop_loss_atr_multiple"]
    reward_distance = atr * risk_profile["take_profit_atr_multiple"]

    if direction == "buy":
        stop_loss_price = entry_price - stop_distance
        take_profit_price = entry_price + reward_distance
    else:  # sell / short
        stop_loss_price = entry_price + stop_distance
        take_profit_price = entry_price - reward_distance

    stop_distance_pct = stop_distance / entry_price * 100

    # Fixed-fractional sizing: risk_per_trade_pct of capital / distance to
    # stop (in %) = how big the position can be while risking exactly
    # risk_per_trade_pct of capital if the stop is hit.
    risk_based_size_pct = risk_profile["risk_per_trade_pct"] / stop_distance_pct * 100

    # Scale down by confidence (0-1) — a 0.5-confidence signal risks half as
    # much as a 1.0-confidence one — then cap at the profile's hard ceiling.
    confidence = max(0.0, min(1.0, confidence))
    position_size_pct = min(risk_based_size_pct * confidence, risk_profile["max_position_pct"])

    risk_reward_ratio = reward_distance / stop_distance if stop_distance else None

    return RiskManagedPosition(
        direction=direction,
        entry_price=entry_price,
        stop_loss_price=round(stop_loss_price, 2),
        take_profit_price=round(take_profit_price, 2),
        position_size_pct=round(position_size_pct, 2),
        risk_reward_ratio=round(risk_reward_ratio, 2) if risk_reward_ratio else None,
        note=(
            f"Sized to risk {risk_profile['risk_per_trade_pct']}% of capital at the stop, "
            f"scaled by confidence, capped at {risk_profile['max_position_pct']}% of capital."
        ),
    )
