"""Typed configuration loaded from TOML.

TOML rather than YAML on purpose: `tomllib` ships with Python 3.11, so the
config format costs no dependency while still allowing comments — and every
strategy rule here is a value you will want to leave a note next to.

Unknown keys are an error, not a shrug. A silently ignored typo in a
threshold is a strategy change you did not make.
"""

from __future__ import annotations

import tomllib
from dataclasses import dataclass, field, fields
from pathlib import Path
from typing import Any

from .liquidity import LiquidityConfig


@dataclass(slots=True)
class MarketConfig:
    exchange: str = "binance"
    symbol: str = "BTC/USDT"
    timeframe: str = "15m"


@dataclass(slots=True)
class DataConfig:
    csv: str = ""  # when set, read this file instead of calling an exchange
    cache_dir: str = "data"
    history_bars: int = 3000
    # A second, finer series covering the same period. Where a coarse bar
    # touches both the stop and a target, the backtester looks inside it
    # instead of assuming the stop came first. Only that ambiguity — a stop on
    # the fill bar is not one, since the entry lies between the open and the
    # stop by construction.
    intrabar_csv: str = ""
    # Real series contain bars no market printed. Dropping them is on by
    # default because one 395x-range bar poisons ATR for `atr_period` bars.
    clean: bool = True
    spike_ratio: float = 10.0   # max high/low within one bar
    jump_ratio: float = 10.0    # max close-to-close move against the previous good bar


@dataclass(slots=True)
class StructureConfig:
    swing_left: int = 3
    swing_right: int = 3
    atr_period: int = 14
    bos_use_close: bool = True


@dataclass(slots=True)
class OrderBlockConfig:
    displacement_atr: float = 1.0
    displacement_max_bars: int = 8
    require_bos: bool = True
    require_fvg: bool = False
    ob_max_lookback: int = 12
    ob_include_sweep_candle: bool = True
    zone_mode: str = "body_to_extreme"  # body | full | body_to_extreme


@dataclass(slots=True)
class EntryConfig:
    edge: str = "proximal"  # proximal | mid | distal | retracement
    # Used when edge = "retracement": how far back across the displacement leg
    # the limit sits, as a fraction. 0.882 is this project's default, not a
    # standard — set it to whatever ratio the strategy being modelled uses.
    retracement: float = 0.882
    # Reject a retracement entry that lands outside the order block. The two
    # are meant to agree; when they do not, the leg and the block are
    # describing different moves and the setup is not the one being traded.
    retracement_in_block: bool = True
    # Which span the ratios are measured across.
    #   "displacement" — the leg from the raid extreme to the far end of the
    #                    move that followed it. Local, and it moves with every
    #                    setup.
    #   "htf"          — the most recent confirmed swing on candles aggregated
    #                    `htf_multiplier` higher. This is what a ladder drawn
    #                    by hand on the 4h chart is anchored to, and it stays
    #                    put while price works inside it.
    leg_anchor: str = "displacement"
    htf_multiplier: int = 4  # 4 x 1h = 4h
    # The ratios drawn as a ladder across the leg. These are the successive
    # square roots of the golden ratio — 0.786 = sqrt(0.618), 0.882 =
    # sqrt(0.786), 0.941 = sqrt(0.882) — which is why the deep levels cluster.
    # Only `retracement` selects the entry; the rest are drawn for context.
    fib_levels: list[float] = field(
        default_factory=lambda: [0.236, 0.382, 0.5, 0.686, 0.786, 0.882, 0.941]
    )
    valid_bars: int = 20
    sl_anchor: str = "sweep_extreme"  # sweep_extreme | ob_extreme | fib
    # Used when sl_anchor = "fib": the rung the stop sits on. 1.0 is the far
    # end of the leg — the raid extreme itself under a local anchor, the swing
    # extreme under an htf one. Above 1.0 places it beyond that end.
    #
    # In this mode `sl_buffer_atr` is deliberately NOT applied. The point of a
    # stop on a rung is that entry, stop and targets are all measured with one
    # ruler; adding an ATR pad would reintroduce the second scale it exists to
    # remove. Put the clearance in `sl_fib` instead (e.g. 1.02).
    sl_fib: float = 1.0
    sl_buffer_atr: float = 0.25
    tp_mode: str = "rr"  # rr | liquidity | fib
    # Used when tp_mode = "fib": exits on lower rungs of the same ladder the
    # entry was taken from. Sniping a rung and then measuring the exit in R
    # mixes two rulers; a level entry wants level exits.
    tp_fib: list[float] = field(default_factory=lambda: [0.786, 0.5])
    tp_rr: list[float] = field(default_factory=lambda: [2.0])
    tp_weights: list[float] = field(default_factory=lambda: [1.0])
    breakeven_after_tp: int = 1  # 0 disables; N moves SL to entry after the Nth TP
    min_rr: float = 1.5


@dataclass(slots=True)
class FilterConfig:
    """Context filters — where in the range, and at what time, a setup counts.

    All off by default: each one cuts the trade count sharply, and a filter
    validated on the sample it was chosen on has demonstrated nothing.
    """

    premium_discount: bool = False  # shorts only in premium, longs only in discount
    range_swings: int = 5  # swings per side that define the dealing range
    pd_threshold: float = 0.5  # 0.5 = equilibrium; higher demands a deeper premium
    require_unmitigated: bool = False  # reject blocks price has already traded back into
    require_inducement: bool = False  # wait for the minor swing in front of the block to be run
    inducement_swing: int = 1  # fractal size for those minor swings
    session_enabled: bool = False
    session_windows: list = field(default_factory=lambda: ["07:00-10:00", "12:00-15:00"])
    session_days: list = field(default_factory=lambda: [0, 1, 2, 3, 4])  # Mon-Fri


@dataclass(slots=True)
class BiasConfig:
    mode: str = "off"  # off | ema | htf_structure
    ema_period: int = 200
    htf_multiplier: int = 4


@dataclass(slots=True)
class RiskConfig:
    starting_equity: float = 10_000.0
    risk_pct: float = 0.5
    max_concurrent: int = 1
    cooldown_bars: int = 3
    max_position_pct: float = 100.0
    allow_long: bool = True
    allow_short: bool = True


@dataclass(slots=True)
class CostConfig:
    maker_fee_bps: float = 2.0
    taker_fee_bps: float = 5.0
    slippage_bps: float = 1.0


@dataclass(slots=True)
class LiveConfig:
    enabled: bool = False
    mode: str = "paper"  # paper | live
    sandbox: bool = True
    poll_seconds: int = 15
    state_file: str = "state/agent.json"
    api_key_env: str = "LSOB_API_KEY"
    api_secret_env: str = "LSOB_API_SECRET"


@dataclass(slots=True)
class NotifyConfig:
    console: bool = True
    telegram_token_env: str = "LSOB_TG_TOKEN"
    telegram_chat_env: str = "LSOB_TG_CHAT"


@dataclass(slots=True)
class WalkForwardConfig:
    train_bars: int = 2000
    test_bars: int = 1000
    min_trades: int = 10
    metric: str = "expectancy_r"  # expectancy_r | total_r | profit_factor
    selection: str = "robust"  # robust (prefer a plateau) | peak (highest score)
    # Dotted paths into any other section, e.g.
    #   "orderblock.displacement_atr" = [0.5, 1.0, 1.5]
    grid: dict = field(default_factory=dict)


@dataclass(slots=True)
class Config:
    market: MarketConfig = field(default_factory=MarketConfig)
    data: DataConfig = field(default_factory=DataConfig)
    structure: StructureConfig = field(default_factory=StructureConfig)
    liquidity: LiquidityConfig = field(default_factory=LiquidityConfig)
    orderblock: OrderBlockConfig = field(default_factory=OrderBlockConfig)
    entry: EntryConfig = field(default_factory=EntryConfig)
    filters: FilterConfig = field(default_factory=FilterConfig)
    bias: BiasConfig = field(default_factory=BiasConfig)
    risk: RiskConfig = field(default_factory=RiskConfig)
    costs: CostConfig = field(default_factory=CostConfig)
    live: LiveConfig = field(default_factory=LiveConfig)
    notify: NotifyConfig = field(default_factory=NotifyConfig)
    walkforward: WalkForwardConfig = field(default_factory=WalkForwardConfig)


def _build(cls: type, raw: dict[str, Any], section: str) -> Any:
    """Construct one config section, rejecting unknown keys.

    TOML has no float literal for `1`, so a threshold written as `1` arrives
    as an int; each value is coerced to the type of the field's own default
    rather than trusted as parsed.
    """
    defaults = {f.name: f for f in fields(cls)}
    unknown = set(raw) - set(defaults)
    if unknown:
        raise ValueError(f"unknown key(s) in [{section}]: {', '.join(sorted(unknown))}")

    blank = cls()
    kwargs: dict[str, Any] = {}
    for name, value in raw.items():
        want = type(getattr(blank, name))
        if want is float and isinstance(value, int) and not isinstance(value, bool):
            value = float(value)
        elif want is list and isinstance(value, list):
            # Only widen to float where the field's own default is a float
            # list. Coercing every list would turn weekday numbers and window
            # strings into something their consumers do not expect.
            default_list = getattr(blank, name)
            if default_list and isinstance(default_list[0], float):
                value = [
                    float(v) if isinstance(v, int) and not isinstance(v, bool) else v
                    for v in value
                ]
        elif not isinstance(value, want):
            raise ValueError(
                f"[{section}] {name}: expected {want.__name__}, got {type(value).__name__}"
            )
        kwargs[name] = value
    return cls(**kwargs)


def load_config(path: str | Path) -> Config:
    raw = tomllib.loads(Path(path).read_text(encoding="utf-8"))
    sections = {f.name: f.default_factory for f in fields(Config)}  # type: ignore[misc]
    unknown = set(raw) - set(sections)
    if unknown:
        raise ValueError(f"unknown config section(s): {', '.join(sorted(unknown))}")
    kwargs = {
        name: _build(type(sections[name]()), value, name) for name, value in raw.items()
    }
    cfg = Config(**kwargs)
    validate(cfg)
    return cfg


def validate(cfg: Config) -> None:
    """Reject configurations that would silently misbehave rather than fail."""
    e = cfg.entry
    # Which target list has to match the weights depends on the mode. Holding
    # an unused list in sync is busywork, and worse, it makes the error for a
    # real mistake name a setting the run never reads.
    if e.tp_mode != "fib" and len(e.tp_rr) != len(e.tp_weights):
        raise ValueError("entry.tp_rr and entry.tp_weights must have the same length")
    if not e.tp_rr:
        raise ValueError("entry.tp_rr must list at least one target")
    if any(r <= 0 for r in e.tp_rr):
        raise ValueError("entry.tp_rr values must be positive")
    if abs(sum(e.tp_weights) - 1.0) > 1e-9:
        raise ValueError(f"entry.tp_weights must sum to 1.0 (got {sum(e.tp_weights)})")
    if list(e.tp_rr) != sorted(e.tp_rr):
        raise ValueError("entry.tp_rr must be listed in ascending order")
    if e.edge not in ("proximal", "mid", "distal", "retracement"):
        raise ValueError("entry.edge must be proximal, mid, distal or retracement")
    if e.leg_anchor not in ("displacement", "htf"):
        raise ValueError('entry.leg_anchor must be "displacement" or "htf"')
    if e.htf_multiplier < 1:
        raise ValueError("entry.htf_multiplier must be >= 1")
    if not 0.0 < e.retracement <= 1.0:
        raise ValueError("entry.retracement must be greater than 0 and at most 1.0")
    if any(not 0.0 <= level <= 1.0 for level in e.fib_levels):
        raise ValueError("entry.fib_levels must all be between 0.0 and 1.0")
    if list(e.fib_levels) != sorted(e.fib_levels):
        raise ValueError("entry.fib_levels must be listed in ascending order")
    if e.sl_anchor not in ("sweep_extreme", "ob_extreme", "fib"):
        raise ValueError("entry.sl_anchor must be sweep_extreme, ob_extreme or fib")
    if e.sl_anchor == "fib":
        if e.sl_fib <= 0:
            raise ValueError("entry.sl_fib must be positive")
        if e.edge != "retracement":
            raise ValueError('entry.sl_anchor = "fib" requires entry.edge = "retracement"')
        if e.sl_fib <= e.retracement:
            raise ValueError(
                f"entry.sl_fib ({e.sl_fib}) must sit beyond entry.retracement "
                f"({e.retracement}), or the stop is on the wrong side of the entry"
            )
    if e.tp_mode not in ("rr", "liquidity", "fib"):
        raise ValueError("entry.tp_mode must be rr, liquidity or fib")
    if e.tp_mode == "fib":
        if not e.tp_fib:
            raise ValueError('entry.tp_fib must list at least one rung when tp_mode = "fib"')
        if any(not 0.0 <= r <= 1.0 for r in e.tp_fib):
            raise ValueError("entry.tp_fib values must be between 0.0 and 1.0")
        if list(e.tp_fib) != sorted(e.tp_fib, reverse=True):
            raise ValueError("entry.tp_fib must be listed from nearest rung to furthest")
        if len(e.tp_fib) != len(e.tp_weights):
            raise ValueError("entry.tp_fib and entry.tp_weights must have the same length")
        if e.breakeven_after_tp > len(e.tp_fib):
            raise ValueError("entry.breakeven_after_tp must be 0..len(tp_fib)")
    if e.tp_mode != "fib" and not 0 <= e.breakeven_after_tp <= len(e.tp_rr):
        raise ValueError("entry.breakeven_after_tp must be 0..len(tp_rr)")

    if cfg.orderblock.zone_mode not in ("body", "full", "body_to_extreme"):
        raise ValueError("orderblock.zone_mode must be body, full or body_to_extreme")
    if cfg.bias.mode not in ("off", "ema", "htf_structure"):
        raise ValueError("bias.mode must be off, ema or htf_structure")
    if cfg.bias.htf_multiplier < 1:
        raise ValueError("bias.htf_multiplier must be >= 1")
    if cfg.risk.risk_pct <= 0:
        raise ValueError("risk.risk_pct must be positive")
    if cfg.risk.max_concurrent < 1:
        raise ValueError("risk.max_concurrent must be >= 1")
    if not (cfg.risk.allow_long or cfg.risk.allow_short):
        raise ValueError("risk: at least one of allow_long/allow_short must be true")
    if cfg.liquidity.max_penetration_atr <= cfg.liquidity.min_penetration_atr:
        raise ValueError(
            "liquidity.max_penetration_atr must exceed liquidity.min_penetration_atr"
        )
    if cfg.live.enabled and cfg.live.mode not in ("paper", "live"):
        raise ValueError("live.mode must be paper or live")

    if cfg.data.spike_ratio <= 1.0 or cfg.data.jump_ratio <= 1.0:
        raise ValueError("data.spike_ratio and data.jump_ratio must be greater than 1.0")

    f = cfg.filters
    if not 0.0 <= f.pd_threshold <= 1.0:
        raise ValueError("filters.pd_threshold must be between 0.0 and 1.0")
    if f.inducement_swing < 1:
        raise ValueError("filters.inducement_swing must be >= 1")
    if f.range_swings < 1:
        raise ValueError("filters.range_swings must be >= 1")
    if f.session_enabled:
        if not f.session_windows:
            raise ValueError("filters.session_windows must not be empty when sessions are on")
        if not f.session_days or any(d not in range(7) for d in f.session_days):
            raise ValueError("filters.session_days must be weekday numbers 0-6")
        from .filters import SessionFilter

        SessionFilter(True, list(f.session_windows), list(f.session_days))

    wf = cfg.walkforward
    if wf.metric not in ("expectancy_r", "total_r", "profit_factor"):
        raise ValueError("walkforward.metric must be expectancy_r, total_r or profit_factor")
    if wf.selection not in ("robust", "peak"):
        raise ValueError("walkforward.selection must be robust or peak")
    if wf.train_bars < 1 or wf.test_bars < 1:
        raise ValueError("walkforward.train_bars and test_bars must be positive")
    for key, values in wf.grid.items():
        if "." not in key:
            raise ValueError(f"walkforward.grid key {key!r} must be 'section.field'")
        if not isinstance(values, list) or not values:
            raise ValueError(f"walkforward.grid[{key!r}] must be a non-empty list")
