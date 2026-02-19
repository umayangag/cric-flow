// Package selection contains DB-backed team selection (replacing the legacy Python prototype).
package selection

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	"github.com/umayangag/cric-flow/go-app/internal/models"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
)

// SelectTeam builds the player pool from DB, constructs features, calls mlCleint service
// for batting/bowling predictions, merges/fills attributes, calls the win model,
// applies constraints, and returns the selected XI and team win probability.
func SelectTeam(
	ctx context.Context,
	matchID int64,
	format string,
	season string,
	opts Options,
) (Result, error) {
	if opts.TeamSize <= 0 {
		opts.TeamSize = 11
	}
	cfg := config.Load()
	// Fetch match context
	mc, err := db.GetMatchContext(ctx, matchID)
	if err != nil {
		return Result{}, fmt.Errorf("get match context: %w", err)
	}
	// Resolve IDs
	fmtCode := strings.ToUpper(strings.TrimSpace(format))
	fmtID, err := db.GetOrCreateMatchFormat(ctx, fmtCode)
	if err != nil {
		return Result{}, fmt.Errorf("format id: %w", err)
	}
	// Previous season for form
	prevSeasonName := prevSeasonName(season)
	prevSeasonID, err := db.GetOrCreateSeason(ctx, prevSeasonName)
	if err != nil {
		return Result{}, fmt.Errorf("prev season id: %w", err)
	}
	// Player pool
	pool, err := db.ListPlayerPoolConsistency(ctx, season, fmtCode)
	if err != nil {
		return Result{}, fmt.Errorf("list player pool: %w", err)
	}
	if len(pool) == 0 {
		return Result{}, fmt.Errorf("no eligible players for season=%s format=%s", season, fmtCode)
	}

	// Build features
	w := cfg.Weather.Mocks
	batSess := encodeSession(int(nz64(mc.Session)))
	bowlSess := encodeSession(int(nz64(mc.Session)))
	batInning := int(nz64(mc.Inning))
	if batInning <= 0 {
		batInning = 1
	}
	toss := int(nz64(mc.Toss))
	visc := encodeViscosity(w.Viscosity)
	seasonNum := parseSeasonInt(season)
	venueID := int64(nz64(mc.VenueID))
	oppoID := int64(nz64(mc.OppositionID))

	batFeats := make([]models.BattingFeatures, 0, len(pool))
	bowlFeats := make([]models.BowlingFeatures, 0, len(pool))
	isBowler := make([]bool, 0, len(pool))
	isKeeper := make([]bool, 0, len(pool))
	playerNames := make([]string, 0, len(pool))

	for _, p := range pool {
		// Per-player aggregates
		batForm, bowlForm, _ := db.GetPlayerFormFmt(ctx, p.PlayerID, prevSeasonID, fmtID)
		batVenue, bowlVenue, _ := db.GetPlayerVenueEffectFmt(ctx, p.PlayerID, venueID, fmtID)
		batOpp, bowlOpp, _ := db.GetPlayerOppositionEffectFmt(ctx, p.PlayerID, oppoID, fmtID)

		bf := models.BattingFeatures{
			BattingConsistency: f32(p.BattingConsistency.Float64),
			BattingForm:        f32(batForm),
			BattingTemp:        w.Temp,
			BattingWind:        w.Wind,
			BattingRain:        w.Rain,
			BattingHumidity:    w.Humidity,
			BattingCloud:       w.Cloud,
			BattingPressure:    w.Pressure,
			BattingViscosity:   visc,
			BattingInning:      batInning,
			BattingSession:     batSess,
			Toss:               toss,
			Venue:              f32(batVenue),
			Opposition:         f32(batOpp),
			Season:             seasonNum,
			PlayerName:         p.PlayerName,
			Format:             fmtCode,
		}
		batFeats = append(batFeats, bf)

		bow := models.BowlingFeatures{
			BowlingConsistency: f32(p.BowlingConsistency.Float64),
			BowlingForm:        f32(bowlForm),
			BowlingTemp:        w.Temp,
			BowlingWind:        w.Wind,
			BowlingRain:        w.Rain,
			BowlingHumidity:    w.Humidity,
			BowlingCloud:       w.Cloud,
			BowlingPressure:    w.Pressure,
			BowlingViscosity:   visc,
			BattingInning:      batInning,
			BowlingSession:     bowlSess,
			Toss:               toss,
			BowlingVenue:       f32(bowlVenue),
			BowlingOpposition:  f32(bowlOpp),
			Season:             seasonNum,
			PlayerName:         p.PlayerName,
			Format:             fmtCode,
		}
		bowlFeats = append(bowlFeats, bow)
		isBowler = append(isBowler, p.BowlingConsistency.Valid && p.BowlingConsistency.Float64 > 0)
		isKeeper = append(isKeeper, p.IsWicketKeeper == 1)
		playerNames = append(playerNames, p.PlayerName)
	}

	cli := mlclient.New()
	batPreds, err := cli.PredictBatting(ctx, batFeats)
	if err != nil {
		return Result{}, fmt.Errorf("predict batting: %w", err)
	}
	bowlPreds, err := cli.PredictBowling(ctx, bowlFeats)
	if err != nil {
		return Result{}, fmt.Errorf("predict bowling: %w", err)
	}
	if len(batPreds) != len(pool) || len(bowlPreds) != len(pool) {
		return Result{}, fmt.Errorf(
			"prediction size mismatch: bat=%d bowl=%d pool=%d",
			len(batPreds),
			len(bowlPreds),
			len(pool),
		)
	}

	players := make([]predictor.PlayerPrediction, 0, len(pool))
	for i := range pool {
		bp := batPreds[i]
		wp := bowlPreds[i]
		pp := predictor.PlayerPrediction{
			PlayerName:      playerNames[i],
			RunsScored:      float64(bp.RunsScored),
			BallsFaced:      float64(bp.BallsFaced),
			FoursScored:     float64(bp.FoursScored),
			SixesScored:     float64(bp.SixesScored),
			BattingPosition: float64(bp.BattingPosition),
			StrikeRate:      float64(bp.StrikeRate),
			RunsConceded:    float64(wp.RunsConceded),
			Deliveries:      float64(wp.Deliveries),
			WicketsTaken:    float64(wp.WicketsTaken),
			Econ:            float64(wp.Econ),
		}
		// If player is not a bowler, zero out bowling predictions to mirror prototype fill_missing
		if !isBowler[i] {
			pp.RunsConceded = 0
			pp.Deliveries = 0
			pp.WicketsTaken = 0
			pp.Econ = 0
		}
		players = append(players, pp)
	}

	// Call win predictor
	winPlayers, err := cli.PredictWin(ctx, players)
	if err != nil {
		return Result{}, fmt.Errorf("predict win: %w", err)
	}
	// Sort by per-player probability desc
	sort.Slice(
		winPlayers,
		func(i, j int) bool { return winPlayers[i].WinningProbability > winPlayers[j].WinningProbability },
	)

	// Enforce MinBowlers and optional RequireKeeper
	selected := make([]predictor.PlayerPrediction, 0, opts.TeamSize)
	bowlCount := 0
	keeperCount := 0
	for i := 0; i < len(winPlayers) && len(selected) < opts.TeamSize; i++ {
		p := winPlayers[i]
		selected = append(selected, p)
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlCount++
		}
		if isKeeper[i] { // aligned by order of pool/winPlayers
			keeperCount++
		}
	}
	// Try to satisfy bowlers
	if bowlCount < opts.MinBowlers {
		for i := opts.TeamSize; i < len(winPlayers) && bowlCount < opts.MinBowlers; i++ {
			cand := winPlayers[i]
			if !(cand.Deliveries > 0 || cand.Econ > 0) {
				continue
			}
			// Replace lowest-ranked non-bowler
			replaced := false
			for j := len(selected) - 1; j >= 0; j-- {
				if !(selected[j].Deliveries > 0 || selected[j].Econ > 0) {
					selected[j] = cand
					bowlCount++
					replaced = true
					break
				}
			}
			if !replaced {
				break
			}
		}
	}
	// Try to ensure at least one keeper if requested
	if opts.RequireKeeper {
		// Determine keeper presence in selected using original pool order mapping
		present := false
		for i := range selected {
			// find index of player in original winPlayers slice
			// since we aligned by order initially, we can map by name (assumed unique enough for our dataset)
			name := selected[i].PlayerName
			for k := range pool {
				if pool[k].PlayerName == name && isKeeper[k] {
					present = true
					break
				}
			}
			if present {
				break
			}
		}
		if !present {
			for i := opts.TeamSize; i < len(winPlayers); i++ {
				cand := winPlayers[i]
				// is this candidate a keeper?
				candIsKeeper := false
				for k := range pool {
					if pool[k].PlayerName == cand.PlayerName && isKeeper[k] {
						candIsKeeper = true
						break
					}
				}
				if !candIsKeeper {
					continue
				}
				// replace lowest-ranked non-keeper
				replaced := false
				for j := len(selected) - 1; j >= 0; j-- {
					curIsKeeper := false
					for k := range pool {
						if pool[k].PlayerName == selected[j].PlayerName && isKeeper[k] {
							curIsKeeper = true
							break
						}
					}
					if !curIsKeeper {
						selected[j] = cand
						replaced = true
						// keeperCount = 1
						break
					}
				}
				if replaced {
					break
				}
			}
		}
	}

	// Compute team average probability
	sum := 0.0
	for _, p := range selected {
		sum += p.WinningProbability
	}
	avg := 0.0
	if len(selected) > 0 {
		avg = sum / float64(len(selected))
	}
	// Keep selected sorted
	sort.Slice(selected, func(i, j int) bool { return selected[i].WinningProbability > selected[j].WinningProbability })

	return Result{Players: selected, TeamWinProbability: avg}, nil
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
