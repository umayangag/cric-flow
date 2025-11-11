package httpadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/mlclient/http"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

type assertFn func(t *testing.T, out mlclient.PredictResponse, err error)

func assertNoErrorPlayers(want []string) assertFn {
	return func(t *testing.T, out mlclient.PredictResponse, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if len(out.Players) != len(want) { t.Fatalf("want %d players got %d", len(want), len(out.Players)) }
		for i := range want {
			if out.Players[i] != want[i] { t.Fatalf("player %d: want %q got %q", i, want[i], out.Players[i]) }
		}
	}
}

func assertErr() assertFn { return func(t *testing.T, _ mlclient.PredictResponse, err error) { if err == nil { t.Fatalf("expected error") } } }

func TestHTTPClient_PredictTeam_and_Reload(t *testing.T) {
	t.Parallel()

	t.Run("predict-team happy path", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/predict-team", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost { t.Fatalf("want POST") }
			if ct := r.Header.Get("Content-Type"); ct != "application/json" { t.Fatalf("content-type: %s", ct) }
			var in mlclient.PredictRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(mlclient.PredictResponse{Players: []string{"A","B","C"}, Score: 0.9})
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := adapter.New(srv.URL)
		out, err := c.PredictTeam(context.Background(), mlclient.PredictRequest{MatchID:1,Format:"T20",Season:"2019"})
		assertNoErrorPlayers([]string{"A","B","C"})(t, out, err)
	})

	t.Run("predict-team non-2xx error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/predict-team", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		c := adapter.New(srv.URL)
		_, err := c.PredictTeam(context.Background(), mlclient.PredictRequest{})
		if err == nil { t.Fatalf("expected error for non-2xx status") }
	})

	t.Run("reload endpoint ok", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost { t.Fatalf("want POST") }
			w.WriteHeader(http.StatusOK)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		c := adapter.New(srv.URL)
		if err := c.Reload(context.Background()); err != nil { t.Fatalf("unexpected err: %v", err) }
	})
}
