package exportqueries

import (
	"context"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

// DefaultEWMAlpha is the default alpha for EWM when computing form at cutoff (same as precompute).
const DefaultEWMAlpha = 0.3

// DefaultConsistencyLastN is the default last-N window for consistency when computing at cutoff.
const DefaultConsistencyLastN = 10

// DefaultFormWindowN is the max number of innings to use for form (0 = no limit).
const DefaultFormWindowN = 0

type battingSnapshotAtCutoff struct {
	form        float64
	consistency float64
	venue       float64
	opposition  float64
}

type bowlingSnapshotAtCutoff struct {
	form        float64
	consistency float64
	venue       float64
	opposition  float64
}

func toInnings(in []db.InnVal) []features.Innings {
	out := make([]features.Innings, 0, len(in))
	for _, iv := range in {
		out = append(out, features.Innings{Date: iv.MatchDate, Value: iv.Value})
	}
	return out
}

// computeBattingSnapshotAtCutoff uses the same EWM and Consistency logic as precompute-features.
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
	if alpha <= 0 {
		alpha = DefaultEWMAlpha
	}
	if lastN < 0 {
		lastN = DefaultConsistencyLastN
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

// computeBattingSnapshotFromHistories computes form/consistency/venue/opposition from pre-fetched histories (avoids N+1 when batching).
func computeBattingSnapshotFromHistories(
	mainHist, venueHist, oppHist []db.InnVal,
	asOf time.Time,
	alpha float64,
	lastN, windowN int,
) battingSnapshotAtCutoff {
	out := battingSnapshotAtCutoff{}
	if alpha <= 0 {
		alpha = DefaultEWMAlpha
	}
	if lastN < 0 {
		lastN = DefaultConsistencyLastN
	}
	inn := toInnings(mainHist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
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
	if alpha <= 0 {
		alpha = DefaultEWMAlpha
	}
	if lastN < 0 {
		lastN = DefaultConsistencyLastN
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

// computeBowlingSnapshotFromHistories computes form/consistency/venue/opposition from pre-fetched histories (avoids N+1 when batching).
func computeBowlingSnapshotFromHistories(
	mainHist, venueHist, oppHist []db.InnVal,
	asOf time.Time,
	alpha float64,
	lastN, windowN int,
) bowlingSnapshotAtCutoff {
	out := bowlingSnapshotAtCutoff{}
	if alpha <= 0 {
		alpha = DefaultEWMAlpha
	}
	if lastN < 0 {
		lastN = DefaultConsistencyLastN
	}
	inn := toInnings(mainHist)
	inn = features.SortAndClip(inn, asOf)
	if windowN > 0 && len(inn) > windowN {
		inn = inn[len(inn)-windowN:]
	}
	out.form, _ = features.EWM(inn, alpha)
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

// ComputeFeaturesAtCutoffForMatch returns a feature map per player using the same EWM/Consistency/venue/opposition
// logic as training. Match context (format, venue, per-player batting/bowling opposition) comes from the DB.
// Missing data yields 0 (no averages or baseline). Weather may use averages when available; here we use 0 for missing.
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
	out := make(map[int64]map[string]float64)
	for _, pid := range playerIDs {
		opps := playerOpps[pid]
		bat, _ := computeBattingSnapshotAtCutoff(
			ctx,
			pid,
			cutoff,
			mctx.FormatID,
			mctx.VenueID,
			opps.BattingOpp,
			DefaultEWMAlpha,
			DefaultConsistencyLastN,
			DefaultFormWindowN,
		)
		bowl, _ := computeBowlingSnapshotAtCutoff(
			ctx,
			pid,
			cutoff,
			mctx.FormatID,
			mctx.VenueID,
			opps.BowlingOpp,
			DefaultEWMAlpha,
			DefaultConsistencyLastN,
			DefaultFormWindowN,
		)
		season := 0.0
		if mctx.SeasonID != nil && *mctx.SeasonID != 0 {
			season = float64(*mctx.SeasonID)
		}
		feats := map[string]float64{
			"batting_form":        bat.form,
			"batting_consistency": bat.consistency,
			"batting_venue":       bat.venue,
			"batting_opposition":  bat.opposition,
			"bowling_form":        bowl.form,
			"bowling_consistency": bowl.consistency,
			"bowling_venue":       bowl.venue,
			"bowling_opposition":  bowl.opposition,
			"venue":               bat.venue,
			"opposition":          bat.opposition,
			"season":              season,
			// Weather: use 0 when missing (averages allowed elsewhere; here we keep consistent with "0 for missing")
			"batting_temp": 0, "batting_wind": 0, "batting_rain": 0, "batting_humidity": 0, "batting_cloud": 0, "batting_pressure": 0, "batting_viscosity": 0,
			"bowling_temp": 0, "bowling_wind": 0, "bowling_rain": 0, "bowling_humidity": 0, "bowling_cloud": 0, "bowling_pressure": 0, "bowling_viscosity": 0,
		}
		out[pid] = feats
	}
	return out, nil
}
