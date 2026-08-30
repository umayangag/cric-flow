package teamselect

import (
	"errors"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// Options holds CLI flags for team-select.
type Options struct {
	MatchID       int64
	Format        string
	Season        string
	TeamSize      int
	MinBowlers    int
	RequireKeeper bool
}

// Validate checks the option values and normalizes where appropriate.
// It returns an error describing the first invalid condition encountered.
func (o *Options) Validate() error {
	// Normalize format to uppercase and trimmed for consistency
	o.Format = formats.CanonicalizeCode(o.Format)

	if o.MatchID <= 0 {
		return errors.New("match is required and must be a positive number")
	}
	if strings.TrimSpace(o.Season) == "" {
		return errors.New("season is required")
	}
	if !formats.IsCanonical(o.Format) {
		return errors.New(
			"invalid format: must be one of " +
				strings.Join(formats.CanonicalCodes(), ", ") +
				" (aliases: MDM, ODM, IT20)",
		)
	}
	if o.TeamSize <= 0 {
		return errors.New("team size must be a positive number")
	}
	if o.MinBowlers < 0 {
		return errors.New("min-bowlers must be a non-negative number")
	}
	return nil
}
