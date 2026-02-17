// Package config provides application configuration loading and access helpers.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
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
		// Feature extraction (EWM form, consistency). Used by export/training-data and precompute when not overridden by CLI.
		EWMAlpha         float64 `json:"ewm_alpha"`          // (0,1]; default 0.3
		EWMAlphaShort    float64 `json:"ewm_alpha_short"`    // for form_short (more recent); default 0.5
		EWMAlphaLong     float64 `json:"ewm_alpha_long"`     // for form_long (longer horizon); default 0.2
		ConsistencyLastN int     `json:"consistency_last_n"` // last-N innings for consistency; default 10
		FormWindowN      int     `json:"form_window_n"`      // max innings for form (0 = no limit); default 0
		MomentumLastN    int     `json:"momentum_last_n"`    // last-N innings for momentum slope; default 5
		FieldingEnrich   struct {
			EWMAlpha           float64 `json:"ewm_alpha"`             // EWM alpha for fielding form fallback; default 0.3
			FormToCatchesRatio float64 `json:"form_to_catches_ratio"` // split of form into catches (rest = run_outs); default 0.7
		} `json:"fielding_enrich"`
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
		DefaultPoolCSV       string                     `json:"default_pool_csv"`
		RequireKeeper        bool                       `json:"require_keeper"`
		ScoreWeights         *ScoreWeights              `json:"score_weights"`
		ScoreNormalization   map[string]ScoreNormParams `json:"score_normalization"`
		ScoreWeightsByFormat map[string]ScoreWeights    `json:"score_weights_by_format"`
		MetaModelPath        string                     `json:"meta_model_path"` // JSON from ml.train_combination_meta
	} `json:"selection"`
}

// ScoreNormParams holds format-specific divisors for normalizing raw predictions to [0,1].
// BatDivisor: typical max runs per player; runs/divisor caps at 1.
// WicketDivisor: typical max wickets per player.
// EconBase: economy above which contribution is 0; lower economy = higher score.
// FieldDivisor: typical max (catches + run_outs*1.5).
type ScoreNormParams struct {
	BatDivisor    float64 `json:"bat_divisor"`
	WicketDivisor float64 `json:"wicket_divisor"`
	EconBase      float64 `json:"econ_base"`
	FieldDivisor  float64 `json:"field_divisor"`
}

// ScoreWeights defines relative weights for combining batting/bowling/fielding signals in team selection.
type ScoreWeights struct {
	Bat         float64 `json:"bat"`          // default 0.45
	Bowl        float64 `json:"bowl"`         // default 0.40
	Field       float64 `json:"field"`        // default 0.10
	KeeperBonus float64 `json:"keeper_bonus"` // default 0.02
}

var (
	cached     *Config
	loadedFrom string // path of config file loaded; empty if none found
)

// Load reads config.json from the current working directory if present.
// It is safe to call multiple times; the result is cached for the process lifetime.
func Load() *Config {
	if cached != nil {
		return cached
	}
	cfg := &Config{}
	loadedFrom = ""
	if p := os.Getenv("GO_APP_CONFIG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
			cached = cfg
			loadedFrom = p
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
			loadedFrom = p
			return cfg
		}
	}
	cached = cfg
	return cfg
}

// ValidateForServer returns an error if config is missing or invalid for the API server.
// Call at server startup; exit on error rather than using fallback defaults.
func ValidateForServer() error {
	cfg := Load()
	if loadedFrom == "" {
		err := fmt.Errorf("config file not found: set GO_APP_CONFIG or ensure config.json exists (CWD, .., or ../..)")
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	if cfg.Features.PrecomputeTimeoutMs < 0 {
		err := fmt.Errorf(
			"features.precompute_timeout_ms must be >= 0 (0 = no timeout); got %d",
			cfg.Features.PrecomputeTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	return nil
}

// PipelineTimeout returns the timeout for long-running pipeline jobs (import, precompute, export).
// Uses features.precompute_timeout_ms. 0 = no timeout.
func PipelineTimeout() time.Duration {
	cfg := Load()
	if cfg == nil || cfg.Features.PrecomputeTimeoutMs <= 0 {
		return 0
	}
	return time.Duration(cfg.Features.PrecomputeTimeoutMs) * time.Millisecond
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

// EffectiveScoreNormParams returns format-specific normalization divisors for score computation.
// Falls back to defaults when format is not configured.
func EffectiveScoreNormParams(cfg *Config, format string) (batDiv, wicketDiv, econBase, fieldDiv float64) {
	batDiv = DefaultScoreNormBatDivisor
	wicketDiv = DefaultScoreNormWicketDivisor
	econBase = DefaultScoreNormEconBase
	fieldDiv = DefaultScoreNormFieldDivisor
	if cfg != nil && len(cfg.Selection.ScoreNormalization) > 0 {
		if p, ok := cfg.Selection.ScoreNormalization[format]; ok && (p.BatDivisor > 0 || p.WicketDivisor > 0) {
			if p.BatDivisor > 0 {
				batDiv = p.BatDivisor
			}
			if p.WicketDivisor > 0 {
				wicketDiv = p.WicketDivisor
			}
			if p.EconBase > 0 {
				econBase = p.EconBase
			}
			if p.FieldDivisor > 0 {
				fieldDiv = p.FieldDivisor
			}
		}
	}
	return batDiv, wicketDiv, econBase, fieldDiv
}

// metaModelWeights holds parsed coefficients from train_combination_meta JSON.
type metaModelWeights struct {
	Bat         float64                    `json:"bat"`
	Bowl        float64                    `json:"bowl"`
	Field       float64                    `json:"field"`
	KeeperBonus float64                    `json:"keeper_bonus"`
	PerFormat   map[string]metaModelWeights `json:"per_format"`
}

var (
	metaModelCache  *metaModelWeights
	metaModelPath   string
	metaModelLoadMu sync.Mutex
)

func loadMetaModel(cfg *Config) *metaModelWeights {
	path := ""
	if cfg != nil && cfg.Selection.MetaModelPath != "" {
		path = cfg.Selection.MetaModelPath
	}
	if path == "" {
		return nil
	}
	metaModelLoadMu.Lock()
	defer metaModelLoadMu.Unlock()
	if metaModelPath == path && metaModelCache != nil {
		return metaModelCache
	}
	abs := path
	if !filepath.IsAbs(path) {
		cwd, _ := os.Getwd()
		abs = filepath.Join(cwd, path)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		slog.Warn("config.loadMetaModel failed", "path", abs, "err", err)
		metaModelPath = ""
		metaModelCache = nil
		return nil
	}
	var m metaModelWeights
	if err := json.Unmarshal(b, &m); err != nil {
		slog.Warn("config.loadMetaModel unmarshal failed", "path", abs, "err", err)
		metaModelPath = ""
		metaModelCache = nil
		return nil
	}
	metaModelPath = path
	metaModelCache = &m
	return metaModelCache
}

// EffectiveScoreWeightsForFormat returns score weights for the given format, with per-format override when configured.
func EffectiveScoreWeightsForFormat(cfg *Config, format string) (bat, bowl, field, keeperBonus float64) {
	// 1. Meta-model (learned from backtest) takes precedence when configured
	if meta := loadMetaModel(cfg); meta != nil {
		if len(meta.PerFormat) > 0 {
			if w, ok := meta.PerFormat[format]; ok {
				bat, bowl, field, keeperBonus = w.Bat, w.Bowl, w.Field, w.KeeperBonus
				if bat > 0 || bowl > 0 {
					if bat == 0 {
						bat = DefaultScoreWeightBat
					}
					if bowl == 0 {
						bowl = DefaultScoreWeightBowl
					}
					if field == 0 {
						field = DefaultScoreWeightField
					}
					return bat, bowl, field, keeperBonus
				}
			}
		}
		if meta.Bat > 0 || meta.Bowl > 0 {
			bat, bowl, field, keeperBonus = meta.Bat, meta.Bowl, meta.Field, meta.KeeperBonus
			if bat == 0 {
				bat = DefaultScoreWeightBat
			}
			if bowl == 0 {
				bowl = DefaultScoreWeightBowl
			}
			if field == 0 {
				field = DefaultScoreWeightField
			}
			return bat, bowl, field, keeperBonus
		}
	}
	// 2. Config score_weights_by_format
	if cfg != nil && len(cfg.Selection.ScoreWeightsByFormat) > 0 {
		if w, ok := cfg.Selection.ScoreWeightsByFormat[format]; ok && (w.Bat > 0 || w.Bowl > 0) {
			bat = w.Bat
			bowl = w.Bowl
			field = w.Field
			keeperBonus = w.KeeperBonus
			if bat > 0 || bowl > 0 {
				if bat == 0 {
					bat = DefaultScoreWeightBat
				}
				if bowl == 0 {
					bowl = DefaultScoreWeightBowl
				}
				if field == 0 {
					field = DefaultScoreWeightField
				}
				return bat, bowl, field, keeperBonus
			}
		}
	}
	return EffectiveScoreWeights(cfg)
}

// EffectiveScoreWeights returns the configured score weights or built-in defaults.
func EffectiveScoreWeights(cfg *Config) (bat, bowl, field, keeperBonus float64) {
	if cfg != nil && cfg.Selection.ScoreWeights != nil {
		w := cfg.Selection.ScoreWeights
		bat = w.Bat
		bowl = w.Bowl
		field = w.Field
		keeperBonus = w.KeeperBonus
	}
	if bat == 0 {
		bat = DefaultScoreWeightBat
	}
	if bowl == 0 {
		bowl = DefaultScoreWeightBowl
	}
	if field == 0 {
		field = DefaultScoreWeightField
	}
	if keeperBonus == 0 {
		keeperBonus = DefaultScoreWeightKeeperBonus
	}
	return bat, bowl, field, keeperBonus
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
		err := fmt.Errorf("min bowlers must be >= 1")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// Default extras cannot be negative when provided.
	if cfg.Predictor.DefaultExtras < 0 {
		err := fmt.Errorf("extras must be non-negative")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// Default batters cannot be negative.
	if cfg.Team.DefaultBatters < 0 {
		err := fmt.Errorf("default batters must be >= 0")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// If both TeamSize and MinBowlers are provided, TeamSize must be >= MinBowlers.
	if cfg.Predictor.TeamSize > 0 && cfg.Team.MinBowlers > 0 && cfg.Predictor.TeamSize < cfg.Team.MinBowlers {
		err := fmt.Errorf("team size must be >= min bowlers")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// If DefaultBowlers is set, it must be >= MinBowlers.
	if cfg.Team.DefaultBowlers > 0 && cfg.Team.MinBowlers > 0 && cfg.Team.DefaultBowlers < cfg.Team.MinBowlers {
		err := fmt.Errorf("default bowlers must be >= min bowlers")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	return nil
}
