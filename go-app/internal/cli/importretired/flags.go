package importretired

import (
	"errors"
	"flag"
	"time"
)

// ParseArgs parses CLI args into Options. Pure and testable.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		file       string
		apply      bool
		othersZero bool
		timeout    time.Duration
	)
	fs.StringVar(&file, "file", "", "CSV file path (required)")
	fs.BoolVar(&apply, "apply", false, "apply changes (default is dry-run)")
	fs.BoolVar(&othersZero, "others-zero", false, "set is_retired=0 for players not listed in CSV")
	fs.DurationVar(&timeout, "timeout", 60*time.Second, "operation timeout")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if file == "" {
		return Options{}, errors.New("missing required --file")
	}
	return Options{File: file, Apply: apply, OthersZero: othersZero, Timeout: timeout}, nil
}
