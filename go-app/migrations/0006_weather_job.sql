-- 0006_weather_job.sql
-- Queue table for asynchronous weather processing

CREATE TABLE IF NOT EXISTS weather_job (
    id BIGSERIAL PRIMARY KEY,
    match_id BIGINT NOT NULL UNIQUE,
    normalized_venue TEXT NOT NULL,
    city TEXT NULL,
    country TEXT NULL,
    start_at_local TIMESTAMPTZ NULL,
    end_at_local TIMESTAMPTZ NULL,
    sessions JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'queued',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_weather_job_status ON weather_job(status, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_weather_job_venue ON weather_job(normalized_venue);
