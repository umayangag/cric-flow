package biographybackfill_test

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
	"github.com/umayangag/cric-flow/go-app/internal/services/biographybackfill"
)

func parse(t *testing.T, args ...string) (biographybackfill.Options, error) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.NewFile(0, os.DevNull))
	return biographybackfill.ParseArgs(fs, args)
}

// TestParseArgs_DefaultsToTheFreeLicenceCleanSources: the register and the query service
// are both free and need no account, which is the whole basis of X-1a.
func TestParseArgs_DefaultsToTheFreeLicenceCleanSources(t *testing.T) {
	options, err := parse(t)

	require.NoError(t, err)
	assert.Equal(t, biography.RegisterURL, options.Register)
	assert.Equal(t, biography.SPARQLEndpoint, options.Endpoint)
	assert.Equal(t, biography.DefaultBatchSize, options.BatchSize)
	assert.NotEmpty(t, options.UserAgent)
}

// TestParseArgs_ReadsTheUserAgentFromTheEnvironment: the contact address in it belongs to
// whoever runs the backfill, not to the repository.
func TestParseArgs_ReadsTheUserAgentFromTheEnvironment(t *testing.T) {
	t.Setenv(biographybackfill.UserAgentEnvVar, "someone/1.0 (someone@example.com)")

	options, err := parse(t)

	require.NoError(t, err)
	assert.Equal(t, "someone/1.0 (someone@example.com)", options.UserAgent)
}

// TestParseArgs_RefusesSettingsThatWouldProduceAnUntrustworthyRun.
func TestParseArgs_RefusesSettingsThatWouldProduceAnUntrustworthyRun(t *testing.T) {
	testCases := []struct {
		name  string
		args  []string
		wants string
	}{
		{name: "no register to join against", args: []string{"-register", ""}, wants: "register"},
		{name: "a batch size of zero would ask nothing", args: []string{"-batch", "0"}, wants: "batch"},
		{name: "a negative pause is not a rate limit", args: []string{"-pause", "-1s"}, wants: "pause"},
		{name: "a zero timeout would end the run at once", args: []string{"-timeout", "0"}, wants: "timeout"},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parse(t, testCase.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wants)
		})
	}
}

// TestParseArgs_ReportOnlyNeedsNoSource because it measures what is stored and fetches
// nothing.
func TestParseArgs_ReportOnlyNeedsNoSource(t *testing.T) {
	options, err := parse(t, "-report-only", "-register", "")

	require.NoError(t, err)
	assert.True(t, options.ReportOnly)
}

// TestParseArgs_RefusesAnAnonymousCaller: the query service asks callers to identify
// themselves, and an empty User-Agent is exactly the traffic it asks not to receive.
func TestParseArgs_RefusesAnAnonymousCaller(t *testing.T) {
	_, err := parse(t, "-user-agent", "  ")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "User-Agent")
}

// TestParseArgs_OfflineNeedsNoCallerIdentity: it calls nobody, so there is nobody to
// identify itself to.
func TestParseArgs_OfflineNeedsNoCallerIdentity(t *testing.T) {
	options, err := parse(t, "-offline", "-register", "reference-data/people.csv",
		"-user-agent", "  ")

	require.NoError(t, err)
	assert.True(t, options.Offline)
}

// TestParseArgs_RefusesAnOfflineRunThatWouldStillFetch. The default register is a URL, so
// an -offline run that inherited it would quietly fetch — the one thing the mode rules out.
func TestParseArgs_RefusesAnOfflineRunThatWouldStillFetch(t *testing.T) {
	testCases := []struct {
		name  string
		args  []string
		wants string
	}{
		{
			name:  "the default register is a URL",
			args:  []string{"-offline"},
			wants: "local",
		},
		{
			name:  "an explicit remote register is no better",
			args:  []string{"-offline", "-register", "https://example.test/people.csv"},
			wants: "local",
		},
		{
			name:  "report-only writes no rows, so it is not a restore",
			args:  []string{"-offline", "-report-only", "-register", "reference-data/people.csv"},
			wants: "alternatives",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parse(t, testCase.args...)

			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wants)
		})
	}
}

// TestLoadRegister_ReadsALocalCopy is the offline path: a box with no outbound network,
// or a run pinned to a register someone has already looked at.
func TestLoadRegister_ReadsALocalCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "people.csv")
	require.NoError(t, os.WriteFile(path,
		[]byte("identifier,name,key_cricinfo\nabc,V Kohli,253802\n"), 0o600))

	entries, err := biographybackfill.LoadRegister(nil, "agent", path)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"253802"}, entries["abc"].CricinfoIDs)
}

// TestLoadRegister_FetchesOverHTTPSWithTheUserAgent. The https prefix is what selects the
// network path, so the fixture server has to be a TLS one for this to exercise it.
func TestLoadRegister_FetchesOverHTTPSWithTheUserAgent(t *testing.T) {
	var sawUserAgent string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUserAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("identifier,name,key_cricinfo\nabc,V Kohli,253802\n"))
	}))
	t.Cleanup(server.Close)

	entries, err := biographybackfill.LoadRegister(server.Client(), "cric-flow-test", server.URL)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "cric-flow-test", sawUserAgent)
}

// TestLoadRegister_ReportsANonOKAnswer rather than parsing an error page as a register.
func TestLoadRegister_ReportsANonOKAnswer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := biographybackfill.LoadRegister(server.Client(), "cric-flow-test", server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// TestLoadRegister_ReportsAMissingFile rather than returning an empty register that would
// read as "nobody has a Cricinfo id".
func TestLoadRegister_ReportsAMissingFile(t *testing.T) {
	_, err := biographybackfill.LoadRegister(nil, "agent", filepath.Join(t.TempDir(), "absent.csv"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening the register")
}

// TestLoadOverrides_MissingFileIsNotAnError: the normal state is that no override has
// been needed yet, and failing on the absence would make the file mandatory for nothing.
func TestLoadOverrides_MissingFileIsNotAnError(t *testing.T) {
	overrides, err := biographybackfill.LoadOverrides(filepath.Join(t.TempDir(), "absent.json"))

	require.NoError(t, err)
	assert.Empty(t, overrides)
}

// TestLoadOverrides_ReadsAndValidatesTheFile.
func TestLoadOverrides_ReadsAndValidatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"players":[{"cricsheet_id":"abc","note":"checked","bowling_style":"pace"}]}`), 0o600))

	overrides, err := biographybackfill.LoadOverrides(path)

	require.NoError(t, err)
	require.Len(t, overrides, 1)
	assert.Equal(t, biography.StylePace, overrides["abc"].BowlingStyle)
}

// TestLoadOverrides_RefusesAnInvalidFile so a typo is caught before it becomes a stored
// fact indistinguishable from a measured one.
func TestLoadOverrides_RefusesAnInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"players":[{"cricsheet_id":"abc","note":"n","bowling_style":"doosra"}]}`), 0o600))

	_, err := biographybackfill.LoadOverrides(path)

	require.Error(t, err)
}

// TestLoadOverrides_EmptyPathLoadsNothing covers the "-overrides ”" case.
func TestLoadOverrides_EmptyPathLoadsNothing(t *testing.T) {
	overrides, err := biographybackfill.LoadOverrides("  ")

	require.NoError(t, err)
	assert.Empty(t, overrides)
}

// TestWriteFile_CreatesTheDirectoryAndSkipsAnEmptyPath.
func TestWriteFile_CreatesTheDirectoryAndSkipsAnEmptyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.md")

	require.NoError(t, biographybackfill.WriteFile(path, []byte("# report")))
	require.NoError(t, biographybackfill.WriteFile("", []byte("nothing")))

	content, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	require.NoError(t, err)
	assert.Equal(t, "# report", string(content))
}
