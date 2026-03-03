# Configuration Semantics for `cric-flow`

This document describes how configuration is resolved across the Go API, ML service, and related tooling. It is the single reference for precedence, key environment variables, and defaulting behavior.

---

## 1. Precedence

- **Go API (`go-app`)**
  - **Config file path**: If `GO_APP_CONFIG` is set, that path is read first. If unset, the loader looks for `config.json` in the current directory, then `../config.json`, then `../../config.json`. The first existing file is used.
  - **Overrides**: Some values are overridden by environment variables even when a config file is present:
    - `PORT` overrides server listen port.
    - `GO_APP_OUTPUT_DIR` overrides outputs (e.g. export dir) when set.
  - So effectively: **env overrides (PORT, GO_APP_OUTPUT_DIR) > config file (from GO_APP_CONFIG or discovered config.json) > built‑in defaults**.

- **ML service (`ml-service`)**
  - Configuration is **environment-only** (no config file). `app/settings.py` and `load_ml_service_settings()` read from `os.environ`. Precedence for directories: **env vars (e.g. ML_SERVICE_OUTPUT_DIR, MODELS_DIR) > service config default_artifacts_dir() > built‑in fallback** (e.g. `../../output/ml-service`).

- **CLI / scripts**
  - Where a CLI accepts flags and also reads env/config, flags typically override env and config (not fully unified across all CLIs; see per-component docs).

---

## 2. Go API – Key Environment Variables

| Variable | Role | Default / behavior |
|----------|------|--------------------|
| `GO_APP_CONFIG` | Path to JSON config file. When set, only this file is used; when unset, `config.json` is discovered. | Unset → discover `config.json` |
| `GO_APP_OUTPUT_DIR` | Overrides output/export directory. | From config file or default in config |
| `PORT` | Overrides server listen port. | From config or e.g. `:8080` |
| `RUN_MIGRATIONS_AT_STARTUP` | If `0`, `false`, or `no`, migrations are not run on API startup. | `1` (migrations run) |
| `RUN_TRACKING_RECONCILIATION_AT_STARTUP` | If `0`, `false`, or `no`, stale run reconciliation is skipped on startup. | `1` (reconciliation runs) |
| `TRACKING_STALE_CANCEL_AGE` | Duration after which an IN_PROGRESS run is considered stale and cancelled on reconciliation (e.g. `24h`, `30m`). | `24h` |

Database URL, ML base URL, and other server options are typically read from the config file (or defaults in code when the file is missing).

---

## 3. ML Service – Key Environment Variables

| Variable | Role | Default / behavior |
|----------|------|--------------------|
| `ML_SERVICE_OUTPUT_DIR` | First choice for models/artifacts directory. | — |
| `MODELS_DIR` | Second choice for models directory if `ML_SERVICE_OUTPUT_DIR` is unset. | — |
| `ENABLE_HOT_RELOAD` | Enable hot reload of artifacts. | `false` |
| `ADMIN_API_KEY` | API key for admin endpoints. | `""` |
| `MAX_CONCURRENT_TRAINING_JOBS` | Max concurrent training jobs. | `1` |
| `MAX_PREDICT_BATCH_SIZE` | Max batch size for predict. | `10000` |
| `DISABLE_BACKTEST_CACHE` | Disable backtest prediction cache. | `false` |
| `BACKTEST_CACHE_TTL` | Backtest cache TTL in seconds. | `300` |
| `TRAIN_ON_THE_FLY_LATEST_CACHE_GRANULARITY` | Cache granularity for “latest” model (e.g. `hour`). | `hour` |
| `MODEL_STATS_CACHE_TTL` | Model stats cache TTL in seconds. | `60` |
| `FRONTEND_ORIGIN` | Allowed frontend origin for CORS. | `http://localhost:5173` |

Models directory precedence: `ML_SERVICE_OUTPUT_DIR` → `MODELS_DIR` → `default_artifacts_dir()` from service config → built‑in fallback path.

---

## 4. Defaulting When Config Is Missing

- **Go API**
  - If no config file is found (and `GO_APP_CONFIG` is unset or the file is missing), `Load()` still returns a `Config` struct; fields keep their zero values unless code applies its own defaults (e.g. listen address, timeouts). Server startup validation may fail if required fields (e.g. DB URL) are missing.
- **ML service**
  - All settings have explicit defaults in `load_ml_service_settings()`; missing env vars use those defaults. Models directory falls back as in section 3.

---

## 5. Conventions and Naming

- **Consistency**
  - Similar concepts use similar names where practical (e.g. `GO_APP_OUTPUT_DIR` vs `ML_SERVICE_OUTPUT_DIR` for process-specific output roots).
- **Boolean env vars**
  - Go startup toggles accept `0`, `false`, or `no` (case-insensitive) to disable. ML service uses `_env_bool()` with `1`, `true`, `yes` for true.
- **Documentation**
  - For more detail on health, pipeline state, and logging, see `docs/observability.md`.
