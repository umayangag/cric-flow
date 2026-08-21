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
	require.Contains(t, body, "precompute")
	require.Contains(t, body, "exports")
	require.Contains(t, body, "artifacts")
}
