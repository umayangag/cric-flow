package scanx

import (
	"fmt"
	"strings"
)

// RowScanner is the minimal subset used to iterate and scan rows.
// It intentionally mirrors the methods we rely on from pgx.Rows / db.Rows,
// without importing those packages to avoid cyclic dependencies.
type RowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close()
}

// CountReturningOnes consumes rows produced by queries like
// "... RETURNING 1" and returns the number of affected rows.
// It closes the rows before returning.
func CountReturningOnes(rows RowScanner) (int64, error) {
	if rows == nil {
		return 0, nil
	}
	defer rows.Close()
	var affected int64
	for rows.Next() {
		var one int
		if err := rows.Scan(&one); err != nil {
			return 0, err
		}
		affected++
	}
	return affected, nil
}

// Scanner is a minimal interface for scanning the current row's columns into destinations.
// It is satisfied by pgx.Rows and database/sql.Rows during a Next() iteration.
// We keep it tiny to avoid direct dependency on a specific rows type in callers.
type Scanner interface {
	Scan(dest ...any) error
}

// ScanToStrings scans the current row into a slice of strings using AnyToString conversion
// rules. The caller must have already advanced the row cursor (e.g., via rows.Next()).
func ScanToStrings(s Scanner, n int) ([]string, error) {
	if n < 0 {
		return nil, fmt.Errorf("n must be >= 0, got %d", n)
	}
	dests := make([]any, n)
	for i := 0; i < n; i++ {
		var v any
		dests[i] = &v
	}
	if err := s.Scan(dests...); err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		v := *dests[i].(*any)
		out[i] = AnyToString(v)
	}
	return out, nil
}

// AnyToString converts common DB scalar types to their string representation,
// matching the legacy export behavior used by the project.
func AnyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return fmt.Sprintf("%d", t)
	case int32:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	case float32:
		return TrimFloat(fmt.Sprintf("%g", float64(t)))
	case float64:
		return TrimFloat(fmt.Sprintf("%g", t))
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// TrimFloat normalizes a float string, removing trailing zeros and a trailing dot.
// Examples: "1.000000" -> "1", "3.140000" -> "3.14".
func TrimFloat(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
