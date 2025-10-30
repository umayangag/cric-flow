-- PostgreSQL schema initialization

-- Drop existing tables (optional for dev). Comment out in prod.
-- DO NOT enable cascade in production without backups.

CREATE TABLE IF NOT EXISTS opposition (
    id BIGSERIAL PRIMARY KEY,
    opposition_name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS venue (
    id BIGSERIAL PRIMARY KEY,
    venue_name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS season (
    id BIGSERIAL PRIMARY KEY,
    season_name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS player (
    id BIGSERIAL PRIMARY KEY,
    player_name VARCHAR(250) NOT NULL UNIQUE,
    is_wicket_keeper SMALLINT DEFAULT 0,
    is_retired SMALLINT DEFAULT 0,
    batting_consistency REAL,
    bowling_consistency REAL
);

CREATE TABLE IF NOT EXISTS match_details (
    id BIGSERIAL PRIMARY KEY,
    score INTEGER,
    wickets INTEGER,
    overs REAL,
    balls INTEGER,
    rpo REAL,
    target INTEGER,
    inning INTEGER,
    result INTEGER,
    opposition_id BIGINT REFERENCES opposition(id),
    date DATE,
    match_id BIGINT UNIQUE,
    batting_session VARCHAR(100),
    bowling_session VARCHAR(100),
    venue_id BIGINT REFERENCES venue(id),
    extras INTEGER,
    toss VARCHAR(16),
    season_id BIGINT REFERENCES season(id),
    match_number INTEGER
);

CREATE INDEX IF NOT EXISTS idx_match_details_match_id ON match_details(match_id);
CREATE INDEX IF NOT EXISTS idx_match_details_venue ON match_details(venue_id);
CREATE INDEX IF NOT EXISTS idx_match_details_opposition ON match_details(opposition_id);
CREATE INDEX IF NOT EXISTS idx_match_details_season ON match_details(season_id);

CREATE TABLE IF NOT EXISTS weather_data (
    id BIGSERIAL PRIMARY KEY,
    match_id BIGINT NOT NULL,
    session VARCHAR(100),
    temp INTEGER,
    feels INTEGER,
    wind INTEGER,
    gust INTEGER,
    rain INTEGER,
    humidity INTEGER,
    cloud INTEGER,
    pressure INTEGER,
    viscosity VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_weather_match_id ON weather_data(match_id);
CREATE INDEX IF NOT EXISTS idx_weather_session ON weather_data(session);

CREATE TABLE IF NOT EXISTS batting_data (
    id BIGSERIAL PRIMARY KEY,
    match_id BIGINT NOT NULL,
    player_id BIGINT NOT NULL REFERENCES player(id),
    description VARCHAR(250),
    runs INTEGER,
    balls INTEGER,
    minutes INTEGER,
    fours INTEGER,
    sixes INTEGER,
    strike_rate REAL,
    batting_position INTEGER
);

CREATE INDEX IF NOT EXISTS idx_batting_player ON batting_data(player_id);
CREATE INDEX IF NOT EXISTS idx_batting_match ON batting_data(match_id);

CREATE TABLE IF NOT EXISTS bowling_data (
    id BIGSERIAL PRIMARY KEY,
    match_id BIGINT NOT NULL,
    player_id BIGINT NOT NULL REFERENCES player(id),
    overs REAL,
    balls INTEGER,
    maidens INTEGER,
    runs INTEGER,
    wickets INTEGER,
    dots INTEGER,
    fours INTEGER,
    sixes INTEGER,
    econ REAL,
    wides INTEGER,
    no_balls INTEGER
);

CREATE INDEX IF NOT EXISTS idx_bowling_player ON bowling_data(player_id);
CREATE INDEX IF NOT EXISTS idx_bowling_match ON bowling_data(match_id);

CREATE TABLE IF NOT EXISTS fielding_data (
    id BIGSERIAL PRIMARY KEY,
    match_id BIGINT NOT NULL,
    player_id BIGINT NOT NULL REFERENCES player(id),
    catches INTEGER,
    run_outs INTEGER,
    dropped_catches INTEGER,
    missed_run_outs INTEGER
);

CREATE INDEX IF NOT EXISTS idx_fielding_player ON fielding_data(player_id);
CREATE INDEX IF NOT EXISTS idx_fielding_match ON fielding_data(match_id);

CREATE TABLE IF NOT EXISTS player_form_data (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    season_id BIGINT NOT NULL REFERENCES season(id),
    batting_form REAL,
    bowling_form REAL,
    UNIQUE(player_id, season_id)
);

CREATE TABLE IF NOT EXISTS player_venue_data (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    venue_id BIGINT NOT NULL REFERENCES venue(id),
    batting_venue REAL,
    bowling_venue REAL,
    UNIQUE(player_id, venue_id)
);

CREATE TABLE IF NOT EXISTS player_opposition_data (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    opposition_id BIGINT NOT NULL REFERENCES opposition(id),
    batting_opposition REAL,
    bowling_opposition REAL,
    UNIQUE(player_id, opposition_id)
);
