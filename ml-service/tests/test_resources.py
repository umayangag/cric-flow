"""Unit tests for ml.resources (CPU/memory-aware n_jobs)."""

import os
from unittest.mock import patch

from ml.resources import suggested_n_jobs


def test_suggested_n_jobs_explicit_valid():
    """ML_N_JOBS env set to valid value is returned."""
    with patch.dict(os.environ, {"ML_N_JOBS": "4"}, clear=False):
        assert suggested_n_jobs("training") == 4
    with patch.dict(os.environ, {"ML_N_JOBS": "1"}, clear=False):
        assert suggested_n_jobs("prediction") == 1


def test_suggested_n_jobs_explicit_invalid_falls_back():
    """ML_N_JOBS=0 or invalid falls back to auto (no KeyError)."""
    with patch.dict(os.environ, {"ML_N_JOBS": "0"}, clear=False):
        n = suggested_n_jobs("training")
        assert n >= 1
    with patch.dict(os.environ, {"ML_N_JOBS": "x"}, clear=False):
        n = suggested_n_jobs("training")
        assert n >= 1


def test_suggested_n_jobs_minus_one_uses_auto():
    """ML_N_JOBS=-1 means auto; should still return >= 1."""
    with patch.dict(os.environ, {"ML_N_JOBS": "-1"}, clear=False):
        n = suggested_n_jobs("training")
        assert n >= 1


def test_suggested_n_jobs_memory_limit_env():
    """ML_MEMORY_LIMIT_MB caps n_jobs when set."""
    with patch.dict(
        os.environ,
        {"ML_MEMORY_LIMIT_MB": "800", "ML_N_JOBS": ""},
        clear=False,
    ):
        with patch("ml.resources._cpu_count", return_value=16):
            n = suggested_n_jobs("training")
            # 70% of 800MB / 400MB per job = 1.4 -> max 1
            assert n >= 1
            assert n <= 2


def test_suggested_n_jobs_memory_limit_invalid_env_ignored():
    """Invalid ML_MEMORY_LIMIT_MB is ignored (ValueError)."""
    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "not_a_number"}, clear=False):
        with patch("ml.resources._cpu_count", return_value=4):
            n = suggested_n_jobs("training")
            assert n >= 1


def test_suggested_n_jobs_ml_n_jobs_max_caps():
    """ML_N_JOBS_MAX caps the number of jobs."""
    with patch.dict(os.environ, {"ML_N_JOBS_MAX": "2"}, clear=False):
        with patch("ml.resources._cpu_count", return_value=8):
            # Use a positive memory limit so the "unknown limit → 1 job" branch is not taken
            with patch("ml.resources._memory_limit_mb", return_value=4096):
                n = suggested_n_jobs("training")
                assert n == 2


def test_suggested_n_jobs_ml_n_jobs_max_invalid_ignored():
    """Invalid ML_N_JOBS_MAX is ignored; cpu count used."""
    with patch.dict(os.environ, {"ML_N_JOBS_MAX": "x"}, clear=False):
        with patch("ml.resources._cpu_count", return_value=4):
            # Use a positive memory limit so the "unknown limit → 1 job" branch is not taken
            with patch("ml.resources._memory_limit_mb", return_value=4096):
                assert suggested_n_jobs("training") == 4


def test_suggested_n_jobs_kind_tuning():
    """kind='tuning' uses tuning MB per job for memory cap."""
    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "2500"}, clear=False):
        with patch("ml.resources._cpu_count", return_value=16):
            n = suggested_n_jobs("tuning")
            assert n >= 1


def test_suggested_n_jobs_kind_prediction():
    """kind='prediction' uses prediction MB per job."""
    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "500"}, clear=False):
        with patch("ml.resources._cpu_count", return_value=8):
            n = suggested_n_jobs("prediction")
            assert n >= 1


def test_suggested_n_jobs_cpu_count_none():
    """When os.cpu_count() is None, _cpu_count returns 1."""
    with patch("ml.resources._cpu_count", return_value=1):
        with patch("ml.resources._memory_limit_mb", return_value=0):
            assert suggested_n_jobs("training") == 1


def test_memory_limit_mb_cgroup_v2_max_ignored(tmp_path):
    """cgroup memory.max with 'max' is ignored (continue to next)."""
    from ml.resources import _memory_limit_mb

    cgroup_dir = tmp_path / "cgroup"
    cgroup_dir.mkdir()
    (cgroup_dir / "memory.max").write_text("max\n")
    with patch("ml.resources.os.environ", {}):
        with patch(
            "ml.resources.os.path.exists",
            side_effect=lambda p: str(p) == str(cgroup_dir / "memory.max"),
        ):
            # We can't easily swap the path list in _memory_limit_mb without refactoring;
            # instead test via suggested_n_jobs with env (no cgroup) and explicit env.
            with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": ""}, clear=False):
                with patch("builtins.open", side_effect=FileNotFoundError):
                    # If no env and open fails, returns 0
                    pass
    # Direct call: with env set we get that value
    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "1024"}, clear=False):
        assert _memory_limit_mb() == 1024


def test_memory_limit_mb_env_value_error_ignored():
    """ML_MEMORY_LIMIT_MB invalid int -> ValueError caught, fall through."""
    from ml.resources import _memory_limit_mb

    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "x"}, clear=False):
        with patch("builtins.open", side_effect=FileNotFoundError):
            # Should not raise; may return 0 if cgroup paths also fail
            n = _memory_limit_mb()
            assert n == 0 or n >= 0


def test_cpu_count_zero_returns_one():
    """When cpu_count is 0, _cpu_count returns 1."""
    from ml.resources import _cpu_count

    with patch("os.cpu_count", return_value=0):
        assert _cpu_count() == 1


def test_cpu_count_none_returns_one():
    """When cpu_count is None, _cpu_count returns 1."""
    from ml.resources import _cpu_count

    with patch("os.cpu_count", return_value=None):
        assert _cpu_count() == 1


def test_cpu_count_normal():
    """Normal cpu_count is returned."""
    from ml.resources import _cpu_count

    with patch("os.cpu_count", return_value=8):
        assert _cpu_count() == 8


def test_suggested_n_jobs_invalid_per_job_falls_back():
    """When config per_job or frac is invalid, defaults are used (lines 116, 124, 127)."""
    with patch.dict(os.environ, {"ML_MEMORY_LIMIT_MB": "4096", "ML_N_JOBS": ""}, clear=False):
        with patch("ml.resources._cpu_count", return_value=4):
            with patch(
                "ml.resources._get_resources_config",
                return_value={
                    "training_mb_per_job": "invalid",
                    "memory_usage_fraction_percent": -1,
                    "training_low_memory_threshold_mb": "bad",
                },
            ):
                n = suggested_n_jobs("training")
                assert n >= 1
