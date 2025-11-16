package exportqueries

import "context"

// BuildBowlingSeqFragments returns SELECT field expressions and JOIN/LATERAL SQL
// fragments for the T20 sequence feature subset. When the sequence flag is OFF,
// it returns empty slices/strings so callers can keep baseline queries unchanged.
//
// NOTE: Integration is performed in a subsequent step; for now we provide
// deterministic fragments and keep them pure string builders to allow offline tests.
func BuildBowlingSeqFragments(ctx context.Context) (fields []string, joins string) {
	if !IsSeqEnabled(ctx) {
		return nil, ""
	}
	fields = []string{
		// Previous context (placeholder NULLs until wired)
		"NULL::bigint  AS bowl_prev_bowler_id",
		"NULL::text    AS bowl_prev_phase",
		"NULL::float8  AS bowl_prev_wkt_rate",
		// Windows (death)
		"pw_death.econ_rate AS bowl_window_econ_24_death",
		"pw_death.wkt_rate  AS bowl_window_wkt_rate_24_death",
		// Extras discipline (PP)
		"ed_pp.wides_per_over AS bowl_extras_wide_rate_pp",
		// Reaction after boundary (next ball wicket rate)
		"er.after_boundary_next_wkt_rate AS bowl_react_after_boundary_wkt_rate_next",
		// Spells (first over)
		"bs.first_over_wkt_rate AS bowl_spell_first_over_wkt_rate",
		// Over position (ball1/ball6)
		"obw1.wicket_rate AS bowl_over_ball1_wkt_rate",
		"obw6.wicket_rate AS bowl_over_ball6_wkt_rate",
	}
	joins = `
	-- Player windows (death phase) latest-as-of for TEST/ODI/T20I/T20 (format ids 1,2,3,4)
	LEFT JOIN LATERAL (
	  SELECT
	    CAST(NULL AS double precision) AS econ_rate,
	    CAST(NULL AS double precision) AS wkt_rate
	  FROM player_window_features pw
	  WHERE pw.format_id IN (1,2,3,4)
   ORDER BY as_of_date DESC
	  LIMIT 1
	) pw_death ON TRUE
	-- Extras discipline in powerplay (wides per over), latest-as-of across formats
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS wides_per_over
	  FROM extras_discipline_features ed
	  WHERE ed.format_id IN (1,2,3,4) AND ed.phase = 'powerplay'
	  ORDER BY ed.as_of_date DESC
	  LIMIT 1
	) ed_pp ON TRUE
	-- Event reaction features (after boundary → next ball wicket rate for bowler stream)
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS after_boundary_next_wkt_rate
	  FROM event_reaction_features er0
	  WHERE er0.format_id IN (1,2,3,4)
	  ORDER BY er0.as_of_date DESC
	  LIMIT 1
	) er ON TRUE
	-- Bowling spell features (first over wicket rate), latest-as-of
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS first_over_wkt_rate
	  FROM bowling_spell_features bs0
	  WHERE bs0.format_id IN (1,2,3,4)
	  ORDER BY bs0.as_of_date DESC
	  LIMIT 1
	) bs ON TRUE
	-- Over boundary/wicket features (positions 1 and 6), latest-as-of
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS wicket_rate
	  FROM over_boundary_wicket_features obw
	  WHERE obw.format_id IN (1,2,3,4) AND obw.position = 1
	  ORDER BY obw.as_of_date DESC
	  LIMIT 1
	) obw1 ON TRUE
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS wicket_rate
	  FROM over_boundary_wicket_features obw
	  WHERE obw.format_id IN (1,2,3,4) AND obw.position = 6
	  ORDER BY obw.as_of_date DESC
	  LIMIT 1
	) obw6 ON TRUE
	`
	return fields, joins
}
