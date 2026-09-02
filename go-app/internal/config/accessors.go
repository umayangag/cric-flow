package config

import (
	"os"
	"path/filepath"
)

// EffectiveExportMaxMatchIDs returns the backtest export-contributions match_ids limit.
func EffectiveExportMaxMatchIDs(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ExportMaxMatchIDs > 0 {
		return cfg.Backtest.ExportMaxMatchIDs
	}
	return DefaultExportMaxMatchIDs
}

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

// Pipeline precompute ETA seconds per format (used before any format completes).
func PipelinePrecomputeETASecondsPerFmt(cfg *Config) int {
	if cfg != nil && cfg.Pipeline.PrecomputeETASecondsPerFmt > 0 {
		return cfg.Pipeline.PrecomputeETASecondsPerFmt
	}
	return DefaultServerPrecomputeETASecPerFmt
}

// Backtest list/accuracy limits and job config.
func BacktestListDefaultLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ListDefaultLimit > 0 {
		return cfg.Backtest.ListDefaultLimit
	}
	return DefaultBacktestListDefaultLimit
}

func BacktestListMaxLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ListMaxLimit > 0 {
		return cfg.Backtest.ListMaxLimit
	}
	return DefaultBacktestListMaxLimit
}

func BacktestAccuracyTrendDefaultLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.AccuracyTrendDefaultLimit > 0 {
		return cfg.Backtest.AccuracyTrendDefaultLimit
	}
	return DefaultBacktestAccuracyTrendLimit
}

func BacktestAccuracyTrendMaxLimit(cfg *Config) int {
	if cfg != nil && cfg.Backtest.AccuracyTrendMaxLimit > 0 {
		return cfg.Backtest.AccuracyTrendMaxLimit
	}
	return DefaultBacktestListMaxLimit
}

func BacktestAccuracyTrendConcurrency(cfg *Config) int {
	if cfg != nil && cfg.Backtest.AccuracyTrendConcurrency > 0 {
		return cfg.Backtest.AccuracyTrendConcurrency
	}
	return DefaultBacktestAccuracyTrendConcurrency
}

func BacktestExportContributionsConcurrency(cfg *Config) int {
	if cfg != nil && cfg.Backtest.ExportContributionsConcurrency > 0 {
		return cfg.Backtest.ExportContributionsConcurrency
	}
	return DefaultBacktestExportContributionsConcurrency
}

func BacktestJobCleanupAgeHours(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.CleanupAgeHours > 0 {
		return cfg.Backtest.Job.CleanupAgeHours
	}
	return DefaultBacktestJobCleanupAgeHours
}

func BacktestJobCleanupIntervalMin(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.CleanupIntervalMin > 0 {
		return cfg.Backtest.Job.CleanupIntervalMin
	}
	return DefaultBacktestJobCleanupIntervalMin
}

func BacktestExportContributionsJobMaxDurationHr(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.ExportContributionsMaxDurHr > 0 {
		return cfg.Backtest.Job.ExportContributionsMaxDurHr
	}
	return DefaultExportContributionsJobMaxDurHr
}

func BacktestEvalJobMaxDurationHr(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobMaxDurationHr > 0 {
		return cfg.Backtest.Job.EvalJobMaxDurationHr
	}
	return DefaultEvalJobMaxDurationHr
}

func BacktestEvalJobConcurrencyMin(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobConcurrencyMin > 0 {
		return cfg.Backtest.Job.EvalJobConcurrencyMin
	}
	return DefaultEvalJobConcurrencyMin
}

func BacktestEvalJobConcurrencyMax(cfg *Config) int {
	if cfg != nil && cfg.Backtest.Job != nil && cfg.Backtest.Job.EvalJobConcurrencyMax > 0 {
		return cfg.Backtest.Job.EvalJobConcurrencyMax
	}
	return DefaultEvalJobConcurrencyMax
}

// Ops pagination.
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
	return DefaultBacktestRecentMigrations
}

// Pipeline replay page size.
func PipelineReplayMatchPageSize(cfg *Config) int {
	if cfg != nil && cfg.Pipeline.ReplayMatchPageSize > 0 {
		return cfg.Pipeline.ReplayMatchPageSize
	}
	return DefaultPipelineReplayMatchPageSize
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

// Resource limits (used by resources package). 0 in config = use default constant.
func ResourcesPrecomputeMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.PrecomputeMBPerWorker > 0 {
		return cfg.Resources.PrecomputeMBPerWorker
	}
	return DefaultPrecomputeMBPerWorker
}

func ResourcesImportMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.ImportMBPerWorker > 0 {
		return cfg.Resources.ImportMBPerWorker
	}
	return DefaultImportMBPerWorker
}

func ResourcesExportMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.ExportMBPerWorker > 0 {
		return cfg.Resources.ExportMBPerWorker
	}
	return DefaultExportMBPerWorker
}

func ResourcesSeqCalcMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.SeqCalcMBPerWorker > 0 {
		return cfg.Resources.SeqCalcMBPerWorker
	}
	return DefaultSeqCalcMBPerWorker
}

func ResourcesFieldingMBPerWorker(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.FieldingMBPerWorker > 0 {
		return cfg.Resources.FieldingMBPerWorker
	}
	return DefaultFieldingMBPerWorker
}

func ResourcesMemoryUsageFractionPercent(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.MemoryUsageFractionPercent > 0 {
		return cfg.Resources.MemoryUsageFractionPercent
	}
	return DefaultMemoryUsageFractionPercent
}

func ResourcesSeqCalcLowMemoryLimitGiB(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.SeqCalcLowMemoryLimitGiB > 0 {
		return cfg.Resources.SeqCalcLowMemoryLimitGiB
	}
	return DefaultSeqCalcLowMemoryLimitGiB
}

func ResourcesPrecomputeConcurrencyWhenNoLimit(cfg *Config) int {
	if cfg != nil && cfg.Resources != nil && cfg.Resources.PrecomputeConcurrencyWhenNoLimit > 0 {
		return cfg.Resources.PrecomputeConcurrencyWhenNoLimit
	}
	return DefaultPrecomputeConcurrencyWhenNoLimit
}

// DefaultExportDir returns the configured export output dir or a built-in default.
// GO_APP_OUTPUT_DIR (when set) overrides config so the API can use a writable path in Docker.
// Otherwise config or cwd-relative "output/go-app" is used.
func DefaultExportDir() string {
	if p := os.Getenv("GO_APP_OUTPUT_DIR"); p != "" {
		return p
	}
	cfg := Load()
	if cfg != nil && cfg.Outputs.ExportDir != "" {
		return cfg.Outputs.ExportDir
	}
	return filepath.Join("output", "go-app")
}
