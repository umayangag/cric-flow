package availability_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// now is the moment every criterion test asks its question at. It is a value rather than
// the clock because a rule that answers differently tomorrow cannot be pinned.
var now = date(2026, 9, 3)

// criterionNamed returns the registered criterion with that name.
func criterionNamed(t *testing.T, cfg *config.Config, name string) availability.Criterion {
	t.Helper()
	for _, criterion := range availability.Criteria(cfg) {
		if criterion.Name() == name {
			return criterion
		}
	}
	require.FailNowf(t, "criterion not registered", "no criterion named %q", name)
	return nil
}

// TestInactivityCriterion_CorroboratesOnlyBeyondTheBound pins criterion (a) at its
// boundary. Five years is the measured default: 0.060% of the player-matches actually
// fielded in the last year of data were a return after that long away from every format.
func TestInactivityCriterion_CorroboratesOnlyBeyondTheBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		lastPlayed       time.Time
		wantCorroborated bool
		wantUnavailable  bool
	}{
		{
			name:             "a decade away corroborates",
			lastPlayed:       date(2016, 5, 1),
			wantCorroborated: true,
		},
		{
			name:             "one day past the five-year bound corroborates",
			lastPlayed:       date(2021, 9, 2),
			wantCorroborated: true,
		},
		{
			name:             "exactly five years is the bound, and the bound corroborates",
			lastPlayed:       date(2021, 9, 3),
			wantCorroborated: true,
		},
		{
			name:       "one day inside five years does not",
			lastPlayed: date(2021, 9, 4),
		},
		{
			name:       "a player who played last month has not retired",
			lastPlayed: date(2026, 8, 1),
		},
		{
			name:            "no appearance on record is a data gap, not a career end",
			lastPlayed:      time.Time{},
			wantUnavailable: true,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			criterion := criterionNamed(t, nil, availability.CriterionInactivity)

			verdict := criterion.Corroborates(availability.Subject{
				Evidence:   availability.Evidence{PlayerID: 1, LastPlayed: testCase.lastPlayed},
				FormatCode: "T20I",
				At:         now,
			})

			assert.Equal(t, testCase.wantCorroborated, verdict.Corroborated)
			assert.Equal(t, testCase.wantUnavailable, verdict.Unavailable)
			assert.NotEmpty(t, verdict.Detail, "every verdict records what it read")
		})
	}
}

// TestInactivityCriterion_HonoursTheConfiguredBound keeps the threshold a setting rather
// than a constant: it is a measurement of one dataset.
func TestInactivityCriterion_HonoursTheConfiguredBound(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Pool.Retirement.InactiveYears = 2
	criterion := criterionNamed(t, cfg, availability.CriterionInactivity)

	verdict := criterion.Corroborates(availability.Subject{
		Evidence: availability.Evidence{PlayerID: 1, LastPlayed: date(2023, 1, 1)},
		At:       now,
	})

	assert.True(t, verdict.Corroborated, "three years away is beyond a two-year bound")
}

// TestCareerEndCriterion_IsUnavailableWithoutACareerEndDate is the answer for the player
// X-1a matched nothing for -- and, since Wikidata states an end of work period for three
// players in this registry, for nearly everyone. "We have no career end date" is not "he
// has not retired", and reporting the first as the second would make the criterion look
// like it had checked.
func TestCareerEndCriterion_IsUnavailableWithoutACareerEndDate(t *testing.T) {
	t.Parallel()
	criterion := criterionNamed(t, nil, availability.CriterionCareerEnd)

	verdict := criterion.Corroborates(availability.Subject{
		Evidence: availability.Evidence{PlayerID: 1, LastPlayed: date(2015, 1, 1)},
		At:       now,
	})

	assert.True(t, verdict.Unavailable)
	assert.False(t, verdict.Corroborated)
	assert.Contains(t, verdict.Detail, "no external career end date on record")
}

// TestCareerEndCriterion_CorroboratesAPastCareerEnd exercises the branch X-1a's acquired
// career end date switches on.
func TestCareerEndCriterion_CorroboratesAPastCareerEnd(t *testing.T) {
	t.Parallel()
	criterion := criterionNamed(t, nil, availability.CriterionCareerEnd)

	past := criterion.Corroborates(availability.Subject{
		Evidence: availability.Evidence{PlayerID: 1, CareerEnd: date(2019, 11, 15)},
		At:       now,
	})
	future := criterion.Corroborates(availability.Subject{
		Evidence: availability.Evidence{PlayerID: 1, CareerEnd: date(2030, 1, 1)},
		At:       now,
	})

	assert.True(t, past.Corroborated)
	assert.Contains(t, past.Detail, "2019-11-15")
	assert.False(t, future.Corroborated, "a career that has not ended yet corroborates nothing")
}

// TestAgeAndInactivityCriterion_NeedsBothHalves pins the combination rule: age alone is
// not retirement and a two-year gap alone is an injury.
func TestAgeAndInactivityCriterion_NeedsBothHalves(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		birthDate        time.Time
		lastPlayed       time.Time
		formatCode       string
		wantCorroborated bool
		wantUnavailable  bool
	}{
		{
			name:             "old and long inactive corroborates",
			birthDate:        date(1980, 1, 1),
			lastPlayed:       date(2023, 1, 1),
			formatCode:       "ODI",
			wantCorroborated: true,
		},
		{
			name:       "old but still playing does not",
			birthDate:  date(1980, 1, 1),
			lastPlayed: date(2026, 8, 1),
			formatCode: "ODI",
		},
		{
			name:       "long inactive but young does not",
			birthDate:  date(1998, 1, 1),
			lastPlayed: date(2023, 1, 1),
			formatCode: "ODI",
		},
		{
			name:            "no date of birth leaves the criterion unable to answer",
			lastPlayed:      date(2015, 1, 1),
			formatCode:      "ODI",
			wantUnavailable: true,
		},
		{
			name:            "no appearance leaves it unable to answer too",
			birthDate:       date(1980, 1, 1),
			formatCode:      "ODI",
			wantUnavailable: true,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			criterion := criterionNamed(t, nil, availability.CriterionAgeAndInactivity)

			verdict := criterion.Corroborates(availability.Subject{
				Evidence: availability.Evidence{
					PlayerID:   1,
					BirthDate:  testCase.birthDate,
					LastPlayed: testCase.lastPlayed,
				},
				FormatCode: testCase.formatCode,
				At:         now,
			})

			assert.Equal(t, testCase.wantCorroborated, verdict.Corroborated)
			assert.Equal(t, testCase.wantUnavailable, verdict.Unavailable)
		})
	}
}

// TestAgeAndInactivityCriterion_UsesThePerFormatBound keeps the age bound per format:
// the formats do not end a career at the same age, and a shared number would say they do.
func TestAgeAndInactivityCriterion_UsesThePerFormatBound(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Pool.Retirement.AgeBoundYears = map[string]int{"TEST": 35, "T20": 50}
	criterion := criterionNamed(t, cfg, availability.CriterionAgeAndInactivity)
	evidence := availability.Evidence{PlayerID: 1, BirthDate: date(1985, 1, 1), LastPlayed: date(2022, 1, 1)}

	inTest := criterion.Corroborates(availability.Subject{Evidence: evidence, FormatCode: "TEST", At: now})
	inT20 := criterion.Corroborates(availability.Subject{Evidence: evidence, FormatCode: "T20", At: now})

	assert.True(t, inTest.Corroborated, "41 is above the Test bound of 35")
	assert.False(t, inT20.Corroborated, "41 is below the T20 bound of 50")
}

// TestCriteria_AreRegisteredInOrder guards the pluggability the plan asks for: X-1a adds
// entries to this list, and nothing else has to change.
func TestCriteria_AreRegisteredInOrder(t *testing.T) {
	t.Parallel()

	criteria := availability.Criteria(nil)
	names := make([]string, 0, len(criteria))
	for _, criterion := range criteria {
		names = append(names, criterion.Name())
	}

	assert.Equal(t, []string{"inactivity", "career_end", "age_and_inactivity"}, names)
}
