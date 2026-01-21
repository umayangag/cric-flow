package evaluate

import "errors"

// validateNonEmpty ensures a flag value is non-empty.
func validateNonEmpty(value, name string) error {
	if value == "" {
		return errors.New(name + " must not be empty")
	}
	return nil
}
