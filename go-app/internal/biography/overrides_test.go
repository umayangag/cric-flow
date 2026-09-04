package biography_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// TestParseOverrides_ReadsACuratedRow is the venue-geocoding pattern applied to people: a
// handful of hand-checked facts over an acquisition pass that missed them.
func TestParseOverrides_ReadsACuratedRow(t *testing.T) {
	t.Parallel()

	overrides, err := biography.ParseOverrides(strings.NewReader(`{"players":[
	  {"cricsheet_id":"4d84ad05","note":"PNG squad page, checked 2026-09","birth_date":"1990-04-01",
	   "bowling_style":"off-spin","batting_hand":"right"}
	]}`))

	require.NoError(t, err)
	require.Len(t, overrides, 1)
	assert.Equal(t, "off-spin", overrides["4d84ad05"].BowlingStyle)
}

// TestParseOverrides_RefusesInvalidRows: a curated file nobody checks is worse than no
// file, because afterwards it looks exactly like measured data.
func TestParseOverrides_RefusesInvalidRows(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		payload string
		wants   string
	}{
		{
			name:    "no cricsheet id",
			payload: `{"players":[{"note":"somewhere"}]}`,
			wants:   "cricsheet_id",
		},
		{
			name:    "no note saying where the fact came from",
			payload: `{"players":[{"cricsheet_id":"abc"}]}`,
			wants:   "note",
		},
		{
			name:    "a bowling style outside the vocabulary",
			payload: `{"players":[{"cricsheet_id":"abc","note":"n","bowling_style":"doosra"}]}`,
			wants:   "bowling_style",
		},
		{
			name:    "a batting hand that is neither left nor right",
			payload: `{"players":[{"cricsheet_id":"abc","note":"n","batting_hand":"both"}]}`,
			wants:   "batting_hand",
		},
		{
			name:    "a date that is not YYYY-MM-DD",
			payload: `{"players":[{"cricsheet_id":"abc","note":"n","birth_date":"01/04/1990"}]}`,
			wants:   "birth_date",
		},
		{
			name:    "the same player twice",
			payload: `{"players":[{"cricsheet_id":"a","note":"n"},{"cricsheet_id":"a","note":"m"}]}`,
			wants:   "twice",
		},
		{
			name:    "not JSON at all",
			payload: `not json`,
			wants:   "decoding",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := biography.ParseOverrides(strings.NewReader(testCase.payload))
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wants)
		})
	}
}

// TestApply_ChangesOnlyTheFieldsTheOverrideNames is what makes the file safe for a single
// correction: supplying a style must not erase an acquired date of birth.
func TestApply_ChangesOnlyTheFieldsTheOverrideNames(t *testing.T) {
	t.Parallel()
	born := time.Date(1990, 4, 1, 0, 0, 0, 0, time.UTC)
	acquired := biography.Record{
		PlayerID:    7,
		WikidataQID: "Q1",
		BirthDate:   &born,
		Source:      biography.SourceWikidata,
		License:     biography.SourceLicense,
	}

	corrected := biography.Apply(acquired, biography.Override{
		CricsheetID:  "abc",
		Note:         "board profile, checked 2026-09",
		BowlingStyle: biography.StyleLegSpin,
	})

	require.NotNil(t, corrected.BirthDate)
	assert.Equal(t, born, *corrected.BirthDate, "an untouched field keeps its acquired value")
	assert.Equal(t, biography.StyleLegSpin, corrected.BowlingStyle)
	assert.Equal(t, biography.SourceOverride, corrected.Source,
		"a curated row must be distinguishable from an acquired one")
	assert.Contains(t, corrected.BowlingStyleRaw, "board profile")
}

// TestApply_DoesNotInventAWikidataMatch: a curated date of birth is a date of birth, not
// a match, and counting it as one would inflate the figure the X-1b gate reads.
func TestApply_DoesNotInventAWikidataMatch(t *testing.T) {
	t.Parallel()

	corrected := biography.Apply(
		biography.Record{PlayerID: 7, CricinfoID: "332980"},
		biography.Override{CricsheetID: "abc", Note: "n", BirthDate: "1990-04-01"})

	assert.False(t, corrected.Matched())
	require.NotNil(t, corrected.BirthDate)
}

// TestApply_TakesEveryStatedField covers the remaining fields in one pass.
func TestApply_TakesEveryStatedField(t *testing.T) {
	t.Parallel()

	corrected := biography.Apply(biography.Record{PlayerID: 7}, biography.Override{
		CricsheetID:   "abc",
		Note:          "n",
		WikidataQID:   "Q42",
		BattingHand:   biography.HandLeft,
		CareerEndDate: "2019-06-01",
		DeathDate:     "2024-01-15",
	})

	assert.Equal(t, "Q42", corrected.WikidataQID)
	assert.Equal(t, biography.HandLeft, corrected.BattingHand)
	require.NotNil(t, corrected.CareerEndDate)
	assert.Equal(t, "2019-06-01", corrected.CareerEndDate.Format(time.DateOnly))
	require.NotNil(t, corrected.DeathDate)
}
