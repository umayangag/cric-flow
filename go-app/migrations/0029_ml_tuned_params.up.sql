-- Stores auto-tuned ML training parameters per (model, format). One row per model+format;
-- auto_tune saves after each format so we know which model and format the params belong to.
-- Training scripts GET by model+format and use these params when retraining (GO_APP_URL set).
CREATE TABLE IF NOT EXISTS ml_tuned_params (
    id SERIAL PRIMARY KEY,
    model VARCHAR(64) NOT NULL,
    format VARCHAR(32) NOT NULL DEFAULT '',
    params JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ml_tuned_params_model_format_created ON ml_tuned_params(model, format, created_at DESC);
