package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// TeamSide is one club as a fixture names it: the club id every downstream read is keyed by,
// the name that club currently plays under, and its gender.
//
// The club id is COALESCE(canonical_id, id) -- a club that renamed is one club (I-4) -- and
// the row it names is gendered, because opposition is keyed (opposition_name, gender) since
// the identity migration. So a club id is a *side*, and it is the only identifier on this
// path that cannot mean two teams at once.
type TeamSide struct {
	ClubID int64
	Name   string
	Gender string
}

// Label is the side as a person reads it: "India (men)".
func (s TeamSide) Label() string { return teams.SideLabel(s.Name, s.Gender) }

// TeamRef is how a request names a side, in the order of decreasing doubt: a club id says
// exactly which side; a name with a gender says it as well but has to be looked up; a bare
// name may say nothing at all, and where it does not, resolving it is refused rather than
// guessed (D-11).
type TeamRef struct {
	ClubID int64
	Name   string
	Gender string
}

// IsEmpty reports whether the reference names nothing.
func (r TeamRef) IsEmpty() bool { return r.ClubID == 0 && strings.TrimSpace(r.Name) == "" }

// ErrOppositionNotFound reports that no side matching the reference has played the format.
var ErrOppositionNotFound = errors.New("no opposition found for team")

// AmbiguousTeamNameError reports a team name that names more than one side in the format.
//
// It carries the candidates rather than a count because the caller's next move is to pick
// one, and the answer to "which did you mean?" is the list itself.
type AmbiguousTeamNameError struct {
	Name       string
	FormatCode string
	Candidates []TeamSide
}

func (e *AmbiguousTeamNameError) Error() string {
	return fmt.Sprintf("%q names more than one side in %s: %s",
		e.Name, e.FormatCode, strings.Join(e.CandidateLabels(), ", "))
}

// CandidateLabels lists the sides the name could have meant, for an error payload.
func (e *AmbiguousTeamNameError) CandidateLabels() []string {
	labels := make([]string, 0, len(e.Candidates))
	for _, side := range e.Candidates {
		labels = append(labels, side.Label())
	}
	return labels
}

// The side queries share one shape: fold every opposition row onto its club, keep the clubs
// that have played the format, and describe each by the row the club id points at -- so a
// club that renamed is listed once, under the name it plays under now.
const (
	teamSideSelect = `
		SELECT club.id, club.opposition_name, club.gender
		FROM opposition o
		JOIN opposition club ON club.id = COALESCE(o.canonical_id, o.id)
		JOIN match_inning mi
		  ON mi.batting_team_opposition_id = o.id OR mi.bowling_team_opposition_id = o.id
		JOIN match m ON m.match_id = mi.match_id
		JOIN match_format mf ON mf.id = m.format_id
		WHERE mf.code = $1`

	teamSideGroup = `
		GROUP BY club.id, club.opposition_name, club.gender
		ORDER BY club.opposition_name, club.gender`

	// A name is matched as played, not as the club is called now: a request naming a club
	// by a name it has retired is asking about the club.
	teamSideByName = ` AND o.opposition_name = $2`

	teamSideByClubID = ` AND COALESCE(o.canonical_id, o.id) = $2`

	// Opponents are the other side of a match this club played, folded onto their club.
	opponentSideSelect = `
		SELECT club.id, club.opposition_name, club.gender
		FROM match_inning mi
		JOIN match m ON m.match_id = mi.match_id
		JOIN match_format mf ON mf.id = m.format_id
		JOIN opposition bat ON bat.id = mi.batting_team_opposition_id
		JOIN opposition bowl ON bowl.id = mi.bowling_team_opposition_id
		JOIN opposition club ON club.id = CASE
		  WHEN COALESCE(bat.canonical_id, bat.id) = $2 THEN COALESCE(bowl.canonical_id, bowl.id)
		  ELSE COALESCE(bat.canonical_id, bat.id) END
		WHERE mf.code = $1
		  AND $2 IN (COALESCE(bat.canonical_id, bat.id), COALESCE(bowl.canonical_id, bowl.id))
		  AND COALESCE(bat.canonical_id, bat.id) <> COALESCE(bowl.canonical_id, bowl.id)`
)

// queryTeamSides runs one of the side queries and reads the rows.
func queryTeamSides(ctx context.Context, query string, args ...any) ([]TeamSide, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sides := make([]TeamSide, 0)
	for rows.Next() {
		var side TeamSide
		if err := rows.Scan(&side.ClubID, &side.Name, &side.Gender); err != nil {
			return nil, err
		}
		sides = append(sides, side)
	}
	return sides, rows.Err()
}

// ListTeamSidesForFormat returns every side that has played the format, each once.
//
// It replaces the name-only list the picker used to read. That list could not name a side:
// it showed "India" once for two teams, so whichever the user picked, the request said the
// same thing and the resolver chose.
func ListTeamSidesForFormat(ctx context.Context, formatCode string) ([]TeamSide, error) {
	return queryTeamSides(ctx, teamSideSelect+teamSideGroup, formatCode)
}

// ListOpponentSidesForFormat returns the sides this club has played in the format.
func ListOpponentSidesForFormat(ctx context.Context, formatCode string, clubID int64) ([]TeamSide, error) {
	return queryTeamSides(ctx, opponentSideSelect+teamSideGroup, formatCode, clubID)
}

// ResolveTeamSide turns a reference into the one side it names, or refuses.
//
// Three cases, and the third is D-11's: a club id resolves to itself; a name with a gender
// resolves to at most one row, because (opposition_name, gender) is unique; a bare name
// resolves only when the format holds exactly one side of that name, and otherwise returns
// an *AmbiguousTeamNameError naming the candidates. Nothing here picks for the caller.
func ResolveTeamSide(ctx context.Context, ref TeamRef, formatCode string) (TeamSide, error) {
	if ref.ClubID != 0 {
		return oneSide(ctx, teamSideSelect+teamSideByClubID+teamSideGroup,
			fmt.Sprintf("club %d", ref.ClubID), formatCode, ref.ClubID)
	}

	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return TeamSide{}, fmt.Errorf("%w: the request named no side", ErrOppositionNotFound)
	}

	candidates, err := queryTeamSides(ctx, teamSideSelect+teamSideByName+teamSideGroup, formatCode, name)
	if err != nil {
		return TeamSide{}, err
	}
	if gender := teams.NormalizeGender(ref.Gender); gender != "" {
		candidates = sidesWithGender(candidates, gender)
		if len(candidates) == 0 {
			return TeamSide{}, fmt.Errorf("%w: %q (%s) in %s",
				ErrOppositionNotFound, name, gender, formatCode)
		}
	}
	switch len(candidates) {
	case 0:
		return TeamSide{}, fmt.Errorf("%w: %q in %s", ErrOppositionNotFound, name, formatCode)
	case 1:
		return candidates[0], nil
	default:
		return TeamSide{}, &AmbiguousTeamNameError{Name: name, FormatCode: formatCode, Candidates: candidates}
	}
}

// oneSide reads a query that must return exactly one side.
func oneSide(ctx context.Context, query, subject, formatCode string, arg any) (TeamSide, error) {
	sides, err := queryTeamSides(ctx, query, formatCode, arg)
	if err != nil {
		return TeamSide{}, err
	}
	if len(sides) == 0 {
		return TeamSide{}, fmt.Errorf("%w: %s in %s", ErrOppositionNotFound, subject, formatCode)
	}
	return sides[0], nil
}

// sidesWithGender keeps the candidates of one gender.
func sidesWithGender(candidates []TeamSide, gender string) []TeamSide {
	kept := make([]TeamSide, 0, len(candidates))
	for _, side := range candidates {
		if teams.NormalizeGender(side.Gender) == gender {
			kept = append(kept, side)
		}
	}
	return kept
}
