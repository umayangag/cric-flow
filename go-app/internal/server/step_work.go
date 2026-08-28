package server

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	exportsvc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// StepRequest carries the optional inputs a step accepts.
//
// The zero value is what a run plan supplies: a plan runs the pipeline as configured,
// and every field here has a default that the single-step handlers also fall back to.
// Anything a plan would have to invent is not a field.
type StepRequest struct {
	// Cutoff is the RFC3339 training cutoff for ml-service steps. Empty means the
	// configured default.
	Cutoff string
	// ImportDir overrides the dataset directory for an import. Empty means the
	// configured one — which is the same directory /ops/status reports.
	ImportDir string
	// PlaceholdersFielding fills missing fielding rows during import.
	PlaceholdersFielding bool
	// Season and Formats narrow a precompute. Empty means all.
	Season  string
	Formats []string
	// ConfirmDefaultParams allows a training step to run on config defaults when the
	// model has no auto-tuned parameters.
	//
	// Single-step runs ask first (confirmDefaultParams answers the request and the
	// client re-posts), because training on defaults by accident is a silent way to
	// get a worse model. A plan cannot stop to ask, and asking for the whole pipeline
	// *is* the confirmation — but the run records that it happened, so "why is this
	// model worse?" has an answer in the history rather than nowhere.
	ConfirmDefaultParams bool
	// Feed and SourceURL name the archive an acquisition step works on. Empty means
	// the configured source (inputs.cricsheet_source_url), which is what a plan and
	// what Import both use.
	Feed      string
	SourceURL string
	// Archive names the staged .zip to extract. Empty means the newest one.
	Archive string
}

// StepJob is everything needed to run one step: what it is recorded as, how long it
// may take, the arguments worth recording, and the work itself.
type StepJob struct {
	Command string
	Args    map[string]any
	Timeout time.Duration
	Run     pipeline.JobFunc
}

// stepJob builds the job for a step.
//
// This is the single definition of what a step *does*, shared by the single-step
// handlers and by the run-plan executor (ops plan R-1). Before it, each step's work
// lived inline in its handler, and a plan would have needed its own copy — which is
// how the two would have drifted, in exactly the way six parallel step tables drifted
// before F-1 replaced them with one registry.
//
// The handlers keep their own request parsing; only the work is shared.
func (a *App) stepJob(step pipelinesvc.Step, req StepRequest) StepJob {
	switch step.ID {
	case "import":
		dir := req.ImportDir
		if dir == "" {
			dir = dataset.Dir()
		}
		opts := &cricsheet.Options{PlaceholdersFielding: req.PlaceholdersFielding}
		return StepJob{
			Command: step.Command,
			Args:    map[string]any{"dir": dir},
			Timeout: config.PipelineTimeout(),
			Run: func(ctx context.Context) (any, error) {
				n, err := cricsheet.ImportDir(ctx, dir, opts, 0)
				return map[string]any{"files": n, "dir": dir}, err
			},
		}

	// Acquisition. These live here, beside the compute steps, because a run plan must
	// be able to run them: Import acquires what it imports (consumer plan W6-2), and a
	// second definition of "what fetch does" is the drift F-1 spent a whole PR undoing.
	case "fetch":
		src, err := a.acquisitionSource(req)
		stagingDir := dataset.StagingDir()
		return StepJob{
			Command: step.Command,
			Args: map[string]any{
				"feed": req.Feed, "url": sourceURLForArgs(src, err), "staging_dir": stagingDir,
			},
			Timeout: dataacquire.DefaultTimeout,
			Run: func(ctx context.Context) (any, error) {
				if err != nil {
					return nil, err
				}
				// Clearing on every exit — success, failure or cancellation — is what
				// stops the console from showing a transfer that is no longer running.
				defer dataacquire.ClearProgress()
				result, fetchErr := dataacquire.Fetch(ctx, src, dataacquire.Options{
					StagingDir: stagingDir,
					Progress: func(p dataacquire.Progress) {
						dataacquire.PublishProgress(src.URL.Redacted(), p)
					},
				})
				if fetchErr != nil {
					return nil, fetchErr
				}
				recordFetch(ctx, result)
				return result, nil
			},
		}

	case "extract":
		stagingDir := dataset.StagingDir()
		destDir := dataset.Dir()
		return StepJob{
			Command: step.Command,
			Args:    map[string]any{"archive": req.Archive, "dest_dir": destDir},
			Timeout: dataacquire.DefaultTimeout,
			Run: func(ctx context.Context) (any, error) {
				// Resolved inside Run rather than when the job is built: in a plan this
				// step is built before fetch has downloaded anything, so resolving
				// earlier would look at a staging directory that is still empty.
				archive, resolveErr := dataacquire.ResolveStagedArchive(stagingDir, req.Archive)
				if resolveErr != nil {
					return nil, resolveErr
				}
				defer dataacquire.ClearExtractProgress()
				result, extractErr := dataacquire.Extract(ctx, dataacquire.ExtractOptions{
					ArchivePath: archive,
					DestDir:     destDir,
					WorkDir:     stagingDir,
					Progress: func(p dataacquire.ExtractProgress) {
						dataacquire.PublishExtractProgress(filepath.Base(archive), p)
					},
				})
				if extractErr != nil {
					return nil, extractErr
				}
				recordExtract(ctx, result)
				return result, nil
			},
		}

	case "precompute":
		season, formats := req.Season, req.Formats
		return StepJob{
			Command: step.Command,
			Args:    map[string]any{"season": season, "formats": formats},
			Timeout: config.PipelineTimeout(),
			Run: func(ctx context.Context) (any, error) {
				err := precompute.Run(ctx, season, formats, nil)
				return map[string]any{"season": season, "formats": formats}, err
			},
		}

	case "export":
		outDir := config.DefaultExportDir()
		return StepJob{
			Command: step.Command,
			Args:    map[string]any{"out_dir": outDir},
			Timeout: config.ExportTimeout(),
			Run: func(ctx context.Context) (any, error) {
				repo := &exportqueries.Repo{}
				runner := exportsvc.NewRunnerWithServices(
					exportsvc.NewBattingService(repo),
					exportsvc.NewBowlingService(repo),
					exportsvc.NewFieldingService(repo),
					exportsvc.NewExtrasService(repo),
					exportsvc.NewWinService(repo),
				)
				// Provenance here as well as in runExportHandler: a plan-driven export
				// and a manually triggered one must produce the same manifest, and
				// without this the plan's would name no dataset at all — silently,
				// which is the failure this whole phase exists to prevent.
				opts := exportsvc.Options{
					OutDir:     outDir,
					Unified:    true,
					Provenance: liveDatasetProvenance(),
				}
				return map[string]any{"out_dir": outDir}, runner.Run(ctx, opts)
			},
		}
	}

	// Everything else runs on ml-service.
	cutoff := req.Cutoff
	if cutoff == "" {
		cutoff = pipelinesvc.DefaultCutoff()
	}
	query := url.Values{"cutoff": []string{cutoff}}
	args := map[string]any{"step": step.ID, "cutoff": cutoff}
	if req.ConfirmDefaultParams {
		query.Set("confirm_use_default", "1")
		args["confirm_use_default"] = true
	}
	return StepJob{
		Command: step.Command,
		Args:    args,
		Timeout: pipelinesvc.TrainStepTimeout(),
		Run: func(ctx context.Context) (any, error) {
			result, err := pipelinesvc.CallMLTrainEndpointWithResult(ctx, step.MLEndpoint, "?"+query.Encode())
			if err != nil {
				return nil, err
			}
			return trainRunMetadata(step, cutoff, result), nil
		},
	}
}

// acquisitionSource resolves the archive a fetch will download.
//
// The configured source is the default and the normal path (consumer plan W6-1): a
// deployment pulls the same archive every time, so being asked to choose one on every
// run was the friction that made acquisition a three-step dance. An explicit feed or
// URL still overrides it, and all three go through the same allowlist — being
// configured is not an exemption.
func (a *App) acquisitionSource(req StepRequest) (dataacquire.Source, error) {
	feed, rawURL := req.Feed, req.SourceURL
	if feed == "" && rawURL == "" {
		rawURL = config.CricsheetSourceURL()
	}
	return dataacquire.ResolveSource(feed, rawURL)
}

// sourceURLForArgs is what a run records as its source. A source that failed to
// resolve still has to record *something*, or the failed run in the history would not
// say what it was trying to fetch.
func sourceURLForArgs(src dataacquire.Source, err error) string {
	if err != nil || src.URL == nil {
		return ""
	}
	return src.URL.Redacted()
}
