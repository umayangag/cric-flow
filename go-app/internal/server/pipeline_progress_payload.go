package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// pipelineProgressPayload is the JSON sent in each SSE "progress" event.
//
// It carries every in-flight step, not one. The stream used to report inProgress[0]
// and call that "the" running step — true only while a single global lock made it
// true, which stopped being the case the moment acquisition got its own lane, and
// stops being it again when the run-plan executor lands.
type pipelineProgressPayload struct {
	// Running is true when at least one step is in flight. Kept as its own field so
	// a client can answer "is anything happening?" without inspecting the list.
	Running bool `json:"running"`
	// Steps holds one entry per in-flight step, most recently started first.
	Steps []stepProgress `json:"steps"`
}

// stepProgress is the live state of one running step.
type stepProgress struct {
	StepID     string                 `json:"step_id,omitempty"`
	StepLabel  string                 `json:"step_label,omitempty"`
	Lane       string                 `json:"lane,omitempty"`
	Detail     string                 `json:"detail,omitempty"` // Human-readable: what is happening
	Params     map[string]interface{} `json:"params,omitempty"` // Current parameters (from migration args)
	StartedAt  string                 `json:"started_at,omitempty"`
	ElapsedSec int64                  `json:"elapsed_sec,omitempty"`
	// Precompute carries per-format progress; only precompute reports it today.
	Precompute *precomputeProgress `json:"precompute,omitempty"`
	// EstimatedSec is seconds remaining, when the step can estimate it.
	EstimatedSec *int64 `json:"estimated_remaining_sec,omitempty"`
	// AutoTune carries live auto-tune progress (phase, algorithm, trial, ...).
	AutoTune map[string]interface{} `json:"auto_tune,omitempty"`
	// Fetch carries live download progress (bytes, rate, ETA) for a dataset fetch.
	Fetch *dataacquire.Progress `json:"fetch,omitempty"`
	// Extract carries live extraction progress (entries, bytes) for a dataset extract.
	Extract *dataacquire.ExtractProgress `json:"extract,omitempty"`
	// Training carries live milestones published by a trainer (ops plan O-2): rows
	// loaded, low-variance columns dropped, CV folds, artifacts written.
	Training map[string]interface{} `json:"training,omitempty"`
	// ProgressUnavailable is true when ml-service could not be asked, as distinct
	// from it answering "nothing published yet".
	//
	// Both render as an empty panel otherwise, but one is a run about to report and
	// the other is a broken link, and the operator can act on the second.
	ProgressUnavailable bool `json:"progress_unavailable,omitempty"`
}

type precomputeProgress struct {
	Formats       []string `json:"formats,omitempty"`
	CurrentFormat string   `json:"current_format,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	// Index of current format in Formats (0-based) for progress bar
	CurrentIndex int `json:"current_index,omitempty"`
	FormatsTotal int `json:"formats_total,omitempty"`
}

// progressReporter turns the in-flight rows of data_migrations into the SSE payload.
// It is separated from the SSE transport so the shape of the payload can be read —
// and tested — without a live HTTP stream.
type progressReporter struct {
	registry *pipelinesvc.Registry
	// autoTuneProgress fetches live auto-tune state; a field so tests can stub it.
	autoTuneProgress func(context.Context) map[string]interface{}
	// stepProgress fetches live milestones for a training step, distinguishing
	// "nothing yet" (nil, nil) from "could not ask" (nil, error).
	stepProgress func(context.Context, string) (map[string]interface{}, error)
	// precomputeStatus reads in-process precompute state; a field for the same reason.
	precomputeStatus func() precompute.Status
	// fetchStatus and extractStatus read in-process acquisition state, likewise stubbable.
	fetchStatus   func() (dataacquire.Progress, string, bool)
	extractStatus func() (dataacquire.ExtractProgress, string, bool)
	now           func() time.Time
}

func newProgressReporter() *progressReporter {
	return &progressReporter{
		registry:         pipelinesvc.Steps(),
		autoTuneProgress: pipelinesvc.FetchAutoTuneProgress,
		stepProgress:     pipelinesvc.FetchStepProgress,
		precomputeStatus: precompute.GetStatus,
		fetchStatus:      dataacquire.Status,
		extractStatus:    dataacquire.ExtractStatus,
		now:              time.Now,
	}
}

// snapshot describes every step currently in flight.
func (p *progressReporter) snapshot(ctx context.Context, running []tracking.Migration) pipelineProgressPayload {
	steps := make([]stepProgress, 0, len(running))
	for _, m := range running {
		steps = append(steps, p.describe(ctx, m))
	}
	return pipelineProgressPayload{Running: len(steps) > 0, Steps: steps}
}

// describe renders one in-flight migration row as live step state.
func (p *progressReporter) describe(ctx context.Context, m tracking.Migration) stepProgress {
	stepID := pipelinesvc.StepIDForCommand(m.Command)
	if stepID == "" {
		stepID = m.Command
	}
	detail, params := pipelinesvc.BuildProgressDetailAndParams(m.Command, m.Args)

	out := stepProgress{
		StepID:     stepID,
		StepLabel:  pipelinesvc.StepLabelForCommand(m.Command),
		Lane:       string(p.registry.LaneForCommand(m.Command)),
		Detail:     detail,
		Params:     params,
		StartedAt:  m.StartedAt.UTC().Format(time.RFC3339),
		ElapsedSec: int64(p.now().Sub(m.StartedAt).Seconds()),
	}

	switch m.Command {
	case "precompute-features":
		out.Precompute, out.EstimatedSec = p.precomputeDetail(m)
	case "ml-auto-tune":
		out.AutoTune = p.autoTuneProgress(ctx)
	case "dataset-fetch":
		out.Fetch, out.EstimatedSec = p.fetchDetail()
	case "dataset-extract":
		out.Extract, out.EstimatedSec = p.extractDetail()
	default:
		p.attachTrainingProgress(ctx, stepID, m, &out)
	}
	return out
}

// attachTrainingProgress folds a trainer's milestones into the step's live state.
//
// This is the whole point of O-3: one stream for the UI. A second endpoint for
// training progress would be a second thing to connect, reconnect and keep in sync,
// for a panel that is already listening here.
//
// Only steps the registry knows are asked about. A command from outside the registry
// has no step id to query with, and asking ml-service about "" on every tick would be
// a request per tick for an answer that cannot exist.
func (p *progressReporter) attachTrainingProgress(
	ctx context.Context,
	stepID string,
	m tracking.Migration,
	out *stepProgress,
) {
	step, known := p.registry.ByID(stepID)
	if !known || !step.RunsOnMLService() {
		return
	}

	progress, err := p.stepProgress(ctx, stepID)
	if err != nil {
		// Unreachable is reported as unknown, not as failure: the step itself may be
		// running perfectly well, and saying "failed" about a healthy run is worse
		// than saying "cannot tell".
		slog.Debug("pipeline progress: step progress unavailable",
			slog.String("step", stepID), slog.Any("err", err))
		out.ProgressUnavailable = true
		return
	}
	if len(progress) == 0 {
		return
	}
	out.Training = progress
	if eta := p.trainingETA(progress, m); eta != nil {
		out.EstimatedSec = eta
	}
}

// trainingETA estimates seconds remaining from the units the step has completed.
//
// Elapsed time comes from the migration row rather than the progress file: the file
// records when the last event was written, which says nothing about when the run
// began. Nothing is reported until at least one unit has finished — with zero
// completed there is no observed rate, and a number invented from a default is a
// number the operator has no reason to believe.
func (p *progressReporter) trainingETA(progress map[string]interface{}, m tracking.Migration) *int64 {
	current, okCurrent := numberFrom(progress["current"])
	total, okTotal := numberFrom(progress["total"])
	if !okCurrent || !okTotal || current <= 0 || total <= current {
		return nil
	}

	elapsed := p.now().Sub(m.StartedAt).Seconds()
	if elapsed <= 0 {
		return nil
	}
	remaining := int64((elapsed / current) * (total - current))
	if remaining <= 0 {
		return nil
	}
	return &remaining
}

// numberFrom reads a JSON number, which decodes as float64.
func numberFrom(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// fetchDetail reports bytes, rate and ETA for a download in flight.
//
// The download runs in this process, so unlike a training subprocess it can publish
// straight into memory — no progress file is needed until O-1 generalises one for the
// trainers. Absent state is reported as absent rather than as zero bytes: a fetch that
// has not written its first sample is not a fetch that has downloaded nothing.
func (p *progressReporter) fetchDetail() (*dataacquire.Progress, *int64) {
	progress, _, ok := p.fetchStatus()
	if !ok {
		return nil, nil
	}
	return &progress, progress.ETASec
}

// extractDetail reports entries, bytes and an ETA for an extraction in flight.
// Like fetchDetail, absent state is reported as absent rather than as zero entries.
func (p *progressReporter) extractDetail() (*dataacquire.ExtractProgress, *int64) {
	progress, _, ok := p.extractStatus()
	if !ok {
		return nil, nil
	}
	return &progress, progress.ETASec
}

// precomputeDetail reports per-format progress and an ETA for a precompute run.
func (p *progressReporter) precomputeDetail(m tracking.Migration) (*precomputeProgress, *int64) {
	status := p.precomputeStatus()
	index := indexOf(status.Formats, status.CurrentFormat)
	progress := &precomputeProgress{
		Formats:       status.Formats,
		CurrentFormat: status.CurrentFormat,
		Phase:         status.Phase,
		CurrentIndex:  index,
		FormatsTotal:  len(status.Formats),
	}
	return progress, p.precomputeETA(m, status, index)
}

// precomputeETA estimates the seconds remaining across the formats still to process.
//
// Until a format has finished there is nothing to measure, so the config default
// stands in; from the second format onwards the observed average per completed
// format replaces it. Returns nil when there is nothing worth claiming.
func (p *progressReporter) precomputeETA(
	m tracking.Migration,
	status precompute.Status,
	index int,
) *int64 {
	if index < 0 || len(status.Formats) == 0 {
		return nil
	}

	elapsedTotal := p.now().Sub(m.StartedAt).Seconds()
	elapsedCurrent := 0.0
	if !status.FormatStartedAt.IsZero() {
		elapsedCurrent = p.now().Sub(status.FormatStartedAt).Seconds()
	}

	secPerFormat := float64(pipelinesvc.DefaultPrecomputeETASecPerFormat())
	if index >= 1 && !status.FormatStartedAt.IsZero() {
		if completed := elapsedTotal - elapsedCurrent; completed > 0 {
			secPerFormat = max(completed/float64(index), 1)
		}
	}

	remainingCurrent := max(secPerFormat-elapsedCurrent, 0)
	remainingFormats := max(len(status.Formats)-index-1, 0)
	total := int64(remainingCurrent + float64(remainingFormats)*secPerFormat)
	if total <= 0 {
		return nil
	}
	return &total
}

// indexOf returns the position of value in list, or -1.
func indexOf(list []string, value string) int {
	for i, v := range list {
		if v == value {
			return i
		}
	}
	return -1
}
