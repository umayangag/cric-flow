-- Stores latest auto-tuned ML training parameters per model (and optional format).
-- ML service POSTs after auto_tune; training scripts GET latest and fall back to config if none.
CREATE TABLE IF NOT EXISTS ml_tuned_params (
    id SERIAL PRIMARY KEY,
    model VARCHAR(64) NOT NULL,
    format VARCHAR(32) NOT NULL DEFAULT '',
    params JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ml_tuned_params_model_format_created ON ml_tuned_params(model, format, created_at DESC);
