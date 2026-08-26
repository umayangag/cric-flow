//go:build linux || darwin

package dataacquire

import (
	"math"
	"syscall"
)

// FreeSpace reports the bytes available to an unprivileged writer at path.
//
// Checking before starting is the difference between "refused, 3 GiB short" and a
// half-written archive plus a full disk that also breaks everything else on the box.
// Bavail rather than Bfree: the blocks reserved for root are not ours to fill.
func FreeSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail and Bsize are platform-typed (uint64 on Linux, uint64/int32 on Darwin),
	// so the arithmetic is done in uint64 and clamped rather than converted blind.
	// A filesystem reporting more free space than an int64 can hold is not a case
	// worth modelling; saturating says "plenty" without wrapping to a negative that
	// the caller would read as "no room".
	avail := uint64(stat.Bavail) * uint64(stat.Bsize) //nolint:gosec // widening, then clamped below
	if avail > math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return int64(avail), nil
}
