"""Finer candles for the questions a coarse bar cannot answer.

A bar says price reached 4,245 and 4,384 sometime in that hour. It does not
say in which order — and when the stop sits at one and a target at the
other, that order *is* the trade. The backtester assumes the stop came
first. Given a second series at a finer interval, it can look instead.

What this cannot decide, and a first attempt here wrongly assumed it could:
whether a stop on the *fill* bar preceded the fill. For a resting limit the
entry lies between the bar's open and the stop by construction, so price has
to cross the entry on its way to the stop. Those are genuine stops, and no
resolution changes them — verified on real data, where the entry sat between
open and stop on 42 of 42 such bars. The function below still reports the
order because a gap through the entry is the one case where it differs, but
it will answer "after the fill" almost always, and correctly so.

Where even the fine candle is ambiguous — both levels inside one minute —
it falls back to the pessimistic reading rather than picking. Resolution
improves the answer; it never invents one.
"""

from __future__ import annotations

from collections.abc import Callable

from .model import Candle


class IntrabarIndex:
    """Fine candles, grouped by the coarse bar that contains them."""

    __slots__ = ("_by_bucket", "_bucket_ms")

    def __init__(self, fine: list[Candle], bucket_ms: int) -> None:
        if bucket_ms <= 0:
            raise ValueError("bucket_ms must be positive")
        self._bucket_ms = bucket_ms
        self._by_bucket: dict[int, list[Candle]] = {}
        for candle in sorted(fine, key=lambda c: c.ts):
            bucket = candle.ts - (candle.ts % bucket_ms)
            self._by_bucket.setdefault(bucket, []).append(candle)

    def __len__(self) -> int:
        return len(self._by_bucket)

    def within(self, coarse_ts: int) -> list[Candle]:
        """The fine candles inside the coarse bar opening at `coarse_ts`."""
        bucket = coarse_ts - (coarse_ts % self._bucket_ms)
        return self._by_bucket.get(bucket, [])

    def covers(self, coarse_ts: int) -> bool:
        return bool(self.within(coarse_ts))


Test = Callable[[Candle], bool]


def first_of(fine: list[Candle], events: list[tuple[str, Test]], pessimistic: str) -> str | None:
    """Name whichever event happens first across `fine`, in order.

    Returns `pessimistic` when a single fine candle satisfies more than one
    event — the resolution ran out before the question was answered, and
    guessing at that point would be the same error one level down.
    Returns None when no event happened at all.
    """
    for candle in fine:
        hits = [name for name, test in events if test(candle)]
        if len(hits) == 1:
            return hits[0]
        if len(hits) > 1:
            return pessimistic
    return None


def touched_after_fill(
    fine: list[Candle], entry: float, level: float, direction: str
) -> bool | None:
    """Did price reach `level` *after* filling at `entry`, within this bar?

    None means the fine data cannot say — either it is missing, or the fill
    and the level fall inside the same fine candle. The caller keeps its
    pessimistic default in that case.
    """
    if not fine:
        return None

    if direction == "short":
        def filled(c: Candle) -> bool:
            return c.high >= entry

        def reached(c: Candle) -> bool:
            return c.high >= level
    else:
        def filled(c: Candle) -> bool:
            return c.low <= entry

        def reached(c: Candle) -> bool:
            return c.low <= level

    seen_fill = False
    for candle in fine:
        if not seen_fill:
            if filled(candle):
                seen_fill = True
                if reached(candle):
                    return None  # same minute — no better answer available
            continue
        if reached(candle):
            return True
    return False if seen_fill else None
