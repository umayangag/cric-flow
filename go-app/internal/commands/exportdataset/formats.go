package exportdataset

import (
	"strings"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// ResolveFormats returns the list of formats to process for the export-dataset command,
// preserving legacy behavior exactly while centralizing the logic for testing.
//
// Precedence:
//  1) CLI flags (already normalized by cli.ParseArgs) when provided.
//  2) Config.Export.RequiredFormat (trimmed, uppercased) if non-empty.
//  3) Config.Export.SplitByFormat == true → all formats [TEST, ODI, T20, T20I].
//  4) Legacy fallback: combined export represented by a single empty string: [""].
//
// The function is pure/deterministic: it does not access I/O and does not mutate inputs.
func ResolveFormats(opts cli.Options, cfg *config.Config) []string {
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
		if cfg.Export.SplitByFormat {
			return []string{"TEST", "ODI", "T20", "T20I"}
		}
	}
	// Legacy combined behavior: single unsuffixed files using all formats combined.
	return []string{""}
}
