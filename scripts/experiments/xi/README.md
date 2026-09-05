# XI experiments (re-architecture plan)

Standalone, leakage-free-by-construction experiments over the raw Cricsheet JSON. They back
the numbers in `docs/ML_PIPELINE_REARCHITECTURE_PLAN.md` (§1, §5, §8 and the appendix). They are research scripts, not part
of the service; the production implementation of the same pass is `ml-service/ml/xi/`.

Run from a scratch directory with the JSON files unzipped under `raw/` (paths at the top of
`parse.py`), Python 3.10+, numpy / pandas / scikit-learn / scipy:

    python parse.py            # -> matches.pkl, lineups.pkl, balls_parts/
    python build_features.py   # -> features.pkl, player_rows.pkl, actuals.pkl (chronological as-of pass)
    python evaluate.py         # feature-family ablation, held-out AUC per format, 3 seeds
    python diagnostics.py      # swap responsiveness, marginal value vs actual performance, cold start
    python diagnostics2.py     # specific XI vs typical XI; swap split by role
    python diagnostics3.py     # monotone objective: unconstrained vs monotone GBM vs logistic
    python perf_experiment.py  # player-performance predictability benchmark (plan §1)

`perf_choices.py` is different from the rest: it runs against the production pass
(`ml-service/ml/xi/`), not the research parse, and makes P-3's modelling choices on the
walk-forward folds — the hyperparameter grid, direct vs two-part structure, E1 (sequence
families) and E6 (T20 + T20I transfer) — writing one JSON with the tables the plan quotes:

    python perf_choices.py --cricsheet-dir ../../../data/go-app/cricsheet --cache frame.pkl --out choices.json

`freeze_ratings.py` used to live here as a one-off that rewrote the serving artifact with a
rating state frozen at a date, because a backtest must not score matches with ratings that
already contain their results. P-2 retired it: the serving path answers "ratings as of
date D" itself (`ml-service/ml/xi/asof.py`; `/xi/*` take an `as_of` date), and the L4
harness (`make xi-evaluate`) proves the as-of rows equal the training frame's (H-8).

`e5_natural_experiment.py` produced plan §8.6's E5 figures over HTTP against the serving
artifact, and stays as that record. `e5_reproduce.py` is the wiring test for the metric's home
in the harness (`ml-service/ml/xi/natural_experiment.py`): the same pairs, the same as-of path
and one run's objective, expected to land on §8.6's development figures (T20 0.509, ODI 0.521,
T20I 0.589); `make evaluate` is where the walk-forward figure and its derived bar come from.
`e3_batting_order.py` runs E3 (plan §5): for sampled fixtures' fielded elevens, N random orders
of the top seven re-predicted through L2-B and simulated batting first with common random
numbers, the best re-simulated with a fresh seed, and the share of elevens whose confirmed
median total moves by more than 3 %. It states its gate's varies / fixed / decides triple
(`ml.xi.gates`, H-23) before it runs:

    python scripts/experiments/xi/e5_reproduce.py --run output/ml-service/runs/<id>
    python scripts/experiments/xi/e3_batting_order.py --postgres --out output/e3_batting_order.json

`a1_fixture_context.py` is gate A-1 (follow-up plan, `docs/ML_PIPELINE_REARCHITECTURE_PLAN.md`
§8.9): on the walk-forward folds, four arms of the performance model -- reading none, the
venue, the competition or both fixture-context families -- with everything else fixed, and per
arm the simulated first-innings bias, 10-90 coverage and width, and the pinball per target.
It runs on `sim_frame_cache.py`'s frames, one format per invocation, and `--decide` prints the
per-family table and the cross-format verdict the plan records:

    python scripts/experiments/xi/sim_frame_cache.py --cricsheet-dir data/go-app/cricsheet --out output/ml-service/a1/frames.pkl
    python scripts/experiments/xi/a1_fixture_context.py --frames output/ml-service/a1/frames.pkl --format T20 --out output/ml-service/a1/a1_T20.json
    python scripts/experiments/xi/a1_fixture_context.py --decide output/ml-service/a1/a1_T20.json output/ml-service/a1/a1_ODI.json

`a2_chase_tails.py` is gate A-2 (plan §8.10): on the same folds, four chase responses the
simulator applies to the chasing side's runs draws -- `none`, the `level` control, `slope`
and `both` -- from one L2-B fit per fold and one fitted calibration sample, with the
display models, the shared factor and the simulator's random numbers shared by the arms.
Per arm it records the chase 10-90 coverage, bias and width, the below-q10 / above-q90
shares, the first-innings coverage and width, E2's Brier delta, the margins and the fitted
coefficients; `--decide` prints the table, the paired effect sizes against their fold-level
standard errors and which arm `simulator.CHASE_RESPONSE` should name:

    python scripts/experiments/xi/a2_chase_tails.py --frames output/ml-service/a1/frames.pkl --format T20 --out output/ml-service/a2/a2_T20.json
    python scripts/experiments/xi/a2_chase_tails.py --decide output/ml-service/a2/a2_T20.json output/ml-service/a2/a2_ODI.json output/ml-service/a2/a2_T20I.json

`a3_t20_lineup_signal.py` is gate A-3 (plan §8.11): can a feature family make the T20 selection
objective select? Three arms of the objective -- `none`, `phase_matchup` (a) and `role_balance`
(b) -- refitted per fold on `XI_FEATURE_COLS` plus the family's columns, with E5's pairs, the
previous elevens (one as-of pass, cached) and the bar's derivation fixed; per arm and format
the fold objective AUC and swap-violation share (the guard) and E5 per fold and pooled with the
bar re-derived from the arm's own claimed effect under three seeds. Family (c) -- E5 reweighted
by the claimed |Δ| -- is a measurement diagnostic computed on every arm and never the verdict.
Every eleven is rebuilt from per-player as-of vectors (parity against the frame 0.0), so the
script runs in minutes on `sim_frame_cache.py`'s frames; `--decide` prints the tables and the
verdict:

    python scripts/experiments/xi/a3_t20_lineup_signal.py --frames output/ml-service/a1/frames.pkl --cricsheet-dir data/go-app/cricsheet --pairs-cache output/ml-service/a3/pairs.pkl --out output/ml-service/a3/a3.json
    python scripts/experiments/xi/a3_t20_lineup_signal.py --decide output/ml-service/a3/a3.json

`x1b_biography_features.py` is X-1b's two runnable gates (`docs/EXTERNAL_DATA_PLAN.md` § X-1b,
plan §8.12), on `sim_frame_cache.py`'s frames built with the archive's dates of birth
(`--birth-dates`, the CSV `make export-birth-dates` writes). Family 1, gate `X-1b-age`: two
arms of the performance model per fold, without and with `AGE_COLS`, decided by the pinball
of runs or wickets beyond E1's 0.5 % band and one fold-level standard error with coverage
held, T20 on men's rows (women's T20 is out of scope at 61.4 % coverage). Family 3, gate
`X-1b-cold-start`: two passes -- the age-band debut prior off and on (`--age-aware-cold-start`
on the cache) -- with the win models, H-4, the H-10 debutant-swap probe and a runs + wickets
fit per fold and arm, decided by H-10 staying bounded, the debut rows' pinball, the check
that no player with history moved, and a display-AUC / H-4 guard. Every fold is written as
it is scored and a re-run resumes; `--decide <family>` prints the tables and the verdict:

    python scripts/experiments/xi/sim_frame_cache.py --cricsheet-dir data/go-app/cricsheet --birth-dates data/go-app/player-birth-dates.csv --out output/ml-service/x1b/frames_off.pkl
    python scripts/experiments/xi/sim_frame_cache.py --cricsheet-dir data/go-app/cricsheet --birth-dates data/go-app/player-birth-dates.csv --age-aware-cold-start --out output/ml-service/x1b/frames_on.pkl
    python scripts/experiments/xi/x1b_biography_features.py --family age --frames output/ml-service/x1b/frames_off.pkl --format T20 --out output/ml-service/x1b/age_T20.json
    python scripts/experiments/xi/x1b_biography_features.py --family cold-start --frames output/ml-service/x1b/frames_off.pkl --frames-on output/ml-service/x1b/frames_on.pkl --format T20 --out output/ml-service/x1b/cold_T20.json
    python scripts/experiments/xi/x1b_biography_features.py --decide age output/ml-service/x1b/age_T20.json output/ml-service/x1b/age_ODI.json

`x3_match_stakes.py` is X-3 (`docs/EXTERNAL_DATA_PLAN.md`): two gates, plus the coverage
figures that qualify both. The derivation itself is not here — it is
`ml-service/ml/xi/stakes.py`, inside the rating pass on both sources — so `--coverage` only
measures what the labels reach, per format and gender, beside the stage vocabulary the
archive actually spells. Gate `X-3-e5` re-runs E5's lineup-only agreement with the pairs
whose matches were dead rubbers or knockouts excluded and, separately, halved, the bar
re-derived on each arm's own weights under three seeds; it informs and decides nothing. Gate
`X-3-stakes` refits the display model per fold with `STAKES_COLS` added, against the same
folds without them, on display AUC beyond the seed noise with the swap-violation share held.
Both run on `sim_frame_cache.py`'s frames and one as-of pass cached in `--pairs-cache`:

    python scripts/experiments/xi/x3_match_stakes.py --coverage --cricsheet-dir data/go-app/cricsheet --out output/ml-service/x3/coverage.json
    python scripts/experiments/xi/x3_match_stakes.py --frames output/ml-service/x3/frames.pkl --cricsheet-dir data/go-app/cricsheet --pairs-cache output/ml-service/x3/pairs.pkl --out output/ml-service/x3/x3.json
    python scripts/experiments/xi/x3_match_stakes.py --decide output/ml-service/x3/x3.json --coverage-json output/ml-service/x3/coverage.json
