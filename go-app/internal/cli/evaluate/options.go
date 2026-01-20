package evaluate

import (
	"flag"
)

// Options holds CLI options for evaluate command.
// Defaults mirror existing behavior (format=T20, season=demo).
// Keeping flags stable preserves current usage.
type Options struct {
	Season string
	Format string
}

// ParseArgs parses args using the provided FlagSet (or a new one if fs is nil).
// It applies defaults and basic validation.
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
	if err := validateNonEmpty(*season, "season"); err != nil {
		return Options{}, err
	}
	if err := validateNonEmpty(*format, "format"); err != nil {
		return Options{}, err
	}
	return Options{Season: *season, Format: *format}, nil
}
