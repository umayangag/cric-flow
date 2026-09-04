package config

import (
	"os"
	"path/filepath"
)

// Server helpers (timeouts in seconds/minutes; 0 = use default from constants).
func ServerMLHealthTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.MLHealthTimeoutSec > 0 {
		return cfg.Server.MLHealthTimeoutSec
	}
	return DefaultServerMLHealthTimeoutSec
}

func ServerMLClientTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.MLClientTimeoutSec > 0 {
		return cfg.Server.MLClientTimeoutSec
	}
	return DefaultServerMLClientTimeoutSec
}

func ServerMLHealthBodyLimitBytes(cfg *Config) int {
	if cfg != nil && cfg.Server.MLHealthBodyLimitBytes > 0 {
		return cfg.Server.MLHealthBodyLimitBytes
	}
	return DefaultServerMLHealthBodyLimitBytes
}

func ServerMLBaseURLFallback(cfg *Config) string {
	if cfg != nil && cfg.Server.MLBaseURLFallback != "" {
		return cfg.Server.MLBaseURLFallback
	}
	return "http://localhost:8000"
}

func ServerReadinessTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.ReadinessTimeoutSec > 0 {
		return cfg.Server.ReadinessTimeoutSec
	}
	return DefaultServerReadinessTimeoutSec
}

func ServerTrainStepTimeoutMin(cfg *Config) int {
	if cfg != nil && cfg.Server.TrainStepTimeoutMin > 0 {
		return cfg.Server.TrainStepTimeoutMin
	}
	return DefaultServerTrainStepTimeoutMin
}

func ServerPipelineProgressSec(cfg *Config) int {
	if cfg != nil && cfg.Server.PipelineProgressSec > 0 {
		return cfg.Server.PipelineProgressSec
	}
	return DefaultServerPipelineProgressSec
}

func ServerDBProbeTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.DBProbeTimeoutSec > 0 {
		return cfg.Server.DBProbeTimeoutSec
	}
	return DefaultServerDBProbeTimeoutSec
}

func ServerDBProbeLongTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.DBProbeLongTimeoutSec > 0 {
		return cfg.Server.DBProbeLongTimeoutSec
	}
	return DefaultServerDBProbeLongTimeoutSec
}

func ServerArtifactsTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.ArtifactsTimeoutSec > 0 {
		return cfg.Server.ArtifactsTimeoutSec
	}
	return DefaultServerArtifactsTimeoutSec
}

func ServerHTTPReadTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPReadTimeoutSec > 0 {
		return cfg.Server.HTTPReadTimeoutSec
	}
	return DefaultServerHTTPReadTimeoutSec
}

func ServerHTTPWriteTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPWriteTimeoutSec > 0 {
		return cfg.Server.HTTPWriteTimeoutSec
	}
	return DefaultServerHTTPWriteTimeoutSec
}

func ServerHTTPIdleTimeoutSec(cfg *Config) int {
	if cfg != nil && cfg.Server.HTTPIdleTimeoutSec > 0 {
		return cfg.Server.HTTPIdleTimeoutSec
	}
	return DefaultServerHTTPIdleTimeoutSec
}

func ServerListenAddress(cfg *Config) string {
	if cfg != nil && cfg.Server.ListenAddress != "" {
		return cfg.Server.ListenAddress
	}
	return DefaultServerListenAddress
}

// Ops pagination helpers.
func OpsMigrationsPageDefault(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageDefault > 0 {
		return cfg.Ops.MigrationsPageDefault
	}
	return DefaultOpsMigrationsPageDefault
}

func OpsMigrationsPageMax(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageMax > 0 {
		return cfg.Ops.MigrationsPageMax
	}
	return DefaultOpsMigrationsPageMax
}

func OpsMigrationsPageCap(cfg *Config) int {
	if cfg != nil && cfg.Ops.MigrationsPageCap > 0 {
		return cfg.Ops.MigrationsPageCap
	}
	return DefaultOpsMigrationsPageCap
}

func OpsRecentMigrationsCount(cfg *Config) int {
	if cfg != nil && cfg.Ops.RecentMigrationsCount > 0 {
		return cfg.Ops.RecentMigrationsCount
	}
	return DefaultRecentMigrations
}

// SelectionMaxWinProbEvalBudget caps how many XIs ml-service may score per side per round
// of the win-probability search.
func SelectionMaxWinProbEvalBudget(cfg *Config) int {
	if cfg != nil && cfg.Selection.MaxWinProbEvalBudget > 0 {
		return cfg.Selection.MaxWinProbEvalBudget
	}
	return DefaultSelectionMaxWinProbEvalBudget
}

// SelectionBestResponseRounds returns the cap on alternating best-response rounds in
// win-probability selection.
func SelectionBestResponseRounds(cfg *Config) int {
	if cfg != nil && cfg.Selection.BestResponseRounds > 0 {
		return cfg.Selection.BestResponseRounds
	}
	return DefaultSelectionBestResponseRounds
}

// RetirementInactiveYears returns criterion (a)'s bound: how long a player must have
// appeared in no format for an inactivity claim to corroborate a retirement flag (D-12).
func RetirementInactiveYears(cfg *Config) int {
	if cfg != nil && cfg.Pool.Retirement.InactiveYears > 0 {
		return cfg.Pool.Retirement.InactiveYears
	}
	return DefaultRetirementInactiveYears
}

// RetirementAgeInactiveYears returns criterion (c)'s inactivity half: how long a player
// above the age bound must also have been inactive (D-12).
func RetirementAgeInactiveYears(cfg *Config) int {
	if cfg != nil && cfg.Pool.Retirement.AgeInactiveYears > 0 {
		return cfg.Pool.Retirement.AgeInactiveYears
	}
	return DefaultRetirementAgeInactiveYears
}

// Resource limits (used by the resources package). 0 in config = use default constant.
func ResourcesImportMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.ImportMBPerWorker > 0 {
		return cfg.Resources.ImportMBPerWorker
	}
	return DefaultImportMBPerWorker
}

func ResourcesMemoryUsageFractionPercent(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.MemoryUsageFractionPercent > 0 {
		return cfg.Resources.MemoryUsageFractionPercent
	}
	return DefaultMemoryUsageFractionPercent
}

// DefaultOutputDir returns go-app's output directory.
// GO_APP_OUTPUT_DIR (when set) overrides config so the API can use a writable path in Docker.
// Otherwise config or cwd-relative "output/go-app" is used.
func DefaultOutputDir() string {
	if p := os.Getenv("GO_APP_OUTPUT_DIR"); p != "" {
		return p
	}
	cfg := Load()
	if cfg != nil && cfg.Outputs.Dir != "" {
		return cfg.Outputs.Dir
	}
	return filepath.Join("output", "go-app")
}
