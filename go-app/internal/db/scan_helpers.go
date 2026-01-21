package db

import "database/sql"

// toNullString converts a plain string to sql.NullString, marking empty strings as invalid.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// toNullFloat64 wraps a float64 value into sql.NullFloat64 as a valid value.
func toNullFloat64(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

// fromNullString returns the string value if valid, otherwise an empty string.
func fromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
