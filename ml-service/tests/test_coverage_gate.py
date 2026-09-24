"""The ml-service coverage gate must be able to fail, and its floor must be one number.

Background. pytest-cov does not decide the exit code from the "FAIL Required test coverage
of N% not reached" line it prints; that line is cosmetic and compares the raw total. The
exit code comes from coverage's ``should_fail_under(total, fail_under, precision)``, which
compares ``round(total, precision)`` against the floor. ``[tool.coverage.report] precision``
defaults to 0, so a real total of 94.94% rounded to 95 and cleared a floor of 95. CI run
35969830980 printed the FAIL line and exited 0, and no ml-service coverage floor had been
enforced for any total within half a point of it.

These tests pin the two things that made that possible: the rounding, and a floor written
in three files that could drift apart. They deliberately re-run pytest in a subprocess, so
the assertion is on the real exit code of the real installed pytest-cov rather than on our
reading of its source.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

ML_SERVICE_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ML_SERVICE_ROOT.parent

ML_SERVICE_MAKEFILE = ML_SERVICE_ROOT / "Makefile"
ROOT_MAKEFILE = REPO_ROOT / "Makefile"
CI_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ml-service-ci.yml"

# The lowest precision at which a floor of N still rejects a total of N - 0.25.
MINIMUM_USEFUL_PRECISION = 2

# Statement count of the generated fixture package. 400 divides by 4, so a floor with at
# most two decimals maps onto a whole number of covered statements.
FIXTURE_STATEMENTS = 400


def _read_configured_precision() -> int:
    """The precision the coverage gate actually runs at, from ml-service/pyproject.toml.

    Absent means coverage's own default of 0 -- the setting that disabled the gate.
    """
    match = re.search(r"^precision = ([0-9]+)", (ML_SERVICE_ROOT / "pyproject.toml").read_text(), re.MULTILINE)
    return int(match.group(1)) if match else 0


def _read_floor(path: Path, pattern: str) -> float:
    match = re.search(pattern, path.read_text(), re.MULTILINE)
    assert match is not None, f"no coverage floor matching {pattern!r} in {path}"
    return float(match.group(1))


def _write_fixture_package(directory: Path, covered: int, missed: int) -> None:
    """Write a module of `covered + missed` statements, of which `missed` are unreachable.

    The module-level assignments run on import; the body of the never-called function does
    not. That gives an exact, deterministic coverage percentage.
    """
    lines = [f"value_{index} = {index}" for index in range(covered - 1)]
    lines.append("def never_called():")
    lines.extend(f"    unreached_{index} = {index}" for index in range(missed))
    (directory / "gate_fixture.py").write_text("\n".join(lines) + "\n")
    (directory / "test_gate_fixture.py").write_text(
        "import gate_fixture\n\n\ndef test_imports():\n    assert gate_fixture.value_0 == 0\n"
    )


def _run_gate(directory: Path, floor: float, precision: int) -> subprocess.CompletedProcess[str]:
    """Run the real coverage gate over the fixture package and return the finished process."""
    # pytest-cov exports COV_CORE_* to make subprocesses join the parent's measurement.
    # This child must measure only its own fixture, so those are dropped.
    environment = {key: value for key, value in os.environ.items() if not key.startswith("COV_CORE")}
    environment.pop("PYTEST_ADDOPTS", None)
    return subprocess.run(
        [
            sys.executable,
            "-m",
            "pytest",
            "-q",
            "-p",
            "no:cacheprovider",
            "--cov=gate_fixture",
            "--cov-report=term",
            f"--cov-precision={precision}",
            f"--cov-fail-under={floor}",
            "test_gate_fixture.py",
        ],
        cwd=directory,
        env=environment,
        capture_output=True,
        text=True,
        check=False,
    )


def test_coverage_precision_is_high_enough_for_the_gate_to_fail() -> None:
    """A precision below two lets a total up to half a point under the floor pass."""
    # Arrange / Act
    precision = _read_configured_precision()

    # Assert
    assert precision >= MINIMUM_USEFUL_PRECISION


def test_gate_exits_non_zero_when_coverage_is_just_below_the_floor(tmp_path: Path) -> None:
    """The exact shape of CI run 35969830980: a total under the floor but within rounding."""
    # Arrange: a fixture package at exactly (floor - 0.25)%, which rounds up to the floor.
    floor = _read_floor(ML_SERVICE_MAKEFILE, r"^COV_MIN\?=([0-9.]+)")
    covered = round(FIXTURE_STATEMENTS * floor / 100) - 1
    missed = FIXTURE_STATEMENTS - covered
    _write_fixture_package(tmp_path, covered, missed)
    measured = 100 * covered / FIXTURE_STATEMENTS
    assert floor - 0.5 < measured < floor, "fixture must land inside the rounding window"

    # Act
    result = _run_gate(tmp_path, floor, _read_configured_precision())

    # Assert
    assert f"Total coverage: {measured:.2f}%" in result.stdout
    assert result.returncode != 0, f"the coverage gate passed at {measured:.2f}% against a floor of {floor}"


def test_gate_exits_zero_when_coverage_meets_the_floor(tmp_path: Path) -> None:
    """The gate is not simply always failing: a total on the floor still passes."""
    # Arrange
    floor = _read_floor(ML_SERVICE_MAKEFILE, r"^COV_MIN\?=([0-9.]+)")
    covered = round(FIXTURE_STATEMENTS * floor / 100)
    _write_fixture_package(tmp_path, covered, FIXTURE_STATEMENTS - covered)

    # Act
    result = _run_gate(tmp_path, floor, _read_configured_precision())

    # Assert
    assert result.returncode == 0, result.stdout


def test_the_three_declared_floors_are_the_same_number() -> None:
    """CLAUDE.md requires ml-service/Makefile, the root Makefile and CI to move together."""
    # Arrange / Act
    floors = {
        "ml-service/Makefile": _read_floor(ML_SERVICE_MAKEFILE, r"^COV_MIN\?=([0-9.]+)"),
        "Makefile": _read_floor(ROOT_MAKEFILE, r"^COV_MIN_ML \?= ([0-9.]+)"),
        ".github/workflows/ml-service-ci.yml": _read_floor(CI_WORKFLOW, r'^ *COV_MIN: "([0-9.]+)"'),
    }

    # Assert
    assert len(set(floors.values())) == 1, floors
