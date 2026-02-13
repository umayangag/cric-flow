package importkeepers

import (
	"time"
)

// Options holds CLI flags for import-keepers.
type Options struct {
	File       string
	Apply      bool
	OthersZero bool
	Timeout    time.Duration
}
