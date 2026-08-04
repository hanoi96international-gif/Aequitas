from __future__ import annotations

from pathlib import Path

import pytest

from lsob.config import load_config

REPO_CONFIG = Path(__file__).resolve().parents[1] / "config.toml"


def write(tmp_path: Path, body: str) -> Path:
    path = tmp_path / "cfg.toml"
    path.write_text(body, encoding="utf-8")
    return path


def test_the_shipped_config_is_valid():
    cfg = load_config(REPO_CONFIG)
    assert cfg.market.symbol
    assert cfg.live.enabled is False, "the shipped config must never trade on its own"
    assert abs(sum(cfg.entry.tp_weights) - 1.0) < 1e-9


def test_defaults_apply_to_omitted_sections(tmp_path):
    cfg = load_config(write(tmp_path, '[market]\nsymbol = "ETH/USDT"\n'))
    assert cfg.market.symbol == "ETH/USDT"
    assert cfg.market.exchange == "binance"
    assert cfg.structure.atr_period == 14


def test_a_misspelled_key_is_an_error_not_a_shrug(tmp_path):
    body = "[orderblock]\ndisplacment_atr = 2.0\n"  # note the typo
    with pytest.raises(ValueError, match="unknown key"):
        load_config(write(tmp_path, body))


def test_an_unknown_section_is_rejected(tmp_path):
    with pytest.raises(ValueError, match="unknown config section"):
        load_config(write(tmp_path, "[stratgy]\nfoo = 1\n"))


def test_integer_literals_are_accepted_where_floats_are_expected(tmp_path):
    cfg = load_config(write(tmp_path, "[orderblock]\ndisplacement_atr = 2\n"))
    assert cfg.orderblock.displacement_atr == 2.0
    assert isinstance(cfg.orderblock.displacement_atr, float)


def test_a_wrongly_typed_value_is_reported_with_its_field(tmp_path):
    with pytest.raises(ValueError, match="displacement_atr"):
        load_config(write(tmp_path, '[orderblock]\ndisplacement_atr = "lots"\n'))


@pytest.mark.parametrize(
    "body, message",
    [
        ("[entry]\ntp_rr = [1.0, 2.0]\ntp_weights = [1.0]\n", "same length"),
        ("[entry]\ntp_rr = [1.0]\ntp_weights = [0.5]\n", "sum to 1.0"),
        ("[entry]\ntp_rr = [3.0, 1.0]\ntp_weights = [0.5, 0.5]\n", "ascending"),
        ('[entry]\nedge = "middle"\n', "proximal"),
        ('[entry]\ntp_mode = "hope"\n', "tp_mode"),
        ('[bias]\nmode = "vibes"\n', "bias.mode"),
        ("[risk]\nrisk_pct = 0.0\n", "risk_pct"),
        ("[risk]\nallow_long = false\nallow_short = false\n", "allow_long"),
        ("[liquidity]\nmin_penetration_atr = 2.0\nmax_penetration_atr = 1.0\n", "max_penetration"),
    ],
)
def test_contradictory_settings_are_rejected(tmp_path, body, message):
    with pytest.raises(ValueError, match=message):
        load_config(write(tmp_path, body))
