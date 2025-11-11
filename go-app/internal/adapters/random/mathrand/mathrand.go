// Package mathrand provides a Random implementation backed by math/rand.
package mathrand

import (
	mrand "math/rand"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/random"
)

// RNG wraps a math/rand.Rand to satisfy the random.Random interface.
type RNG struct{ r *mrand.Rand }

var _ random.Random = (*RNG)(nil)

// New returns a new RNG using the provided *rand.Rand.
func New(r *mrand.Rand) *RNG { return &RNG{r: r} }

// Intn returns, as an int, a non-negative pseudo-random number in [0,n).
func (g *RNG) Intn(n int) int { return g.r.Intn(n) }

// Seed uses the provided seed value to initialize the generator.
func (g *RNG) Seed(seed int64) { g.r.Seed(seed) }
