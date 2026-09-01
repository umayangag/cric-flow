# XI experiments (S-9 / re-architecture plan)

Standalone, leakage-free-by-construction experiments over the raw Cricsheet JSON. They back
the numbers in `docs/WIN_PROB_SELECTION_PR_CHECKLIST.md` (S-9 results) and
`docs/ML_PIPELINE_REARCHITECTURE_PLAN.md` (§1, appendix). They are research scripts, not part
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
