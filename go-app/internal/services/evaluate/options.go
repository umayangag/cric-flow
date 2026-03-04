package evaluate

import (
	"errors"
	"flag"
)

// Options holds CLI options for evaluate command.
type Options struct {
	Season string
	Format string
}

// ParseArgs parses args using the provided FlagSet (or a new one if fs is nil).
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	if fs == nil {
		fs = flag.NewFlagSet("evaluate", flag.ContinueOnError)
	}

	var (
		season = fs.String("season", "demo", "Season identifier")
		format = fs.String("format", "T20", "Match format (e.g., T20, ODI)")
	)

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if *season == "" {
		return Options{}, errors.New("season must not be empty")
	}
	if *format == "" {
		return Options{}, errors.New("format must not be empty")
	}

	return Options{Season: *season, Format: *format}, nil
}
