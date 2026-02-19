# ML-Service Dead / Outdated Code Analysis

Summary of dead or outdated code identified in the ml-service codebase that can be safely removed or cleaned up without impacting current functionality.

---

## 1. **Removed: Broken export_pool script and its sole dependency**

**Files removed:**
- `ml/export_pool.py`
- `ml/fill_missing_attributes.py`

**Reason:** `export_pool.py` is **unrunnable** in the current tree:

- It uses script-relative imports (`from batting_regressor import ...`, `from match_data import ...`) that assume either running from `ml/` with `ml` on `PYTHONPATH`, or a top-level `match_data` module. There is **no `match_data` module** in ml-service; the only `match_data` lived in the removed `src/team_selection/shared/match_data.py` (legacy prototype).
- The Makefile runs `$(PY) ml/export_pool.py $(MATCH)` from the ml-service root, so imports resolve from the project root and fail (e.g. `ModuleNotFoundError: No module named 'batting_regressor'` or, if that were fixed, `No module named 'match_data'`).
- `fill_missing_attributes.py` is **only** used by `export_pool.py` within ml-service; no other ml-service code or tests reference it.

Team prediction in production uses the **go-app pipeline**: go-app builds the pool from DB, fetches features, calls the ML service `/predict/batting` and `/predict/bowling`, then runs team selection. The root Makefile’s `team-predictor` target had been running `export_pool` before `go-app` team-predictor; that step has been removed so the target no longer depends on the broken script.

**Impact:** None on the API or on go-app. Pool generation for team prediction is done by go-app (DB + ML predict endpoints). If a standalone pool CSV is needed in future, it can be re-added using the ML service HTTP API and go-app data sources.

---

## 2. **Verified as used (not dead)**

| Symbol / area | Used by |
|---------------|--------|
| Legacy artifact loading (`_LEGACY_`, `_load_legacy`) | `app/artifacts.py`, `app/main.py` (predict fallback), tests |
| Legacy CSV paths (e.g. `batting_encoded.csv`) | `train_batting.py`, `train_bowling.py`, `train_*_model.py`, `validate_exports.py` |
| `validate_exports.py` | Makefile (`validate-exports`, `validate-exports-infer`), docs, CI-related docs |
| `walk_forward`, `train_combination_meta` | Makefile / docs |
| `calibrate` (calibrate_classifier, reliability_diagram_data, evaluate_calibration) | `tests/test_calibrate.py` |
| `ml.tracking`, `ml.db`, `ml.config`, `ml.resources` | `app/main.py`, `ml/walk_forward.py`, `ml/train_*.py`, etc. |

---

## 3. **Summary**

- **Done:** Removed broken `ml/export_pool.py` and its only consumer dependency `ml/fill_missing_attributes.py`; removed the `export-pool` Makefile target and root Makefile’s dependency on it for `team-predictor`.
- **Optional:** None. No remaining optional dead-code cleanups identified for ml-service at this time.
