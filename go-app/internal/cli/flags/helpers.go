package flags

import (
	"errors"
	"strings"
	"time"
)

// ParseDateISO parses a date in the strict ISO layout YYYY-MM-DD (Go's 2006-01-02).
// It returns a time.Time on success or an error that includes the provided value
// when the input cannot be parsed.
func ParseDateISO(value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("invalid date (expected YYYY-MM-DD): " + value)
	}
	return t, nil
}

// ParseRFC3339 parses a timestamp in RFC3339 format.
func ParseRFC3339(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("invalid RFC3339 timestamp: " + value)
	}
	return t, nil
}

// ParseCSVList splits a comma-separated list, trimming spaces and dropping empty items.
// It always returns a non-nil slice (possibly empty).
func ParseCSVList(value string) []string {
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s := strings.TrimSpace(item)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// RequireNonEmpty validates that a required flag/string value is non-empty.
// The name is included in the error message for clarity to end users.
func RequireNonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("missing required: " + name)
	}
	return nil
}

// ParseDurationFlag parses a duration string using time.ParseDuration.
// The error message is standardized for consistency across commands.
func ParseDurationFlag(value string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, errors.New("invalid duration (e.g. 5s, 2m, 1h): " + value)
	}
	return d, nil
}
