package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// backtestEvaluateStartHandler handles POST /api/backtest/evaluate-start (body or query: format, team1, team2, match_id).
// Starts evaluation in the background and returns { "job_id": "..." } so the client can poll evaluate-status.
func (a *App) backtestEvaluateStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Code: "METHOD_NOT_ALLOWED", Message: "POST or GET required"})
		return
	}
	var format, team1, team2, matchID string
	var useLatestModel bool
	useLatestFromBody := false
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		var body struct {
			Format         string `json:"format"`
			Team1          string `json:"team1"`
			Team2          string `json:"team2"`
			MatchID        int64  `json:"match_id"`
			UseLatestModel *bool  `json:"use_latest_model,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(
				w,
				http.StatusBadRequest,
				apiError{Code: "INVALID_BODY", Message: "JSON body with format, team1, team2, match_id required"},
			)
			return
		}
		format = strings.TrimSpace(body.Format)
		team1 = strings.TrimSpace(body.Team1)
		team2 = strings.TrimSpace(body.Team2)
		if body.MatchID != 0 {
			matchID = strconv.FormatInt(body.MatchID, 10)
		}
		if body.UseLatestModel != nil {
			useLatestModel = *body.UseLatestModel
			useLatestFromBody = true
		}
	}
	if !useLatestFromBody {
		useLatestModel = parseUseLatestModel(r, false)
	}
	if format == "" || team1 == "" || team2 == "" || matchID == "" {
		q := r.URL.Query()
		if format == "" {
			format = strings.TrimSpace(q.Get("format"))
		}
		if team1 == "" {
			team1 = strings.TrimSpace(q.Get("team1"))
		}
		if team2 == "" {
			team2 = strings.TrimSpace(q.Get("team2"))
		}
		if matchID == "" {
			matchID = strings.TrimSpace(q.Get("match_id"))
		}
	}
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if matchID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	if _, err := strconv.ParseInt(matchID, 10, 64); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}

	jobID, err := startEvaluateJob(r.Context(), format, team1, team2, matchID, useLatestModel)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// backtestEvaluateStatusHandler handles GET /api/backtest/evaluate-status?job_id=...
// Returns current job state: status (running|done|error), steps, result (if done), error (if error).
func (a *App) backtestEvaluateStatusHandler(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "job_id is required"})
		return
	}
	snap, ok := getEvaluateJobStatus(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "job not found (expired or invalid id)"})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// backtestEvaluateStreamHandler handles GET /api/backtest/evaluate-stream and streams progress via SSE, then the result.
// Query params: format, team1, team2, match_id (same as evaluate). use_ml=1 is not supported for streaming.
func (a *App) backtestEvaluateStreamHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := strings.TrimSpace(q.Get("format"))
	team1 := strings.TrimSpace(q.Get("team1"))
	team2 := strings.TrimSpace(q.Get("team2"))
	matchID := strings.TrimSpace(q.Get("match_id"))
	if format == "" || team1 == "" || team2 == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "format, team1, team2 are required"},
		)
		return
	}
	if matchID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "match_id is required"})
		return
	}
	if _, err := strconv.ParseInt(matchID, 10, 64); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "invalid match_id"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	writeSSE := func(event, data string) bool {
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + data + "\n\n")); err != nil {
			log.Printf("backtest evaluate-stream: write failed (client may have disconnected): %v", err)
			return false
		}
		flusher.Flush()
		return true
	}

	progress := func(step, message string) {
		payload := map[string]string{"step": step, "message": message}
		data, _ := json.Marshal(payload)
		writeSSE("progress", string(data))
	}

	useLatest := parseUseLatestModel(r, false)
	resp, err := doEvaluateWork(r.Context(), format, team1, team2, matchID, useLatest, progress)
	if err != nil {
		payload := map[string]string{"message": err.Error()}
		data, _ := json.Marshal(payload)
		if !writeSSE("error", string(data)) {
			return
		}
		return
	}
	resultData, err := json.Marshal(resp)
	if err != nil {
		payload := map[string]string{"message": "failed to encode result"}
		data, _ := json.Marshal(payload)
		if !writeSSE("error", string(data)) {
			return
		}
		return
	}
	writeSSE("result", string(resultData))
}
