package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

func mlServiceBaseURLForProgress() string {
	s := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return config.ServerMLBaseURLFallback(config.Load())
}

func fetchAutoTuneProgress(ctx context.Context) map[string]interface{} {
	base := mlServiceBaseURLForProgress()
	url := base + "/admin/train/auto-tune/progress"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	var m map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return nil
	}
	return m
}

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
	"ml-auto-tune":           "auto_tune",
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
	"ml-auto-tune":           "Auto-tune",
}

func pipelineProgressInterval() time.Duration {
	sec := config.ServerPipelineProgressSec(config.Load())
	return time.Duration(sec) * time.Second
}

// defaultPrecomputeETASecPerFormat returns config precompute_eta_seconds_per_fmt, or 180 if unset (used only before any format completes).
func defaultPrecomputeETASecPerFormat() int {
	return config.PipelinePrecomputeETASecondsPerFmt(config.Load())
}

// pipelineProgressPayload is the JSON sent in each SSE "progress" event.
type pipelineProgressPayload struct {
	Running      bool                   `json:"running"`
	StepID       string                 `json:"step_id,omitempty"`
	StepLabel    string                 `json:"step_label,omitempty"`
	Detail       string                 `json:"detail,omitempty"` // Human-readable: what is happening
	Params       map[string]interface{} `json:"params,omitempty"` // Current parameters (from migration args)
	StartedAt    string                 `json:"started_at,omitempty"`
	ElapsedSec   int64                  `json:"elapsed_sec,omitempty"`
	Precompute   *precomputeProgress    `json:"precompute,omitempty"`
	EstimatedSec *int64                 `json:"estimated_remaining_sec,omitempty"`
	AutoTune     map[string]interface{} `json:"auto_tune,omitempty"` // Live auto-tune progress (phase, algorithm, hyperparams, trial, etc.)
}

type precomputeProgress struct {
	Formats       []string `json:"formats,omitempty"`
	CurrentFormat string   `json:"current_format,omitempty"`
	Phase         string   `json:"phase,omitempty"`
	// Index of current format in Formats (0-based) for progress bar
	CurrentIndex int `json:"current_index,omitempty"`
	FormatsTotal int `json:"formats_total,omitempty"`
}

// buildProgressDetailAndParams returns a short human-readable detail string and a params map from migration command and args.
func buildProgressDetailAndParams(
	command string,
	argsJSON json.RawMessage,
) (detail string, params map[string]interface{}) {
	params = make(map[string]interface{})
	if len(argsJSON) > 0 {
		_ = json.Unmarshal(argsJSON, &params)
	}
	// Omit internal "step" from params for display
	delete(params, "step")

	switch command {
	case "cricsheet-import":
		if dir, _ := params["dir"].(string); dir != "" {
			detail = fmt.Sprintf("Importing Cricsheet from %s", dir)
		} else {
			detail = "Importing Cricsheet data"
		}
	case "precompute-features":
		detail = "Computing features and consistency per format"
		if season, _ := params["season"].(string); season != "" {
			params["season"] = season
		}
	case "export-dataset":
		if outDir, _ := params["out_dir"].(string); outDir != "" {
			detail = fmt.Sprintf("Exporting training dataset to %s", outDir)
		} else {
			detail = "Exporting training dataset"
		}
	case "train-batting", "train-bowling", "train-fielding", "train-extras", "train-win":
		model := strings.TrimPrefix(command, "train-")
		if len(model) > 0 {
			model = strings.ToUpper(model[:1]) + model[1:]
		}
		if cutoff, _ := params["cutoff"].(string); cutoff != "" {
			detail = fmt.Sprintf("Training %s models (cutoff %s)", model, cutoff)
		} else {
			detail = fmt.Sprintf("Training %s models", model)
		}
	case "ml-auto-tune":
		model, _ := params["model"].(string)
		if model == "" {
			model = "all"
		}
		format, _ := params["format"].(string)
		allFormats, _ := params["all_formats"].(string)
		switch {
		case allFormats != "":
			detail = fmt.Sprintf(
				"Auto-tuning: model %s, all formats (searching best algorithm and hyperparameters)",
				model,
			)
		case format != "":
			detail = fmt.Sprintf(
				"Auto-tuning: model %s, format %s (searching best algorithm and hyperparameters)",
				model,
				format,
			)
		default:
			detail = fmt.Sprintf("Auto-tuning: model %s (searching best algorithm and hyperparameters)", model)
		}
		params["model"] = model
		if format != "" {
			params["format"] = format
		}
		if cutoff, _ := params["cutoff"].(string); cutoff != "" {
			params["cutoff"] = cutoff
		}
	case "train-combination-meta":
		detail = "Training combination meta-model"
	default:
		detail = command
	}
	return detail, params
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

	ticker := time.NewTicker(pipelineProgressInterval())
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
		detail, params := buildProgressDetailAndParams(m.Command, m.Args)
		payload := pipelineProgressPayload{
			Running:    true,
			StepID:     stepID,
			StepLabel:  stepLabel,
			Detail:     detail,
			Params:     params,
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
		if m.Command == "ml-auto-tune" {
			payload.AutoTune = fetchAutoTuneProgress(r.Context())
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
