# Go Test Standards — Violation Checklist

Generated: 2026-05-15

Reference: [Testing Guidelines](./testing-guidelines.md)

---

## Summary

| Violation | Remaining | Status |
|-----------|-----------|--------|
| V1: Internal test package (should be `package foo_test`) | ~75 files legitimately need internal access (unexported symbols) | ✅ All convertible files done |
| V2: Slice not named `testCases` | 0 | ✅ Done |
| V3: Raw `t.Fatal`/`t.Error` (should use testify) | 0 | ✅ Done |
| V4: `for _, tc := range` in parallel subtests | 0 | ✅ Done |
| V5: SUT instantiated outside `t.Run` | needs manual review | TODO |
| V6: Exported privates for testing | needs manual review | TODO |

---

## PR Plan

Each PR targets one package group. Keep PRs small and reviewable.

### PR 1 — Guidelines + V2/V4 sweep ✅ (this PR)
- [x] Created `docs/testing-guidelines.md`
- [x] Created `docs/test-standards-checklist.md`
- [x] Fixed all V2 violations (slice naming → `testCases`)
- [x] Fixed all V4 violations (`for _, tc := range` → index-based)
- [x] Rewrote `internal/phase`, `internal/eval`, `internal/formats`, `internal/features` tests (V1+V3)
- [x] Fixed `internal/db/scanx`, `internal/db/connection`, `internal/weather`, `internal/jobs`, `internal/pipeline` tests

### PR 2 — V1+V3: `internal/db` package group
- [ ] `internal/db/cache_test.go`
- [ ] `internal/db/db_test.go`
- [ ] `internal/db/migrations_helper_test.go`
- [ ] `internal/db/repo_backtest_query_test.go`
- [ ] `internal/db/repo_ball_event_integration_test.go`
- [ ] `internal/db/repo_bat_transitions_integration_test.go`
- [ ] `internal/db/repo_bowl_sequences_integration_test.go`
- [ ] `internal/db/repo_bowling_spells_integration_test.go`
- [ ] `internal/db/repo_cricsheet_batch_integration_test.go`
- [ ] `internal/db/repo_features_cutoff_test.go`
- [ ] `internal/db/repo_match_aggregates_test.go`
- [ ] `internal/db/repo_stats_integration_test.go`
- [ ] `internal/db/scan_helpers_test.go`
- [ ] `internal/db/scanx/scanx_convert_test.go`
- [ ] `internal/db/scanx/scanx_test.go`

### PR 3 — V1+V3: `internal/db/exportqueries`
- [ ] `internal/db/exportqueries/batting_seq_test.go`
- [ ] `internal/db/exportqueries/bowling_seq_test.go`
- [ ] `internal/db/exportqueries/contextflag_test.go`
- [ ] `internal/db/exportqueries/features_at_cutoff_test.go`
- [ ] `internal/db/exportqueries/headers_test.go`
- [ ] `internal/db/exportqueries/innings_cte_test.go`
- [ ] `internal/db/exportqueries/params_test.go`
- [ ] `internal/db/exportqueries/win_test.go`

### PR 4 — V1+V3: `internal/mlclient`
- [ ] `internal/mlclient/client_batting_bowling_errors_test.go`
- [ ] `internal/mlclient/client_test.go`
- [ ] `internal/mlclient/http_helpers_test.go`
- [ ] `internal/mlclient/mlclient_cancel_test.go`
- [ ] `internal/mlclient/mlclient_test.go`

### PR 5 — V1+V3: `internal/server`
- [ ] `internal/server/auth_test.go`
- [ ] `internal/server/backtest_accuracy_trend_test.go`
- [ ] `internal/server/backtest_accuracy_trend_validation_test.go`
- [ ] `internal/server/backtest_evaluate_test.go`
- [ ] `internal/server/backtest_handlers_ml_delegate_test.go`
- [ ] `internal/server/backtest_handlers_test.go`
- [ ] `internal/server/backtest_test.go`
- [ ] `internal/server/cors_test.go`
- [ ] `internal/server/handlers_test.go`
- [ ] `internal/server/health_test.go`
- [ ] `internal/server/helpers_test.go`
- [ ] `internal/server/ml_backtest_client_test.go`
- [ ] `internal/server/ops_handlers_test.go`
- [ ] `internal/server/ops_status_db_test.go`
- [ ] `internal/server/ops_status_test.go`
- [ ] `internal/server/predict_handlers_test.go`

### PR 6 — V1+V3: `internal/seqcalc`
- [ ] `internal/seqcalc/bat_transitions_test.go`
- [ ] `internal/seqcalc/bowl_sequences_test.go`
- [ ] `internal/seqcalc/default_registry_test.go`
- [ ] `internal/seqcalc/discipline_test.go`
- [ ] `internal/seqcalc/dot_streaks_test.go`
- [ ] `internal/seqcalc/end_pressure_test.go`
- [ ] `internal/seqcalc/overpos_test.go`
- [ ] `internal/seqcalc/player_rolling_helpers_test.go`
- [ ] `internal/seqcalc/reaction_test.go`
- [ ] `internal/seqcalc/registry_test.go`
- [ ] `internal/seqcalc/spells_test.go`
- [ ] `internal/seqcalc/wicket_modes_test.go`

### PR 7 — V1+V3: `internal/predictor` + `internal/selection`
- [ ] `internal/predictor/csvparse_test.go`
- [ ] `internal/predictor/predict_test.go`
- [ ] `internal/predictor/predictor_test.go`
- [ ] `internal/predictor/selector_test.go`
- [ ] `internal/selection/encodings_test.go`
- [ ] `internal/selection/selection_helpers_test.go`

### PR 8 — V1+V3: `internal/services/opsstatus`
- [ ] `internal/services/opsstatus/db_insights_test.go`
- [ ] `internal/services/opsstatus/helpers_test.go`
- [ ] `internal/services/opsstatus/ops_status_artifacts_test.go`
- [ ] `internal/services/opsstatus/ops_status_db_insights_test.go`
- [ ] `internal/services/opsstatus/ops_status_exports_test.go`
- [ ] `internal/services/opsstatus/ops_status_pipeline_test.go`
- [ ] `internal/services/opsstatus/ops_status_precompute_test.go`

### PR 9 — V1+V3: `internal/services/evaluate` + `internal/services/exportdataset`
- [ ] `internal/services/evaluate/options_test.go`
- [ ] `internal/services/evaluate/runner_test.go`
- [ ] `internal/services/evaluate/service_test.go`
- [ ] `internal/services/exportdataset/formats_test.go`
- [ ] `internal/services/exportdataset/guards_test.go`
- [ ] `internal/services/exportdataset/options_test.go`
- [ ] `internal/services/exportdataset/runner_internal_test.go`

### PR 10 — V1+V3: remaining `internal/services/*`
- [ ] `internal/services/backtest/service_test.go`
- [ ] `internal/services/cricsheetimporter/options_test.go`
- [ ] `internal/services/migrate/options_test.go`
- [ ] `internal/services/migrate/runner_test.go`
- [ ] `internal/services/pipeline/service_test.go`
- [ ] `internal/services/pipeline/steps_test.go`
- [ ] `internal/services/precomputeall/flags_precedence_test.go`
- [ ] `internal/services/precomputeall/flags_test.go`
- [ ] `internal/services/precomputefeatures/helpers_test.go`
- [ ] `internal/services/precomputefeatures/runner_test.go`

### PR 11 — V1+V3: `internal/services/predictteam` + `internal/services/teampredictor` + `internal/services/teamselect`
- [ ] `internal/services/predictteam/predict_team_helpers_test.go`
- [ ] `internal/services/predictteam/rescale_test.go`
- [ ] `internal/services/predictteam/scorecard_summary_test.go`
- [ ] `internal/services/predictteam/simulation_test.go`
- [ ] `internal/services/teampredictor/flags_test.go`
- [ ] `internal/services/teampredictor/options_test.go`
- [ ] `internal/services/teamselect/pool_more_test.go`
- [ ] `internal/services/teamselect/pool_test.go`

### PR 12 — V1+V3: remaining packages
- [ ] `internal/config/config_test.go`
- [ ] `internal/config/load_test.go`
- [ ] `internal/cricsheet/ingest_helpers_test.go`
- [ ] `internal/cricsheet/ingest_edgecases_test.go`
- [ ] `internal/cricsheet/ingest_error_test.go`
- [ ] `internal/cricsheet/ingest_integration_test.go`
- [ ] `internal/features/contract_test.go`
- [ ] `internal/features/features_parity_test.go`
- [ ] `internal/precompute/helpers_test.go`
- [ ] `internal/precompute/run_test.go`
- [ ] `internal/precompute/state_test.go`
- [ ] `internal/resources/limits_test.go`
- [ ] `internal/resources/memlog_test.go`
- [ ] `internal/resources/observations_test.go`
- [ ] `internal/tracking/repository_test.go`
- [ ] `internal/tracking/service_test.go`
- [ ] `internal/weather/weather_test.go`
- [ ] `cmd/api/main_test.go`
- [ ] `cmd/precompute-sequence-features/main_test.go`

### PR 13 — V5/V6 audit + final sweep
- [ ] Manual review: SUT instantiated outside `t.Run`
- [ ] Manual review: exported privates for testing
- [ ] Final `go test ./...` validation

---

## Notes

- **V1 (internal package)**: Some files legitimately need internal access (e.g., `config_test.go` accesses `cached` var, `weather_test.go` tests `buildSessions`). For these, consider restructuring the production code to expose testable seams, or document the exception.
- **V3 (raw assertions)**: Many remaining patterns are complex multi-line `if/else` blocks that need manual conversion.
- **V6 (exported privates)**: Requires auditing production code for functions/types exported solely for test access.
