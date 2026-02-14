-- Migration: Allow multiple match_details rows per match (one per inning).
-- Drops UNIQUE(match_id) and adds UNIQUE(match_id, inning) so innings can be stored separately.
-- This enables MIN/MAX(opposition_name) across innings to derive both teams per match.

-- Ensure existing rows have inning set (COALESCE to 1 for NULLs)
UPDATE match_details SET inning = COALESCE(inning, 1) WHERE inning IS NULL;

-- Drop single-column unique on match_id (PostgreSQL default name)
ALTER TABLE match_details DROP CONSTRAINT IF EXISTS match_details_match_id_key;

-- Add composite unique so we can have (match_id, inning 1) and (match_id, inning 2)
ALTER TABLE match_details ADD CONSTRAINT uq_match_details_match_inning UNIQUE (match_id, inning);
