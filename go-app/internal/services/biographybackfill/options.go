// Package biographybackfill holds the CLI options for the player-biography backfill and
// the file loading it does before any network call.
//
// It is separate from cmd/ for the reason every other CLI here is: option parsing and
// file loading are the parts worth testing, and a main package is the one place they
// cannot be.
package biographybackfill

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// Defaults for the backfill. The register URL and the query endpoint are the free,
// licence-clean sources X-1a is built on; neither needs an account and neither is paid.
const (
	// DefaultCacheFile is where the resumable record of what Wikidata has been asked
	// lives. Under output/ because it is a working file, not data the system serves.
	DefaultCacheFile = "output/player-biographies/wikidata-lookups.jsonl"
	// DefaultOverridesFile is the curated file, in the same place as the other
	// hand-maintained mappings.
	DefaultOverridesFile = "configs/player_biography_overrides.json"
	// DefaultReportFile is the coverage report a run commits.
	DefaultReportFile = "docs/player-biography-coverage.md"
	// DefaultPause is how long to wait between SPARQL batches. The query service asks
	// for a reasonable rate rather than naming one; a second between batches of five
	// hundred is about one request per five hundred players.
	DefaultPause = 1 * time.Second
	// DefaultTimeout bounds the whole run. Thirty batches with a second between them is
	// minutes, not hours; an hour is generous enough that a slow day does not fail and
	// short enough that a wedged run does not sit forever.
	DefaultTimeout = 1 * time.Hour
	// UserAgentEnvVar overrides the User-Agent the query service sees. The service asks
	// callers to identify themselves with a contact address, and the address belongs to
	// whoever runs the backfill, not to this repository.
	UserAgentEnvVar = "WIKIDATA_USER_AGENT"
	// DefaultUserAgent is used when the environment names none. It identifies the
	// software and points at the project rather than at a person.
	DefaultUserAgent = "cric-flow/1.0 (https://github.com/umayangag/cric-flow) player-biography-backfill"
)

// Options are one run's settings.
type Options struct {
	// Register is the people register: an https URL to fetch or a path to a local copy.
	Register string
	// CacheFile is the resumable lookup cache.
	CacheFile string
	// OverridesFile is the curated overrides. A missing file is not an error — the
	// normal state is that no override is needed yet.
	OverridesFile string
	// ReportFile is where the markdown coverage report is written. Empty writes none.
	ReportFile string
	// JSONReportFile is where the machine-readable coverage is written, if anywhere.
	JSONReportFile string
	// Endpoint is the SPARQL endpoint.
	Endpoint string
	// UserAgent identifies this caller to the query service.
	UserAgent string
	// BatchSize is how many ESPNcricinfo ids go into one query.
	BatchSize int
	// Pause is the wait between batches.
	Pause time.Duration
	// Timeout bounds the run.
	Timeout time.Duration
	// ReportOnly re-measures what is stored and writes the report, fetching nothing. It
	// is how the committed report is regenerated without asking Wikidata again.
	ReportOnly bool
	// Offline rebuilds the table from committed snapshots and makes no network call at
	// all: the register must be a local copy and Wikidata is never asked. It is the
	// restore path after a purge — see the Makefile target restore-player-biographies.
	Offline bool
}

// ParseArgs parses the flags, returning an error rather than exiting so tests can assert
// on it.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var options Options
	fs.StringVar(&options.Register, "register", biography.RegisterURL,
		"Cricsheet people register: an https URL to fetch, or a path to a local copy")
	fs.StringVar(&options.CacheFile, "cache", DefaultCacheFile,
		"resumable record of what Wikidata has already been asked")
	fs.StringVar(&options.OverridesFile, "overrides", DefaultOverridesFile,
		"curated biography overrides; a missing file is fine")
	fs.StringVar(&options.ReportFile, "report", DefaultReportFile,
		"where to write the markdown coverage report; empty writes none")
	fs.StringVar(&options.JSONReportFile, "json-report", "",
		"where to write the coverage report as JSON; empty writes none")
	fs.StringVar(&options.Endpoint, "endpoint", biography.SPARQLEndpoint,
		"the SPARQL endpoint to query")
	fs.StringVar(&options.UserAgent, "user-agent", userAgentFromEnv(),
		"User-Agent sent to the query service; it asks callers to identify themselves")
	fs.IntVar(&options.BatchSize, "batch", biography.DefaultBatchSize,
		"ESPNcricinfo ids per SPARQL query")
	fs.DurationVar(&options.Pause, "pause", DefaultPause, "wait between SPARQL batches")
	fs.DurationVar(&options.Timeout, "timeout", DefaultTimeout, "overall run timeout")
	fs.BoolVar(&options.ReportOnly, "report-only", false,
		"re-measure what is stored and write the report, fetching nothing")
	fs.BoolVar(&options.Offline, "offline", false,
		"rebuild from committed snapshots, making no network call; -register must be a local copy")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	return options, options.validate()
}

// validate refuses settings that would produce a run whose result cannot be trusted.
func (o Options) validate() error {
	if o.ReportOnly && o.Offline {
		return errors.New("-report-only and -offline are alternatives: -report-only writes " +
			"no rows at all, -offline writes them from the committed snapshots")
	}
	if !o.ReportOnly && strings.TrimSpace(o.Register) == "" {
		return errors.New("a people register is required: pass -register")
	}
	if o.Offline && strings.HasPrefix(o.Register, "https://") {
		// The default register is a URL, so an -offline run that inherited it would
		// quietly fetch — the one thing this mode exists to rule out.
		return errors.New("-offline makes no network call, so -register must name a local " +
			"copy of the people register, not " + o.Register)
	}
	if !o.ReportOnly && !o.Offline && strings.TrimSpace(o.UserAgent) == "" {
		return errors.New(
			"the query service requires a User-Agent identifying the caller: set " + UserAgentEnvVar)
	}
	if o.BatchSize <= 0 {
		return errors.New("-batch must be positive")
	}
	if o.Pause < 0 {
		return errors.New("-pause cannot be negative")
	}
	if o.Timeout <= 0 {
		return errors.New("-timeout must be positive")
	}
	return nil
}

// userAgentFromEnv reads the override, falling back to the built-in identity.
func userAgentFromEnv() string {
	if value := strings.TrimSpace(os.Getenv(UserAgentEnvVar)); value != "" {
		return value
	}
	return DefaultUserAgent
}

// LoadRegister reads the people register, from https or from a local file.
//
// Both are supported for the same reason acquisition stages an archive before importing
// it: an operator on a box with no outbound network, or one who wants a run pinned to a
// register they have already seen, should not have to patch the command to get one.
func LoadRegister(client *http.Client, userAgent, source string) (map[string]biography.RegisterEntry, error) {
	if !strings.HasPrefix(source, "https://") {
		file, err := os.Open(filepath.Clean(source))
		if err != nil {
			return nil, fmt.Errorf("opening the register %s: %w", source, err)
		}
		defer func() { _ = file.Close() }()
		return biography.ParseRegister(file)
	}

	request, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetching the register: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the register answered %d", response.StatusCode)
	}
	return biography.ParseRegister(response.Body)
}

// LoadOverrides reads the curated file. A missing file is an empty map and no error: the
// normal state of this project is that no override has been needed yet, and a command
// that failed on its absence would make the file mandatory for no reason.
func LoadOverrides(path string) (map[string]biography.Override, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	file, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening the overrides %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	return biography.ParseOverrides(file)
}

// WriteFile writes a report, creating the directory it lives in.
func WriteFile(path string, content []byte) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return os.WriteFile(path, content, 0o600)
}
