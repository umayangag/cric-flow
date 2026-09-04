package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	"github.com/umayangag/cric-flow/go-app/internal/services/datasetregistry"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// fetchCommand is the data_migrations command for a dataset download. It is the
// registry's command for the "fetch" step, not a literal chosen here.
var (
	fetchCommand   = mustStepCommand("fetch")
	extractCommand = mustStepCommand("extract")
)

// dataFeedsHandler handles GET /ops/data/feeds.
//
// It returns the named feeds and the host allowlist together, so the UI can state
// the rule up front rather than let an operator discover it by being refused.
func (a *App) dataFeedsHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"feeds":         dataacquire.Feeds(),
		"allowed_hosts": dataacquire.AllowedHosts(),
		"staging_dir":   dataset.StagingDir(),
	})
}

// dataStagedHandler handles GET /ops/data/staged: the archives available to extract,
// each with whatever provenance its fetch recorded, plus the manifest of the dataset
// currently live. Together they answer "what is on this box, and where did it come
// from?" — the question A-3 turns into a registry.
func (a *App) dataStagedHandler(w http.ResponseWriter, _ *http.Request) {
	dir := dataset.Dir()
	payload := map[string]any{
		"staging_dir": dataset.StagingDir(),
		"archives":    dataacquire.ListStaged(dataset.StagingDir()),
		"dataset_dir": dir,
	}
	if manifest, ok := dataacquire.ReadManifest(dir); ok {
		payload["live"] = manifest
	}
	respondJSON(w, http.StatusOK, payload)
}

// dataExtractRequest is the body of POST /ops/data/extract. An empty Archive means
// the newest staged archive.
type dataExtractRequest struct {
	Archive string `json:"archive"`
}

// dataExtractHandler handles POST /ops/data/extract: it inflates a staged archive
// into the dataset directory as a tracked background job.
//
// Like fetch it answers 202. Unlike fetch, the risk is not the network but the
// archive: see dataacquire.Extract for the zip-slip, bomb and postcondition checks,
// all of which run before a single file reaches the live directory.
func (a *App) dataExtractHandler(w http.ResponseWriter, r *http.Request) {
	var body dataExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Info("data extract: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}

	stagingDir := dataset.StagingDir()
	archive, err := dataacquire.ResolveStagedArchive(stagingDir, body.Archive)
	if err != nil {
		slog.Info("data extract: no usable archive",
			slog.String("archive", body.Archive), slog.Any("err", err))
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"error":       err.Error(),
			"staging_dir": stagingDir,
			"archives":    dataacquire.ListStaged(stagingDir),
		})
		return
	}

	if busy, _ := pipeline.LaneBusy(r.Context(), extractCommand); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": pipeline.ErrPipelineBusy.Error()})
		return
	}

	destDir := dataset.Dir()
	args := map[string]any{"archive": archive, "dest_dir": destDir}
	slog.Info("data extract: request accepted, starting background job", slog.Any("args", args))

	a.startTrackedJob(extractCommand, args, dataacquire.DefaultTimeout,
		func(ctx context.Context) (any, error) {
			defer dataacquire.ClearExtractProgress()
			result, err := dataacquire.Extract(ctx, dataacquire.ExtractOptions{
				ArchivePath: archive,
				DestDir:     destDir,
				WorkDir:     stagingDir,
				Progress: func(p dataacquire.ExtractProgress) {
					dataacquire.PublishExtractProgress(filepath.Base(archive), p)
				},
			})
			if err != nil {
				return nil, err
			}
			recordExtract(ctx, result)
			return result, nil
		})

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"step":     "extract",
		"archive":  filepath.Base(archive),
		"dest_dir": destDir,
	})
}

// dataFetchRequest is the body of POST /ops/data/fetch. Exactly one of Feed and URL.
type dataFetchRequest struct {
	Feed string `json:"feed"`
	URL  string `json:"url"`
}

// dataFetchHandler handles POST /ops/data/fetch: it downloads a Cricsheet archive
// into the staging directory as a tracked background job.
//
// It answers 202 rather than holding the request open. A multi-hundred-megabyte
// archive over a slow link outlives any sane HTTP timeout, and a request-scoped
// download would be cancelled by the proxy mid-transfer with nothing recorded —
// which is why every long step in this service is a tracked job.
func (a *App) dataFetchHandler(w http.ResponseWriter, r *http.Request) {
	var body dataFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Info("data fetch: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}

	src, err := dataacquire.ResolveSource(body.Feed, body.URL)
	if err != nil {
		slog.Info("data fetch: rejected source",
			slog.String("feed", body.Feed), slog.String("url", body.URL), slog.Any("err", err))
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"error":         err.Error(),
			"allowed_hosts": dataacquire.AllowedHosts(),
		})
		return
	}

	if busy, _ := pipeline.LaneBusy(r.Context(), fetchCommand); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": pipeline.ErrPipelineBusy.Error()})
		return
	}

	stagingDir := dataset.StagingDir()
	args := map[string]any{
		"feed":        src.FeedID,
		"url":         src.URL.Redacted(),
		"filename":    src.Filename,
		"staging_dir": stagingDir,
	}
	slog.Info("data fetch: request accepted, starting background job", slog.Any("args", args))

	a.startTrackedJob(fetchCommand, args, dataacquire.DefaultTimeout,
		func(ctx context.Context) (any, error) {
			// Clearing on every exit — success, failure or cancellation — is what
			// stops the console from showing a transfer that is no longer running.
			defer dataacquire.ClearProgress()
			result, err := dataacquire.Fetch(ctx, src, dataacquire.Options{
				StagingDir: stagingDir,
				Progress: func(p dataacquire.Progress) {
					dataacquire.PublishProgress(src.URL.Redacted(), p)
				},
			})
			if err != nil {
				return nil, err
			}
			recordFetch(ctx, result)
			return result, nil
		})

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"step":     "fetch",
		"feed":     src.FeedID,
		"url":      src.URL.Redacted(),
		"filename": src.Filename,
	})
}

// mustStepCommand returns a registry step's command, panicking at startup if the step
// is missing. A typo here would otherwise surface as a job recorded under a command
// no lane recognises — which LaneForCommand would then treat as LaneCompute, quietly
// making the download block training. Failing at init is the cheaper failure.
func mustStepCommand(stepID string) string {
	step, ok := pipelinesvc.Steps().ByID(stepID)
	if !ok {
		panic("pipeline registry has no step " + strings.TrimSpace(stepID))
	}
	return step.Command
}

// dataDatasetsHandler handles GET /ops/data/datasets: the dataset registry, newest
// first, with the row currently live in the data directory marked.
//
// Which dataset is live is read from the on-disk manifest rather than a column. An
// operator who rsyncs files into the data directory changes what is live without
// touching Postgres; a stored flag would go on asserting the old answer, and a
// provenance record that can be quietly wrong is worse than none.
func (a *App) dataDatasetsHandler(w http.ResponseWriter, r *http.Request) {
	limit := datasetListLimit(r.URL.Query().Get("limit"))

	dir := dataset.Dir()
	liveSHA := ""
	if manifest, ok := dataacquire.ReadManifest(dir); ok {
		liveSHA = manifest.ArchiveSHA256
	}

	datasets, err := datasetregistry.List(r.Context(), limit, liveSHA)
	if err != nil {
		slog.Error("ops datasets: list failed", slog.Any("err", err))
		respondJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{"error": "could not read the dataset registry"},
		)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"datasets":    datasets,
		"dataset_dir": dir,
		// live_sha256 is reported even when no row matches it, because "the directory
		// holds a dataset this registry has never seen" is a state worth showing
		// rather than rendering as an empty list.
		"live_sha256": liveSHA,
	})
}

// dataBiographyCoverageHandler handles GET /ops/data/biography-coverage: how much of the
// archive the acquired player biographies (X-1a) actually cover, per format and gender.
//
// It sits beside the dataset registry rather than in a log because the coverage *is* the
// state of a data source, and §8.7's rule is that the data's state belongs on a surface.
// A biography pass that has not been run and one that ran and found nothing produce very
// different figures here, and neither is visible from the row counts.
func (a *App) dataBiographyCoverageHandler(w http.ResponseWriter, r *http.Request) {
	coverage, err := db.NewPlayerBiographyStore().Coverage(r.Context())
	if err != nil {
		slog.Error("ops biography coverage: measuring failed", slog.Any("err", err))
		respondJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "could not measure biography coverage"})
		return
	}
	respondJSON(w, http.StatusOK, coverage)
}

// Dataset listing bounds, mirroring the backtest list conventions.
const (
	defaultDatasetListLimit = 50
	maxDatasetListLimit     = 200
)

// datasetListLimit reads the ?limit= parameter, bounded.
//
// Absent, unparseable and non-positive all mean the default rather than "no limit":
// an unbounded list endpoint is a slow query waiting for the registry to grow.
func datasetListLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return defaultDatasetListLimit
	}
	return min(n, maxDatasetListLimit)
}

// recordFetch and recordExtract write the registry row for a completed step.
//
// A failure here is logged, not returned. The step itself has already succeeded —
// the bytes are on disk — and failing the job for a registry write would report work
// that happened as work that did not. Provenance is not lost either way: it is in the
// job's own data_migrations metadata, in the archive's sidecar and in the extracted
// directory's manifest. The registry is the index over those, not their only copy.
func recordFetch(ctx context.Context, result dataacquire.Result) {
	err := datasetregistry.RecordFetch(ctx, datasetregistry.FetchRecord{
		SHA256:       result.SHA256,
		Feed:         result.FeedID,
		SourceURL:    result.SourceURL,
		Filename:     filepath.Base(result.Path),
		Bytes:        result.Bytes,
		ETag:         result.ETag,
		LastModified: result.LastModified,
		FetchedAt:    parseTimestamp(result.FetchedAt),
	})
	if err != nil {
		slog.Warn("dataset registry: recording the fetch failed",
			slog.String("sha256", result.SHA256), slog.Any("err", err))
	}
}

func recordExtract(ctx context.Context, result dataacquire.ExtractResult) {
	err := datasetregistry.RecordExtract(ctx, datasetregistry.ExtractRecord{
		SHA256:         result.ArchiveSHA256,
		Feed:           result.FeedID,
		SourceURL:      result.SourceURL,
		Filename:       filepath.Base(result.ArchivePath),
		EntryCount:     result.Entries,
		MatchFiles:     result.MatchFiles,
		ExtractedBytes: result.Bytes,
		DestDir:        result.DestDir,
		ExtractedAt:    parseTimestamp(result.ExtractedAt),
	})
	if err != nil {
		slog.Warn("dataset registry: recording the extract failed",
			slog.String("sha256", result.ArchiveSHA256), slog.Any("err", err))
	}
}

// parseTimestamp reads an RFC3339 stamp, falling back to now. The values come from
// results this process just produced, so a parse failure means a bug rather than bad
// input — but a wrong-by-seconds timestamp beats a zero one that renders as year 1.
func parseTimestamp(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}
