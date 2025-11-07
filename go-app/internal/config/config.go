// Package config provides application configuration loading and access helpers.
package config

import (
	"encoding/json"
	"fmt"
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
		HistoryWindowMatches int     `json:"history_window_matches"`
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
	Weather struct {
		Enabled           bool     `json:"enabled"`
		RateLimitPerSec   int      `json:"rate_limit_per_sec"`
		MaxAttempts       int      `json:"max_attempts"`
		GeocodeCacheOnly  bool     `json:"geocode_cache_only"`
		OverwriteExisting bool     `json:"overwrite_existing"`
		WhitelistVenues   []string `json:"whitelist_venues"`
		Mocks             struct {
			Temp      int `json:"temp"`
			Wind      int `json:"wind"`
			Rain      int `json:"rain"`
			Humidity  int `json:"humidity"`
			Cloud     int `json:"cloud"`
			Pressure  int `json:"pressure"`
			Viscosity int `json:"viscosity"`
			Session   int `json:"session"`
		} `json:"mocks"`
	} `json:"weather"`
	Predictor struct {
		TeamSize      int     `json:"team_size"`
		DefaultExtras float64 `json:"default_extras"`
	} `json:"predictor"`
	Selection struct {
		DefaultPoolCSV string `json:"default_pool_csv"`
		RequireKeeper  bool   `json:"require_keeper"`
	} `json:"selection"`
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

// ValidateTeamSettings validates a subset of team/predictor settings for sanity.
// It is a pure helper and does not perform any I/O. It does not modify cfg.
// Only non-zero values are validated for cross-field constraints to avoid
// over-constraining partially-specified configs. Callers may enforce stricter
// policies as needed at the edges (e.g., in main).
func ValidateTeamSettings(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	// Min bowlers must be at least 1.
	if cfg.Team.MinBowlers < 1 {
		return fmt.Errorf("min bowlers must be >= 1")
	}
	// Default extras cannot be negative when provided.
	if cfg.Predictor.DefaultExtras < 0 {
		return fmt.Errorf("extras must be non-negative")
	}
	// Default batters cannot be negative.
	if cfg.Team.DefaultBatters < 0 {
		return fmt.Errorf("default batters must be >= 0")
	}
	// If both TeamSize and MinBowlers are provided, TeamSize must be >= MinBowlers.
	if cfg.Predictor.TeamSize > 0 && cfg.Team.MinBowlers > 0 && cfg.Predictor.TeamSize < cfg.Team.MinBowlers {
		return fmt.Errorf("team size must be >= min bowlers")
	}
	// If DefaultBowlers is set, it must be >= MinBowlers.
	if cfg.Team.DefaultBowlers > 0 && cfg.Team.MinBowlers > 0 && cfg.Team.DefaultBowlers < cfg.Team.MinBowlers {
		return fmt.Errorf("default bowlers must be >= min bowlers")
	}
	return nil
}
