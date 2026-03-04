package backtest

import (
	"math"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// BuildPredictedScorecard builds a scorecard from the actual layout with ML-predicted stats per player.
// Predictions use only data before the match date. Batting rows get predicted runs; bowling rows get predicted wickets, economy, and derived runs.
// playerTeams maps player_id -> team name so we sum predicted runs for all 11 of the batting team.
func BuildPredictedScorecard(
	actual *db.MatchScorecard,
	preds map[int64]PlayerPredictions,
	playerTeams map[int64]string,
) *db.MatchScorecard {
	if actual == nil {
		return nil
	}
	out := &db.MatchScorecard{
		MatchID:   actual.MatchID,
		MatchDate: actual.MatchDate,
		Venue:     actual.Venue,
		Innings:   make([]db.ScorecardInning, 0, len(actual.Innings)),
	}

	// Build playerID->name map from all innings for DNB entries.
	playerNames := make(map[int64]string)
	for _, inn := range actual.Innings {
		for _, b := range inn.Batting {
			playerNames[b.PlayerID] = b.PlayerName
		}
		for _, w := range inn.Bowling {
			playerNames[w.PlayerID] = w.PlayerName
		}
	}

	for _, in := range actual.Innings {
		inn := db.ScorecardInning{
			InningNumber:    in.InningNumber,
			BattingTeamName: in.BattingTeamName,
			BowlingTeamName: in.BowlingTeamName,
			Extras:          in.Extras,
			TargetRuns:      in.TargetRuns,
			Batting:         make([]db.ScorecardBatting, 0, 11),
			Bowling:         make([]db.ScorecardBowling, 0, len(in.Bowling)),
		}

		// Sum predicted runs for ALL players in the batting team (full XI).
		var predRunsSum int
		battedSet := make(map[int64]struct{})
		for _, b := range in.Batting {
			battedSet[b.PlayerID] = struct{}{}
		}
		for pid, team := range playerTeams {
			if team != in.BattingTeamName {
				continue
			}
			p := preds[pid]
			predRunsSum += int(math.Round(p.Runs))
		}

		// Build batting rows: those who batted first, then DNB entries.
		for _, b := range in.Batting {
			p := preds[b.PlayerID]
			r := int(math.Round(p.Runs))
			bl := int(math.Round(p.Balls))
			f := int(math.Round(p.Fours))
			s := int(math.Round(p.Sixes))
			var strikeRate *float32
			if p.Balls > 0 {
				sr := float32(100.0 * p.Runs / p.Balls)
				strikeRate = &sr
			}
			inn.Batting = append(inn.Batting, db.ScorecardBatting{
				PlayerID:   b.PlayerID,
				PlayerName: b.PlayerName,
				Runs:       intPtr(r),
				Balls:      &bl,
				Fours:      &f,
				Sixes:      &s,
				StrikeRate: strikeRate,
				HowOut:     nil,
			})
		}

		// Add "Did Not Bat" rows for batting team players who didn't bat.
		for pid, team := range playerTeams {
			if team != in.BattingTeamName {
				continue
			}
			if _, batted := battedSet[pid]; batted {
				continue
			}
			battedSet[pid] = struct{}{}
			p := preds[pid]
			r := int(math.Round(p.Runs))
			inn.Batting = append(inn.Batting, db.ScorecardBatting{
				PlayerID:   pid,
				PlayerName: playerNames[pid],
				Runs:       intPtr(r),
			})
		}
		inn.RunsScored = predRunsSum

		// Bowling: collect raw wicket predictions, then scale to max 10 per inning.
		type bowlerPred struct {
			w    db.ScorecardBowling
			wkts float64
		}
		var bowlerPreds []bowlerPred
		var predWicketsSum float64
		for _, w := range in.Bowling {
			p := preds[w.PlayerID]
			wkts := math.Max(0, p.Wickets)
			predWicketsSum += wkts
			ec := float32(p.Economy)
			var predRuns *int
			if w.Balls != nil && *w.Balls > 0 {
				predRuns = intPtr(int(math.Round(float64(*w.Balls) * p.Economy / 6)))
			} else if w.Overs != nil && *w.Overs > 0 {
				ov := float64(*w.Overs)
				whole := int(ov)
				frac := ov - float64(whole)
				ballsInOver := int(math.Round(frac * 10))
				if ballsInOver > 5 {
					ballsInOver = 5
				}
				totalBalls := whole*6 + ballsInOver
				predRuns = intPtr(int(math.Round(float64(totalBalls) * p.Economy / 6)))
			}
			bowlerPreds = append(bowlerPreds, bowlerPred{
				w: db.ScorecardBowling{
					PlayerID:   w.PlayerID,
					PlayerName: w.PlayerName,
					Overs:      w.Overs,
					Runs:       predRuns,
					Economy:    &ec,
					Balls:      w.Balls,
				},
				wkts: wkts,
			})
		}

		const maxWicketsPerInning = 10
		scale := 1.0
		if predWicketsSum > maxWicketsPerInning && predWicketsSum > 0 {
			scale = maxWicketsPerInning / predWicketsSum
		}
		var scaledSum int
		for _, bp := range bowlerPreds {
			scaled := int(math.Round(bp.wkts * scale))
			scaledSum += scaled
			bp.w.Wickets = intPtr(scaled)
			inn.Bowling = append(inn.Bowling, bp.w)
		}
		inn.WicketsLost = scaledSum
		out.Innings = append(out.Innings, inn)
	}
	return out
}

// RescalePredictionsToWinProbability rescales each player's predicted runs and wickets
// so that team1 total = total*p and team2 total = total*(1-p), preserving proportions within each team.
func RescalePredictionsToWinProbability(
	resp *EvaluateResponse,
	playerTeams map[int64]string,
	team1, team2 string,
	p float64,
) {
	var runs1, runs2, wkt1, wkt2 float64
	for i := range resp.Players {
		pid := resp.Players[i].PlayerID
		t := playerTeams[pid]
		var r, w float64
		if resp.Players[i].Predicted != nil {
			r = resp.Players[i].Predicted["runs"]
			w = resp.Players[i].Predicted["wickets"]
		}
		switch t {
		case team1:
			runs1 += r
			wkt1 += w
		case team2:
			runs2 += r
			wkt2 += w
		}
	}
	totalRuns := runs1 + runs2
	totalWickets := wkt1 + wkt2
	factorR1, factorR2 := 1.0, 1.0
	if totalRuns > 0 {
		if runs1 > 0 {
			factorR1 = (totalRuns * p) / runs1
		}
		if runs2 > 0 {
			factorR2 = (totalRuns * (1 - p)) / runs2
		}
	}
	factorW1, factorW2 := 1.0, 1.0
	if totalWickets > 0 {
		if wkt1 > 0 {
			factorW1 = (totalWickets * p) / wkt1
		}
		if wkt2 > 0 {
			factorW2 = (totalWickets * (1 - p)) / wkt2
		}
	}
	for i := range resp.Players {
		t := playerTeams[resp.Players[i].PlayerID]
		if resp.Players[i].Predicted == nil {
			continue
		}
		switch t {
		case team1:
			if v, ok := resp.Players[i].Predicted["runs"]; ok {
				resp.Players[i].Predicted["runs"] = v * factorR1
			}
			if v, ok := resp.Players[i].Predicted["wickets"]; ok {
				resp.Players[i].Predicted["wickets"] = v * factorW1
			}
		case team2:
			if v, ok := resp.Players[i].Predicted["runs"]; ok {
				resp.Players[i].Predicted["runs"] = v * factorR2
			}
			if v, ok := resp.Players[i].Predicted["wickets"]; ok {
				resp.Players[i].Predicted["wickets"] = v * factorW2
			}
		}
	}
}

// BuildBacktestWinFeatures constructs win model features from match context and player features.
func BuildBacktestWinFeatures(
	winCtx *db.MatchWinContext,
	format string,
	playerTeams map[int64]string,
	team1, team2 string,
	features map[int64]map[string]float64,
) predictteam.WinFeatures {
	sumForTeam := func(team string, key string) float64 {
		s := 0.0
		for pid, t := range playerTeams {
			if t != team {
				continue
			}
			if m := features[pid]; m != nil {
				s += m[key]
			}
		}
		return s
	}
	venueID := 0
	if winCtx.VenueID != 0 {
		venueID = int(winCtx.VenueID)
	}
	return predictteam.WinFeatures{
		FormatID:                int(winCtx.FormatID),
		VenueID:                 venueID,
		Team1OppositionID:       int(winCtx.Team1OppositionID),
		Team2OppositionID:       int(winCtx.Team2OppositionID),
		TossWinnerOppositionID:  int(winCtx.TossWinnerOppositionID),
		Team1BatConsistencySum:  sumForTeam(team1, "batting_consistency"),
		Team1BowlConsistencySum: sumForTeam(team1, "bowling_consistency"),
		Team2BatConsistencySum:  sumForTeam(team2, "batting_consistency"),
		Team2BowlConsistencySum: sumForTeam(team2, "bowling_consistency"),
		Team1BatFormSum:         sumForTeam(team1, "batting_form"),
		Team1BowlFormSum:        sumForTeam(team1, "bowling_form"),
		Team2BatFormSum:         sumForTeam(team2, "batting_form"),
		Team2BowlFormSum:        sumForTeam(team2, "bowling_form"),
		Format:                  format,
	}
}

func intPtr(n int) *int { return &n }
