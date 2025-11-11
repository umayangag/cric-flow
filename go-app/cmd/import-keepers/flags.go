package main

import (
	"errors"
	"flag"
)

// options holds CLI flags for import-keepers.
type options struct {
	file       string
	apply      bool
	othersZero bool
}

// parseFlags parses CLI args into options. Pure and testable.
func parseFlags(args []string) (options, error) {
	var (
		file       string
		apply      bool
		othersZero bool
	)
	fs := flag.NewFlagSet("import-keepers", flag.ContinueOnError)
	fs.StringVar(&file, "file", "", "path to CSV file with keepers")
	fs.BoolVar(&apply, "apply", false, "apply changes (default is dry-run)")
	fs.BoolVar(&othersZero, "others-zero", false, "set is_wicket_keeper=0 for players not in CSV")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if file == "" {
		return options{}, errors.New("missing required --file")
	}
	return options{
		file:       file,
		apply:      apply,
		othersZero: othersZero,
	}, nil
}
