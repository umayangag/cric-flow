package importkeepers

import (
	"errors"
	"flag"
)

// ParseArgs parses CLI args into Options. Pure and testable.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		file       string
		apply      bool
		othersZero bool
	)
	fs.StringVar(&file, "file", "", "path to CSV file with keepers")
	fs.BoolVar(&apply, "apply", false, "apply changes (default is dry-run)")
	fs.BoolVar(&othersZero, "others-zero", false, "set is_wicket_keeper=0 for players not in CSV")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if file == "" {
		return Options{}, errors.New("missing required --file")
	}
	return Options{File: file, Apply: apply, OthersZero: othersZero}, nil
}
