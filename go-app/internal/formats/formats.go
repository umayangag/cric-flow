package formats

import (
	"fmt"
	"log/slog"
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

// CanonicalizeCode maps alternative/alias codes to the canonical ones we use in the system.
// Supported aliases:
//   - MDM  -> TEST  (Multi-Day Match)
//   - ODM  -> ODI   (One Day Match)
//   - IT20 -> T20I  (International T20)
//
// The function also uppercases and trims the input.
func CanonicalizeCode(code string) string {
	normalized := NormalizeCode(code)
	switch normalized {
	case "MDM":
		return CodeTest
	case "ODM":
		return CodeODI
	case "IT20":
		return CodeT20I
	default:
		return normalized
	}
}

// IDForCode maps a canonical format code to its numeric ID.
// Supported codes: TEST, ODI, T20, T20I (case-insensitive).
func IDForCode(code string) (int, error) {
	switch CanonicalizeCode(code) {
	case CodeT20:
		return IDT20, nil
	case CodeT20I:
		return IDT20I, nil
	case CodeODI:
		return IDODI, nil
	case CodeTest:
		return IDTest, nil
	default:
		err := fmt.Errorf("unknown format code: %s", code)
		slog.Error("formats.IDForCode failed", slog.String("code", code), slog.Any("err", err))
		return 0, err
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
	switch CanonicalizeCode(code) {
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

// FormatHierarchyNode represents a node in the format hierarchy.
type FormatHierarchyNode struct {
	Code     string                `json:"code"`
	Name     string                `json:"name"`
	Children []FormatHierarchyNode `json:"children,omitempty"`
}

// GetHierarchy returns the structural hierarchy of match types as used in the system.
// This matches the logic in CanonicalizeCode and MapFormatIDs.
func GetHierarchy() []FormatHierarchyNode {
	return []FormatHierarchyNode{
		{
			Code: "MDM",
			Name: "Multi-Day Match",
			Children: []FormatHierarchyNode{
				{Code: CodeTest, Name: "Test Matches"},
			},
		},
		{
			Code: "ODM",
			Name: "One Day Match",
			Children: []FormatHierarchyNode{
				{Code: CodeODI, Name: "One Day International"},
			},
		},
		{
			Code: CodeT20,
			Name: "T20 (Bucket)",
			Children: []FormatHierarchyNode{
				{Code: CodeT20, Name: "Domestic T20"},
				{
					Code: CodeT20I,
					Name: "T20 International",
					Children: []FormatHierarchyNode{
						{Code: "IT20", Name: "International T20"},
					},
				},
			},
		},
	}
}
