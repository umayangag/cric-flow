package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// DetectFormat reads Cricsheet's match_type and team_type and nothing else. Until IMPORT-09
// the T20 / T20I split was a hand list of twelve team names, so the cases here name sides
// that were never on it: what decides the code is the level the file states.
func TestDetectFormat_Table(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		matchType        string
		competitionLevel string
		want             string
	}{
		{name: "Test", matchType: "Test", competitionLevel: formats.CompetitionInternational, want: "TEST"},
		{name: "ODI", matchType: "ODI", competitionLevel: formats.CompetitionInternational, want: "ODI"},
		{name: "T20I", matchType: "T20I", competitionLevel: formats.CompetitionInternational, want: "T20I"},
		// IT20 is an international without T20I status; it is international by definition.
		{name: "IT20 alias", matchType: "IT20", competitionLevel: formats.CompetitionInternational, want: "T20I"},
		// MDM and ODM stay pooled with TEST and ODI whatever their level (see the § 9 entry).
		{name: "MDM pools with TEST", matchType: "MDM", competitionLevel: formats.CompetitionClub, want: "TEST"},
		{
			name:             "international MDM pools with TEST",
			matchType:        "MDM",
			competitionLevel: formats.CompetitionInternational,
			want:             "TEST",
		},
		{name: "ODM pools with ODI", matchType: "ODM", competitionLevel: formats.CompetitionClub, want: "ODI"},
		{
			name:             "international ODM pools with ODI",
			matchType:        "ODM",
			competitionLevel: formats.CompetitionInternational,
			want:             "ODI",
		},
		// The T20 split is the level, not the team names: a qualifier between two
		// associate sides is a T20I, a franchise game is not.
		{
			name:             "T20 between national sides is T20I",
			matchType:        "T20",
			competitionLevel: formats.CompetitionInternational,
			want:             "T20I",
		},
		{name: "T20 between clubs is T20", matchType: "T20", competitionLevel: formats.CompetitionClub, want: "T20"},
		{name: "T20 with no level is T20", matchType: "T20", competitionLevel: "", want: "T20"},
		// Case and whitespace on the match type are tolerated; the level is already parsed.
		{name: "lower-case test", matchType: "test", competitionLevel: formats.CompetitionInternational, want: "TEST"},
		{name: "padded odi", matchType: "  odi \n", competitionLevel: formats.CompetitionInternational, want: "ODI"},
		{
			name:             "lower-case t20 international",
			matchType:        "t20",
			competitionLevel: formats.CompetitionInternational,
			want:             "T20I",
		},
		{name: "unknown type is no format", matchType: "Friendly", competitionLevel: formats.CompetitionClub, want: ""},
		{name: "empty type is no format", matchType: "", competitionLevel: formats.CompetitionClub, want: ""},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act
			got := cricsheet.DetectFormat(tc.matchType, tc.competitionLevel)
			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
