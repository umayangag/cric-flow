package opsstatus

import (
	"context"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/precompute"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// GetPrecomputeStatus is a function variable to allow test-time substitution.
// In production it points to precompute.GetStatus.
var GetPrecomputeStatus = precompute.GetStatus

// BuildPrecomputeSection constructs the precompute part of the ops status.
func BuildPrecomputeSection(ctx context.Context, now time.Time) map[string]any {
	stat := GetPrecomputeStatus()
	formats := CricketFormatCodes

	section := map[string]any{
		"last_run": "",
		"as_of":    "",
		"formats":  map[string]any{},
	}

	fm := map[string]any{}
	for _, f := range formats {
		fm[f] = map[string]any{"status": "missing"}
	}

	var finishedAt time.Time
	var formatsRan []string
	if !stat.FinishedAt.IsZero() {
		finishedAt = stat.FinishedAt.UTC()
		formatsRan = stat.Formats
	} else {
		lastCompleted, err := tracking.GetLastCompletedAtForCommand(ctx, "precompute-features")
		if err != nil {
			slog.Warn("precompute section: GetLastCompletedAtForCommand failed", "err", err)
		}
		if lastCompleted != nil {
			finishedAt = lastCompleted.UTC()
			formatsRan = formats
		}
	}

	if !finishedAt.IsZero() {
		section["last_run"] = finishedAt.Format(time.RFC3339)
		section["as_of"] = finishedAt.Format("2006-01-02")

		ran := make(map[string]struct{})
		for _, f := range formatsRan {
			ran[f] = struct{}{}
		}
		y1, m1, d1 := now.UTC().Date()
		y2, m2, d2 := finishedAt.Date()
		sameDay := (y1 == y2 && m1 == m2 && d1 == d2)

		for _, f := range formats {
			if _, ok := ran[f]; ok {
				if sameDay {
					fm[f] = map[string]any{"status": "ok"}
				} else {
					fm[f] = map[string]any{"status": "stale"}
				}
			} else {
				fm[f] = map[string]any{"status": "missing"}
			}
		}
	}

	section["formats"] = fm
	return section
}
