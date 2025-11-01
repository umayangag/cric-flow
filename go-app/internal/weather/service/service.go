package service

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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

// BuildSessions derives session labels and approximate timestamps.
// If inningsCount > 0, uses inning1..inningN; otherwise defaults to 2 innings.
// Timestamps are nil for now; worker may enrich later when available.
func BuildSessions(info cricsheet.Info, inningsCount int) []sessionSpec {
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
func EnqueueJob(ctx context.Context, matchID int64, info cricsheet.Info, inningsCount int) error {
	venueName := strings.TrimSpace(firstNonEmpty(info.Venue, info.City))
	if venueName == "" {
		// No venue: nothing to enqueue
		return nil
	}
	norm := NormalizeVenue(venueName)
	var city *string
	if strings.TrimSpace(info.City) != "" {
		c := strings.TrimSpace(info.City)
		city = &c
	}
	sessions := BuildSessions(info, inningsCount)
	b, _ := json.Marshal(sessions)
	job := &db.WeatherJob{
		MatchID:         matchID,
		NormalizedVenue: norm,
		City:            city,
		Country:         nil,
		StartAtLocal:    nil,
		EndAtLocal:      nil,
		SessionsJSON:    b,
	}
	if err := db.EnqueueWeatherJob(ctx, job); err != nil {
		log.Printf("warn: enqueue weather job failed for match %d: %v", matchID, err)
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
