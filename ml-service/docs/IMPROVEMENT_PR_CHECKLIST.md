# ML Service improvement PR checklist

Tracked improvements from the architecture review (May 2026). Implement **one PR at a time**; mark status here as work progresses.

**Status legend:** `todo` | `in_progress` | `done` | `skipped`

| ID | Status | PR branch (when done) | Title |
|----|--------|----------------------|-------|
| P4-4 | done | `ml-service/p4-4-readme-endpoints` | Update README with current endpoints and layout |
| P2-5 | done | `ml-service/p2-5-remove-main-test-reexports` | Remove test re-exports from `main.py` |
| P2-4 | done | `ml-service/p2-4-win-features-public-api` | Stop importing private `ml` symbols; add public helpers |
| P1-4 | todo | | Remove stale `ImportError` fallbacks in `prediction_service` |
| P0-2 | todo | | Split `app/models.py` by domain |
| P0-3 | todo | | Split `prediction_service.py` into focused modules |
| P0-4 | todo | | Split `main.py` into FastAPI routers |
| P0-1 | todo | | Introduce shared `contracts` package (break `ml` ↔ `app` cycle) |
| P0-5 | todo | | Fold `ml_service/` into `ml/` (datasets, baselines) |
| P1-1 | todo | | Artifact store with safe reload under concurrency |
| P1-2 | todo | | Run CPU-heavy routes via thread pool / `to_thread` |
| P1-3 | todo | | Replace `urllib` with `httpx` in `train_on_the_fly` |
| P2-1 | todo | | Expand ruff rules (phased) |
| P2-2 | todo | | Add mypy/pyright in CI (phased) |
| P2-3 | todo | | Standardize structlog in `ml/` |
| P3-1 | todo | | Decompose `ml/config.py` |
| P3-2 | todo | | Consolidate training entrypoints |
| P3-3 | todo | | Document reconciliation layers |
| P3-4 | todo | | Coverage/smoke tests for omitted training paths |
| P4-1 | todo | | Tighten CORS defaults |
| P4-2 | todo | | API versioning (`/v1/...`) — **skipped unless cross-repo approved** |
| P4-3 | todo | | Trusted artifact directory validation for joblib |

---

## P0 — Architecture & boundaries

### P0-1 — Shared `contracts` package

- **Why:** `ml` imports `app.models`; `app` imports `ml.*`. Contracts belong in a neutral layer.
- **Scope:** New package; move Pydantic DTOs used by both; update imports.
- **Risk:** Medium

### P0-2 — Split `app/models.py`

- **Why:** ~735 lines mixing predict, backtest, reconciliation models.
- **Scope:** `app/models/` package with `predict.py`, `backtest.py`, `reconciliation.py`; re-export from `__init__.py` during transition.
- **Risk:** Low

### P0-3 — Split `prediction_service.py`

- **Why:** ~1.3k lines; hardest module to change safely.
- **Scope:** `player_predict`, `match_predict`, `extras_win`, `batch` modules.
- **Risk:** Medium

### P0-4 — Split `main.py` into routers

- **Why:** ~900 lines; routes, middleware, admin, cache in one file.
- **Scope:** `app/routes/health.py`, `predict.py`, `backtest.py`, `admin.py`; register in `main.py`.
- **Risk:** Medium

### P0-5 — Unify `ml_service/` into `ml/`

- **Why:** Three top-level names; `ml_service` not copied in Docker.
- **Scope:** `ml/datasets`, `ml/baselines`; update tests and imports.
- **Risk:** Low–medium

---

## P1 — Runtime correctness & performance

### P1-1 — Artifact store abstraction

- **Why:** Global registries + hot reload without explicit concurrency control.
- **Scope:** Immutable snapshot or lock around `reload()`.
- **Risk:** Medium

### P1-2 — Non-blocking CPU-heavy handlers

- **Why:** Sync `def` backtest/predict routes block the event loop.
- **Scope:** `asyncio.to_thread` or executor for sklearn/numpy paths.
- **Risk:** Low–medium

### P1-3 — `httpx` in `train_on_the_fly`

- **Why:** `httpx` already a dependency; better timeouts/retries than `urllib`.
- **Scope:** `train_on_the_fly.py` + tests.
- **Risk:** Low

### P1-4 — Remove `ImportError` fallbacks in `prediction_service`

- **Why:** Dead paths hide production misconfiguration.
- **Scope:** Delete stubs; ensure CI always installs `ml`.
- **Risk:** Low

---

## P2 — Code quality & standards

### P2-1 — Expand ruff rules

- **Why:** Only `F` + `I` today.
- **Scope:** `pyproject.toml` + phased fixes (`E`, `W`, `B`, `UP`, …).
- **Risk:** Medium if done in one shot

### P2-2 — Static typing in CI

- **Why:** Many `Any` and `type: ignore`.
- **Scope:** mypy or pyright on `app/` + core `ml/`.
- **Risk:** Medium

### P2-3 — Structlog in `ml/`

- **Why:** `app` uses structlog; `ml` mostly stdlib logging.
- **Scope:** Adapter + key train/config modules.
- **Risk:** Low–medium

### P2-4 — Public APIs for private imports

- **Why:** `_format_one_hot_from_code` imported across packages.
- **Scope:** Public helpers in `ml/win_features.py`.
- **Risk:** Low

### P2-5 — Remove test re-exports from `main.py`

- **Why:** `# noqa: F401` coupling for tests.
- **Scope:** Tests import from real modules.
- **Risk:** Low

---

## P3 — ML / config maintainability

### P3-1 — Decompose `ml/config.py`

- **Why:** ~730 lines of getters and defaults.
- **Scope:** Submodules + thin facade.
- **Risk:** Medium

### P3-2 — Consolidate training entrypoints

- **Why:** `train_batting` vs `train_batting_model` duplication.
- **Scope:** Single documented entry path.
- **Risk:** Medium

### P3-3 — Reconciliation documentation

- **Why:** Hybrid rescale (`app/reconciliation`) vs integer solver (`ml/reconciliation_service`) unclear.
- **Scope:** Module docstrings + short doc section.
- **Risk:** Low

### P3-4 — Training path coverage

- **Why:** Coverage omits `train_*`, `tuning/*`.
- **Scope:** Targeted smoke tests for critical paths.
- **Risk:** Low–medium

---

## P4 — API, security, ops

### P4-1 — Tighten CORS

- **Why:** `allow_methods=["*"]`, `allow_headers=["*"]` is permissive.
- **Scope:** `settings.py` + config.
- **Risk:** Low

### P4-2 — API versioning

- **Why:** Safer HTTP evolution.
- **Scope:** `/v1/...` + go-app client — **cross-repo; confirm before starting**.
- **Risk:** High

### P4-3 — Trusted artifact directory

- **Why:** joblib pickle deserialization risk.
- **Scope:** Startup validation / documented constraint.
- **Risk:** Low

### P4-4 — Update README

- **Why:** Endpoint list and layout description are stale.
- **Scope:** `ml-service/README.md` only.
- **Risk:** Low

---

## Recommended order

1. P4-4 → P2-5 → P2-4 → P1-4 (low risk)
2. P0-2 → P0-3 → P0-4 → P0-1 → P0-5 (structure)
3. P1-1 → P1-2 → P1-3 (runtime)
4. P2-1 → P2-2 → P2-3 (quality gates)
5. P3-* → P4-1 → P4-3 (config/docs/security)
6. P4-2 only if explicitly approved
