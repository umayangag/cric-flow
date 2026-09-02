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
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	// Minimal shape assertions for scaffold
	require.Contains(t, body, "timestamp")
	require.Contains(t, body, "services")
	require.Contains(t, body, "db")
	require.Contains(t, body, "artifacts")
	require.Contains(t, body, "pipeline")
	require.NotContains(t, body, "precompute", "the precompute step and its section are gone")
	require.NotContains(t, body, "exports", "there is no export step to report on")
	require.NotContains(t, body, "weather", "nothing has ever populated weather_data")
}
