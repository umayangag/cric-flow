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

// MustIDForCode is like IDForCode but panics on error. Prefer IDForCode unless you are sure.
func MustIDForCode(code string) int {
	id, err := IDForCode(code)
	if err != nil {
		panic(err)
	}
	return id
}

// CodeForID maps a numeric ID to the canonical code. Returns empty string if unknown.
func CodeForID(id int) string {
	switch id {
	case IDT20:
		return CodeT20
	case IDT20I:
		return CodeT20I
	case IDODI:
		return CodeODI
	case IDTest:
		return CodeTest
	default:
		return ""
	}
}

// IsLimitedOvers returns true for ODI and T20/T20I.
func IsLimitedOvers(codeOrID any) bool {
	switch v := codeOrID.(type) {
	case string:
		c := NormalizeCode(v)
		return c == CodeODI || c == CodeT20 || c == CodeT20I
	case int:
		return v == IDODI || v == IDT20 || v == IDT20I
	default:
		return false
	}
}

// IsInternational returns true for formats that are international by definition (T20I).
// Note: ODI and Test are always internationals in many datasets, but keep conservative.
func IsInternational(codeOrID any) bool {
	switch v := codeOrID.(type) {
	case string:
		return NormalizeCode(v) == CodeT20I
	case int:
		return v == IDT20I
	default:
		return false
	}
}
