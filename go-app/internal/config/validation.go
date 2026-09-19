package config

import (
	"fmt"
	"log/slog"
)

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
	// Default batters cannot be negative.
	if cfg.Team.DefaultBatters < 0 {
		err := fmt.Errorf("default batters must be >= 0")
		slog.Error("config.ValidateTeamSettings failed", slog.Any("err", err))
		return err
	}
	// An eleven cannot hold more bowling options than it holds players. The bound is the
	// team size and the team size is 11: every model the constraint is checked against was
	// fitted on elevens, and ml-service refuses any other side size outright (SERVE-02), so
	// there is no configured size for this to be relative to.
	if cfg.Team.MinBowlers > DefaultTeamSize {
		err := fmt.Errorf("min bowlers must be <= %d, which is how many players an eleven holds", DefaultTeamSize)
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
