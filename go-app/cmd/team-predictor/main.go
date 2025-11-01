package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// simple session/toss/viscosity encoders for happy-path numeric features
func encodeSession(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "morning"):
		return 0
	case strings.Contains(s, "afternoon"):
		return 1
	case strings.Contains(s, "evening"):
		return 2
	default:
		return 0
	}
}

func encodeToss(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	// 1 => bat first, 0 => field/bowl first (very rough mapping)
	if strings.Contains(s, "bat") {
		return 1
	}
	return 0
}

func encodeViscosity(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "dry":
		return 0
	case "humid":
		return 1
	case "windy":
		return 2
	default:
		return 0
	}
}

func main() {
	var matchID int64
	var wantBatters int
	var wantBowlers int
	var formatCode string
	flag.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	// Use 0 defaults to allow config-driven values
	flag.IntVar(&wantBatters, "bat", 0, "number of batters to pick (defaults from config.team.default_batters)")
	flag.IntVar(&wantBowlers, "bowl", 0, "number of bowlers to pick (defaults from config.team.default_bowlers)")
	flag.StringVar(&formatCode, "format", "", "match format code (TEST, ODI, T20, T20I)")
	flag.Parse()

	cfg := config.Load()
	if matchID == 0 || strings.TrimSpace(formatCode) == "" {
		fmt.Fprintln(os.Stderr, "usage: team-predictor -match=<match_id> -format=<CODE> [-bat=N] [-bowl=N]")
		os.Exit(2)
	}
	// Apply config defaults when flags are not provided (0)
	if wantBatters <= 0 {
		if cfg.Team.DefaultBatters > 0 {
			wantBatters = cfg.Team.DefaultBatters
		} else {
			wantBatters = 6
		}
	}
	minB := 5
	if cfg.Team.MinBowlers > 0 {
		minB = cfg.Team.MinBowlers
	}
	if wantBowlers <= 0 {
		if cfg.Team.DefaultBowlers > 0 {
			wantBowlers = cfg.Team.DefaultBowlers
		} else {
			wantBowlers = minB
		}
	}
	if wantBowlers < minB {
		log.Printf("requested bowlers=%d < %d; adjusting to satisfy minimum", wantBowlers, minB)
		wantBowlers = minB
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Resolve format_id
	fid, err := db.GetMatchFormatIDByCode(ctx, strings.ToUpper(strings.TrimSpace(formatCode)))
	if err != nil {
		log.Fatalf("resolve format %s: %v", formatCode, err)
	}

	// Load match context
	var inning int
	var battingSession, bowlingSession, toss string
	var seasonID, venueID, oppositionID *int64
	row := db.Pool.QueryRow(
		ctx,
		`SELECT inning, batting_session, bowling_session, toss, season_id, venue_id, opposition_id
		FROM match_details WHERE match_id = $1`,
		matchID,
	)
	if err := row.Scan(&inning, &battingSession, &bowlingSession, &toss, &seasonID, &venueID, &oppositionID); err != nil {
		log.Fatalf("load match_details(%d): %v", matchID, err)
	}

	// Load weather for batting and bowling sessions (defaults to zeros if missing)
	batWX := weatherRow(ctx, matchID, "batting")
	bowlWX := weatherRow(ctx, matchID, "bowling")

	// Candidate players = union of batting/bowling participants in this match (by player table join)
	candidates, err := loadCandidates(ctx, matchID)
	if err != nil {
		log.Fatalf("load candidates: %v", err)
	}
	if len(candidates) == 0 {
		log.Fatalf("no players found for match_id=%d", matchID)
	}

	// Build feature vectors
	batFeats := make([]contracts.BattingFeatures, 0, len(candidates))
	bowlFeats := make([]contracts.BowlingFeatures, 0, len(candidates))
	for _, p := range candidates {
		bf := contracts.BattingFeatures{
			BattingConsistency: p.BatCons,
			BattingForm:        loadFormFmt(ctx, p.ID, seasonID, fid, true),
			BattingTemp:        batWX.Temp,
			BattingWind:        batWX.Wind,
			BattingRain:        batWX.Rain,
			BattingHumidity:    batWX.Humidity,
			BattingCloud:       batWX.Cloud,
			BattingPressure:    batWX.Pressure,
			BattingViscosity:   encodeViscosity(batWX.Viscosity),
			BattingInning:      inning,
			BattingSession:     encodeSession(battingSession),
			Toss:               encodeToss(toss),
			Venue:              loadVenueFmt(ctx, p.ID, venueID, fid, true),
			Opposition:         loadOppositionFmt(ctx, p.ID, oppositionID, fid, true),
			Season:             ptrToInt(seasonID),
			PlayerName:         p.Name,
			Format:             strings.ToUpper(strings.TrimSpace(formatCode)),
		}
		batFeats = append(batFeats, bf)

		wf := contracts.BowlingFeatures{
			BowlingConsistency: p.BowlCons,
			BowlingForm:        loadFormFmt(ctx, p.ID, seasonID, fid, false),
			BowlingTemp:        bowlWX.Temp,
			BowlingWind:        bowlWX.Wind,
			BowlingRain:        bowlWX.Rain,
			BowlingHumidity:    bowlWX.Humidity,
			BowlingCloud:       bowlWX.Cloud,
			BowlingPressure:    bowlWX.Pressure,
			BowlingViscosity:   encodeViscosity(bowlWX.Viscosity),
			BattingInning:      inning,
			BowlingSession:     encodeSession(bowlingSession),
			Toss:               encodeToss(toss),
			BowlingVenue:       loadVenueFmt(ctx, p.ID, venueID, fid, false),
			BowlingOpposition:  loadOppositionFmt(ctx, p.ID, oppositionID, fid, false),
			Season:             ptrToInt(seasonID),
			PlayerName:         p.Name,
			Format:             strings.ToUpper(strings.TrimSpace(formatCode)),
		}
		bowlFeats = append(bowlFeats, wf)
	}

	// Call ML service
	cli := mlclient.New()
	batPreds, err := cli.PredictBatting(ctx, batFeats)
	if err != nil {
		log.Fatalf("predict batting: %v", err)
	}
	bowlPreds, err := cli.PredictBowling(ctx, bowlFeats)
	if err != nil {
		log.Fatalf("predict bowling: %v", err)
	}

	// Rank players
	type batRank struct {
		name     string
		runs     float32
		isBowler bool
	}
	type bowlRank struct {
		name string
		wkts float32
	}
	bats := make([]batRank, 0, len(candidates))
	bowls := make([]bowlRank, 0, len(candidates))
	for i, p := range candidates {
		bats = append(bats, batRank{name: p.Name, runs: batPreds[i].RunsScored, isBowler: p.IsBowler})
		bowls = append(bowls, bowlRank{name: p.Name, wkts: bowlPreds[i].WicketsTaken})
	}
	sort.Slice(bats, func(i, j int) bool { return bats[i].runs > bats[j].runs })
	sort.Slice(bowls, func(i, j int) bool { return bowls[i].wkts > bowls[j].wkts })
	// Selection rationale (top candidates) for observability
	log.Printf("Top batting candidates (format=%s): %v", strings.ToUpper(strings.TrimSpace(formatCode)), topNamesBat(bats, 5))
	log.Printf("Top bowling candidates (format=%s): %v", strings.ToUpper(strings.TrimSpace(formatCode)), topNamesBowl(bowls, 5))

	// Pick bowlers first to satisfy minimum 5
	selected := map[string]bool{}
	pickedBowlers := 0
	for _, br := range bowls {
		if pickedBowlers >= wantBowlers {
			break
		}
		// prefer actual bowlers
		if !candidatesByName(candidates)[br.name].IsBowler && pickedBowlers < 5 {
			// Skip part-time bowlers until we satisfy minimum of 5 specialist bowlers
			continue
		}
		selected[br.name] = true
		pickedBowlers++
	}
	if pickedBowlers < 5 {
		// If somehow fewer than 5 picked (e.g., too few candidates), fill from top wickets
		for _, br := range bowls {
			if pickedBowlers >= 5 {
				break
			}
			if !selected[br.name] {
				selected[br.name] = true
				pickedBowlers++
			}
		}
	}

	// Pick batters from remaining by runs
	pickedBatters := 0
	for _, br := range bats {
		if pickedBatters >= wantBatters {
			break
		}
		if selected[br.name] {
			continue
		}
		selected[br.name] = true
		pickedBatters++
	}

	// If still fewer than 11, fill by best remaining runs
	for _, br := range bats {
		if len(selected) >= 11 {
			break
		}
		if selected[br.name] {
			continue
		}
		selected[br.name] = true
	}

	// Emit team
	fmt.Printf("Team for match %d (bat=%d, bowl=%d)\n", matchID, wantBatters, wantBowlers)
	fmt.Println("-------------------------------------")
	// List bowlers (by wickets) then batters (by runs)
	fmt.Println("Bowlers:")
	count := 0
	for _, br := range bowls {
		if selected[br.name] {
			fmt.Printf(" - %s (pred wickets: %.2f)\n", br.name, br.wkts)
			count++
			if count >= wantBowlers {
				break
			}
		}
	}
	fmt.Println("Batters:")
	// Build set of top N bowlers by wickets to avoid listing them as batters
	topBowlers := make(map[string]bool)
	tcount := 0
	for _, b := range bowls {
		if tcount >= wantBowlers {
			break
		}
		if b.name != "" {
			topBowlers[b.name] = true
			tcount++
		}
	}
	count = 0
	for _, br := range bats {
		if selected[br.name] && !topBowlers[br.name] {
			fmt.Printf(" - %s (pred runs: %.2f)\n", br.name, br.runs)
			count++
			if count >= wantBatters {
				break
			}
		}
	}
}

type wx struct {
	Temp, Wind, Rain, Humidity, Cloud, Pressure int
	Viscosity                                   string
}

func weatherRow(ctx context.Context, matchID int64, session string) wx {
	row := db.Pool.QueryRow(
		ctx,
		`SELECT COALESCE(temp,0), COALESCE(wind,0), COALESCE(rain,0), COALESCE(humidity,0), COALESCE(cloud,0), COALESCE(pressure,0), COALESCE(viscosity,'')
		FROM weather_data WHERE match_id = $1 AND session = $2`,
		matchID,
		session,
	)
	var w wx
	_ = row.Scan(&w.Temp, &w.Wind, &w.Rain, &w.Humidity, &w.Cloud, &w.Pressure, &w.Viscosity)
	return w
}

type candidate struct {
	ID       int64
	Name     string
	IsBowler bool
	BatCons  float32
	BowlCons float32
}

func loadCandidates(ctx context.Context, matchID int64) ([]candidate, error) {
	q := `WITH bat AS (
		SELECT DISTINCT bd.player_id FROM batting_data bd WHERE bd.match_id = $1
	), bowl AS (
		SELECT DISTINCT bw.player_id FROM bowling_data bw WHERE bw.match_id = $1
	), allp AS (
		SELECT player_id, TRUE AS is_bowler FROM bowl
		UNION
		SELECT player_id, FALSE AS is_bowler FROM bat
	)
	SELECT p.id, p.player_name, MAX(CASE WHEN a.is_bowler THEN 1 ELSE 0 END) AS is_bowler,
		COALESCE(p.batting_consistency, 0)::real, COALESCE(p.bowling_consistency, 0)::real
	FROM allp a
	JOIN player p ON p.id = a.player_id
	GROUP BY p.id, p.player_name`
	rows, err := db.Pool.Query(ctx, q, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		var c candidate
		var isb int32
		if err := rows.Scan(&c.ID, &c.Name, &isb, &c.BatCons, &c.BowlCons); err != nil {
			return nil, err
		}
		c.IsBowler = isb == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadFormFmt(ctx context.Context, playerID int64, seasonID *int64, formatID int64, batting bool) float32 {
	if seasonID == nil {
		return 0
	}
	col := "batting_form"
	if !batting {
		col = "bowling_form"
	}
	q := `SELECT COALESCE(` + col + `,0)::real FROM player_form_data_fmt WHERE player_id=$1 AND season_id=$2 AND ($3 = 0 OR format_id=$3)`
	row := db.Pool.QueryRow(ctx, q, playerID, *seasonID, formatID)
	var v float32
	if err := row.Scan(&v); err != nil {
		return 0
	}
	return v
}

func loadVenueFmt(ctx context.Context, playerID int64, venueID *int64, formatID int64, batting bool) float32 {
	if venueID == nil {
		return 0
	}
	col := "batting_venue"
	if !batting {
		col = "bowling_venue"
	}
	q := `SELECT COALESCE(` + col + `,0)::real FROM player_venue_data_fmt WHERE player_id=$1 AND venue_id=$2 AND ($3 = 0 OR format_id=$3)`
	row := db.Pool.QueryRow(ctx, q, playerID, *venueID, formatID)
	var v float32
	if err := row.Scan(&v); err != nil {
		return 0
	}
	return v
}

func loadOppositionFmt(ctx context.Context, playerID int64, oppositionID *int64, formatID int64, batting bool) float32 {
	if oppositionID == nil {
		return 0
	}
	col := "batting_opposition"
	if !batting {
		col = "bowling_opposition"
	}
	q := `SELECT COALESCE(` + col + `,0)::real FROM player_opposition_data_fmt WHERE player_id=$1 AND opposition_id=$2 AND ($3 = 0 OR format_id=$3)`
	row := db.Pool.QueryRow(ctx, q, playerID, *oppositionID, formatID)
	var v float32
	if err := row.Scan(&v); err != nil {
		return 0
	}
	return v
}

func ptrToInt(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func candidatesByName(cs []candidate) map[string]candidate {
	m := make(map[string]candidate, len(cs))
	for _, c := range cs {
		m[c.Name] = c
	}
	return m
}
