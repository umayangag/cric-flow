package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// evalJobStep is one progress step for an evaluation job.
type evalJobStep struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

// evalJobStatusResponse is the JSON shape for evaluate-status; no mutex so it is safe to copy.
type evalJobStatusResponse struct {
	JobID           string                    `json:"job_id"`
	MatchID         string                    `json:"match_id"`
	Format          string                    `json:"format"`
	Team1           string                    `json:"team1"`
	Team2           string                    `json:"team2"`
	UseUnifiedModel bool                      `json:"use_unified_model,omitempty"`
	UseLatestModel  bool                      `json:"use_latest_model,omitempty"`
	Status          string                    `json:"status"` // "running" | "done" | "error"
	Steps           []evalJobStep             `json:"steps,omitempty"`
	Result          *backtestEvaluateResponse `json:"result,omitempty"`
	Error           string                    `json:"error,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

// evalJobState holds the state of a single evaluate job (in-memory; survives refresh, not server restart).
type evalJobState struct {
	mu              sync.Mutex
	JobID           string                    `json:"job_id"`
	MatchID         string                    `json:"match_id"`
	Format          string                    `json:"format"`
	Team1           string                    `json:"team1"`
	Team2           string                    `json:"team2"`
	UseUnifiedModel bool                      `json:"use_unified_model,omitempty"`
	UseLatestModel  bool                      `json:"use_latest_model,omitempty"`
	Status          string                    `json:"status"` // "running" | "done" | "error"
	Steps           []evalJobStep             `json:"steps,omitempty"`
	Result          *backtestEvaluateResponse `json:"result,omitempty"`
	Error           string                    `json:"error,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

func (s *evalJobState) appendStep(step, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Steps = append(s.Steps, evalJobStep{Step: step, Message: message})
	s.UpdatedAt = time.Now()
}

func (s *evalJobState) setDone(result *backtestEvaluateResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "done"
	s.Result = result
	s.UpdatedAt = time.Now()
}

func (s *evalJobState) setError(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "error"
	s.Error = errMsg
	s.UpdatedAt = time.Now()
}

func (s *evalJobState) snapshot() evalJobStatusResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	stepsCopy := make([]evalJobStep, len(s.Steps))
	copy(stepsCopy, s.Steps)
	return evalJobStatusResponse{
		JobID:           s.JobID,
		MatchID:         s.MatchID,
		Format:          s.Format,
		Team1:           s.Team1,
		Team2:           s.Team2,
		UseUnifiedModel: s.UseUnifiedModel,
		UseLatestModel:  s.UseLatestModel,
		Status:          s.Status,
		Steps:           stepsCopy,
		Result:          s.Result,
		Error:           s.Error,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

var (
	evalJobStore     = make(map[string]*evalJobState)
	evalJobStoreMu   sync.RWMutex
	evalJobSem       chan struct{} // limits concurrent running jobs
	evalJobCleanupCh chan struct{} // closed to stop cleanup goroutine
)

func evalJobCleanupAge() time.Duration {
	cfg := config.Load()
	hr := config.BacktestJobCleanupAgeHours(cfg)
	return time.Duration(hr) * time.Hour
}

func evalJobCleanupEvery() time.Duration {
	cfg := config.Load()
	mins := config.BacktestJobCleanupIntervalMin(cfg)
	return time.Duration(mins) * time.Minute
}

func evalJobMaxDuration() time.Duration {
	cfg := config.Load()
	hr := config.BacktestEvalJobMaxDurationHr(cfg)
	return time.Duration(hr) * time.Hour
}

func evalJobMaxConcurrent() int {
	if v := os.Getenv("EVAL_JOB_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	cfg := config.Load()
	minC := config.BacktestEvalJobConcurrencyMin(cfg)
	maxC := config.BacktestEvalJobConcurrencyMax(cfg)
	n := runtime.NumCPU()
	if n < minC {
		return minC
	}
	if n > maxC {
		return maxC
	}
	return n
}

func generateEvalJobID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func init() {
	evalJobSem = make(chan struct{}, evalJobMaxConcurrent())
	evalJobCleanupCh = make(chan struct{})
	go evalJobCleanupLoop()
}

// evalJobCleanupLoop periodically removes jobs older than evalJobCleanupAge() to prevent unbounded memory growth.
func evalJobCleanupLoop() {
	ticker := time.NewTicker(evalJobCleanupEvery())
	defer ticker.Stop()
	for {
		select {
		case <-evalJobCleanupCh:
			return
		case <-ticker.C:
			evalJobCleanup()
		}
	}
}

func evalJobCleanup() {
	cutoff := time.Now().Add(-evalJobCleanupAge())
	evalJobStoreMu.Lock()
	defer evalJobStoreMu.Unlock()
	for id, job := range evalJobStore {
		job.mu.Lock()
		updated := job.UpdatedAt
		job.mu.Unlock()
		if updated.Before(cutoff) {
			delete(evalJobStore, id)
		}
	}
}

// startEvaluateJob starts doEvaluateWork in a goroutine and returns the job ID immediately.
// The job state is updated with progress and final result or error.
// Uses a long-lived context (not the request context) so the job is not cancelled when the HTTP
// request ends, and has a generous deadline so it can run for hours without exceeding it.
func startEvaluateJob(
	_ context.Context,
	format, team1, team2, matchID string,
	useUnifiedModel, useLatestModel bool,
) (string, error) {
	jobID, err := generateEvalJobID()
	if err != nil {
		slog.Error("startEvaluateJob: generate job ID failed", slog.Any("err", err))
		return "", err
	}
	now := time.Now()
	job := &evalJobState{
		JobID:           jobID,
		MatchID:         matchID,
		Format:          format,
		Team1:           team1,
		Team2:           team2,
		UseUnifiedModel: useUnifiedModel,
		UseLatestModel:  useLatestModel,
		Status:          "running",
		Steps:           nil,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	evalJobStoreMu.Lock()
	evalJobStore[jobID] = job
	evalJobStoreMu.Unlock()

	go func() {
		// Limit concurrent evaluation jobs to avoid exhausting server resources.
		evalJobSem <- struct{}{}
		defer func() { <-evalJobSem }()
		slog.Info(
			"evaluate job started",
			slog.String("job_id", jobID),
			slog.String("format", format),
			slog.String("team1", team1),
			slog.String("team2", team2),
			slog.String("match_id", matchID),
			slog.Bool("use_unified_model", useUnifiedModel),
			slog.Bool("use_latest_model", useLatestModel),
		)
		// Not the request context (cancelled when we return 202). Use a long deadline so the job
		// can run for hours (e.g. ML train-on-the-fly) without exceeding it.
		jobCtx, cancel := context.WithTimeout(context.Background(), evalJobMaxDuration())
		defer cancel()
		progress := func(step, message string) {
			job.appendStep(step, message)
		}
		resp, err := doEvaluateWork(
			jobCtx,
			format,
			team1,
			team2,
			matchID,
			job.UseUnifiedModel,
			job.UseLatestModel,
			progress,
		)
		if err != nil {
			slog.Error(
				"evaluate job failed",
				slog.String("job_id", jobID),
				slog.String("format", format),
				slog.String("team1", team1),
				slog.String("team2", team2),
				slog.String("match_id", matchID),
				slog.Any("err", err),
			)
			job.setError(err.Error())
			return
		}
		slog.Info("evaluate job completed", slog.String("job_id", jobID), slog.String("format", format))
		job.setDone(resp)
	}()

	return jobID, nil
}

// getEvaluateJobStatus returns a snapshot of the job state. The second return is false if not found.
func getEvaluateJobStatus(jobID string) (evalJobStatusResponse, bool) {
	evalJobStoreMu.RLock()
	job := evalJobStore[jobID]
	evalJobStoreMu.RUnlock()
	if job == nil {
		return evalJobStatusResponse{}, false
	}
	return job.snapshot(), true
}
