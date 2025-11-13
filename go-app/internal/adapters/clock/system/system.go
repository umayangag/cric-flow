// Package system provides a real-time Clock implementation.
package system

import (
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/clock"
)

// Clock implements clock.Clock using the system time.
type Clock struct{}

var _ clock.Clock = (*Clock)(nil)

// Now returns the current time.
func (Clock) Now() time.Time { return time.Now() }
