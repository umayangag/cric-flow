CREATE TABLE IF NOT EXISTS data_migrations (
    id SERIAL PRIMARY KEY,
    command VARCHAR(255) NOT NULL,
    args JSONB,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL, -- IN_PROGRESS, COMPLETED, FAILED, CANCELLED
    metadata JSONB,
    error_message TEXT
);

CREATE INDEX idx_data_migrations_started_at ON data_migrations(started_at DESC);
CREATE INDEX idx_data_migrations_command ON data_migrations(command);
CREATE INDEX idx_data_migrations_status ON data_migrations(status);
