"""The track record's arithmetic is the harness's (P2-4).

go-app scores the record in Go -- Brier, the ten-bin reliability curve, inclusive 10-90
coverage -- and asserts its results against ``tests/fixtures/track_record_reliability.json``.
That file was generated from ``ml.xi.sim_harness.reliability`` and ``_brier``; this test
holds the harness to still producing it, so the two implementations are pinned to one
reference and cannot drift apart unnoticed.
"""

from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pytest

from ml.xi import sim_harness

FIXTURE = Path(__file__).resolve().parent / "fixtures" / "track_record_reliability.json"


@pytest.fixture(scope="module")
def pinned() -> dict:
    with open(FIXTURE) as fh:
        return json.load(fh)


def test_the_harness_reproduces_the_pinned_reliability_curve(pinned: dict) -> None:
    """Bin by bin, including the 0.3 that falls just below the third edge."""
    p, y = np.asarray(pinned["p"]), np.asarray(pinned["y"])

    curve = sim_harness.reliability(p, y, pinned["bins"])

    assert pinned["bins"] == sim_harness.RELIABILITY_BINS
    assert len(curve) == len(pinned["reliability"])
    for got, want in zip(curve, pinned["reliability"]):
        assert got["n"] == want["n"]
        assert got["lo"] == pytest.approx(want["lo"]) and got["hi"] == pytest.approx(want["hi"])
        assert got["predicted"] == pytest.approx(want["predicted"])
        assert got["observed"] == pytest.approx(want["observed"])
    trap = [bin_ for bin_ in curve if bin_["n"] == 2 and bin_["lo"] == pytest.approx(0.2)]
    assert trap and trap[0]["predicted"] == pytest.approx(0.3), "0.3 belongs to [0.2, 0.3) in numpy's digitize"


def test_the_harness_reproduces_the_pinned_brier_and_base_rate(pinned: dict) -> None:
    p, y = np.asarray(pinned["p"]), np.asarray(pinned["y"])

    assert sim_harness._brier(p, y) == pytest.approx(pinned["brier"])
    assert float(y.mean()) == pytest.approx(pinned["base_rate"])
    assert sim_harness._brier(np.full(len(y), y.mean()), y) == pytest.approx(pinned["base_rate_brier"])
