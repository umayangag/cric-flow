package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectRemovedParams(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		rawURL     string
		wantStatus int
		wantCode   string
	}{
		{"no removed params passes through", "/api/backtest?match_id=1&format=ODI", http.StatusOK, ""},
		{
			"use_unified_model=1 is refused",
			"/api/backtest?match_id=1&use_unified_model=1",
			http.StatusBadRequest,
			"UNIFIED_MODEL_REMOVED",
		},
		{
			"use_unified_model=0 is refused too",
			"/api/backtest?match_id=1&use_unified_model=0",
			http.StatusBadRequest,
			"UNIFIED_MODEL_REMOVED",
		},
		{
			"model=unified alias is refused",
			"/api/backtest?match_id=1&model=unified",
			http.StatusBadRequest,
			"UNIFIED_MODEL_REMOVED",
		},
		{
			"auto_tune model values are untouched",
			"/ops/pipeline/run/auto_tune?model=batting",
			http.StatusOK,
			"",
		},
		{
			"auto_tune unified training flag is untouched",
			"/ops/pipeline/run/auto_tune?model=all&unified=1",
			http.StatusOK,
			"",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reached := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			rejectRemovedParams(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.rawURL, nil))

			require.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantCode == "" {
				assert.True(t, reached, "request should have reached the handler")
				return
			}
			assert.False(t, reached, "request should not have reached the handler")

			var body apiError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.wantCode, body.Code)
			assert.NotEmpty(t, body.Message, "a refusal must say what was removed")
			assert.NotEmpty(t, body.Hint, "a refusal must say what to do instead")
		})
	}
}
