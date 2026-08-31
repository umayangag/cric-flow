import numpy as np, pandas as pd, warnings, sys
from sklearn.linear_model import LogisticRegression
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.preprocessing import StandardScaler
from sklearn.pipeline import make_pipeline
from sklearn.metrics import roc_auc_score, brier_score_loss
warnings.filterwarnings("ignore")

CUTOFF = pd.Timestamp("2025-09-01")
df = pd.read_pickle("features.pkl")
df = df[df.y.notna()].copy()
if "--male" in sys.argv:
    df = df[df.gender == "male"]

TEAM = ["team_elo_diff", "team_form_diff", "team_h2h", "team_h2h_n", "venue_bf_rate", "venue_n", "venue_fam_diff"]
PELO = ["d_pelo_mean", "d_pelo_top3", "d_pelo_min", "t1_pelo_mean", "t2_pelo_mean", "t1_pelo_std", "t2_pelo_std"]
IMP_CORE = ["d_imp_bat_sum", "d_imp_bat_top6", "d_imp_bat_tail", "d_imp_bat_wk", "d_imp_bowl_sum", "d_imp_bowl_top5",
            "d_imp_bowl_wk", "d_imp_bowl_wk_top5"]
IMP_SIDES = [c for c in df.columns if c.startswith(("t1_imp_", "t2_imp_"))]
ROLE = ["d_n_bowlers", "d_exp_balls_bowled_top5", "d_exp_balls_faced_sum", "d_n_allrounders", "d_exp_mean_matches",
        "d_n_debutants", "d_exp_mean_matches_all", "t1_n_bowlers", "t2_n_bowlers", "t1_n_debutants", "t2_n_debutants"]
XI = PELO + IMP_CORE + IMP_SIDES + ROLE
SETS = {
    "A team-level only (Elo/form/h2h/venue)": TEAM,
    "B player-Elo only (XI-responsive)": PELO,
    "C impact ratings only (XI-responsive)": IMP_CORE + IMP_SIDES,
    "D impact + roles (XI-responsive)": IMP_CORE + IMP_SIDES + ROLE,
    "E all XI-responsive (B+D)": XI,
    "F everything (A+E)": TEAM + XI,
}

def models(seed):
    return {
        "logit": make_pipeline(StandardScaler(), LogisticRegression(C=0.3, max_iter=2000)),
        "hgb": HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0,
                                              min_samples_leaf=40, random_state=seed),
    }

def run(fmt, cols, seeds=(0, 1, 2)):
    d = df[df.fmt == fmt]
    tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    Xtr, ytr, Xte, yte = tr[cols].fillna(0).values, tr.y.values, te[cols].fillna(0).values, te.y.values
    res = {}
    for name in ("logit", "hgb"):
        aucs, briers = [], []
        for s in seeds:
            mdl = models(s)[name]
            # bootstrap-free seed variation: shuffle rows
            rng = np.random.RandomState(s); idx = rng.permutation(len(Xtr))
            mdl.fit(Xtr[idx], ytr[idx]); p = mdl.predict_proba(Xte)[:, 1]
            aucs.append(roc_auc_score(yte, p)); briers.append(brier_score_loss(yte, p))
        res[name] = (np.mean(aucs), np.std(aucs), np.mean(briers))
    base_brier = brier_score_loss(yte, np.full(len(yte), ytr.mean()))
    return len(tr), len(te), res, base_brier

print(f"holdout >= {CUTOFF.date()}   (AUC mean ± sd over 3 seeds; Brier of best model vs base-rate Brier)")
for fmt in ["T20", "ODI", "T20I", "TEST"]:
    print(f"\n=== {fmt} ===")
    for label, cols in SETS.items():
        ntr, nte, res, bb = run(fmt, cols)
        lg, hg = res["logit"], res["hgb"]
        print(f"{label:42s} n_tr={ntr:5d} n_te={nte:4d} | logit AUC {lg[0]:.3f}±{lg[1]:.3f} | hgb AUC {hg[0]:.3f}±{hg[1]:.3f} | Brier {min(lg[2],hg[2]):.3f} (base {bb:.3f})")

# single-column AUCs for the strongest XI features, T20
print("\nsingle-column holdout AUC, T20:")
d = df[(df.fmt == "T20") & (df.date >= CUTOFF)]
sc = {c: roc_auc_score(d.y, d[c].fillna(0)) for c in TEAM + XI}
for c, a in sorted(sc.items(), key=lambda kv: -abs(kv[1] - 0.5))[:15]:
    print(f"  {c:28s} {a:.3f}")
