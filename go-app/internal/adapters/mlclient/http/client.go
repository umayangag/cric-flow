package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// HTTPClient implements mlclient.Service by talking to the Python ML HTTP API.
// It is intentionally small and focused: request building, error handling, decode.
//
// Endpoints (conventional):
//   POST /predict-team   body: PredictRequest  -> 200 JSON PredictResponse
//   POST /reload         body: {}              -> 200 empty/JSON
//
// BaseURL should not have a trailing slash.
// HTTP must be non-nil; if nil, a default client with 20s timeout is used.
// UserAgent is optional.

type HTTPClient struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

func New(baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, in any, out any) error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("nil client or missing base url")
	}
	cli := c.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 20 * time.Second}
	}
	var body *bytes.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil { return err }
	if in != nil { req.Header.Set("Content-Type", "application/json") }
	if c.UserAgent != "" { req.Header.Set("User-Agent", c.UserAgent) }
	resp, err := cli.Do(req)
	if err != nil { return err }
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ml-service status: %s", resp.Status)
	}
	if out == nil { return nil }
	return json.NewDecoder(resp.Body).Decode(out)
}

// PredictTeam calls the ML service /predict-team endpoint.
func (c *HTTPClient) PredictTeam(ctx context.Context, in mlclient.PredictRequest) (mlclient.PredictResponse, error) {
	var out mlclient.PredictResponse
	if err := c.doJSON(ctx, http.MethodPost, "/predict-team", in, &out); err != nil {
		return mlclient.PredictResponse{}, err
	}
	return out, nil
}

// Reload triggers model reload if supported by the service.
func (c *HTTPClient) Reload(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/reload", struct{}{}, nil)
}
