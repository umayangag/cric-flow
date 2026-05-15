package mlclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	"github.com/umayangag/cric-flow/go-app/internal/models"
)

// newTestClient creates a Client pointing at the given base URL.
func newTestClient(base string, httpClient *http.Client) *mlclient.Client {
	return &mlclient.Client{
		BaseURL:   base,
		HTTP:      httpClient,
		UserAgent: "mlclient-test/1.0",
		Timeout:   200 * time.Millisecond,
	}
}

type badJSON struct{}

func (badJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("marshal boom") }

// New() configuration tests (env default/override)
func TestClient_New(t *testing.T) {
	testCases := []struct {
		name      string
		setEnv    bool
		envVal    string
		expectURL string
	}{
		{"defaults when env unset", false, "", "http://localhost:8000"},
		{"respects ML_BASE_URL env", true, "http://example.test:1234", "http://example.test:1234"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				os.Setenv("ML_BASE_URL", tc.envVal)
				t.Cleanup(func() { _ = os.Unsetenv("ML_BASE_URL") })
			} else {
				_ = os.Unsetenv("ML_BASE_URL")
			}
			c := mlclient.New()
			require.Equal(t, tc.expectURL, c.BaseURL)
			require.NotNil(t, c.HTTP)
			require.Greater(t, c.Timeout, time.Duration(0))
			require.Greater(t, c.HTTP.Timeout, time.Duration(0))
			require.NotEmpty(t, c.UserAgent)
		})
	}
}

// postJSON behavior tested through PredictBatting (public API)
func TestClient_PostJSON_SuccessAndUserAgent(t *testing.T) {
	wantUA := "mlclient-test/1.0"
	var seenUA, seenCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		seenCT = r.Header.Get("Content-Type")
		seenUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]models.BattingPrediction{{RunsScored: 42}})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	preds, err := c.PredictBatting(context.Background(), []models.BattingFeatures{{}})
	require.NoError(t, err)
	require.Len(t, preds, 1)
	require.Equal(t, float32(42), preds[0].RunsScored)
	require.Equal(t, "application/json", seenCT)
	require.Equal(t, wantUA, seenUA)
}

func TestClient_PostJSON_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	_, err := c.PredictBatting(context.Background(), []models.BattingFeatures{{}})
	require.Error(t, err)
}

func TestClient_PostJSON_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	_, err := c.PredictBatting(context.Background(), []models.BattingFeatures{{}})
	require.Error(t, err)
}

func TestClient_PostJSON_ContextCanceled(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-done
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.PredictBatting(ctx, []models.BattingFeatures{{}})
	close(done)
	require.Error(t, err)
}

// PredictBatting behavior grouped
func TestClient_PredictBatting(t *testing.T) {
	testCases := []struct {
		name       string
		serverFunc http.HandlerFunc
		expectErr  bool
		expectRuns float32
	}{
		{
			"success",
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/predict/batting", r.URL.Path)
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				_ = json.NewEncoder(w).Encode([]models.BattingPrediction{{RunsScored: 42}})
			}),
			false,
			42,
		},
		{
			"non-200",
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`{"error":"nope"}`))
			}),
			true,
			0,
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.serverFunc)
			defer srv.Close()
			c := newTestClient(srv.URL, srv.Client())
			preds, err := c.PredictBatting(context.Background(), []models.BattingFeatures{{}})
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, preds, 1)
			require.Equal(t, tc.expectRuns, preds[0].RunsScored)
		})
	}
}

// PredictBowling grouped
func TestClient_PredictBowling(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/predict/bowling", r.URL.Path)
			_ = json.NewEncoder(w).Encode([]models.BowlingPrediction{{WicketsTaken: 3}})
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		preds, err := c.PredictBowling(context.Background(), []models.BowlingFeatures{{}})
		require.NoError(t, err)
		require.Len(t, preds, 1)
		require.Equal(t, float32(3), preds[0].WicketsTaken)
	})

	t.Run("bad JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		_, err := c.PredictBowling(context.Background(), []models.BowlingFeatures{{}})
		require.Error(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		hc := srv.Client()
		hc.Timeout = 50 * time.Millisecond
		c := newTestClient(srv.URL, hc)
		_, err := c.PredictBowling(context.Background(), []models.BowlingFeatures{{}})
		require.Error(t, err)
	})

	t.Run("user-agent absent uses Go default", func(t *testing.T) {
		var sawUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawUA = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		c := newTestClient(srv.URL, srv.Client())
		c.UserAgent = "" // explicitly clear
		_, err := c.PredictBowling(context.Background(), []models.BowlingFeatures{{}})
		require.NoError(t, err)
		require.Equal(t, "Go-http-client/1.1", sawUA)
	})
}
