package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Competition-level readings: how the level the displayed probability was read at was
// known (H-24).
//
// The display model reads the fixture's level -- international between national sides,
// club otherwise -- as context, the way it reads the toss (ml.xi.contract,
// COMPETITION_LEVEL_COLS). No request names it: a caller picks two sides, not a level. It
// is read off the sides instead, and the reading says so.
const (
	// CompetitionLevelReadingSidesHistory is the level every match both sides have played
	// on record was at. On the archive 531 of 532 clubs have played at exactly one level,
	// so for a fixture between two of them the level is a fact of their history, not an
	// inference; the one club that has played at both (Barbados women, nine club matches
	// and three at the 2022 Commonwealth Games) reads as no single level.
	CompetitionLevelReadingSidesHistory = "sides_history"
	// CompetitionLevelReadingMarginalised is the display averaged over both levels: a side
	// with no single level on record, or two sides whose levels disagree, which no fixture
	// on record has. The average is a substitution and is named as one (§8.7).
	CompetitionLevelReadingMarginalised = "marginalised"
)

// CompetitionLevelReadings returns every value `competition_level.reading` may carry
// (H-24), for the surfaces that have to spell it.
func CompetitionLevelReadings() []string {
	return []string{CompetitionLevelReadingSidesHistory, CompetitionLevelReadingMarginalised}
}

// CompetitionLevelSummary says which competition level the displayed probability was read
// at, and how that level was known.
//
// It is on the wire for the toss's reason: the level is a display-model input, and an
// answer averaged over both levels is a different number from one read at the fixture's --
// #356 measured the two levels of ODI 0.04 apart in display AUC. `reading` is checked
// against what ml-service reports having done before the result is assembled, so it
// describes the answer rather than restating the lookup.
type CompetitionLevelSummary struct {
	// Level is CompetitionInternational or CompetitionClub where one was read, absent
	// where the display averaged both.
	Level string `json:"level,omitempty"`
	// Reading is CompetitionLevelReadingSidesHistory or CompetitionLevelReadingMarginalised.
	Reading string `json:"reading"`
	// Note says why no single level could be read, on a marginalised answer only.
	Note string `json:"note,omitempty"`
}

// CompetitionLevelLookup reads, for each club id, the one level every match the club
// played before the fixture's day was at; a club that has played at both levels, or at
// none, is absent from the map. As-of by the date bound (H-21): what a side's later
// matches say about it is not knowable at this fixture's toss, so a backtest reads the
// history as it stood. It is a parameter so the resolution can be tested without a
// database, and it is read-only by contract.
type CompetitionLevelLookup func(ctx context.Context, clubIDs []int64, before time.Time) (map[int64]string, error)

// resolveCompetitionLevel reads the fixture's level off the two sides' history before the
// fixture's day, keeping the outcomes apart: both sides at one and the same level, a side
// with no single level, two sides whose levels disagree, and a lookup failure -- which is
// returned as itself, so a database fault can never be served as "level unknown".
func resolveCompetitionLevel(
	ctx context.Context,
	team1, team2 db.TeamSide,
	fixtureDay time.Time,
	lookup CompetitionLevelLookup,
) (CompetitionLevelSummary, error) {
	levels, err := lookup(ctx, []int64{team1.ClubID, team2.ClubID}, fixtureDay)
	if err != nil {
		slog.Error("predictteam.PredictTeams competition level lookup failed",
			slog.Int64("team1_club_id", team1.ClubID), slog.Int64("team2_club_id", team2.ClubID), slog.Any("err", err))
		return CompetitionLevelSummary{}, fmt.Errorf("resolve competition level: %w", err)
	}
	level1, known1 := levels[team1.ClubID]
	level2, known2 := levels[team2.ClubID]
	for _, side := range []struct {
		team  db.TeamSide
		known bool
	}{{team1, known1}, {team2, known2}} {
		if !side.known {
			return marginalisedCompetitionLevel(fmt.Sprintf(
				"%s has played at no single competition level on record before %s",
				side.team.Label(), fixtureDay.Format(time.DateOnly))), nil
		}
	}
	if level1 != level2 {
		return marginalisedCompetitionLevel(fmt.Sprintf(
			"%s plays %s cricket and %s plays %s cricket on record; no fixture between them is",
			team1.Label(), level1, team2.Label(), level2)), nil
	}
	return CompetitionLevelSummary{Level: level1, Reading: CompetitionLevelReadingSidesHistory}, nil
}

func marginalisedCompetitionLevel(why string) CompetitionLevelSummary {
	slog.Info("predictteam.PredictTeams reading the display over both competition levels", slog.String("why", why))
	return CompetitionLevelSummary{
		Reading: CompetitionLevelReadingMarginalised,
		Note:    why + "; the displayed probability is averaged over both levels",
	}
}

// refuseCompetitionLevelMismatch refuses an answer whose level reading is not the one that
// was asked for: a level that was sent and averaged over anyway, or the reverse, is the
// request being silently changed, and the `competition_level_marginalised` flag is the one
// field that can catch it (§8.7).
func refuseCompetitionLevelMismatch(step string, summary CompetitionLevelSummary, marginalised bool) error {
	if marginalised == (summary.Level == "") {
		return nil
	}
	sent := "no competition level"
	if summary.Level != "" {
		sent = "competition level " + summary.Level
	}
	return fmt.Errorf("%s: %s was sent but ml-service reports competition_level_marginalised=%t",
		step, sent, marginalised)
}

// productionCompetitionLevelLookup is the lookup the served path uses: one read of the
// levels each club's matches before the fixture's day were played at, and nothing else.
func productionCompetitionLevelLookup(
	ctx context.Context,
	clubIDs []int64,
	before time.Time,
) (map[int64]string, error) {
	return db.CompetitionLevelsByClub(ctx, clubIDs, before)
}
