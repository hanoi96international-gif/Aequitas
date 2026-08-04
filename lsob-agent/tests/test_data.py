from __future__ import annotations

import pytest

from lsob.data import load_csv


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
