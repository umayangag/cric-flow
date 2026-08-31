"""Parse Cricsheet JSON into compact parquet tables: matches, lineups, balls.

Format taxonomy mirrors go-app/internal/cricsheet/format.go so results are comparable
with the repo's win export: TEST (Test/MDM), ODI (ODI/ODM), T20I (T20I/IT20, or T20 between
two configured international teams), T20 (everything else T20).
"""
import glob, json, os, sys
import numpy as np, pandas as pd
from multiprocessing import Pool

INTERNATIONAL = {t.lower() for t in ["Afghanistan","Australia","Bangladesh","England","India","Ireland",
    "New Zealand","Pakistan","South Africa","Sri Lanka","West Indies","Zimbabwe"]}

def detect_format(mt, teams):
    mt = mt.upper()
    if mt in ("TEST","MDM"): return "TEST"
    if mt in ("ODI","ODM"): return "ODI"
    if mt in ("T20I","IT20"): return "T20I"
    if mt == "T20":
        if sum(t.lower() in INTERNATIONAL for t in teams) >= 2: return "T20I"
        return "T20"
    return ""

def parse_one(path):
    try:
        d = json.load(open(path))
    except Exception as e:
        return None
    info = d["info"]; mid = os.path.basename(path).rsplit(".",1)[0]
    teams = info["teams"]
    fmt = detect_format(info["match_type"], teams)
    if not fmt or len(teams) != 2: return None
    reg = info.get("registry", {}).get("people", {})
    outcome = info.get("outcome", {})
    winner = outcome.get("winner")
    result = outcome.get("result")  # 'no result' / 'tie' / 'draw'
    # team batting first = innings[0].team
    inns = d.get("innings", [])
    if not inns: return None
    bat_first = inns[0]["team"]
    if bat_first not in teams: return None
    other = teams[1] if teams[0] == bat_first else teams[0]
    m = dict(match_id=mid, date=info["dates"][0], fmt=fmt, match_type=info["match_type"],
             gender=info.get("gender"), team_type=info.get("team_type"),
             team1=bat_first, team2=other, venue=info.get("venue"), city=info.get("city"),
             event=(info.get("event") or {}).get("name"),
             toss_winner=(info.get("toss") or {}).get("winner"), toss_decision=(info.get("toss") or {}).get("decision"),
             winner=winner, result=result, method=outcome.get("method"),
             overs=info.get("overs"), balls_per_over=info.get("balls_per_over", 6))
    lineups = []
    for t, pl in info.get("players", {}).items():
        side = 1 if t == bat_first else 2
        for name in pl:
            lineups.append((mid, side, reg.get(name, "name:" + name), name))
    balls = []
    for ino, inn in enumerate(inns):
        side = 1 if inn["team"] == bat_first else 2
        seq = 0
        for ov in inn.get("overs", []):
            onum = ov["over"]
            for b in ov.get("deliveries", []):
                ex = b.get("extras", {})
                legal = not ("wides" in ex or "noballs" in ex)
                wk = b.get("wickets") or []
                # a wicket credited to the bowler (not run out / retired / obstructing)
                bowler_wk = 0; any_wk = 0; kind = ""
                for w in wk:
                    kind = w["kind"]; any_wk = 1
                    if kind in ("caught","bowled","lbw","caught and bowled","stumped","hit wicket"): bowler_wk = 1
                balls.append((mid, ino, side, onum, seq, legal,
                              reg.get(b["batter"], "name:"+b["batter"]), reg.get(b["bowler"], "name:"+b["bowler"]),
                              b["runs"]["batter"], b["runs"]["extras"], b["runs"]["total"], any_wk, bowler_wk, kind,
                              ex.get("wides",0)+ex.get("noballs",0)))
                seq += 1
    return m, lineups, balls

if __name__ == "__main__":
    files = sorted(glob.glob("/home/claude/xi/raw/*.json"))
    print(len(files), "files", flush=True)
    os.makedirs("balls_parts", exist_ok=True)
    M, L, B, part = [], [], [], 0
    cols = ["match_id","inn","side","over","seq","legal","batter","bowler",
            "runs_bat","runs_extra","runs_total","wicket","bowler_wicket","kind","wide_nb"]
    def flush():
        global B, part
        if not B: return
        pd.DataFrame(B, columns=cols).to_pickle(f"balls_parts/part{part:03d}.pkl"); part += 1; B = []
    with Pool(2) as p:
        for i, r in enumerate(p.imap_unordered(parse_one, files, chunksize=64)):
            if r is None: continue
            m, l, b = r; M.append(m); L.extend(l); B.extend(b)
            if len(B) > 1_500_000: flush(); print(i, "flushed", flush=True)
    flush()
    matches = pd.DataFrame(M); matches["date"] = pd.to_datetime(matches["date"])
    matches = matches.sort_values(["date","match_id"]).reset_index(drop=True)
    lineups = pd.DataFrame(L, columns=["match_id","side","pid","name"])
    matches.to_pickle("matches.pkl"); lineups.to_pickle("lineups.pkl")
    print(matches.shape, lineups.shape, part, "ball parts")
    print(matches.fmt.value_counts()); print(matches.result.value_counts(dropna=False))
