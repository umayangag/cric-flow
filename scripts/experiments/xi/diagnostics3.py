"""Monotone objective: enforce that p(team1 wins) is non-decreasing in team1's impact/quality
features and non-increasing in team2's, so a hill-climb over XIs cannot be rewarded for
making the side worse. Compare AUC and swap-negativity with / without constraints, and vs logit.
"""
import numpy as np, pandas as pd, warnings
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.preprocessing import StandardScaler
from sklearn.pipeline import make_pipeline
from sklearn.metrics import roc_auc_score
warnings.filterwarnings("ignore")
exec(open("diagnostics.py").read().split("for fmt in [")[0])

# direction of each feature w.r.t. team1 winning: +1 increasing, -1 decreasing, 0 free
def direction(c):
    good = ("imp_bat_sum", "imp_bat_top6", "imp_bat_tail", "imp_bat_wk", "imp_bowl_sum", "imp_bowl_top5", "imp_bowl_wk",
            "imp_bowl_wk_top5", "pelo_mean", "pelo_top3", "pelo_min", "n_bowlers", "exp_mean_matches", "exp_mean_matches_all")
    bad = ("n_debutants",)
    stem = c[2:] if c.startswith("d_") else c[3:]
    sign = 0
    if stem in good: sign = 1
    elif stem in bad: sign = -1
    if c.startswith("t2_"): sign = -sign
    return sign

MONO = [direction(c) for c in XI]
print("monotone constraints:", sum(1 for m in MONO if m != 0), "of", len(XI), "features")

def fit(fmt, kind, seed=0):
    d = feats[feats.fmt == fmt]; tr = d[d.date < CUTOFF]
    if kind == "hgb":
        m = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=seed)
    elif kind == "hgb_mono":
        m = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=seed, monotonic_cst=MONO)
    else:
        m = make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=3000))
    m.fit(tr[XI].fillna(0).values, tr.y.values); return m

for fmt in ["T20", "ODI"]:
    f = fmt_idx[fmt]
    d = feats[feats.fmt == fmt]; te = d[d.date >= CUTOFF]
    morder = {m: i for i, m in enumerate(matches.match_id.values)}
    print(f"\n=== {fmt} ===")
    for kind in ["hgb", "hgb_mono", "logit"]:
        mdl = fit(fmt, kind)
        auc = roc_auc_score(te.y, mdl.predict_proba(te[XI].fillna(0).values)[:, 1])
        neg = []; dps = []
        for _, m in te.iterrows():
            g = prow[prow.mo == morder[m.match_id]]; g1, g2 = g[g.side == 1], g[g.side == 2]
            if len(g1) < 11 or len(g2) < 11: continue
            v1, v2 = vec(g1), vec(g2)
            p0 = mdl.predict_proba(np.array([[row_from_vectors(f, v1, v2)[c] for c in XI]]))[0, 1]
            bc = v1["bat_rate"] * v1["exp_balls_faced"]; jw, jb = np.argmin(bc), np.argmax(bc)
            vs = {k: a.copy() for k, a in v1.items()}
            for k in ["bat_rate", "bat_wrate", "exp_balls_faced"]: vs[k][jw] = v1[k][jb]
            pb = mdl.predict_proba(np.array([[row_from_vectors(f, vs, v2)[c] for c in XI]]))[0, 1]
            dps.append(pb - p0)
        dps = np.array(dps)
        print(f"  {kind:9s} AUC {auc:.3f} | batting-only upgrade: median dp {np.median(dps):+.3f}, share<0 {(dps<0).mean():.1%}, share<-0.01 {(dps<-0.01).mean():.1%}")
