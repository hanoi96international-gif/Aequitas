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
    edge: str = "proximal"  # proximal | mid | distal
    valid_bars: int = 20
    sl_anchor: str = "sweep_extreme"  # sweep_extreme | ob_extreme
    sl_buffer_atr: float = 0.25
    tp_mode: str = "rr"  # rr | liquidity
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
    if len(e.tp_rr) != len(e.tp_weights):
        raise ValueError("entry.tp_rr and entry.tp_weights must have the same length")
    if not e.tp_rr:
        raise ValueError("entry.tp_rr must list at least one target")
    if any(r <= 0 for r in e.tp_rr):
        raise ValueError("entry.tp_rr values must be positive")
    if abs(sum(e.tp_weights) - 1.0) > 1e-9:
        raise ValueError(f"entry.tp_weights must sum to 1.0 (got {sum(e.tp_weights)})")
    if list(e.tp_rr) != sorted(e.tp_rr):
        raise ValueError("entry.tp_rr must be listed in ascending order")
    if e.edge not in ("proximal", "mid", "distal"):
        raise ValueError("entry.edge must be proximal, mid or distal")
    if e.sl_anchor not in ("sweep_extreme", "ob_extreme"):
        raise ValueError("entry.sl_anchor must be sweep_extreme or ob_extreme")
    if e.tp_mode not in ("rr", "liquidity"):
        raise ValueError("entry.tp_mode must be rr or liquidity")
    if not 0 <= e.breakeven_after_tp <= len(e.tp_rr):
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

    f = cfg.filters
    if not 0.0 <= f.pd_threshold <= 1.0:
        raise ValueError("filters.pd_threshold must be between 0.0 and 1.0")
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
