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
