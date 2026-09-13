package wicketkinds_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/wicketkinds"
)

// committedVocabulary is configs/wicket_kinds.json as the package finds it from this
// directory: the file the importer and the rating pass both read.
func committedVocabulary(t *testing.T) wicketkinds.Vocabulary {
	t.Helper()
	vocabulary, err := wicketkinds.LoadFile(filepath.Join("..", "..", "..", "configs", wicketkinds.FileName))
	require.NoError(t, err)
	return vocabulary
}

func writeVocabulary(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), wicketkinds.FileName)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestVocabulary_Kind_TheCommittedFileClassifiesEveryKindTheArchiveUses(t *testing.T) {
	t.Parallel()

	// The fourteen kinds in the archive as of 2026-09-13, each with what the scorecard
	// makes of it. A kind arriving here that the file does not carry fails the import.
	testCases := []struct {
		name      string
		raw       string
		wantName  string
		wantClass wicketkinds.Class
	}{
		{name: "bowled", raw: "bowled", wantName: "bowled", wantClass: wicketkinds.CreditedToBowler},
		{name: "caught", raw: "caught", wantName: "caught", wantClass: wicketkinds.CreditedToBowler},
		{
			name:      "caught and bowled",
			raw:       "caught and bowled",
			wantName:  "caught and bowled",
			wantClass: wicketkinds.CreditedToBowler,
		},
		{name: "hit wicket", raw: "hit wicket", wantName: "hit wicket", wantClass: wicketkinds.CreditedToBowler},
		{name: "lbw", raw: "lbw", wantName: "lbw", wantClass: wicketkinds.CreditedToBowler},
		{name: "stumped", raw: "stumped", wantName: "stumped", wantClass: wicketkinds.CreditedToBowler},
		{
			name:      "run out is a dismissal and not the bowler's",
			raw:       "run out",
			wantName:  "run out",
			wantClass: wicketkinds.DismissalNotCredited,
		},
		{
			name:      "retired out is a dismissal",
			raw:       "retired out",
			wantName:  "retired out",
			wantClass: wicketkinds.DismissalNotCredited,
		},
		{
			name:      "obstructing the field",
			raw:       "obstructing the field",
			wantName:  "obstructing the field",
			wantClass: wicketkinds.DismissalNotCredited,
		},
		{
			name:      "handled the ball",
			raw:       "handled the ball",
			wantName:  "handled the ball",
			wantClass: wicketkinds.DismissalNotCredited,
		},
		{
			name:      "hit the ball twice",
			raw:       "hit the ball twice",
			wantName:  "hit the ball twice",
			wantClass: wicketkinds.DismissalNotCredited,
		},
		{name: "timed out", raw: "timed out", wantName: "timed out", wantClass: wicketkinds.DismissalNotCredited},
		{name: "retired hurt is not out", raw: "retired hurt", wantName: "retired hurt", wantClass: wicketkinds.NotOut},
		{
			name:      "retired not out is not out",
			raw:       "retired not out",
			wantName:  "retired not out",
			wantClass: wicketkinds.NotOut,
		},
		{
			name:      "case and space are not a different kind",
			raw:       "  Run Out ",
			wantName:  "run out",
			wantClass: wicketkinds.DismissalNotCredited,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vocabulary := committedVocabulary(t)

			got, err := vocabulary.Kind(tc.raw)

			require.NoError(t, err)
			assert.Equal(t, wicketkinds.Kind{Name: tc.wantName, Class: tc.wantClass}, got)
		})
	}
}

func TestVocabulary_Kind_UnknownKindIsAnError(t *testing.T) {
	t.Parallel()
	vocabulary := committedVocabulary(t)

	_, err := vocabulary.Kind("mankaded")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"mankaded"`)
}

func TestClass_IsDismissal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		class wicketkinds.Class
		want  bool
	}{
		{name: "the bowler's wicket is a wicket lost", class: wicketkinds.CreditedToBowler, want: true},
		{name: "a run out is a wicket lost", class: wicketkinds.DismissalNotCredited, want: true},
		{name: "a retirement not out is not", class: wicketkinds.NotOut, want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.class.IsDismissal())
		})
	}
}

func TestLoadFile_RejectsAVocabularyThatCannotClassify(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name:     "a kind listed under two classes",
			contents: `{"credited_to_bowler":["caught"],"dismissal_not_credited":["caught"],"not_out":["retired hurt"]}`,
			wantErr:  "listed twice",
		},
		{
			name:     "an empty class",
			contents: `{"credited_to_bowler":["caught"],"dismissal_not_credited":["run out"],"not_out":[]}`,
			wantErr:  "is empty",
		},
		{
			name:     "a kind not in the file's own spelling",
			contents: `{"credited_to_bowler":["Caught"],"dismissal_not_credited":["run out"],"not_out":["retired hurt"]}`,
			wantErr:  "spelling",
		},
		{
			name:     "a file that is not JSON",
			contents: `{`,
			wantErr:  "parse",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeVocabulary(t, tc.contents)

			_, err := wicketkinds.LoadFile(path)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoad_ReadsTheFileNamedByTheEnvironment(t *testing.T) {
	// Not parallel: sets the process environment.
	path := writeVocabulary(
		t,
		`{"version":"t","credited_to_bowler":["bowled"],"dismissal_not_credited":["run out"],"not_out":["retired hurt"]}`,
	)
	t.Setenv(wicketkinds.EnvVar, path)

	vocabulary, err := wicketkinds.Load()

	require.NoError(t, err)
	assert.Equal(t, "t", vocabulary.Version)
	assert.Equal(t, []string{"bowled"}, vocabulary.CreditedToBowler)
}

func TestLoad_FailsWhenTheFileIsMissing(t *testing.T) {
	// Not parallel: sets the process environment.
	t.Setenv(wicketkinds.EnvVar, filepath.Join(t.TempDir(), "absent.json"))

	_, err := wicketkinds.Load()

	require.Error(t, err)
}
