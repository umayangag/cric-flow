package importretired

import "time"

// Options contains CLI inputs for the import-retired command.
type Options struct {
	File       string
	Apply      bool
	OthersZero bool
	Timeout    time.Duration
}
