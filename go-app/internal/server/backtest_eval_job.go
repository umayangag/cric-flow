package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// evalJobStep is one progress step for an evaluation job.
type evalJobStep struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

// evalJobState holds the state of a single evaluate job (in-memory; survives refresh, not server restart).
type evalJobState struct {
	mu        sync.Mutex
	JobID     string    `json:"job_id"`
	MatchID   string    `json:"match_id"`
	Format    string    `json:"format"`
	Team1     string    `json:"team1"`
	Team2     string    `json:"team2"`
	Status    string    `json:"status"` // "running" | "done" | "error"
	Steps     []evalJobStep `json:"steps,omitempty"`
	Result    *backtestEvaluateResponse `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

func (s *evalJobState) snapshot() evalJobState {
	s.mu.Lock()
	defer s.mu.Unlock()
	stepsCopy := make([]evalJobStep, len(s.Steps))
	copy(stepsCopy, s.Steps)
	return evalJobState{
		JobID:     s.JobID,
		MatchID:   s.MatchID,
		Format:    s.Format,
		Team1:     s.Team1,
		Team2:     s.Team2,
		Status:    s.Status,
		Steps:     stepsCopy,
		Result:    s.Result,
		Error:     s.Error,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

var (
	evalJobStore   = make(map[string]*evalJobState)
	evalJobStoreMu sync.RWMutex
)

func generateEvalJobID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// startEvaluateJob starts doEvaluateWork in a goroutine and returns the job ID immediately.
// The job state is updated with progress and final result or error.
func startEvaluateJob(ctx context.Context, format, team1, team2, matchID string) (string, error) {
	jobID, err := generateEvalJobID()
	if err != nil {
		return "", err
	}
	now := time.Now()
	job := &evalJobState{
		JobID:     jobID,
		MatchID:   matchID,
		Format:    format,
		Team1:     team1,
		Team2:     team2,
		Status:    "running",
		Steps:     nil,
		CreatedAt: now,
		UpdatedAt: now,
	}
	evalJobStoreMu.Lock()
	evalJobStore[jobID] = job
	evalJobStoreMu.Unlock()

	go func() {
		progress := func(step, message string) {
			job.appendStep(step, message)
		}
		resp, err := doEvaluateWork(ctx, format, team1, team2, matchID, progress)
		if err != nil {
			job.setError(err.Error())
			return
		}
		job.setDone(resp)
	}()

	return jobID, nil
}

// getEvaluateJobStatus returns a snapshot of the job state. The second return is false if not found.
func getEvaluateJobStatus(jobID string) (evalJobState, bool) {
	evalJobStoreMu.RLock()
	job := evalJobStore[jobID]
	evalJobStoreMu.RUnlock()
	if job == nil {
		return evalJobState{}, false
	}
	return job.snapshot(), true
}
