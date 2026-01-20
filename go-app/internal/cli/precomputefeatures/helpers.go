package precomputefeatures

import (
	"errors"
	"strings"
	"time"
)

// parseAsOf parses a YYYY-MM-DD date string.
// Returns (zeroTime, false, nil) when input is empty/whitespace.
func parseAsOf(raw string) (time.Time, bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

// validateAlpha ensures alpha is within (0,1].
func validateAlpha(alpha float64) error {
	if !(alpha > 0 && alpha <= 1) {
		return errors.New("ewm-alpha must be in (0,1]")
	}
	return nil
}

// validateLastN ensures lastN is non-negative.
func validateLastN(n int) error {
	if n < 0 {
		return errors.New("lastN must be >= 0")
	}
	return nil
}
