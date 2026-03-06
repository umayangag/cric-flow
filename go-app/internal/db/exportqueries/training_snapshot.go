package exportqueries

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// sequenceFeatureKeys lists bat_*/bowl_* sequence features that are always 0
// at future-match prediction time because no in-match sequence data exists yet.
// The ML service defaults missing keys to 0.0, so omitting them saves payload.
var sequenceFeatureKeys = map[string]struct{}{
	"bat_prev_sr": {}, "bat_prev_out_rate": {}, "bat_window_sr_12_pp": {}, "bat_window_boundary_rate_12_pp": {},
	"bat_entry_sr_1_6": {}, "bat_set_sr_13_30": {}, "bat_react_after_dot_sr": {}, "bat_after_k_dots_boundary_p_k2": {},
	"bowl_prev_wkt_rate": {}, "bowl_window_econ_24_death": {}, "bowl_window_wkt_rate_24_death": {}, "bowl_extras_wide_rate_pp": {},
	"bowl_react_after_boundary_wkt_rate_next": {}, "bowl_spell_first_over_wkt_rate": {}, "bowl_over_ball1_wkt_rate": {}, "bowl_over_ball6_wkt_rate": {},
}

// ensureContractKeys fills all canonical contract keys (configs/feature_vectors.json) with 0 when absent.
func ensureContractKeys(feats map[string]float64) {
	allFeatureNames := append(features.BattingFeatureNames(), features.BowlingFeatureNames()...)
	for _, k := range allFeatureNames {
		if _, ok := feats[k]; !ok {
			feats[k] = 0
		}
	}
}

// ensureContractKeysSkipSequence fills all canonical contract keys except
// sequence features (bat_*/bowl_*) which are always 0 for future matches.
func ensureContractKeysSkipSequence(feats map[string]float64) {
	allFeatureNames := append(features.BattingFeatureNames(), features.BowlingFeatureNames()...)
	for _, k := range allFeatureNames {
		if _, skip := sequenceFeatureKeys[k]; skip {
			continue
		}
		if _, ok := feats[k]; !ok {
			feats[k] = 0
		}
	}
}

type battingSnapshotAtCutoff struct {
	form        float64
	formShort   float64
	formLong    float64
	momentum    float64
	consistency float64
	venue       float64
	opposition  float64
	raw         features.RawStats // v2: multi-scale windowed stats for ML to learn form/consistency
}

type bowlingSnapshotAtCutoff struct {
	form        float64
	formShort   float64
	formLong    float64
	momentum    float64
	consistency float64
	venue       float64
	opposition  float64
	careerAvg   float64           // simple mean of all historical bowling values (wickets per innings)
	raw         features.RawStats // v2: multi-scale windowed stats
}

type fieldingSnapshotAtCutoff struct {
	form        float64
	consistency float64
}

func toInnings(in []db.InnVal) []features.Innings {
	out := make([]features.Innings, 0, len(in))
	for _, iv := range in {
		out = append(out, features.Innings{Date: iv.MatchDate, Value: iv.Value})
	}
	return out
}

// rawStatsToExportStrings returns 18 export values in contract order (mean_w3 through innings_in_last_90d).
func rawStatsToExportStrings(r features.RawStats) []string {
	f := func(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }
	return []string{
		f(r.MeanW3), f(r.MeanW5), f(r.MeanW10), f(r.MeanW20),
		f(r.StdW5), f(r.StdW10), f(r.MaxW10), f(r.MinW10), f(r.MedianW10),
		f(r.Last1), f(r.Last2), f(r.Last3),
		f(r.CareerMean), strconv.Itoa(r.CareerCount), f(r.PctZeroW10), f(r.TrendW5),
		f(r.DaysSinceLast), strconv.Itoa(r.InningsInLast90D),
	}
}

// computeBattingSnapshotAtCutoff uses the same EWM and Consistency logic as precompute-features.
//
//nolint:unused // kept for consistency with precompute-features and possible future use
func computeBattingSnapshotAtCutoff(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	venueID, oppositionID *int64,
	alpha float64,
	lastN, windowN int,
) (battingSnapshotAtCutoff, error) {
	out := battingSnapshotAtCutoff{}
	alphaShort, alphaLong, momentumN := config.DefaultFeatureEWMAlphaShort, config.DefaultFeatureEWMAlphaLong, config.DefaultFeatureMomentumLastN
	if alpha <= 0 || lastN < 0 {
		fa, fn, fw, fs, fl, mn := GetFeatureExtractionParams()
		if alpha <= 0 {
			alpha = fa
		}
		if lastN < 0 {
			lastN = fn
		}
		alphaShort, alphaLong, momentumN = fs, fl, mn
		_ = fw // windowN already passed
	}

	hist, err := db.ListBattingBefore(ctx, playerID, asOf, formatID, nil, nil)
	if err != nil {
		return out, err
	}
	inn := toInnings(hist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
	out.formShort, _ = features.EWM(inn, alphaShort)
	out.formLong, _ = features.EWM(inn, alphaLong)
	out.momentum, _ = features.Momentum(inn, momentumN)
	out.consistency, _ = features.Consistency(inn, lastN)

	if venueID != nil && *venueID != 0 {
		venHist, err := db.ListBattingBefore(ctx, playerID, asOf, formatID, nil, venueID)
		if err != nil {
			return out, err
		}
		venInn := toInnings(venHist)
		venInn = features.SortAndClip(venInn, asOf)
		if windowN > 0 && len(venInn) > windowN {
			venInn = venInn[len(venInn)-windowN:]
		}
		out.venue, _ = features.EWM(venInn, alpha)
	}
	if oppositionID != nil && *oppositionID != 0 {
		oppHist, err := db.ListBattingBefore(ctx, playerID, asOf, formatID, oppositionID, nil)
		if err != nil {
			return out, err
		}
		oppInn := toInnings(oppHist)
		oppInn = features.SortAndClip(oppInn, asOf)
		if windowN > 0 && len(oppInn) > windowN {
			oppInn = oppInn[len(oppInn)-windowN:]
		}
		out.opposition, _ = features.EWM(oppInn, alpha)
	}
	return out, nil
}

// computeBattingSnapshotFromHistories computes form, form_short, form_long, momentum, consistency, venue, opposition
// from pre-fetched histories (avoids N+1 when batching).
func computeBattingSnapshotFromHistories(
	mainHist, venueHist, oppHist []db.InnVal,
	asOf time.Time,
	alpha, alphaShort, alphaLong float64,
	lastN, windowN, momentumN int,
) battingSnapshotAtCutoff {
	out := battingSnapshotAtCutoff{}
	if alpha <= 0 || lastN < 0 {
		fa, fn, fw, fs, fl, mn := GetFeatureExtractionParams()
		if alpha <= 0 {
			alpha = fa
		}
		if lastN < 0 {
			lastN = fn
		}
		alphaShort, alphaLong = fs, fl
		windowN, momentumN = fw, mn
	}
	inn := toInnings(mainHist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
	out.formShort, _ = features.EWM(inn, alphaShort)
	out.formLong, _ = features.EWM(inn, alphaLong)
	out.momentum, _ = features.Momentum(inn, momentumN)
	out.consistency, _ = features.Consistency(inn, lastN)
	if len(venueHist) > 0 {
		venInn := toInnings(venueHist)
		venInn = features.SortAndClip(venInn, asOf)
		if windowN > 0 && len(venInn) > windowN {
			venInn = venInn[len(venInn)-windowN:]
		}
		out.venue, _ = features.EWM(venInn, alpha)
	}
	if len(oppHist) > 0 {
		oppInn := toInnings(oppHist)
		oppInn = features.SortAndClip(oppInn, asOf)
		if windowN > 0 && len(oppInn) > windowN {
			oppInn = oppInn[len(oppInn)-windowN:]
		}
		out.opposition, _ = features.EWM(oppInn, alpha)
	}
	out.raw = features.WindowedStats(inn, asOf)
	return out
}

// computeBowlingSnapshotAtCutoff uses the same EWM and Consistency logic as precompute-features.
//
//nolint:unused // kept for consistency with precompute-features and possible future use
func computeBowlingSnapshotAtCutoff(
	ctx context.Context,
	playerID int64,
	asOf time.Time,
	formatID int64,
	venueID, oppositionID *int64,
	alpha float64,
	lastN, windowN int,
) (bowlingSnapshotAtCutoff, error) {
	out := bowlingSnapshotAtCutoff{}
	alphaShort, alphaLong, momentumN := config.DefaultFeatureEWMAlphaShort, config.DefaultFeatureEWMAlphaLong, config.DefaultFeatureMomentumLastN
	if alpha <= 0 || lastN < 0 {
		fa, fn, fw, fs, fl, mn := GetFeatureExtractionParams()
		if alpha <= 0 {
			alpha = fa
		}
		if lastN < 0 {
			lastN = fn
		}
		alphaShort, alphaLong, momentumN = fs, fl, mn
		_ = fw
	}

	hist, err := db.ListBowlingBefore(ctx, playerID, asOf, formatID, nil, nil)
	if err != nil {
		return out, err
	}
	inn := toInnings(hist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
	out.formShort, _ = features.EWM(inn, alphaShort)
	out.formLong, _ = features.EWM(inn, alphaLong)
	out.momentum, _ = features.Momentum(inn, momentumN)
	out.consistency, _ = features.Consistency(inn, lastN)

	if venueID != nil && *venueID != 0 {
		venHist, err := db.ListBowlingBefore(ctx, playerID, asOf, formatID, nil, venueID)
		if err != nil {
			return out, err
		}
		venInn := toInnings(venHist)
		venInn = features.SortAndClip(venInn, asOf)
		if windowN > 0 && len(venInn) > windowN {
			venInn = venInn[len(venInn)-windowN:]
		}
		out.venue, _ = features.EWM(venInn, alpha)
	}
	if oppositionID != nil && *oppositionID != 0 {
		oppHist, err := db.ListBowlingBefore(ctx, playerID, asOf, formatID, oppositionID, nil)
		if err != nil {
			return out, err
		}
		oppInn := toInnings(oppHist)
		oppInn = features.SortAndClip(oppInn, asOf)
		if windowN > 0 && len(oppInn) > windowN {
			oppInn = oppInn[len(oppInn)-windowN:]
		}
		out.opposition, _ = features.EWM(oppInn, alpha)
	}
	return out, nil
}

// computeFieldingSnapshotFromHistories computes form and consistency from fielding history (no venue/opposition scope).
func computeFieldingSnapshotFromHistories(
	mainHist []db.InnVal,
	asOf time.Time,
	alpha float64,
	lastN, windowN int,
) fieldingSnapshotAtCutoff {
	out := fieldingSnapshotAtCutoff{}
	if alpha <= 0 || lastN < 0 {
		fa, fn, _, _, _, _ := GetFeatureExtractionParams()
		if alpha <= 0 {
			alpha = fa
		}
		if lastN < 0 {
			lastN = fn
		}
	}
	inn := toInnings(mainHist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
	out.consistency, _ = features.Consistency(inn, lastN)
	return out
}

// computeBowlingSnapshotFromHistories computes form, form_short, form_long, momentum, consistency, venue, opposition,
// and career_avg from pre-fetched histories (avoids N+1 when batching).
func computeBowlingSnapshotFromHistories(
	mainHist, venueHist, oppHist []db.InnVal,
	asOf time.Time,
	alpha, alphaShort, alphaLong float64,
	lastN, windowN, momentumN int,
) bowlingSnapshotAtCutoff {
	out := bowlingSnapshotAtCutoff{}
	if alpha <= 0 || lastN < 0 {
		fa, fn, fw, fs, fl, mn := GetFeatureExtractionParams()
		if alpha <= 0 {
			alpha = fa
		}
		if lastN < 0 {
			lastN = fn
		}
		alphaShort, alphaLong = fs, fl
		windowN, momentumN = fw, mn
	}
	inn := toInnings(mainHist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
	out.formShort, _ = features.EWM(inn, alphaShort)
	out.formLong, _ = features.EWM(inn, alphaLong)
	out.momentum, _ = features.Momentum(inn, momentumN)
	out.consistency, _ = features.Consistency(inn, lastN)
	if len(inn) > 0 {
		var sum float64
		for _, i := range inn {
			sum += i.Value
		}
		out.careerAvg = sum / float64(len(inn))
	}
	if len(venueHist) > 0 {
		venInn := toInnings(venueHist)
		venInn = features.SortAndClip(venInn, asOf)
		if windowN > 0 && len(venInn) > windowN {
			venInn = venInn[len(venInn)-windowN:]
		}
		out.venue, _ = features.EWM(venInn, alpha)
	}
	if len(oppHist) > 0 {
		oppInn := toInnings(oppHist)
		oppInn = features.SortAndClip(oppInn, asOf)
		if windowN > 0 && len(oppInn) > windowN {
			oppInn = oppInn[len(oppInn)-windowN:]
		}
		out.opposition, _ = features.EWM(oppInn, alpha)
	}
	out.raw = features.WindowedStats(inn, asOf)
	return out
}

// getPrecomputedFeaturesForMatch reads form, consistency, venue, and opposition from the precomputed
// snapshot tables (feature_form_snapshots, feature_consistency_snapshots) populated by the precompute
// cmd tool per format. Returns a map of playerID -> feature name -> value; only keys present in the
// snapshots are set (so callers can fall back to on-the-fly computation for missing keys).
func getPrecomputedFeaturesForMatch(
	ctx context.Context,
	cutoff time.Time,
	formatID int64,
	venueID *int64,
	playerOpps map[int64]struct{ BattingOpp, BowlingOpp *int64 },
	playerIDs []int64,
) (map[int64]map[string]float64, error) {
	if db.Pool == nil {
		return nil, nil
	}
	cutoffDate := cutoff.Truncate(24 * time.Hour)
	out := make(map[int64]map[string]float64)
	for _, pid := range playerIDs {
		out[pid] = make(map[string]float64)
	}

	// 1) Overall form (scope=overall, scope_id NULL)
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT ON (player_id) player_id, batting_value, bowling_value
		FROM feature_form_snapshots
		WHERE player_id = ANY($1::bigint[]) AND format_id = $2 AND scope = 'overall' AND scope_id IS NULL AND as_of_date <= $3
		ORDER BY player_id, as_of_date DESC
	`, playerIDs, formatID, cutoffDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var batVal, bowlVal float64
		if err := rows.Scan(&pid, &batVal, &bowlVal); err != nil {
			rows.Close()
			return nil, err
		}
		out[pid]["batting_form"] = batVal
		out[pid]["bowling_form"] = bowlVal
	}
	rows.Close()

	// 2) Overall consistency
	rows, err = db.Pool.Query(ctx, `
		SELECT DISTINCT ON (player_id) player_id, batting_value, bowling_value
		FROM feature_consistency_snapshots
		WHERE player_id = ANY($1::bigint[]) AND format_id = $2 AND scope = 'overall' AND scope_id IS NULL AND as_of_date <= $3
		ORDER BY player_id, as_of_date DESC
	`, playerIDs, formatID, cutoffDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var batVal, bowlVal float64
		if err := rows.Scan(&pid, &batVal, &bowlVal); err != nil {
			rows.Close()
			return nil, err
		}
		out[pid]["batting_consistency"] = batVal
		out[pid]["bowling_consistency"] = bowlVal
	}
	rows.Close()

	// 3) Venue form (when match has a venue)
	if venueID != nil && *venueID != 0 {
		rows, err = db.Pool.Query(ctx, `
			SELECT DISTINCT ON (player_id) player_id, batting_value, bowling_value
			FROM feature_form_snapshots
			WHERE player_id = ANY($1::bigint[]) AND format_id = $2 AND scope = 'venue' AND scope_id = $3 AND as_of_date <= $4
			ORDER BY player_id, as_of_date DESC
		`, playerIDs, formatID, *venueID, cutoffDate)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pid int64
			var batVal, bowlVal float64
			if err := rows.Scan(&pid, &batVal, &bowlVal); err != nil {
				rows.Close()
				return nil, err
			}
			out[pid]["batting_venue"] = batVal
			out[pid]["bowling_venue"] = bowlVal
			out[pid]["venue"] = batVal
		}
		rows.Close()
	}

	// 4) Opposition form: (player_id, scope_id) pairs for batting and bowling opposition
	var oppPids, oppScopeIDs []int64
	for _, pid := range playerIDs {
		opps := playerOpps[pid]
		if opps.BattingOpp != nil && *opps.BattingOpp != 0 {
			oppPids = append(oppPids, pid)
			oppScopeIDs = append(oppScopeIDs, *opps.BattingOpp)
		}
		if opps.BowlingOpp != nil && *opps.BowlingOpp != 0 {
			oppPids = append(oppPids, pid)
			oppScopeIDs = append(oppScopeIDs, *opps.BowlingOpp)
		}
	}
	if len(oppPids) > 0 {
		rows, err = db.Pool.Query(ctx, `
			SELECT DISTINCT ON (f.player_id, f.scope_id) f.player_id, f.scope_id, f.batting_value, f.bowling_value
			FROM feature_form_snapshots f
			INNER JOIN unnest($3::bigint[], $4::bigint[]) AS pairs(pid, sid) ON f.player_id = pairs.pid AND f.scope_id = pairs.sid
			WHERE f.format_id = $1 AND f.scope = 'opposition' AND f.as_of_date <= $2
			ORDER BY f.player_id, f.scope_id, f.as_of_date DESC
		`, formatID, cutoffDate, oppPids, oppScopeIDs)
		if err != nil {
			return nil, err
		}
		type oppRow struct {
			pid     int64
			scopeID int64
			batVal  float64
			bowlVal float64
		}
		var oppRows []oppRow
		for rows.Next() {
			var r oppRow
			var scopeID sql.NullInt64
			if err := rows.Scan(&r.pid, &scopeID, &r.batVal, &r.bowlVal); err != nil {
				rows.Close()
				return nil, err
			}
			if scopeID.Valid {
				r.scopeID = scopeID.Int64
				oppRows = append(oppRows, r)
			}
		}
		rows.Close()
		for _, r := range oppRows {
			opps := playerOpps[r.pid]
			if opps.BattingOpp != nil && *opps.BattingOpp == r.scopeID {
				out[r.pid]["batting_opposition"] = r.batVal
				out[r.pid]["opposition"] = r.batVal
			}
			if opps.BowlingOpp != nil && *opps.BowlingOpp == r.scopeID {
				out[r.pid]["bowling_opposition"] = r.bowlVal
			}
		}
	}

	// 5) Overall raw windowed stats (scope=overall) for v2 contract
	rows, err = db.Pool.Query(ctx, `
		SELECT DISTINCT ON (player_id) player_id,
			batting_mean_w3, batting_mean_w5, batting_mean_w10, batting_mean_w20,
			batting_std_w5, batting_std_w10, batting_max_w10, batting_min_w10, batting_median_w10,
			batting_last_1, batting_last_2, batting_last_3,
			batting_career_mean, batting_career_count, batting_pct_zero_w10, batting_trend_w5,
			batting_days_since_last, batting_innings_in_last_90d,
			bowling_mean_w3, bowling_mean_w5, bowling_mean_w10, bowling_mean_w20,
			bowling_std_w5, bowling_std_w10, bowling_max_w10, bowling_min_w10, bowling_median_w10,
			bowling_last_1, bowling_last_2, bowling_last_3,
			bowling_career_mean, bowling_career_count, bowling_pct_zero_w10, bowling_trend_w5,
			bowling_days_since_last, bowling_innings_in_last_90d
		FROM feature_raw_stats_snapshots
		WHERE player_id = ANY($1::bigint[]) AND format_id = $2 AND scope = 'overall' AND scope_id IS NULL AND as_of_date <= $3
		ORDER BY player_id, as_of_date DESC
	`, playerIDs, formatID, cutoffDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var bm3, bm5, bm10, bm20, bs5, bs10, bmax10, bmin10, bmed10 float64
		var bl1, bl2, bl3, bcarM float64
		var bcarC, binn90 int
		var bpct0, btr5, bdays float64
		var om3, om5, om10, om20, os5, os10, omax10, omin10, omed10 float64
		var ol1, ol2, ol3, ocarM float64
		var ocarC, oinn90 int
		var opct0, otr5, odays float64
		if err := rows.Scan(&pid,
			&bm3, &bm5, &bm10, &bm20, &bs5, &bs10, &bmax10, &bmin10, &bmed10,
			&bl1, &bl2, &bl3, &bcarM, &bcarC, &bpct0, &btr5, &bdays, &binn90,
			&om3, &om5, &om10, &om20, &os5, &os10, &omax10, &omin10, &omed10,
			&ol1, &ol2, &ol3, &ocarM, &ocarC, &opct0, &otr5, &odays, &oinn90); err != nil {
			rows.Close()
			return nil, err
		}
		m := out[pid]
		m["batting_mean_w3"] = bm3
		m["batting_mean_w5"] = bm5
		m["batting_mean_w10"] = bm10
		m["batting_mean_w20"] = bm20
		m["batting_std_w5"] = bs5
		m["batting_std_w10"] = bs10
		m["batting_max_w10"] = bmax10
		m["batting_min_w10"] = bmin10
		m["batting_median_w10"] = bmed10
		m["batting_last_1"] = bl1
		m["batting_last_2"] = bl2
		m["batting_last_3"] = bl3
		m["batting_career_mean"] = bcarM
		m["batting_career_count"] = float64(bcarC)
		m["batting_pct_zero_w10"] = bpct0
		m["batting_trend_w5"] = btr5
		m["batting_days_since_last"] = bdays
		m["batting_innings_in_last_90d"] = float64(binn90)
		m["bowling_mean_w3"] = om3
		m["bowling_mean_w5"] = om5
		m["bowling_mean_w10"] = om10
		m["bowling_mean_w20"] = om20
		m["bowling_std_w5"] = os5
		m["bowling_std_w10"] = os10
		m["bowling_max_w10"] = omax10
		m["bowling_min_w10"] = omin10
		m["bowling_median_w10"] = omed10
		m["bowling_last_1"] = ol1
		m["bowling_last_2"] = ol2
		m["bowling_last_3"] = ol3
		m["bowling_career_mean"] = ocarM
		m["bowling_career_count"] = float64(ocarC)
		m["bowling_pct_zero_w10"] = opct0
		m["bowling_trend_w5"] = otr5
		m["bowling_days_since_last"] = odays
		m["bowling_innings_in_last_90d"] = float64(oinn90)
	}
	rows.Close()

	return out, nil
}

// requiredPrecomputedKeysForMatchAll are always required when using match context.
var requiredPrecomputedKeysForMatchAll = []string{
	"batting_form", "batting_consistency", "bowling_form", "bowling_consistency",
}

// requiredPrecomputedKeysNoMatch are the feature keys required when no match context (overall form/consistency only).
var requiredPrecomputedKeysNoMatch = []string{
	"batting_form", "batting_consistency", "bowling_form", "bowling_consistency",
}

// WeatherOverride optionally overrides weather feature values (otherwise 0) for future-match prediction.
type WeatherOverride struct {
	Temp, Humidity, Wind, Rain, Cloud, Pressure float64
}

// computeOppositionStrength returns average batting form and bowling form (as of cutoff) across the
// given opposition player IDs for the format. Used to add opposition_batting_strength and
// opposition_bowling_strength at prediction when the opposition pool is known. On error or empty
// precomp returns 0, 0.
func computeOppositionStrength(
	ctx context.Context,
	cutoff time.Time,
	formatID int64,
	oppositionPlayerIDs []int64,
) (battingStrength, bowlingStrength float64, _ error) {
	if len(oppositionPlayerIDs) == 0 {
		return 0, 0, nil
	}
	emptyOpps := make(map[int64]struct{ BattingOpp, BowlingOpp *int64 })
	for _, pid := range oppositionPlayerIDs {
		emptyOpps[pid] = struct{ BattingOpp, BowlingOpp *int64 }{nil, nil}
	}
	precomp, err := getPrecomputedFeaturesForMatch(ctx, cutoff, formatID, nil, emptyOpps, oppositionPlayerIDs)
	if err != nil || precomp == nil {
		return 0, 0, err
	}
	var sumBat, sumBowl float64
	var nBat, nBowl int
	for _, pc := range precomp {
		if b, ok := pc["batting_form"]; ok {
			sumBat += b
			nBat++
		}
		if b, ok := pc["bowling_form"]; ok {
			sumBowl += b
			nBowl++
		}
	}
	if nBat > 0 {
		battingStrength = sumBat / float64(nBat)
	}
	if nBowl > 0 {
		bowlingStrength = sumBowl / float64(nBowl)
	}
	return battingStrength, bowlingStrength, nil
}

// ComputeFeaturesAtCutoffForFutureMatch returns a feature map per player for a hypothetical future match.
// Used when predicting team selection: same venue and opposition for all players (the opposition team).
// Missing precomputed values are filled with 0 to support new/auction players with no prior history.
// When weather is non-nil, its values override the default 0 for batting_* and bowling_* weather features.
// Sequence features (bat_*, bowl_*) are omitted because they are always 0 for future matches (no
// in-match data exists yet); the ML service defaults missing keys to 0.0. Optional
// oppositionPlayerIDs (e.g. the opposition team's pool) are used to compute opposition_batting_strength
// and opposition_bowling_strength; when not provided or empty, those keys are 0 (training does not yet
// include them; when a weather source is added, use the same feature names in training and prediction).
func ComputeFeaturesAtCutoffForFutureMatch(
	ctx context.Context,
	cutoff time.Time,
	format string,
	venueID *int64,
	oppositionID int64,
	seasonID *int64,
	playerIDs []int64,
	weather *WeatherOverride,
	oppositionPlayerIDs []int64,
) (map[int64]map[string]float64, error) {
	if len(playerIDs) == 0 {
		return map[int64]map[string]float64{}, nil
	}
	formatID, err := db.GetGlobalCache().GetFormatID(ctx, strings.TrimSpace(strings.ToUpper(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format for features: %w", err)
	}
	playerOpps := make(map[int64]struct{ BattingOpp, BowlingOpp *int64 })
	var oppPtr *int64
	if oppositionID != 0 {
		oppPtr = &oppositionID
	}
	for _, pid := range playerIDs {
		playerOpps[pid] = struct{ BattingOpp, BowlingOpp *int64 }{
			BattingOpp: oppPtr,
			BowlingOpp: oppPtr,
		}
	}
	precomp, err := getPrecomputedFeaturesForMatch(ctx, cutoff, formatID, venueID, playerOpps, playerIDs)
	if err != nil {
		return nil, err
	}
	if precomp == nil {
		precomp = make(map[int64]map[string]float64)
	}

	season := 0.0
	if seasonID != nil && *seasonID != 0 {
		season = float64(*seasonID)
	}

	oppBatStr, oppBowlStr := 0.0, 0.0
	if len(oppositionPlayerIDs) > 0 {
		oppBatStr, oppBowlStr, _ = computeOppositionStrength(ctx, cutoff, formatID, oppositionPlayerIDs)
	}

	out := make(map[int64]map[string]float64)
	for _, pid := range playerIDs {
		pc := precomp[pid]
		if pc == nil {
			pc = make(map[string]float64)
		}
		get := func(k string) float64 {
			if v, ok := pc[k]; ok {
				return v
			}
			return 0
		}
		getOrDefault := func(k string, d float64) float64 {
			if v, ok := pc[k]; ok {
				return v
			}
			return d
		}
		wt, wh, ww, wr, wc, wp := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
		if weather != nil {
			wt, wh, ww, wr, wc, wp = weather.Temp, weather.Humidity, weather.Wind, weather.Rain, weather.Cloud, weather.Pressure
		}
		batForm := get("batting_form")
		bowlForm := get("bowling_form")
		feats := map[string]float64{
			"batting_form":        batForm,
			"batting_form_short":  getOrDefault("batting_form_short", batForm),
			"batting_form_long":   getOrDefault("batting_form_long", batForm),
			"batting_momentum":    get("batting_momentum"),
			"batting_consistency": get("batting_consistency"),
			"batting_venue":       get("batting_venue"),
			"batting_opposition":  get("batting_opposition"),
			"bowling_form":        bowlForm,
			"bowling_form_short":  getOrDefault("bowling_form_short", bowlForm),
			"bowling_form_long":   getOrDefault("bowling_form_long", bowlForm),
			"bowling_momentum":    get("bowling_momentum"),
			"bowling_career_avg":  getOrDefault("bowling_career_avg", bowlForm),
			"bowling_consistency": get("bowling_consistency"),
			"bowling_venue":       get("bowling_venue"),
			"bowling_opposition":  get("bowling_opposition"),
			"venue":               get("venue"),
			"opposition":          get("opposition"),
			"season":              season,
			"batting_temp":        wt, "batting_wind": ww, "batting_rain": wr, "batting_humidity": wh, "batting_cloud": wc, "batting_pressure": wp, "batting_viscosity": 0,
			"bowling_temp": wt, "bowling_wind": ww, "bowling_rain": wr, "bowling_humidity": wh, "bowling_cloud": wc, "bowling_pressure": wp, "bowling_viscosity": 0,
			"batting_inning": 0, "batting_session": 0, "toss": 0,
			"bowling_session":             0,
			"opposition_batting_strength": oppBatStr,
			"opposition_bowling_strength": oppBowlStr,
			"match_date_unix":             float64(cutoff.Unix()),
		}
		for _, k := range features.RawStatsFeatureNames() {
			feats[k] = get(k)
		}
		ensureContractKeysSkipSequence(feats)
		out[pid] = feats
	}
	return out, nil
}

func missingPrecomputedKeys(precomp map[int64]map[string]float64, playerIDs []int64, keys []string) []string {
	var missing []string
	for _, pid := range playerIDs {
		pc := precomp[pid]
		for _, k := range keys {
			if _, ok := pc[k]; !ok {
				missing = append(missing, fmt.Sprintf("player %d missing %s", pid, k))
			}
		}
	}
	return missing
}

// ComputeFeaturesAtCutoffNoMatch returns a feature map per player using only precomputed overall form and
// consistency from snapshot tables (per format). Used when there is no match context (matchID 0). Venue and
// opposition are set to 0 (no context). Weather is 0 until weather data is available. Missing precomputed
// values (e.g. new/debut players) are filled with 0 and a warning is logged; run precompute for complete data.
func ComputeFeaturesAtCutoffNoMatch(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
) (map[int64]map[string]float64, error) {
	if len(playerIDs) == 0 {
		return map[int64]map[string]float64{}, nil
	}
	formatID, err := db.GetGlobalCache().GetFormatID(ctx, strings.TrimSpace(strings.ToUpper(format)))
	if err != nil {
		return nil, fmt.Errorf("resolve format for features: %w", err)
	}
	emptyOpps := make(map[int64]struct{ BattingOpp, BowlingOpp *int64 })
	precomp, err := getPrecomputedFeaturesForMatch(ctx, cutoff, formatID, nil, emptyOpps, playerIDs)
	if err != nil {
		return nil, err
	}
	if precomp == nil {
		precomp = make(map[int64]map[string]float64)
	}
	// When precomputed features are missing (e.g. debut players), fill with 0 to avoid pipeline failure.
	// Monitor warning frequency; high rates may warrant improving the precompute process to cover more players.
	if m := missingPrecomputedKeys(precomp, playerIDs, requiredPrecomputedKeysNoMatch); len(m) > 0 {
		slog.Warn("precomputed features missing; filling with 0 for new/debut players",
			slog.String("format", format),
			slog.String("missing", strings.Join(m, "; ")),
		)
		// Ensure keys exist so pc[key] returns 0 when building feats
		for _, pid := range playerIDs {
			pc := precomp[pid]
			if pc == nil {
				precomp[pid] = make(map[string]float64)
				continue
			}
			for _, k := range requiredPrecomputedKeysNoMatch {
				if _, ok := pc[k]; !ok {
					pc[k] = 0
				}
			}
		}
	}
	out := make(map[int64]map[string]float64)
	for _, pid := range playerIDs {
		pc := precomp[pid]
		if pc == nil {
			pc = make(map[string]float64)
		}
		batForm := pc["batting_form"]
		bowlForm := pc["bowling_form"]
		feats := map[string]float64{
			"batting_form":        batForm,
			"batting_form_short":  batForm,
			"batting_form_long":   batForm,
			"batting_momentum":    0,
			"batting_consistency": pc["batting_consistency"],
			"bowling_form":        bowlForm,
			"bowling_form_short":  bowlForm,
			"bowling_form_long":   bowlForm,
			"bowling_momentum":    0,
			"bowling_career_avg":  bowlForm,
			"bowling_consistency": pc["bowling_consistency"],
			"batting_venue":       0,
			"batting_opposition":  0,
			"bowling_venue":       0,
			"bowling_opposition":  0,
			"venue":               0,
			"opposition":          0,
			"season":              0,
			"batting_temp":        0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
			"batting_inning": 1, "batting_session": 1, "toss": 0, "bowling_session": 1,
			"match_date_unix": float64(cutoff.Unix()),
		}
		ensureContractKeys(feats)
		out[pid] = feats
	}
	return out, nil
}

// ComputeFeaturesAtCutoffForMatch returns a feature map per player using only precomputed form, consistency,
// venue, and opposition from the snapshot tables (populated by the precompute cmd per format). Missing
// precomputed values (e.g. new/debut players with no prior history) are filled with 0 and a warning is logged;
// run precompute for complete data. Weather is set to 0 until weather data is available. Season comes from match context.
func ComputeFeaturesAtCutoffForMatch(
	ctx context.Context,
	matchID int64,
	cutoff time.Time,
	playerIDs []int64,
) (map[int64]map[string]float64, error) {
	mctx, err := db.GetMatchFeatureContext(ctx, matchID)
	if err != nil {
		return nil, err
	}
	playerOpps := make(map[int64]struct{ BattingOpp, BowlingOpp *int64 })
	for _, po := range mctx.PlayerOpps {
		playerOpps[po.PlayerID] = struct{ BattingOpp, BowlingOpp *int64 }{
			BattingOpp: po.BattingOppositionID,
			BowlingOpp: po.BowlingOppositionID,
		}
	}
	precomp, err := getPrecomputedFeaturesForMatch(ctx, cutoff, mctx.FormatID, mctx.VenueID, playerOpps, playerIDs)
	if err != nil {
		return nil, err
	}
	if precomp == nil {
		precomp = make(map[int64]map[string]float64)
	}
	requiredMatch := append([]string(nil), requiredPrecomputedKeysForMatchAll...)
	if mctx.VenueID != nil && *mctx.VenueID != 0 {
		requiredMatch = append(requiredMatch, "batting_venue", "bowling_venue", "venue")
	}
	hasOpposition := false
	for _, po := range playerOpps {
		if po.BattingOpp != nil && *po.BattingOpp != 0 || po.BowlingOpp != nil && *po.BowlingOpp != 0 {
			hasOpposition = true
			break
		}
	}
	if hasOpposition {
		requiredMatch = append(requiredMatch, "batting_opposition", "bowling_opposition", "opposition")
	}
	if m := missingPrecomputedKeys(precomp, playerIDs, requiredMatch); len(m) > 0 {
		slog.Warn("precomputed features missing; filling with 0 for new/debut players",
			slog.Int64("match_id", matchID),
			slog.String("missing", strings.Join(m, "; ")),
		)
		// Ensure keys exist so pc[key] returns 0 when building feats
		for _, pid := range playerIDs {
			pc := precomp[pid]
			if pc == nil {
				precomp[pid] = make(map[string]float64)
				continue
			}
			for _, k := range requiredMatch {
				if _, ok := pc[k]; !ok {
					pc[k] = 0
				}
			}
		}
	}
	out := make(map[int64]map[string]float64)
	for _, pid := range playerIDs {
		pc := precomp[pid]
		if pc == nil {
			pc = make(map[string]float64)
		}
		venue := 0.0
		if v, ok := pc["venue"]; ok {
			venue = v
		}
		if v, ok := pc["batting_venue"]; ok {
			venue = v
		}
		opposition := 0.0
		if v, ok := pc["opposition"]; ok {
			opposition = v
		}
		if v, ok := pc["batting_opposition"]; ok {
			opposition = v
		}
		batVenue, bowlVenue, batOpp, bowlOpp := 0.0, 0.0, 0.0, 0.0
		if v, ok := pc["batting_venue"]; ok {
			batVenue = v
		}
		if v, ok := pc["bowling_venue"]; ok {
			bowlVenue = v
		}
		if v, ok := pc["batting_opposition"]; ok {
			batOpp = v
		}
		if v, ok := pc["bowling_opposition"]; ok {
			bowlOpp = v
		}
		season := 0.0
		if mctx.SeasonID != nil && *mctx.SeasonID != 0 {
			season = float64(*mctx.SeasonID)
		}
		batForm := pc["batting_form"]
		bowlForm := pc["bowling_form"]
		feats := map[string]float64{
			"batting_form":        batForm,
			"batting_form_short":  batForm,
			"batting_form_long":   batForm,
			"batting_momentum":    0,
			"batting_consistency": pc["batting_consistency"],
			"batting_venue":       batVenue,
			"batting_opposition":  batOpp,
			"bowling_form":        bowlForm,
			"bowling_form_short":  bowlForm,
			"bowling_form_long":   bowlForm,
			"bowling_momentum":    0,
			"bowling_career_avg":  bowlForm,
			"bowling_consistency": pc["bowling_consistency"],
			"bowling_venue":       bowlVenue,
			"bowling_opposition":  bowlOpp,
			"venue":               venue,
			"opposition":          opposition,
			"season":              season,
			"batting_temp":        0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
			"batting_inning": 1, "batting_session": 1, "toss": 0, "bowling_session": 1,
			"match_date_unix": float64(cutoff.Unix()),
		}
		ensureContractKeys(feats)
		out[pid] = feats
	}
	return out, nil
}
