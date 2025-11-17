package predictor

import (
	"errors"
	"flag"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// options represents parsed CLI inputs for team-predictor.
type options struct {
	matchID    int64
	batters    int
	bowlers    int
	formatCode string
	seasonName string
}

// parseFlags parses CLI args into options, applying defaults from cfg when values are zero.
// This function is pure and does not read environment or files; it exists to enable unit testing.
func parseFlags(args []string, cfg *config.Config) (options, error) {
	var (
		matchID    int64
		bat        int
		bowl       int
		formatCode string
		season     string
	)
	fs := flag.NewFlagSet("team-predictor", flag.ContinueOnError)
	fs.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	fs.IntVar(&bat, "bat", 0, "number of batters to pick (defaults from config.team.default_batters)")
	fs.IntVar(&bowl, "bowl", 0, "number of bowlers to pick (defaults from config.team.default_bowlers)")
	fs.StringVar(&formatCode, "format", "", "match format code (TEST, ODI, T20, T20I)")
	fs.StringVar(&season, "season", "", "season name (e.g. 2019)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	if matchID == 0 || strings.TrimSpace(formatCode) == "" || strings.TrimSpace(season) == "" {
		return options{}, errors.New(
			"usage: team-predictor -match=<match_id> -format=<CODE> -season=<season> [-bat=N] [-bowl=N]",
		)
	}

	// Apply defaults from cfg
	minB := 5
	if cfg != nil && cfg.Team.MinBowlers > 0 {
		minB = cfg.Team.MinBowlers
	}
	if bat <= 0 {
		if cfg != nil && cfg.Team.DefaultBatters > 0 {
			bat = cfg.Team.DefaultBatters
		} else {
			bat = 6
		}
	}
	if bowl <= 0 {
		if cfg != nil && cfg.Team.DefaultBowlers > 0 {
			bowl = cfg.Team.DefaultBowlers
		} else {
			bowl = minB
		}
	}
	if bowl < minB {
		bowl = minB
	}

	return options{
		matchID:    matchID,
		batters:    bat,
		bowlers:    bowl,
		formatCode: formatCode,
		seasonName: season,
	}, nil
}
