package exportqueries

import "context"

// BuildBattingSeqFragments returns SELECT field expressions and JOIN/LATERAL SQL
// fragments for the T20 sequence feature subset. When the sequence flag is OFF,
// it returns empty slices/strings so callers can keep baseline queries unchanged.
//
// NOTE: These fragments are placeholders for safe integration and offline testing.
func BuildBattingSeqFragments(ctx context.Context) (fields []string, joins string) {
	if !IsSeqEnabled(ctx) {
		return nil, ""
	}
	fields = []string{
		// Previous context (placeholders until wired)
		"NULL::bigint  AS bat_prev_batter_id",
		"NULL::text    AS bat_prev_phase",
		"NULL::float8  AS bat_prev_sr",
		"NULL::float8  AS bat_prev_out_rate",
		// Windows (PP)
		"pw_pp.sr   AS bat_window_sr_12_pp",
		"pw_pp.br   AS bat_window_boundary_rate_12_pp",
		// Entry and set SR
		"ent.sr_1_6    AS bat_entry_sr_1_6",
		"set.sr_13_30  AS bat_set_sr_13_30",
		// Reaction features
		"er_bat.after_dot_sr AS bat_react_after_dot_sr",
		"er_bat.after_k_dots_boundary_p_k2 AS bat_after_k_dots_boundary_p_k2",
	}
	joins = `
	-- Player windows (powerplay) latest-as-of for TEST/ODI/T20I/T20 (format ids 1,2,3,4)
	LEFT JOIN LATERAL (
	  SELECT
	    CAST(NULL AS double precision) AS sr,
	    CAST(NULL AS double precision) AS br
	  FROM player_window_features pw
	  WHERE pw.format_id IN (1,2,3,4)
   ORDER BY as_of_date DESC LIMIT 1
	) pw_pp ON TRUE
	-- Entry/Set scoring rates (no confirmed surface; keep NULLs)
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS sr_1_6
	) ent ON TRUE
	LEFT JOIN LATERAL (
	  SELECT CAST(NULL AS double precision) AS sr_13_30
	) set ON TRUE
	-- Batting reaction features
	LEFT JOIN LATERAL (
	  SELECT 
	    CAST(NULL AS double precision) AS after_dot_sr,
	    CAST(NULL AS double precision) AS after_k_dots_boundary_p_k2
	  FROM event_reaction_features er0
	  WHERE er0.format_id IN (1,2,3,4)
	  ORDER BY er0.as_of_date DESC
	  LIMIT 1
	) er_bat ON TRUE
	`
	return fields, joins
}
