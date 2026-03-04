// Package predictteam: Monte Carlo simulation for win probability and outcome distributions.
package predictteam

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// SimulationOpts configures Monte Carlo simulation over top-k XIs and sampled outcomes.
type SimulationOpts struct {
	// TopKPerTeam: number of top XIs to consider per team (default 50). Only used when pool size allows enumeration (<=18).
	TopKPerTeam int
	// NumSamplesPerMatchup: number of match outcome samples per (XI1, XI2) pair (default 500).
	NumSamplesPerMatchup int
	// MaxMatchups: cap on total matchup pairs to simulate (0 = no cap, use all topK1*topK2).
	MaxMatchups int
	// RunsCV: coefficient of variation for sampling runs (sigma = mean * RunsCV). Default 0.35.
	RunsCV float64
	// WicketsCV: same for wickets. Default 0.4.
	WicketsCV float64
	// EconomyCV: same for economy. Default 0.15.
	EconomyCV float64
	// Seed for RNG (0 = use time-based seed).
	Seed int64
}

// DefaultSimulationOpts returns defaults; TopKPerTeam and NumSamplesPerMatchup are 0 and must be set by caller from config (or PredictTeamsWithSimulation fills from config when zero).
// CV values use config defaults; override via config file predictor.simulation (runs_cv, wickets_cv, economy_cv).
func DefaultSimulationOpts() SimulationOpts {
	return SimulationOpts{
		TopKPerTeam:          0,
		NumSamplesPerMatchup: 0,
		MaxMatchups:          0,
		RunsCV:               config.DefaultSimulationRunsCV,
		WicketsCV:            config.DefaultSimulationWicketsCV,
		EconomyCV:            config.DefaultSimulationEconomyCV,
		Seed:                 0,
	}
}

// SimulationResult holds win probabilities and outcome distribution summaries.
type SimulationResult struct {
	WinProbabilityTeam1 float64 `json:"win_probability_team1"`
	WinProbabilityTeam2 float64 `json:"win_probability_team2"`
	DrawProbability     float64 `json:"draw_probability"`

	Innings1TotalMean float64 `json:"innings1_total_mean"`
	Innings1TotalStd  float64 `json:"innings1_total_std"`
	Innings1TotalP10  float64 `json:"innings1_total_p10"`
	Innings1TotalP50  float64 `json:"innings1_total_p50"`
	Innings1TotalP90  float64 `json:"innings1_total_p90"`

	Innings2TotalMean float64 `json:"innings2_total_mean"`
	Innings2TotalStd  float64 `json:"innings2_total_std"`
	Innings2TotalP10  float64 `json:"innings2_total_p10"`
	Innings2TotalP50  float64 `json:"innings2_total_p50"`
	Innings2TotalP90  float64 `json:"innings2_total_p90"`

	NumMatchups int `json:"num_matchups"`
	NumSamples  int `json:"num_samples"`
}

// PredictTeamsWithSimulation runs the same pipeline as PredictTeams (sharing pools and ML predictions),
// then runs Monte Carlo simulation over top-k XIs per team and returns the best-XI result plus simulation summary.
// reconciledGen is optional; when set, reconciled/compare scorecard behaviour is the same as in PredictTeams.
func PredictTeamsWithSimulation(
	ctx context.Context,
	input Input,
	predictor MLPredictor,
	opts SimulationOpts,
	reconciledGen GenerateMatchFunc,
) (*Result, *SimulationResult, error) {
	result, mid, err := predictTeamsWithIntermediates(ctx, input, predictor, reconciledGen)
	if err != nil {
		return nil, nil, err
	}
	if mid == nil {
		return result, nil, nil
	}
	if opts.TopKPerTeam <= 0 {
		opts.TopKPerTeam = config.EffectiveSimulationTopKPerTeam(config.Load())
	}
	if opts.NumSamplesPerMatchup <= 0 {
		opts.NumSamplesPerMatchup = config.EffectiveSimulationNumSamplesPerMatchup(config.Load())
	}
	format := strings.ToUpper(strings.TrimSpace(input.Format))
	cfg := config.Load()
	tsPool1 := buildTeamSelectPool(mid.Pool1, mid.Preds1, format, cfg)
	tsPool2 := buildTeamSelectPool(mid.Pool2, mid.Preds2, format, cfg)
	teamSize := config.DefaultTeamSize
	if cfg != nil && cfg.Predictor.TeamSize > 0 {
		teamSize = cfg.Predictor.TeamSize
	}
	batW, bowlW, fieldW, keeperW := config.EffectiveScoreWeightsForFormat(cfg, format)
	weights := teamselect.ScoreWeights{Bat: batW, Bowl: bowlW, Field: fieldW, KeeperBonus: keeperW}
	constraints := teamselect.Constraints{
		Size:          teamSize,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	}
	topK1, err := teamselect.SelectTopK(tsPool1, weights, constraints, opts.TopKPerTeam)
	if err != nil {
		slog.Warn("predictteam simulation: SelectTopK team1 failed", "team", mid.Team1, "err", err)
		return result, nil, nil
	}
	topK2, err := teamselect.SelectTopK(tsPool2, weights, constraints, opts.TopKPerTeam)
	if err != nil {
		slog.Warn("predictteam simulation: SelectTopK team2 failed", "team", mid.Team2, "err", err)
		return result, nil, nil
	}
	nameToPred1 := make(map[string]PlayerPred)
	for _, p := range mid.Pool1 {
		nameToPred1[p.PlayerName] = mid.Preds1[p.PlayerID]
	}
	nameToPred2 := make(map[string]PlayerPred)
	for _, p := range mid.Pool2 {
		nameToPred2[p.PlayerName] = mid.Preds2[p.PlayerID]
	}
	sim := runSimulation(topK1, topK2, nameToPred1, nameToPred2, mid.Extras1, mid.Extras2, mid.Team1, mid.Team2, opts)
	return result, sim, nil
}

// runSimulation runs Monte Carlo over (topK1 × topK2) matchups, each with NumSamplesPerMatchup samples.
func runSimulation(
	topK1, topK2 [][]teamselect.Player,
	nameToPred1, nameToPred2 map[string]PlayerPred,
	extras1, extras2 float64,
	_, _ string, // team1Code, team2Code reserved for future use
	opts SimulationOpts,
) *SimulationResult {
	if opts.RunsCV <= 0 {
		opts.RunsCV = config.DefaultSimulationRunsCV
	}
	if opts.WicketsCV <= 0 {
		opts.WicketsCV = config.DefaultSimulationWicketsCV
	}
	if opts.EconomyCV <= 0 {
		opts.EconomyCV = config.DefaultSimulationEconomyCV
	}
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	// Reproducible sampling for Monte Carlo; crypto/rand not required for simulation.
	// #nosec G404
	rng := rand.New(rand.NewSource(seed))

	samplesPerMatchup := opts.NumSamplesPerMatchup
	if samplesPerMatchup <= 0 {
		samplesPerMatchup = config.EffectiveSimulationNumSamplesPerMatchup(config.Load())
	}
	if samplesPerMatchup <= 0 {
		samplesPerMatchup = config.DefaultSimulationNumSamplesPerMatchup
	}
	numMatchups := len(topK1) * len(topK2)
	if opts.MaxMatchups > 0 && opts.MaxMatchups < numMatchups {
		numMatchups = opts.MaxMatchups
	}
	totalSamples := numMatchups * samplesPerMatchup
	innings1Samples := make([]float64, 0, totalSamples)
	innings2Samples := make([]float64, 0, totalSamples)
	var team1Wins, team2Wins, draws int

	matchupCount := 0
	for _, xi1 := range topK1 {
		for _, xi2 := range topK2 {
			if opts.MaxMatchups > 0 && matchupCount >= opts.MaxMatchups {
				break
			}
			preds1 := predsForXI(xi1, nameToPred1)
			preds2 := predsForXI(xi2, nameToPred2)
			for s := 0; s < samplesPerMatchup; s++ {
				tot1, tot2 := simulateOneMatch(preds1, preds2, extras1, extras2, opts, rng)
				innings1Samples = append(innings1Samples, tot1)
				innings2Samples = append(innings2Samples, tot2)
				switch {
				case tot1 > tot2:
					team1Wins++
				case tot2 > tot1:
					team2Wins++
				default:
					draws++
				}
			}
			matchupCount++
		}
		if opts.MaxMatchups > 0 && matchupCount >= opts.MaxMatchups {
			break
		}
	}

	n := len(innings1Samples)
	if n == 0 {
		return &SimulationResult{NumMatchups: 0, NumSamples: 0}
	}
	win1 := float64(team1Wins) / float64(n)
	win2 := float64(team2Wins) / float64(n)
	draw := float64(draws) / float64(n)

	mean1, std1 := meanStd(innings1Samples)
	mean2, std2 := meanStd(innings2Samples)
	p10_1, p50_1, p90_1 := percentiles(innings1Samples, 10, 50, 90)
	p10_2, p50_2, p90_2 := percentiles(innings2Samples, 10, 50, 90)

	return &SimulationResult{
		WinProbabilityTeam1: win1,
		WinProbabilityTeam2: win2,
		DrawProbability:     draw,
		Innings1TotalMean:   mean1,
		Innings1TotalStd:    std1,
		Innings1TotalP10:    p10_1,
		Innings1TotalP50:    p50_1,
		Innings1TotalP90:    p90_1,
		Innings2TotalMean:   mean2,
		Innings2TotalStd:    std2,
		Innings2TotalP10:    p10_2,
		Innings2TotalP50:    p50_2,
		Innings2TotalP90:    p90_2,
		NumMatchups:         matchupCount,
		NumSamples:          n,
	}
}

func predsForXI(xi []teamselect.Player, nameToPred map[string]PlayerPred) []PlayerPred {
	out := make([]PlayerPred, 0, len(xi))
	for _, p := range xi {
		out = append(out, nameToPred[p.Name])
	}
	return out
}

func simulateOneMatch(
	preds1, preds2 []PlayerPred,
	extras1, extras2 float64,
	opts SimulationOpts,
	rng *rand.Rand,
) (innings1Total, innings2Total float64) {
	var runs1, runs2 float64
	for _, p := range preds1 {
		runs1 += sampleRuns(p.Runs, opts.RunsCV, rng)
	}
	for _, p := range preds2 {
		runs2 += sampleRuns(p.Runs, opts.RunsCV, rng)
	}
	return runs1 + extras1, runs2 + extras2
}

func sampleRuns(mean, cv float64, rng *rand.Rand) float64 {
	if mean <= 0 {
		return 0
	}
	sigma := mean * cv
	if sigma <= 0 {
		sigma = math.Sqrt(mean)
	}
	x := rng.NormFloat64()*sigma + mean
	if x < 0 {
		return 0
	}
	return x
}

func meanStd(x []float64) (mean, std float64) {
	if len(x) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	mean = sum / float64(len(x))
	var sq float64
	for _, v := range x {
		d := v - mean
		sq += d * d
	}
	std = math.Sqrt(sq / float64(len(x)))
	return mean, std
}

func percentiles(sorted []float64, p10, p50, p90 int) (v10, v50, v90 float64) {
	if len(sorted) == 0 {
		return 0, 0, 0
	}
	cp := make([]float64, len(sorted))
	copy(cp, sorted)
	sort.Float64s(cp)
	n := len(cp)
	idx := func(p int) int {
		i := (n * p) / 100
		if i >= n {
			i = n - 1
		}
		return i
	}
	return cp[idx(p10)], cp[idx(p50)], cp[idx(p90)]
}
