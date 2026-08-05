"""Draw a setup as SVG, so its levels can be checked by eye rather than argued about.

A backtest reports what happened; it cannot tell you whether the levels were
drawn where you would have drawn them. This renders one setup — the raided
level, the order block, the retracement line, the entry, stop and targets —
onto its candles, at the point in the chart where the agent acted.

Colour carries only two things: the **retracement line** and the **liquidity
levels**. Everything else is ink or status. That is deliberate — candles are
context, and painting them red and green would put four saturated hues on
screen competing with the two that mean something. Every line is also
directly labelled, so nothing depends on hue alone.

Stdlib only, like the rest of the package: this writes SVG text.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone

from .model import Candle
from .orderblock import retracement_level
from .strategy import Signal

# Validated in both modes against the chart surfaces below (all-pairs:
# CVD ΔE 24.7 light / 26.8 dark, normal-vision 33.6 / 31.8).
_LIGHT = {
    "surface": "#fcfcfb",
    "ink": "#0b0b0b",
    "muted": "#898781",
    "grid": "#e1e0d9",
    "axis": "#c3c2b7",
    "retracement": "#2a78d6",
    "liquidity": "#eb6834",
    "stop": "#d03b3b",
    "target": "#0ca30c",
    "fib": "#7d7b75",
    "zone": "rgba(11,11,11,0.07)",
    "candle_up": "#fcfcfb",
    "candle_down": "#52514e",
    "candle_edge": "#52514e",
}
_DARK = {
    "surface": "#1a1a19",
    "ink": "#ffffff",
    "muted": "#898781",
    "grid": "#2c2c2a",
    "axis": "#383835",
    "retracement": "#3987e5",
    "liquidity": "#d95926",
    "stop": "#d03b3b",
    "target": "#0ca30c",
    "fib": "#918f88",
    "zone": "rgba(255,255,255,0.09)",
    "candle_up": "#1a1a19",
    "candle_down": "#c3c2b7",
    "candle_edge": "#c3c2b7",
}


@dataclass(slots=True)
class Level:
    price: float
    label: str
    role: str  # retracement | liquidity | stop | target | entry
    dashed: bool = True


def _escape(text: str) -> str:
    return (
        text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")
    )


def _fmt(price: float) -> str:
    if price >= 1000:
        return f"{price:,.0f}"
    if price >= 10:
        return f"{price:.2f}"
    return f"{price:.5g}"


def fib_ladder(signal: Signal, ratios: list[float]) -> list[Level]:
    """Every configured retracement ratio across the leg, priced and labelled.

    The chart draws the whole ladder even though only one ratio selects the
    entry. Read on its own, a single line says nothing about whether it sits
    where it should; beside 0.5 and 0.786 it is immediately obvious which
    part of the leg the entry is in.
    """
    if signal.leg_extreme <= 0:
        return []
    out: list[Level] = []
    for ratio in ratios:
        price = retracement_level(
            signal.sweep_extreme, signal.leg_extreme, signal.direction, ratio
        )
        out.append(Level(price, f"{ratio:.3f}  {_fmt(price)}", "fib"))
    return out


def levels_for(signal: Signal, inducement: float | None = None) -> list[Level]:
    """The lines a reader needs to judge whether the agent read the chart right."""
    out = [
        Level(signal.sweep_level, "swept liquidity", "liquidity"),
        Level(signal.sweep_extreme, "raid extreme", "liquidity"),
        Level(signal.entry, f"entry {_fmt(signal.entry)}", "retracement", dashed=False),
        Level(signal.stop, f"stop {_fmt(signal.stop)}", "stop"),
    ]
    for n, target in enumerate(signal.targets, start=1):
        out.append(Level(target, f"TP{n} {_fmt(target)}", "target"))
    if inducement is not None:
        out.append(Level(inducement, "inducement", "liquidity"))
    return out


def render_svg(
    candles: list[Candle],
    signal: Signal,
    title: str,
    levels: list[Level] | None = None,
    width: int = 900,
    height: int = 460,
    theme: str = "light",
) -> str:
    """Render `candles` with `signal`'s levels drawn across them."""
    if not candles:
        raise ValueError("nothing to draw")
    palette = _LIGHT if theme == "light" else _DARK
    levels = levels if levels is not None else levels_for(signal)

    pad_left, pad_right, pad_top, pad_bottom = 8, 132, 34, 26
    plot_w = width - pad_left - pad_right
    plot_h = height - pad_top - pad_bottom

    lows = [c.low for c in candles] + [lv.price for lv in levels]
    highs = [c.high for c in candles] + [lv.price for lv in levels]
    lo, hi = min(lows), max(highs)
    span = (hi - lo) or 1.0
    lo -= span * 0.06
    hi += span * 0.06
    span = hi - lo

    def y_of(price: float) -> float:
        return pad_top + plot_h * (hi - price) / span

    step = plot_w / max(len(candles), 1)
    body = max(1.6, min(step * 0.62, 11.0))

    def x_of(index: int) -> float:
        return pad_left + step * (index + 0.5)

    parts: list[str] = []
    add = parts.append

    add(
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
        f'width="100%" role="img" aria-label="{_escape(title)}" '
        f'style="max-width:100%;height:auto;font-family:system-ui,-apple-system,\'Segoe UI\',sans-serif">'
    )
    add(f'<rect width="{width}" height="{height}" fill="{palette["surface"]}"/>')
    add(
        f'<text x="{pad_left}" y="20" fill="{palette["ink"]}" font-size="13" '
        f'font-weight="600">{_escape(title)}</text>'
    )

    # Order block band first: it is the stage the lines stand on.
    ob = signal.order_block
    zone_top, zone_bottom = y_of(ob.top), y_of(ob.bottom)
    add(
        f'<rect x="{pad_left}" y="{zone_top:.1f}" width="{plot_w:.1f}" '
        f'height="{max(zone_bottom - zone_top, 1):.1f}" fill="{palette["zone"]}"/>'
    )
    add(
        f'<rect x="{pad_left}" y="{zone_top:.1f}" width="{plot_w:.1f}" '
        f'height="{max(zone_bottom - zone_top, 1):.1f}" fill="none" '
        f'stroke="{palette["axis"]}" stroke-width="1" stroke-dasharray="2 3"/>'
    )
    # A backing plate, because the band is often thin enough that the caption
    # would otherwise sit directly on candle bodies.
    # Above the band, or below it when the band sits at the very top of the
    # plot. Inside would read as a label *on* the entry line, which crosses it.
    caption_y = zone_top - 6 if zone_top - 6 > pad_top + 10 else zone_bottom + 12
    caption_x = pad_left + plot_w - 64
    add(
        f'<rect x="{caption_x - 3:.1f}" y="{caption_y - 7:.1f}" width="64" height="14" '
        f'rx="2" fill="{palette["surface"]}" opacity="0.88"/>'
    )
    add(
        f'<text x="{caption_x:.1f}" y="{caption_y + 3.5:.1f}" '
        f'fill="{palette["muted"]}" font-size="10">order block</text>'
    )

    # Candles: neutral, so the coloured lines are the only thing that shouts.
    for i, candle in enumerate(candles):
        x = x_of(i)
        add(
            f'<line x1="{x:.1f}" y1="{y_of(candle.high):.1f}" x2="{x:.1f}" '
            f'y2="{y_of(candle.low):.1f}" stroke="{palette["candle_edge"]}" stroke-width="1"/>'
        )
        top, bottom = y_of(candle.body_top), y_of(candle.body_bottom)
        fill = palette["candle_up"] if candle.close >= candle.open else palette["candle_down"]
        add(
            f'<rect x="{x - body / 2:.1f}" y="{top:.1f}" width="{body:.1f}" '
            f'height="{max(bottom - top, 1.2):.1f}" fill="{fill}" '
            f'stroke="{palette["candle_edge"]}" stroke-width="1"/>'
        )

    # The signal bar, marked so "where it acted" needs no counting.
    signal_x = None
    for i, candle in enumerate(candles):
        if candle.ts == signal.ts:
            signal_x = x_of(i)
            break
    if signal_x is not None:
        marker = signal_x
        add(
            f'<line x1="{marker:.1f}" y1="{pad_top}" x2="{marker:.1f}" '
            f'y2="{pad_top + plot_h}" stroke="{palette["axis"]}" stroke-width="1" '
            f'stroke-dasharray="1 3"/>'
        )
        add(
            f'<text x="{marker:.1f}" y="{pad_top - 6}" fill="{palette["muted"]}" '
            f'font-size="10" text-anchor="middle">signal</text>'
        )

    # The ladder goes under the trade levels: it is the ruler the entry is
    # read against, not a line anyone acts on. Labels sit on the left so the
    # right gutter stays reserved for the levels that carry orders.
    ladder = [lv for lv in levels if lv.role == "fib"]
    ladder = sorted(
        (lv for lv in ladder if pad_top <= y_of(lv.price) <= pad_top + plot_h),
        key=lambda lv: -lv.price,
    )
    # The square-root series clusters hard at the deep end — 0.786, 0.882 and
    # 0.941 can land within a few ticks of each other — so the labels need the
    # same nudging the trade levels get, or the deepest three are unreadable
    # exactly where the entry is.
    ladder_y: list[float] = []
    for level in ladder:
        y = y_of(level.price)
        if ladder_y and y - ladder_y[-1] < 11.0:
            y = ladder_y[-1] + 11.0
        ladder_y.append(y)

    for level, ly in zip(ladder, ladder_y, strict=True):
        y = y_of(level.price)
        add(
            f'<line x1="{pad_left}" y1="{y:.1f}" x2="{pad_left + plot_w:.1f}" y2="{y:.1f}" '
            f'stroke="{palette["fib"]}" stroke-width="1" opacity="0.5"/>'
        )
        add(
            f'<rect x="{pad_left + 1}" y="{ly - 6.5:.1f}" width="78" height="13" rx="2" '
            f'fill="{palette["surface"]}" opacity="0.85"/>'
        )
        add(
            f'<text x="{pad_left + 4}" y="{ly + 3.5:.1f}" fill="{palette["fib"]}" '
            f'font-size="9.5" font-variant-numeric="tabular-nums">{_escape(level.label)}</text>'
        )

    # Trade levels last, so nothing draws over them. Labels sit in the right gutter,
    # nudged apart where the levels themselves are closer together than the
    # text is tall — a stop and a raid extreme two ticks apart are common, and
    # overlapping labels would make exactly those setups unreadable.
    ordered = sorted([lv for lv in levels if lv.role != "fib"], key=lambda lv: -lv.price)
    label_y: list[float] = []
    min_gap = 13.0
    for level in ordered:
        y = y_of(level.price)
        if label_y and y - label_y[-1] < min_gap:
            y = label_y[-1] + min_gap
        label_y.append(y)

    for level, ly in zip(ordered, label_y, strict=True):
        y = y_of(level.price)
        colour = palette[level.role]
        dash = ' stroke-dasharray="6 4"' if level.dashed else ""
        emphasis = 2.4 if level.role == "retracement" and not level.dashed else 1.6
        add(
            f'<line x1="{pad_left}" y1="{y:.1f}" x2="{pad_left + plot_w:.1f}" y2="{y:.1f}" '
            f'stroke="{colour}" stroke-width="{emphasis}"{dash}/>'
        )
        if abs(ly - y) > 1.0:
            # A leader, so a nudged label still points at its own line.
            add(
                f'<path d="M{pad_left + plot_w:.1f} {y:.1f} L{pad_left + plot_w + 4:.1f} '
                f'{ly:.1f}" stroke="{colour}" stroke-width="1" fill="none" opacity="0.6"/>'
            )
        add(
            f'<text x="{pad_left + plot_w + 6:.1f}" y="{ly + 3.5:.1f}" fill="{colour}" '
            f'font-size="11" font-weight="{600 if not level.dashed else 400}">'
            f'{_escape(level.label)}</text>'
        )

    first = datetime.fromtimestamp(candles[0].ts / 1000, tz=timezone.utc)
    last = datetime.fromtimestamp(candles[-1].ts / 1000, tz=timezone.utc)
    add(
        f'<text x="{pad_left}" y="{height - 8}" fill="{palette["muted"]}" font-size="10">'
        f'{first:%Y-%m-%d %H:%M} — {last:%Y-%m-%d %H:%M} UTC · {len(candles)} bars</text>'
    )
    add("</svg>")
    return "".join(parts)


def window_around(candles: list[Candle], signal: Signal, before: int, after: int) -> list[Candle]:
    """The slice of candles a reader needs to see the setup form and resolve."""
    for i, candle in enumerate(candles):
        if candle.ts == signal.ts:
            return candles[max(0, i - before) : i + after + 1]
    return candles[-(before + after + 1) :]
