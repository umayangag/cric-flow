-- Migration: Add original_match_type to match_details
ALTER TABLE match_details ADD COLUMN original_match_type VARCHAR(100);
