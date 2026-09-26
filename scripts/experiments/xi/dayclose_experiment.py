"""Same-day ordering: matches on one date are processed in id order, so a match later in
the id order can see the result of a same-day match it may actually have preceded. The
strict alternative is day-close batching: features for every match on a date come from
prior dates only. Measure the difference."""
import numpy as np, pandas as pd, sys, warnings
from sklearn.linear_model import LogisticRegression
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.preprocessing import StandardScaler
from sklearn.pipeline import make_pipeline
from sklearn.metrics import roc_auc_score
warnings.filterwarnings("ignore")
sys.path.insert(0, "/home/claude/xi/repo/ml-service")
from ml.xi import contract as C
from ml.xi.ratings import RatingState, aggregate_side, match_features
from ml.xi.sources import CricsheetJsonSource
CUTOFF = pd.Timestamp("2025-09-01")

def build(day_close: bool):
    state = RatingState(); rows = []; pending = []; current = None
    def flush():
        for m in pending: state.update(m)
        pending.clear()
    for m in CricsheetJsonSource("/home/claude/xi/raw").iter_matches():
        if day_close and current is not None and m.match_date != current: flush()
        current = m.match_date
        if m.outcome is not None:
            r = {"format_code": m.format_code, "match_date": pd.Timestamp(m.match_date), C.TARGET_COL: m.outcome}
            r.update(match_features(aggregate_side(state.side_vectors(m.format_code, m.team1_players), m.format_code),
                                    aggregate_side(state.side_vectors(m.format_code, m.team2_players), m.format_code)))
            r.update(state.team_context(m)); rows.append(r)
        if day_close: pending.append(m)
        else: state.update(m)
    flush(); return pd.DataFrame(rows)

same_day = None
for label, dc in (("sequential (id order within a day)", False), ("day-close batching", True)):
    fr = build(dc); out = []
    for fmt in ["T20", "ODI", "T20I"]:
        d = fr[fr.format_code == fmt]; tr, te = d[d.match_date < CUTOFF], d[d.match_date >= CUTOFF]
        obj = make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000)).fit(tr[C.XI_FEATURE_COLS].fillna(0).values, tr[C.TARGET_COL].values)
        dis = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=0, monotonic_cst=C.monotone_directions(C.DISPLAY_FEATURE_COLS)).fit(tr[C.DISPLAY_FEATURE_COLS].fillna(0).values, tr[C.TARGET_COL].values)
        out.append(f"{fmt} obj {roc_auc_score(te[C.TARGET_COL], obj.predict_proba(te[C.XI_FEATURE_COLS].fillna(0).values)[:,1]):.3f} disp {roc_auc_score(te[C.TARGET_COL], dis.predict_proba(te[C.DISPLAY_FEATURE_COLS].fillna(0).values)[:,1]):.3f}")
    print(f"{label:36s} | " + " | ".join(out), flush=True)
m = pd.read_pickle("matches.pkl"); print("share of matches sharing a date with another match in the same format:", f"{m.duplicated(['date','fmt'], keep=False).mean():.0%}")
