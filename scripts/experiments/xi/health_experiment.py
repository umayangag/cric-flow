"""Two health checks on the XI model that the plan did not yet cover.

H-a  Toss / batting-order marginalisation. team1 is defined as the side batting first, which
     is unknown at selection time. Compare the oriented prediction against the average of
     both orientations (p(A bats first) and 1 - p(B bats first)), on held-out AUC.
H-b  Rating hyperparameter sensitivity: DECAY and PRIOR_BALLS were chosen by judgement.
     Re-run the pass for a small grid and report objective AUC (T20/ODI).
"""
import numpy as np, pandas as pd, warnings, subprocess, re, sys
from sklearn.linear_model import LogisticRegression
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.preprocessing import StandardScaler
from sklearn.pipeline import make_pipeline
from sklearn.metrics import roc_auc_score, brier_score_loss
warnings.filterwarnings("ignore")
CUTOFF = pd.Timestamp("2025-09-01")

# ---- H-a on the existing frame -------------------------------------------------------
df = pd.read_pickle("features.pkl"); df = df[df.y.notna()]
stems = [c[3:] for c in df.columns if c.startswith("t1_")]
TEAM = ["team_elo_diff", "team_form_diff", "team_h2h", "team_h2h_n", "venue_bf_rate", "venue_n", "venue_fam_diff"]
XI = [f"d_{s}" for s in stems] + [f"t1_{s}" for s in stems] + [f"t2_{s}" for s in stems]
ALL = XI + TEAM

def swapped(d):
    """The same fixture with the sides exchanged: B bats first."""
    s = d.copy()
    for st in stems:
        s[f"t1_{st}"], s[f"t2_{st}"] = d[f"t2_{st}"], d[f"t1_{st}"]
        s[f"d_{st}"] = -d[f"d_{st}"]
    s["team_elo_diff"] = -d["team_elo_diff"]; s["team_form_diff"] = -d["team_form_diff"]
    s["team_h2h"] = 1 - d["team_h2h"]; s["venue_fam_diff"] = -d["venue_fam_diff"]
    return s

print("H-a: oriented (knows who bats first) vs marginalised over both orders  [held-out AUC / Brier]")
for fmt in ["T20", "ODI", "T20I"]:
    d = df[df.fmt == fmt]; tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    for name, cols, mk in [("objective(logit, XI)", XI, lambda: make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000))),
                           ("display(hgb, XI+team)", ALL, lambda: HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=0))]:
        m = mk().fit(tr[cols].fillna(0).values, tr.y.values)
        p_or = m.predict_proba(te[cols].fillna(0).values)[:, 1]
        p_sw = 1 - m.predict_proba(swapped(te)[cols].fillna(0).values)[:, 1]
        p_avg = 0.5 * (p_or + p_sw)
        print(f"  {fmt:5s} {name:24s} oriented {roc_auc_score(te.y, p_or):.3f}/{brier_score_loss(te.y, p_or):.3f} | swapped-only {roc_auc_score(te.y, p_sw):.3f} | marginalised {roc_auc_score(te.y, p_avg):.3f}/{brier_score_loss(te.y, p_avg):.3f} | mean|p_or-p_sw| {np.abs(p_or-p_sw).mean():.3f}")

# ---- H-b: rating hyperparameter grid via the packaged pass ---------------------------
sys.path.insert(0, "/home/claude/xi/repo/ml-service")
from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.sources import CricsheetJsonSource
INTL = ["Afghanistan","Australia","Bangladesh","England","India","Ireland","New Zealand","Pakistan","South Africa","Sri Lanka","West Indies","Zimbabwe"]
print("\nH-b: rating hyperparameters (objective AUC, logit on XI cols; display AUC hgb)")
grid = [(0.90, 60.0), (0.80, 60.0), (0.95, 60.0), (0.90, 20.0), (0.90, 150.0), (0.97, 60.0)]
import ml.xi.ratings as R
for decay, prior in grid:
    C.DECAY_PER_MATCH = decay; C.PRIOR_BALLS = prior
    res = build(CricsheetJsonSource("/home/claude/xi/raw", INTL))
    fr = res.frame
    out = []
    for fmt in ["T20", "ODI"]:
        d = fr[fr.format_code == fmt]; tr, te = d[d.match_date < CUTOFF], d[d.match_date >= CUTOFF]
        obj = make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000)).fit(tr[C.XI_FEATURE_COLS].fillna(0).values, tr[C.TARGET_COL].values)
        dis = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=0, monotonic_cst=C.monotone_directions(C.DISPLAY_FEATURE_COLS)).fit(tr[C.DISPLAY_FEATURE_COLS].fillna(0).values, tr[C.TARGET_COL].values)
        out.append(f"{fmt} obj {roc_auc_score(te[C.TARGET_COL], obj.predict_proba(te[C.XI_FEATURE_COLS].fillna(0).values)[:,1]):.3f} disp {roc_auc_score(te[C.TARGET_COL], dis.predict_proba(te[C.DISPLAY_FEATURE_COLS].fillna(0).values)[:,1]):.3f}")
    print(f"  decay={decay:.2f} prior={prior:5.0f} | " + " | ".join(out), flush=True)
