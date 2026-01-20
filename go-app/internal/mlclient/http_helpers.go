package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultClient returns an *http.Client with the provided timeout.
func DefaultClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// newRequest builds an HTTP request with JSON body and common headers.
// If payload is non-nil, it will be JSON-encoded as the request body.
func newRequest(ctx context.Context, method, url string, payload any, userAgent string) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	return req, nil
}

// doJSON executes the request and decodes a successful JSON response into out.
// It preserves the error style used elsewhere in the package (status text on non-2xx).
func doJSON(client *http.Client, req *http.Request, out any) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	// Ensure body is closed by caller for flexibility (some callers may inspect it).
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain and close to aid connection reuse.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp, fmt.Errorf("ml-service status: %s", resp.Status)
	}
	decErr := json.NewDecoder(resp.Body).Decode(out)
	// Close after decode to free the connection.
	_ = resp.Body.Close()
	if decErr != nil {
		return resp, decErr
	}
	return resp, nil
}
