package teampredictor

import (
	"errors"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

// normalizeFormat trims and uppercases the format and validates allowed values.
// Returns the normalized value or an error with the existing message style.
func normalizeFormat(s string) (string, error) {
	// Accept aliases (MDM, ODM, IT20) and return the canonical code.
	f := formats.CanonicalizeCode(s)
	switch f {
	case "TEST", "ODI", "T20I", "T20":
		return f, nil
	default:
		return "", errors.New("invalid format")
	}
}

// parsePositiveInt64ForMatch parses a required positive int64 for match id.
func parsePositiveInt64ForMatch(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid match id")
	}
	return id, nil
}

// parseNonNegativeInt parses a non-negative integer value; name controls error message.
func parseNonNegativeInt(name, s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v < 0 {
		return 0, errors.New("invalid " + name)
	}
	return v, nil
}
