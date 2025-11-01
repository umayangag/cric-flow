package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds directory defaults for go-app commands.
type Config struct {
	Inputs struct {
		CricsheetDir string `json:"cricsheet_dir"`
		EtlDir       string `json:"etl_dir"`
	} `json:"inputs"`
	Outputs struct {
		ExportDir string `json:"export_dir"`
	} `json:"outputs"`
	Formats struct {
		TreatT20ISubset    bool     `json:"treat_t20i_as_subset"`
		InternationalTeams []string `json:"international_teams"`
	} `json:"formats"`
	Features struct {
		PrecomputeTimeoutMs  int     `json:"precompute_timeout_ms"`
		MinBattingInnings    int     `json:"min_batting_innings"`
		MinBowlingInnings    int     `json:"min_bowling_innings"`
		FormShrinkageAlpha   float32 `json:"form_shrinkage_alpha"`
		ConsistencyPerFormat bool    `json:"consistency_per_format"`
	} `json:"features"`
	Export struct {
		SplitByFormat  bool   `json:"split_by_format"`
		RequiredFormat string `json:"required_format"`
	} `json:"export"`
	Team struct {
		MinBowlers     int `json:"min_bowlers"`
		DefaultBatters int `json:"default_batters"`
		DefaultBowlers int `json:"default_bowlers"`
	} `json:"team"`
}

var cached *Config

// Load reads config.json from the current working directory if present.
// It is safe to call multiple times; the result is cached for the process lifetime.
func Load() *Config {
	if cached != nil {
		return cached
	}
	cfg := &Config{}
	// Search locations (in order):
	//  1) GO_APP_CONFIG env var path
	//  2) ./config.json (CWD)
	//  3) ../config.json (if executed from a subdir)
	//  4) go-app/config.json when running from a cmd subdir
	if p := os.Getenv("GO_APP_CONFIG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
			cached = cfg
			return cfg
		}
	}
	candidates := []string{
		"config.json",
		filepath.Join("..", "config.json"),
		filepath.Join("..", "..", "config.json"),
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
			cached = cfg
			return cfg
		}
	}
	cached = cfg
	return cfg
}

// DefaultCricsheetDir returns the configured cricsheet input dir or a sensible built-in default.
func DefaultCricsheetDir() string {
	cfg := Load()
	if cfg != nil && cfg.Inputs.CricsheetDir != "" {
		return cfg.Inputs.CricsheetDir
	}
	return filepath.Join("..", "data", "go-app", "cricsheet")
}

// DefaultEtlDir returns the configured curated CSV dir for ETL importer or a built-in default.
func DefaultEtlDir() string {
	cfg := Load()
	if cfg != nil && cfg.Inputs.EtlDir != "" {
		return cfg.Inputs.EtlDir
	}
	return filepath.Join("..", "data", "go-app", "createdb")
}

// DefaultExportDir returns the configured export output dir or a built-in default.
func DefaultExportDir() string {
	cfg := Load()
	if cfg != nil && cfg.Outputs.ExportDir != "" {
		return cfg.Outputs.ExportDir
	}
	return filepath.Join("..", "output", "go-app")
}
