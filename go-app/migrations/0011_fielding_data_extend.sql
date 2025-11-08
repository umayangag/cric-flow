-- Extend fielding_data with stumpings and runouts_direct_hits
ALTER TABLE fielding_data
  ADD COLUMN IF NOT EXISTS stumpings INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS runouts_direct_hits INT NOT NULL DEFAULT 0;