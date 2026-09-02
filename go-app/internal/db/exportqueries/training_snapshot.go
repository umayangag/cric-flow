package exportqueries

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// ensureContractKeys fills all canonical contract keys (configs/feature_vectors.json) with 0 when absent.
func ensureContractKeys(feats map[string]float64) {
	allFeatureNames := append(features.BattingFeatureNames(), features.BowlingFeatureNames()...)
	for _, k := range allFeatureNames {
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
	values := r.Values()
	strs := make([]string, len(values))
	for i, val := range values {
		switch v := val.(type) {
		case float64:
			strs[i] = strconv.FormatFloat(v, 'g', -1, 64)
		case int:
			strs[i] = strconv.Itoa(v)
		}
	}
	return strs
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

// getPrecomputedFeaturesForMatch reads raw windowed stats and venue/opposition from
// feature_raw_stats_snapshots (populated by the precompute cmd per format). Returns a map of
// playerID -> feature name -> value. Venue and opposition use mean_w5 as the scalar for display/context.
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

	// 1) Venue scope (when match has a venue): use mean_w5 as venue effect
	if venueID != nil && *venueID != 0 {
		rows, err := db.Pool.Query(ctx, `
			SELECT DISTINCT ON (player_id) player_id, batting_mean_w5, bowling_mean_w5
			FROM feature_raw_stats_snapshots
			WHERE player_id = ANY($1::bigint[]) AND format_id = $2 AND scope = 'venue' AND scope_id = $3 AND as_of_date <= $4
			ORDER BY player_id, as_of_date DESC
		`, playerIDs, formatID, *venueID, cutoffDate)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pid int64
			var batVenue, bowlVenue float64
			if err := rows.Scan(&pid, &batVenue, &bowlVenue); err != nil {
				rows.Close()
				return nil, err
			}
			out[pid]["batting_venue"] = batVenue
			out[pid]["bowling_venue"] = bowlVenue
			out[pid]["venue"] = batVenue
		}
		rows.Close()
	}

	// 2) Opposition scope: (player_id, scope_id) pairs; use mean_w5 as opposition effect
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
		rows, err := db.Pool.Query(ctx, `
			SELECT DISTINCT ON (f.player_id, f.scope_id) f.player_id, f.scope_id, f.batting_mean_w5, f.bowling_mean_w5
			FROM feature_raw_stats_snapshots f
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

	// 3) Overall raw windowed stats (scope=overall) for v2 contract
	rows, err := db.Pool.Query(ctx, rawStatsSnapshotQuery, playerIDs, formatID, cutoffDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var bat, bowl features.RawStats
		dest := make([]any, 0, 1+18+18)
		dest = append(dest, &pid)
		dest = append(dest, bat.ScanDest()...)
		dest = append(dest, bowl.ScanDest()...)
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return nil, err
		}
		m := out[pid]
		putRawStatsIntoMap(m, "batting_", bat)
		putRawStatsIntoMap(m, "bowling_", bowl)
	}
	rows.Close()

	return out, nil
}

// putRawStatsIntoMap writes RawStats fields into the feature map with the given prefix (e.g. "batting_", "bowling_").
func putRawStatsIntoMap(m map[string]float64, prefix string, r features.RawStats) {
	m[prefix+"mean_w3"] = r.MeanW3
	m[prefix+"mean_w5"] = r.MeanW5
	m[prefix+"mean_w10"] = r.MeanW10
	m[prefix+"mean_w20"] = r.MeanW20
	m[prefix+"std_w5"] = r.StdW5
	m[prefix+"std_w10"] = r.StdW10
	m[prefix+"max_w10"] = r.MaxW10
	m[prefix+"min_w10"] = r.MinW10
	m[prefix+"median_w10"] = r.MedianW10
	m[prefix+"last_1"] = r.Last1
	m[prefix+"last_2"] = r.Last2
	m[prefix+"last_3"] = r.Last3
	m[prefix+"career_mean"] = r.CareerMean
	m[prefix+"career_count"] = float64(r.CareerCount)
	m[prefix+"pct_zero_w10"] = r.PctZeroW10
	m[prefix+"trend_w5"] = r.TrendW5
	m[prefix+"days_since_last"] = r.DaysSinceLast
	m[prefix+"innings_in_last_90d"] = float64(r.InningsInLast90D)
}

// rawStatsSnapshotQuery is the SELECT for overall raw windowed stats (v2 contract). Kept as constant for clarity.
const rawStatsSnapshotQuery = `
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
		ORDER BY player_id, as_of_date DESC`

// requiredPrecomputedKeysBase is empty; raw stats replace form/consistency and are optional for new/debut players (filled with 0).
var requiredPrecomputedKeysBase = []string{}

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
	if m := missingPrecomputedKeys(precomp, playerIDs, requiredPrecomputedKeysBase); len(m) > 0 {
		slog.Warn(
			"precomputed features missing; filling with 0 for new/debut players",
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
			for _, k := range requiredPrecomputedKeysBase {
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
		feats := map[string]float64{
			"batting_venue": 0, "batting_opposition": 0, "bowling_venue": 0, "bowling_opposition": 0,
			"venue": 0, "opposition": 0,
			"batting_temp": 0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
			"batting_inning": 1, "batting_session": 1, "toss": 0, "bowling_session": 1,
			"match_month_sin":       math.Sin(2 * math.Pi * float64(cutoff.Month()) / 12.0),
			"match_month_cos":       math.Cos(2 * math.Pi * float64(cutoff.Month()) / 12.0),
			"match_day_of_week_sin": math.Sin(2 * math.Pi * float64(cutoff.Weekday()) / 7.0),
			"match_day_of_week_cos": math.Cos(2 * math.Pi * float64(cutoff.Weekday()) / 7.0),
		}
		for _, k := range features.RawStatsFeatureNames() {
			feats[k] = pc[k]
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
	requiredMatch := append([]string(nil), requiredPrecomputedKeysBase...)
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
		slog.Warn(
			"precomputed features missing; filling with 0 for new/debut players",
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
		feats := map[string]float64{
			"batting_venue":      batVenue,
			"batting_opposition": batOpp,
			"bowling_venue":      bowlVenue,
			"bowling_opposition": bowlOpp,
			"venue":              venue,
			"opposition":         opposition,
			"batting_temp":       0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
			"batting_inning": 1, "batting_session": 1, "toss": 0, "bowling_session": 1,
			"match_month_sin":       math.Sin(2 * math.Pi * float64(cutoff.Month()) / 12.0),
			"match_month_cos":       math.Cos(2 * math.Pi * float64(cutoff.Month()) / 12.0),
			"match_day_of_week_sin": math.Sin(2 * math.Pi * float64(cutoff.Weekday()) / 7.0),
			"match_day_of_week_cos": math.Cos(2 * math.Pi * float64(cutoff.Weekday()) / 7.0),
		}
		for _, k := range features.RawStatsFeatureNames() {
			feats[k] = pc[k]
		}
		ensureContractKeys(feats)
		out[pid] = feats
	}
	return out, nil
}
