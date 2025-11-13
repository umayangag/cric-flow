// Package random defines a minimal RNG interface for deterministic testing.
package random

// Random is a minimal RNG interface.
//
//go:generate mockery --name Random --output internal/mocks --case underscore
type Random interface {
	Intn(n int) int
	Seed(seed int64)
}
