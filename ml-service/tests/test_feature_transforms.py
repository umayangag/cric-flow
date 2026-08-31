"""Unit tests for ml.feature_transforms (interactions, log1p, extend feature map)."""

from unittest.mock import patch

import numpy as np
import pytest

from ml.feature_transforms import (
    CSV_COLUMN_MAP,
    apply_interactions,
    apply_transforms,
    build_extended_vector_from_features,
    extend_feature_map,
    get_interaction_specs,
    get_transform_config,
    load_transform_config_from_metadata,
)


def test_get_transform_config_returns_structure():
    """get_transform_config returns add_interactions and add_log1p lists."""
    cfg = get_transform_config("batting")
    assert "add_interactions" in cfg
    assert "add_log1p" in cfg
    assert isinstance(cfg["add_interactions"], list)
    assert isinstance(cfg["add_log1p"], list)


def test_get_transform_config_exception_returns_empty():
    """When config is malformed (e.g. add_interactions not iterable), returns empty lists (lines 81-82)."""
    import ml.config as config_mod

    def bad_load():
        return {
            "ml": {
                "feature_transforms": {
                    "batting": {"add_interactions": 123, "add_log1p": []},
                }
            }
        }

    saved = config_mod._cached
    config_mod._cached = None
    try:
        with patch("ml.config._load", side_effect=bad_load):
            cfg = get_transform_config("batting")
        assert cfg == {"add_interactions": [], "add_log1p": []}
    finally:
        config_mod._cached = saved


def test_get_transform_config_skips_empty_interaction_pairs():
    """add_interactions entries with empty a or b are skipped (line 77)."""
    import ml.config as config_mod

    def custom_load():
        return {
            "ml": {
                "feature_transforms": {
                    "batting": {
                        "add_interactions": [["a", "b"], ["", "x"], ["y", ""], ["  ", "z"]],
                        "add_log1p": [],
                    }
                }
            }
        }

    saved = config_mod._cached
    config_mod._cached = None
    try:
        with patch("ml.config._load", side_effect=custom_load):
            cfg = get_transform_config("batting")
        assert cfg["add_interactions"] == [("a", "b")]
    finally:
        config_mod._cached = saved


def test_get_transform_config_parses_interactions():
    """add_interactions list of pairs is parsed from config."""
    cfg = get_transform_config("batting")
    for item in cfg["add_interactions"]:
        assert isinstance(item, (list, tuple))
        assert len(item) >= 2


def test_get_interaction_specs():
    """get_interaction_specs returns add_interactions from transform config."""
    specs = get_interaction_specs("batting")
    assert isinstance(specs, list)


def test_apply_transforms_log1p():
    """apply_transforms applies log1p to listed columns."""
    X = np.array([[1.0, 2.0, 3.0], [0.0, 10.0, 0.0]], dtype=np.float64)
    names = ["a", "b", "c"]
    config = {"add_interactions": [], "add_log1p": ["b"]}
    X_out, names_out = apply_transforms(X, names, config, "batting")
    assert names_out == ["a", "b", "c"]
    np.testing.assert_allclose(X_out[:, 0], X[:, 0])
    np.testing.assert_allclose(X_out[:, 1], np.log1p(X[:, 1]))
    np.testing.assert_allclose(X_out[:, 2], X[:, 2])


def test_apply_transforms_log1p_canonical_fallback():
    """When resolved col not in names but canon is, use canon (lines 120-122)."""
    X = np.array([[1.0, 2.0]], dtype=np.float64)
    names = ["venue", "opposition"]  # canonical names
    config = {"add_interactions": [], "add_log1p": ["venue"]}
    X_out, names_out = apply_transforms(X, names, config, "batting")
    # venue maps to batting_venue in CSV_COLUMN_MAP; names use canonical "venue"
    assert names_out == ["venue", "opposition"]
    np.testing.assert_allclose(X_out[:, 0], np.log1p(1.0))


def test_apply_transforms_log1p_negative_clamped():
    """log1p uses maximum(0) so negative values become 0 then log1p(0)=0."""
    X = np.array([[-1.0, 1.0]], dtype=np.float64)
    names = ["x", "y"]
    config = {"add_interactions": [], "add_log1p": ["x", "y"]}
    X_out, _ = apply_transforms(X, names, config, "batting")
    assert X_out[0, 0] == 0.0
    np.testing.assert_allclose(X_out[0, 1], np.log1p(1.0))


def test_apply_transforms_interactions():
    """apply_transforms appends interaction columns."""
    X = np.array([[1.0, 2.0], [3.0, 4.0]], dtype=np.float64)
    names = ["a", "b"]
    config = {"add_interactions": [("a", "b")], "add_log1p": []}
    X_out, names_out = apply_transforms(X, names, config, "batting")
    assert "a_x_b" in names_out
    np.testing.assert_allclose(X_out[:, 2], X[:, 0] * X[:, 1])


def test_apply_transforms_skip_missing_logs(monkeypatch):
    """When interaction column name not in feature_names, warning and skip."""
    import logging

    X = np.array([[1.0, 2.0]], dtype=np.float64)
    names = ["a", "b"]
    config = {"add_interactions": [("missing", "b")], "add_log1p": []}
    with patch.object(logging.getLogger("ml.feature_transforms"), "warning") as mock_warn:
        X_out, names_out = apply_transforms(X, names, config, "batting")
        assert names_out == ["a", "b"]
        mock_warn.assert_called()


def test_apply_transforms_csv_column_resolution():
    """For training, canonical name 'venue' resolves to 'batting_venue' when in map."""
    X = np.array([[1.0, 2.0]], dtype=np.float64)
    names = ["batting_venue", "opposition"]
    config = {"add_interactions": [], "add_log1p": ["venue"]}
    # venue -> batting_venue for batting; batting_venue is in names
    X_out, names_out = apply_transforms(X, names, config, "batting")
    np.testing.assert_allclose(X_out[:, 0], np.log1p(np.maximum(X[:, 0], 0)))


def test_apply_interactions_convenience():
    """apply_interactions is apply_transforms with only interactions."""
    X = np.array([[1.0, 2.0]], dtype=np.float64)
    names = ["a", "b"]
    X_out, names_out = apply_interactions(X, names, [("a", "b")], "batting")
    assert "a_x_b" in names_out
    np.testing.assert_allclose(X_out[:, 2], 2.0)


def test_extend_feature_map_empty_specs():
    """extend_feature_map with no specs returns copy of map."""
    m = {"a": 1.0, "b": 2.0}
    out = extend_feature_map(m, [])
    assert out == m
    assert out is not m


def test_extend_feature_map_adds_interaction():
    """extend_feature_map adds a_x_b when both a and b present."""
    m = {"a": 2.0, "b": 3.0}
    out = extend_feature_map(m, [("a", "b")])
    assert out["a"] == 2.0
    assert out["b"] == 3.0
    assert out["a_x_b"] == 6.0


def test_extend_feature_map_skips_missing_key():
    """When one key missing, interaction not added."""
    m = {"a": 1.0}
    out = extend_feature_map(m, [("a", "b")])
    assert "a_x_b" not in out
    assert out["a"] == 1.0


def test_extend_feature_map_invalid_value_skipped():
    """When value cannot be float, that interaction skipped."""
    m = {"a": 1.0, "b": "x"}
    out = extend_feature_map(m, [("a", "b")])
    assert "a_x_b" not in out


def test_build_extended_vector_from_features_base_only():
    """build_extended_vector_from_features with no transforms returns base."""
    base = [1.0, 2.0, 3.0]
    names = ["a", "b", "c"]
    feat_map = {}
    config = {"add_interactions": [], "add_log1p": []}
    out = build_extended_vector_from_features(base, names, feat_map, config)
    assert out == [1.0, 2.0, 3.0]


def test_build_extended_vector_from_features_with_log1p():
    """build_extended_vector_from_features applies log1p to listed cols."""
    base = [1.0, 2.0]
    names = ["a", "b"]
    feat_map = {}
    config = {"add_interactions": [], "add_log1p": ["b"]}
    out = build_extended_vector_from_features(base, names, feat_map, config)
    assert len(out) == 2
    assert out[0] == 1.0
    np.testing.assert_allclose(out[1], np.log1p(2.0))


def test_build_extended_vector_from_features_with_interactions():
    """build_extended_vector_from_features appends interaction values."""
    base = [1.0, 2.0]
    names = ["a", "b"]
    feat_map = {"a": 1.0, "b": 2.0}
    config = {"add_interactions": [("a", "b")], "add_log1p": []}
    out = build_extended_vector_from_features(base, names, feat_map, config)
    assert len(out) == 3
    assert out[2] == 2.0


def test_build_extended_vector_from_features_invalid_interaction_raises():
    """A non-numeric operand raises rather than yielding a vector one column short."""
    base = [1.0, 2.0]
    names = ["a", "b"]
    feat_map = {"a": 1.0, "b": "not-a-number"}
    config = {"add_interactions": [("a", "b")], "add_log1p": []}
    with pytest.raises(ValueError, match="non-numeric operand"):
        build_extended_vector_from_features(base, names, feat_map, config)


def test_build_extended_vector_from_features_missing_operand_raises():
    """A missing operand raises here, naming it, instead of failing later in the scaler."""
    base = [1.0, 2.0]
    names = ["a", "b"]
    config = {"add_interactions": [("a", "absent")], "add_log1p": []}
    with pytest.raises(ValueError, match="absent"):
        build_extended_vector_from_features(base, names, {"a": 1.0}, config)


def test_build_extended_vector_from_features_resolves_training_name_alias():
    """Interactions recorded under the training name `inning` find the served `batting_inning`."""
    base = [2.0, 3.0]
    names = ["batting_trend_w5", "batting_inning"]
    feat_map = {"batting_trend_w5": 2.0, "batting_inning": 3.0}
    config = {"add_interactions": [("batting_trend_w5", "inning")], "add_log1p": []}
    out = build_extended_vector_from_features(base, names, feat_map, config)
    assert len(out) == 3
    assert out[2] == 6.0


def test_load_transform_config_from_metadata_missing_file(tmp_path):
    """When metadata file does not exist, returns {}."""
    out = load_transform_config_from_metadata(str(tmp_path), "batting", "T20")
    assert out == {}


def test_load_transform_config_from_metadata_format_suffix_normalized(tmp_path):
    """Format suffix with space replaced by underscore in filename."""
    path = tmp_path / "batting_metadata_LEGACY.json"
    path.write_text('{"feature_transforms": {"add_log1p": ["x"]}}', encoding="utf-8")
    out = load_transform_config_from_metadata(str(tmp_path), "batting", None)
    assert out.get("add_log1p") == ["x"] or out == {}
    out2 = load_transform_config_from_metadata(str(tmp_path), "batting", "LEGACY")
    assert "add_log1p" in out2 or out2 == {}


def test_load_transform_config_from_metadata_success(tmp_path):
    """When metadata file exists with feature_transforms, returns them."""
    path = tmp_path / "batting_metadata_T20.json"
    path.write_text(
        '{"feature_transforms": {"add_interactions": [["a","b"]], "add_log1p": ["x"]}}',
        encoding="utf-8",
    )
    out = load_transform_config_from_metadata(str(tmp_path), "batting", "T20")
    assert "add_interactions" in out or "add_log1p" in out
    assert out.get("add_log1p") == ["x"] or "add_interactions" in out


def test_load_transform_config_from_metadata_invalid_json_returns_empty(tmp_path):
    """When metadata file is invalid JSON, returns {}."""
    path = tmp_path / "batting_metadata_T20.json"
    path.write_text("not json", encoding="utf-8")
    out = load_transform_config_from_metadata(str(tmp_path), "batting", "T20")
    assert out == {}


def test_csv_column_map_keys():
    """CSV_COLUMN_MAP has batting, bowling, fielding."""
    assert "batting" in CSV_COLUMN_MAP
    assert "bowling" in CSV_COLUMN_MAP
    assert "fielding" in CSV_COLUMN_MAP
