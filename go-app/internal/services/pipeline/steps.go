// Package pipeline provides pipeline domain operations (stop run, step mapping, ML service calls, etc.)
// so HTTP handlers can stay thin and delegate to this package.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// StepToCommand returns the CLI command string for a given pipeline step ID.
func StepToCommand(step string) string {
	switch step {
	case "train_batting":
		return "make train-batting CUTOFF=2025-01-01T00:00:00Z"
	case "train_bowling":
		return "make train-bowling CUTOFF=2025-01-01T00:00:00Z"
	case "train_fielding":
		return "make train-fielding CUTOFF=2025-01-01T00:00:00Z"
	case "train_extras":
		return "make train-extras CUTOFF=2025-01-01T00:00:00Z"
	case "train_win":
		return "make train-win CUTOFF=2025-01-01T00:00:00Z"
	case "train_innings":
		return "make train-innings CUTOFF=2025-01-01T00:00:00Z"
	case "train_combination_meta":
		return "make train-combination-meta CSV=<export_dir>/backtest_contributions.csv OUT=<export_dir>/combination_meta.json"
	case "auto_tune":
		return "make ml-auto-tune MODEL=all ALL_FORMATS=1"
	default:
		return ""
	}
}

// TrainingStepToModel maps a pipeline train step ID to the ML model name for tuned-params lookup.
func TrainingStepToModel(stepID string) string {
	switch stepID {
	case "train_batting":
		return "batting"
	case "train_bowling":
		return "bowling"
	case "train_fielding":
		return "fielding"
	case "train_extras":
		return "extras"
	case "train_innings":
		return "innings"
	case "train_win":
		return "win"
	default:
		return ""
	}
}

// MLServiceBaseURL returns the ML service base URL from env or config fallback.
// The returned URL never has a trailing slash.
func MLServiceBaseURL() string {
	s := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return config.ServerMLBaseURLFallback(config.Load())
}

// TrainStepTimeout returns the timeout duration for a training step from config.
func TrainStepTimeout() time.Duration {
	mins := config.ServerTrainStepTimeoutMin(config.Load())
	return time.Duration(mins) * time.Minute
}

// DefaultCutoff returns the current UTC time formatted as RFC3339, used as the default cutoff for training steps.
func DefaultCutoff() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CallMLTrainEndpoint POSTs to ML service /admin/train/{step} and returns an error on non-2xx or context cancel.
// When ml-service ADMIN_API_KEY is set, sends X-API-Key header.
func CallMLTrainEndpoint(ctx context.Context, step string, querySuffix string) error {
	base := MLServiceBaseURL()
	url := base + "/admin/train/" + step + querySuffix
	slog.Info("pipeline: calling ML service train endpoint",
		slog.String("step", step),
		slog.String("url", url))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if key := strings.TrimSpace(os.Getenv("ML_SERVICE_ADMIN_API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
	} else if key := strings.TrimSpace(os.Getenv("API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	client := &http.Client{Timeout: TrainStepTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := string(body)
		if msg != "" {
			return fmt.Errorf("ml-service %s: %s — %s", url, resp.Status, msg)
		}
		return fmt.Errorf("ml-service %s: %s", url, resp.Status)
	}
	return nil
}

// ProgressInterval returns the SSE polling interval for pipeline progress from config.
func ProgressInterval() time.Duration {
	sec := config.ServerPipelineProgressSec(config.Load())
	return time.Duration(sec) * time.Second
}

// DefaultPrecomputeETASecPerFormat returns the config precompute_eta_seconds_per_fmt (used for ETA before any format completes).
func DefaultPrecomputeETASecPerFormat() int {
	return config.PipelinePrecomputeETASecondsPerFmt(config.Load())
}

// CommandToStepID maps data_migrations command to pipeline step ID for the UI.
var CommandToStepID = map[string]string{
	"cricsheet-import":       "import",
	"precompute-features":    "precompute",
	"export-dataset":         "export",
	"train-batting":          "train_batting",
	"train-bowling":          "train_bowling",
	"train-fielding":         "train_fielding",
	"train-extras":           "train_extras",
	"train-win":              "train_win",
	"train-innings":          "train_innings",
	"train-combination-meta": "train_combination_meta",
	"ml-auto-tune":           "auto_tune",
}

// CommandToStepLabel maps data_migrations command to a human-readable label.
var CommandToStepLabel = map[string]string{
	"cricsheet-import":       "Import",
	"precompute-features":    "Precompute",
	"export-dataset":         "Export",
	"train-batting":          "Train Batting",
	"train-bowling":          "Train Bowling",
	"train-fielding":         "Train Fielding",
	"train-extras":           "Train Extras",
	"train-win":              "Train Win",
	"train-innings":          "Train Innings",
	"train-combination-meta": "Train Combination Meta",
	"ml-auto-tune":           "Auto-tune",
}

// FetchAutoTuneProgress fetches live auto-tune progress from the ML service. Returns nil on error.
func FetchAutoTuneProgress(ctx context.Context) map[string]interface{} {
	base := MLServiceBaseURL()
	url := base + "/admin/train/auto-tune/progress"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	if key := strings.TrimSpace(os.Getenv("ML_SERVICE_ADMIN_API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
	} else if key := strings.TrimSpace(os.Getenv("API_KEY")); key != "" {
		req.Header.Set("X-API-Key", key)
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

// BuildProgressDetailAndParams returns a short human-readable detail string and a params map from migration command and args.
func BuildProgressDetailAndParams(
	command string,
	argsJSON json.RawMessage,
) (detail string, params map[string]interface{}) {
	params = make(map[string]interface{})
	if len(argsJSON) > 0 {
		_ = json.Unmarshal(argsJSON, &params)
	}
	// Omit internal "step" from params for display.
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
