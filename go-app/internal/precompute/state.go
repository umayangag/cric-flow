package precompute

import (
	"log/slog"
	"sync"
	"time"
)

// Phases a run reports. The first four track progress; the last two are terminal and
// say how the run ended.
const (
	PhaseStarting = "starting"
	PhaseDone     = "done"   // the run finished its work
	PhaseFailed   = "failed" // the run stopped early: an error, or a cancellation
)

// Status represents the current/last known state of a precompute run.
// It is intentionally simple and in-memory; resets on process restart.
type Status struct {
	Running         bool      `json:"running"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
	Season          string    `json:"season,omitempty"`
	Formats         []string  `json:"formats,omitempty"`
	CurrentFormat   string    `json:"current_format,omitempty"`    // format currently being processed (when running)
	FormatStartedAt time.Time `json:"format_started_at,omitempty"` // when current format started (for ETA)
	Phase           string    `json:"phase,omitempty"`             // one of: form, venue, opposition, consistency, starting, done, failed
	LastError       string    `json:"last_error,omitempty"`
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
		Phase:     PhaseStarting,
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
	currentStat.FormatStartedAt = time.Now().UTC()
}

// setDone records how the run ended. It takes the run's error rather than leaving the
// outcome to a separate call, because a status that stamps FinishedAt whatever
// happened cannot be told apart from a successful one: a cancelled run reported every
// format as freshly computed, green, on the console.
func setDone(runErr error) {
	var errMsg string
	if runErr != nil {
		errMsg = runErr.Error()
	}

	mu.Lock()
	defer mu.Unlock()
	currentStat.Running = false
	currentStat.FinishedAt = time.Now().UTC()
	currentStat.CurrentFormat = ""
	currentStat.FormatStartedAt = time.Time{}
	currentStat.LastError = errMsg
	if runErr != nil {
		currentStat.Phase = PhaseFailed
		slog.Error("precompute run error recorded in status", slog.String("last_error", errMsg))
		return
	}
	currentStat.Phase = PhaseDone
}

// Succeeded reports whether the last finished run completed its work. A run that is
// still going has not succeeded yet, and neither has one that stopped early.
func (s Status) Succeeded() bool {
	return !s.Running && !s.FinishedAt.IsZero() && s.Phase == PhaseDone
}
