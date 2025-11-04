# Schema Parity Report — Prototype vs Implementation (Initial Draft)

Status: In progress (read-only validation). This report normalizes the database schema from the Python prototype (`src/`) and the Go implementation (`go-app`) and highlights differences with recommended actions. The objective is to meet or exceed the prototype’s capabilities with clearer constraints and better integrity, while keeping pipelines modular and maintainable.

Sources
- Prototype DDL: `src/createdb/queries/create_tables.py` (MySQL-style DDL)
- Implementation DDL: `go-app/migrations/0001_init.sql` (+ pending review of 0002–0008)

Conventions
- Types are normalized conceptually (e.g., INT ≈ INTEGER, FLOAT/REAL ≈ REAL).
- Severity: [Must-fix] affects correctness/parity; [Improve] optional enhancement; [Intentional] justified difference.

---

## Canonical Tables (by purpose)

1. Dimension/lookup
- `opposition(id, opposition_name)`
- `venue(id, venue_name)`
- `season(id, season_name)`

2. Core entities
- `player(id, player_name, is_wicket_keeper, is_retired, batting_consistency, bowling_consistency)`

3. Match & context
- `match_details(id, score, wickets, overs, balls, rpo, target, inning, result, opposition_id, date, match_id, batting_session, bowling_session, venue_id, extras, toss, season_id, match_number)`
- `weather_data(id, match_id, session, temp, feels, wind, gust, rain, humidity, cloud, pressure, viscosity)`

4. Performance facts
- `batting_data(id, match_id, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position)`
- `bowling_data(id, match_id, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls)`
- `fielding_data(id, match_id, player_id, catches, run_outs, dropped_catches, missed_run_outs)`

5. Historical feature slices
- `player_form_data(id, player_id, season_id, batting_form, bowling_form)`
- `player_venue_data(id, player_id, venue_id, batting_venue, bowling_venue)`
- `player_opposition_data(id, player_id, opposition_id, batting_opposition, bowling_opposition)`

All of the above exist in both prototype and implementation per `0001_init.sql`.

---

## Key Differences and Constraints

### player
- Prototype: `player_name` (no explicit UNIQUE), `is_wicket_keeper INT`, `is_retired INT`, consistency as FLOAT.
- Implementation: `player_name VARCHAR(250) NOT NULL UNIQUE`; `is_wicket_keeper SMALLINT DEFAULT 0`; `is_retired SMALLINT DEFAULT 0`; consistencies as REAL.
- Assessment: [Improve] Implementation adds sensible defaults and a UNIQUE on `player_name`. Parity OK; improvement retained.
- Action: Define idempotent process to maintain `is_wicket_keeper` and `is_retired` (see Keeper/Retired lifecycle).

### match_details
- Prototype: MySQL types; `toss VARCHAR(10)`; FKs to `venue`, `opposition`, `season`.
- Implementation: PostgreSQL; indexes on `match_id`, `venue_id`, `opposition_id`, `season_id`; `toss VARCHAR(16)`; `match_id BIGINT UNIQUE`.
- Assessment: [Improve] Stronger indexing and a unique `match_id`. `toss` widened. Parity OK.
- Action: Ensure exporter encodes `toss` consistently to expected ML input (0/1) — currently handled in exporter queries.

### weather_data
- Prototype: `viscosity VARCHAR(100)`; no explicit uniqueness; `session VARCHAR(100)`.
- Implementation: `UNIQUE (match_id, session)` and helpful indexes; same `viscosity` textual field.
- Assessment: [Improve] Enforces one weather row per (match, session). Parity OK.
- Action: Exporter already encodes viscosity `dry→0`, `humid→1`, NULL→0 via CASE/COALESCE. Keep validator coverage for nulls.

### batting_data / bowling_data / fielding_data
- Prototype: FKs; no unique constraints across (match_id, player_id) specified.
- Implementation: `UNIQUE (match_id, player_id)` on all three; indexes on `player_id` and `match_id`.
- Assessment: [Improve] Prevents duplicates; improves query performance. Parity OK.

### player_form_data / player_venue_data / player_opposition_data
- Prototype: FKs only.
- Implementation: `UNIQUE(player_id, season_id)`; `UNIQUE(player_id, venue_id)`; `UNIQUE(player_id, opposition_id)`.
- Assessment: [Improve] Data integrity improvements; semantics unchanged.

---

## Keeper/Retired Lifecycle (Semantics)
- Fields exist and are surfaced by API/exporter.
- Prototype has dedicated importers: `import_keepers_data.py`, `import_retired.py`.
- Implementation: No dedicated CLI found yet. Flags likely default to 0 until set.
- Risk: If flags are unset, player pool selection may include wrong candidates.
- Action [Must-fix]: Implement idempotent CLI importers or integrate into ETL with UPSERTs by `player_name`. Provide `--dry-run` and CSV schema validation. (See separate task in parity checklist.)

---

## Encoding & Export Expectations (for ML)
- Session: textual → encoded 1..3 (done in exporter).
- Viscosity: textual → encoded {0=dry, 1=humid}, NULL→0 (done in exporter).
- Toss: textual → numeric {0/1} (done in exporter; confirm mapping matches prototype semantics).
- Venue/Opposition: numeric aggregates used as features (AVG/COALESCE) — matches prototype intent.

---

## Pending Review (Migrations 0002–0008)
- 0002: unique/constraint refinements
- 0003: toss widening or value domain adjustments
- 0004: format dimension (if introduced)
- 0005: venue normalization tweaks
- 0006: weather job tables (if any)
- 0007: player consistency tables or materializations
- 0008: deprecations removed (ensure they don’t regress prototype coverage)

Action: Finish reviewing 0002–0008 and update this report with any deviations and remediation steps.

---

## Summary Assessment (Initial)
- Structural parity is strong; implementation improves integrity via UNIQUEs, defaults, and indexes.
- The main open parity gap is operational: maintaining `is_wicket_keeper` and `is_retired` flags. Implement CLI importers.
- Export encodings align with ML expectations; validator should be enforced in CI to prevent drift.

---

## Recommended Actions
1. [Must-fix] Add keepers/retired importer CLIs with idempotent UPSERTs; document CSV schema.
2. [Must-fix] Enforce export schema validation in CI (headers, order, types, nullability) for all formats.
3. [Improve] Document encoding conventions (session/toss/viscosity) centrally; assert in tests.
4. [Improve] Complete review of migrations 0002–0008 and document any intentional deviations.
