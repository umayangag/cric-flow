"""The chronological rating pass.

``RatingState`` holds every as-of quantity. For each match, in date order, the caller first
reads features from the state (``side_vectors`` / ``aggregate_side`` / ``team_context``) and
only then calls ``update`` with that match. Nothing a feature reads can have seen the match
it describes -- that is the whole leakage guarantee, and it is structural rather than a query
convention.

Impact ratings are relative to a context baseline: the running average runs-per-ball and
wickets-per-ball for (format, over number). A batter's ``bat_rate`` is therefore "runs above
what an average ball in that over yields", so a death-overs 150 strike rate is worth more
than a powerplay one, and the ratings are already venue- and era-neutral in the mean.
"""

from __future__ import annotations

import math
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Sequence

import numpy as np

from ml.xi import contract as C
from ml.xi.sources import Deliveries, MatchRecord

_N_FMT = len(C.FORMAT_CODES)


def _grow(arr: np.ndarray, n: int, fill: float) -> np.ndarray:
    if arr.shape[-1] >= n:
        return arr
    new_len = max(n, arr.shape[-1] * 2, 1024)
    out = np.full(arr.shape[:-1] + (new_len,), fill, dtype=arr.dtype)
    out[..., : arr.shape[-1]] = arr
    return out


@dataclass
class PlayerIndex:
    """Stable string key -> integer slot."""

    key_to_slot: Dict[str, int] = field(default_factory=dict)
    keys: List[str] = field(default_factory=list)

    def slot(self, key: str) -> int:
        s = self.key_to_slot.get(key)
        if s is None:
            s = len(self.keys)
            self.key_to_slot[key] = s
            self.keys.append(key)
        return s

    def slots(self, keys: Sequence[str]) -> np.ndarray:
        return np.fromiter((self.slot(k) for k in keys), dtype=np.int64, count=len(keys))

    def __len__(self) -> int:
        return len(self.keys)


def elo_expected(rating_a: float, rating_b: float) -> float:
    return 1.0 / (1.0 + 10 ** ((rating_b - rating_a) / 400.0))


class RatingState:
    """All as-of state. Arrays are (format, player_slot); grown on demand."""

    def __init__(self) -> None:
        self.players = PlayerIndex()
        n = 1024
        z = lambda: np.zeros((_N_FMT, n))  # noqa: E731
        self.bat_rae, self.bat_balls, self.bat_wae, self.bat_matches = z(), z(), z(), z()
        self.bowl_rse, self.bowl_balls, self.bowl_wae, self.bowl_matches = z(), z(), z(), z()
        self.career = z()
        self.career_all = np.zeros(n)
        self.keeper = np.zeros(n)
        self.pelo = np.full((_N_FMT, n), C.ELO_INITIAL)
        # context baselines: (format, over) cumulative balls / runs / wickets with a weak prior
        self.ctx_balls = np.ones((_N_FMT, C.MAX_OVER_INDEX))
        self.ctx_runs = np.full((_N_FMT, C.MAX_OVER_INDEX), 1.2)
        self.ctx_wickets = np.full((_N_FMT, C.MAX_OVER_INDEX), 0.05)
        # team-level state
        self.team_elo: Dict[tuple, float] = defaultdict(lambda: C.ELO_INITIAL)
        self.team_results: Dict[tuple, List[float]] = defaultdict(list)
        self.head_to_head: Dict[tuple, List[float]] = defaultdict(list)
        self.venue_bat_first: Dict[tuple, List[float]] = defaultdict(lambda: [0.0, 0.0])
        self.team_venue_matches: Dict[tuple, int] = defaultdict(int)
        self.matches_seen = 0
        self.last_date = None

    # -- capacity ------------------------------------------------------------------------
    def _ensure(self, n: int) -> None:
        if self.bat_rae.shape[1] >= n:
            return
        for name in (
            "bat_rae", "bat_balls", "bat_wae", "bat_matches", "bowl_rse", "bowl_balls", "bowl_wae", "bowl_matches", "career",
        ):  # fmt: skip
            setattr(self, name, _grow(getattr(self, name), n, 0.0))
        self.career_all = _grow(self.career_all, n, 0.0)
        self.keeper = _grow(self.keeper, n, 0.0)
        self.pelo = _grow(self.pelo, n, C.ELO_INITIAL)

    def _slots(self, keys: Sequence[str]) -> np.ndarray:
        s = self.players.slots(keys)
        self._ensure(len(self.players))
        return s

    # -- reads ---------------------------------------------------------------------------
    def side_vectors(self, format_code: str, player_keys: Sequence[str]) -> Dict[str, np.ndarray]:
        """Per-player as-of vectors for one side (``contract.PLAYER_VECTOR_KEYS``)."""
        f = C.FORMAT_INDEX[format_code]
        s = self._slots(player_keys)
        bat_m = self.bat_matches[f, s]
        bowl_m = self.bowl_matches[f, s]
        return {
            "exp_balls_faced": np.where(bat_m > 0, self.bat_balls[f, s] / np.maximum(bat_m, 1e-9), 0.0),
            "exp_balls_bowled": np.where(bowl_m > 0, self.bowl_balls[f, s] / np.maximum(bowl_m, 1e-9), 0.0),
            "bat_rate": self.bat_rae[f, s] / (self.bat_balls[f, s] + C.PRIOR_BALLS),
            "bat_wrate": self.bat_wae[f, s] / (self.bat_balls[f, s] + C.PRIOR_BALLS),
            "bowl_rate": self.bowl_rse[f, s] / (self.bowl_balls[f, s] + C.PRIOR_BALLS),
            "bowl_wrate": self.bowl_wae[f, s] / (self.bowl_balls[f, s] + C.PRIOR_BALLS),
            "career": self.career[f, s].copy(),
            "career_all": self.career_all[s].copy(),
            "pelo": self.pelo[f, s].copy(),
            "keeper": self.keeper[s].copy(),
        }

    def team_context(self, match: MatchRecord) -> Dict[str, float]:
        """Team-level features (constant w.r.t. the XI)."""
        fmt, t1, t2, v = match.format_code, match.team1, match.team2, match.venue
        e1, e2 = self.team_elo[(fmt, t1)], self.team_elo[(fmt, t2)]
        r1 = self.team_results[(fmt, t1)][-C.TEAM_FORM_WINDOW :]
        r2 = self.team_results[(fmt, t2)][-C.TEAM_FORM_WINDOW :]
        hh = self.head_to_head[(fmt, t1, t2)][-C.HEAD_TO_HEAD_WINDOW :]
        vb = self.venue_bat_first[(fmt, v)]
        return {
            "team_elo_diff": e1 - e2,
            "team_form_diff": (float(np.mean(r1)) if r1 else 0.5) - (float(np.mean(r2)) if r2 else 0.5),
            "team_h2h": float(np.mean(hh)) if hh else 0.5,
            "team_h2h_n": float(len(hh)),
            "venue_bf_rate": (vb[0] + 0.5 * C.VENUE_PRIOR_MATCHES) / (vb[1] + C.VENUE_PRIOR_MATCHES),
            "venue_n": vb[1],
            "venue_fam_diff": math.log1p(self.team_venue_matches[(t1, v)])
            - math.log1p(self.team_venue_matches[(t2, v)]),
        }

    # -- update --------------------------------------------------------------------------
    def update(self, match: MatchRecord) -> None:
        """Fold one match into the state. Must be called after its features were read."""
        f = C.FORMAT_INDEX[match.format_code]
        s1, s2 = self._slots(match.team1_players), self._slots(match.team2_players)
        d = match.deliveries
        if len(d):
            self._update_impact(f, d)
        both = np.concatenate([s1, s2])
        self.career[f, both] += 1.0
        self.career_all[both] += 1.0
        self.team_venue_matches[(match.team1, match.venue)] += 1
        self.team_venue_matches[(match.team2, match.venue)] += 1
        y = match.outcome
        fmt = match.format_code
        if y is not None:
            k1, k2 = (fmt, match.team1), (fmt, match.team2)
            e1, e2 = self.team_elo[k1], self.team_elo[k2]
            exp1 = elo_expected(e1, e2)
            self.team_elo[k1] = e1 + C.K_TEAM_ELO * (y - exp1)
            self.team_elo[k2] = e2 + C.K_TEAM_ELO * ((1.0 - y) - (1.0 - exp1))
            self.team_results[k1].append(y)
            self.team_results[k2].append(1.0 - y)
            self.head_to_head[(fmt, match.team1, match.team2)].append(y)
            self.head_to_head[(fmt, match.team2, match.team1)].append(1.0 - y)
            vb = self.venue_bat_first[(fmt, match.venue)]
            vb[0] += y
            vb[1] += 1.0
            p_exp = elo_expected(float(self.pelo[f, s1].mean()), float(self.pelo[f, s2].mean()))
            self.pelo[f, s1] += C.K_PLAYER_ELO * (y - p_exp)
            self.pelo[f, s2] += C.K_PLAYER_ELO * ((1.0 - y) - (1.0 - p_exp))
        elif match.result in ("tie", "draw"):
            self.team_results[(fmt, match.team1)].append(0.5)
            self.team_results[(fmt, match.team2)].append(0.5)
        self.matches_seen += 1
        self.last_date = match.match_date

    def _update_impact(self, f: int, d: Deliveries) -> None:
        over = np.minimum(d.over, C.MAX_OVER_INDEX - 1)
        exp_runs = self.ctx_runs[f, over] / self.ctx_balls[f, over]
        exp_wk = self.ctx_wickets[f, over] / self.ctx_balls[f, over]
        batters = self._slots(list(d.batter))
        bowlers = self._slots(list(d.bowler))
        ones = np.ones(len(d))
        self._accumulate(
            self.bat_rae,
            self.bat_balls,
            self.bat_wae,
            self.bat_matches,
            f,
            batters,
            d.runs_batter - exp_runs,
            exp_wk - d.wicket,
            ones,
        )
        self._accumulate(
            self.bowl_rse,
            self.bowl_balls,
            self.bowl_wae,
            self.bowl_matches,
            f,
            bowlers,
            exp_runs - d.runs_total,
            d.bowler_wicket - exp_wk,
            ones,
        )
        np.add.at(self.ctx_balls[f], over, 1.0)
        np.add.at(self.ctx_runs[f], over, d.runs_total)
        np.add.at(self.ctx_wickets[f], over, d.wicket)
        stumped = np.nonzero(d.stumping)[0]
        for i in stumped:
            for key in d.fielders[i] if i < len(d.fielders) else []:
                self.keeper[self._slots([key])[0]] = 1.0

    def _accumulate(self, total, balls, wtotal, matches, f, who, value, wvalue, count) -> None:
        uniq, inv = np.unique(who, return_inverse=True)
        for arr in (total, balls, wtotal, matches):
            arr[f, uniq] *= C.DECAY_PER_MATCH
        total[f, uniq] += np.bincount(inv, weights=value, minlength=len(uniq))
        balls[f, uniq] += np.bincount(inv, weights=count, minlength=len(uniq))
        wtotal[f, uniq] += np.bincount(inv, weights=wvalue, minlength=len(uniq))
        matches[f, uniq] += 1.0


# ---------------------------------------------------------------------------
# Aggregation: per-player vectors -> one side's features. Pure, reused for counterfactual XIs.
# ---------------------------------------------------------------------------


def aggregate_side(vectors: Dict[str, np.ndarray], format_code: str) -> Dict[str, float]:
    ebf, ebb = vectors["exp_balls_faced"], vectors["exp_balls_bowled"]
    bat = vectors["bat_rate"] * ebf
    bat_w = vectors["bat_wrate"] * ebf
    bowl = vectors["bowl_rate"] * ebb
    bowl_w = vectors["bowl_wrate"] * ebb
    bat_sorted = np.sort(bat)[::-1]
    bowl_sorted = np.sort(bowl)[::-1]
    is_bowler = C.is_bowling_option(ebb, format_code)
    career = vectors["career"]
    pelo = vectors["pelo"]
    return {
        "imp_bat_sum": float(bat.sum()),
        "imp_bat_top6": float(bat_sorted[:6].sum()),
        "imp_bat_tail": float(bat_sorted[6:].sum()),
        "imp_bat_wk": float(bat_w.sum()),
        "imp_bowl_sum": float(bowl.sum()),
        "imp_bowl_top5": float(bowl_sorted[:5].sum()),
        "imp_bowl_wk": float(bowl_w.sum()),
        "imp_bowl_wk_top5": float(np.sort(bowl_w)[::-1][:5].sum()),
        "n_bowlers": float(is_bowler.sum()),
        "exp_balls_bowled_top5": float(np.sort(ebb)[::-1][:5].sum()),
        "exp_balls_faced_sum": float(ebf.sum()),
        "has_keeper": float(vectors["keeper"].any()),
        "n_allrounders": float((is_bowler & (ebf >= 0.6 * np.median(ebf + 1e-9))).sum()),
        "exp_mean_matches": float(career.mean()),
        "n_debutants": float((career == 0).sum()),
        "exp_mean_matches_all": float(vectors["career_all"].mean()),
        "pelo_mean": float(pelo.mean()),
        "pelo_top3": float(np.sort(pelo)[::-1][:3].mean()),
        "pelo_min": float(pelo.min()),
        "pelo_std": float(pelo.std()),
    }


def match_features(side1: Dict[str, float], side2: Dict[str, float]) -> Dict[str, float]:
    """Both sides' aggregates -> the XI feature row (``contract.XI_FEATURE_COLS``)."""
    row: Dict[str, float] = {}
    for stem in C.SIDE_FEATURE_STEMS:
        row[f"t1_{stem}"] = side1[stem]
        row[f"t2_{stem}"] = side2[stem]
        row[f"d_{stem}"] = side1[stem] - side2[stem]
    return row


def xi_feature_vector(
    side1: Dict[str, float], side2: Dict[str, float], columns: Optional[List[str]] = None
) -> np.ndarray:
    row = match_features(side1, side2)
    return np.asarray([row[c] for c in (columns or C.XI_FEATURE_COLS)], dtype=float)
