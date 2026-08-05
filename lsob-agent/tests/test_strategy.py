"""End-to-end detector tests over a hand-built short setup.

The scenario below is the textbook sequence the agent exists to find:
a swing high leaves stops above it, price raids that high and closes back
under it, then sells off through the last pullback low. Every assertion here
is about that sequence, so when one fails it names the rule that broke.
"""

from __future__ import annotations

import copy

import pytest

from conftest import candle, flat

from lsob.config import Config
from lsob.strategy import LsobStrategy

# bar 24 prints the swing high at 106  -> buy-side liquidity
# bar 27 prints the pullback low at 100.5 -> the level a break must take
# bar 31 raids 106 up to 107 and closes back at 104.5 -> the sweep
# bars 32-33 sell off through 100.5     -> displacement + break of structure
SETUP = (
    flat(21, 100.0)
    + [
        candle(21, 100.5, 102.0, 100.0, 101.5),
        candle(22, 101.5, 103.0, 101.0, 102.5),
        candle(23, 102.5, 104.0, 102.0, 103.5),
        candle(24, 103.5, 106.0, 103.0, 105.0),  # swing high
        candle(25, 105.0, 105.5, 103.0, 103.5),
        candle(26, 103.5, 104.0, 101.5, 102.0),
        candle(27, 102.0, 102.5, 100.5, 101.0),  # swing low
        candle(28, 101.0, 102.0, 101.0, 101.8),
        candle(29, 101.8, 103.0, 101.5, 102.8),
        candle(30, 102.8, 104.0, 102.5, 103.8),
        candle(31, 103.8, 107.0, 103.5, 104.5),  # sweep: wick over 106, close under
        candle(32, 104.5, 104.7, 101.0, 101.2),
        candle(33, 101.2, 101.5, 98.5, 99.0),  # break of structure through 100.5
        candle(34, 99.0, 99.5, 97.0, 97.5),
        candle(35, 97.5, 98.0, 96.0, 96.5),
        candle(36, 96.5, 97.0, 95.0, 95.5),
    ]
)


def base_config() -> Config:
    cfg = Config()
    cfg.entry.tp_rr = [1.5, 3.0]
    cfg.entry.tp_weights = [0.5, 0.5]
    return cfg


def run(cfg: Config, candles=SETUP):
    strategy = LsobStrategy(cfg)
    out = []
    for c in candles:
        out.extend(strategy.on_bar(c))
    return out


def test_the_textbook_sequence_produces_exactly_one_short_setup():
    signals = run(base_config())
    shorts = [s for s in signals if s.direction == "short"]
    assert len(shorts) == 1, [s.label for s in signals]

    sig = shorts[0]
    assert sig.index == 33, "the setup completes on the bar that breaks structure"
    assert sig.sweep_level == 106.0
    assert sig.sweep_extreme == 107.0
    assert sig.entry == 103.8, "proximal edge of the sweep candle's body"
    assert sig.stop > 107.0, "the stop sits beyond the raid's extreme, plus a buffer"
    assert len(sig.targets) == 2
    assert sig.targets[0] > sig.targets[1], "targets run away from entry for a short"
    assert sig.reward_risk >= cfg_min_rr()
    assert sig.displacement >= 1.0


def cfg_min_rr() -> float:
    return base_config().entry.min_rr


def test_no_setup_without_the_break_of_structure():
    """Truncating before the BOS bar must leave the sweep unconfirmed."""
    cfg = base_config()
    assert run(cfg, SETUP[:33]) == []


def test_fvg_filter_accepts_a_leg_that_gapped():
    """Bar 33's high never reaches bar 31's low, so this leg does gap."""
    cfg = base_config()
    cfg.orderblock.require_fvg = True
    assert len(run(cfg)) == 1


def test_fvg_filter_rejects_a_leg_that_did_not_gap():
    """Same setup, but the sell-off overlaps bar by bar — no untraded band."""
    overlapping = list(SETUP[:32]) + [
        candle(32, 104.5, 104.7, 102.5, 102.8),
        candle(33, 102.8, 103.6, 100.0, 100.2),  # high reaches back into bar 31
        candle(34, 100.2, 102.6, 99.0, 99.5),
    ]
    cfg = base_config()
    assert len(run(cfg, overlapping)) == 1, "the setup itself is still valid"

    cfg.orderblock.require_fvg = True
    assert run(cfg, overlapping) == []


def test_a_displacement_threshold_nobody_can_meet_suppresses_everything():
    cfg = base_config()
    cfg.orderblock.displacement_atr = 50.0
    assert run(cfg) == []


def test_min_rr_filters_setups_that_cannot_pay():
    cfg = base_config()
    cfg.entry.tp_rr = [1.0]
    cfg.entry.tp_weights = [1.0]
    cfg.entry.min_rr = 2.0
    assert run(cfg) == []


def test_direction_switches_can_disable_a_side():
    cfg = base_config()
    cfg.risk.allow_short = False
    assert [s for s in run(cfg) if s.direction == "short"] == []


def test_reclaim_window_decides_whether_a_late_reclaim_still_counts():
    """The raid bar closes *above* the swept high and only reclaims next bar."""
    candles = copy.deepcopy(list(SETUP))
    candles[31] = candle(31, 103.8, 107.0, 103.5, 106.5)

    lenient = base_config()
    lenient.liquidity.reclaim_bars = 2
    assert len(run(lenient, candles)) == 1, "a next-bar reclaim is still a rejection"

    strict = base_config()
    strict.liquidity.reclaim_bars = 0  # same-bar close-back-inside or nothing
    assert run(strict, candles) == []


def test_closing_back_above_the_raid_extreme_abandons_the_setup():
    """The premise dies if price reclaims the high before the entry is built."""
    candles = list(SETUP[:32]) + [
        candle(32, 104.5, 108.5, 104.0, 108.0),  # straight back through 107
        candle(33, 108.0, 108.5, 100.0, 100.5),
    ]
    assert [s for s in run(base_config(), candles) if s.index == 33] == []


def test_bias_filter_can_veto_a_valid_setup():
    cfg = base_config()
    cfg.bias.mode = "ema"
    cfg.bias.ema_period = 10  # price is below its EMA here, so a short is allowed
    assert len(run(cfg)) == 1

    cfg2 = base_config()
    cfg2.bias.mode = "ema"
    cfg2.bias.ema_period = 5
    cfg2.risk.allow_short = True
    signals = run(cfg2)
    assert all(s.direction == "short" for s in signals)


def test_liquidity_targets_fall_back_to_r_multiples_when_no_pool_exists():
    cfg = base_config()
    cfg.entry.tp_mode = "liquidity"
    signals = run(cfg)
    assert len(signals) == 1
    assert signals[0].targets[-1] < signals[0].entry


def test_a_short_whose_target_would_fall_below_zero_is_rejected():
    """Found on real monthly BTC data: a wide stop pushed 3R under zero.

    The setup is scaled so the stop sits far from entry; the resulting
    R-multiple target is a negative price, which no market can reach.
    """
    scaled = [
        candle(
            i,
            c.open * 400,
            c.high * 400,
            c.low * 400,
            c.close * 400,
            c.volume,
        )
        for i, c in enumerate(SETUP)
    ]
    cfg = base_config()
    cfg.entry.tp_rr = [1.5, 300.0]  # a target far beyond zero for this short
    cfg.entry.tp_weights = [0.5, 0.5]
    cfg.entry.min_rr = 1.0

    for sig in run(cfg, scaled):
        assert all(t > 0 for t in sig.targets), f"unreachable target: {sig.targets}"
    assert run(cfg, scaled) == [], "the setup cannot pay its target, so it is not emitted"


# ── 88.2% retracement entry ──────────────────────────────────────────────
#
# The displacement leg on the reference setup runs from the raid extreme at
# 107 down to a low of 98.5. A 0.882 retracement of that leg sits at
# 98.5 + 0.882 * 8.5 = 105.997 — inside the order block (103.8-107), which is
# the arrangement these two rules are meant to produce.


def test_the_retracement_entry_lands_where_the_leg_says_it_should():
    from lsob.orderblock import retracement_level

    assert retracement_level(107.0, 98.5, "short", 0.882) == pytest.approx(105.997)
    assert retracement_level(98.5, 107.0, "long", 0.882) == pytest.approx(99.503)
    # A full retracement returns to the raid's own extreme.
    assert retracement_level(107.0, 98.5, "short", 1.0) == pytest.approx(107.0)


def test_edge_retracement_prices_the_entry_off_the_leg_not_the_block():
    cfg = base_config()
    cfg.entry.edge = "retracement"
    signals = run(cfg)
    assert len(signals) == 1
    sig = signals[0]
    assert sig.entry == pytest.approx(105.997, abs=0.01)
    assert sig.order_block.bottom <= sig.entry <= sig.order_block.top


def test_a_deeper_entry_buys_a_tighter_stop_and_more_r():
    shallow = base_config()
    shallow.entry.edge = "proximal"  # the block's near edge, at 103.8
    deep = base_config()
    deep.entry.edge = "retracement"  # 88.2% of the leg, at ~106.0

    near = run(shallow)[0]
    far = run(deep)[0]
    assert far.entry > near.entry, "the deep entry sits closer to the raid extreme"
    assert far.risk < near.risk, "and therefore risks less per unit"


def test_a_retracement_that_misses_the_block_is_rejected_only_when_required():
    """0.30 of the leg is 101.05 — a real level, but not inside the block."""
    cfg = base_config()
    cfg.entry.edge = "retracement"
    cfg.entry.retracement = 0.30
    cfg.entry.retracement_in_block = True
    assert run(cfg) == []

    cfg.entry.retracement_in_block = False
    signals = run(cfg)
    assert len(signals) == 1, "without the agreement rule the shallow level stands"
    assert signals[0].entry == pytest.approx(101.05, abs=0.01)
    assert signals[0].entry > SETUP[33].close, "still on the far side of price"


def test_a_retracement_shallower_than_price_has_already_come_is_rejected():
    """1% of the leg is 98.58, below the close of 99 — price is already past it."""
    cfg = base_config()
    cfg.entry.edge = "retracement"
    cfg.entry.retracement = 0.01
    cfg.entry.retracement_in_block = False
    assert run(cfg) == []


def test_the_ratio_is_configurable_for_any_variant_of_this_rule():
    for ratio, expected in ((0.886, 106.03), (0.79, 105.22), (0.95, 106.58)):
        cfg = base_config()
        cfg.entry.edge = "retracement"
        cfg.entry.retracement = ratio
        cfg.entry.retracement_in_block = False
        signals = run(cfg)
        assert len(signals) == 1, f"ratio {ratio} produced no setup"
        assert signals[0].entry == pytest.approx(expected, abs=0.02)


def test_an_out_of_range_ratio_is_rejected_by_config_validation():
    from lsob.config import validate

    cfg = base_config()
    for bad in (0.0, -0.5, 1.5):
        cfg.entry.retracement = bad
        with pytest.raises(ValueError, match="retracement"):
            validate(cfg)


# ── fib-level take profits (sniping a rung, exiting on rungs) ────────────


def sniper_config(tp_fib, ratio=0.882):
    cfg = base_config()
    cfg.entry.edge = "retracement"
    cfg.entry.retracement = ratio
    cfg.entry.retracement_in_block = False
    cfg.entry.tp_mode = "fib"
    cfg.entry.tp_fib = list(tp_fib)
    cfg.entry.tp_weights = [1.0 / len(tp_fib)] * len(tp_fib)
    cfg.entry.min_rr = 0.1
    return cfg


def test_fib_targets_land_on_the_rungs_they_name():
    from lsob.orderblock import retracement_level

    cfg = sniper_config([0.786, 0.5])
    sig = run(cfg)[0]
    for target, ratio in zip(sig.targets, [0.786, 0.5], strict=True):
        expected = retracement_level(sig.sweep_extreme, sig.leg_extreme, "short", ratio)
        assert target == pytest.approx(expected, abs=1e-9)


def test_the_entry_rung_and_the_target_rungs_share_one_leg():
    """Entry and exits must be measured across the same span, or R is fiction."""
    cfg = sniper_config([0.5])
    sig = run(cfg)[0]
    span = abs(sig.sweep_extreme - sig.leg_extreme)
    assert abs(sig.entry - sig.leg_extreme) / span == pytest.approx(0.882, abs=1e-9)
    assert abs(sig.targets[0] - sig.leg_extreme) / span == pytest.approx(0.5, abs=1e-9)


def test_scalping_to_the_next_rung_cannot_pay_one_r():
    """Geometry, not market behaviour: 0.882 to 0.786 is 0.096 of the leg
    while the stop beyond 1.0 costs at least 0.118 of it. The setup is
    rejected by min_rr rather than taken at a structural loss.
    """
    cfg = sniper_config([0.786])
    cfg.entry.min_rr = 1.0
    assert run(cfg) == []

    generous = sniper_config([0.65])
    generous.entry.min_rr = 1.0
    assert len(run(generous)) == 1, "the next rung down clears 1R comfortably"


def test_a_rung_behind_price_voids_the_setup_rather_than_retargeting_it():
    """0.941 sits above a 0.882 short entry — that is not a target."""
    cfg = sniper_config([0.941])
    assert run(cfg) == []


def test_fib_targets_must_be_ordered_from_nearest_to_furthest():
    from lsob.config import validate

    cfg = sniper_config([0.5, 0.786])  # the wrong way round
    with pytest.raises(ValueError, match="nearest rung"):
        validate(cfg)


def test_fib_target_mode_needs_matching_weights():
    from lsob.config import validate

    cfg = sniper_config([0.786, 0.5])
    cfg.entry.tp_weights = [1.0]
    with pytest.raises(ValueError, match="same length"):
        validate(cfg)


# ── stop on the 1.0 rung ─────────────────────────────────────────────────


def test_the_stop_sits_exactly_on_the_rung_it_names():
    from lsob.orderblock import retracement_level

    cfg = sniper_config([0.5])
    cfg.entry.sl_anchor = "fib"
    cfg.entry.sl_fib = 1.0
    sig = run(cfg)[0]

    expected = retracement_level(sig.leg_raid_end or sig.sweep_extreme, sig.leg_extreme, "short", 1.0)
    assert sig.stop == pytest.approx(expected, abs=1e-9)
    assert sig.stop == pytest.approx(sig.sweep_extreme, abs=1e-9), (
        "under a local anchor the 1.0 rung is the raid extreme itself"
    )


def test_a_fib_stop_ignores_the_atr_buffer_on_purpose():
    """One ruler for the whole trade — an ATR pad would reintroduce a second."""
    tight = sniper_config([0.5])
    tight.entry.sl_anchor = "fib"
    tight.entry.sl_buffer_atr = 0.0
    wide = sniper_config([0.5])
    wide.entry.sl_anchor = "fib"
    wide.entry.sl_buffer_atr = 5.0

    assert run(tight)[0].stop == run(wide)[0].stop


def test_clearance_belongs_in_the_rung_not_the_buffer():
    at_one = sniper_config([0.5])
    at_one.entry.sl_anchor = "fib"
    at_one.entry.sl_fib = 1.0
    beyond = sniper_config([0.5])
    beyond.entry.sl_anchor = "fib"
    beyond.entry.sl_fib = 1.05

    assert run(beyond)[0].stop > run(at_one)[0].stop, "1.05 sits past the leg's end"


def test_the_whole_trade_is_measured_on_one_ladder():
    """Entry 0.882, stop 1.0, target 0.5 — every price is a rung of one leg."""
    cfg = sniper_config([0.5])
    cfg.entry.sl_anchor = "fib"
    cfg.entry.sl_fib = 1.0
    sig = run(cfg)[0]

    span = abs((sig.leg_raid_end or sig.sweep_extreme) - sig.leg_extreme)
    for price, ratio in ((sig.entry, 0.882), (sig.stop, 1.0), (sig.targets[0], 0.5)):
        assert abs(price - sig.leg_extreme) / span == pytest.approx(ratio, abs=1e-9)

    # Risk is then a pure property of the ladder: 1.0 - 0.882 of the leg.
    assert sig.risk / span == pytest.approx(1.0 - 0.882, abs=1e-9)


def test_a_stop_rung_inside_the_entry_is_rejected():
    from lsob.config import validate

    cfg = sniper_config([0.5])
    cfg.entry.sl_anchor = "fib"
    cfg.entry.sl_fib = 0.8  # nearer the leg's far end than the 0.882 entry
    with pytest.raises(ValueError, match="beyond"):
        validate(cfg)


def test_a_fib_stop_requires_a_fib_entry():
    from lsob.config import validate

    cfg = base_config()
    cfg.entry.sl_anchor = "fib"
    with pytest.raises(ValueError, match="retracement"):
        validate(cfg)
