// Package clock defines a minimal time source interface.
package clock

import "time"

// Clock provides current time; useful for deterministic tests.
//
//go:generate mockery --name Clock --output internal/mocks --case underscore
type Clock interface {
	Now() time.Time
}
