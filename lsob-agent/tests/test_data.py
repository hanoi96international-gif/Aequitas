from __future__ import annotations

import pytest

from lsob.data import (
    archive_url,
    audit_candles,
    clean_candles,
    load_any,
    load_csv,
    load_klines,
    months_between,
)
from lsob.model import Candle


def write(tmp_path, body: str):
    path = tmp_path / "in.csv"
    path.write_text(body, encoding="utf-8")
    return path


def test_milliseconds_seconds_and_iso_all_land_on_the_same_bar(tmp_path):
    variants = [
        "timestamp,open,high,low,close,volume\n1704067200000,100,101,99,100.5,5\n",
        "time,open,high,low,close,volume\n1704067200,100,101,99,100.5,5\n",
        "date,open,high,low,close,volume\n2024-01-01T00:00:00Z,100,101,99,100.5,5\n",
    ]
    parsed = [load_csv(write(tmp_path, body))[0] for body in variants]
    assert {c.ts for c in parsed} == {1_704_067_200_000}
    assert all(c.close == 100.5 for c in parsed)


def test_naive_iso_timestamps_are_read_as_utc(tmp_path):
    path = write(tmp_path, "date,open,high,low,close\n2024-01-01 00:00:00,100,101,99,100\n")
    assert load_csv(path)[0].ts == 1_704_067_200_000


def test_volume_is_optional(tmp_path):
    path = write(tmp_path, "timestamp,open,high,low,close\n1704067200000,100,101,99,100\n")
    assert load_csv(path)[0].volume == 0.0


def test_a_missing_price_column_is_named_in_the_error(tmp_path):
    path = write(tmp_path, "timestamp,open,high,close\n1704067200000,100,101,100\n")
    with pytest.raises(ValueError, match="'low'"):
        load_csv(path)


def test_an_unnamed_leading_index_column_is_treated_as_the_timestamp(tmp_path):
    """What `pandas.DataFrame.to_csv` produces for a time-indexed frame."""
    path = write(tmp_path, ",Open,High,Low,Close,Volume\n2024-01-01 00:00:00,100,101,99,100.5,5\n")
    candle = load_csv(path)[0]
    assert candle.ts == 1_704_067_200_000
    assert candle.close == 100.5


def test_a_missing_timestamp_column_is_rejected(tmp_path):
    path = write(tmp_path, "open,high,low,close\n100,101,99,100\n")
    with pytest.raises(ValueError, match="no timestamp column"):
        load_csv(path)


def test_a_corrupt_row_reports_its_line_number(tmp_path):
    body = (
        "timestamp,open,high,low,close\n"
        "1704067200000,100,101,99,100\n"
        "1704068100000,100,oops,99,100\n"
    )
    with pytest.raises(ValueError, match=":3:"):
        load_csv(write(tmp_path, body))


def test_headers_are_matched_case_insensitively(tmp_path):
    path = write(tmp_path, "Open_Time,Open,High,Low,Close,Volume\n1704067200000,1,2,0.5,1.5,9\n")
    candle = load_csv(path)[0]
    assert candle.high == 2.0 and candle.volume == 9.0


# ── Binance public archive ───────────────────────────────────────────────


def test_the_headerless_binance_archive_layout_is_read(tmp_path):
    """Twelve columns, no header — the format the monthly dumps ship in."""
    path = tmp_path / "BTCUSDT-15m-2024-01.csv"
    path.write_text(
        "1704067200000,42000.1,42100.5,41900.0,42050.2,123.45,"
        "1704068099999,5187000.0,1500,60.1,2500000.0,0\n"
        "1704068100000,42050.2,42200.0,42000.0,42150.0,98.76,"
        "1704068999999,4160000.0,1200,50.0,2100000.0,0\n",
        encoding="utf-8",
    )
    candles = load_klines(path)
    assert len(candles) == 2
    assert candles[0].ts == 1_704_067_200_000
    assert candles[0].open == 42000.1
    assert candles[0].volume == 123.45
    assert candles[1].close == 42150.0


def test_a_header_row_on_a_newer_dump_is_skipped(tmp_path):
    path = tmp_path / "k.csv"
    path.write_text(
        "open_time,open,high,low,close,volume,close_time,qav,trades,tbb,tbq,ignore\n"
        "1704067200000,42000,42100,41900,42050,123,1,2,3,4,5,0\n",
        encoding="utf-8",
    )
    assert len(load_klines(path)) == 1


def test_microsecond_timestamps_are_recognised(tmp_path):
    """Binance switched the archive from milliseconds to microseconds."""
    path = tmp_path / "k.csv"
    path.write_text("1704067200000000,42000,42100,41900,42050,123,1,2,3,4,5,0\n", encoding="utf-8")
    assert load_klines(path)[0].ts == 1_704_067_200_000


def test_a_zipped_archive_is_read_without_unpacking(tmp_path):
    import zipfile

    inner = "BTCUSDT-15m-2024-01.csv"
    archive = tmp_path / "BTCUSDT-15m-2024-01.zip"
    with zipfile.ZipFile(archive, "w") as zf:
        zf.writestr(inner, "1704067200000,42000,42100,41900,42050,123,1,2,3,4,5,0\n")
    candles = load_klines(archive)
    assert len(candles) == 1 and candles[0].close == 42050.0


def test_load_any_accepts_both_layouts(tmp_path):
    labelled = tmp_path / "a.csv"
    labelled.write_text("timestamp,open,high,low,close\n1704067200000,1,2,0.5,1.5\n")
    headerless = tmp_path / "b.csv"
    headerless.write_text("1704067200000,1,2,0.5,1.5,9,1,2,3,4,5,0\n")
    assert load_any(labelled)[0].close == 1.5
    assert load_any(headerless)[0].close == 1.5


def test_a_short_archive_row_is_rejected(tmp_path):
    path = tmp_path / "k.csv"
    path.write_text("1704067200000,42000,42100\n", encoding="utf-8")
    with pytest.raises(ValueError, match="at least 6 columns"):
        load_klines(path)


def test_the_archive_url_matches_binances_layout():
    assert archive_url("BTC/USDT", "15m", "2024-01") == (
        "https://data.binance.vision/data/spot/monthly/klines/"
        "BTCUSDT/15m/BTCUSDT-15m-2024-01.zip"
    )


def test_month_ranges_expand_across_year_boundaries():
    assert months_between("2024-11", "2025-02") == ["2024-11", "2024-12", "2025-01", "2025-02"]
    assert months_between("2024-03", "2024-03") == ["2024-03"]


@pytest.mark.parametrize(
    "start, end", [("2024-13", "2024-14"), ("2024-05", "2024-01"), ("nope", "2024-01")]
)
def test_bad_month_ranges_are_rejected(start, end):
    with pytest.raises(ValueError):
        months_between(start, end)


# ── integrity of real-world series ───────────────────────────────────────


def bar(ts_index: int, price: float, span: float = 0.01) -> Candle:
    return Candle(
        ts=1_704_067_200_000 + ts_index * 3_600_000,
        open=price,
        high=price * (1 + span),
        low=price * (1 - span),
        close=price,
        volume=1.0,
    )


def test_a_sentinel_filled_gap_is_caught_even_though_the_bar_looks_consistent():
    """The real defect: 1.7e308 in every column, so high/low is exactly 1.00."""
    sentinel = 1.7e308
    series = [
        bar(0, 5.65),
        Candle(ts=1_704_070_800_000, open=sentinel, high=sentinel, low=sentinel, close=sentinel),
        bar(2, 5.70),
    ]
    audit = audit_candles(series)
    assert audit.non_finite == 1
    assert audit.clean is False
    assert len(clean_candles(series)) == 2


@pytest.mark.parametrize("value", [float("inf"), float("nan")])
def test_infinities_and_nans_are_rejected(value):
    series = [bar(0, 100.0), Candle(ts=1_704_070_800_000, open=value, high=value, low=value, close=value)]
    assert audit_candles(series).non_finite == 1
    assert len(clean_candles(series)) == 1


def test_a_bar_disconnected_from_the_previous_close_is_rejected():
    series = [bar(0, 100.0), bar(1, 5000.0), bar(2, 101.0)]
    audit = audit_candles(series, jump_ratio=10.0)
    assert audit.jumps == 1
    kept = clean_candles(series, jump_ratio=10.0)
    assert [round(c.close) for c in kept] == [100, 101], "the good bars either side survive"


def test_a_long_term_trend_is_not_mistaken_for_corruption():
    """BTC went from ~5 to ~4000 in this dataset; gradual moves must survive."""
    series = [bar(i, 5.0 * (1.001**i)) for i in range(3000)]
    assert series[-1].close / series[0].close > 15
    audit = audit_candles(series)
    assert audit.jumps == 0
    assert audit.clean is True
    assert len(clean_candles(series)) == len(series)


def test_an_intra_bar_spike_is_still_caught():
    spike = Candle(ts=1_704_070_800_000, open=592.63, high=593.0, low=1.5, close=581.13)
    series = [bar(0, 590.0), spike, bar(2, 585.0)]
    assert audit_candles(series).spikes == 1
    assert len(clean_candles(series)) == 2


def test_duplicate_timestamps_collapse_to_one_bar():
    duplicate = Candle(ts=bar(0, 100.0).ts, open=100.0, high=101.0, low=99.0, close=100.5)
    series = [bar(0, 100.0), duplicate, bar(1, 100.2)]
    assert audit_candles(series).duplicate_ts == 1
    kept = clean_candles(series)
    assert len(kept) == 2
    assert kept[0].close == 100.5, "the later duplicate wins"


def test_missing_bars_are_counted_but_never_invented():
    series = [bar(0, 100.0), bar(1, 100.1), bar(5, 100.2)]
    audit = audit_candles(series)
    assert audit.gaps == 3
    assert len(clean_candles(series)) == 3, "gaps are reported, not filled"


def test_a_clean_series_says_so():
    series = [bar(i, 100.0 + i * 0.1) for i in range(50)]
    assert audit_candles(series).clean is True
    assert "no integrity problems" in audit_candles(series).format()


def test_the_audit_names_every_defect_it_found():
    sentinel = 1.7e308
    series = [
        bar(0, 100.0),
        Candle(ts=1_704_070_800_000, open=sentinel, high=sentinel, low=sentinel, close=sentinel),
        Candle(ts=1_704_074_400_000, open=100.0, high=101.0, low=1.0, close=100.0),
    ]
    text = audit_candles(series).format()
    assert "sentinel" in text and "spike" in text
