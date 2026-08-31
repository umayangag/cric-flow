"""Player-performance predictability benchmark (secondary goal).

For every (player, match) in the holdout where the player batted / bowled, predict runs /
wickets from as-of information only. Compare:
  naive_mean      global mean of the target
  career_mean     player's as-of career mean (what the RF leans on: 47% importance)
  ewm_mean        player's as-of exponentially weighted mean (0.9 per innings)
  rating_expect   expected balls faced x (context runs/ball + bat_rate)  [pure rating pass]
  gbm_context     HistGB on player vectors + own-side/opponent XI aggregates + venue + batting-order proxy
Metrics: MAE (raw and clipped at 84 like the repo's export), within-match Spearman of
predicted vs actual (the metric that matters for choosing players), top-3-performer hit rate,
and pinball loss for a quantile model (are distributions any better than points?).
"""
import numpy as np, pandas as pd, warnings
from sklearn.ensemble import HistGradientBoostingRegressor
from sklearn.metrics import mean_absolute_error
from scipy.stats import spearmanr
warnings.filterwarnings("ignore")

CUTOFF = pd.Timestamp("2025-09-01")
feats = pd.read_pickle("features.pkl")
prow = pd.read_pickle("player_rows.pkl")
act = pd.read_pickle("actuals.pkl")
matches = pd.read_pickle("matches.pkl")
mo_info = matches[["fmt", "date", "venue"]].copy(); mo_info["mo"] = np.arange(len(matches))
morder = {m: i for i, m in enumerate(matches.match_id.values)}
feats["mo"] = feats.match_id.map(morder)

rows = prow.merge(act, on=["mo", "pc"], how="left").fillna({"balls_faced": 0, "runs": 0, "balls_bowled": 0, "wkts": 0})
rows = rows.merge(mo_info, on="mo")
# as-of career mean runs per innings (batted only) and EWM mean, computed chronologically
rows = rows.sort_values(["mo"]).reset_index(drop=True)
bat = rows[rows.balls_faced > 0].copy()
bat["career_mean"] = bat.groupby("pc").runs.transform(lambda s: s.shift(1).expanding().mean())
bat["ewm_mean"] = bat.groupby("pc").runs.transform(lambda s: s.shift(1).ewm(alpha=0.1).mean())
bat["n_prev"] = bat.groupby("pc").cumcount()
bowl = rows[rows.balls_bowled > 0].copy()
bowl["career_mean_w"] = bowl.groupby("pc").wkts.transform(lambda s: s.shift(1).expanding().mean())
bowl["ewm_mean_w"] = bowl.groupby("pc").wkts.transform(lambda s: s.shift(1).ewm(alpha=0.1).mean())

# match context: own side and opponent aggregates from the features frame
side_cols = [c for c in feats.columns if c.startswith("t1_")]
stems = [c[3:] for c in side_cols]
ctx = []
for side in (1, 2):
    own = feats[["mo"] + [f"t{side}_{s}" for s in stems] + [f"t{3-side}_{s}" for s in stems] + ["venue_bf_rate", "venue_inn1_runs", "team_elo_diff"]].copy()
    own.columns = ["mo"] + [f"own_{s}" for s in stems] + [f"opp_{s}" for s in stems] + ["venue_bf_rate", "venue_inn1_runs", "team_elo_diff"]
    own["side"] = side
    own["elo_edge"] = own.team_elo_diff * (1 if side == 1 else -1)
    ctx.append(own)
ctx = pd.concat(ctx)
VEC = ["exp_balls_faced", "exp_balls_bowled", "bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate", "career", "career_all", "pelo", "keeper"]
CTX = [f"opp_{s}" for s in stems if "bowl" in s or "n_bowlers" in s] + [f"own_{s}" for s in stems if "bat" in s] + ["venue_bf_rate", "venue_inn1_runs", "elo_edge", "side"]

def report(fmt):
    d = bat[(bat.fmt == fmt)].merge(ctx, on=["mo", "side"])
    d["ctx_rpb"] = 1.25  # ~league runs per ball; rating_expect below uses the player's own expected balls
    tr, te = d[d.date < CUTOFF], d[d.date >= CUTOFF]
    te = te[te.n_prev >= 3]  # players with some history, as the repo's min_batting_innings=5 roughly does
    y = te.runs.values; yc = np.minimum(y, 84)
    preds = {
        "naive_mean": np.full(len(te), tr.runs.mean()),
        "career_mean": te.career_mean.fillna(tr.runs.mean()).values,
        "ewm_mean": te.ewm_mean.fillna(tr.runs.mean()).values,
        "rating_expect": np.maximum(te.exp_balls_faced.values * (1.25 + te.bat_rate.values), 0),
    }
    X_cols = VEC + CTX + ["career_mean", "ewm_mean", "n_prev"]
    Xtr = tr[X_cols].fillna(0).values; Xte = te[X_cols].fillna(0).values
    gbm = HistGradientBoostingRegressor(loss="absolute_error", max_depth=4, learning_rate=0.05, max_iter=400, min_samples_leaf=50, random_state=0).fit(Xtr, tr.runs.values)
    preds["gbm_context_mae"] = gbm.predict(Xte)
    gbm2 = HistGradientBoostingRegressor(loss="squared_error", max_depth=4, learning_rate=0.05, max_iter=400, min_samples_leaf=50, random_state=0).fit(Xtr, tr.runs.values)
    preds["gbm_context_mse"] = gbm2.predict(Xte)
    gbm_np = HistGradientBoostingRegressor(loss="squared_error", max_depth=4, learning_rate=0.05, max_iter=400, min_samples_leaf=50, random_state=0).fit(tr[VEC + ["career_mean", "ewm_mean", "n_prev"]].fillna(0).values, tr.runs.values)
    preds["gbm_player_only"] = gbm_np.predict(te[VEC + ["career_mean", "ewm_mean", "n_prev"]].fillna(0).values)
    q = {a: HistGradientBoostingRegressor(loss="quantile", quantile=a, max_depth=4, learning_rate=0.05, max_iter=300, min_samples_leaf=50, random_state=0).fit(Xtr, tr.runs.values).predict(Xte) for a in (0.1, 0.5, 0.9)}
    cover = ((y >= q[0.1]) & (y <= q[0.9])).mean()
    print(f"\n=== {fmt} batting: runs per innings, holdout n={len(te)}, mean {y.mean():.1f}, sd {y.std():.1f} ===")
    print(f"{'predictor':20s} {'MAE':>6s} {'MAE(clip84)':>11s} {'R2':>6s} {'Spearman/match':>15s} {'top3 hit':>9s}")
    for name, p in preds.items():
        tt = te.assign(p=p)
        rho = tt.groupby("mo").apply(lambda g: spearmanr(g.p, g.runs).correlation if len(g) >= 4 and g.runs.std() > 0 and g.p.std() > 0 else np.nan).dropna().mean()
        top = tt.groupby("mo").apply(lambda g: len(set(g.nlargest(3, "p").pc) & set(g.nlargest(3, "runs").pc)) / 3 if len(g) >= 6 else np.nan).dropna().mean()
        r2 = 1 - ((y - p) ** 2).sum() / ((y - y.mean()) ** 2).sum()
        print(f"{name:20s} {mean_absolute_error(y, p):6.2f} {mean_absolute_error(yc, np.minimum(p, 84)):11.2f} {r2:6.3f} {rho:15.3f} {top:9.3f}")
    print(f"quantile model: 10-90 interval coverage {cover:.2f} (ideal 0.80); median-of-quantile MAE {mean_absolute_error(y, q[0.5]):.2f}")

    # bowling: wickets per bowling innings
    b = bowl[bowl.fmt == fmt].merge(ctx, on=["mo", "side"])
    btr, bte = b[b.date < CUTOFF], b[b.date >= CUTOFF]
    yb = bte.wkts.values
    bp = {
        "naive_mean": np.full(len(bte), btr.wkts.mean()),
        "career_mean": bte.career_mean_w.fillna(btr.wkts.mean()).values,
        "rating_expect": np.maximum(bte.exp_balls_bowled.values * (0.05 + bte.bowl_wrate.values), 0),
    }
    BX = VEC + [f"opp_{s}" for s in stems if "bat" in s] + ["venue_bf_rate", "venue_inn1_runs", "elo_edge", "side", "career_mean_w", "ewm_mean_w"]
    g = HistGradientBoostingRegressor(loss="poisson", max_depth=4, learning_rate=0.05, max_iter=400, min_samples_leaf=50, random_state=0).fit(btr[BX].fillna(0).values, btr.wkts.values)
    bp["gbm_context_poisson"] = g.predict(bte[BX].fillna(0).values)
    print(f"--- {fmt} bowling: wickets per bowling innings, holdout n={len(bte)}, mean {yb.mean():.2f} ---")
    for name, p in bp.items():
        tt = bte.assign(p=p)
        rho = tt.groupby("mo").apply(lambda g: spearmanr(g.p, g.wkts).correlation if len(g) >= 4 and g.wkts.std() > 0 and g.p.std() > 0 else np.nan).dropna().mean()
        print(f"{name:22s} MAE {mean_absolute_error(yb, p):5.3f}  Spearman/match {rho:6.3f}")

for fmt in ["T20", "ODI"]:
    report(fmt)
