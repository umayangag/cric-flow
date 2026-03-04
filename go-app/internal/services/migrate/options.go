package migrate

import (
	"flag"
	"os"
)

// Options for migrate command.
type Options struct {
	Dir string
}

// ParseArgs parses flags for migrate.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	if fs == nil {
		fs = flag.NewFlagSet("migrate", flag.ContinueOnError)
	}

	// Default from env MIGRATIONS_DIR if set, else fallback to "migrations"
	defDir := os.Getenv("MIGRATIONS_DIR")
	if defDir == "" {
		defDir = "migrations"
	}

	dir := fs.String("dir", defDir, "directory with .sql migration files (can also set MIGRATIONS_DIR)")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	return Options{Dir: *dir}, nil
}
