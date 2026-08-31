"""Chronological as-of feature builder.

For every match (date order) we first compute features from state accumulated over
EARLIER matches only, then update the state with this match. No feature can see its own
result. Three families are produced so their contributions can be separated:

  team_*    : team Elo, form, head-to-head, venue bat-first bias, venue familiarity.
              (constant w.r.t. the XI -- useful for outcome accuracy, useless for selection)
  pelo_*    : player Elo -- each player carries a rating updated by the results of the
              matches they played in. XI-responsive analogue of team Elo.
  imp_*     : ball-level impact ratings -- runs above expectation (batting), runs saved
              (bowling), wickets above expectation, weighted by expected involvement
              (expected balls faced / bowled, known before the match) plus role coverage.
"""
import numpy as np, pandas as pd, glob, math, pickle, sys
from collections import defaultdict

CUTOFF = pd.Timestamp("2025-09-01")

matches = pd.read_pickle("matches.pkl")
lineups = pd.read_pickle("lineups.pkl")
parts = sorted(glob.glob("balls_parts/*.pkl"))
balls = pd.concat([pd.read_pickle(p) for p in parts], ignore_index=True)
print("loaded", matches.shape, lineups.shape, balls.shape, flush=True)

# --- integer codes for players ------------------------------------------------------
all_pids = pd.Index(pd.unique(np.concatenate([lineups.pid.values, balls.batter.values, balls.bowler.values])))
pid_code = {p: i for i, p in enumerate(all_pids)}
NP = len(all_pids)
lineups["pc"] = lineups.pid.map(pid_code).astype(np.int32)
balls["bc"] = balls.batter.map(pid_code).astype(np.int32)
balls["wc"] = balls.bowler.map(pid_code).astype(np.int32)
balls.drop(columns=["batter", "bowler", "kind"], inplace=True)

# order balls by match order, build slices
morder = {m: i for i, m in enumerate(matches.match_id.values)}
balls["mo"] = balls.match_id.map(morder).astype(np.int32)
balls = balls[balls.mo.notna()].sort_values(["mo", "inn", "seq"]).reset_index(drop=True)
mo = balls.mo.values
starts = np.searchsorted(mo, np.arange(len(matches)))
ends = np.searchsorted(mo, np.arange(len(matches)), side="right")
lineups["mo"] = lineups.match_id.map(morder)
lu_groups = {k: g for k, g in lineups.groupby("mo")}

B_side = balls.side.values.astype(np.int8); B_inn = balls.inn.values.astype(np.int8)
B_over = np.minimum(balls.over.values, 99).astype(np.int16)
B_legal = balls.legal.values.astype(bool)
B_rb = balls.runs_bat.values.astype(np.float32); B_rt = balls.runs_total.values.astype(np.float32)
B_wk = balls.wicket.values.astype(np.float32); B_bwk = balls.bowler_wicket.values.astype(np.float32)
B_bc = balls.bc.values; B_wc = balls.wc.values
del balls

FMTS = ["T20", "T20I", "ODI", "TEST"]
fmt_idx = {f: i for i, f in enumerate(FMTS)}

# --- state --------------------------------------------------------------------------
# context baselines per format x over: runs/ball, wicket/ball (as-of, cumulative)
ctx_balls = np.ones((4, 100)) * 1.0
ctx_runs = np.ones((4, 100)) * 1.2     # weak prior ~1.2 runs/ball
ctx_wk = np.ones((4, 100)) * 0.05
# per player per format accumulators (decayed)
DECAY = 0.90            # per match played
PRIOR_BALLS = 60.0      # shrinkage for rate estimates
bat_rae = np.zeros((4, NP)); bat_balls = np.zeros((4, NP)); bat_wae = np.zeros((4, NP)); bat_matches = np.zeros((4, NP))
bowl_rse = np.zeros((4, NP)); bowl_balls = np.zeros((4, NP)); bowl_wae = np.zeros((4, NP)); bowl_matches = np.zeros((4, NP))
bat_pos_sum = np.zeros((4, NP)); bat_pos_n = np.zeros((4, NP))
career_matches = np.zeros((4, NP)); career_matches_all = np.zeros(NP)
stumpings = np.zeros(NP)
# player Elo per format; team Elo per format
pelo = np.full((4, NP), 1500.0); pelo_n = np.zeros((4, NP))
telo = defaultdict(lambda: 1500.0)
team_hist = defaultdict(list)      # (fmt, team) -> list of results (1 win, 0 loss, 0.5 tie)
h2h = defaultdict(list)            # (fmt, a, b) -> results for a
venue_bf = defaultdict(lambda: [0.0, 0.0])   # (fmt, venue) -> [bat-first wins, matches]
venue_runs1 = defaultdict(lambda: [0.0, 0.0])  # (fmt, venue) -> [inn1 runs, n]
team_venue = defaultdict(int)      # (team, venue) -> matches played
K_TEAM = 24.0; K_PLAYER = 12.0

def elo_exp(ra, rb): return 1.0 / (1.0 + 10 ** ((rb - ra) / 400.0))

MIN_BALLS = {0: 12, 1: 12, 2: 30, 3: 60}

def player_vectors(f, pcs):
    """Per-player as-of quantities for one side (pure read of state)."""
    pcs = np.asarray(pcs)
    bm = np.maximum(bat_matches[f, pcs], 1e-9); wm = np.maximum(bowl_matches[f, pcs], 1e-9)
    ebf = np.where(bat_matches[f, pcs] > 0, bat_balls[f, pcs] / bm, 0.0)
    ebb = np.where(bowl_matches[f, pcs] > 0, bowl_balls[f, pcs] / wm, 0.0)
    return dict(
        pc=pcs,
        exp_balls_faced=ebf, exp_balls_bowled=ebb,
        bat_rate=bat_rae[f, pcs] / (bat_balls[f, pcs] + PRIOR_BALLS),
        bat_wrate=bat_wae[f, pcs] / (bat_balls[f, pcs] + PRIOR_BALLS),
        bowl_rate=bowl_rse[f, pcs] / (bowl_balls[f, pcs] + PRIOR_BALLS),
        bowl_wrate=bowl_wae[f, pcs] / (bowl_balls[f, pcs] + PRIOR_BALLS),
        career=career_matches[f, pcs].astype(float), career_all=career_matches_all[pcs].astype(float),
        pelo=pelo[f, pcs].copy(), keeper=(stumpings[pcs] > 0).astype(float),
    )

def aggregate(v, f, prefix):
    """XI aggregates from per-player vectors (pure function; reused for counterfactual XIs)."""
    out = {}
    bat_contrib = v["bat_rate"] * v["exp_balls_faced"]; bat_wcontrib = v["bat_wrate"] * v["exp_balls_faced"]
    bowl_contrib = v["bowl_rate"] * v["exp_balls_bowled"]; bowl_wcontrib = v["bowl_wrate"] * v["exp_balls_bowled"]
    srt = np.sort(bat_contrib)[::-1]
    out[prefix + "imp_bat_sum"] = bat_contrib.sum(); out[prefix + "imp_bat_top6"] = srt[:6].sum(); out[prefix + "imp_bat_tail"] = srt[6:].sum()
    out[prefix + "imp_bat_wk"] = bat_wcontrib.sum()
    srb = np.sort(bowl_contrib)[::-1]
    out[prefix + "imp_bowl_sum"] = bowl_contrib.sum(); out[prefix + "imp_bowl_top5"] = srb[:5].sum()
    out[prefix + "imp_bowl_wk"] = bowl_wcontrib.sum(); out[prefix + "imp_bowl_wk_top5"] = np.sort(bowl_wcontrib)[::-1][:5].sum()
    mb = MIN_BALLS[f]; ebb = v["exp_balls_bowled"]; ebf = v["exp_balls_faced"]
    out[prefix + "n_bowlers"] = float((ebb >= mb).sum())
    out[prefix + "exp_balls_bowled_top5"] = np.sort(ebb)[::-1][:5].sum()
    out[prefix + "exp_balls_faced_sum"] = ebf.sum()
    out[prefix + "has_keeper"] = float(v["keeper"].any())
    out[prefix + "n_allrounders"] = float(((ebb >= mb) & (ebf >= 0.6 * np.median(ebf + 1e-9))).sum())
    out[prefix + "exp_mean_matches"] = v["career"].mean(); out[prefix + "n_debutants"] = float((v["career"] == 0).sum())
    out[prefix + "exp_mean_matches_all"] = v["career_all"].mean()
    pe = v["pelo"]
    out[prefix + "pelo_mean"] = pe.mean(); out[prefix + "pelo_top3"] = np.sort(pe)[::-1][:3].mean()
    out[prefix + "pelo_min"] = pe.min(); out[prefix + "pelo_std"] = pe.std()
    return out

def side_features(f, pcs, prefix):
    return aggregate(player_vectors(f, pcs), f, prefix)

def with_differentials(df):
    for c in [c for c in df.columns if c.startswith("t1_")]:
        df["d_" + c[3:]] = df[c] - df["t2_" + c[3:]]
    return df

PLAYER_ROWS = []
def record_players(mo, f, p1, p2):
    for side, pcs in ((1, p1), (2, p2)):
        v = player_vectors(f, pcs)
        n = len(pcs)
        PLAYER_ROWS.append(pd.DataFrame({"mo": mo, "side": side, **{k: val for k, val in v.items()}}))

rows = []
ACT = []
for i in range(len(matches)):
    m = matches.iloc[i]
    f = fmt_idx[m.fmt]
    lg = lu_groups.get(i)
    if lg is None or lg.side.nunique() < 2:
        continue
    p1 = lg[lg.side == 1].pc.values; p2 = lg[lg.side == 2].pc.values
    t1, t2, v = m.team1, m.team2, m.venue
    key1, key2 = (m.fmt, t1), (m.fmt, t2)
    # ---------------- features (as-of) ----------------
    r = {"match_id": m.match_id, "date": m.date, "fmt": m.fmt, "gender": m.gender, "team1": t1, "team2": t2,
         "venue": v, "winner": m.winner, "result": m.result, "method": m.method}
    r["y"] = 1.0 if m.winner == t1 else (0.0 if m.winner == t2 else np.nan)
    e1, e2 = telo[key1], telo[key2]
    r["team_elo_diff"] = e1 - e2
    fh1, fh2 = team_hist[key1][-10:], team_hist[key2][-10:]
    r["team_form_diff"] = (np.mean(fh1) if fh1 else 0.5) - (np.mean(fh2) if fh2 else 0.5)
    hh = h2h[(m.fmt, t1, t2)][-10:]
    r["team_h2h"] = (np.mean(hh) if hh else 0.5); r["team_h2h_n"] = len(hh)
    vb = venue_bf[(m.fmt, v)]
    r["venue_bf_rate"] = (vb[0] + 0.5 * 5) / (vb[1] + 5); r["venue_n"] = vb[1]
    vr = venue_runs1[(m.fmt, v)]
    r["venue_inn1_runs"] = vr[0] / vr[1] if vr[1] > 0 else np.nan
    r["venue_fam_diff"] = math.log1p(team_venue[(t1, v)]) - math.log1p(team_venue[(t2, v)])
    r["n1"] = len(p1); r["n2"] = len(p2)
    r.update(side_features(f, p1, "t1_")); r.update(side_features(f, p2, "t2_"))
    rows.append(r)
    record_players(i, f, p1, p2)
    # ---------------- update state with this match ----------------
    s, e = starts[i], ends[i]
    if e > s:
        sl = slice(s, e)
        side = B_side[sl]; over = B_over[sl]; legal = B_legal[sl]
        rb = B_rb[sl]; rt = B_rt[sl]; wk = B_wk[sl]; bwk = B_bwk[sl]; bc = B_bc[sl]; wc = B_wc[sl]
        exp_r = ctx_runs[f, over] / ctx_balls[f, over]
        exp_w = ctx_wk[f, over] / ctx_balls[f, over]
        # batting: runs above expectation (batter runs vs total-ball expectation), dismissals below expectation
        rae = rb - exp_r; wae_bat = exp_w - wk
        ACT.append((i, np.unique(bc, return_counts=True), np.bincount(np.unique(bc, return_inverse=True)[1], weights=rb), np.unique(wc, return_counts=True), np.bincount(np.unique(wc, return_inverse=True)[1], weights=bwk)))
        rse = exp_r - rt; wae_bowl = bwk - exp_w
        # aggregate per player for this match
        for arr_sum, arr_balls, arr_w, arr_m, who, val, wval, cnt in (
            (bat_rae, bat_balls, bat_wae, bat_matches, bc, rae, wae_bat, np.ones_like(rb)),
            (bowl_rse, bowl_balls, bowl_wae, bowl_matches, wc, rse, wae_bowl, np.ones_like(rb)),
        ):
            u, inv = np.unique(who, return_inverse=True)
            arr_sum[f, u] *= DECAY; arr_balls[f, u] *= DECAY; arr_w[f, u] *= DECAY; arr_m[f, u] *= DECAY
            arr_sum[f, u] += np.bincount(inv, weights=val, minlength=len(u))
            arr_balls[f, u] += np.bincount(inv, weights=cnt, minlength=len(u))
            arr_w[f, u] += np.bincount(inv, weights=wval, minlength=len(u))
            arr_m[f, u] += 1.0
        # context baselines
        np.add.at(ctx_balls[f], over, 1.0); np.add.at(ctx_runs[f], over, rt); np.add.at(ctx_wk[f], over, wk)
        # venue first-innings runs (limited overs only)
        if f != 3:
            inn0 = B_inn[sl] == 0
            venue_runs1[(m.fmt, v)][0] += rt[inn0].sum(); venue_runs1[(m.fmt, v)][1] += 1
    # keeper proxy: stumpings credited via 'stumped' -> we lack fielder ids here; approximate using
    # lineups: mark players who never bowl and ... skipped; use stumpings from kind (not stored). Keep 0.
    all_p = np.concatenate([p1, p2])
    career_matches[f, all_p] += 1; career_matches_all[all_p] += 1
    team_venue[(t1, v)] += 1; team_venue[(t2, v)] += 1
    # results
    if not np.isnan(r["y"]):
        y = r["y"]
        exp1 = elo_exp(e1, e2)
        telo[key1] = e1 + K_TEAM * (y - exp1); telo[key2] = e2 + K_TEAM * ((1 - y) - (1 - exp1))
        team_hist[key1].append(y); team_hist[key2].append(1 - y)
        h2h[(m.fmt, t1, t2)].append(y); h2h[(m.fmt, t2, t1)].append(1 - y)
        venue_bf[(m.fmt, v)][0] += y; venue_bf[(m.fmt, v)][1] += 1
        pr1, pr2 = pelo[f, p1].mean(), pelo[f, p2].mean()
        pexp = elo_exp(pr1, pr2)
        pelo[f, p1] += K_PLAYER * (y - pexp); pelo[f, p2] += K_PLAYER * ((1 - y) - (1 - pexp))
    elif m.result in ("tie", "draw"):
        team_hist[key1].append(0.5); team_hist[key2].append(0.5)
    if i % 2000 == 0: print(i, m.date.date(), flush=True)

df = with_differentials(pd.DataFrame(rows))
df.to_pickle("features.pkl")
pd.concat(PLAYER_ROWS, ignore_index=True).to_pickle("player_rows.pkl")
act = []
for (mo_, (ub, cb), rb_, (uw, cw), wk_) in ACT:
    act.append(pd.DataFrame({"mo": mo_, "pc": ub, "balls_faced": cb, "runs": rb_}).merge(pd.DataFrame({"mo": mo_, "pc": uw, "balls_bowled": cw, "wkts": wk_}), how="outer", on=["mo", "pc"]))
pd.concat(act, ignore_index=True).fillna(0).to_pickle("actuals.pkl")
with open("state.pkl", "wb") as fh:
    pickle.dump(dict(all_pids=list(all_pids), morder=morder), fh)
print(df.shape); print(df.groupby("fmt").y.agg(["count", "mean"]))
