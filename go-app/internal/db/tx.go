package db

// ErrPoolNotInitialized is returned when Pool is nil.
var ErrPoolNotInitialized = fmtError("db pool not initialized")

type fmtError string

func (e fmtError) Error() string { return string(e) }
