package weatherworker

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"
)

// Options captures CLI options for weather-worker.
type Options struct {
	Provider string
	Apply    bool
	MaxJobs  int // 0 => unlimited
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		provider string
		apply    bool
		maxStr   string
	)
	provider = getenv("WEATHER_PROVIDER", "dummy")

	fs.StringVar(&provider, "provider", provider, "weather provider name (e.g., dummy)")
	fs.BoolVar(&apply, "apply", false, "apply changes; if false, dry-run")
	fs.StringVar(&maxStr, "max", "0", "max jobs to process (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return Options{}, errors.New("provider is required")
	}
	maxJobs, err := strconv.Atoi(strings.TrimSpace(maxStr))
	if err != nil || maxJobs < 0 {
		return Options{}, errors.New("invalid max")
	}
	return Options{Provider: provider, Apply: apply, MaxJobs: maxJobs}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
