import os
import pathlib

import numpy as np

from ml.datasets import (
    BATTING_SEQ_COLUMNS,
    BOWLING_SEQ_COLUMNS,
    build_feature_matrix,
    load_batting_dataframe,
    load_bowling_dataframe,
)


def find_repo_root(start: str) -> str:
    d = pathlib.Path(start).resolve()
    for _ in range(8):
        if (d / "go.work").exists():
            return str(d)
        if d.parent == d:
            break
        d = d.parent
    return str(pathlib.Path(start).resolve())


def test_reader_handles_off_and_on_batting():
    wd = os.getcwd()
    root = find_repo_root(wd)
    off_path = os.path.join(root, "tests", "fixtures", "exporter", "t20", "batting_off.csv")
    on_path = os.path.join(root, "tests", "fixtures", "exporter", "t20", "batting_on.csv")

    # OFF
    df_off = load_batting_dataframe(off_path)
    assert set(["runs", "balls", "player_name"]).issubset(df_off.columns)
    # Build feature matrix requiring seq columns (which are missing) -> should backfill zeros
    X_off, names_off = build_feature_matrix(df_off, BATTING_SEQ_COLUMNS)
    assert X_off.shape[0] == len(df_off)
    assert X_off.shape[1] == len(BATTING_SEQ_COLUMNS)
    assert np.allclose(X_off.sum(), 0.0)  # all zeros backfilled

    # ON
    df_on = load_batting_dataframe(on_path)
    # Ensure seq columns are present
    for c in BATTING_SEQ_COLUMNS:
        assert c in df_on.columns
    X_on, names_on = build_feature_matrix(df_on, BATTING_SEQ_COLUMNS)
    assert X_on.shape == (len(df_on), len(BATTING_SEQ_COLUMNS))
    # Not all zeros now (since fixture has non-zero values)
    assert np.sum(X_on) != 0.0
    assert names_on == BATTING_SEQ_COLUMNS


def test_reader_handles_off_and_on_bowling():
    wd = os.getcwd()
    root = find_repo_root(wd)
    off_path = os.path.join(root, "tests", "fixtures", "exporter", "t20", "bowling_off.csv")
    on_path = os.path.join(root, "tests", "fixtures", "exporter", "t20", "bowling_on.csv")

    # OFF
    df_off = load_bowling_dataframe(off_path)
    assert set(["overs", "balls", "player_name"]).issubset(df_off.columns)
    X_off, names_off = build_feature_matrix(df_off, BOWLING_SEQ_COLUMNS)
    assert X_off.shape[0] == len(df_off)
    assert X_off.shape[1] == len(BOWLING_SEQ_COLUMNS)
    assert np.allclose(X_off.sum(), 0.0)

    # ON
    df_on = load_bowling_dataframe(on_path)
    for c in BOWLING_SEQ_COLUMNS:
        assert c in df_on.columns
    X_on, names_on = build_feature_matrix(df_on, BOWLING_SEQ_COLUMNS)
    assert X_on.shape == (len(df_on), len(BOWLING_SEQ_COLUMNS))
    assert np.sum(X_on) != 0.0
    assert names_on == BOWLING_SEQ_COLUMNS
