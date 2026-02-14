# Database Schema Redesign for Cricsheet JSON Data

This document proposes a restructured database schema that cleanly mirrors the Cricsheet JSON structure and supports T20, ODI, Test, and other formats. Data is reproducible from `data/go-app/cricsheet/*.json`, so we can safely drop and recreate.

---

## 1. JSON Structure Summary

Each Cricsheet JSON file has:

| Level | Fields | Notes |
|-------|--------|-------|
| **meta** | data_version, created, revision | Metadata |
| **info** | teams, venue, city, dates, match_type, season, toss, outcome, event, officials, balls_per_over, overs, gender, team_type, players (by team), registry (people IDs) | Match-level |
| **innings[]** | team (batting), overs[], powerplays[], target? | Per-inning |
| **overs[]** | over (0-indexed), deliveries[] | Per-over |
| **deliveries[]** | batter, bowler, non_striker, runs {batter, extras, total}, extras?, wickets?, review? | Per-ball |

Key insight: Batting and bowling aggregates are **per inning**. In Tests, a player can bat in innings 1 and 3, bowl in 2 and 4. The current `(match_id, player_id)` uniqueness in `batting_data` and `bowling_data` corrupts Test data (last inning overwrites).

---

## 2. Proposed Schema

### 2.1 Dimension Tables (unchanged or minor tweaks)

- **match_format** – T20, ODI, TEST, T20I, etc.
- **venue** – venue_name, city (optional from JSON)
- **season** – season_name (e.g. "2025/26")
- **opposition** – team names (India, Australia, etc.)
- **player** – player_name, cricsheet_id (from registry.people for deduplication)

### 2.2 Core Match / Inning Tables

#### `match`
Central match table. `match_id` is the current derived ID (StableMatchID).

| Column | Type | Notes |
|--------|------|-------|
| match_id | BIGINT PK | Derived from date+teams (StableMatchID) |
| format_id | BIGINT NOT NULL FK | T20/ODI/TEST |
| match_date | DATE NOT NULL | First date from info.dates |
| original_match_type | VARCHAR(100) | Raw value (T20, MDM, etc.) |
| venue_id | BIGINT FK | From info.venue / city |
| season_id | BIGINT FK | From info.season |
| toss_winner_opposition_id | BIGINT FK | Team that won toss |
| toss_decision | VARCHAR(16) | "bat" or "field" |
| outcome_winner_opposition_id | BIGINT FK | Winner (null = no result/tie) |
| outcome_by_runs | INT | Margin by runs |
| outcome_by_wickets | INT | Margin by wickets |
| event_name | VARCHAR(255) | info.event.name |
| match_number | INT | info.event.match_number |
| gender | VARCHAR(16) | male/female |
| balls_per_over | SMALLINT | Default 6 |
| overs_per_innings | INT | 20 for T20, 50 for ODI, null for Tests |

**Rationale:** Match-level info lives in one place. No more denormalising venue/season/toss into every inning row.

#### `match_details` (inning-level) – simplified

| Column | Type | Notes |
|--------|------|-------|
| match_id | BIGINT FK | References match |
| inning | SMALLINT NOT NULL | 1, 2, 3, 4 |
| batting_team_opposition_id | BIGINT FK | Batting side |
| bowling_team_opposition_id | BIGINT FK | Bowling side |
| score | INT | Runs scored |
| wickets | INT | Wickets lost |
| overs | REAL | Overs bowled (e.g. 19.4) |
| balls | INT | Legal balls |
| rpo | REAL | Run rate |
| target | INT | Chase target (2nd inn) |
| extras | INT | Extras conceded |
| result_opposition_id | BIGINT FK | Winner (if decided this inn) |

**Composite PK:** (match_id, inning)

**Changes:**
- Remove venue_id, season_id, format_id, match_date, toss – move to `match`
- Replace opposition_id (single) with batting_team + bowling_team
- Remove batting_session, bowling_session – redundant with batting/bowling team FKs
- Keep result only where inning determines outcome (e.g. 2nd innings chase)

### 2.3 Player Performance Tables

#### `batting_data` – add `inning`

| Column | Type | Notes |
|--------|------|-------|
| match_id | BIGINT NOT NULL | FK to match |
| inning | SMALLINT NOT NULL | 1, 2, 3, 4 |
| player_id | BIGINT NOT NULL FK | Batter |
| runs | INT | |
| balls | INT | Legal balls faced |
| fours | INT | |
| sixes | INT | |
| strike_rate | REAL | |
| batting_position | INT | Order (1–11) |
| description | VARCHAR(250) | Dismissal (bowled, lbw, not out) |
| minutes | INT | Optional |

**Composite UNIQUE:** (match_id, inning, player_id)

**Rationale:** In Tests, same player bats in multiple innings. Must key by inning.

#### `bowling_data` – add `inning`

| Column | Type | Notes |
|--------|------|-------|
| match_id | BIGINT NOT NULL | |
| inning | SMALLINT NOT NULL | |
| player_id | BIGINT NOT NULL FK | Bowler |
| overs | REAL | |
| balls | INT | |
| maidens | INT | |
| runs | INT | |
| wickets | INT | |
| dots | INT | |
| fours | INT | |
| sixes | INT | |
| econ | REAL | |
| wides | INT | |
| no_balls | INT | |

**Composite UNIQUE:** (match_id, inning, player_id)

### 2.4 Event Tables

#### `ball_event` (unchanged structure)

Already has (match_id, innings, over, ball). Keeps ball-by-ball data.

#### `fielding_event` (unchanged)

Ball-level fielding events with innings.

#### `fielding_data`

Keep (match_id, player_id) as **match-level aggregates** – catches, run_outs, stumpings across the whole match. No inning needed for aggregates.

### 2.5 Tables to Simplify or Drop

| Table | Action |
|-------|--------|
| weather_data | Keep – (match_id, session) |
| player_form_data, player_venue_data, player_opposition_data | Keep – format-aware variants exist |
| feature_* tables | Keep – depend on core tables |

---

## 3. Optional: Introduce `match` Table

Two approaches:

### Option A: Add `match` table, keep `match_details` as inning-only

- Create `match` with match-level fields.
- `match_details` references `match(match_id)` and holds only inning-specific data.
- `batting_data`, `bowling_data` add `inning` and use UNIQUE(match_id, inning, player_id).
- No `match` table: keep match-level data in `match_details` (one row per inning) but consolidate by having shared columns (venue_id, season_id, format_id, match_date, toss) repeated per inning – current approach, just fix batting/bowling.

### Option B: Minimal changes (recommended for incremental migration)

1. Add `inning` to `batting_data` and `bowling_data`.
2. Change UNIQUE to `(match_id, inning, player_id)`.
3. Keep `match_details` as-is (one row per inning) with venue, season, format, etc. denormalised per inning.
4. Add `batting_team_opposition_id` and `bowling_team_opposition_id` to `match_details` (or keep inferring from batting_session/bowling_session).

**Recommendation:** Option B first. Add `inning` to batting/bowling, fix uniqueness. Option A can be a later refactor.

---

## 4. Migration Plan (Option B)

### Step 1: Add inning to batting_data and bowling_data

**Note:** Current `batting_data` and `bowling_data` use `(match_id, player_id)`. With 2 innings per match, the ingest overwrites inning 1 with inning 2 data. For a clean state, truncate these tables after migration and re-run the cricsheet import.

```sql
-- 0090_add_inning_to_batting_bowling.sql

-- Batting
ALTER TABLE batting_data ADD COLUMN inning SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE batting_data DROP CONSTRAINT IF EXISTS uq_batting_match_player;
ALTER TABLE batting_data ADD CONSTRAINT uq_batting_match_inning_player UNIQUE (match_id, inning, player_id);

-- Bowling  
ALTER TABLE bowling_data ADD COLUMN inning SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE bowling_data DROP CONSTRAINT IF EXISTS uq_bowling_match_player;
ALTER TABLE bowling_data ADD CONSTRAINT uq_bowling_match_inning_player UNIQUE (match_id, inning, player_id);

-- Optional: clean slate for correct per-inning data (data is reproducible)
-- TRUNCATE batting_data, bowling_data;
-- Then re-run: go-app import of data/go-app/cricsheet/*.json
```

### Step 2: Update export/feature joins

Change joins from:
```sql
JOIN match_details md ON md.match_id = bd.match_id
```
To:
```sql
JOIN match_details md ON md.match_id = bd.match_id AND md.inning = bd.inning
```

### Step 3: Update ingestion

- Pass `inningNo` into `UpsertBattingBatch` and `UpsertBowlingBatch`.
- Ensure `Batting` and `Bowling` structs include `Inning`.

---

## 5. Column Cleanup (optional, data wipe ok)

Columns in `match_details` that could be dropped or moved:

| Column | Verdict |
|--------|---------|
| date | Already replaced by match_date (0029) |
| batting_session, bowling_session | Redundant if we add batting_team_id, bowling_team_id. Keep for now as team names. |
| result | Stores winner opposition_id. Keep. |
| toss | Match-level; repeated per inning. Denormalised but harmless. |

---

## 6. Summary

| Change | Impact |
|--------|--------|
| Add `inning` to batting_data, bowling_data | Fixes Test matches, enables correct joins |
| UNIQUE(match_id, inning, player_id) | Correct uniqueness for multi-innings |
| Join bd/bw to md on (match_id, inning) | Removes duplicate rows in exports |
| Optional: Add match table | Cleaner separation; can be Phase 2 |

Next step: Implement migration 0090 and update ingestion + export queries.
