"""Guards against the failure mode that makes a backtest worthless.

If any rule can see a bar it has not been given yet, the results turn
plausible-looking and meaningless. Two independent checks here:

  * prefix stability — the signals produced for the first k bars must not
    change when more bars are appended
  * no edge on noise — a random walk has nothing to find, so anything much
    better than the cost of trading is a leak, not an edge
"""

from __future__ import annotations

import random

from lsob.backtest import run_backtest, scan_signals
from lsob.config import Config
from lsob.model import Candle

BASE_MS = 1_704_067_200_000
STEP_MS = 15 * 60 * 1000


def random_walk(n: int, seed: int, start: float = 50_000.0, vol: float = 0.0025) -> list[Candle]:
    """A driftless walk with a sampled intrabar path, so wicks are realistic."""
    rng = random.Random(seed)
    out: list[Candle] = []
    price = start
    for i in range(n):
        path = [price]
        for _ in range(6):
            path.append(path[-1] * (1 + rng.gauss(0, vol / 2.5)))
        close = path[-1]
        out.append(
            Candle(
                ts=BASE_MS + i * STEP_MS,
                open=price,
                high=max(path),
                low=min(path),
                close=close,
                volume=abs(rng.gauss(100, 20)),
            )
        )
        price = close
    return out


def config() -> Config:
    cfg = Config()
    cfg.entry.tp_rr = [1.5, 3.0]
    cfg.entry.tp_weights = [0.5, 0.5]
    return cfg


def test_signals_for_a_prefix_do_not_change_when_more_bars_arrive():
    candles = random_walk(4_000, seed=42)
    cfg = config()

    full = scan_signals(cfg, candles)
    assert len(full) > 5, "the fixture must actually produce setups to be meaningful"

    for cut in (1_000, 2_000, 3_000):
        prefix = scan_signals(cfg, candles[:cut])
        expected = [s for s in full if s.index < cut]
        assert [(s.index, s.direction, s.entry, s.stop) for s in prefix] == [
            (s.index, s.direction, s.entry, s.stop) for s in expected
        ], f"signals up to bar {cut} changed once later bars were appended"


def test_the_strategy_has_no_edge_on_a_random_walk():
    cfg = config()
    total_r = 0.0
    trades = 0
    for seed in range(4):
        result = run_backtest(cfg, random_walk(15_000, seed=seed))
        total_r += result.stats.total_r
        trades += result.stats.trades

    assert trades > 100, "too few trades for the result to mean anything"
    expectancy = total_r / trades
    # Noise plus costs should be mildly negative. A meaningfully positive
    # number here means a rule is reading the future.
    assert expectancy < 0.10, f"suspicious edge on noise: {expectancy:+.3f}R"
    assert expectancy > -1.0, f"implausibly bad: {expectancy:+.3f}R suggests a sizing bug"


def test_a_signal_never_references_a_bar_that_has_not_closed():
    candles = random_walk(3_000, seed=11)
    for sig in scan_signals(config(), candles):
        assert sig.order_block.index <= sig.index
        assert sig.ts == candles[sig.index].ts
