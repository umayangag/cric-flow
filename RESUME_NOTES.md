# RESUME_NOTES — feat/sim-day-night-calibration

Working notes for the simulator day/night calibration experiment. Delete before the PR is
merged; kept on the branch so a capped run can be picked up without re-measuring.

## Where things are

- Branch `feat/sim-day-night-calibration`, off main `e0ee4ce5`.
- venv: `ml-service/.venv` (python 3.12, `pip install -r ml-service/requirements.txt` plus
  `pytest pytest-cov ruff`).
- Frames cache (rating pass on current main, 93 s to build):
  `output/ml-service/daynight/frames.pkl` — 465,402 player rows, 21,096 matches.
  Rebuild: `scripts/experiments/xi/sim_frame_cache.py --cricsheet-dir
  /Users/umayanga/go/src/cric-flow/data/go-app/cricsheet --out <path>`.
- Experiment: `scripts/experiments/xi/daynight_dispersion.py` (arms control / split / scale).
- Gates registered: `SIM-DN-split`, `SIM-DN-scale` in `ml-service/ml/xi/gates.py`.
- Run outputs: `output/ml-service/daynight/dn_T20.json`, `dn_ODI.json` (a re-run resumes
  from the file, fold by fold).

## Database counts (verified 2026-09-06, unchanged, nothing written)

`cricket_data`: 22,818 matches (`match`), 11,539,808 ball events (`ball_event`),
13,662 `player_biography` rows.

## Feasibility probe (before any harness run) — day/night counts per fold

Labels from `ml.weather.sessions` over 22,818 archive matches: 35.2 % night, matching X-2.

| format | night share | calibration night per fold | eval night per fold |
|---|---|---|---|
| T20 | 0.434 | 43–179 (all ≥ 30) | 74–200 (all ≥ 20) |
| ODI | 0.300 | 0, 2, 6, 6, 7, 7, 9, 11, 13, 22, 22, 43 | 0, 2, 2, 5, 6, 7, 8, 13, 14, 23, 24 |
| T20I | 0.512 | 6–34 | 6–35 |

**Consequence, decided before running:** T20 is the only deciding format. ODI's night side
clears the harness's 20-match floor in **2 of 11** folds and its night calibration fold
clears the 30-match deconvolution guard in **1** (never in the same fold), so ODI cannot
decide either candidate and is reported only. This also bounds X-2's ODI night figures
(0.932 coverage, 0.749 dispersion): they are the mean of **2 folds**, ~24 matches each.

## Measured so far

(nothing yet — see the fold tables below as they land)

### T20

_pending_

### ODI

_pending_
