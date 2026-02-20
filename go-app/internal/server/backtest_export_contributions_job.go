package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// exportContributionsJobStatusResponse is the JSON shape for export-contributions-status.
type exportContributionsJobStatusResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"` // "running" | "done" | "error"
	Path      string    `json:"path,omitempty"`
	Rows      int       `json:"rows,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// exportContributionsJobState holds the state of a single export-contributions job.
type exportContributionsJobState struct {
	mu        sync.Mutex
	JobID     string
	Status    string // "running" | "done" | "error"
	Path      string
	Rows      int
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *exportContributionsJobState) setDone(path string, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "done"
	s.Path = path
	s.Rows = rows
	s.UpdatedAt = time.Now()
}

func (s *exportContributionsJobState) setError(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "error"
	s.Error = errMsg
	s.UpdatedAt = time.Now()
}

func (s *exportContributionsJobState) snapshot() exportContributionsJobStatusResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return exportContributionsJobStatusResponse{
		JobID:     s.JobID,
		Status:    s.Status,
		Path:      s.Path,
		Rows:      s.Rows,
		Error:     s.Error,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

var (
	exportContributionsJobStore   = make(map[string]*exportContributionsJobState)
	exportContributionsJobStoreMu sync.RWMutex
	exportContributionsJobSem     chan struct{} // limit concurrent jobs (1)
	exportContributionsCleanupCh  chan struct{}
)

func exportContributionsJobCleanupAge() time.Duration {
	cfg := config.Load()
	hr := config.BacktestJobCleanupAgeHours(cfg)
	return time.Duration(hr) * time.Hour
}

func exportContributionsJobCleanupEvery() time.Duration {
	cfg := config.Load()
	mins := config.BacktestJobCleanupIntervalMin(cfg)
	return time.Duration(mins) * time.Minute
}

func exportContributionsJobMaxDuration() time.Duration {
	cfg := config.Load()
	hr := config.BacktestExportContributionsJobMaxDurationHr(cfg)
	return time.Duration(hr) * time.Hour
}

func generateExportContributionsJobID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func init() {
	exportContributionsJobSem = make(chan struct{}, 1) // one export-contributions job at a time
	exportContributionsCleanupCh = make(chan struct{})
	go exportContributionsCleanupLoop()
}

func exportContributionsCleanupLoop() {
	ticker := time.NewTicker(exportContributionsJobCleanupEvery())
	defer ticker.Stop()
	for {
		select {
		case <-exportContributionsCleanupCh:
			return
		case <-ticker.C:
			exportContributionsCleanup()
		}
	}
}

func exportContributionsCleanup() {
	cutoff := time.Now().Add(-exportContributionsJobCleanupAge())
	exportContributionsJobStoreMu.Lock()
	defer exportContributionsJobStoreMu.Unlock()
	for id, job := range exportContributionsJobStore {
		job.mu.Lock()
		updated := job.UpdatedAt
		job.mu.Unlock()
		if updated.Before(cutoff) {
			delete(exportContributionsJobStore, id)
		}
	}
}

// startExportContributionsJob starts runExportContributionsWork in a background goroutine and returns the job ID.
// The request body must already be validated by the handler.
func startExportContributionsJob(body exportContributionsRequest) (string, error) {
	jobID, err := generateExportContributionsJobID()
	if err != nil {
		slog.Error("startExportContributionsJob: generate job ID failed", slog.Any("err", err))
		return "", err
	}
	now := time.Now()
	job := &exportContributionsJobState{
		JobID:     jobID,
		Status:    "running",
		CreatedAt: now,
		UpdatedAt: now,
	}
	exportContributionsJobStoreMu.Lock()
	exportContributionsJobStore[jobID] = job
	exportContributionsJobStoreMu.Unlock()

	go func() {
		exportContributionsJobSem <- struct{}{}
		defer func() { <-exportContributionsJobSem }()
		slog.Info(
			"export-contributions job started",
			slog.String("job_id", jobID),
			slog.Int("match_count", len(body.MatchIDs)),
		)
		jobCtx, cancel := context.WithTimeout(context.Background(), exportContributionsJobMaxDuration())
		defer cancel()
		path, rows, err := runExportContributionsWork(jobCtx, body)
		if err != nil {
			slog.Error("export-contributions job failed", slog.String("job_id", jobID), slog.Any("err", err))
			job.setError(err.Error())
			return
		}
		slog.Info("export-contributions job completed", slog.String("job_id", jobID), slog.Int("rows", rows))
		job.setDone(path, rows)
	}()

	return jobID, nil
}

// getExportContributionsJobStatus returns a snapshot of the job state. The second return is false if not found.
func getExportContributionsJobStatus(jobID string) (exportContributionsJobStatusResponse, bool) {
	exportContributionsJobStoreMu.RLock()
	job := exportContributionsJobStore[jobID]
	exportContributionsJobStoreMu.RUnlock()
	if job == nil {
		return exportContributionsJobStatusResponse{}, false
	}
	return job.snapshot(), true
}
