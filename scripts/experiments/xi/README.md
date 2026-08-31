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

`freeze_ratings.py` is the exception: it reads Postgres, not the JSON, and rewrites
`output/ml-service/xi_ratings.joblib` with a rating state that stops at a date. The serving
store holds ratings through today, which is what a live prediction wants and what a
*backtest* must not have — the S-3b selection comparison would otherwise score matches with
ratings that already contain their results. Run it from `ml-service/` with `POSTGRES_*` set:

    PYTHONPATH=. .venv/bin/python ../scripts/experiments/xi/freeze_ratings.py \
        --before 2025-09-01 --out ../output/ml-service

Keep a copy of the full-history artifact first; restore it and `POST /admin/reload` afterwards.
An as-of serving path belongs in the L4 harness (re-architecture plan, P-2), which retires this.
