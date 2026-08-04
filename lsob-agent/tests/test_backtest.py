"""One full pass: detection, fill, partial exit, breakeven stop, final target."""

from __future__ import annotations

from conftest import candle
from test_strategy import SETUP, base_config

from lsob.backtest import run_backtest, scan_signals
from lsob.data import load_csv, save_csv

# The short setup completes on bar 33; price then retraces into the order
# block on bar 35, which is the bar that fills the resting limit order.
WITH_RETRACE = list(SETUP[:34]) + [
    candle(34, 99.0, 101.5, 98.5, 101.0),
    candle(35, 101.0, 104.0, 100.5, 103.0),  # trades back through the entry
    candle(36, 103.0, 103.2, 99.0, 99.5),
    candle(37, 99.5, 99.7, 97.5, 98.0),  # first target
    candle(38, 98.0, 98.2, 95.0, 95.5),
    candle(39, 95.5, 95.7, 92.0, 92.5),  # second target
    candle(40, 92.5, 93.0, 91.0, 91.5),
]


def test_a_setup_that_retraces_into_the_block_runs_to_its_final_target():
    cfg = base_config()
    result = run_backtest(cfg, WITH_RETRACE)

    assert len(result.signals) == 1
    assert len(result.trades) == 1

    trade = result.trades[0]
    assert trade.direction == "short"
    assert trade.exit_reason == "tp2"
    assert trade.entry_index == 35, "the fill lands on the bar that traded through the limit"
    assert trade.r_multiple > 1.0
    assert trade.pnl > 0
    assert result.stats.win_rate == 100.0
    assert result.stats.ending_equity > cfg.risk.starting_equity


def test_a_setup_that_never_retraces_expires_unfilled():
    result = run_backtest(base_config(), SETUP)
    assert len(result.signals) == 1
    assert result.trades == []
    assert result.stats.trades == 0
    assert result.stats.ending_equity == base_config().risk.starting_equity


def test_the_equity_curve_covers_every_bar():
    result = run_backtest(base_config(), WITH_RETRACE)
    assert len(result.equity_curve) == len(WITH_RETRACE)
    assert [bar for bar, _ in result.equity_curve] == list(range(len(WITH_RETRACE)))


def test_scan_finds_the_same_setups_as_the_backtest():
    cfg = base_config()
    scanned = scan_signals(cfg, WITH_RETRACE)
    traded = run_backtest(cfg, WITH_RETRACE).signals
    assert [s.index for s in scanned] == [s.index for s in traded]


def test_a_run_over_quiet_data_trades_nothing():
    quiet = [candle(i, 100.0, 100.5, 99.5, 100.0) for i in range(300)]
    result = run_backtest(base_config(), quiet)
    assert result.signals == []
    assert result.trades == []


def test_candles_survive_a_csv_round_trip(tmp_path):
    path = tmp_path / "candles.csv"
    save_csv(path, WITH_RETRACE)
    restored = load_csv(path)
    assert len(restored) == len(WITH_RETRACE)
    assert restored[0] == WITH_RETRACE[0]
    assert restored[-1] == WITH_RETRACE[-1]
    assert run_backtest(base_config(), restored).trades[0].pnl > 0


def test_out_of_order_rows_are_sorted_rather_than_traded_as_given(tmp_path):
    path = tmp_path / "shuffled.csv"
    shuffled = [WITH_RETRACE[5], WITH_RETRACE[0], WITH_RETRACE[3]]
    save_csv(path, shuffled)
    restored = load_csv(path)
    assert [c.ts for c in restored] == sorted(c.ts for c in shuffled)
