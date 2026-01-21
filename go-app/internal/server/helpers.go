package server

import "database/sql"

// sqlNullString converts a plain string into sql.NullString, marking it invalid when empty.
func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// sqlNullFloat64 wraps a float64 into a valid sql.NullFloat64.
func sqlNullFloat64(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

// nullString extracts the string value from sql.NullString, returning empty when invalid.
func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
