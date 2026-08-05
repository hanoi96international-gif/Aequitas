from __future__ import annotations

import re

import pytest
from test_backtest import WITH_RETRACE
from test_strategy import base_config, run

from lsob.chart import Level, levels_for, render_svg, window_around


def a_signal():
    cfg = base_config()
    signals = run(cfg, WITH_RETRACE)
    assert signals, "the fixture must produce a setup to draw"
    return signals[0]


def test_every_level_of_the_signal_is_drawn_and_named():
    sig = a_signal()
    svg = render_svg(WITH_RETRACE, sig, "test")
    for text in ("swept liquidity", "raid extreme", "entry", "stop", "TP1", "TP2", "order block"):
        assert text in svg, f"{text!r} missing from the chart"


def test_the_output_is_well_formed_standalone_svg():
    import xml.etree.ElementTree as ET

    svg = render_svg(WITH_RETRACE, a_signal(), "test")
    root = ET.fromstring(svg)  # raises if malformed
    assert root.tag.endswith("svg")
    assert "http" not in svg.replace("http://www.w3.org/2000/svg", ""), "no external references"


def test_labels_and_titles_are_escaped():
    svg = render_svg(WITH_RETRACE, a_signal(), 'BTC <script>alert("x")</script> & co')
    assert "<script>" not in svg
    assert "&lt;script&gt;" in svg and "&amp;" in svg


def test_both_themes_render_and_differ_in_surface():
    sig = a_signal()
    light = render_svg(WITH_RETRACE, sig, "t", theme="light")
    dark = render_svg(WITH_RETRACE, sig, "t", theme="dark")
    assert "#fcfcfb" in light and "#1a1a19" in dark
    assert light != dark


def test_close_levels_get_separated_labels():
    """Two levels a hair apart must not print their labels on top of each other."""
    sig = a_signal()
    crowded = [
        Level(sig.entry, "one", "liquidity"),
        Level(sig.entry * 1.0001, "two", "liquidity"),
        Level(sig.entry * 1.0002, "three", "liquidity"),
    ]
    svg = render_svg(WITH_RETRACE, sig, "t", crowded)
    ys = [float(m) for m in re.findall(r'<text x="[\d.]+" y="([\d.]+)" fill="#[0-9a-f]{6}" font-size="11"', svg)]
    assert len(ys) == 3
    for earlier, later in zip(sorted(ys), sorted(ys)[1:], strict=False):
        assert later - earlier >= 12.0, "labels are still overlapping"


def test_the_price_scale_covers_both_candles_and_levels():
    """A stop far outside the visible candles must still be on the canvas."""
    sig = a_signal()
    far = [Level(sig.entry * 2.0, "far above", "stop")]
    svg = render_svg(WITH_RETRACE, sig, "t", far)
    ys = [float(m) for m in re.findall(r'y1="([\d.]+)"', svg)]
    assert ys and min(ys) >= 0, "nothing may be drawn off the top of the canvas"
    assert max(float(m) for m in re.findall(r'y2="([\d.]+)"', svg)) <= 460


def test_the_window_is_centred_on_the_signal_bar():
    sig = a_signal()
    window = window_around(WITH_RETRACE, sig, before=5, after=3)
    assert any(c.ts == sig.ts for c in window)
    assert len(window) <= 9


def test_an_inducement_level_is_included_when_given():
    sig = a_signal()
    assert "inducement" not in "".join(lv.label for lv in levels_for(sig))
    assert "inducement" in "".join(lv.label for lv in levels_for(sig, inducement=sig.entry * 0.99))


def test_drawing_nothing_is_an_error_not_a_blank_chart():
    with pytest.raises(ValueError, match="nothing to draw"):
        render_svg([], a_signal(), "t")


# ── the retracement ladder ───────────────────────────────────────────────


def test_the_ladder_reproduces_the_ratios_it_was_given():
    from lsob.chart import fib_ladder

    sig = a_signal()
    ratios = [0.236, 0.5, 0.882]
    ladder = fib_ladder(sig, ratios)
    assert len(ladder) == 3

    span = abs(sig.sweep_extreme - sig.leg_extreme)
    assert span > 0, "the fixture's signal must carry its leg"
    for level, ratio in zip(ladder, ratios, strict=True):
        travelled = abs(level.price - sig.leg_extreme) / span
        assert travelled == pytest.approx(ratio, abs=1e-9)
        assert f"{ratio:.3f}" in level.label


def test_the_entry_ratio_lands_on_its_own_rung():
    """The 0.882 rung and a 0.882 entry must be the same price, not merely close."""
    from lsob.chart import fib_ladder
    from test_strategy import base_config

    cfg = base_config()
    cfg.entry.edge = "retracement"
    cfg.entry.retracement = 0.882
    cfg.entry.retracement_in_block = False
    sig = run(cfg, WITH_RETRACE)[0]

    rung = next(lv for lv in fib_ladder(sig, [0.882]))
    assert rung.price == pytest.approx(sig.entry, abs=1e-9)


def test_a_signal_without_a_leg_draws_no_ladder():
    from dataclasses import replace

    from lsob.chart import fib_ladder

    assert fib_ladder(replace(a_signal(), leg_extreme=0.0), [0.5]) == []


def test_ladder_rungs_are_drawn_and_labelled_with_ratio_and_price():
    from lsob.chart import fib_ladder

    sig = a_signal()
    svg = render_svg(WITH_RETRACE, sig, "t", levels_for(sig) + fib_ladder(sig, [0.5, 0.786]))
    assert "0.500" in svg and "0.786" in svg


def test_crowded_rungs_get_separated_labels():
    """0.786/0.882/0.941 sit within a few ticks — their labels must not stack."""
    from lsob.chart import fib_ladder

    sig = a_signal()
    ladder = fib_ladder(sig, [0.786, 0.882, 0.941])
    svg = render_svg(WITH_RETRACE, sig, "t", ladder)
    ys = [
        float(m)
        for m in re.findall(
            r'<text x="[\d.]+" y="([\d.]+)" fill="#[0-9a-f]{6}" font-size="9.5"', svg
        )
    ]
    assert len(ys) == 3
    for earlier, later in zip(sorted(ys), sorted(ys)[1:], strict=False):
        assert later - earlier >= 10.0
