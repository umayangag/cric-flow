package migrate

import (
	"flag"
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
	dir := fs.String("dir", "migrations", "directory with .sql migration files")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	return Options{Dir: *dir}, nil
}
