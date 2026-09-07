package opsstatus

import (
	"context"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/freshness"
)

// Freshness is the one freshness object on /ops/status: three named facts and nothing
// else.
//
// It replaced `db_freshness`, which bucketed the database's latest match per format at 7
// and 30 days and rolled a worst-of `overall` badge — a second rule, with a second
// threshold, answering a question nobody had defined. On 2026-09-04 the two disagreed:
// `db_freshness` read *stale* because TEST's latest match was 8 days old, while H-11 read
// *fresh* at 2 days of 14. Neither was wrong; they measured different things and nothing
// said which one a badge meant. A Test played a fortnight apart is not a fault, so the
// database's lag is reported here as a dated fact and never as a verdict (P2-1).
type Freshness struct {
	// Served is H-11's verdict, copied through from ml-service. The only thing that
	// decides whether a prediction would be refused, and the only badge.
	Served ServedFreshness `json:"served"`
	// Database is the per-format lag of the import, as facts: the latest match date and
	// its age in days. No bucket, no status.
	Database map[string]FormatLag `json:"database"`
	// RetrainDue is whether the database holds matches the served run never saw.
	RetrainDue RetrainStatus `json:"retrain_due"`
}

// ServedFreshness is H-11's verdict as ml-service reports it, plus the word that spells
// it. Every field but Status is copied through verbatim; Status is the vocabulary applied
// to those fields, not a second computation — in particular the age is never recompared
// against a limit of go-app's own, because go-app has none.
type ServedFreshness struct {
	Status         string  `json:"status"`
	Fresh          bool    `json:"fresh"`
	AgeDays        *int    `json:"age_days"`
	MaxAgeDays     *int    `json:"max_age_days"`
	RatingsThrough *string `json:"ratings_through"`
	Code           *string `json:"code"`
}

// FormatLag is how far behind one format's imported data is: a date and a number of days.
// Note carries the reason when there is no date to report, so an empty lag and an
// unreadable database do not look the same on the wire (§8.7).
type FormatLag struct {
	LatestMatchDate *string `json:"latest_match_date"`
	AgeDays         *int    `json:"age_days"`
	MatchCount      int64   `json:"match_count"`
	Note            string  `json:"note,omitempty"`
}

// RetrainStatus answers "has anything been imported that the served run never saw?" —
// the B-2 state a green pipeline once hid, now on the wire with the date that produced
// it and the format that holds it.
type RetrainStatus struct {
	Status          string  `json:"status"`
	DaysBehind      *int    `json:"days_behind"`
	LatestMatchDate *string `json:"latest_match_date"`
	Format          string  `json:"format,omitempty"`
}

const (
	noteDatabaseUnreadable = "the database could not be read for this format"
	noteNoMatches          = "no matches of this format have been imported"
	noteNoProbe            = "no database connection, so no import date is known"
)

// BuildFreshnessSection assembles the one freshness object.
//
// go-app is the only component that sees both the database and ml-service, which is why
// the assembly is here rather than in either of them. `artifacts` is ml-service's
// /artifacts/status answer as BuildArtifactsSection copied it through; the verdict is
// read out of it and never recomputed.
func BuildFreshnessSection(
	ctx context.Context,
	probe InsightsProbe,
	artifacts map[string]any,
	now time.Time,
) Freshness {
	served := readServedVerdict(artifacts)
	database, latestMatch, latestFormat := buildDatabaseLag(ctx, probe, now)
	return Freshness{
		Served:     served,
		Database:   database,
		RetrainDue: buildRetrainStatus(served, latestMatch, latestFormat),
	}
}

// readServedVerdict copies H-11's verdict off ml-service's answer.
//
// A missing verdict reads `unknown`: the artifacts section falls back to a filesystem
// scan when ml-service is unreachable, and a scan can say what is on disk but never what
// is loaded or how old it is. Reporting `fresh` there would be inventing the one fact
// this object exists to carry.
func readServedVerdict(artifacts map[string]any) ServedFreshness {
	verdict, ok := artifacts["ratings"].(map[string]any)
	if !ok {
		return ServedFreshness{Status: freshness.Unknown}
	}
	served := ServedFreshness{
		Fresh:          verdict["fresh"] == true,
		AgeDays:        readIntPointer(verdict["age_days"]),
		MaxAgeDays:     readIntPointer(verdict["max_age_days"]),
		RatingsThrough: readStringPointer(verdict["ratings_through"]),
		Code:           readStringPointer(verdict["code"]),
	}
	served.Status = spellVerdict(served)
	return served
}

// spellVerdict is the vocabulary applied to the copied verdict. "Nothing loaded" is a
// state ml-service reports as `fresh: false` with no date, and calling that `stale` would
// tell an operator to retrain when what they have to do is reload.
func spellVerdict(served ServedFreshness) string {
	switch {
	case served.RatingsThrough == nil:
		return freshness.NotLoaded
	case served.Fresh:
		return freshness.Fresh
	default:
		return freshness.Stale
	}
}

// buildDatabaseLag reports each format's latest imported match and its age, and returns
// the newest match date across every format with the format that holds it — which is the
// date the retrain-due comparison is made on.
func buildDatabaseLag(
	ctx context.Context,
	probe InsightsProbe,
	now time.Time,
) (lags map[string]FormatLag, latestMatch time.Time, latestFormat string) {
	lags = make(map[string]FormatLag, len(CricketFormatCodes))
	for _, code := range CricketFormatCodes {
		lag := FormatLag{}
		if probe == nil {
			lag.Note = noteNoProbe
			lags[code] = lag
			continue
		}
		count, err := probe.CountMatchesByFormat(ctx, code)
		if err != nil {
			slog.Error("failed to count matches by format", "format", code, "err", err)
			lag.Note = noteDatabaseUnreadable
			lags[code] = lag
			continue
		}
		lag.MatchCount = count

		latest, err := probe.LatestMatchDateByFormat(ctx, code)
		switch {
		case err != nil:
			slog.Error("failed to get latest match date", "format", code, "err", err)
			lag.Note = noteDatabaseUnreadable
		case latest.IsZero():
			lag.Note = noteNoMatches
		default:
			latest = latest.UTC()
			date := latest.Format(time.DateOnly)
			days := wholeDaysBetween(latest, now)
			lag.LatestMatchDate = &date
			lag.AgeDays = &days
			if latest.After(latestMatch) {
				latestMatch, latestFormat = latest, code
			}
		}
		lags[code] = lag
	}
	return lags, latestMatch, latestFormat
}

// buildRetrainStatus compares the database's latest match against the date the served
// ratings run through. Matches imported that the served run never saw is exactly the
// state a green pipeline once hid (B-2), so it is a named fact rather than something an
// operator has to spot by reading two dates off two panels.
func buildRetrainStatus(served ServedFreshness, latestMatch time.Time, latestFormat string) RetrainStatus {
	status := RetrainStatus{Status: freshness.Unknown}
	if !latestMatch.IsZero() {
		date := latestMatch.Format(time.DateOnly)
		status.LatestMatchDate = &date
		status.Format = latestFormat
	}
	if served.RatingsThrough == nil || latestMatch.IsZero() {
		return status
	}
	through, err := time.Parse(time.DateOnly, *served.RatingsThrough)
	if err != nil {
		slog.Error("ml-service reported a ratings_through this service cannot parse",
			"ratings_through", *served.RatingsThrough, "err", err)
		return status
	}
	days := wholeDaysBetween(through, latestMatch)
	if days < 0 {
		days = 0
	}
	status.DaysBehind = &days
	status.Status = freshness.UpToDate
	if days > 0 {
		status.Status = freshness.RetrainDue
	}
	return status
}

// wholeDaysBetween counts calendar days between two UTC instants. Both dates come from a
// date column or a YYYY-MM-DD string, so both are midnight UTC and the division is exact.
func wholeDaysBetween(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

func readIntPointer(v any) *int {
	number, ok := v.(float64)
	if !ok {
		return nil
	}
	value := int(number)
	return &value
}

func readStringPointer(v any) *string {
	text, ok := v.(string)
	if !ok || text == "" {
		return nil
	}
	return &text
}
