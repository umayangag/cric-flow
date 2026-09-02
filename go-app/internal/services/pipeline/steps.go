// Package pipeline provides pipeline domain operations (stop run, step mapping, ML service calls, etc.)
// so HTTP handlers can stay thin and delegate to this package.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// MLServiceBaseURL returns the ML service base URL from env or config fallback.
// The returned URL never has a trailing slash.
func MLServiceBaseURL() string {
	s := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return config.ServerMLBaseURLFallback(config.Load())
}

// AdminAPIKey is the key ml-service's admin endpoints are called with. ML_SERVICE_ADMIN_API_KEY
// names it explicitly; API_KEY is the single-key local setup. Empty means ml-service is not
// protecting its admin surface, which is the docker-compose default.
func AdminAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("ML_SERVICE_ADMIN_API_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("API_KEY"))
}

// TrainStepTimeout returns the timeout duration for a training step from config.
func TrainStepTimeout() time.Duration {
	mins := config.ServerTrainStepTimeoutMin(config.Load())
	return time.Duration(mins) * time.Minute
}

// CallMLTrainEndpoint POSTs to ML service /admin/train/{step} and returns an error on non-2xx or context cancel.
// When ml-service ADMIN_API_KEY is set, sends X-API-Key header.
func CallMLTrainEndpoint(ctx context.Context, step string, querySuffix string) error {
	_, err := CallMLTrainEndpointWithResult(ctx, step, querySuffix)
	return err
}

// mlSuccessBodyLimit bounds the success body we will read. A run summary is a few
// kilobytes; anything far larger is a bug on the other side, and reading it into a
// migration row would be storing that bug.
const mlSuccessBodyLimit = 1 << 20

// TrainResult is what a completed training step reports about itself.
type TrainResult struct {
	Status string `json:"status"`
	Step   string `json:"step"`
	// Summary is the run's terminal state as the trainer recorded it: formats
	// trained, rows, metrics, dropped columns, artifacts. Absent for a step that
	// publishes no progress.
	Summary map[string]interface{} `json:"summary,omitempty"`
}

// CallMLTrainEndpointWithResult runs a training step and returns what it reported.
//
// The summary comes back on the response rather than being polled for afterwards,
// because there is no afterwards to poll in: the progress file is removed as the run
// ends (a file left behind reads as a run still going), so the response is the only
// moment the outcome is still available.
func CallMLTrainEndpointWithResult(ctx context.Context, step string, querySuffix string) (*TrainResult, error) {
	base := MLServiceBaseURL()
	url := base + MLTrainPathPrefix + step + querySuffix
	slog.Info("pipeline: calling ML service train endpoint",
		slog.String("step", step),
		slog.String("url", url))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	if key := AdminAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	client := &http.Client{Timeout: TrainStepTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, mlErrorBodyLimit))
		return nil, newMLError(url, resp.StatusCode, resp.Status, body)
	}

	// The step succeeded. An unreadable body loses the summary, not the run, so it
	// is logged rather than turned into a failure the operator has to interpret.
	body, err := io.ReadAll(io.LimitReader(resp.Body, mlSuccessBodyLimit))
	if err != nil {
		slog.Warn("pipeline: could not read train response body", slog.String("step", step), slog.Any("err", err))
		return nil, nil
	}
	var result TrainResult
	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("pipeline: unreadable train response body", slog.String("step", step), slog.Any("err", err))
		return nil, nil
	}
	return &result, nil
}

// ProgressInterval returns the SSE polling interval for pipeline progress from config.
func ProgressInterval() time.Duration {
	sec := config.ServerPipelineProgressSec(config.Load())
	return time.Duration(sec) * time.Second
}

// ProgressFetchTimeout bounds one poll of ml-service for step progress.
//
// It is short on purpose: this runs once per SSE tick, and a slow ml-service must
// degrade the progress panel to "unknown" rather than stall the whole stream behind it.
func ProgressFetchTimeout() time.Duration { return 5 * time.Second }

// StepIDForCommand maps a data_migrations command to the pipeline step ID the UI
// knows it by. Returns "" for a command written by something outside the registry.
func StepIDForCommand(command string) string {
	step, ok := Steps().ByCommand(command)
	if !ok {
		return ""
	}
	return step.ID
}

// StepLabelForCommand maps a data_migrations command to a human-readable label,
// falling back to the command itself when the registry does not know it.
func StepLabelForCommand(command string) string {
	return Steps().LabelForCommand(command)
}

// ErrProgressUnavailable reports that ml-service could not be asked, as distinct from
// it answering "nothing is running".
//
// The difference matters to the operator and nothing else can recover it. A step that
// has published no progress yet and a step whose progress cannot be reached both look
// like an empty panel, but one is a run about to report and the other is a broken
// link — and the second is worth saying out loud rather than rendering as silence.
var ErrProgressUnavailable = errors.New("ml-service progress is unreachable")

// FetchStepProgress fetches live progress for a pipeline step from ml-service.
//
// It returns (nil, nil) when the step is running but has published nothing yet, and
// (nil, ErrProgressUnavailable) when ml-service could not be reached or refused.
func FetchStepProgress(ctx context.Context, stepID string) (map[string]interface{}, error) {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil, nil
	}

	endpoint := MLServiceBaseURL() + MLProgressPath + "?" + MLQueryStep + "=" + url.QueryEscape(stepID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProgressUnavailable, err)
	}
	if key := AdminAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}

	client := &http.Client{Timeout: ProgressFetchTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProgressUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: ml-service returned %s", ErrProgressUnavailable, resp.Status)
	}
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProgressUnavailable, err)
	}
	if len(m) == 0 {
		// A reachable service saying "nothing yet" is not an error.
		return nil, nil
	}
	return m, nil
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
	case "xi-retrain":
		if cutoff, _ := params["cutoff"].(string); cutoff != "" {
			detail = fmt.Sprintf("Rating pass, XI win models, performance models and L4 report (cutoff %s)", cutoff)
		} else {
			detail = "Rating pass, XI win models, performance models and L4 report"
		}
	case "xi-evaluate":
		if cutoff, _ := params["cutoff"].(string); cutoff != "" {
			detail = fmt.Sprintf("Evaluating at cutoff %s (leaves `current` alone)", cutoff)
		} else {
			detail = "Evaluating (leaves `current` alone)"
		}
	case "xi-reload":
		if run, _ := params["run_id"].(string); run != "" {
			detail = fmt.Sprintf("Pointing `current` at run %s and loading it", run)
		} else {
			detail = "Reloading the run `current` points at"
		}
	default:
		detail = command
	}
	return detail, params
}

// mlErrorBodyLimit caps how much of an ml-service error body is read. The bodies
// we care about are a few hundred bytes of JSON; anything larger is a stack trace
// we do not want in data_migrations.error_message.
const mlErrorBodyLimit = 4 << 10

// MLError is a failure reported by ml-service. ml-service answers preconditions
// with a structured body — {"detail": {"code", "message", "hint"}} — precisely so
// the operator can be told what to do next (C5-2's CONTRIBUTIONS_CSV_MISSING is the
// canonical example). Keeping that structure instead of flattening it to a string
// is what lets the run-history row say "run export-contributions first" rather than
// "ml-service returned 400".
type MLError struct {
	// URL is the ml-service endpoint that failed.
	URL string
	// StatusCode is the HTTP status returned by ml-service.
	StatusCode int
	// Code is the machine-readable error code, when ml-service supplied one.
	Code string
	// Message is the human-readable explanation, when ml-service supplied one.
	Message string
	// Hint is the next action the operator should take, when ml-service supplied one.
	Hint string
	// Body is the raw response body, kept for errors that carry no structure.
	Body string

	// rawStatus is the status line as ml-service phrased it ("400 Bad Request").
	rawStatus string
}

// IsPrecondition reports whether the failure is the caller's to fix — a missing
// input or a bad argument — rather than a fault inside ml-service.
func (e *MLError) IsPrecondition() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// Error renders the failure with the actionable parts first, because this string is
// what lands in data_migrations.error_message and is what the operator reads.
func (e *MLError) Error() string {
	switch {
	case e.Code != "" && e.Hint != "":
		return fmt.Sprintf("%s: %s — %s", e.Code, e.Message, e.Hint)
	case e.Code != "":
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	case e.Body != "":
		return fmt.Sprintf("ml-service %s: %s — %s", e.URL, e.status(), e.Body)
	default:
		return fmt.Sprintf("ml-service %s: %s", e.URL, e.status())
	}
}

func (e *MLError) status() string {
	if e.rawStatus != "" {
		return e.rawStatus
	}
	return http.StatusText(e.StatusCode)
}

// newMLError parses an ml-service error body into an MLError, falling back to the
// raw body when the response is not the structured shape.
func newMLError(url string, statusCode int, status string, body []byte) *MLError {
	e := &MLError{
		URL:        url,
		StatusCode: statusCode,
		rawStatus:  status,
		Body:       strings.TrimSpace(string(body)),
	}
	var envelope struct {
		Detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		e.Code = envelope.Detail.Code
		e.Message = envelope.Detail.Message
		e.Hint = envelope.Detail.Hint
	}
	return e
}
