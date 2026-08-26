package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// pipelineProgressStreamHandler handles GET /ops/pipeline/stream and streams pipeline
// progress via SSE. It is transport only: the payload is built by progressReporter.
func (a *App) pipelineProgressStreamHandler(w http.ResponseWriter, r *http.Request) {
	stream, ok := newSSEStream(w)
	if !ok {
		return
	}

	reporter := newProgressReporter()
	send := func() bool {
		running, err := tracking.GetInProgressMigrations(r.Context())
		if err != nil {
			slog.Warn("pipeline progress stream: reading in-flight steps failed", slog.Any("err", err))
			running = nil
		}
		data, err := json.Marshal(reporter.snapshot(r.Context(), running))
		if err != nil {
			slog.Error("pipeline progress stream: marshal failed", slog.Any("err", err))
			return true
		}
		return stream.send("progress", string(data))
	}

	if !send() {
		return
	}

	ticker := time.NewTicker(pipelinesvc.ProgressInterval())
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

// sseStream writes server-sent events to a response, flushing each one.
type sseStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// newSSEStream writes the SSE headers and returns a stream, or ok=false when the
// response cannot be flushed incrementally.
func newSSEStream(w http.ResponseWriter) (*sseStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &sseStream{w: w, flusher: flusher}, true
}

// send writes one event and reports whether the client is still connected.
func (s *sseStream) send(event, data string) bool {
	if _, err := s.w.Write([]byte("event: " + event + "\ndata: " + data + "\n\n")); err != nil {
		slog.Info("pipeline progress stream: write failed (client likely disconnected)", slog.Any("err", err))
		return false
	}
	s.flusher.Flush()
	return true
}
