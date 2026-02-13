-- Migration to fix result type and mapping columns in match_details
ALTER TABLE match_details ALTER COLUMN result TYPE BIGINT;
ALTER TABLE match_details ADD CONSTRAINT fk_match_details_result FOREIGN KEY (result) REFERENCES opposition(id);
