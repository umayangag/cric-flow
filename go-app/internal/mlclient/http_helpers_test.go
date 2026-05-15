package mlclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
)

func TestDefaultClient_Timeout(t *testing.T) {
	// Server sleeps longer than client timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c := &mlclient.Client{
		BaseURL: srv.URL,
		HTTP:    mlclient.DefaultClient(50 * time.Millisecond),
		Timeout: 50 * time.Millisecond,
	}
	_, err := c.PredictBatting(context.Background(), nil)
	require.Error(t, err)
	// Accept either client timeout or context deadline exceeded wording
	require.True(t,
		strings.Contains(err.Error(), "Client.Timeout") ||
			errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), "timeout"),
		"expected timeout-related error, got: %v", err)
}

func TestClient_PredictBatting_Non2xx(t *testing.T) {
	testCases := []struct {
		name   string
		status int
	}{
		{name: "400 bad request", status: http.StatusBadRequest},
		{name: "500 internal server error", status: http.StatusInternalServerError},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("oops"))
			}))
			defer srv.Close()

			c := &mlclient.Client{
				BaseURL:   srv.URL,
				HTTP:      mlclient.DefaultClient(2 * time.Second),
				UserAgent: "test-agent",
			}
			_, err := c.PredictBatting(context.Background(), nil)
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), "ml-service status:"),
				"error should start with 'ml-service status:', got %v", err)
		})
	}
}

func TestClient_PredictBatting_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid-json"))
	}))
	defer srv.Close()

	c := &mlclient.Client{
		BaseURL:   srv.URL,
		HTTP:      mlclient.DefaultClient(2 * time.Second),
		UserAgent: "test-agent",
	}
	_, err := c.PredictBatting(context.Background(), nil)
	require.Error(t, err)
}

func TestClient_RequestHeaders(t *testing.T) {
	var gotContentType, gotUserAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := &mlclient.Client{
		BaseURL:   srv.URL,
		HTTP:      mlclient.DefaultClient(2 * time.Second),
		UserAgent: "ua-1",
	}
	_, _ = c.PredictBatting(context.Background(), nil)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "ua-1", gotUserAgent)
}

func TestClient_RequestHeaders_NoUserAgent(t *testing.T) {
	var gotUserAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := &mlclient.Client{
		BaseURL: srv.URL,
		HTTP:    mlclient.DefaultClient(2 * time.Second),
	}
	_, _ = c.PredictBatting(context.Background(), nil)
	// Go's http.Client sets a default User-Agent if empty, so just verify our custom one isn't set
	require.NotEqual(t, "ua-1", gotUserAgent)
}
