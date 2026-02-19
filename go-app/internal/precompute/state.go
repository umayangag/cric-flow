package precompute

import (
	"log/slog"
	"sync"
	"time"
)

// Status represents the current/last known state of a precompute run.
// It is intentionally simple and in-memory; resets on process restart.
type Status struct {
	Running       bool      `json:"running"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	Season        string    `json:"season,omitempty"`
	Formats       []string  `json:"formats,omitempty"`
	CurrentFormat string    `json:"current_format,omitempty"` // format currently being processed (when running)
	Phase         string    `json:"phase,omitempty"`          // one of: form, venue, opposition, consistency, starting, done
	LastError     string    `json:"last_error,omitempty"`
}

var (
	mu          sync.RWMutex
	currentStat Status
)

// snapshot returns a copy of the current status.
func snapshot() Status {
	mu.RLock()
	defer mu.RUnlock()
	return currentStat
}

// GetStatus exposes a snapshot of the current status.
func GetStatus() Status { return snapshot() }

func setStart(season string, formats []string) {
	mu.Lock()
	defer mu.Unlock()
	currentStat = Status{
		Running:   true,
		StartedAt: time.Now().UTC(),
		Season:    season,
		Formats:   append([]string{}, formats...),
		Phase:     "starting",
		LastError: "",
	}
}

func setPhase(phase string) {
	mu.Lock()
	defer mu.Unlock()
	currentStat.Phase = phase
}

func setCurrentFormat(code string) {
	mu.Lock()
	defer mu.Unlock()
	currentStat.CurrentFormat = code
}

func setDone() {
	mu.Lock()
	defer mu.Unlock()
	currentStat.Running = false
	currentStat.FinishedAt = time.Now().UTC()
	currentStat.Phase = "done"
	currentStat.CurrentFormat = ""
}

// setLastError records the error message in status (e.g. when Run fails). Call before setDone.
func setLastError(errMsg string) {
	mu.Lock()
	defer mu.Unlock()
	currentStat.LastError = errMsg
	if errMsg != "" {
		slog.Error("precompute run error recorded in status", slog.String("last_error", errMsg))
	}
}
