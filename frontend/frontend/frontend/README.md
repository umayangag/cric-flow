Frontend (React/Vite)

Overview
- Minimal UI for historical backtesting of already-played matches via the Go API and ML service.

Quick start
- Install deps: npm install
- Run tests: npm run test
- Dev server: make -C .. frontend-dev (or npm run dev inside frontend/)
- Production build: npm run build

Historical Backtest UI
- Filter by format, team1, team2 and list already-played matches (select mode).
- Pick a match and evaluate it with a cutoff timestamp (RFC3339, e.g., 2024-10-30T14:00:00Z) delegating to the ML service.
- See results: per-player predicted vs actual, match aggregates (runs/wickets/extras), summary metrics (player_runs_mae, winner_accuracy), and model_version.

Endpoints used (served by Go API)
- Select mode: GET /api/backtest/match?format=<FMT>&team1=<T1>&team2=<T2>&mode=select
- Evaluate (ML): GET /api/backtest/match?format=<FMT>&team1=<T1>&team2=<T2>&mode=evaluate&match_id=<ID>&use_ml=1&cutoff=<RFC3339>

Notes
- Ensure Go API (default http://localhost:8080) and ML service (default http://localhost:8000) are running.
- The Go API must have ML_SERVICE_URL set if the ML base URL differs from default.

Key files
- src/api/types.ts — DTOs for select/evaluate
- src/api/client.ts — Query builders and fetch helpers
- src/components/BacktestFilters.tsx — Filters form and candidates list
- src/components/BacktestEvaluate.tsx — Evaluate action with cutoff
- src/components/BacktestResults.tsx — Structured results view
- src/pages/BacktestPage.tsx — Wires filters → evaluate → results
