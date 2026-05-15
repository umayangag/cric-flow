package mlclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPredictWin_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict-win" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"PlayerName":"A","WinningProbability":0.9}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	preds, err := c.PredictWin(ctx, nil)
	require.NoError(t, err)
	if len(preds) != 1 || preds[0].PlayerName != "A" || preds[0].WinningProbability != 0.9 {
		t.Fatalf("unexpected predictions: %#v", preds)
	}
}

func TestPredictWin_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	_, err := c.PredictWin(ctx, nil)
	require.Error(t, err)
}

func TestPredictWin_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.Client())
	ctx := context.Background()
	_, err := c.PredictWin(ctx, nil)
	require.Error(t, err)
}
