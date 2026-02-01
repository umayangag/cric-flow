package exportdataset

import (
	"errors"
	"flag"
	"os"
	"sort"
	"strings"
)

// Options captures CLI options for export-dataset.
// Only CLI-derived options live here; config/env merging can happen in a higher layer.
type Options struct {
	OutDir        string
	Formats       []string // empty => caller may decide defaults
	Unified       bool
	InferenceOnly bool
	EnableSeq     bool // gate sequence feature columns in exporter
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var outDir string
	var format string
	var formats string
	var allFormats bool
	var unified bool
	var inferenceOnly bool
	var enableSeq bool

	defOut := os.Getenv("GO_APP_OUTPUT_DIR")
	// Default for enable-seq from env; accepted truthy values: 1, true, yes (case-insensitive)
	defSeqEnv := os.Getenv("ENABLE_SEQ_FEATURES")
	defEnableSeq := defSeqEnv == "1" || strings.EqualFold(defSeqEnv, "true") || strings.EqualFold(defSeqEnv, "yes")

	fs.StringVar(&outDir, "out", defOut, "output directory for exported CSVs")
	fs.StringVar(&format, "format", "", "single format code (TEST, ODI, T20, T20I). Aliases: MDM→TEST, ODM→ODI, IT20→T20I")
	fs.StringVar(&formats, "formats", "", "comma-separated list of format codes (aliases supported: MDM, ODM, IT20)")
	fs.BoolVar(&allFormats, "all-formats", false, "export for all formats")
	fs.BoolVar(&unified, "unified", false, "export single merged CSV per task across all formats")
	fs.BoolVar(&inferenceOnly, "inference-only", false, "emit inputs-only CSVs for inference")
	fs.BoolVar(
		&enableSeq,
		"enable-seq",
		defEnableSeq,
		"enable sequence feature columns (can also set ENABLE_SEQ_FEATURES=1)",
	)

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	list, err := computeFormats(allFormats, formats, format)
	if err != nil {
		return Options{}, err
	}

	return Options{
		OutDir:        outDir,
		Formats:       list,
		Unified:       unified,
		InferenceOnly: inferenceOnly,
		EnableSeq:     enableSeq,
	}, nil
}

func computeFormats(allFormats bool, formatsCSV, single string) ([]string, error) {
	if allFormats {
		return []string{"TEST", "ODI", "T20", "T20I"}, nil
	}
	if formatsCSV != "" {
		items := splitAndNorm(formatsCSV)
		if len(items) == 0 {
			return nil, errors.New("formats flag provided but empty after parsing")
		}
		return items, nil
	}
	if single != "" {
		return splitAndNorm(single), nil
	}
	return nil, nil // let caller decide defaults (config or legacy behavior)
}

func splitAndNorm(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.ToUpper(strings.TrimSpace(p))
		if v != "" {
			out = append(out, v)
		}
	}
	// Keep order stable but remove duplicates
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(out))
	for _, v := range out {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			uniq = append(uniq, v)
		}
	}
	// For deterministic tests, sort when multiple came from CSV
	if len(uniq) > 1 && strings.Contains(s, ",") {
		sort.Strings(uniq)
	}
	return uniq
}
