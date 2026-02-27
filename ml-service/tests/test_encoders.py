"""Unit tests for ml.encoders."""

from ml.encoders import (
    encode_econ,
    encode_how_out,
    encode_runs,
    encode_runs_conceded,
    encode_session,
    encode_viscosity,
)


def test_encode_session_day():
    assert encode_session("day") == 0


def test_encode_session_night():
    assert encode_session("night") == 1


def test_encode_session_other():
    assert encode_session("other") == 1


def test_encode_viscosity_excellent():
    assert encode_viscosity("Excellent") == 3


def test_encode_viscosity_good():
    assert encode_viscosity("Good") == 2


def test_encode_viscosity_average():
    assert encode_viscosity("Average") == 1


def test_encode_viscosity_unknown():
    assert encode_viscosity("Unknown") == 0


def test_encode_how_out_zero():
    assert encode_how_out(0) == 0


def test_encode_how_out_not_out():
    assert encode_how_out("not out") == 1


def test_encode_how_out_out():
    assert encode_how_out("caught") == 2


def test_encode_runs_tiers():
    assert encode_runs(0) == 0
    assert encode_runs(24) == 0
    assert encode_runs(25) == 1
    assert encode_runs(49) == 1
    assert encode_runs(50) == 2
    assert encode_runs(74) == 2
    assert encode_runs(75) == 3
    assert encode_runs(100) == 3


def test_encode_runs_conceded_tiers():
    assert encode_runs_conceded(0) == 0
    assert encode_runs_conceded(39) == 0
    assert encode_runs_conceded(40) == 1
    assert encode_runs_conceded(59) == 1
    assert encode_runs_conceded(60) == 2
    assert encode_runs_conceded(79) == 2
    assert encode_runs_conceded(80) == 3
    assert encode_runs_conceded(100) == 3


def test_encode_econ_tiers():
    assert encode_econ(0) == 0
    assert encode_econ(3.9) == 0
    assert encode_econ(4) == 1
    assert encode_econ(7.9) == 1
    assert encode_econ(8) == 2
    assert encode_econ(11.9) == 2
    assert encode_econ(12) == 3
    assert encode_econ(15) == 3
