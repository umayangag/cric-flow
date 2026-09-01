// Package predictteam answers "who should play, and what will happen" for a future match.
//
// Everything it returns comes from the XI layer in ml-service (plan L2/L3): the XI from
// /xi/optimize, the displayed win probability from /xi/predict-win, and the totals,
// per-player points and their ranges from /simulate — or, where there is no innings length,
// from /performance/predict. Nothing here scores players, weighs a batting score against a
// bowling one, or rescales one model's answer toward another's; those were the windowed-form
// path and they are gone (plan P-5).
//
// go-app's remaining job is the part ml-service cannot know: who is available (the pool),
// which fixture this is (format, teams, venue, date), and what the caller requires of an XI.
package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Input defines the request for future-match team selection.
type Input struct {
	Format        string    `json:"format"`
	Team1         string    `json:"team1"`
	Team2         string    `json:"team2"`
	Venue         string    `json:"venue,omitempty"` // venue name; empty = unknown venue
	MatchDate     time.Time `json:"match_date"`
	ExtraTeam1    []int64   `json:"extra_team1,omitempty"` // extra player IDs for team1 (e.g. IPL auction)
	ExtraTeam2    []int64   `json:"extra_team2,omitempty"` // extra player IDs for team2
	MinBowlers    int       `json:"min_bowlers,omitempty"` // default from config
	RequireKeeper bool      `json:"require_keeper,omitempty"`
	// AsOf, when set, asks for ratings as they stood strictly before this date instead of
	// "through today". Backtests over played matches set it to the match date, so a
	// prediction provably cannot see the match's own result or any later one; live
	// predictions leave it zero.
	AsOf time.Time `json:"as_of,omitempty"`
}

// Constraints are what the caller knows about an XI and the model does not: how many
// players, how many bowling options, whether a keeper is required. ml-service reads them
// off the same as-of vectors the objective reads, so "a bowling option" means one thing.
type Constraints struct {
	Size          int
	MinBowlers    int
	RequireKeeper bool
}

// ValueRange is a 10-90 range shown beside a point.
type ValueRange struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
}

// SelectedPlayer is one player in the selected XI, with the point the scorecard shows, the
// 10-90 range around it, and the two L3 explanations: what the XI loses without this player
// and how much of the innings total's spread he accounts for.
type SelectedPlayer struct {
	PlayerID int64 `json:"player_id"`
	// PlayerKey is the registry id ml-service knows this player by. It is how the
	// simulator's and the optimiser's answers are matched back to these rows, and it stays
	// off the wire: a client joins on player_id, which is this repo's own identifier.
	PlayerKey         string      `json:"-"`
	PlayerName        string      `json:"player_name"`
	Runs              float64     `json:"runs"`
	RunsRange         *ValueRange `json:"runs_range,omitempty"`
	Balls             float64     `json:"balls,omitempty"`
	BallsRange        *ValueRange `json:"balls_range,omitempty"`
	Wickets           float64     `json:"wickets"`
	WicketsRange      *ValueRange `json:"wickets_range,omitempty"`
	RunsConceded      float64     `json:"runs_conceded"`
	RunsConcededRange *ValueRange `json:"runs_conceded_range,omitempty"`
	// Economy is runs conceded per over, and needs a balls-bowled forecast to exist: only
	// the simulator produces one, so it is zero on the performance-only path.
	Economy float64 `json:"economy,omitempty"`
	// MarginalValue is P(win) lost if this player were replaced by an average one. Absent
	// on a rating-ordered XI: nothing was maximised, so nothing has a margin.
	MarginalValue *float64 `json:"marginal_value,omitempty"`
	// SpreadShare is the player's share of the side total's variance, from the simulator.
	SpreadShare *float64 `json:"spread_share,omitempty"`
}

// SelectionSummary says how the XIs were chosen.
//
// Optimised is false where the objective does not rank (H-17: TEST): the XI is the
// rating-ordered pick, which is a selection but not an optimised one, and every surface
// that shows it has to say so.
type SelectionSummary struct {
	Objective string `json:"objective"` // "win" or "ratings"
	Optimised bool   `json:"optimised"`
	Note      string `json:"note,omitempty"`
}

// WinProbabilitySummary is the headline probability and where it came from.
//
// Source is "display" (the monotone GBM over both elevens) or "simulator" (the share of
// simulated matches won). E2 decided which is the headline per format; the other is
// reported beside it, never blended.
type WinProbabilitySummary struct {
	Team1           float64  `json:"team1"`
	Source          string   `json:"source"`
	Simulated       *float64 `json:"simulated,omitempty"`
	PredictedWinner string   `json:"predicted_winner"`
}

// InningsTotal is one simulated innings: the median-band total the scorecard lines and
// extras sum to, and the distribution the draws actually produced.
type InningsTotal struct {
	Total  float64 `json:"total"`
	Extras float64 `json:"extras"`
	P10    float64 `json:"p10"`
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
}

// Scorecard is the simulated match: present only for formats with an innings length.
type Scorecard struct {
	Samples          int          `json:"samples"`
	TossMarginalised bool         `json:"toss_marginalised"`
	Innings1         InningsTotal `json:"innings1"`
	Innings2         InningsTotal `json:"innings2"`
}

// Result is one prediction: two XIs, how they were chosen, the headline probability, and —
// where the format has an innings length — the simulated scorecard the player points and
// ranges come from.
type Result struct {
	Team1          []SelectedPlayer      `json:"team1"`
	Team2          []SelectedPlayer      `json:"team2"`
	Selection      SelectionSummary      `json:"selection"`
	WinProbability WinProbabilitySummary `json:"win_probability"`
	Scorecard      *Scorecard            `json:"scorecard,omitempty"`
}

// XIService is everything the prediction path needs from ml-service.
//
// One interface because after P-5 there is one path: the selection, the displayed
// probability, the simulated match and the per-player forecasts are all functions of the
// same as-of rating state, keyed by player id. Nothing here takes a feature map.
type XIService interface {
	OptimizeXI(ctx context.Context, req XIOptimizationRequest) (*XIOptimizationResult, error)
	PredictMatchWinXI(ctx context.Context, req XIWinRequest) (float64, error)
	SimulateMatchXI(ctx context.Context, req XISimulationRequest) (*XISimulationResult, error)
	PredictPerformance(ctx context.Context, req XIPerformanceRequest) (*XIPerformanceResult, error)
}

// fixture is the resolved match: ids for everything the ML service is told about.
type fixture struct {
	format               string
	team1Code, team2Code string
	team1ID, team2ID     int64
	venueID              int64
	pool1, pool2         []db.PlayerPoolRow
	constraints          Constraints
	asOf                 time.Time
}

// PredictTeams picks both XIs and predicts the match.
func PredictTeams(ctx context.Context, input Input, service XIService) (*Result, error) {
	fix, err := resolveFixture(ctx, input)
	if err != nil {
		return nil, err
	}

	xi1, xi2, selection, marginals, err := selectBothXIs(ctx, service, fix)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Team1:     newSelectedPlayers(xi1, fix.pool1, marginals),
		Team2:     newSelectedPlayers(xi2, fix.pool2, marginals),
		Selection: selection,
	}

	display, err := service.PredictMatchWinXI(ctx, XIWinRequest{
		Format:          fix.format,
		Team1PlayerKeys: xi1,
		Team2PlayerKeys: xi2,
		Team1ID:         fix.team1ID,
		Team2ID:         fix.team2ID,
		VenueID:         fix.venueID,
		AsOf:            fix.asOf,
	})
	if err != nil {
		return nil, fmt.Errorf("win probability: %w", err)
	}
	result.WinProbability = WinProbabilitySummary{
		Team1:           display,
		Source:          winProbabilitySourceDisplay,
		PredictedWinner: winnerFrom(display, fix.team1Code, fix.team2Code),
	}

	if err := applyMatchForecast(ctx, service, fix, xi1, xi2, result); err != nil {
		return nil, err
	}
	return result, nil
}

// applyMatchForecast fills in the per-player points and ranges, and the scorecard where
// there is one to fill: the simulator for formats with an innings length, the performance
// model's own quantiles for the rest (they have no innings to draw, but every player still
// has a forecast).
func applyMatchForecast(
	ctx context.Context,
	service XIService,
	fix fixture,
	xi1, xi2 []string,
	result *Result,
) error {
	if formatHasInningsLength(fix.format) {
		return applyXISimulation(ctx, service, fix, xi1, xi2, result)
	}
	return applyPerformanceForecast(ctx, service, fix, xi1, xi2, result)
}

// resolveFixture turns names into the ids ml-service is addressed with, and loads both
// player pools. Availability is the caller's knowledge, not the model's (plan §9.4).
func resolveFixture(ctx context.Context, input Input) (fixture, error) {
	format := normalizeFormat(input.Format)
	team1 := strings.TrimSpace(input.Team1)
	team2 := strings.TrimSpace(input.Team2)
	if format == "" || team1 == "" || team2 == "" {
		err := fmt.Errorf("format, team1, team2 are required")
		slog.Error("predictteam.PredictTeams validation failed", slog.Any("err", err))
		return fixture{}, err
	}
	cutoff := input.MatchDate.Truncate(24 * time.Hour)

	// A team name alone can name two sides -- a men's and a women's -- and this request
	// carries no gender, so the resolver picks the side that has played the format.
	team1ID, err := db.FindOppositionIDForFormat(ctx, team1, format)
	if err != nil {
		slog.Error("predictteam.PredictTeams resolve team1 failed", slog.String("team1", team1), slog.Any("err", err))
		return fixture{}, fmt.Errorf("resolve team1 %q: %w", team1, err)
	}
	team2ID, err := db.FindOppositionIDForFormat(ctx, team2, format)
	if err != nil {
		slog.Error("predictteam.PredictTeams resolve team2 failed", slog.String("team2", team2), slog.Any("err", err))
		return fixture{}, fmt.Errorf("resolve team2 %q: %w", team2, err)
	}

	var venueID int64
	if input.Venue != "" {
		if id, verr := db.GetGlobalCache().GetVenueID(ctx, input.Venue); verr == nil {
			venueID = id
		}
	}

	constraints := resolveConstraints(input)
	pool1, err := loadPool(ctx, format, team1, team1ID, cutoff, input.ExtraTeam1, constraints.Size)
	if err != nil {
		return fixture{}, err
	}
	pool2, err := loadPool(ctx, format, team2, team2ID, cutoff, input.ExtraTeam2, constraints.Size)
	if err != nil {
		return fixture{}, err
	}

	return fixture{
		format:      format,
		team1Code:   team1,
		team2Code:   team2,
		team1ID:     team1ID,
		team2ID:     team2ID,
		venueID:     venueID,
		pool1:       pool1,
		pool2:       pool2,
		constraints: constraints,
		asOf:        input.AsOf,
	}, nil
}

func resolveConstraints(input Input) Constraints {
	cfg := config.Load()
	size := config.DefaultTeamSize
	if cfg != nil && cfg.Predictor.TeamSize > 0 {
		size = cfg.Predictor.TeamSize
	}
	minBowlers := input.MinBowlers
	if minBowlers <= 0 {
		minBowlers = config.DefaultMinBowlers
		if cfg != nil && cfg.Team.MinBowlers > 0 {
			minBowlers = cfg.Team.MinBowlers
		}
	}
	return Constraints{Size: size, MinBowlers: minBowlers, RequireKeeper: input.RequireKeeper}
}

func loadPool(
	ctx context.Context,
	format, teamCode string,
	teamID int64,
	cutoff time.Time,
	extra []int64,
	teamSize int,
) ([]db.PlayerPoolRow, error) {
	pool, err := db.ListPlayerPoolByOpposition(ctx, format, teamID, cutoff, extra)
	if err != nil {
		slog.Error("predictteam.PredictTeams pool failed", slog.String("team", teamCode), slog.Any("err", err))
		return nil, fmt.Errorf("%s pool: %w", teamCode, err)
	}
	if len(pool) < teamSize {
		err := fmt.Errorf("%s has only %d players, need at least %d", teamCode, len(pool), teamSize)
		slog.Error("predictteam.PredictTeams pool size", slog.String("team", teamCode), slog.Any("err", err))
		return nil, err
	}
	return pool, nil
}

// newSelectedPlayers turns the chosen registry keys into response rows, in the order the
// optimiser returned them, resolving each back to the pool row he was chosen out of.
//
// A key the pool cannot resolve is dropped rather than returned as a nameless row with a
// zero id: it would mean ml-service answered with a player nobody asked about.
func newSelectedPlayers(keys []string, pool []db.PlayerPoolRow, marginals map[string]float64) []SelectedPlayer {
	byKey := make(map[string]db.PlayerPoolRow, len(pool))
	for _, p := range pool {
		byKey[p.ExternalID] = p
	}
	out := make([]SelectedPlayer, 0, len(keys))
	for _, key := range keys {
		row, ok := byKey[key]
		if !ok {
			slog.Warn("selection returned a player the pool does not hold", slog.String("player_key", key))
			continue
		}
		player := SelectedPlayer{PlayerID: row.PlayerID, PlayerKey: key, PlayerName: row.PlayerName}
		if v, ok := marginals[key]; ok {
			value := v
			player.MarginalValue = &value
		}
		out = append(out, player)
	}
	return out
}

// winnerFrom names the side the headline probability favours. Exactly 0.5 is team2's, as
// it has always been; the probability is displayed beside it, so nothing is hidden.
func winnerFrom(team1Probability float64, team1Code, team2Code string) string {
	if team1Probability >= 0.5 {
		return team1Code
	}
	return team2Code
}

// poolPlayerKeys is the pool as ml-service addresses it: registry ids, skipping anyone the
// importer never matched to a registry entry (0 rows today) because ml-service has no
// rating for a player it has never been told about under that name.
func poolPlayerKeys(pool []db.PlayerPoolRow) []string {
	keys := make([]string, 0, len(pool))
	for _, p := range pool {
		if p.ExternalID == "" {
			slog.Warn("player has no registry id and cannot be selected",
				slog.Int64("player_id", p.PlayerID), slog.String("player_name", p.PlayerName))
			continue
		}
		keys = append(keys, p.ExternalID)
	}
	return keys
}
