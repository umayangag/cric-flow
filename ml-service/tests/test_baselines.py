from pathlib import Path

from ml_service.baselines import (
    train_batting_from_csv,
    train_bowling_from_csv,
)


def _repo_root() -> Path:
    d = Path(__file__).resolve()
    for _ in range(8):
        if (d.parent.parent.parent / "go.work").exists():
            return d.parent.parent.parent
        d = d.parent
    return d


def test_train_batting_baseline(tmp_path):
    root = _repo_root()
    csv_path = root / "tests/fixtures/exporter/t20/batting_on.csv"
    out_path = tmp_path / "batting.joblib"
    res = train_batting_from_csv(str(csv_path), model_out_path=str(out_path))
    assert res.n_rows > 0 and res.n_features > 0
    assert out_path.exists()


def test_train_bowling_baseline(tmp_path):
    root = _repo_root()
    csv_path = root / "tests/fixtures/exporter/t20/bowling_on.csv"
    out_path = tmp_path / "bowling.joblib"
    res = train_bowling_from_csv(str(csv_path), model_out_path=str(out_path))
    assert res.n_rows > 0 and res.n_features > 0
    assert out_path.exists()
