//go:build !linux && !darwin

package dataacquire

import "errors"

// FreeSpace has no portable implementation outside the platforms this service is
// built for. Reporting it as unavailable rather than guessing keeps the precheck
// honest: Options.checkSpace logs and proceeds, it does not invent a number.
func FreeSpace(string) (int64, error) {
	return 0, errors.New("free space is not reportable on this platform")
}
