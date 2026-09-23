package formats_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

func TestCanonicalCodes(t *testing.T) {
	t.Parallel()

	// Act
	codes := formats.CanonicalCodes()

	// Assert
	assert.Equal(t, []string{formats.CodeTest, formats.CodeODI, formats.CodeT20, formats.CodeT20I}, codes)
}

func TestNormalizeCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "lowercase t20", input: "t20", expected: "T20"},
		{name: "padded odi", input: " odi ", expected: "ODI"},
		{name: "lowercase test", input: "test", expected: "TEST"},
		{name: "already uppercase T20I", input: "T20I", expected: "T20I"},
		{name: "empty string", input: "", expected: ""},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := formats.NormalizeCode(tc.input)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

// A competition level is one of Cricsheet's two words or it is an error: the importer
// refuses a file it cannot place rather than inferring the level from team names, which
// is the hand list IMPORT-09 retired.
func TestParseCompetitionLevel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		teamType  string
		want      string
		wantError string
	}{
		{name: "international", teamType: "international", want: formats.CompetitionInternational},
		{name: "club", teamType: "club", want: formats.CompetitionClub},
		{name: "case and padding are folded", teamType: " International\n", want: formats.CompetitionInternational},
		{name: "missing is refused", teamType: "", wantError: "missing team_type"},
		{name: "blank is refused", teamType: "   ", wantError: "missing team_type"},
		{name: "an unknown word is refused", teamType: "franchise", wantError: `unsupported team_type: "franchise"`},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got, err := formats.ParseCompetitionLevel(tc.teamType)

			// Assert
			assertCompetitionLevel(t, got, err, tc.want, tc.wantError)
		})
	}
}

func assertCompetitionLevel(t *testing.T, got string, err error, want, wantError string) {
	t.Helper()
	if wantError != "" {
		require.EqualError(t, err, wantError)
		require.Empty(t, got)
		return
	}
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCanonicalizeCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "MDM maps to TEST", input: "MDM", expected: formats.CodeTest},
		{name: "lowercase mdm maps to TEST", input: "mdm", expected: formats.CodeTest},
		{name: "ODM maps to ODI", input: "ODM", expected: formats.CodeODI},
		{name: "IT20 maps to T20I", input: "IT20", expected: formats.CodeT20I},
		{name: "lowercase it20 maps to T20I", input: "it20", expected: formats.CodeT20I},
		{name: "T20 stays T20", input: "T20", expected: formats.CodeT20},
		{name: "ODI stays ODI", input: "ODI", expected: formats.CodeODI},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := formats.CanonicalizeCode(tc.input)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestIDForCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		code       string
		expectedID int
		wantErr    bool
	}{
		{name: "TEST format", code: "TEST", expectedID: formats.IDTest},
		{name: "ODI format", code: "ODI", expectedID: formats.IDODI},
		{name: "T20 format", code: "T20", expectedID: formats.IDT20},
		{name: "T20I format", code: "T20I", expectedID: formats.IDT20I},
		{name: "lowercase t20", code: "t20", expectedID: formats.IDT20},
		{name: "unknown format returns error", code: "UNKNOWN", wantErr: true},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			id, err := formats.IDForCode(tc.code)

			// Assert
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown format")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedID, id)
		})
	}
}

func TestMapFormatIDs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		code     string
		expected []int
	}{
		{name: "empty defaults to T20 bucket", code: "", expected: []int{formats.IDT20, formats.IDT20I}},
		{name: "T20 returns T20 bucket", code: "T20", expected: []int{formats.IDT20, formats.IDT20I}},
		{name: "T20I returns T20 bucket", code: "T20I", expected: []int{formats.IDT20, formats.IDT20I}},
		{name: "ODI returns ODI only", code: "ODI", expected: []int{formats.IDODI}},
		{name: "TEST returns Test only", code: "TEST", expected: []int{formats.IDTest}},
		{name: "unknown defaults to T20 bucket", code: "UNKNOWN", expected: []int{formats.IDT20, formats.IDT20I}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := formats.MapFormatIDs(tc.code)

			// Assert
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestGetHierarchy(t *testing.T) {
	t.Parallel()

	// Act
	hierarchy := formats.GetHierarchy()

	// Assert
	require.Len(t, hierarchy, 3, "Hierarchy should have 3 root nodes")

	testCases := []struct {
		name          string
		index         int
		expectedCode  string
		expectedName  string
		expectedChild *formats.FormatHierarchyNode
	}{
		{
			name:         "MDM node",
			index:        0,
			expectedCode: "MDM",
			expectedName: "Multi-Day Match",
			expectedChild: &formats.FormatHierarchyNode{
				Code: formats.CodeTest,
				Name: "Test Matches",
			},
		},
		{
			name:         "ODM node",
			index:        1,
			expectedCode: "ODM",
			expectedName: "One Day Match",
			expectedChild: &formats.FormatHierarchyNode{
				Code: formats.CodeODI,
				Name: "One Day International",
			},
		},
		{
			name:         "T20 bucket node",
			index:        2,
			expectedCode: formats.CodeT20,
			expectedName: "T20 (Bucket)",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Assert
			node := hierarchy[tc.index]
			assert.Equal(t, tc.expectedCode, node.Code)
			assert.Equal(t, tc.expectedName, node.Name)

			if tc.expectedChild != nil {
				require.NotEmpty(t, node.Children)
				assert.Equal(t, tc.expectedChild.Code, node.Children[0].Code)
				assert.Equal(t, tc.expectedChild.Name, node.Children[0].Name)
			}
		})
	}
}

func TestIsCanonical(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		code string
		want bool
	}{
		{name: "TEST", code: "TEST", want: true},
		{name: "ODI", code: "ODI", want: true},
		{name: "T20", code: "T20", want: true},
		{name: "T20I", code: "T20I", want: true},
		{name: "lowercase is normalized", code: "t20i", want: true},
		{name: "surrounding whitespace is trimmed", code: "  ODI  ", want: true},
		{name: "empty", code: "", want: false},
		{name: "unknown code", code: "HUNDRED", want: false},
		// aliases are deliberately not accepted: callers canonicalize first
		{name: "alias MDM is not canonical", code: "MDM", want: false},
		{name: "alias ODM is not canonical", code: "ODM", want: false},
		{name: "alias IT20 is not canonical", code: "IT20", want: false},
		{name: "the all sentinel is not a format", code: "all", want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, formats.IsCanonical(tc.code))
		})
	}
}

func TestIsCanonicalAcceptsEveryCanonicalCode(t *testing.T) {
	t.Parallel()
	// guards against CanonicalCodes and IsCanonical drifting apart
	for _, c := range formats.CanonicalCodes() {
		require.Truef(t, formats.IsCanonical(c), "CanonicalCodes() returned %q but IsCanonical rejects it", c)
	}
}

func TestCanonicalizeThenIsCanonicalAcceptsAliases(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"MDM", "ODM", "IT20"} {
		require.Truef(t, formats.IsCanonical(formats.CanonicalizeCode(alias)),
			"alias %q should be canonical after CanonicalizeCode", alias)
	}
}
