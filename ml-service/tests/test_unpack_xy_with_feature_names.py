"""Tests for unpack_xy_with_feature_names (loader return normalization)."""

from __future__ import annotations

import numpy as np
import pytest

from ml.tuning.data_loaders import unpack_xy_with_feature_names


def test_unpack_none_returns_none_triple() -> None:
    x, y, names = unpack_xy_with_feature_names(None)
    assert x is None and y is None and names is None


def test_unpack_two_tuple() -> None:
    X = np.array([[1.0]])
    Y = np.array([[2.0]])
    x, y, names = unpack_xy_with_feature_names((X, Y))
    assert x is X and y is Y and names is None


def test_unpack_three_tuple_with_list_names() -> None:
    X = np.array([[1.0]])
    Y = np.array([[2.0]])
    x, y, names = unpack_xy_with_feature_names((X, Y, ["a", "b"]))
    assert names == ["a", "b"]


def test_unpack_four_tuple_win_style_feature_list_last() -> None:
    X = np.array([[1.0]])
    Y = np.array([1])
    w = np.array([1.0])
    feat = ["f1", "f2"]
    x, y, names = unpack_xy_with_feature_names((X, Y, w, feat))
    assert names == feat


def test_unpack_dict_single_key() -> None:
    X = np.array([[1.0]])
    Y = np.array([[2.0]])
    x, y, names = unpack_xy_with_feature_names({"T20": (X, Y, ["n"])})
    assert x is X and names == ["n"]


def test_unpack_dict_respects_format_key() -> None:
    X1 = np.array([[1.0]])
    Y1 = np.array([[1.0]])
    X2 = np.array([[2.0]])
    Y2 = np.array([[2.0]])
    d = {"T20": (X1, Y1, ["a"]), "ODI": (X2, Y2, ["b"])}
    x, y, names = unpack_xy_with_feature_names(d, format_key="ODI")
    assert x is X2 and names == ["b"]


def test_unpack_dict_multiple_keys_without_format_raises() -> None:
    d = {"T20": (np.array([[1.0]]), np.array([[1.0]])), "ODI": (np.array([[2.0]]), np.array([[2.0]]))}
    with pytest.raises(TypeError, match="format_key"):
        unpack_xy_with_feature_names(d)
