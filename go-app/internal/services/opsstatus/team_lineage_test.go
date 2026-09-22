package opsstatus_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
)

// stubTeamLineageProbe answers with a fixed report, which is all the section needs.
type stubTeamLineageProbe struct {
	report db.TeamLineageReport
	err    error
}

func (s stubTeamLineageProbe) TeamLineageCoverage(_ context.Context) (db.TeamLineageReport, error) {
	return s.report, s.err
}

func lineageRename(from, to string, state db.TeamLineageState) db.TeamLineageRename {
	return db.TeamLineageRename{
		Rename: db.TeamRename{FromName: from, ToName: to, Gender: "male"},
		State:  state,
	}
}

// TestBuildTeamLineageSection_SaysWhetherTheArchiveIsSettled is the operator's view of
// IMPORT-07: an import that aborted before settlement wrote no links and logged nothing,
// so the only place the state was visible was a SQL prompt.
func TestBuildTeamLineageSection_SaysWhetherTheArchiveIsSettled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		probe            opsstatus.TeamLineageProbe
		expectedStatus   string
		expectedLinked   int
		expectedUnlinked []string
	}{
		{
			name: "every rename in the mapping is linked",
			probe: stubTeamLineageProbe{report: db.TeamLineageReport{Renames: []db.TeamLineageRename{
				lineageRename("Delhi Daredevils", "Delhi Capitals", db.TeamLineageLinked),
				lineageRename("Kings XI Punjab", "Punjab Kings", db.TeamLineageLinked),
			}}},
			expectedStatus:   "ok",
			expectedLinked:   2,
			expectedUnlinked: []string{},
		},
		{
			name: "a rename this archive has only one side of is not a fault",
			probe: stubTeamLineageProbe{report: db.TeamLineageReport{Renames: []db.TeamLineageRename{
				lineageRename("Delhi Daredevils", "Delhi Capitals", db.TeamLineageLinked),
				lineageRename("Deccan Chargers", "Sunrisers Hyderabad", db.TeamLineageAbsent),
			}}},
			expectedStatus:   "ok",
			expectedLinked:   1,
			expectedUnlinked: []string{},
		},
		{
			name: "both clubs are here and the link was never written",
			probe: stubTeamLineageProbe{report: db.TeamLineageReport{Renames: []db.TeamLineageRename{
				lineageRename("Delhi Daredevils", "Delhi Capitals", db.TeamLineageUnlinked),
			}}},
			expectedStatus:   "incomplete",
			expectedLinked:   0,
			expectedUnlinked: []string{"Delhi Daredevils -> Delhi Capitals (male)"},
		},
		{
			name:           "the archive could not be read",
			probe:          stubTeamLineageProbe{err: errors.New("db pool not initialized")},
			expectedStatus: "unknown",
		},
		{
			name:           "no probe at all",
			probe:          nil,
			expectedStatus: "unknown",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			section := opsstatus.BuildTeamLineageSection(context.Background(), testCase.probe)

			require.Equal(t, testCase.expectedStatus, section["status"])
			assertLineageCounts(t, section, testCase.expectedStatus, testCase.expectedLinked, testCase.expectedUnlinked)
		})
	}
}

// assertLineageCounts checks the counts only where the section reports them, so an
// unreadable archive is not asserted to have counted anything.
func assertLineageCounts(t *testing.T, section map[string]any, status string, linked int, unlinked []string) {
	t.Helper()
	if status == "unknown" {
		assert.NotContains(t, section, "linked", "an unknown archive reports no counts it did not make")
		return
	}
	assert.Equal(t, linked, section["linked"])
	assert.Equal(t, unlinked, section["unlinked_renames"])
	assert.Equal(t, len(unlinked), section["unlinked"])
}
