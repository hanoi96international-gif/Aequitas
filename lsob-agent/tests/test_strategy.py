"""End-to-end detector tests over a hand-built short setup.

The scenario below is the textbook sequence the agent exists to find:
a swing high leaves stops above it, price raids that high and closes back
under it, then sells off through the last pullback low. Every assertion here
is about that sequence, so when one fails it names the rule that broke.
"""

from __future__ import annotations

import copy

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
