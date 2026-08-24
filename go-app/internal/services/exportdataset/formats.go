package exportdataset

import (
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
)

// ResolveFormats returns the list of formats to process for the export-dataset command.
//
// Precedence:
//  1. CLI flags (already normalized by cli.ParseArgs) when provided.
//  2. Config.Export.RequiredFormat (trimmed, uppercased) if non-empty.
//  3. Every canonical format.
//
// Exports are always per-format. The combined unsuffixed CSVs this used to fall back to
// existed only to feed ml.train_batting_model / train_bowling_model, removed in C3-1.
//
// The function is pure/deterministic: it does not access I/O and does not mutate inputs.
func ResolveFormats(opts Options, cfg *config.Config) []string {
	if len(opts.Formats) > 0 {
		// CLI precedence; return a copy to avoid accidental mutation by callers.
		out := make([]string, len(opts.Formats))
		copy(out, opts.Formats)
		return out
	}
	// No CLI formats; fall back to config.
	var required string
	if cfg != nil {
		required = strings.ToUpper(strings.TrimSpace(cfg.Export.RequiredFormat))
		if required != "" {
			return []string{required}
		}
	}
	return formatsPkg.CanonicalCodes()
}
