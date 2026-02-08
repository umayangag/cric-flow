package tracking

import (
	"encoding/json"
	"time"
)

type MigrationStatus string

const (
	StatusInProgress MigrationStatus = "IN_PROGRESS"
	StatusCompleted  MigrationStatus = "COMPLETED"
	StatusFailed     MigrationStatus = "FAILED"
	StatusCancelled  MigrationStatus = "CANCELLED"
)

type Migration struct {
	ID           int             `json:"id"`
	Command      string          `json:"command"`
	Args         json.RawMessage `json:"args"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	Status       MigrationStatus `json:"status"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}
