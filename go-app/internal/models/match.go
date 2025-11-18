package models

// Match is a minimal representation of a parsed match used by the importer.
// Keep this decoupled from DB/storage models so tests can construct values easily.
type Match struct {
	ID     int64
	Format string
	Season string
	Teams  []string
}
