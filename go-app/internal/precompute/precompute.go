// Package precompute orchestrates precompute phases and maintains in-memory
// status for long-running feature computations. The code in this package is
// intentionally split into small, descriptive helpers to make the execution
// flow easier to follow for new developers.
package precompute

import (
	"context"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Run orchestrates precompute for the given season and list of format codes.
// If season is empty, computes for all seasons. If formats is empty, computes for all formats.
func Run(parent context.Context, season string, formats []string) error {
	ctx := parent
	if d := time.Duration(config.Load().Features.PrecomputeTimeoutMs) * time.Millisecond; d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, d)
		defer cancel()
	}
	// ensure DB connection
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	// Discover formats if not provided
	codes, err := discoverFormatCodes(ctx, formats)
	if err != nil {
		return err
	}

	// Update status tracker
	setStart(season, codes)
	defer setDone()

	for _, code := range codes {
		if err := runPhasesForFormat(ctx, season, code); err != nil {
			return err
		}
	}
	return nil
}
