# Rollout strategy for reconciled outputs (§5.2)

This document describes how to roll out the reconciliation pipeline and harmony metrics so that user-visible scorecards and win probabilities can move from the current (decoupled) pipeline to the reconciled pipeline in a controlled way.

## 5.2.1 Offline prototype runs

Before changing any user-facing behaviour:

1. **Historical matches with known outcomes**
   - Run the full reconciliation pipeline (e.g. via `POST /api/ml/generate-match` or backtest flows) on a set of historical matches.
   - Compare reconciled innings totals and win probability to actuals; compute harmony/realism metrics (e.g. `ml.compute_harmony_realism_metrics`, `ml.compute_win_coherence_metrics`).
   - Use `ml.analyze_reconciliation_adjustments` on logs to inspect adjustment magnitude by format and time window.

2. **Synthetic / corner-case matches**
   - Run reconciliation on synthetic matches that stress corner cases: very low scores, collapses, very high totals, narrow margins.
   - Ensure constraints (runs, wickets, balls) are always satisfied and that win coherence stays within acceptable bands.

3. **Artifacts**
   - Save prototype reports (adjustment summaries, realism vs historical, win coherence) for baseline comparison and future dashboards.

## 5.2.2 Shadow mode

- Run the reconciled pipeline **alongside** the current user-visible pipeline without changing what users see.
- **Go app:** Use `include_both_scorecards=true` (or equivalent) so that both standard and reconciled scorecards are computed; log or store the difference (e.g. innings totals diff, winner agreement, win-prob diff) for analysis.
- **ML service:** Already logs `backtest_predict.reconciliation.applied` and `win_coherence.metrics`; ensure these are ingested (e.g. into your log store) for dashboards.
- Collect expert or internal feedback on reconciled vs standard outputs before switching defaults.

## 5.2.3 Gradual adoption

- **Enable reconciled outputs** in stages:
  - First on internal dashboards and/or a limited segment (e.g. a single format or a feature flag).
  - Then roll out by format subset (e.g. T20 first, then ODI).
- **User control:** The team-selection API supports `use_reconciled_scorecard` and `include_both_scorecards` so clients can switch or compare; use these for A/B or opt-in before making reconciled the default.
- **Fallback:** Keep the current (non-reconciled) path available; if the ML service is unavailable or reconciliation fails, the Go app can fall back to the standard scorecard.
- **Kill switch:** If constraints or performance regress, disable reconciliation via config or feature flag (e.g. stop sending `use_reconciled_scorecard` / `include_both_scorecards` from the client, or add a server-side flag to ignore them).

## Optional: shadow-mode config

To run both pipelines and log differences without changing the default response, the Go app can support a config or env (e.g. `RECONCILED_SHADOW_MODE=true`) that, when set, always calls generate-match when possible and logs a comparison (standard vs reconciled totals and win prob) without returning the reconciled scorecard as primary. This can be implemented as a small extension to the current predict handler.
