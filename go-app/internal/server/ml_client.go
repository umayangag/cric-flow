package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// MLClient is the HTTP client for ml-service.
//
// After P-5 every call it makes is an XI-layer call -- /xi/optimize, /xi/predict-win,
// /simulate, /performance/predict and the L4 report -- so the whole surface is player ids
// and a format, never a feature map.
type MLClient struct {
	BaseURL string
	HTTP    *http.Client
}

// mlClientTimeout is generous because an as-of request can ask ml-service to replay the
// rating pass from the start of history before it answers.
const mlClientTimeout = 30 * time.Minute

// NewMLClient returns a client for the configured ml-service.
func NewMLClient() *MLClient {
	base := os.Getenv("ML_SERVICE_URL")
	if base == "" {
		base = "http://localhost:8000"
	}
	return &MLClient{BaseURL: base, HTTP: &http.Client{Timeout: mlClientTimeout}}
}

type mlErrorDetail struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Hint      string   `json:"hint"`
	Available []string `json:"available"`
}

// mlServiceError is a structured failure from ml-service, kept structured all the way
// to the HTTP response (consumer plan W1-2).
//
// Before this type the chain was: ml-service writes {code, message, hint, available};
// this client formats it into "endpoint http 404: msg — hint", dropping code and
// available; respondErr wraps that string in a 500 INTERNAL. A 404 the operator could
// have acted on reached the browser as an unexplained server error. Every link in that
// chain lost something, so the fix has to be a type that survives all of them.
type mlServiceError struct {
	Endpoint  string
	Status    int
	Code      string
	Message   string
	Hint      string
	Available []string
}

func (e *mlServiceError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "ml-service returned an error"
	}
	if e.Hint != "" {
		msg += " — " + e.Hint
	}
	return fmt.Sprintf("%s http %d: %s", e.Endpoint, e.Status, msg)
}

// logMLNon2xx reads the response body, logs status and body for debugging, and returns an error
// that includes status and a short message extracted from the body if present.
func logMLNon2xx(resp *http.Response, endpoint string) error {
	const maxResponseBody = 1 << 20 // 1 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		slog.Error("ml service non-2xx: failed to read response body",
			slog.String("endpoint", endpoint),
			slog.Int("status", resp.StatusCode),
			slog.Any("err", err))
		return fmt.Errorf("%s http %d (body read failed: %v)", endpoint, resp.StatusCode, err)
	}
	bodyStr := string(body)
	const maxLog = 2000
	if len(bodyStr) > maxLog {
		bodyStr = bodyStr[:maxLog] + "..."
	}
	slog.Error("ml service returned non-2xx",
		slog.String("endpoint", endpoint),
		slog.Int("status", resp.StatusCode),
		slog.String("body", bodyStr))

	// Try to extract a short message from FastAPI-style {"detail": "..."} or {"detail": {"code","message","hint"}}
	var detail struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &detail); err == nil && len(detail.Detail) > 0 {
		var s string
		if err := json.Unmarshal(detail.Detail, &s); err == nil {
			return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, s)
		}
		var d mlErrorDetail
		if err := json.Unmarshal(detail.Detail, &d); err == nil {
			return &mlServiceError{
				Endpoint:  endpoint,
				Status:    resp.StatusCode,
				Code:      d.Code,
				Message:   d.Message,
				Hint:      d.Hint,
				Available: d.Available,
			}
		}
		// Neither shape matched -- this is FastAPI's own default for a pydantic
		// validation failure this service never overrode: {"detail": [{"loc",
		// "msg", "type"}, ...]}, an array, which fails to unmarshal into either
		// struct above. Before this branch that left `err` from the attempt just
		// above as the only signal, which the code below discards, so a 422 the
		// caller could have acted on (a bad `team_size`, a repeated pool id) fell
		// through to respondErr's generic 500 INTERNAL (GO-12). A 4xx here is
		// always the caller's fault, whatever shape it arrived in, so it is
		// relayed as a structured error rather than swallowed.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return &mlServiceError{
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     "VALIDATION_ERROR",
				Message:  describeUnstructuredDetail(detail.Detail),
			}
		}
	}
	return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(bodyStr))
}

// fastAPIValidationIssue is one entry of FastAPI's default RequestValidationError body:
// no "code", so describeUnstructuredDetail is what turns it into something a caller can
// read rather than a raw JSON array.
type fastAPIValidationIssue struct {
	Loc  []any  `json:"loc"`
	Msg  string `json:"msg"`
	Type string `json:"type"`
}

// describeUnstructuredDetail renders a 4xx body's "detail" as a message, for the shapes
// that are not {"detail": "..."} or {"detail": {"code", "message", "hint"}}. It reads
// FastAPI's own validation-issue array first, because "team_size: value is not a valid
// integer" is more useful than the raw JSON it was built from; anything else falls back to
// the raw detail so no information here is ever dropped, only reformatted.
func describeUnstructuredDetail(raw json.RawMessage) string {
	var issues []fastAPIValidationIssue
	if err := json.Unmarshal(raw, &issues); err == nil && len(issues) > 0 {
		parts := make([]string, 0, len(issues))
		for _, issue := range issues {
			locParts := make([]string, 0, len(issue.Loc))
			for _, p := range issue.Loc {
				locParts = append(locParts, fmt.Sprintf("%v", p))
			}
			loc := strings.Join(locParts, ".")
			if loc == "" {
				parts = append(parts, issue.Msg)
				continue
			}
			parts = append(parts, loc+": "+issue.Msg)
		}
		return strings.Join(parts, "; ")
	}
	return strings.TrimSpace(string(raw))
}
