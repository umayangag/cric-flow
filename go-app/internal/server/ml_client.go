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
	}
	return fmt.Errorf("%s http %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(bodyStr))
}

