package availability

import (
	"fmt"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// Evidence is everything the criteria are allowed to read about one player. It is a
// value, not a query, so a criterion cannot reach the database and cannot therefore
// depend on when it happens to run.
//
// BirthDate and CareerEnd are X-1a's facts and are zero until X-1a lands. That is why a
// criterion reports Unavailable rather than false when its evidence is missing: "we have
// no age for him" and "he is not old enough" are different answers, and only the second
// is a reason to leave a claim uncorroborated.
type Evidence struct {
	PlayerID int64
	// LastPlayed is the player's most recent appearance in *any* format. Retirement is
	// not per format -- a Test specialist still playing T20 has not retired -- so the
	// inactivity criterion looks wider than the pool window does. Zero means he has
	// never appeared, which corroborates nothing: it is a data gap, not a career end.
	LastPlayed time.Time
	// BirthDate is X-1a's date of birth. Zero until X-1a lands.
	BirthDate time.Time
	// CareerEnd is X-1a's Wikidata career end date. Zero until X-1a lands.
	CareerEnd time.Time
}

// Subject is one corroboration question: this player's evidence, the format the claim
// was made in, and the moment the question is asked. Promotion is evaluated at flag
// time, so `At` is that moment and is passed in rather than read from the clock, which
// is what makes the rules testable.
type Subject struct {
	Evidence   Evidence
	FormatCode string
	At         time.Time
}

// Verdict is one criterion's answer.
//
// Unavailable is not "no". A criterion whose evidence does not exist yet says so, so the
// ledger can report "nothing corroborated this, and here is what could not be checked"
// instead of implying the checks were made and failed.
type Verdict struct {
	Corroborated bool
	Unavailable  bool
	// Detail is the evidence the verdict was reached on, recorded beside a promotion so
	// it can be read back years later. It is prose for a person, not a parsed field.
	Detail string
}

// Criterion is one independent check that can corroborate a user's retirement claim.
//
// The interface is deliberately narrow and pure: a name, and a function from evidence to
// a verdict. Adding X-1a's sources means adding entries to Criteria, not changing this
// contract, the ledger, or the schema.
type Criterion interface {
	Name() string
	Corroborates(subject Subject) Verdict
}

// Criterion names. They are recorded in player_status.promoted_criterion and in the
// event log, so they are values a person reads years later; they do not change once
// written.
const (
	// CriterionInactivity is (a): no match in any format for N years.
	CriterionInactivity = "inactivity"

	// CriterionCareerEnd is (b): a Wikidata career end date before the cutoff. Needs
	// X-1a.
	CriterionCareerEnd = "career_end"

	// CriterionAgeAndInactivity is (c): age above a per-format bound with M years
	// inactive. Needs X-1a.
	CriterionAgeAndInactivity = "age_and_inactivity"
)

// Criteria returns the corroboration criteria, in the order they are tried. Inactivity
// is first because it is the only one whose evidence this repository already holds;
// the other two are registered now, report themselves unavailable, and start answering
// the day X-1a fills their fields in.
func Criteria(cfg *config.Config) []Criterion {
	return []Criterion{
		inactivityCriterion{years: config.RetirementInactiveYears(cfg)},
		careerEndCriterion{},
		ageAndInactivityCriterion{
			ageBounds:     ageBounds(cfg),
			fallbackBound: config.DefaultRetirementAgeBoundYears,
			inactiveYears: config.RetirementAgeInactiveYears(cfg),
		},
	}
}

// ageBounds returns the configured per-format age bounds, or nil when none are set.
func ageBounds(cfg *config.Config) map[string]int {
	if cfg == nil {
		return nil
	}
	return cfg.Pool.Retirement.AgeBoundYears
}

// inactivityCriterion is (a): the player has appeared in no format for N years.
//
// N is 5 by default, and that is measured rather than picked. Over the last year of real
// matches, 0.060% of the player-matches actually fielded were a return after a five-year
// absence from every format (30 players); at four years it is 0.117%, at three 0.228%.
// Five years is where a promotion to a global fact stops being a bet. The table is in
// docs/EXTERNAL_DATA_PLAN.md § D-12.
type inactivityCriterion struct{ years int }

func (c inactivityCriterion) Name() string { return CriterionInactivity }

func (c inactivityCriterion) Corroborates(subject Subject) Verdict {
	if subject.Evidence.LastPlayed.IsZero() {
		return Verdict{Unavailable: true, Detail: "no appearance on record in any format"}
	}
	threshold := subject.At.AddDate(-c.years, 0, 0)
	if subject.Evidence.LastPlayed.After(threshold) {
		return Verdict{Detail: fmt.Sprintf(
			"last appeared %s, inside the %d-year inactivity bound",
			subject.Evidence.LastPlayed.Format(time.DateOnly), c.years)}
	}
	return Verdict{Corroborated: true, Detail: fmt.Sprintf(
		"no appearance in any format since %s (%d-year bound)",
		subject.Evidence.LastPlayed.Format(time.DateOnly), c.years)}
}

// careerEndCriterion is (b): an external career end date before the moment of the claim.
// Its evidence arrives with X-1a; until then it reports unavailable, which is the honest
// answer and not a "no".
type careerEndCriterion struct{}

func (c careerEndCriterion) Name() string { return CriterionCareerEnd }

func (c careerEndCriterion) Corroborates(subject Subject) Verdict {
	if subject.Evidence.CareerEnd.IsZero() {
		return Verdict{Unavailable: true, Detail: "no external career end date on record (needs X-1a)"}
	}
	if subject.Evidence.CareerEnd.Before(subject.At) {
		return Verdict{Corroborated: true, Detail: fmt.Sprintf(
			"external career end date %s", subject.Evidence.CareerEnd.Format(time.DateOnly))}
	}
	return Verdict{Detail: fmt.Sprintf(
		"external career end date %s is not in the past",
		subject.Evidence.CareerEnd.Format(time.DateOnly))}
}

// ageAndInactivityCriterion is (c): older than the format's bound and inactive for M
// years. Two weak signals that corroborate together and neither of which would on its
// own -- a 41-year-old who played last month has not retired, and a two-year gap at 28
// is an injury.
//
// The age bound is per format because the formats do not end a career at the same age.
// Its evidence arrives with X-1a; until then it reports unavailable.
type ageAndInactivityCriterion struct {
	ageBounds     map[string]int
	fallbackBound int
	inactiveYears int
}

func (c ageAndInactivityCriterion) Name() string { return CriterionAgeAndInactivity }

func (c ageAndInactivityCriterion) Corroborates(subject Subject) Verdict {
	if subject.Evidence.BirthDate.IsZero() {
		return Verdict{Unavailable: true, Detail: "no date of birth on record (needs X-1a)"}
	}
	if subject.Evidence.LastPlayed.IsZero() {
		return Verdict{Unavailable: true, Detail: "no appearance on record in any format"}
	}
	bound := c.boundFor(subject.FormatCode)
	age := yearsBetween(subject.Evidence.BirthDate, subject.At)
	inactiveSince := subject.At.AddDate(-c.inactiveYears, 0, 0)
	if age <= bound {
		return Verdict{Detail: fmt.Sprintf("age %d is within the %s bound of %d", age, subject.FormatCode, bound)}
	}
	if subject.Evidence.LastPlayed.After(inactiveSince) {
		return Verdict{Detail: fmt.Sprintf(
			"age %d is above the %s bound of %d but he appeared on %s, inside the %d-year bound",
			age, subject.FormatCode, bound,
			subject.Evidence.LastPlayed.Format(time.DateOnly), c.inactiveYears)}
	}
	return Verdict{Corroborated: true, Detail: fmt.Sprintf(
		"age %d is above the %s bound of %d and no appearance since %s (%d-year bound)",
		age, subject.FormatCode, bound,
		subject.Evidence.LastPlayed.Format(time.DateOnly), c.inactiveYears)}
}

// boundFor returns the age bound for a format, falling back to the shared default where
// the config names no bound for it.
func (c ageAndInactivityCriterion) boundFor(formatCode string) int {
	if bound, ok := c.ageBounds[formats.CanonicalizeCode(formatCode)]; ok && bound > 0 {
		return bound
	}
	return c.fallbackBound
}

// yearsBetween returns completed years from `from` to `to`.
func yearsBetween(from, to time.Time) int {
	years := to.Year() - from.Year()
	if to.YearDay() < from.YearDay() {
		years--
	}
	return years
}
