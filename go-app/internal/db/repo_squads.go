package db

import (
    "context"
    "errors"
    "fmt"
    "strings"
    "time"
)

// PlayerPredictionRow holds numeric features expected by the ML service for one player.
type PlayerPredictionRow struct {
    PlayerName      string
    RunsScored      float64
    BallsFaced      float64
    FoursScored     float64
    SixesScored     float64
    BattingPosition float64
    StrikeRate      float64
    RunsConceded    float64
    Deliveries      float64
    WicketsTaken    float64
    Econ            float64
}

// SquadRow represents one team squad for a match.
type SquadRow struct {
    TeamName  string
    ActualWin int // 0|1
    Players   []PlayerPredictionRow
}

// MatchSquads aggregates the two squads and match metadata.
type MatchSquads struct {
    MatchID int64
    Date    time.Time
    Teams   [2]string
    Squads  [2]SquadRow
}

var (
    // ErrMatchNotFound indicates the match id does not exist.
    ErrMatchNotFound = errors.New("match_not_found")
    // ErrIncompleteSquads indicates fewer than two squads or missing players.
    ErrIncompleteSquads = errors.New("incomplete_squads")
)

// GetMatchSquads returns the actual squads for a match with features computed as-of the given date.
// NOTE: This is a stub to be implemented in the next step.
func GetMatchSquads(ctx context.Context, matchID int64, asof time.Time, format string) (MatchSquads, error) {
    if Pool == nil {
        return MatchSquads{}, errors.New("db pool not initialized")
    }

    // 1) Resolve match date
    var matchDate time.Time
    err := Pool.QueryRow(ctx, `SELECT date FROM match_details WHERE match_id = $1`, matchID).Scan(&matchDate)
    if err != nil {
        // No rows or other error → treat as not found for contract simplicity
        return MatchSquads{}, ErrMatchNotFound
    }

    // 2) Resolve two teams and their result flags
    rows, err := Pool.Query(ctx, `
        SELECT t.name AS team_name, COALESCE(tm.result, '') AS result
        FROM team_match tm
        JOIN team t ON t.id = tm.team_id
        WHERE tm.match_id = $1
        ORDER BY t.name ASC
    `, matchID)
    if err != nil {
        return MatchSquads{}, err
    }
    defer rows.Close()

    type teamInfo struct{ name string; win int }
    teams := make([]teamInfo, 0, 2)
    for rows.Next() {
        var name, result string
        if err := rows.Scan(&name, &result); err != nil {
            return MatchSquads{}, err
        }
        win := 0
        // Map common result encodings to 0|1; adjust if schema differs
        switch strings.ToUpper(strings.TrimSpace(result)) {
        case "W", "WIN", "1", "TRUE", "T":
            win = 1
        default:
            win = 0
        }
        teams = append(teams, teamInfo{name: name, win: win})
    }
    if err := rows.Err(); err != nil {
        return MatchSquads{}, err
    }
    if len(teams) != 2 {
        return MatchSquads{}, ErrIncompleteSquads
    }

    // Helper to load actual XI player ids for a team
    fetchPlayers := func(ctx context.Context, mid int64, teamName string) ([]int64, []string, error) {
        // Resolve team_id by name
        var teamID int64
        if err := Pool.QueryRow(ctx, `SELECT id FROM team WHERE name = $1`, teamName).Scan(&teamID); err != nil {
            return nil, nil, fmt.Errorf("resolve team id: %w", err)
        }
        // Resolve actual XI (assumes player_match table). If different, adjust join accordingly.
        r, err := Pool.Query(ctx, `
            SELECT pm.player_id, p.name
            FROM player_match pm
            JOIN player p ON p.id = pm.player_id
            WHERE pm.match_id = $1 AND pm.team_id = $2
            ORDER BY p.name ASC
        `, mid, teamID)
        if err != nil {
            return nil, nil, err
        }
        defer r.Close()
        ids := make([]int64, 0, 11)
        names := make([]string, 0, 11)
        for r.Next() {
            var pid int64
            var pname string
            if err := r.Scan(&pid, &pname); err != nil {
                return nil, nil, err
            }
            ids = append(ids, pid)
            names = append(names, pname)
        }
        if err := r.Err(); err != nil {
            return nil, nil, err
        }
        return ids, names, nil
    }

    // Helper to fetch a single player's feature row as-of date
    fetchPlayerFeatures := func(ctx context.Context, playerID int64) (PlayerPredictionRow, error) {
        // Default zeros
        out := PlayerPredictionRow{PlayerName: ""}
        // Resolve player name
        if err := Pool.QueryRow(ctx, `SELECT name FROM player WHERE id = $1`, playerID).Scan(&out.PlayerName); err != nil {
            // keep empty name on error; not fatal
            out.PlayerName = fmt.Sprintf("player_%d", playerID)
        }
        // Batting features (latest as-of)
        var (
            runsScored, ballsFaced, fours, sixes, batPos, sr float64
        )
        // Bowling features (latest as-of)
        var (
            runsConc, deliveries, wkts, econ float64
        )

        // Optional format filter; if schema lacks format, omit condition
        batQuery := `
            SELECT
                COALESCE(runs_scored,0), COALESCE(balls_faced,0), COALESCE(fours_scored,0), COALESCE(sixes_scored,0),
                COALESCE(batting_position,0), COALESCE(strike_rate,0)
            FROM batting_features
            WHERE player_id = $1 AND feature_date <= $2` + func() string {
            if format != "" { return " AND format_code = $3" }
            return ""
        }() + `
            ORDER BY feature_date DESC
            LIMIT 1`
        if format != "" {
            _ = Pool.QueryRow(ctx, batQuery, playerID, asof, format).Scan(&runsScored, &ballsFaced, &fours, &sixes, &batPos, &sr)
        } else {
            _ = Pool.QueryRow(ctx, batQuery, playerID, asof).Scan(&runsScored, &ballsFaced, &fours, &sixes, &batPos, &sr)
        }

        bowlQuery := `
            SELECT
                COALESCE(runs_conceded,0), COALESCE(deliveries,0), COALESCE(wickets_taken,0), COALESCE(econ,0)
            FROM bowling_features
            WHERE player_id = $1 AND feature_date <= $2` + func() string {
            if format != "" { return " AND format_code = $3" }
            return ""
        }() + `
            ORDER BY feature_date DESC
            LIMIT 1`
        if format != "" {
            _ = Pool.QueryRow(ctx, bowlQuery, playerID, asof, format).Scan(&runsConc, &deliveries, &wkts, &econ)
        } else {
            _ = Pool.QueryRow(ctx, bowlQuery, playerID, asof).Scan(&runsConc, &deliveries, &wkts, &econ)
        }

        out.RunsScored = runsScored
        out.BallsFaced = ballsFaced
        out.FoursScored = fours
        out.SixesScored = sixes
        out.BattingPosition = batPos
        out.StrikeRate = sr
        out.RunsConceded = runsConc
        out.Deliveries = deliveries
        out.WicketsTaken = wkts
        out.Econ = econ
        return out, nil
    }

    // Build squads for both teams
    makeSquad := func(team teamInfo) (SquadRow, error) {
        ids, names, err := fetchPlayers(ctx, matchID, team.name)
        if err != nil {
            return SquadRow{}, err
        }
        if len(ids) == 0 {
            return SquadRow{}, ErrIncompleteSquads
        }
        players := make([]PlayerPredictionRow, 0, len(ids))
        for i, pid := range ids {
            pr, _ := fetchPlayerFeatures(ctx, pid)
            // ensure name set from names list if feature lookup failed
            if pr.PlayerName == "" && i < len(names) {
                pr.PlayerName = names[i]
            }
            players = append(players, pr)
        }
        return SquadRow{
            TeamName:  team.name,
            ActualWin: team.win,
            Players:   players,
        }, nil
    }

    s0, err := makeSquad(teams[0])
    if err != nil {
        if errors.Is(err, ErrIncompleteSquads) {
            return MatchSquads{}, ErrIncompleteSquads
        }
        return MatchSquads{}, err
    }
    s1, err := makeSquad(teams[1])
    if err != nil {
        if errors.Is(err, ErrIncompleteSquads) {
            return MatchSquads{}, ErrIncompleteSquads
        }
        return MatchSquads{}, err
    }

    return MatchSquads{
        MatchID: matchID,
        Date:    matchDate,
        Teams:   [2]string{teams[0].name, teams[1].name},
        Squads:  [2]SquadRow{s0, s1},
    }, nil
}
