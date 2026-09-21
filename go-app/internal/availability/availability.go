// Package availability answers the one question about a match that no model can:
// who may play.
//
// Two things live here, because they are two halves of one answer (D-12).
//
// The **recency window** bounds the default candidate pool. The pool used to be
// all-time -- everyone who had ever appeared for the club in the format -- which offered
// players who retired a decade ago, and the rating pass decays a rating per match played
// rather than per year elapsed, so those players still carried the ratings they stopped
// with. Bounding the pool is the product-boundary fix; time-decaying ratings is model
// work and is deliberately not done here.
//
// The **retirement ledger** holds what a user knows and the database does not. A user
// flagging a player retired is a claim, scoped to that user's own pools. It becomes a
// stored fact -- `player.is_retired` -- only when an independent criterion corroborates
// it, and the criterion that did so is recorded beside the promotion. The criteria are a
// list rather than a condition so X-1a's age and career-end facts join them without a
// schema change or a rewrite here.
//
// Nothing in this package reaches the database; `db` implements Store. The L4 harness
// and the E5 gate build sides from fielded XIs and never call any of it.
package availability

import (
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// How a side's candidate pool was chosen. These strings are on the wire: the prediction
// response and the candidate list both name their source, and the console renders the
// name. They are declared here, generated into contracts/ops-console.contract.json, and
// asserted from both sides -- H-24, because a vocabulary re-typed on the far side of a
// boundary is exactly how D-9 and D-10 happened.
const (
	// SourceRecencyWindow is the default: players who appeared for the club in this
	// format within the window ending at the request's cutoff.
	SourceRecencyWindow = "recency_window"

	// SourceAllTime is the old behaviour, kept as a deliberate widening a user asks
	// for: everyone who has ever appeared for the club in the format.
	SourceAllTime = "all_time"

	// SourceManual is the pool a user picked by hand out of one of the other two.
	SourceManual = "manual"
)

// PoolSources returns every pool source, in the order a surface should offer them.
func PoolSources() []string {
	return []string{SourceRecencyWindow, SourceAllTime, SourceManual}
}

// Why a candidate the window offered is not in the pool that was sent. An excluded
// player is never silently gone: the reason travels with him so a surface can show it
// and a user can undo it (§8.7).
const (
	// ReasonUserFlagged is a claim standing on its own -- this user flagged the player
	// retired and no criterion has corroborated it. It hides him from this user's
	// default pools and from nobody else's.
	ReasonUserFlagged = "user_flagged"

	// ReasonRetired is the stored fact: a flag that a criterion corroborated, so
	// `player.is_retired` was raised and every pool honours it.
	ReasonRetired = "retired"

	// ReasonBothSides is the one exclusion the ledger has nothing to do with: this
	// fixture's other side holds the same player, and nobody plays both elevens
	// (GO-04). It happens whenever a recency window covers a transfer -- franchise
	// T20 with a twelve-month window is the ordinary case -- and it is an exclusion
	// rather than a refusal because the fixture is real and one of the two clubs is
	// the one he actually plays for now. The detail says which side kept him and on
	// what evidence, so the choice is visible and can be overridden by hand (§8.7).
	ReasonBothSides = "both_sides"
)

// ExclusionReasons returns every exclusion reason a pool can report.
func ExclusionReasons() []string {
	return []string{ReasonUserFlagged, ReasonRetired, ReasonBothSides}
}

// DefaultActor is whose ledger applies when a request names no user.
//
// A deployment carries a single API key today, so there is one actor and it is this one.
// The name exists rather than an empty string because "nobody's ledger" and "the default
// user's ledger" are different requests: a backtest sends the first (H-19 -- a 2019
// cutoff must see the 2019 pool, and a flag set in 2026 is not evidence about 2019), and
// an upcoming-match request sends the second.
const DefaultActor = "default"

// WindowMonths returns the recency window for a format, in months.
//
// The per-format defaults are measured, not chosen: over the last year of real matches,
// the smallest window covering >= 95% of the players who actually took the field (of
// those the all-time pool would have offered at all). The measurement table is in
// docs/EXTERNAL_DATA_PLAN.md § D-12. A request may override the window; a format the
// config does not name falls back to the fallback constant rather than to all-time,
// because an unbounded pool is the defect.
func WindowMonths(cfg *config.Config, formatCode string) int {
	code := formats.CanonicalizeCode(formatCode)
	if cfg != nil {
		if months, ok := cfg.Pool.RecencyMonths[code]; ok && months > 0 {
			return months
		}
	}
	if months, ok := config.DefaultPoolRecencyMonths[code]; ok {
		return months
	}
	return config.DefaultPoolRecencyMonthsFallback
}

// CalendarDay is midnight UTC of t's own calendar date -- the date as the caller wrote
// it, not the date the same instant falls on in UTC.
//
// A match is keyed by the day it was played on, and that day is a property of the fixture
// rather than of any timezone: `match.match_date` is a `date`, the rating pass keys on a
// date, and a caller writing `2025-03-01T01:00:00-05:00` means the first of March.
//
// It exists because `t.Truncate(24 * time.Hour)` is not that day and cannot be made into
// it. Truncate rounds the *instant* down to a multiple of 24h since the zero time, which
// is midnight UTC, and leaves the value in its own zone; pgx then encodes a `date`
// parameter from the value's own year/month/day. So `2025-03-01T01:00:00-05:00` truncates
// to `2025-02-28T19:00:00-05:00` and reaches Postgres as 28 February -- a pool cutoff a
// day before the fixture, silently dropping the previous day's matches from the window
// while the stored prediction records the first of March (GO-09).
func CalendarDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// WindowStart returns the first match date a recency-bounded pool accepts: the cutoff
// less `months` months, so the window is [start, cutoff) -- half-open at the cutoff
// because the cutoff is the match being predicted and its own result may not be read.
//
// Month arithmetic is clamped rather than normalised. Go's AddDate rolls 31 March less
// one month over into 3 March; a window is a span of months and a user reading "the last
// 12 months" means the same day-of-month a year earlier, or the last day of that month
// where there is no such day. So 31 March less one month is 28 February, not 3 March.
func WindowStart(cutoff time.Time, months int) time.Time {
	if months <= 0 {
		return time.Time{}
	}
	day := cutoff.Day()
	firstOfMonth := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, cutoff.Location())
	shifted := firstOfMonth.AddDate(0, -months, 0)
	if last := daysInMonth(shifted.Year(), shifted.Month()); day > last {
		day = last
	}
	return time.Date(shifted.Year(), shifted.Month(), day, 0, 0, 0, 0, cutoff.Location())
}

// daysInMonth returns the number of days in the given month, leap years included.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
