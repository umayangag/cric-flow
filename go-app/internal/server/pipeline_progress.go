package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// commandToStepID maps data_migrations command to pipeline step ID for the UI.
var commandToStepID = map[string]string{
	"cricsheet-import":       "import",
	"precompute-features":    "precompute",
	"export-dataset":         "export",
	"train-batting":          "train_batting",
	"train-bowling":          "train_bowling",
	"train-fielding":         "train_fielding",
	"train-extras":           "train_extras",
	"train-win":              "train_win",
	"train-combination-meta": "train_combination_meta",
}

var commandToStepLabel = map[string]string{
	"cricsheet-import":       "Import",
	"precompute-features":    "Precompute",
	"export-dataset":         "Export",
	"train-batting":          "Train Batting",
	"train-bowling":          "Train Bowling",
	"train-fielding":         "Train Fielding",
	"train-extras":           "Train Extras",
	"train-win":              "Train Win",
	"train-combination-meta": "Train Combination Meta",
}

const pipelineProgressInterval = 2 * time.Second

// defaultPrecomputeETASecPerFormat returns config precompute_eta_seconds_per_fmt, or 180 if unset (used only before any format completes).
func defaultPrecomputeETASecPerFormat() int {
	if cfg := config.Load(); cfg != nil && cfg.Pipeline.PrecomputeETASecondsPerFmt > 0 {
		return cfg.Pipeline.PrecomputeETASecondsPerFmt
	}
	return 180
}

// pipelineProgressPayload is the JSON sent in each SSE "progress" event.
type pipelineProgressPayload struct {
	Running      bool                `json:"running"`
	StepID       string              `json:"step_id,omitempty"`
	StepLabel    string              `json:"step_label,omitempty"`
	StartedAt    string              `json:"started_at,omitempty"`
	ElapsedSec   int64               `json:"elapsed_sec,omitempty"`
	Precompute   *precomputeProgress `json:"precompute,omitempty"`
	EstimatedSec *int64              `json:"estimated_remaining_sec,omitempty"`
}

type precomputeProgress struct {
	Formats       []string `json:"formats,omitempty"`
	CurrentFormat string   `json:"current_format,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	// Index of current format in Formats (0-based) for progress bar
	CurrentIndex int `json:"current_index,omitempty"`
	FormatsTotal int `json:"formats_total,omitempty"`
}

// pipelineProgressStreamHandler handles GET /ops/pipeline/stream and streams pipeline progress via SSE.
func (a *App) pipelineProgressStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	writeSSE := func(event, data string) bool {
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + data + "\n\n")); err != nil {
			log.Printf("pipeline progress stream: write failed (client may have disconnected): %v", err)
			return false
		}
		flusher.Flush()
		return true
	}

	ticker := time.NewTicker(pipelineProgressInterval)
	defer ticker.Stop()

	// Send initial event immediately
	sendProgress := func() bool {
		ctx := r.Context()
		inProgress, err := tracking.GetInProgressMigrations(ctx)
		if err != nil || len(inProgress) == 0 {
			payload := pipelineProgressPayload{Running: false}
			data, _ := json.Marshal(payload)
			return writeSSE("progress", string(data))
		}
		// Use the most recently started migration (first in list)
		m := inProgress[0]
		stepID := commandToStepID[m.Command]
		if stepID == "" {
			stepID = m.Command
		}
		stepLabel := commandToStepLabel[m.Command]
		if stepLabel == "" {
			stepLabel = m.Command
		}
		elapsed := time.Since(m.StartedAt).Seconds()
		payload := pipelineProgressPayload{
			Running:    true,
			StepID:     stepID,
			StepLabel:  stepLabel,
			StartedAt:  m.StartedAt.UTC().Format(time.RFC3339),
			ElapsedSec: int64(elapsed),
		}
		if m.Command == "precompute-features" {
			pc := precompute.GetStatus()
			idx := -1
			for i, f := range pc.Formats {
				if f == pc.CurrentFormat {
					idx = i
					break
				}
			}
			payload.Precompute = &precomputeProgress{
				Formats:       pc.Formats,
				CurrentFormat: pc.CurrentFormat,
				Phase:         pc.Phase,
				CurrentIndex:  idx,
				FormatsTotal:  len(pc.Formats),
			}
			// ETA: use observed time per format when available; else config default (fallback for first format).
			secPerFormat := defaultPrecomputeETASecPerFormat()
			elapsedTotalSec := time.Since(m.StartedAt).Seconds()
			elapsedCurrentFormatSec := 0.0
			if !pc.FormatStartedAt.IsZero() {
				elapsedCurrentFormatSec = time.Since(pc.FormatStartedAt).Seconds()
			}
			if idx >= 1 && !pc.FormatStartedAt.IsZero() {
				elapsedCompletedSec := elapsedTotalSec - elapsedCurrentFormatSec
				if elapsedCompletedSec > 0 {
					secPerFormat = int(elapsedCompletedSec / float64(idx))
					if secPerFormat < 1 {
						secPerFormat = 1
					}
				}
			}
			if idx >= 0 && len(pc.Formats) > 0 {
				remainingCurrentSec := 0.0
				if !pc.FormatStartedAt.IsZero() {
					r := float64(secPerFormat) - elapsedCurrentFormatSec
					if r > 0 {
						remainingCurrentSec = r
					}
				}
				remainingFormats := len(pc.Formats) - idx - 1
				if remainingFormats < 0 {
					remainingFormats = 0
				}
				totalRemainingSec := int64(remainingCurrentSec + float64(remainingFormats)*float64(secPerFormat))
				if totalRemainingSec > 0 {
					payload.EstimatedSec = ptrInt64(totalRemainingSec)
				}
			}
		}
		data, _ := json.Marshal(payload)
		return writeSSE("progress", string(data))
	}

	if !sendProgress() {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !sendProgress() {
				return
			}
		}
	}
}

func ptrInt64(n int64) *int64 { return &n }
