"""(a) Swap delta-p split by whether the replaced player was a bowling option.
(b) Does the SPECIFIC XI carry signal beyond the team's usual strength?
    Compare: team-level (A) + team's rolling-typical XI features  vs  A + the actual XI's features.
    If the actual XI beats the typical XI, lineup composition -- the thing a selector changes --
    is measurably predictive, and the model captures it.
"""
import numpy as np, pandas as pd, warnings
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.metrics import roc_auc_score
warnings.filterwarnings("ignore")
exec(open("diagnostics.py").read().split("for fmt in [")[0])   # reuse setup, model cols, helpers

# ---------- (a) ----------
for fmt in ["T20", "ODI"]:
    f = fmt_idx[fmt]
    d = feats[feats.fmt == fmt]; tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    mdl = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=0)
    mdl.fit(tr[XI].fillna(0).values, tr.y.values)
    morder = {m: i for i, m in enumerate(matches.match_id.values)}
    rec = []
    for _, m in te.iterrows():
        g = prow[prow.mo == morder[m.match_id]]; g1, g2 = g[g.side == 1], g[g.side == 2]
        if len(g1) < 11 or len(g2) < 11: continue
        v1, v2 = vec(g1), vec(g2)
        p0 = mdl.predict_proba(np.array([[row_from_vectors(f, v1, v2)[c] for c in XI]]))[0, 1]
        bc = v1["bat_rate"] * v1["exp_balls_faced"]; jw, jb = np.argmin(bc), np.argmax(bc)
        was_bowler = v1["exp_balls_bowled"][jw] >= ns["MIN_BALLS"][f]
        # full swap (loses the bowler) vs batting-only upgrade (keeps bowling vectors)
        vs_full = {k: a.copy() for k, a in v1.items()}
        for k in VEC_KEYS: vs_full[k][jw] = v1[k][jb]
        vs_bat = {k: a.copy() for k, a in v1.items()}
        for k in ["bat_rate", "bat_wrate", "exp_balls_faced"]: vs_bat[k][jw] = v1[k][jb]
        pf = mdl.predict_proba(np.array([[row_from_vectors(f, vs_full, v2)[c] for c in XI]]))[0, 1]
        pb = mdl.predict_proba(np.array([[row_from_vectors(f, vs_bat, v2)[c] for c in XI]]))[0, 1]
        rec.append((was_bowler, pf - p0, pb - p0))
    R = pd.DataFrame(rec, columns=["was_bowler", "dp_full", "dp_batonly"])
    print(f"\n=== {fmt} swap analysis (n={len(R)}) ===")
    for wb, g in R.groupby("was_bowler"):
        print(f"  replaced player was a bowling option={wb!s:5s} n={len(g):4d} | full swap: median {g.dp_full.median():+.3f}, share<0 {(g.dp_full<0).mean():.0%} | batting-only upgrade: median {g.dp_batonly.median():+.3f}, share<0 {(g.dp_batonly<0).mean():.0%}")

# ---------- (b) ----------
side_cols = [c[3:] for c in feats.columns if c.startswith("t1_") and ("imp_" in c or "pelo" in c or c[3:] in
             ["n_bowlers", "exp_balls_bowled_top5", "exp_balls_faced_sum", "n_allrounders", "exp_mean_matches", "n_debutants", "exp_mean_matches_all"])]
F = feats.sort_values("date").copy()
# rolling typical XI features per (fmt, team): mean of that team's side features over its previous 5 matches
long = pd.concat([
    F[["match_id", "date", "fmt", "team1"] + ["t1_" + c for c in side_cols]].rename(columns={"team1": "team", **{"t1_" + c: c for c in side_cols}}),
    F[["match_id", "date", "fmt", "team2"] + ["t2_" + c for c in side_cols]].rename(columns={"team2": "team", **{"t2_" + c: c for c in side_cols}}),
]).sort_values("date")
typ = long.groupby(["fmt", "team"])[side_cols].transform(lambda s: s.shift(1).rolling(5, min_periods=1).mean())
typ.columns = ["typ_" + c for c in side_cols]
long = pd.concat([long, typ], axis=1)
t1 = long.drop_duplicates(["match_id", "team"]).set_index(["match_id", "team"])
for side, tcol in ((1, "team1"), (2, "team2")):
    idx = pd.MultiIndex.from_arrays([F.match_id, F[tcol]])
    for c in side_cols:
        F[f"t{side}_typ_{c}"] = t1.loc[idx, "typ_" + c].values
for c in side_cols:
    F[f"d_typ_{c}"] = F[f"t1_typ_{c}"] - F[f"t2_typ_{c}"]
    F[f"d_dev_{c}"] = (F[f"t1_{c}"] - F[f"t1_typ_{c}"]) - (F[f"t2_{c}"] - F[f"t2_typ_{c}"])
TYP = [f"d_typ_{c}" for c in side_cols] + [f"t1_typ_{c}" for c in side_cols] + [f"t2_typ_{c}" for c in side_cols]
ACT = [f"d_{c}" for c in side_cols] + [f"t1_{c}" for c in side_cols] + [f"t2_{c}" for c in side_cols]
DEV = [f"d_dev_{c}" for c in side_cols]
TEAM = ["team_elo_diff", "team_form_diff", "team_h2h", "team_h2h_n", "venue_bf_rate", "venue_n", "venue_fam_diff"]

def auc(fmt, cols, seeds=(0, 1, 2)):
    d = F[F.fmt == fmt]; tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    out = []
    for s in seeds:
        mdl = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=s)
        rng = np.random.RandomState(s); idx = rng.permutation(len(tr))
        mdl.fit(tr[cols].fillna(0).values[idx], tr.y.values[idx]); out.append(roc_auc_score(te.y, mdl.predict_proba(te[cols].fillna(0).values)[:, 1]))
    return np.mean(out), np.std(out)

print("\n=== does the specific XI matter beyond the team's usual XI? (held-out AUC, 3 seeds) ===")
for fmt in ["T20", "ODI", "T20I"]:
    a = auc(fmt, TEAM); b = auc(fmt, TEAM + TYP); c = auc(fmt, TEAM + ACT); d_ = auc(fmt, TEAM + TYP + DEV)
    print(f"{fmt:5s} team-level only {a[0]:.3f} | + team's TYPICAL XI (last 5) {b[0]:.3f} | + ACTUAL XI {c[0]:.3f} | + typical + deviation-of-actual {d_[0]:.3f}   (sd≈{max(a[1],b[1],c[1],d_[1]):.3f})")
