package teamlineage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/teamlineage"
)

func writeMapping(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), teamlineage.FileName)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestSuccessor(t *testing.T) {
	t.Parallel()

	// A club may rename its men's side and its women's side separately, so the gender is
	// part of the key and not a detail of the entry.
	mapping := teamlineage.Mapping{Renames: []teamlineage.Rename{
		{From: "Royal Challengers Bangalore", To: "Royal Challengers Bengaluru", Gender: "male"},
		{From: "Lightning", To: "The Blaze", Gender: "female"},
	}}

	testCases := []struct {
		name      string
		team      string
		gender    string
		wantFound bool
		wantName  string
	}{
		{
			name:      "a renamed club",
			team:      "Royal Challengers Bangalore",
			gender:    "male",
			wantFound: true,
			wantName:  "Royal Challengers Bengaluru",
		},
		{
			name:   "the same name under a gender that did not rename",
			team:   "Royal Challengers Bangalore",
			gender: "female",
		},
		{name: "a club that never renamed", team: "Mumbai Indians", gender: "male"},
		{name: "the successor itself", team: "Royal Challengers Bengaluru", gender: "male"},
		{name: "a women's rename", team: "Lightning", gender: "female", wantFound: true, wantName: "The Blaze"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, found := mapping.Successor(tc.team, tc.gender)

			assert.Equal(t, tc.wantFound, found)
			assert.Equal(t, tc.wantName, got.Name)
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		renames []teamlineage.Rename
		wantErr string
	}{
		{
			name:    "a reviewed mapping",
			renames: []teamlineage.Rename{{From: "A", To: "B", Gender: "male"}, {From: "C", To: "D", Gender: "male"}},
		},
		{
			name:    "the same club renamed for two genders",
			renames: []teamlineage.Rename{{From: "A", To: "B", Gender: "male"}, {From: "A", To: "B", Gender: "female"}},
		},
		{
			name:    "a chain",
			renames: []teamlineage.Rename{{From: "A", To: "B", Gender: "male"}, {From: "B", To: "C", Gender: "male"}},
			wantErr: "collapse the chain into one hop by hand",
		},
		{
			name:    "one club becoming two things",
			renames: []teamlineage.Rename{{From: "A", To: "B", Gender: "male"}, {From: "A", To: "C", Gender: "male"}},
			wantErr: "named as the predecessor twice",
		},
		{
			name:    "a club succeeding itself",
			renames: []teamlineage.Rename{{From: "A", To: "A", Gender: "male"}},
			wantErr: "cannot succeed itself",
		},
		{
			name:    "a missing gender",
			renames: []teamlineage.Rename{{From: "A", To: "B"}},
			wantErr: "from, to and gender are all required",
		},
		{
			name:    "a missing successor",
			renames: []teamlineage.Rename{{From: "A", To: "  ", Gender: "male"}},
			wantErr: "from, to and gender are all required",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := teamlineage.Mapping{Renames: tc.renames}.Validate()

			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoadFile_ReadsAReviewedMapping(t *testing.T) {
	t.Parallel()

	path := writeMapping(t, `{"version":"1","renames":[
		{"from":"Kings XI Punjab","to":"Punjab Kings","gender":"male","note":"IPL, 2021"}]}`)

	mapping, err := teamlineage.LoadFile(path)

	require.NoError(t, err)
	require.Len(t, mapping.Renames, 1)
	assert.Equal(t, "1", mapping.Version)
	assert.Equal(t, "IPL, 2021", mapping.Renames[0].Note)
}

func TestLoadFile_RejectsAMappingThatCannotMeanWhatItSays(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{name: "not json", contents: "{", wantErr: "parse"},
		{
			name:     "a chain",
			contents: `{"renames":[{"from":"A","to":"B","gender":"male"},{"from":"B","to":"C","gender":"male"}]}`,
			wantErr:  "collapse the chain",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := teamlineage.LoadFile(writeMapping(t, tc.contents))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoadFile_MissingFileIsAnError(t *testing.T) {
	t.Parallel()

	_, err := teamlineage.LoadFile(filepath.Join(t.TempDir(), "absent.json"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestLoad_NoMappingAnywhereIsNotAnError(t *testing.T) {
	// Not parallel: sets an environment variable and the working directory is shared.
	t.Setenv(teamlineage.EnvVar, "")
	t.Chdir(t.TempDir())

	mapping, err := teamlineage.Load()

	require.NoError(t, err, "a deployment with no renames recorded is a valid one")
	assert.Empty(t, mapping.Renames)
}

func TestLoad_PrefersTheEnvironmentPath(t *testing.T) {
	// Not parallel: sets an environment variable.
	path := writeMapping(t, `{"renames":[{"from":"A","to":"B","gender":"male"}]}`)
	t.Setenv(teamlineage.EnvVar, path)

	mapping, err := teamlineage.Load()

	require.NoError(t, err)
	require.Len(t, mapping.Renames, 1)
}

func TestLoad_FindsTheMappingBesideTheRepository(t *testing.T) {
	// Not parallel: changes the working directory.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "configs"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "configs", teamlineage.FileName),
		[]byte(`{"renames":[{"from":"A","to":"B","gender":"female"}]}`), 0o600))
	t.Setenv(teamlineage.EnvVar, "")
	t.Chdir(dir)

	mapping, err := teamlineage.Load()

	require.NoError(t, err)
	require.Len(t, mapping.Renames, 1)
}

func TestTheCommittedMappingIsValid(t *testing.T) {
	t.Parallel()

	// The mapping is data a human edits, so the file itself is under test: a chain or a
	// duplicate predecessor added by hand would otherwise only surface at import time.
	mapping, err := teamlineage.LoadFile(filepath.Join("..", "..", "..", "configs", teamlineage.FileName))

	require.NoError(t, err)
	assert.NotEmpty(t, mapping.Renames)
	for _, rename := range mapping.Renames {
		assert.NotEmpty(t, rename.Note, "every rename records why it was accepted: %+v", rename)
	}
}
