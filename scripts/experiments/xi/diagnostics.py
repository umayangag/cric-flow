"""Selection diagnostics on the XI-responsive model (feature set E), per format.

1. Swap responsiveness (S-9 diagnostic): upgrade team1's weakest batter to its best -> delta p.
2. Ground truth for marginal player value: drop each team1 player in turn (XI of 10 aggregated
   as if the slot were an average player) -> delta p. Correlate with the player's ACTUAL
   performance in that match (runs above expectation + wickets). Also: does delta p rank the
   player-of-match candidates? (top-3 actual performers vs the rest).
3. Cold start: replace a player by a debutant -> delta p must be small and non-explosive.
"""
import numpy as np, pandas as pd, warnings
from sklearn.ensemble import HistGradientBoostingClassifier
from scipy.stats import spearmanr
warnings.filterwarnings("ignore")

# reuse aggregate() without re-running the pass
import importlib.util, types
src = open("build_features.py").read()
ns = {}
ns = {"np": np, "pd": pd}; exec(src[src.index("MIN_BALLS = "):src.index("PLAYER_ROWS = []")], ns)
aggregate = ns["aggregate"]

CUTOFF = pd.Timestamp("2025-09-01")
feats = pd.read_pickle("features.pkl"); feats = feats[feats.y.notna()]
prow = pd.read_pickle("player_rows.pkl"); act = pd.read_pickle("actuals.pkl")
matches = pd.read_pickle("matches.pkl")
FMTS = ["T20", "T20I", "ODI", "TEST"]; fmt_idx = {f: i for i, f in enumerate(FMTS)}

PELO = ["d_pelo_mean", "d_pelo_top3", "d_pelo_min", "t1_pelo_mean", "t2_pelo_mean", "t1_pelo_std", "t2_pelo_std"]
IMP_CORE = ["d_imp_bat_sum", "d_imp_bat_top6", "d_imp_bat_tail", "d_imp_bat_wk", "d_imp_bowl_sum", "d_imp_bowl_top5", "d_imp_bowl_wk", "d_imp_bowl_wk_top5"]
IMP_SIDES = [c for c in feats.columns if c.startswith(("t1_imp_", "t2_imp_"))]
ROLE = ["d_n_bowlers", "d_exp_balls_bowled_top5", "d_exp_balls_faced_sum", "d_n_allrounders", "d_exp_mean_matches", "d_n_debutants", "d_exp_mean_matches_all", "t1_n_bowlers", "t2_n_bowlers", "t1_n_debutants", "t2_n_debutants"]
XI = PELO + IMP_CORE + IMP_SIDES + ROLE
VEC_KEYS = ["exp_balls_faced", "exp_balls_bowled", "bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate", "career", "career_all", "pelo", "keeper"]

def row_from_vectors(f, v1, v2):
    r = {}; r.update(aggregate(v1, f, "t1_")); r.update(aggregate(v2, f, "t2_"))
    for c in [c for c in r if c.startswith("t1_")]:
        r["d_" + c[3:]] = r[c] - r["t2_" + c[3:]]
    return r

def vec(g):
    return {k: g[k].values.astype(float) for k in VEC_KEYS}

def drop_player(v, j):
    return {k: np.delete(a, j) for k, a in v.items()}

for fmt in ["T20", "ODI", "T20I"]:
    f = fmt_idx[fmt]
    d = feats[feats.fmt == fmt]; tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    mdl = HistGradientBoostingClassifier(max_depth=3, learning_rate=0.04, max_iter=300, l2_regularization=1.0, min_samples_leaf=40, random_state=0)
    mdl.fit(tr[XI].fillna(0).values, tr.y.values)
    te_mo = te.index  # feats index == match order (rows appended in order, but filtered) -> use match_id map
    morder = {m: i for i, m in enumerate(matches.match_id.values)}
    swap_dp, marg = [], []
    cold = []
    for _, m in te.iterrows():
        mo = morder[m.match_id]
        g = prow[prow.mo == mo]
        g1, g2 = g[g.side == 1], g[g.side == 2]
        if len(g1) < 11 or len(g2) < 11: continue
        v1, v2 = vec(g1), vec(g2)
        base = row_from_vectors(f, v1, v2)
        X0 = np.array([[base[c] for c in XI]]); p0 = mdl.predict_proba(X0)[0, 1]
        # 1. swap: weakest batter -> copy of best batter
        bc = v1["bat_rate"] * v1["exp_balls_faced"]; jw, jb = np.argmin(bc), np.argmax(bc)
        v1s = {k: a.copy() for k, a in v1.items()}
        for k in VEC_KEYS: v1s[k][jw] = v1[k][jb]
        rs = row_from_vectors(f, v1s, v2); ps = mdl.predict_proba(np.array([[rs[c] for c in XI]]))[0, 1]
        swap_dp.append(ps - p0)
        # 2. marginal value of each team1 player = p(with) - p(without, replaced by neutral player)
        a = act[act.mo == mo].set_index("pc")
        for j in range(len(g1)):
            v1d = {k: a_.copy() for k, a_ in v1.items()}
            # neutral replacement: zero rates, typical involvement, median experience, mean pelo of the side
            v1d["bat_rate"][j] = 0; v1d["bat_wrate"][j] = 0; v1d["bowl_rate"][j] = 0; v1d["bowl_wrate"][j] = 0
            v1d["pelo"][j] = 1500.0; v1d["career"][j] = np.median(v1["career"]); v1d["career_all"][j] = np.median(v1["career_all"])
            rd = row_from_vectors(f, v1d, v2); pd_ = mdl.predict_proba(np.array([[rd[c] for c in XI]]))[0, 1]
            pc = g1.pc.values[j]
            runs = a.runs.get(pc, 0.0); bf = a.balls_faced.get(pc, 0.0); wk = a.wkts.get(pc, 0.0); bb = a.balls_bowled.get(pc, 0.0)
            marg.append((mo, pc, p0 - pd_, runs, bf, wk, bb))
        # 3. cold start: replace weakest batter with a debutant
        v1c = {k: a_.copy() for k, a_ in v1.items()}
        for k in ["bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate", "exp_balls_faced", "exp_balls_bowled", "career", "career_all", "keeper"]: v1c[k][jw] = 0.0
        v1c["pelo"][jw] = 1500.0
        rc = row_from_vectors(f, v1c, v2); pc_ = mdl.predict_proba(np.array([[rc[c] for c in XI]]))[0, 1]
        cold.append(pc_ - p0)
    swap_dp = np.array(swap_dp); cold = np.array(cold)
    M = pd.DataFrame(marg, columns=["mo", "pc", "dp", "runs", "bf", "wk", "bb"])
    # actual contribution score: runs + 20*wickets (T20-ish weight), rank within match
    wt = {"T20": 20, "T20I": 20, "ODI": 25}[fmt]
    M["perf"] = M.runs + wt * M.wk
    M["perf_rank"] = M.groupby("mo").perf.rank(ascending=False)
    M["dp_rank"] = M.groupby("mo").dp.rank(ascending=False)
    rho = M.groupby("mo").apply(lambda g: spearmanr(g.dp, g.perf).correlation if g.perf.std() > 0 and g.dp.std() > 0 else np.nan).dropna()
    top3 = M[M.perf_rank <= 3].dp.mean(); rest = M[M.perf_rank > 3].dp.mean()
    print(f"\n=== {fmt}  (holdout matches with full XIs: {len(swap_dp)}) ===")
    print(f"1. swap weakest->best batter: median dp {np.median(swap_dp):+.3f}, p90 {np.percentile(swap_dp, 90):+.3f}, max {swap_dp.max():+.3f}, share >0.05: {(swap_dp > 0.05).mean():.0%}, share <0: {(swap_dp < 0).mean():.0%}")
    print(f"2. marginal value vs actual performance: within-match Spearman mean {rho.mean():+.3f} (n={len(rho)}), "
          f"mean dp of top-3 actual performers {top3:+.4f} vs rest {rest:+.4f}; pooled Spearman {spearmanr(M.dp, M.perf).correlation:+.3f}")
    print(f"   marginal dp distribution: median {M.dp.median():+.4f}, p10 {M.dp.quantile(.1):+.4f}, p90 {M.dp.quantile(.9):+.4f}, share negative {(M.dp < 0).mean():.0%}")
    print(f"3. replace weakest batter with debutant: median dp {np.median(cold):+.3f}, p10 {np.percentile(cold, 10):+.3f}, p90 {np.percentile(cold, 90):+.3f}")
