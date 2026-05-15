package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/stretchr/testify/require"
)

func TestOpsStatusHandler_ScaffoldShape(t *testing.T) {
	app := &App{mlClient: nil}
	req := httptest.NewRequest(http.MethodGet, "/ops/status", nil)
	rr := httptest.NewRecorder()

	app.opsStatusHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// Minimal shape assertions for scaffold
	if _, ok := body["timestamp"]; !ok {
		t.Fatalf("missing timestamp field")
	}
	if _, ok := body["services"]; !ok {
		t.Fatalf("missing services field")
	}
	if _, ok := body["db"]; !ok {
		t.Fatalf("missing db field")
	}
	if _, ok := body["precompute"]; !ok {
		t.Fatalf("missing precompute field")
	}
	if _, ok := body["exports"]; !ok {
		t.Fatalf("missing exports field")
	}
	if _, ok := body["artifacts"]; !ok {
		t.Fatalf("missing artifacts field")
	}
}
