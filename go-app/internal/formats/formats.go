package formats

import (
	"fmt"
	"strings"
)

// Canonical format codes.
const (
	CodeTest = "TEST"
	CodeODI  = "ODI"
	CodeT20  = "T20"
	CodeT20I = "T20I"
)

// Numeric IDs aligned with seed order from migrations (0004_format_dimension.sql):
// 1=TEST, 2=ODI, 3=T20, 4=T20I.
const (
	IDTest = 1
	IDODI  = 2
	IDT20  = 3
	IDT20I = 4
)

// NormalizeCode trims and upper-cases the input code.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IDForCode maps a canonical format code to its numeric ID.
// Supported codes: TEST, ODI, T20, T20I (case-insensitive).
func IDForCode(code string) (int, error) {
	switch NormalizeCode(code) {
	case CodeT20:
		return IDT20, nil
	case CodeT20I:
		return IDT20I, nil
	case CodeODI:
		return IDODI, nil
	case CodeTest:
		return IDTest, nil
	default:
		return 0, fmt.Errorf("unknown format code: %s", code)
	}
}

// MapFormatIDs maps a (possibly empty) format code to a slice of numeric ids
// used in queries. T20 and T20I are treated as the same bucket for most
// aggregate calculations, hence both ids are returned for those codes.
//
// Rules:
//   - "", "T20", or "T20I" => {3,4}
//   - "ODI" => {2}
//   - "TEST" => {1}
//   - Any other/unrecognized => {3,4}
func MapFormatIDs(code string) []int {
	switch NormalizeCode(code) {
	case "", CodeT20, CodeT20I:
		return []int{IDT20, IDT20I}
	case CodeODI:
		return []int{IDODI}
	case CodeTest:
		return []int{IDTest}
	default:
		return []int{IDT20, IDT20I}
	}
}
