package weather

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// NormalizeVenue makes a stable lookup key from a raw venue/ground name.
func NormalizeVenue(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace
	return s
}

// sessionSpec is a minimal struct stored in JSONB sessions field for a job.
type sessionSpec struct {
	Label string     `json:"label"`
	At    *time.Time `json:"at,omitempty"`
}

// buildSessions derives session labels and approximate timestamps.
// If inningsCount > 0, uses inning1..inningN; otherwise defaults to 2 innings.
// Timestamps are nil for now; worker may enrich later when available.
func buildSessions(inningsCount int) []sessionSpec {
	if inningsCount <= 0 {
		inningsCount = 2
	}
	sess := make([]sessionSpec, 0, inningsCount)
	for i := 1; i <= inningsCount; i++ {
		label := "inning" + strconv.Itoa(i)
		sess = append(sess, sessionSpec{Label: label})
	}
	return sess
}

// EnqueueJob creates a weather_job row (idempotent on match_id) and returns nil on success.
func EnqueueJob(ctx context.Context, matchID int64, city string, venue string, inningsCount int) error {
	venueName := strings.TrimSpace(firstNonEmpty(venue, city))
	if venueName == "" {
		// No venue: nothing to enqueue
		return nil
	}
	norm := NormalizeVenue(venueName)
	var cityPtr *string
	if strings.TrimSpace(city) != "" {
		c := strings.TrimSpace(city)
		cityPtr = &c
	}
	sessions := buildSessions(inningsCount)
	b, err := json.Marshal(sessions)
	if err != nil {
		slog.Error("marshal sessions failed", slog.Any("err", err), slog.Int("innings", inningsCount))
		return err
	}
	job := &db.WeatherJob{
		MatchID:         matchID,
		NormalizedVenue: norm,
		City:            cityPtr,
		Country:         nil,
		StartAtLocal:    nil,
		EndAtLocal:      nil,
		SessionsJSON:    b,
	}
	if err := db.EnqueueWeatherJob(ctx, job); err != nil {
		slog.Warn("enqueue weather job failed", slog.Int64("match_id", matchID), slog.Any("err", err))
		return err
	}
	return nil
}

// firstNonEmpty helper (duplicated minimal to avoid import cycles)
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
