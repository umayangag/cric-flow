package opsstatus_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/freshness"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
)

// contractPath is the generated H-24 contract, read from this package the way the
// frontend and ml-service read it.
const contractPath = "../../../../contracts/ops-console.contract.json"

// stubProbe is a database that answers with whatever the test set, so the freshness
// object can be assembled without a database.
type stubProbe struct {
	latest    map[string]time.Time
	counts    map[string]int64
	latestErr map[string]error
	countErr  map[string]error
}

func (p stubProbe) LatestMatchDateByFormat(_ context.Context, format string) (time.Time, error) {
	if err, ok := p.latestErr[format]; ok {
		return time.Time{}, err
	}
	return p.latest[format], nil
}

func (p stubProbe) CountMatchesByFormat(_ context.Context, format string) (int64, error) {
	if err, ok := p.countErr[format]; ok {
		return 0, err
	}
	return p.counts[format], nil
}

func (p stubProbe) CountMatchesSinceByFormat(_ context.Context, format string, _ time.Time) (int64, error) {
	return p.counts[format], nil
}

type probeError struct{}

func (probeError) Error() string { return "connection refused" }

func day(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, value)
	require.NoError(t, err)
	return parsed
}

// artifactsFromMLService runs the real copy-through path: an httptest ml-service answers
// /artifacts/status with the verdict, BuildArtifactsSection copies it whole, and the
// freshness object is assembled off that. The seam matters — the point of the object is
// that H-11's verdict travels from the process that computed it to the badge unchanged.
func artifactsFromMLService(t *testing.T, verdict map[string]any) map[string]any {
	t.Helper()
	client := mlServiceStub(t, okHealth, func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"root":            "/models",
			"current_run":     "20260903T160602Z-0e1e39c2",
			"loaded_run":      "20260903T160602Z-0e1e39c2",
			"ratings_through": verdict["ratings_through"],
			"ratings":         verdict,
			"runs":            []map[string]any{{"run_id": "20260903T160602Z-0e1e39c2", "loaded": true}},
		})
	})
	section, mlOK := opsstatus.BuildArtifactsSection(client, t.TempDir())
	require.True(t, mlOK)
	return section
}

// The P0-4 state, reproduced as a unit test: TEST's latest match is 8 days old — past the
// deleted 7-day bucket, which read `stale` — while the served ratings are 2 days of 14,
// which H-11 reads `fresh`. One verdict now, with the format's lag stated beside it as a
// fact rather than as a second opinion.
func TestBuildFreshnessSection_SparseFormatIsALagNotAVerdict(t *testing.T) {
	now := day(t, "2026-09-04")
	probe := stubProbe{
		latest: map[string]time.Time{
			"TEST": day(t, "2026-08-27"),
			"ODI":  day(t, "2026-09-01"),
			"T20I": day(t, "2026-09-01"),
			"T20":  day(t, "2026-09-02"),
		},
		counts: map[string]int64{"TEST": 2857, "ODI": 4835, "T20I": 3402, "T20": 11724},
	}
	artifacts := artifactsFromMLService(t, map[string]any{
		"fresh": true, "age_days": 2, "max_age_days": 14,
		"ratings_through": "2026-09-02", "code": nil,
	})

	got := opsstatus.BuildFreshnessSection(context.Background(), probe, artifacts, now)

	assert.Equal(t, freshness.Fresh, got.Served.Status, "H-11 decides the badge, alone")
	assert.True(t, got.Served.Fresh)
	require.NotNil(t, got.Served.AgeDays)
	assert.Equal(t, 2, *got.Served.AgeDays)
	require.NotNil(t, got.Served.MaxAgeDays)
	assert.Equal(t, 14, *got.Served.MaxAgeDays, "the limit is ml-service's, copied through")
	require.NotNil(t, got.Served.RatingsThrough)
	assert.Equal(t, "2026-09-02", *got.Served.RatingsThrough)
	assert.Nil(t, got.Served.Code)

	test := got.Database["TEST"]
	require.NotNil(t, test.LatestMatchDate)
	assert.Equal(t, "2026-08-27", *test.LatestMatchDate)
	require.NotNil(t, test.AgeDays)
	assert.Equal(t, 8, *test.AgeDays, "the lag is a fact, and 8 days of Test cricket is not a fault")
	assert.Equal(t, int64(2857), test.MatchCount)
	assert.Empty(t, test.Note)

	assert.Equal(t, freshness.UpToDate, got.RetrainDue.Status)
	require.NotNil(t, got.RetrainDue.DaysBehind)
	assert.Equal(t, 0, *got.RetrainDue.DaysBehind)
	require.NotNil(t, got.RetrainDue.LatestMatchDate)
	assert.Equal(t, "2026-09-02", *got.RetrainDue.LatestMatchDate)
	assert.Equal(t, "T20", got.RetrainDue.Format)
}

// The B-2 state a green pipeline once hid: matches imported that the served run never
// saw. Nothing about it is stale — H-11 still says fresh — so it needs its own fact.
func TestBuildFreshnessSection_ImportsTheServedRunNeverSaw(t *testing.T) {
	now := day(t, "2026-09-07")
	probe := stubProbe{
		latest: map[string]time.Time{"ODI": day(t, "2026-09-05"), "T20": day(t, "2026-09-06")},
		counts: map[string]int64{"ODI": 10, "T20": 20},
	}
	artifacts := artifactsFromMLService(t, map[string]any{
		"fresh": true, "age_days": 5, "max_age_days": 14,
		"ratings_through": "2026-09-02", "code": nil,
	})

	got := opsstatus.BuildFreshnessSection(context.Background(), probe, artifacts, now)

	assert.Equal(t, freshness.Fresh, got.Served.Status)
	assert.Equal(t, freshness.RetrainDue, got.RetrainDue.Status)
	require.NotNil(t, got.RetrainDue.DaysBehind)
	assert.Equal(t, 4, *got.RetrainDue.DaysBehind)
	assert.Equal(t, "T20", got.RetrainDue.Format)
}

// The limit is ml-service's and only ml-service's. Here the served ratings are 5 days old
// against a limit of 3 — a state the deleted 7-day bucket would have called `ok` — and
// go-app reports exactly what it was told, because it holds no threshold to check with.
func TestBuildFreshnessSection_StaleVerdictIsCopiedNeverRecomputed(t *testing.T) {
	now := day(t, "2026-09-07")
	probe := stubProbe{
		latest: map[string]time.Time{"T20": day(t, "2026-09-02")},
		counts: map[string]int64{"T20": 20},
	}
	artifacts := artifactsFromMLService(t, map[string]any{
		"fresh": false, "age_days": 5, "max_age_days": 3,
		"ratings_through": "2026-09-02", "code": freshness.RatingsStaleCode,
	})

	got := opsstatus.BuildFreshnessSection(context.Background(), probe, artifacts, now)

	assert.Equal(t, freshness.Stale, got.Served.Status)
	assert.False(t, got.Served.Fresh)
	require.NotNil(t, got.Served.MaxAgeDays)
	assert.Equal(t, 3, *got.Served.MaxAgeDays)
	require.NotNil(t, got.Served.Code)
	assert.Equal(t, freshness.RatingsStaleCode, *got.Served.Code)
	require.NotNil(t, got.Served.RatingsThrough)
	assert.Equal(t, "2026-09-02", *got.Served.RatingsThrough,
		"the badge and the refusal name the same date")
}

// Nothing loaded is not the same state as stale: the remedy is a reload, not a retrain,
// and there is no age to report because there is no state to be old.
func TestBuildFreshnessSection_NothingLoaded(t *testing.T) {
	now := day(t, "2026-09-07")
	probe := stubProbe{
		latest: map[string]time.Time{"T20": day(t, "2026-09-06")},
		counts: map[string]int64{"T20": 20},
	}
	artifacts := artifactsFromMLService(t, map[string]any{
		"fresh": false, "age_days": nil, "max_age_days": 14,
		"ratings_through": nil, "code": nil,
	})

	got := opsstatus.BuildFreshnessSection(context.Background(), probe, artifacts, now)

	assert.Equal(t, freshness.NotLoaded, got.Served.Status)
	assert.Nil(t, got.Served.AgeDays, "no state, so no age; an invented one would be a lie")
	assert.Nil(t, got.Served.RatingsThrough)
	assert.Equal(t, freshness.Unknown, got.RetrainDue.Status,
		"nothing to compare the import against")
	require.NotNil(t, got.RetrainDue.LatestMatchDate, "the import's own date is still a fact")
	assert.Equal(t, "2026-09-06", *got.RetrainDue.LatestMatchDate)
}

// An unreachable ml-service leaves the artifacts section on its filesystem fallback,
// which can say what is on disk but never what is loaded. `unknown` says so; reporting
// `fresh` there would invent the one fact this object exists to carry (§8.7).
func TestBuildFreshnessSection_MLServiceSilentReadsUnknown(t *testing.T) {
	now := day(t, "2026-09-07")
	probe := stubProbe{
		latest: map[string]time.Time{"T20": day(t, "2026-09-06")},
		counts: map[string]int64{"T20": 20},
	}

	got := opsstatus.BuildFreshnessSection(context.Background(), probe,
		map[string]any{"root": "/models", "reachable": false}, now)

	assert.Equal(t, freshness.Unknown, got.Served.Status)
	assert.False(t, got.Served.Fresh)
	assert.Nil(t, got.Served.MaxAgeDays)
	assert.Equal(t, freshness.Unknown, got.RetrainDue.Status)
}

// A format with no rows and a format the database would not answer for are different
// states, and both are on the wire rather than only in a log (§8.7).
func TestBuildFreshnessSection_DatabaseSilencesAreNamed(t *testing.T) {
	now := day(t, "2026-09-07")
	probe := stubProbe{
		latest:    map[string]time.Time{"T20": day(t, "2026-09-06")},
		counts:    map[string]int64{"T20": 20},
		latestErr: map[string]error{"ODI": probeError{}},
		countErr:  map[string]error{"T20I": probeError{}},
	}
	artifacts := artifactsFromMLService(t, map[string]any{
		"fresh": true, "age_days": 1, "max_age_days": 14,
		"ratings_through": "2026-09-06", "code": nil,
	})

	got := opsstatus.BuildFreshnessSection(context.Background(), probe, artifacts, now)

	assert.Nil(t, got.Database["TEST"].LatestMatchDate)
	assert.NotEmpty(t, got.Database["TEST"].Note, "no rows is a stated fact")
	assert.NotEmpty(t, got.Database["ODI"].Note, "an unreadable date is a stated fact")
	assert.NotEmpty(t, got.Database["T20I"].Note, "an unreadable count is a stated fact")
	require.NotNil(t, got.Database["T20"].AgeDays)
	assert.Equal(t, 1, *got.Database["T20"].AgeDays)
}

// With no database at all, every format says why rather than reading as an empty import.
func TestBuildFreshnessSection_NoProbe(t *testing.T) {
	now := day(t, "2026-09-07")

	got := opsstatus.BuildFreshnessSection(context.Background(), nil, map[string]any{}, now)

	require.Len(t, got.Database, len(opsstatus.CricketFormatCodes))
	for _, code := range opsstatus.CricketFormatCodes {
		assert.NotEmpty(t, got.Database[code].Note, "format %s must say why it has no date", code)
	}
	assert.Equal(t, freshness.Unknown, got.RetrainDue.Status)
}

// go-app's half of H-24 for the freshness vocabulary: every word this service can put on
// the wire is one the contract publishes, so the frontend cannot be rendering a status
// word the backend never sends or missing one it does.
func TestFreshnessVocabularyIsTheContracts(t *testing.T) {
	raw, err := os.ReadFile(contractPath)
	require.NoError(t, err, "contract file missing; regenerate it from the pipeline registry")
	var contract struct {
		FreshnessStatuses []string `json:"freshness_statuses"`
		RetrainStatuses   []string `json:"retrain_statuses"`
		RatingsStaleCode  string   `json:"ratings_stale_code"`
	}
	require.NoError(t, json.Unmarshal(raw, &contract))

	assert.ElementsMatch(t, freshness.Statuses(), contract.FreshnessStatuses)
	assert.ElementsMatch(t, freshness.RetrainStatuses(), contract.RetrainStatuses)
	assert.Equal(t, freshness.RatingsStaleCode, contract.RatingsStaleCode)

	// And the words are the ones the assembly actually emits, not a list beside it.
	assert.Contains(t, contract.FreshnessStatuses, freshness.Fresh)
	assert.Contains(t, contract.FreshnessStatuses, freshness.Stale)
	assert.Contains(t, contract.FreshnessStatuses, freshness.NotLoaded)
	assert.Contains(t, contract.FreshnessStatuses, freshness.Unknown)
	assert.Contains(t, contract.RetrainStatuses, freshness.RetrainDue)
	assert.Contains(t, contract.RetrainStatuses, freshness.UpToDate)
	assert.Contains(t, contract.RetrainStatuses, freshness.Unknown)
}
