package exportqueries

import (
	"context"
)

// BowlingSeqHeaders returns the ordered list of sequence feature columns for bowling (T20 subset).
func BowlingSeqHeaders() []string {
	return []string{
		"bowl_prev_bowler_id",
		"bowl_prev_phase",
		"bowl_prev_wkt_rate",
		"bowl_window_econ_24_death",
		"bowl_window_wkt_rate_24_death",
		"bowl_extras_wide_rate_pp",
		"bowl_react_after_boundary_wkt_rate_next",
		"bowl_spell_first_over_wkt_rate",
		"bowl_over_ball1_wkt_rate",
		"bowl_over_ball6_wkt_rate",
	}
}

// BattingSeqHeaders returns the ordered list of sequence feature columns for batting (T20 subset).
func BattingSeqHeaders() []string {
	return []string{
		"bat_prev_batter_id",
		"bat_prev_phase",
		"bat_prev_sr",
		"bat_prev_out_rate",
		"bat_window_sr_12_pp",
		"bat_window_boundary_rate_12_pp",
		"bat_entry_sr_1_6",
		"bat_set_sr_13_30",
		"bat_react_after_dot_sr",
		"bat_after_k_dots_boundary_p_k2",
	}
}

// AppendSeqIfEnabled appends seq headers to base only when the sequence flag is enabled in the context.
func AppendSeqIfEnabled(ctx context.Context, base, seq []string) []string {
	if IsSeqEnabled(ctx) {
		out := make([]string, 0, len(base)+len(seq))
		out = append(out, base...)
		out = append(out, seq...)
		return out
	}
	return base
}
