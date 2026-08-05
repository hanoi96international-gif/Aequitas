from __future__ import annotations

from conftest import candle

from lsob.config import Config
from lsob.execution import Executor
from lsob.orderblock import OrderBlock
from lsob.strategy import Signal


def zero_cost_config(**risk) -> Config:
    """Costs off by default so a test asserting on P&L isn't really testing fees."""
    cfg = Config()
    cfg.costs.maker_fee_bps = 0.0
    cfg.costs.taker_fee_bps = 0.0
    cfg.costs.slippage_bps = 0.0
    cfg.risk.starting_equity = 10_000.0
    cfg.risk.risk_pct = 1.0
    for key, value in risk.items():
        setattr(cfg.risk, key, value)
    return cfg


def short_signal(index: int = 0, entry: float = 100.0, stop: float = 102.0, targets=None) -> Signal:
    targets = targets if targets is not None else [96.0]
    weights = [1.0 / len(targets)] * len(targets)
    return Signal(
        direction="short",
        index=index,
        ts=index * 1000,
        entry=entry,
        stop=stop,
        targets=list(targets),
        weights=weights,
        expires_at=index + 20,
        risk=abs(entry - stop),
        reward_risk=abs(targets[-1] - entry) / abs(entry - stop),
        order_block=OrderBlock("short", index, index * 1000, stop, entry),
        sweep_level=entry,
        sweep_extreme=stop,
        sweep_touches=1,
        displacement=2.0,
        atr=1.0,
    )


def test_a_signal_cannot_fill_on_the_bar_that_created_it():
    ex = Executor(zero_cost_config())
    sig = short_signal(index=5)
    ex.on_signal(5, sig)
    ex.on_bar(5, candle(5, 100.5, 101.0, 99.5, 100.2))
    assert ex.positions == [], "same-bar fills are an artefact of bar granularity"
    ex.on_bar(6, candle(6, 99.0, 100.5, 98.5, 99.5))
    assert len(ex.positions) == 1


def test_position_size_puts_exactly_risk_pct_at_stake():
    cfg = zero_cost_config()
    ex = Executor(cfg)
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.0, 99.0))

    pos = ex.positions[0]
    assert pos.entry_price == 100.0
    # 1% of 10,000 = 100 at risk, over a 2.00 stop distance = 50 units.
    assert pos.qty == 50.0
    assert abs(pos.qty * abs(pos.entry_price - pos.stop) - 100.0) < 1e-9


def test_a_bar_that_could_have_hit_both_stop_and_target_is_scored_as_the_stop():
    ex = Executor(zero_cost_config())
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
    ex.on_bar(1, candle(1, 100.0, 100.5, 99.0, 99.5))
    assert len(ex.positions) == 1

    # This bar spans 95..103: it reached the target and the stop.
    trades = ex.on_bar(2, candle(2, 99.5, 103.0, 95.0, 99.0))
    assert len(trades) == 1
    assert trades[0].exit_reason == "stop"
    assert trades[0].r_multiple < 0


def test_a_gap_through_the_limit_fills_at_the_open_not_at_a_better_price():
    ex = Executor(zero_cost_config())
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0))
    ex.on_bar(1, candle(1, 100.8, 101.0, 99.0, 99.5))
    assert ex.positions[0].entry_price == 100.8, "the fill is marked to the gap, not the limit"


def test_partial_targets_scale_out_and_move_the_stop_to_entry():
    cfg = zero_cost_config()
    cfg.entry.breakeven_after_tp = 1
    ex = Executor(cfg)
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[98.0, 96.0]))
    ex.on_bar(1, candle(1, 100.0, 100.2, 99.0, 99.5))
    pos = ex.positions[0]
    full_qty = pos.qty

    ex.on_bar(2, candle(2, 99.5, 99.6, 97.9, 98.5))  # first target only
    assert pos.filled_targets == 1
    assert abs(pos.remaining - full_qty / 2) < 1e-9
    assert pos.stop == pos.entry_price, "breakeven_after_tp should have moved the stop"

    trades = ex.on_bar(3, candle(3, 98.5, 98.7, 95.5, 96.0))
    assert len(trades) == 1
    assert trades[0].exit_reason == "tp2"
    assert trades[0].r_multiple > 0


def test_an_unfilled_order_is_pulled_when_it_expires():
    ex = Executor(zero_cost_config())
    sig = short_signal(index=0, entry=100.0, stop=102.0)
    ex.on_signal(0, sig)
    for i in range(1, sig.expires_at + 2):
        ex.on_bar(i, candle(i, 90.0, 91.0, 89.0, 90.0))  # price never returns
    assert ex.pending == []
    assert ex.positions == []
    assert ex.rejected.get("expired") == 1


def test_a_close_beyond_the_stop_cancels_the_order_even_if_price_crossed_the_entry():
    ex = Executor(zero_cost_config())
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0))
    # The bar trades through 100 on its way to closing above the stop.
    ex.on_bar(1, candle(1, 99.0, 103.0, 98.5, 102.5))
    assert ex.pending == []
    assert ex.positions == [], "the setup was invalidated, so the fill does not count"


def test_max_concurrent_counts_working_orders_not_just_open_positions():
    ex = Executor(zero_cost_config(max_concurrent=1))
    assert ex.on_signal(0, short_signal(index=0, entry=100.0)) is True
    assert ex.on_signal(0, short_signal(index=0, entry=101.0)) is False
    assert ex.rejected.get("max_concurrent") == 1


def test_fees_and_slippage_reduce_the_result():
    plain = Executor(zero_cost_config())
    costed_cfg = zero_cost_config()
    costed_cfg.costs.maker_fee_bps = 10.0
    costed_cfg.costs.taker_fee_bps = 20.0
    costed_cfg.costs.slippage_bps = 10.0
    costed = Executor(costed_cfg)

    bars = [
        candle(1, 100.0, 100.2, 99.0, 99.5),
        candle(2, 99.5, 99.6, 95.5, 96.0),
    ]
    for ex in (plain, costed):
        ex.on_signal(0, short_signal(entry=100.0, stop=102.0, targets=[96.0]))
        for i, bar in enumerate(bars, start=1):
            ex.on_bar(i, bar)

    assert plain.trades and costed.trades
    assert costed.trades[0].pnl < plain.trades[0].pnl
    assert costed.trades[0].fees > 0


def test_the_notional_cap_is_reported_because_it_breaks_constant_risk():
    """A tight stop makes the notional cap bind, and the trade then risks
    far less than risk_pct — which silently makes R multiples incomparable.
    """
    cfg = zero_cost_config()
    cfg.risk.risk_pct = 1.0
    cfg.risk.max_position_pct = 100.0  # notional may not exceed equity
    ex = Executor(cfg)

    # Stop is 0.1 wide at a price of 100: 1% of 10,000 = 100 at risk would
    # need 1,000 units = 100,000 notional, ten times the cap.
    ex.on_signal(0, short_signal(entry=100.0, stop=100.1, targets=[99.0]))
    ex.on_bar(1, candle(1, 99.5, 100.5, 99.0, 99.5))

    assert ex.capped_entries == 1
    assert ex.worst_risk_fraction < 0.2, "the trade risked a fraction of what was asked"
    pos = ex.positions[0]
    assert pos.qty * pos.entry_price <= cfg.risk.starting_equity * 1.0000001


def test_no_warning_when_the_cap_never_binds():
    cfg = zero_cost_config()
    cfg.risk.risk_pct = 1.0
    cfg.risk.max_position_pct = 10_000.0
    ex = Executor(cfg)
    ex.on_signal(0, short_signal(entry=100.0, stop=102.0))
    ex.on_bar(1, candle(1, 99.0, 100.5, 98.0, 99.0))
    assert ex.capped_entries == 0
    assert ex.worst_risk_fraction == 1.0
