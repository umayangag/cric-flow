package exportqueries

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

type battingSnapshotAtCutoff struct {
	form        float64
	formShort   float64
	formLong    float64
	momentum    float64
	consistency float64
	venue       float64
	opposition  float64
}

type bowlingSnapshotAtCutoff struct {
	form        float64
	formShort   float64
	formLong    float64
	momentum    float64
	consistency float64
	venue       float64
	opposition  float64
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

// computeBowlingSnapshotFromHistories computes form, form_short, form_long, momentum, consistency, venue, opposition
// from pre-fetched histories (avoids N+1 when batching).
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

// ComputeFeaturesAtCutoffForFutureMatch returns a feature map per player for a hypothetical future match.
// Used when predicting team selection: same venue and opposition for all players (the opposition team).
// Missing precomputed values are filled with 0 to support new/auction players with no prior history.
// When weather is non-nil, its values override the default 0 for batting_* and bowling_* weather features.
func ComputeFeaturesAtCutoffForFutureMatch(
	ctx context.Context,
	cutoff time.Time,
	format string,
	venueID *int64,
	oppositionID int64,
	seasonID *int64,
	playerIDs []int64,
	weather *WeatherOverride,
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
			"bowling_consistency": get("bowling_consistency"),
			"bowling_venue":       get("bowling_venue"),
			"bowling_opposition":  get("bowling_opposition"),
			"venue":               get("venue"),
			"opposition":          get("opposition"),
			"season":              season,
			"batting_temp":        wt, "batting_wind": ww, "batting_rain": wr, "batting_humidity": wh, "batting_cloud": wc, "batting_pressure": wp, "batting_viscosity": 0,
			"bowling_temp": wt, "bowling_wind": ww, "bowling_rain": wr, "bowling_humidity": wh, "bowling_cloud": wc, "bowling_pressure": wp, "bowling_viscosity": 0,
		}
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
// opposition are set to 0 (no context). Weather is 0 until weather data is available. Returns error if any
// required precomputed value (batting_form, batting_consistency, bowling_form, bowling_consistency) is missing.
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
	if m := missingPrecomputedKeys(precomp, playerIDs, requiredPrecomputedKeysNoMatch); len(m) > 0 {
		return nil, fmt.Errorf(
			"precomputed features required (run precompute for format %s): %s",
			format,
			strings.Join(m, "; "),
		)
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
		}
		out[pid] = feats
	}
	return out, nil
}

// ComputeFeaturesAtCutoffForMatch returns a feature map per player using only precomputed form, consistency,
// venue, and opposition from the snapshot tables (populated by the precompute cmd per format). No averages or
// on-the-fly computation: if any required value is missing, returns error. Weather is set to 0 until weather
// data is available. Season comes from match context.
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
		return nil, fmt.Errorf(
			"precomputed features required (run precompute for this format): %s",
			strings.Join(m, "; "),
		)
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
			"bowling_consistency": pc["bowling_consistency"],
			"bowling_venue":       bowlVenue,
			"bowling_opposition":  bowlOpp,
			"venue":               venue,
			"opposition":          opposition,
			"season":              season,
			"batting_temp":        0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
		}
		out[pid] = feats
	}
	return out, nil
}
