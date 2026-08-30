# Plan 03: Predicted Scorecard Summary for Upcoming Match (Priority 3)

**Goal:** Predict the scorecard summary of the match based on predicted player performance and the chosen team combination. For an **upcoming** match (no actual scorecard), return innings totals, predicted winner, and optional extras.

---

## 1. Objective

- After selecting the best XI for each team and obtaining per-player predictions (runs, wickets, economy, etc.), compute:
  - **Innings 1:** Team1 bats, Team2 bowls → total runs (sum of Team1 batting predictions) + extras; wickets lost = 10 (or sum of Team2 bowling wickets if we model that).
  - **Innings 2:** Team2 bats, Team1 bowls → same.
  - **Predicted winner:** Compare innings totals (or use win model if available).
- Expose this as part of the team-selection API response (e.g. `scorecard_summary`) so one call returns both selected XIs and the match summary.

---

## 2. Current State

| Component | Location | Behavior |
|-----------|----------|----------|
| Team selection API | `go-app/internal/server/predict_handlers.go` | `POST /api/predict/team-selection`: returns `result` with `Team1` and `Team2` (each []SelectedPlayer with PlayerID, PlayerName, Runs, Wickets, Economy, Catches, RunOuts). No innings totals or winner. |
| Predict team | `go-app/internal/services/predictteam/predict_team.go` | `PredictTeams` returns `Result{ Team1, Team2 []SelectedPlayer }`. |
| Backtest aggregates | `go-app/internal/server/backtest_handlers.go` | `populateMatchAggregatesAndMetrics`: sums runs from resp.Players, groups by team for teamRuns, predWinner = team with higher runs; predExtras from GetAverageExtrasForFormat. |

We have all inputs: two selected XIs and their Runs/Wickets/Economy. We need to aggregate and add extras, then determine winner.

---

## 3. Acceptance Criteria

- [ ] Response of `POST /api/predict/team-selection` includes a **scorecard_summary** object with at least: `innings1_total` (runs), `innings2_total` (runs), `predicted_winner` (team code or empty if tie), `extras_innings1`, `extras_innings2` (optional; from model or default).
- [ ] Innings 1 = Team1 batting total (sum of Team1[].Runs) + extras for innings 1; Innings 2 = Team2 batting total + extras for innings 2.
- [ ] Predicted winner: team with higher innings total; tie if equal.
- [ ] Extras: use format/venue average from DB when available (e.g. GetAverageExtrasForFormat), else a sensible default (e.g. 0 or a constant per format) so we don’t require the extras model to be loaded.

---

## 4. Implementation Details

### 4.1 Types

**File:** `go-app/internal/services/predictteam/predict_team.go`

- Add struct:
  - `ScorecardSummary struct { Innings1Total float64 \`json:"innings1_total"\`; Innings2Total float64 \`json:"innings2_total"\`; PredictedWinner string \`json:"predicted_winner"\`; ExtrasInnings1 float64 \`json:"extras_innings1,omitempty"\`; ExtrasInnings2 float64 \`json:"extras_innings2,omitempty"\` }`
- Add to `Result`:
  - `ScorecardSummary *ScorecardSummary \`json:"scorecard_summary,omitempty"\``

### 4.2 Computation

**File:** `go-app/internal/services/predictteam/predict_team.go`

- Add function:
  - `ComputeScorecardSummary(team1, team2 []SelectedPlayer, extrasInnings1, extrasInnings2 float64) ScorecardSummary`
  - Batting runs team1 = sum of team1[].Runs; batting runs team2 = sum of team2[].Runs.
  - Innings1Total = team1 batting runs + extrasInnings1; Innings2Total = team2 batting runs + extrasInnings2.
  - PredictedWinner = team1 if Innings1Total > Innings2Total, team2 if Innings2Total > Innings1Total, "" if equal.
  - Return ScorecardSummary.
- In `PredictTeams`, after building `result` (Team1 and Team2):
  - Resolve extras: need format and optionally venue. We have `format`, and we have `venueID` (from input.Venue). Call a DB helper e.g. `db.GetAverageExtrasForFormat(ctx, formatID, venueID)` for both innings (same value for both or optionally different if we ever have innings-specific extras). If error or no data, use 0.
  - Call `summary := ComputeScorecardSummary(result.Team1, result.Team2, extras1, extras2)` and set `result.ScorecardSummary = &summary`.

### 4.3 Extras lookup

**File:** `go-app/internal/db` (existing or new)

- Use existing `GetAverageExtrasForFormat(ctx, formatID, venueID)` if it exists and accepts nullable venueID. If it doesn’t exist, add it or use a simple fallback: e.g. query average extras for format (and optionally venue) from matches; return 0 on error. Check `go-app/internal/db` for existing extras helpers.

### 4.4 Predict team flow

**File:** `go-app/internal/services/predictteam/predict_team.go`

- After building `result.Team1` and `result.Team2`, and before `return result`:
  - Get formatID (already have from earlier in PredictTeams).
  - Get venueID (already have from input).
  - extras1, extras2 := getExtrasForMatch(ctx, formatID, venueID) // both innings same for now.
  - result.ScorecardSummary = &ComputeScorecardSummary(result.Team1, result.Team2, extras1, extras2).
- Pass context and format/venue into PredictTeams (already have); add a small helper that returns (extras1, extras2 float64) from DB or 0.

### 4.5 API response

**File:** `go-app/internal/server/predict_handlers.go`

- No change needed: the handler returns `result` from `predictteam.PredictTeams`; once Result contains ScorecardSummary, the JSON response will include it automatically.

### 4.6 Tests

**File:** `go-app/internal/services/predictteam/scorecard_summary_test.go` (new)

- TestComputeScorecardSummary: team1 total 150, team2 total 140, extras 5 each → Innings1Total=155, Innings2Total=145, PredictedWinner=team1 code. Need to pass team codes; either add to ScorecardSummary (team1_code, team2_code) or pass them into ComputeScorecardSummary so winner is one of those strings. Prefer: ComputeScorecardSummary(team1, team2, extras1, extras2, team1Code, team2Code string) so we can set PredictedWinner to the actual team code.
- Test tie: both 150 → PredictedWinner "".

### 4.7 Backward compatibility

- Adding an optional field `scorecard_summary` to the response is backward compatible; clients that don’t expect it can ignore it.

---

## 5. File Checklist

| File | Action |
|------|--------|
| `go-app/internal/services/predictteam/predict_team.go` | Add ScorecardSummary type and field to Result; add ComputeScorecardSummary; in PredictTeams, get extras and set result.ScorecardSummary. |
| `go-app/internal/db` | Use or add GetAverageExtrasForFormat(ctx, formatID, venueID) for extras lookup. |
| `go-app/internal/services/predictteam/scorecard_summary_test.go` | New: unit tests for ComputeScorecardSummary. |

---

## 6. Edge Cases

- No DB for extras: use 0 for both innings.
- Tie: PredictedWinner = "" (or "tie" if we prefer a sentinel).
- Team codes: Ensure we have team1 and team2 names/codes when building summary so PredictedWinner is a string the client can use (e.g. "IND", "AUS"). Input already has Team1 and Team2 strings; use those for PredictedWinner.
