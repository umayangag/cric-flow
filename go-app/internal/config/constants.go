package config

import "time"

// DefaultTimeout is the default timeout for long-running CLI operations (imports, precompute, etc.).
// Set to 1 year so runs effectively have no deadline; use -timeout to cap (e.g. -timeout=5h).
const DefaultTimeout = 7 * 24 * time.Hour
