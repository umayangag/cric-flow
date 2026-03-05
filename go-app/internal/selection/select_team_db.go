// Package selection contains DB-backed team selection (replacing the legacy Python prototype).
package selection

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
)

type scoredPlayer struct {
	Name     string
	IsBowler bool
	IsKeeper bool
	Score    float64
	Pred     mlclient.UnifiedPlayerPrediction
}

// SelectTeam builds the player pool from DB, computes features via the standard
// feature pipeline, calls the unified ML endpoint for batting/bowling/fielding
// predictions in a single HTTP call, scores players, applies constraints, and
// returns the selected XI.
func SelectTeam(
	ctx context.Context,
	matchID int64,
	format string,
	_ string,
	opts Options,
) (Result, error) {
	if opts.TeamSize <= 0 {
		opts.TeamSize = 11
	}
	cfg := config.Load()

	mc, err := db.GetMatchContext(ctx, matchID)
	if err != nil {
		return Result{}, fmt.Errorf("get match context: %w", err)
	}

	fmtCode := strings.ToUpper(strings.TrimSpace(format))

	pool, err := db.ListPlayerPoolConsistency(ctx, "", fmtCode)
	if err != nil {
		return Result{}, fmt.Errorf("list player pool: %w", err)
	}
	if len(pool) == 0 {
		return Result{}, fmt.Errorf("no eligible players for format=%s", fmtCode)
	}

	playerIDs := make([]int64, 0, len(pool))
	for _, p := range pool {
		playerIDs = append(playerIDs, p.PlayerID)
	}

	venueID := nz64(mc.VenueID)
	oppoID := nz64(mc.OppositionID)
	var venuePtr *int64
	if venueID != 0 {
		venuePtr = &venueID
	}
	var seasonPtr *int64
	if sid := nz64(mc.SeasonID); sid != 0 {
		seasonPtr = &sid
	}

	cutoff := time.Now().Truncate(24 * time.Hour)

	features, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx, cutoff, fmtCode, venuePtr, oppoID, seasonPtr, playerIDs, nil, nil,
	)
	if err != nil {
		return Result{}, fmt.Errorf("compute features: %w", err)
	}

	cli := mlclient.New()
	preds, err := cli.PredictPlayers(ctx, cutoff, fmtCode, playerIDs, features)
	if err != nil {
		return Result{}, fmt.Errorf("predict players: %w", err)
	}

	batDiv, wickDiv, econBase, fieldDiv := config.EffectiveScoreNormParams(cfg, fmtCode)
	batW, bowlW, fieldW, keeperW := config.EffectiveScoreWeightsForFormat(cfg, fmtCode)

	scored := make([]scoredPlayer, 0, len(pool))
	for _, p := range pool {
		pr := preds[p.PlayerID]
		isBowler := p.BowlingConsistency.Valid && p.BowlingConsistency.Float64 > 0
		batScore := math.Min(1, pr.Runs/batDiv)
		bowlScore := 0.0
		if isBowler {
			wickPart := math.Min(1, pr.Wickets/wickDiv)
			econPart := math.Max(0, 1-(pr.Economy/econBase))
			bowlScore = (wickPart + econPart) / 2
		}
		fieldScore := math.Min(1, (pr.Catches+pr.RunOuts*1.5)/fieldDiv)

		s := batW*batScore + bowlW*bowlScore + fieldW*fieldScore
		if p.IsWicketKeeper == 1 {
			s += keeperW
		}

		scored = append(scored, scoredPlayer{
			Name:     p.PlayerName,
			IsBowler: isBowler,
			IsKeeper: p.IsWicketKeeper == 1,
			Score:    s,
			Pred:     pr,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Name < scored[j].Name
	})

	selected := selectWithConstraints(scored, opts.TeamSize, opts.MinBowlers, opts.RequireKeeper)

	result := make([]predictor.PlayerPrediction, 0, len(selected))
	for _, s := range selected {
		result = append(result, predictor.PlayerPrediction{
			PlayerName:         s.Name,
			RunsScored:         s.Pred.Runs,
			BallsFaced:         s.Pred.Balls,
			FoursScored:        s.Pred.Fours,
			SixesScored:        s.Pred.Sixes,
			WicketsTaken:       s.Pred.Wickets,
			Econ:               s.Pred.Economy,
			WinningProbability: s.Score,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].WinningProbability > result[j].WinningProbability
	})

	avgScore := 0.0
	for _, p := range result {
		avgScore += p.WinningProbability
	}
	if len(result) > 0 {
		avgScore /= float64(len(result))
	}

	return Result{Players: result, TeamWinProbability: avgScore}, nil
}

// selectWithConstraints picks teamSize players from the pre-sorted scored list,
// ensuring MinBowlers and optionally at least one keeper via swap-ins.
func selectWithConstraints(scored []scoredPlayer, teamSize, minBowlers int, requireKeeper bool) []scoredPlayer {
	if len(scored) < teamSize {
		teamSize = len(scored)
	}
	selected := make([]scoredPlayer, teamSize)
	copy(selected, scored[:teamSize])
	remaining := scored[teamSize:]

	bowlerCount := 0
	for _, p := range selected {
		if p.IsBowler {
			bowlerCount++
		}
	}
	for _, cand := range remaining {
		if bowlerCount >= minBowlers {
			break
		}
		if !cand.IsBowler {
			continue
		}
		for j := len(selected) - 1; j >= 0; j-- {
			if !selected[j].IsBowler {
				selected[j] = cand
				bowlerCount++
				break
			}
		}
	}

	if requireKeeper {
		hasKeeper := false
		for _, p := range selected {
			if p.IsKeeper {
				hasKeeper = true
				break
			}
		}
		if !hasKeeper {
			for _, cand := range remaining {
				if !cand.IsKeeper {
					continue
				}
				for j := len(selected) - 1; j >= 0; j-- {
					if !selected[j].IsKeeper && !selected[j].IsBowler {
						selected[j] = cand
						break
					}
				}
				break
			}
		}
	}

	return selected
}

func nz64(v struct {
	Int64 int64
	Valid bool
},
) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

func f32(v float64) float32 { return float32(v) }

func prevSeasonName(season string) string {
	n, err := strconv.Atoi(strings.TrimSpace(season))
	if err != nil {
		return season
	}
	return strconv.Itoa(n - 1)
}

func parseSeasonInt(season string) int {
	n, err := strconv.Atoi(strings.TrimSpace(season))
	if err != nil {
		return 0
	}
	return n
}
