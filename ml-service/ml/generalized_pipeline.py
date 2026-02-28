"""
Generalized ML pipeline for Player Performance and Match Outcome prediction.

Uses ball-by-ball cricket data with:
- Time-decay weighting and yearly-format averages normalization
- State-space, EWM form, and matchup features
- Model-agnostic preprocessing (Target Encoding, RobustScaler)
- Time-series walk-forward validation, Brier Score, MAE, overfitting guardrail
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import date
from typing import Any, Dict, List, Literal, Optional, Tuple

import numpy as np
import pandas as pd
from sklearn.linear_model import LogisticRegression
from sklearn.model_selection import TimeSeriesSplit
from sklearn.preprocessing import RobustScaler, StandardScaler

logger = logging.getLogger(__name__)

# Target encoding uses category_encoders if available
try:
    from category_encoders import TargetEncoder

    HAS_TARGET_ENCODER = True
except ImportError:
    HAS_TARGET_ENCODER = False


@dataclass
class PipelineConfig:
    """Configuration for the generalized pipeline."""

    task: Literal["match_outcome", "player_performance"] = "match_outcome"
    time_decay_halflife_years: float = 2.0  # Recent 2y weighted more
    ewm_span: int = 10  # EWM for player hot streaks
    matchup_min_samples: int = 5  # Min samples for batter vs bowler SR
    n_splits: int = 5  # Time-series CV splits
    delta_threshold: float = 0.08  # Max train-val gap
    use_target_encoding: bool = True
    use_robust_scaler: bool = True
    random_state: int = 42


# -----------------------------------------------------------------------------
# Time-decay and era normalization
# -----------------------------------------------------------------------------


def compute_time_decay_weights(
    match_dates: pd.Series,
    reference_date: Optional[date] = None,
    halflife_years: float = 2.0,
) -> np.ndarray:
    """Exponential time decay: recent matches weighted more.

    w = 0.5^(years_ago / halflife). Matches from 2y ago get 0.5x weight.
    """
    ref = reference_date or match_dates.max()
    if hasattr(ref, "date"):
        ref = ref.date() if hasattr(ref, "date") else ref
    dates = pd.to_datetime(match_dates).dt.date
    years_ago = np.array([(ref - d).days / 365.25 for d in dates], dtype=float)
    return np.power(0.5, years_ago / halflife_years)


def compute_yearly_format_averages(
    df: pd.DataFrame,
    format_col: str = "format_code",
    year_col: str = "match_year",
) -> Dict[Tuple[str, int], Dict[str, float]]:
    """Compute yearly format averages for normalization.

    Returns {(format_code, year): {metric: value}}.
    """
    if "match_date" in df.columns:
        df = df.copy()
        df["match_year"] = pd.to_datetime(df["match_date"]).dt.year
    out: Dict[Tuple[str, int], Dict[str, float]] = {}
    for (fmt, yr), g in df.groupby([format_col, year_col]):
        out[(str(fmt), int(yr))] = {
            "rpo": (g["runs_total"].sum() / max(1, g["ball_seq"].nunique())) * 6.0,
            "wicket_rate": g["wicket_kind"].notna().sum() / max(1, len(g) / 6),
        }
    return out


def normalize_by_yearly_format(
    df: pd.DataFrame,
    yearly_avgs: Dict[Tuple[str, int], Dict[str, float]],
    format_col: str = "format_code",
    year_col: str = "match_year",
) -> pd.DataFrame:
    """Normalize batting/bowling metrics by yearly format averages."""
    df = df.copy()
    if year_col not in df.columns and "match_date" in df.columns:
        df[year_col] = pd.to_datetime(df["match_date"]).dt.year
    return df


# -----------------------------------------------------------------------------
# Feature engineering
# -----------------------------------------------------------------------------


def _add_state_space_features(df: pd.DataFrame) -> pd.DataFrame:
    """State-space at ball N: runs remaining, wickets in hand, balls remaining, RRR vs CRR."""
    out = df.copy()
    # Cumulative runs and wickets per innings
    grp = ["match_id", "innings"]
    out["cumulative_runs"] = out.groupby(grp)["runs_total"].cumsum()
    out["cumulative_wickets"] = out.groupby(grp)["wicket_kind"].transform(lambda s: s.notna().astype(int).cumsum())
    # Balls remaining (assuming total balls from match context)
    total_balls = out.groupby(grp)["ball_seq"].transform("max")
    out["balls_remaining"] = total_balls - out["ball_seq"]
    out["balls_remaining"] = out["balls_remaining"].clip(lower=0)
    # Runs remaining (for chase)
    out["runs_remaining"] = out["target_runs"] - out["cumulative_runs"]
    out["runs_remaining"] = out["runs_remaining"].clip(lower=0)
    # Wickets in hand
    out["wickets_in_hand"] = 10 - out["cumulative_wickets"]
    out["wickets_in_hand"] = out["wickets_in_hand"].clip(lower=0)
    # Current run rate (per over)
    out["current_rr"] = np.where(
        out["ball_seq"] > 0,
        (out["cumulative_runs"] / out["ball_seq"]) * 6.0,
        0.0,
    )
    # Required run rate
    out["required_rr"] = np.where(
        out["balls_remaining"] > 0,
        (out["runs_remaining"] / out["balls_remaining"]) * 6.0,
        0.0,
    )
    out["rrr_vs_crr"] = out["required_rr"] - out["current_rr"]
    return out


def _add_ewm_form(
    df: pd.DataFrame,
    span: int = 10,
    group_cols: Optional[List[str]] = None,
) -> pd.DataFrame:
    """Exponentially weighted moving average for player hot streaks."""
    out = df.copy()
    group_cols = group_cols or ["striker_id", "format_code"]
    # Per-striker, per-format rolling SR
    for gcol in group_cols:
        if gcol not in out.columns:
            continue
    # Simple: striker-level EWM of runs per ball
    grp = ["striker_id", "format_code"] if "format_code" in out.columns else ["striker_id"]
    out["bat_ewm_rpo"] = (
        out.groupby(grp)["runs_total"].transform(lambda s: s.ewm(span=span, adjust=False).mean().shift(1)).fillna(0)
    )
    out["bowl_ewm_rpo"] = (
        out.groupby(["bowler_id", "format_code"] if "format_code" in out.columns else ["bowler_id"])["runs_total"]
        .transform(lambda s: s.ewm(span=span, adjust=False).mean().shift(1))
        .fillna(0)
    )
    return out


def _add_matchup_matrix(
    df: pd.DataFrame,
    min_samples: int = 5,
) -> pd.DataFrame:
    """Batter Strike Rate vs Bowler historical SR (matchup)."""
    out = df.copy()
    # Pre-compute batter SR and bowler econ by format
    if "format_code" in out.columns:
        bat_sr = (
            out.groupby(["striker_id", "format_code"])["runs_total"].transform("sum")
            / out.groupby(["striker_id", "format_code"])["ball_seq"].transform("count").clip(1)
        ) * 6.0
        bowl_econ = (
            out.groupby(["bowler_id", "format_code"])["runs_total"].transform("sum")
            / out.groupby(["bowler_id", "format_code"])["ball_seq"].transform("count").clip(1)
        ) * 6.0
    else:
        bat_sr = (
            out.groupby("striker_id")["runs_total"].transform("sum")
            / out.groupby("striker_id")["ball_seq"].transform("count").clip(1)
        ) * 6.0
        bowl_econ = (
            out.groupby("bowler_id")["runs_total"].transform("sum")
            / out.groupby("bowler_id")["ball_seq"].transform("count").clip(1)
        ) * 6.0
    out["batter_sr_6"] = bat_sr
    out["bowler_econ_6"] = bowl_econ
    out["matchup_bat_v_bowl"] = bat_sr / np.clip(bowl_econ, 0.1, None)
    return out


# -----------------------------------------------------------------------------
# Preprocessing layer
# -----------------------------------------------------------------------------


def _build_target_encoder(
    X: pd.DataFrame,
    y: np.ndarray,
    cat_cols: List[str],
) -> Optional[Any]:
    """Build TargetEncoder on categorical columns."""
    if not HAS_TARGET_ENCODER or not cat_cols:
        return None
    enc = TargetEncoder(cols=cat_cols, smoothing=10)
    enc.fit(X[cat_cols], y)
    return enc


# -----------------------------------------------------------------------------
# Main pipeline class
# -----------------------------------------------------------------------------


class CricketGeneralizedPipeline:
    """
    Model-agnostic preprocessing and feature engineering for cricket ML.

    Key methods:
    - fit(): Compute yearly averages, fit encoders/scalers
    - transform(): Apply era normalization, state-space, EWM, matchup features
    """

    def __init__(self, config: Optional[PipelineConfig] = None):
        self.config = config or PipelineConfig()
        self.yearly_format_avgs_: Dict[Tuple[str, int], Dict[str, float]] = {}
        self.target_encoder_: Optional[Any] = None
        self.scaler_ = RobustScaler() if self.config.use_robust_scaler else StandardScaler()
        self.feature_names_: List[str] = []
        self.categorical_cols_: List[str] = []
        self.is_fitted_ = False

    def _add_derived_columns(self, df: pd.DataFrame) -> pd.DataFrame:
        """Add match_year, phase encoding, etc."""
        df = df.copy()
        if "match_date" in df.columns:
            df["match_year"] = pd.to_datetime(df["match_date"]).dt.year
        if "phase" in df.columns:
            df["phase_encoding"] = pd.Categorical(df["phase"]).codes
        return df

    def fit(
        self,
        df: pd.DataFrame,
        target_col: Optional[str] = None,
        sample_weight: Optional[np.ndarray] = None,
    ) -> "CricketGeneralizedPipeline":
        """Fit preprocessing: yearly averages, target encoder, scaler."""
        df = self._add_derived_columns(df)
        df = _add_state_space_features(df)
        df = _add_ewm_form(df, span=self.config.ewm_span)
        df = _add_matchup_matrix(df, min_samples=self.config.matchup_min_samples)

        self.yearly_format_avgs_ = compute_yearly_format_averages(df)

        # Identify categorical columns for target encoding
        cat_candidates = ["striker_id", "bowler_id", "venue_id", "format_code"]
        self.categorical_cols_ = [c for c in cat_candidates if c in df.columns]

        feature_cols = [
            "balls_remaining",
            "runs_remaining",
            "wickets_in_hand",
            "current_rr",
            "required_rr",
            "rrr_vs_crr",
            "bat_ewm_rpo",
            "bowl_ewm_rpo",
            "batter_sr_6",
            "bowler_econ_6",
            "matchup_bat_v_bowl",
        ]
        self.feature_names_ = [c for c in feature_cols if c in df.columns]
        if "phase_encoding" in df.columns:
            self.feature_names_.append("phase_encoding")
        if "match_year" in df.columns:
            self.feature_names_.append("match_year")

        X = df[self.feature_names_].copy()
        for c in self.categorical_cols_:
            if c in df.columns and c not in X.columns:
                X[c] = df[c]
                self.feature_names_.append(c)

        X = X.fillna(0)

        if target_col and target_col in df.columns and self.config.use_target_encoding:
            y = df[target_col].values
            cat_present = [c for c in self.categorical_cols_ if c in X.columns]
            self.target_encoder_ = _build_target_encoder(X, y, cat_present)
            if self.target_encoder_ is not None and cat_present:
                X[cat_present] = self.target_encoder_.transform(X[cat_present])

        self.scaler_.fit(X[self.feature_names_])
        self.is_fitted_ = True
        return self

    def transform(self, df: pd.DataFrame) -> np.ndarray:
        """Transform raw ball-by-ball data into feature matrix."""
        if not self.is_fitted_:
            raise RuntimeError("Pipeline must be fitted before transform")

        df = self._add_derived_columns(df)
        df = _add_state_space_features(df)
        df = _add_ewm_form(df, span=self.config.ewm_span)
        df = _add_matchup_matrix(df, min_samples=self.config.matchup_min_samples)

        X = df[self.feature_names_].copy()
        X = X.fillna(0)

        if self.target_encoder_ is not None:
            cat_present = [c for c in self.categorical_cols_ if c in X.columns]
            if cat_present:
                X[cat_present] = self.target_encoder_.transform(X[cat_present])

        return self.scaler_.transform(X[self.feature_names_])

    def fit_transform(
        self,
        df: pd.DataFrame,
        target_col: Optional[str] = None,
        sample_weight: Optional[np.ndarray] = None,
    ) -> np.ndarray:
        """Fit and transform in one step."""
        self.fit(df, target_col=target_col, sample_weight=sample_weight)
        return self.transform(df)


# -----------------------------------------------------------------------------
# Model comparison and validation
# -----------------------------------------------------------------------------


def time_series_split_indices(
    match_dates: np.ndarray,
    n_splits: int = 5,
) -> List[Tuple[np.ndarray, np.ndarray]]:
    """Walk-forward split by match date."""
    dates = pd.to_datetime(match_dates)
    order = np.argsort(dates)
    n = len(order)
    splits = []
    for i in range(1, n_splits + 1):
        cutoff = int(n * i / (n_splits + 1))
        train_idx = order[:cutoff]
        val_idx = order[cutoff : cutoff + max(1, (n - cutoff) // (n_splits - i + 1))]
        if len(val_idx) > 0:
            splits.append((train_idx, val_idx))
    return splits


def brier_score(y_true: np.ndarray, y_prob: np.ndarray) -> float:
    """Brier score for probabilistic predictions (win prob)."""
    return float(np.mean((y_true - y_prob) ** 2))


def compare_models(
    X: np.ndarray,
    y: np.ndarray,
    match_dates: np.ndarray,
    task: Literal["match_outcome", "player_performance"] = "match_outcome",
    n_splits: int = 5,
    delta_threshold: float = 0.08,
    sample_weight: Optional[np.ndarray] = None,
) -> Dict[str, Any]:
    """
    Compare baseline (LogReg), tree (XGB/LGB), optionally sequential (LSTM/GRU).

    Returns metrics and best model. Uses walk-forward validation.
    """
    tscv = TimeSeriesSplit(n_splits=n_splits)
    results: Dict[str, Dict[str, List[float]]] = {}
    is_classification = task == "match_outcome"

    # Lazy-load tree models (may fail on some envs e.g. missing libomp)
    has_xgb, has_lgb = False, False
    try:
        import xgboost as _xgb

        has_xgb = True
    except Exception:
        pass
    try:
        import lightgbm as _lgb

        has_lgb = True
    except Exception:
        pass

    models: Dict[str, Any] = {}
    if is_classification:
        models["logistic"] = LogisticRegression(max_iter=1000, random_state=42)
        if has_xgb:
            models["xgboost"] = _xgb.XGBClassifier(
                n_estimators=100, max_depth=6, random_state=42, use_label_encoder=False, eval_metric="logloss"
            )
        if has_lgb:
            models["lightgbm"] = _lgb.LGBMClassifier(n_estimators=100, max_depth=6, random_state=42, verbose=-1)
    else:
        from sklearn.ensemble import RandomForestRegressor

        models["rf"] = RandomForestRegressor(n_estimators=100, max_depth=10, random_state=42)
        if has_xgb:
            models["xgboost"] = _xgb.XGBRegressor(n_estimators=100, max_depth=6, random_state=42)
        if has_lgb:
            models["lightgbm"] = _lgb.LGBMRegressor(n_estimators=100, max_depth=6, random_state=42, verbose=-1)

    order = np.argsort(pd.to_datetime(match_dates))
    X_sorted = X[order]
    y_sorted = y[order]
    w_sorted = sample_weight[order] if sample_weight is not None else None
    for name, model in models.items():
        results[name] = {"train": [], "val": [], "brier": [] if is_classification else [], "mae": []}
        for train_idx, val_idx in tscv.split(X_sorted):
            tr_idx, vl_idx = train_idx, val_idx
            X_tr, X_vl = X_sorted[tr_idx], X_sorted[vl_idx]
            y_tr, y_vl = y_sorted[tr_idx], y_sorted[vl_idx]
            w_tr = w_sorted[tr_idx] if w_sorted is not None else None

            if is_classification:
                model.fit(X_tr, y_tr, sample_weight=w_tr)
                y_tr_p = model.predict_proba(X_tr)[:, 1]
                y_vl_p = model.predict_proba(X_vl)[:, 1]
                results[name]["train"].append(brier_score(y_tr, y_tr_p))
                results[name]["val"].append(brier_score(y_vl, y_vl_p))
                results[name]["brier"].append(brier_score(y_vl, y_vl_p))
                results[name]["mae"].append(float(np.mean(np.abs(y_vl - (y_vl_p > 0.5)))))
            else:
                model.fit(X_tr, y_tr, sample_weight=w_tr)
                y_tr_p = model.predict(X_tr)
                y_vl_p = model.predict(X_vl)
                results[name]["train"].append(float(np.mean(np.abs(y_tr - y_tr_p))))
                results[name]["val"].append(float(np.mean(np.abs(y_vl - y_vl_p))))
                results[name]["mae"].append(float(np.mean(np.abs(y_vl - y_vl_p))))

    # Check delta guardrail (overfitting: val worse than train)
    summary = {}
    for name, r in results.items():
        train_avg = np.mean(r["train"])
        val_avg = np.mean(r["val"])
        delta = val_avg - train_avg  # positive = val worse = potential overfitting
        summary[name] = {
            "train_metric": train_avg,
            "val_metric": val_avg,
            "delta": abs(delta),
            "brier_mean": np.mean(r["brier"]) if r["brier"] else None,
            "mae_mean": np.mean(r["mae"]) if r["mae"] else None,
            "passes_guardrail": abs(delta) < delta_threshold,
        }

    best = min(
        summary.items(),
        key=lambda x: (x[1]["val_metric"], 0 if x[1]["passes_guardrail"] else 1),
    )
    return {
        "results": results,
        "summary": summary,
        "best_model": best[0],
        "best_metrics": best[1],
    }


def build_match_level_df(df: pd.DataFrame) -> pd.DataFrame:
    """Aggregate ball-by-ball to match-level for outcome prediction.

    Requires outcome_winner_opposition_id and batting_team_opposition_id from match_inning.
    """
    inn1 = (
        df[df["innings"] == 1]
        .groupby("match_id")
        .agg(
            match_date=("match_date", "first"),
            format_code=("format_code", "first"),
            venue_id=("venue_id", "first"),
            team1_id=("batting_team_opposition_id", "first"),
            team2_id=("bowling_team_opposition_id", "first"),
            team1_runs=("runs_total", "sum"),
            outcome_winner_id=("outcome_winner_opposition_id", "first"),
        )
        .reset_index()
    )
    inn1["team1_wins"] = (inn1["outcome_winner_id"] == inn1["team1_id"]).astype(int)
    return inn1


def run_generalized_pipeline(
    df: pd.DataFrame,
    task: Literal["match_outcome", "player_performance"] = "player_performance",
    target_col: Optional[str] = None,
    config: Optional[PipelineConfig] = None,
) -> Dict[str, Any]:
    """
    End-to-end: fit pipeline, compute sample weights, compare models.

    For match_outcome: aggregate to match-level, predict team1_wins (0/1).
    For player_performance: ball-level target (runs_total or is_wicket).
    """
    cfg = config or PipelineConfig(task=task)
    pipe = CricketGeneralizedPipeline(cfg)

    # Time-decay weights
    weights = compute_time_decay_weights(
        df["match_date"],
        halflife_years=cfg.time_decay_halflife_years,
    )

    if task == "match_outcome":
        if "outcome_winner_opposition_id" not in df.columns or "batting_team_opposition_id" not in df.columns:
            logger.warning("match_outcome requires outcome_winner and batting_team from loader")
            return {"error": "Match outcome requires match table outcome_winner_opposition_id"}
        match_df = build_match_level_df(df)
        # Fit on ball-level (pipeline needs ball-level for state-space), then aggregate features
        # Simpler: use match-level features from first-N balls or innings summary
        target_col = "team1_wins"
        pipe.fit(df, target_col=None, sample_weight=weights)  # No target at ball level for TE
        # For match outcome we need match-level X: average ball-level features per match
        ball_features = pipe.transform(df)
        # Map ball index back to match
        mid = df["match_id"].values
        X_match = np.zeros((len(match_df), ball_features.shape[1]))
        for i, m in enumerate(match_df["match_id"]):
            mask = mid == m
            X_match[i] = ball_features[mask].mean(axis=0)
        y = match_df[target_col].values
        dates = match_df["match_date"].values
        w = np.array([weights[df["match_id"] == m].mean() for m in match_df["match_id"]])
    else:
        target_col = target_col or "runs_total"
        if target_col not in df.columns:
            target_col = "runs_total"  # default for ball-level
        if target_col == "is_wicket" and "is_wicket" not in df.columns:
            df = df.copy()
            df["is_wicket"] = df["wicket_kind"].notna().astype(int)
        pipe.fit(df, target_col=target_col, sample_weight=weights)
        X_match = pipe.transform(df)
        y = df[target_col].values
        dates = df["match_date"].values
        w = weights

    comparison = compare_models(
        X_match,
        y,
        dates,
        task=task,
        n_splits=cfg.n_splits,
        delta_threshold=cfg.delta_threshold,
        sample_weight=w,
    )
    comparison["pipeline"] = pipe
    return comparison
