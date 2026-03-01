package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "ok", body["status"])
}

func TestFormatAccuracyPct(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"float64", float64(85.5), "85.5%"},
		{"int", 90, "90%"},
		{"string", "95", "95%"},
		{"default", int32(77), "77%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAccuracyPct(tt.v)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAlgorithmDisplayName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"rf", "Random Forest"},
		{"gb", "Gradient Boosting"},
		{"RF", "Random Forest"},
		{"  stacked  ", "Stacking Regressor"},
		{"unknown", "unknown"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := algorithmDisplayName(tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}
