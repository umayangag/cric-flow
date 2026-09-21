package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneStoreMLService answers /xi/status, /artifacts/status and /health from one rating
// state, exactly as ml-service does: `XiRegistry.freshness()` computes the verdict once
// and every endpoint reports that object.
func oneStoreMLService(t *testing.T, verdict map[string]any, runID string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "ratings": verdict})
		case "/xi/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"loaded": true, "formats": []string{"T20"}, "players": 1234,
				"ratings_through": verdict["ratings_through"], "run_id": runID,
				"ratings": verdict,
			})
		case "/artifacts/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"root": "/models", "reachable": true,
				"current_run": runID, "loaded_run": runID,
				"ratings_through": verdict["ratings_through"], "ratings": verdict,
				"runs": []map[string]any{{"run_id": runID, "loaded": true, "current": true}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("ML_SERVICE_URL", server.URL)
}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

// TestFreshnessVerdictIsTheSameOnBothSurfaces is the test P2-1 asks for: the verdict the
// Ops console reads off /ops/status and the verdict the Lab reads off /api/ml/xi-status
// must describe the same store identically.
//
// They are the same computation — ml-service's `XiRegistry.freshness()`, H-11 — and this
// asserts that go-app's assembly copies it rather than re-deriving it. A badge saying
// fresh over a Lab that refuses is the P0-4 disagreement in a new place.
func TestFreshnessVerdictIsTheSameOnBothSurfaces(t *testing.T) {
	testCases := []struct {
		name    string
		verdict map[string]any
	}{
		{
			name: "fresh",
			verdict: map[string]any{
				"fresh": true, "data_age_days": 2, "max_age_days": 14, "data_through": "2026-09-05",
				"ratings_through": "2026-09-02", "code": nil,
			},
		},
		{
			name: "stale under a lowered limit",
			verdict: map[string]any{
				"fresh": false, "data_age_days": 5, "max_age_days": 3, "data_through": "2026-09-02",
				"ratings_through": "2026-09-02", "code": "RATINGS_STALE",
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			oneStoreMLService(t, tc.verdict, "20260903T160602Z-0e1e39c2")
			app := &App{}

			opsRecorder := httptest.NewRecorder()
			app.opsStatusHandler(opsRecorder, httptest.NewRequest(http.MethodGet, "/ops/status", nil))
			xiRecorder := httptest.NewRecorder()
			app.mlServiceProxy("/xi/status", "xi status proxy")(
				xiRecorder, httptest.NewRequest(http.MethodGet, "/api/ml/xi-status", nil))

			require.Equal(t, http.StatusOK, opsRecorder.Code)
			require.Equal(t, http.StatusOK, xiRecorder.Code)
			ops := decodeBody(t, opsRecorder.Body.Bytes())
			xi := decodeBody(t, xiRecorder.Body.Bytes())

			freshness, ok := ops["freshness"].(map[string]any)
			require.True(t, ok, "/ops/status must carry the one freshness object")
			served, ok := freshness["served"].(map[string]any)
			require.True(t, ok)
			labVerdict, ok := xi["ratings"].(map[string]any)
			require.True(t, ok, "/api/ml/xi-status must carry H-11's verdict")

			fields := []string{
				"fresh", "data_age_days", "max_age_days", "data_through", "ratings_through", "code",
			}
			for _, field := range fields {
				assert.Equal(t, labVerdict[field], served[field],
					"the Ops badge and the Lab's readiness notice must read the same %s", field)
			}
			assert.Equal(t, tc.verdict["ratings_through"], served["ratings_through"],
				"both surfaces name the date ml-service named")
		})
	}
}

// The deleted object stays deleted: two objects answering one question is the defect P2-1
// closed, so a re-introduced `db_freshness` fails here rather than quietly disagreeing
// with the verdict again.
func TestOpsStatusCarriesOneFreshnessObject(t *testing.T) {
	oneStoreMLService(t, map[string]any{
		"fresh": true, "data_age_days": 1, "max_age_days": 14, "data_through": "2026-09-06",
		"ratings_through": "2026-09-06", "code": nil,
	}, "20260906T101500Z-ab12cd34")
	app := &App{}

	recorder := httptest.NewRecorder()
	app.opsStatusHandler(recorder, httptest.NewRequest(http.MethodGet, "/ops/status", nil))

	body := decodeBody(t, recorder.Body.Bytes())
	require.Contains(t, body, "freshness")
	assert.NotContains(t, body, "db_freshness", "the 7/30 buckets and their worst-of badge are gone")
	freshness := body["freshness"].(map[string]any)
	assert.Contains(t, freshness, "served")
	assert.Contains(t, freshness, "database")
	assert.Contains(t, freshness, "retrain_due")
	assert.Len(t, freshness, 3, "three named facts and nothing else")
	assert.Contains(t, body, "db_completeness", "a different question, and still asked")
}
