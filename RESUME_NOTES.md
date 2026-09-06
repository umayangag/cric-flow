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

### T20 — 11 folds, 1000 draws, done (25 min)

**The gap reproduces on current main to three decimals**: control first-innings coverage
0.734 day / 0.841 night (nominal 0.80), width 78.4 / 96.3, dispersion 1.099 / 0.865, pooled
0.771 / 84.7 — every figure equal to X-2's, so #267 dropping `t1_pelo_std` changed nothing here.

| arm | pop | matches/fold | first cov | first width | dispersion | first bias | chase cov | chase width | Δ Brier |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| control | all | 399 | 0.771 | 84.7 | — | +0.5 | 0.712 | 69.9 | +0.0019 |
| control | day | 259 | 0.734 | 78.4 | 1.099 | +1.8 | 0.689 | 64.6 | +0.0024 |
| control | night | 139 | 0.841 | 96.3 | 0.865 | −2.1 | 0.759 | 79.8 | +0.0002 |
| split | all | 399 | 0.756 | 82.0 | — | +0.4 | 0.692 | 67.2 | +0.0017 |
| split | day | 259 | 0.756 | 83.9 | 1.026 | +1.0 | 0.713 | 69.2 | +0.0023 |
| split | night | 139 | 0.759 | 78.7 | 1.082 | −0.8 | 0.659 | 63.6 | −0.0004 |
| scale | all | 399 | 0.754 | 81.6 | — | +0.6 | 0.698 | 66.8 | +0.0015 |
| scale | day | 259 | 0.760 | 83.6 | 1.027 | +1.7 | 0.726 | 69.2 | +0.0022 |
| scale | night | 139 | 0.748 | 78.2 | 1.076 | −1.6 | 0.655 | 62.8 | −0.0003 |

Paired per fold (arm − control), one fold-level s.e. as the floor:

| arm | first cov \|Δ→0.80\| day | night | dispersion \|Δ→1\| day | night | chase cov \|Δ→0.80\| day | night | Δ E2 | pooled width |
|---|---|---|---|---|---|---|---|---|
| split | −0.0113 ± 0.0072 | −0.0130 ± 0.0162 | −0.021 ± 0.020 | −0.069 ± 0.044 | −0.0240 ± 0.0057 | **+0.0829 ± 0.0156** | −0.00023 ± 0.00018 | 84.7 → 82.0 |
| scale | −0.0160 ± 0.0075 | −0.0118 ± 0.0189 | −0.020 ± 0.019 | −0.072 ± 0.043 | −0.0352 ± 0.0049 | **+0.0873 ± 0.0150** | −0.00042 ± 0.00024 | 84.7 → 81.6 |

Both fail, identically: `night_first_coverage_moves_to_nominal`,
`night_chase_coverage_no_worse`, `e2_unchanged`. Passing: both day clauses, both dispersion
clauses, `pooled_width_not_inflated` (the pooled width *falls*, 84.7 → 82.0 / 81.6).

Per-fold factor spreads (split arm): pooled 0.103–0.224, day 0.124–0.257, night
**0.000**–0.172 — the 2025-01 fold's night group deconvolves to exactly zero excess variance,
i.e. no shared factor at all for its night matches.

### ODI — running

_pending_
