# Plan: Streamline System for Best 11 Team Selection

## Goal

Select the best 11 players for each team for an upcoming match, optimizing for **batting, bowling, and fielding** (not purely individual performance). Use context: venue, inning, weather, and opposition players.

---

## 1. Current State Summary

### Data Pipeline
- **Import**: Cricsheet JSON → `match`, `match_inning`, `batting_data`, `bowling_data`, `fielding_event`, `fielding_data`, `weather_data` (optional placeholders)
- **Precompute**: `feature_form_snapshots`, `feature_consistency_snapshots` (batting/bowling only)
- **Export**: Batting/bowling rows with form, consistency, venue, opposition, weather, fielding aggregates

### Features (Today)
| Feature | Batting | Bowling | Future Match |
|---------|---------|---------|--------------|
| form | ✓ | ✓ | ✓ |
| consistency | ✓ | ✓ | ✓ |
| venue | ✓ | ✓ | ✓ |
| opposition (team) | ✓ | ✓ | ✓ |
| weather | ✓ (training) | ✓ (training) | **0 (not used)** |
| fielding aggregates | ✓ (input) | ✓ (input) | **not used** |

### ML Models
- **Batting**: predicts runs, balls, fours, sixes, batting_position
- **Bowling**: predicts runs, deliveries, wickets, economy
- **Fielding**: **no model** — predictions always 0

### Team Selection
- Greedy by composite score (BatScore + BowlScore + FieldScore + KeeperBonus)
- Constraints: MinBowlers, RequireKeeper
- FieldScore is 0 because ML returns catches=0, run_outs=0

### Gaps
1. **Fielding prediction**: Always 0; no form or model
2. **Weather for future match**: Not integrated (always 0)
3. **Opposition players**: Not used (only opposition team)
4. **Team composition**: Greedy, no batter–bowler matchups

---

## 2. Feature Extraction Enhancements

### 2.1 Fielding Form (Implement)

**Source**: `fielding_data` (catches, run_outs, stumpings, runouts_direct_hits)

- Add `ListFieldingBefore(ctx, playerID, cutoff, formatID)` in `db`
- Value = `catches + run_outs*1.5 + stumpings` (weighted involvements)
- Compute EWM (form) at cutoff; use as predicted fielding for team selection

**Changes**:
- `go-app/internal/db/repo_feature_snapshot.go` or new `repo_fielding.go`: `ListFieldingBefore`
- `go-app/internal/services/predictteam/predict_team.go`: after ML predict, enrich with fielding form from DB

### 2.2 Weather (API Input)

**Source**: External forecast or user input

- Add optional `Weather *WeatherInput` to `Input` in predictteam
- Pass to `ComputeFeaturesAtCutoffForFutureMatch` and feature map

**Changes**:
- `predictteam.Input`: `Weather *WeatherInput` (temp, humidity, etc.)
- `ComputeFeaturesAtCutoffForFutureMatch`: accept optional weather override
- Frontend: optional weather fields in upcoming match form

### 2.3 Opposition Players (Future)

- Add optional `OppositionPlayerIDs []int64` to `Input`
- Use for batter–bowler matchup features (future ML enhancement)

### 2.4 Inning Context

- For future matches, we don’t know batting order. Support optional `BattingFirst *string` (team1 or team2) for scenario analysis.

---

## 3. Implementation Order

| Phase | Task | Effort |
|-------|------|--------|
| 1 | Add `ListFieldingBefore` and fielding form enrichment in predictteam | Medium |
| 2 | Add optional Weather to Input and feature computation | Low |
| 3 | Add fielding columns to ComputeFeaturesAtCutoffForFutureMatch (for ML input consistency) | Low |
| 4 | Add OppositionPlayerIDs to Input (stub for future) | Low |
| 5 | Document and archive `src/` as legacy | Low |

---

## 4. Dead Code / Streamlining

### `src/` directory
- **Status**: Legacy Python (scrapers, createdb, team_selection, preprocessing)
- **Action**: Add `src/README.md` marking as legacy; go-app + ml-service are canonical

### Unused tables / code
- `over_*`, `wicket_mode_*`, `ball_event` features: used only if ball-level pipeline is enabled; keep for now
- `player_form_data`, `player_venue_data`, etc. (pre-feature_snapshots): may be superseded by feature_form_snapshots

---

## 5. Data Import Notes

- Cricsheet import already extracts fielding (catches, run_outs, stumpings) via `fielding_event` → `fielding_data`
- Run `backfill-fielding` after import to populate `fielding_data`
- Weather: placeholder rows via `PlaceholdersWeather`; real weather requires external source

---

## 6. Execution Checklist

- [x] Plan document created
- [x] ListFieldingBefore + fielding form enrichment (enrichFieldingFromHistory in predictteam)
- [x] Weather input for future match (WeatherInput, WeatherOverride, handler support)
- [x] OppositionPlayerIDs stub (optional field in Input)
- [x] src/ README (legacy notice)
