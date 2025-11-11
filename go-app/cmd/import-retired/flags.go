package main

import (
	"errors"
	"flag"
	"time"
)

// options holds CLI flags for import-retired.
type options struct {
	file       string
	apply      bool
	othersZero bool
	timeout    time.Duration
}

// parseFlags parses CLI args into options. Pure and testable.
func parseFlags(args []string) (options, error) {
	var (
		file       string
		apply      bool
		othersZero bool
		timeout    time.Duration
	)
	fs := flag.NewFlagSet("import-retired", flag.ContinueOnError)
	fs.StringVar(&file, "file", "", "CSV file path (required)")
	fs.BoolVar(&apply, "apply", false, "apply changes (default is dry-run)")
	fs.BoolVar(&othersZero, "others-zero", false, "set is_retired=0 for players not listed in CSV")
	fs.DurationVar(&timeout, "timeout", 60*time.Second, "operation timeout")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if file == "" {
		return options{}, errors.New("missing required --file")
	}
	return options{file: file, apply: apply, othersZero: othersZero, timeout: timeout}, nil
}
