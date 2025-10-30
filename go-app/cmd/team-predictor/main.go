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

	"github.com/umayangag/cric-app/go-app/internal/contracts"
	"github.com/umayangag/cric-app/go-app/internal/db"
	"github.com/umayangag/cric-app/go-app/internal/mlclient"
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
	flag.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	flag.IntVar(&wantBatters, "bat", 6, "number of batters to pick (default 6)")
	flag.IntVar(&wantBowlers, "bowl", 5, "number of bowlers to pick (default 5, minimum 5)")
	flag.Parse()

	if matchID == 0 {
		fmt.Fprintln(os.Stderr, "usage: team-predictor -match=<match_id> [-bat=6] [-bowl=5]")
		os.Exit(2)
	}
	if wantBowlers < 5 {
		log.Printf("requested bowlers=%d < 5; adjusting to 5 to satisfy minimum", wantBowlers)
		wantBowlers = 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Load match context
	var inning int
	var battingSession, bowlingSession, toss string
	var seasonID, venueID, oppositionID *int64
	row := db.Pool.QueryRow(ctx, `SELECT inning, batting_session, bowling_session, toss, season_id, venue_id, opposition_id
		FROM match_details WHERE match_id = $1`, matchID)
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
			BattingForm:        loadForm(ctx, p.ID, seasonID, true),
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
			Venue:              loadVenue(ctx, p.ID, venueID, true),
			Opposition:         loadOpposition(ctx, p.ID, oppositionID, true),
			Season:             ptrToInt(seasonID),
			PlayerName:         p.Name,
		}
		batFeats = append(batFeats, bf)

		wf := contracts.BowlingFeatures{
			BowlingConsistency: p.BowlCons,
			BowlingForm:        loadForm(ctx, p.ID, seasonID, false),
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
			BowlingVenue:       loadVenue(ctx, p.ID, venueID, false),
			BowlingOpposition:  loadOpposition(ctx, p.ID, oppositionID, false),
			Season:             ptrToInt(seasonID),
			PlayerName:         p.Name,
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
	type batRank struct { name string; runs float32; isBowler bool }
	type bowlRank struct { name string; wkts float32 }
	bats := make([]batRank, 0, len(candidates))
	bowls := make([]bowlRank, 0, len(candidates))
	for i, p := range candidates {
		bats = append(bats, batRank{name: p.Name, runs: batPreds[i].RunsScored, isBowler: p.IsBowler})
		bowls = append(bowls, bowlRank{name: p.Name, wkts: bowlPreds[i].WicketsTaken})
	}
	sort.Slice(bats, func(i, j int) bool { return bats[i].runs > bats[j].runs })
	sort.Slice(bowls, func(i, j int) bool { return bowls[i].wkts > bowls[j].wkts })

	// Pick bowlers first to satisfy minimum 5
	selected := map[string]bool{}
	pickedBowlers := 0
	for _, br := range bowls {
		if pickedBowlers >= wantBowlers { break }
		// prefer actual bowlers
		if !candidatesByName(candidates)[br.name].IsBowler && pickedBowlers < 5 {
			// allow part-time bowlers only after satisfying minimum? On happy path, we accept top wicket preds.
		}
		selected[br.name] = true
		pickedBowlers++
	}
	if pickedBowlers < 5 {
		// If somehow fewer than 5 picked (e.g., too few candidates), fill from top wickets
		for _, br := range bowls {
			if pickedBowlers >= 5 { break }
			if !selected[br.name] {
				selected[br.name] = true
				pickedBowlers++
			}
		}
	}

	// Pick batters from remaining by runs
	pickedBatters := 0
	for _, br := range bats {
		if pickedBatters >= wantBatters { break }
		if selected[br.name] { continue }
		selected[br.name] = true
		pickedBatters++
	}

	// If still fewer than 11, fill by best remaining runs
	for _, br := range bats {
		if len(selected) >= 11 { break }
		if selected[br.name] { continue }
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
			if count >= wantBowlers { break }
		}
	}
	fmt.Println("Batters:")
	count = 0
	for _, br := range bats {
		if selected[br.name] && !inTop(bowls, br.name, wantBowlers) {
			fmt.Printf(" - %s (pred runs: %.2f)\n", br.name, br.runs)
			count++
			if count >= wantBatters { break }
		}
	}
}

type wx struct {
	Temp, Wind, Rain, Humidity, Cloud, Pressure int
	Viscosity string
}

func weatherRow(ctx context.Context, matchID int64, session string) wx {
	row := db.Pool.QueryRow(ctx, `SELECT COALESCE(temp,0), COALESCE(wind,0), COALESCE(rain,0), COALESCE(humidity,0), COALESCE(cloud,0), COALESCE(pressure,0), COALESCE(viscosity,'')
		FROM weather_data WHERE match_id = $1 AND session = $2`, matchID, session)
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
	if err != nil { return nil, err }
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		var c candidate
		var isb int32
		if err := rows.Scan(&c.ID, &c.Name, &isb, &c.BatCons, &c.BowlCons); err != nil { return nil, err }
		c.IsBowler = isb == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadForm(ctx context.Context, playerID int64, seasonID *int64, batting bool) float32 {
	if seasonID == nil { return 0 }
	col := "batting_form"
	if !batting { col = "bowling_form" }
	row := db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(%s,0)::real FROM player_form_data WHERE player_id=$1 AND season_id=$2`, col), playerID, *seasonID)
	var v float32
	if err := row.Scan(&v); err != nil { return 0 }
	return v
}

func loadVenue(ctx context.Context, playerID int64, venueID *int64, batting bool) float32 {
	if venueID == nil { return 0 }
	col := "batting_venue"
	if !batting { col = "bowling_venue" }
	row := db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(%s,0)::real FROM player_venue_data WHERE player_id=$1 AND venue_id=$2`, col), playerID, *venueID)
	var v float32
	if err := row.Scan(&v); err != nil { return 0 }
	return v
}

func loadOpposition(ctx context.Context, playerID int64, oppositionID *int64, batting bool) float32 {
	if oppositionID == nil { return 0 }
	col := "batting_opposition"
	if !batting { col = "bowling_opposition" }
	row := db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(%s,0)::real FROM player_opposition_data WHERE player_id=$1 AND opposition_id=$2`, col), playerID, *oppositionID)
	var v float32
	if err := row.Scan(&v); err != nil { return 0 }
	return v
}

func ptrToInt(p *int64) int {
	if p == nil { return 0 }
	return int(*p)
}

func inTop(b []bowlRank, name string, n int) bool {
	count := 0
	for _, x := range b {
		if count >= n { break }
		if x.name == name { return true }
		if x.name != "" { count++ }
	}
	return false
}

func candidatesByName(cs []candidate) map[string]candidate {
	m := make(map[string]candidate, len(cs))
	for _, c := range cs { m[c.Name] = c }
	return m
}
