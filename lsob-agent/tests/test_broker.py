"""The live broker's day-trading rules, against a fake exchange.

`LiveBroker.__init__` deliberately refuses to build without ccxt, credentials
and three separate confirmations, which is the right behaviour and the wrong
thing to fight in a test. These build the object without running that
constructor and hand it a recorder in place of the exchange, so what is under
test is the decision and the order sequence — the only part that can be
checked without an account.
"""

from __future__ import annotations

from dataclasses import replace

from conftest import BASE_MS, candle

from lsob.broker import LiveBroker, LiveOrderSet
from lsob.config import Config
from lsob.daytrade import day_clock
from test_execution import short_signal

HOUR_MS = 60 * 60 * 1000


class FakeExchange:
    def __init__(self) -> None:
        self.calls: list[tuple] = []
        self.order_status = "open"
        self.filled_qty = 0.0

    def create_limit_order(self, symbol, side, qty, price, params=None):
        self.calls.append(("limit", side, qty, price))
        return {"id": f"o{len(self.calls)}"}

    def create_order(self, symbol, kind, side, qty, price=None, params=None):
        self.calls.append((kind, side, qty, (params or {}).get("reduceOnly")))
        return {"id": f"o{len(self.calls)}"}

    def cancel_order(self, order_id, symbol):
        self.calls.append(("cancel", order_id))

    def fetch_order(self, order_id, symbol):
        return {"status": self.order_status, "filled": self.filled_qty}

    def fetch_balance(self):
        return {"total": {"USDT": 10_000.0}}

    def amount_to_precision(self, symbol, qty):
        return round(qty, 6)


def broker(**rules) -> tuple[LiveBroker, FakeExchange]:
    cfg = Config()
    cfg.market.timeframe = "1h"
    cfg.daytrade.enabled = True
    for key, value in rules.items():
        setattr(cfg.daytrade, key, value)
    b = LiveBroker.__new__(LiveBroker)
    b.cfg = cfg
    b.exchange = FakeExchange()
    b.symbol = "BTC/USDT"
    b.working = []
    b.clock = day_clock(cfg)
    b.entries_by_day = {}
    return b, b.exchange


def hour(h: int) -> object:
    """A 1h bar opening at `h` o'clock on the fixtures' base day."""
    ts = BASE_MS + h * HOUR_MS
    c = candle(0, 100.0, 100.5, 99.5, 100.0)
    return type(c)(ts=ts, open=c.open, high=c.high, low=c.low, close=c.close, volume=c.volume)


def filled_set(index: int = 0, qty: float = 2.0) -> LiveOrderSet:
    return LiveOrderSet(
        entry_id="e1",
        signal=short_signal(),
        stop_id="s1",
        target_ids=["t1", "t2"],
        filled=True,
        filled_qty=qty,
        filled_index=index,
    )


def test_the_session_end_cancels_protection_then_closes_reduce_only():
    b, ex = broker(flat_at="22:00")
    b.working.append(filled_set())

    b.on_bar(5, hour(21))  # the bar that ends at 22:00

    assert ex.calls[:3] == [("cancel", "s1"), ("cancel", "t1"), ("cancel", "t2")]
    kind, side, qty, reduce_only = ex.calls[3]
    assert (kind, side, qty) == ("market", "buy", 2.0), "a short is closed by buying"
    assert reduce_only is True, "without this a stale size opens a position the other way"
    assert b.working == []


def test_a_position_survives_a_bar_before_the_cutoff():
    b, ex = broker(flat_at="22:00")
    b.working.append(filled_set())
    b.on_bar(5, hour(20))
    assert not any(c[0] == "market" for c in ex.calls)
    assert len(b.working) == 1


def test_the_time_stop_closes_a_live_position_too():
    b, ex = broker(max_bars_in_trade=3)
    b.working.append(filled_set(index=10))

    b.on_bar(12, hour(3))
    assert not any(c[0] == "market" for c in ex.calls), "two bars held"

    b.on_bar(13, hour(4))
    assert any(c[0] == "market" for c in ex.calls)
    assert b.working == []


def test_a_failed_flatten_keeps_the_order_set_so_the_next_bar_retries():
    b, ex = broker(flat_at="22:00")

    def refuse(*args, **kwargs):
        raise RuntimeError("venue down")

    ex.create_order = refuse
    b.working.append(filled_set())
    b.on_bar(5, hour(21))
    assert len(b.working) == 1, "a position that is still open must not be forgotten"


def test_an_unfilled_order_is_pulled_at_the_cutoff_when_configured():
    b, ex = broker(flat_at="22:00", cancel_orders_at_cutoff=True)
    b.working.append(LiveOrderSet(entry_id="e9", signal=short_signal()))
    b.on_bar(5, hour(21))
    assert ("cancel", "e9") in ex.calls
    assert b.working == []


def test_an_unfilled_order_may_be_left_working_instead():
    b, ex = broker(flat_at="22:00", cancel_orders_at_cutoff=False)
    b.working.append(LiveOrderSet(entry_id="e9", signal=short_signal()))
    b.on_bar(5, hour(21))
    assert ("cancel", "e9") not in ex.calls
    assert len(b.working) == 1, "no position overnight is not the same as no order"


def test_no_new_signal_is_accepted_past_the_entry_cutoff():
    b, _ = broker(flat_at="22:00", no_entry_after="20:00")
    late = replace(short_signal(), ts=BASE_MS + 20 * HOUR_MS)
    assert not b.on_signal(0, late)
    assert b.working == []

    early = replace(short_signal(), ts=BASE_MS + 19 * HOUR_MS)
    assert b.on_signal(0, early)


def test_the_daily_cap_counts_fills_not_orders():
    b, ex = broker(max_trades_per_day=1)
    sig = replace(short_signal(), ts=BASE_MS + 2 * HOUR_MS)
    assert b.on_signal(0, sig)

    ex.order_status = "closed"
    ex.filled_qty = 1.5
    b.on_bar(1, hour(3))
    assert b.entries_by_day[b.clock.day_key(BASE_MS)] == 1

    again = replace(short_signal(), ts=BASE_MS + 4 * HOUR_MS)
    assert not b.on_signal(2, again), "the day's allowance is spent"


def test_the_rules_stay_out_of_the_way_while_daytrade_is_off():
    cfg = Config()
    cfg.daytrade.enabled = False
    cfg.daytrade.flat_at = "22:00"
    b = LiveBroker.__new__(LiveBroker)
    b.cfg = cfg
    b.exchange = FakeExchange()
    b.symbol = "BTC/USDT"
    b.working = [filled_set()]
    b.clock = day_clock(cfg)
    b.entries_by_day = {}

    b.on_bar(5, hour(23))
    assert len(b.working) == 1
    assert not any(c[0] == "market" for c in b.exchange.calls)
