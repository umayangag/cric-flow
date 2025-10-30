### Overview
This repository is a prototype data pipeline and analysis toolkit for international cricket (One Day Internationals, judging by the URLs) built around scraping match data from ESPNcricinfo, storing it in MySQL, engineering features, training simple ML models, and assembling a final dataset for team selection and performance prediction.

The current stack is Python-centric with pandas, BeautifulSoup, scikit-learn, and a MySQL backend. The project produces CSV artifacts at multiple stages and contains helper scripts to create and populate a relational schema.

You asked for two deliverables:
- A thorough README that explains the project’s structure and how to run it.
- A concrete plan to port the application to Go for performance/scalability, likely keeping ML in Python.

Below you’ll find both, plus some clarifying questions at the end.

---

### Key Features
- Scrapes ODI match lists and individual match pages from ESPNcricinfo.
- Extracts batting, bowling, fielding, and match metadata, including weather-related context derived from the match page (sessions and conditions parsed from match details).
- Normalizes and stores data in MySQL with a defined schema.
- Computes player-level metrics (form, venue/opposition-specific performance, consistency) into separate tables.
- Produces feature-engineered datasets for ML and runs basic regressors/classifiers with scikit-learn.
- Assembles a final combined dataset for player selection and match-level predictions.

---

### Repository Layout
- `src/scrapers/`
  - `main.py`: Orchestrates scraping for a fixed team (currently `Sri Lanka`) and time span; builds per-match DataFrames.
  - `match_list.py`: Scrapes team innings list (scorecards) from a Cricinfo results page; returns a DataFrame of matches with `Match_Id` and summary stats.
  - `parse_match.py`: Fetches and parses individual match pages.
  - `match_info.py`: Extracts match metadata (venue, toss, hours of play) from a match page.
  - `batting.py`, `bowling.py`, `fielding.py`: Extract player-level batting, bowling, and fielding stats from match page sections.
  - `session.py`: Infers batting/bowling sessions from the “Hours of play (local time)” section.
  - Outputs go to `src/extracted/*.csv` (when the commented lines in `main.py` are enabled).
- `src/createdb/`
  - `create_db.py`, `create_tables.py`: Create database and tables in MySQL using queries under `createdb/queries`.
  - `import_data.py`: Loads pre-extracted CSVs into MySQL via importers.
  - `importers/*.py`: Row-by-row CSV import to normalized tables.
  - `data/*.csv`: Example datasets to import (batting/bowling/match_details/weather/keepers/retired).
  - `queries/create_tables.py`: Full DDL strings for MySQL tables.
- `src/config/mysql.py`: MySQL connection helpers. Currently hard-coded to `localhost`, `root`, empty password, database `cricket_data`.
- `src/preprocessing/`
  - `preporcess.py`: Runs sequence of preprocessing steps to compute player stats (form, venue, opposition, consistency) for batting and bowling and stores them in the DB.
  - `calculate_*` modules: Implement each metric (not all opened here, but wired via `preporcess.py`).
- `src/final_data/`
  - `queries.py`: SQL queries that assemble training datasets by joining player, match, and weather data and the derived metrics tables.
  - `batting_regressor.py`, `bowling_regressor.py`, `batting_classifier.py`, `bowling_classifier.py`: Model training/prediction scripts (sklearn). `batting_regressor.py` shows feature scaling and a `MultiOutputRegressor(RandomForestRegressor)` pipeline.
  - `encoders.py`: Likely encodes categorical features (e.g., session/viscosity); referenced by team_selection modules.
  - `output/*.csv`: Encoded/engineered dataset outputs.
- `src/team_selection/`
  - `create_final_dataset.py`: Central script that composes per-player match-level features (using DB queries), merges batting and bowling vectors for the same player, and generates the final dataset; fills missing attributes.
  - `dataset_definitions.py`: Column name contracts for features/outputs used in the model pipelines.
  - `shared/match_data.py`: Helpers to query DB for match-level inputs (players, weather, scores, etc.).
  - `*.csv`: Example final dataset artifacts and pools.
- `src/analyze/`
  - Exploratory scripts: clustering, normalizing, correlations; includes sample input and outputs and generated graphs.
- `src/shared/utils.py`: Small helpers (numeric parsing, `get_record_id` upserting by name).

---

### Data Flow (End-to-End)
1. Scrape from ESPNcricinfo
   - `src/scrapers/match_list.py` pulls match/innings list for a team between dates.
   - For each match, `parse_match.get_content()` fetches the scorecard HTML.
   - `match_info.get_match_info()` extracts venue/toss/hours-of-play; `session.get_session_data()` derives batting/bowling session named values.
   - `batting.extract_batting_data()`, `bowling.extract_bowling_data()`, and `fielding.extract_fielding_data()` parse per-player stats.
   - Output: CSVs written to `src/extracted/*.csv` (when enabled in `main.py`).

2. Create and populate MySQL
   - `src/createdb/create_db.py` and `create_tables.py` create `cricket_data` schema with tables:
     - `match_details`, `weather_data`, `player`, `batting_data`, `bowling_data`, `fielding_data`, and derived metric tables: `player_form_data`, `player_venue_data`, `player_opposition_data`.
   - `src/createdb/import_data.py` loads CSVs from `src/createdb/data/` into MySQL via `importers/*.py`.

3. Preprocessing / Feature Engineering
   - `src/preprocessing/preporcess.py` calculates and persists player-level metrics (form, consistency, venue/opposition effects) into the derived tables.

4. Assemble ML datasets
   - `src/final_data/queries.py` joins the raw tables plus derived metrics and weather to produce model-ready views for batting and bowling.
   - `src/final_data/*_regressor.py` and `*_classifier.py` load encoded CSVs from `src/final_data/output/` or DB queries to train or predict.

5. Final dataset (team selection)
   - `src/team_selection/create_final_dataset.py` composes per-player rows for each match by combining batting and bowling features, fills missing attributes, and outputs `final_dataset.csv`.

6. Analysis (optional)
   - `src/analyze/*` contains EDA/normalization/clustering and generated plots.

---

### Architecture (ASCII)
```
[ESPNcricinfo] --HTTP--> [Scrapers (Python, bs4)] --CSV--> [Extracted CSVs]
                                          |                          
                                          v                          
                                 [createdb/importers] --MySQL--> [cricket_data]
                                                                    |
                                                                    v
                                                       [preprocessing (metrics)]
                                                                    |
                                                                    v
                                                      [final_data queries]
                                                                    |
                                                                    v
                                        [ML (sklearn) + team_selection dataset]
                                                                    |
                                                                    v
                                                          [Outputs/Reports]
```

---

### Prerequisites
- Python 3.8+ recommended.
- MySQL Server (local) with a user that can create databases. Default config is `root`/empty password, `localhost`.
- Python packages from `requirements.txt`:
  - pandas, numpy, scikit-learn, seaborn/matplotlib, mysql-connector, BeautifulSoup (`bs4`), scipy.
- Network access to `stats.espncricinfo.com`.

Note: Some scripts use Windows-style path separators (`"\\"`). On macOS/Linux, using `os.path.join` as in the code should resolve correctly, but a few hard-coded strings show `"output\\file.csv"`. These will still work because `os.path.join(dirname, "output\\file.csv")` will embed backslashes; if you hit path issues on non-Windows systems, we can normalize those.

---

### Setup
1. Clone the project.
2. Create a virtual environment and install requirements:
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # Windows: .venv\\Scripts\\activate
   pip install -r requirements.txt
   ```
3. Configure MySQL connection in `src/config/mysql.py` if needed (host, user, password).
4. Create DB and tables:
   ```bash
   python -m src.createdb.create_db
   python -m src.createdb.create_tables
   ```
5. Scrape data (optional if you use the provided CSVs):
   - Adjust `src/scrapers/main.py` for desired team/dates.
   - Uncomment the CSV writes in the script:
     - `match_info.csv`, `batting.csv`, `bowling.csv`, `fielding.csv` lines in `main.py`.
   - Run:
     ```bash
     python -m src.scrapers.main
     ```
   - CSVs will be saved under `src/extracted/`.
6. Import data from CSVs
   - If you use the curated CSVs under `src/createdb/data/`:
     ```bash
     python -m src.createdb.import_data
     ```
7. Precompute metrics
   ```bash
   python -m src.preprocessing.preporcess
   ```
8. Build final dataset
   ```bash
   python -m src.team_selection.create_final_dataset
   ```
9. Train/evaluate ML models (examples)
   ```bash
   python -m src.final_data.batting_regressor
   # similarly for other model scripts
   ```

---

### Configuration
- `src/config/mysql.py` controls DB credentials and database name.
- Team and date range for scraping are hard-coded in `src/scrapers/main.py`:
  - `team = "Sri Lanka"`
  - URL filters span 2010-01-01 to 2020-01-01 and `class=2` (ODIs). You can replace with different team IDs or date windows.

---

### Outputs
- `src/extracted/*.csv`: Raw scraped per-match/player CSVs.
- `src/createdb/data/*.csv`: Curated datasets ready for import.
- `src/final_data/output/*.csv`: Encoded/engineered datasets for modeling (e.g. `batting_encoded.csv`).
- `src/team_selection/final_dataset.csv`: Final merged dataset per player per match.
- `src/analyze/graphs/*.png`: EDA plots.

---

### Limitations and Notes
- Fragility to HTML structure changes on ESPNcricinfo (selectors like `tbody` indices are brittle, e.g., `table_body[2]`).
- No rate-limiting or retries in scraper (risk of blocking/timeouts).
- Hard-coded team and date filters.
- MySQL schema has limited constraints and no migrations/versioning.
- Some pandas patterns (`DataFrame.append`) are deprecated in modern pandas; future-proofing would use `pd.concat`.
- Windows path separators are embedded in some file paths; cross-platform cleanup would help.

---

### Plan to Port to Go (Keep ML in Python)
Below is a pragmatic, phased migration plan that improves performance and reliability while preserving core functionality. The ML remains in Python, exposed as a service.

#### Target Architecture
- Services/Modules:
  1. Scraper Service (Go)
     - HTTP client + HTML parsing with `goquery` for DOM traversal.
     - Configurable rate limiting, retries, backoff, and polite scraping (robots.txt awareness where applicable).
     - Outputs directly to DB or a message queue for ETL.
  2. ETL/Importer (Go)
     - Ingests scraped payloads into the relational schema (MySQL or PostgreSQL).
     - Uses a migration tool (`golang-migrate` or `goose`) to manage schema.
  3. Feature/Preprocessing Service (Go)
     - Computes player form, venue/opposition effects, consistency—mirroring `src/preprocessing` logic.
     - Runs as idempotent jobs per match or per season; can be triggered by queue or schedule (Cron).
  4. API Gateway (Go)
     - Exposes REST/gRPC endpoints to query datasets, trigger jobs, or request predictions.
  5. ML Inference Service (Python)
     - Serves trained sklearn models via FastAPI/Flask.
     - Implements endpoints like `/predict/batting`, `/predict/bowling` that accept the feature vectors defined in `dataset_definitions.py` and return predictions.
- Shared:
  - Database: Consider PostgreSQL for richer features and tooling, but MySQL is fine if you prefer to keep it.
  - Messaging (optional, recommended at scale): NATS/Kafka/RabbitMQ for decoupling scraper from ETL and preprocessing.
  - Observability: Prometheus metrics, OpenTelemetry tracing, structured logging (Zap/zerolog), Grafana dashboards.
  - Config: Viper for Go services; environment-based config and secrets via Vault/SSM.

#### Data Contracts
- Freeze column/feature names for interop between Go services and the Python ML service:
  - Adopt JSON schemas or protobuf definitions for feature payloads (e.g., `BattingFeatures`, `BowlingFeatures`).
  - Keep parity with `team_selection/dataset_definitions.py` and `final_data/queries.py`.
- Version payloads (`features.v1`, `features.v2`) to support evolution.

#### Go Module Breakdown
- `cmd/` with binaries:
  - `scraper`: CLI/service that crawls match lists and scorecards for a team/date range.
  - `etl-importer`: Listens to queue or scans a directory of JSON payloads and writes to DB.
  - `precompute`: Runs preprocessing batches with flags (e.g., `--season=2018`).
  - `api`: HTTP server for orchestration and querying.
- `internal/` packages:
  - `cricinfo`: page models, parsers (selectors mapped from current BeautifulSoup logic).
  - `db`: repositories for tables (`match_details`, `player`, `weather_data`, etc.).
  - `features`: implementations of form/consistency/venue/opposition calculations.
  - `contracts`: protobuf/JSON schemas for feature vectors and predictions.
  - `mlclient`: client for the Python inference service with retries and timeouts.
  - `scheduler`: cron/worker orchestration.

#### Schema and Migrations
- Translate current DDL from `src/createdb/queries/create_tables.py` into migration files.
- Add indexes on common join/filter keys:
  - `match_details.match_id` (unique), `player.player_name`, `player.id`, foreign keys on `*_id` columns, `weather_data.match_id`, `batting_data.player_id`, `bowling_data.player_id`.
- Consider normalization of `viscosity` to an enum and sessions to a smallint for compactness.

#### Scraper Parity Plan
1. Replicate `match_list.extract_match_list()` in Go using `goquery`.
   - Replace brittle `tbody[2]` indexing with robust selectors (e.g., exact table by header text, or CSS selectors anchored to id/class).
2. Implement match page parser with tests that assert against fixtures saved from ESPN pages (HTML snapshots) to avoid breaking with small HTML changes.
3. Add rate limiting and polite delays; rotate user agents if necessary.
4. Emit structured JSON payloads per match for downstream ETL.

#### Preprocessing Port
- Port the formulas/logic in `src/preprocessing/calculate_*` to Go with unit tests against sample DB snapshots.
- Ensure idempotency: reruns should upsert/update derived tables without duplicates.

#### ML Service (Python)
- Extract model training to an offline training pipeline (still Python) that saves artifacts (pickle or, better, ONNX/PMML if you plan cross-language inference later).
- Serve with FastAPI:
  - Endpoints accept standardized feature JSON (matching Go `contracts`).
  - Apply the same scalers used in training (persist `StandardScaler` objects alongside models).
  - Return predictions and derived metrics (e.g., strike rate) in a stable schema.
- Optionally schedule periodic retraining jobs.

#### API and Orchestration (Go)
- Expose endpoints:
  - `POST /scrape` (kick off for a team/date range)
  - `POST /precompute` (compute metrics for a season/team)
  - `POST /predict` (proxy to ML service, translate to/from contracts)
  - `GET /matches/:id`, `GET /players/:id`, dataset exports
- Auth (if needed) via JWT/API keys; rate limit public endpoints.

#### CI/CD, Packaging, and Ops
- Containerize services (Docker) and define `docker-compose.yml` for local dev (DB + services + ML service).
- Use GitHub Actions/GitLab CI to run:
  - Go unit tests and linters (golangci-lint)
  - Python unit tests for ML service
  - DB migration checks
- Deploy with Helm charts or Terraform on your infra of choice.

#### Validation Strategy
- Golden dataset approach:
  - Freeze a small set of matches and players; run the Python prototype to produce reference CSVs and DB snapshots.
  - Implement Go services and compare outputs within tolerances (especially floating-point metrics).
- Contract tests between Go `mlclient` and Python ML service.
- Performance tests for scraping throughput and ETL ingestion.

#### Phased Roadmap
1. Baseline and Contracts
   - Document feature schemas, freeze DB schema (add migrations), and produce golden datasets.
2. Scraper in Go
   - Implement match list + match page parser with fixtures and tests. Output JSON to disk or queue.
3. ETL in Go
   - Write JSON payloads into DB with migrations and indexes; validate against golden DB.
4. Preprocessing in Go
   - Port `calculate_*` logic; validate derived tables.
5. ML Service (Python)
   - Wrap existing sklearn models in a FastAPI service; implement request/response contracts.
6. API Gateway (Go)
   - User-facing endpoints, job triggers, and ML proxy.
7. Hardening
   - Logging, metrics, retries, rate limits, config, secrets management, and containerization.
8. Optional
   - Replace parts of ML with Go inference if/when models are reimplemented (e.g., XGBoost or ONNX-runtime in Go), or keep Python long-term.

---

### Suggested Enhancements During Port
- Replace brittle HTML indexing with robust selectors.
- Introduce rate limiting and exponential backoff.
- Add retries with jitter; handle request/parse errors gracefully.
- Normalize time zones and date parsing.
- Add unique keys and NOT NULL constraints where appropriate; use transactions for batch imports.
- Add unit/integration tests with stored HTML fixtures and seeded DBs.
- Replace pandas `append` loops with vectorized `concat` if you keep any Python preprocessing.
- Remove Windows backslashes in paths; use `pathlib` in Python and `filepath` in Go.

---

### Open Questions for You
1. Scope of formats: Is it strictly ODIs (`class=2`) or also Tests/T20s? Any specific teams beyond Sri Lanka?
2. Database choice: Stay on MySQL or migrate to PostgreSQL during the Go port?
3. Desired cadence: one-off historical backfill plus incremental updates (e.g., daily after matches)?
4. Target deployment: local/server, on-prem/cloud? Any preferred cloud vendor/services?
5. Interfaces: Do you want a public API/UI, or is this an internal pipeline producing CSVs and DB tables only?
6. ML expectations: Keep current sklearn models and feature definitions, or plan to revisit feature engineering/model selection?
7. Licensing and data usage: Any constraints for scraping Cricinfo and reusing data?
8. Authentication/security requirements for the API (if exposed)?
9. Performance targets: throughput for scraping (matches per minute), and latency for predictions.
10. OS/platform targets: Windows/macOS/Linux; do we need to maintain compatibility with Windows-like paths?

If you can share preferences on these, I’ll adjust the migration plan and the README’s setup instructions accordingly.