package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// TeamRename is one club's change of name: both rows are (name, gender). See
// internal/teamlineage.
type TeamRename struct {
	FromName string
	ToName   string
	Gender   string
}

// String names a rename the way a log line or an operator screen wants it.
func (r TeamRename) String() string {
	return fmt.Sprintf("%s -> %s (%s)", r.FromName, r.ToName, r.Gender)
}

// TeamLineageState is what the archive says about one rename in the reviewed mapping.
//
// The three states exist because "no rows changed" used to mean all of them at once. A
// lineage pass that reports only a row count cannot say whether it linked nothing because
// everything was already linked, because the dataset stops before the rename, or because
// it never ran at all -- and the last of those is a corrupted archive that looks healthy.
type TeamLineageState string

const (
	// TeamLineageLinked: both rows are in the archive and the superseded one points at
	// the current one. The club is one club.
	TeamLineageLinked TeamLineageState = "linked"
	// TeamLineageUnlinked: both rows are in the archive and the link was never written.
	// This is the state an import that stopped before settlement leaves behind, and the
	// only one of the three that is a defect: the club is two clubs, with two Elo
	// histories, and nothing else in the system will notice.
	TeamLineageUnlinked TeamLineageState = "unlinked"
	// TeamLineageAbsent: one side of the rename has not played in this archive. The
	// mapping describes cricket, not one import, so this is reported, not an error.
	TeamLineageAbsent TeamLineageState = "absent"
)

// TeamLineageRename is one rename's standing in the archive.
type TeamLineageRename struct {
	Rename TeamRename
	State  TeamLineageState
	// Changed is true when the call that produced this report wrote the link itself,
	// which distinguishes a link this import made from one an earlier import made.
	Changed bool
}

// TeamLineageReport is the standing of every rename in the mapping that was asked about.
//
// It is deliberately the whole list rather than a count: the counts below are derived from
// it, and the operator surface needs to name the renames that are wrong.
type TeamLineageReport struct {
	Renames []TeamLineageRename
}

// Requested is how many renames the mapping asked about.
func (r TeamLineageReport) Requested() int { return len(r.Renames) }

// Count is how many renames are in the given state.
func (r TeamLineageReport) Count(state TeamLineageState) int {
	n := 0
	for _, rename := range r.Renames {
		if rename.State == state {
			n++
		}
	}
	return n
}

// Changed is how many links the call that produced this report wrote.
func (r TeamLineageReport) Changed() int {
	n := 0
	for _, rename := range r.Renames {
		if rename.Changed {
			n++
		}
	}
	return n
}

// InState names the renames in the given state, for a log line or an operator screen.
func (r TeamLineageReport) InState(state TeamLineageState) []string {
	out := []string{}
	for _, rename := range r.Renames {
		if rename.State == state {
			out = append(out, rename.Rename.String())
		}
	}
	return out
}

// ApplyTeamLineage points every superseded team row at the club's current row and reports
// where each rename stands afterwards.
//
// Both rows have to exist for a rename to mean anything, and a dataset that stops at the
// boundary legitimately has only the old one; that is reported as TeamLineageAbsent rather
// than treated as an error. A rename an earlier import already linked comes back
// TeamLineageLinked with Changed false -- which is the distinction the row count alone
// could not make, and the reason this returns a report.
func ApplyTeamLineage(ctx context.Context, renames []TeamRename) (TeamLineageReport, error) {
	if Pool == nil {
		return TeamLineageReport{}, errors.New("db pool not initialized")
	}
	if len(renames) == 0 {
		return TeamLineageReport{}, nil
	}
	changed, err := linkSupersededTeams(ctx, renames)
	if err != nil {
		return TeamLineageReport{}, err
	}
	report, err := TeamLineageCoverage(ctx, renames)
	if err != nil {
		return TeamLineageReport{}, err
	}
	for i := range report.Renames {
		report.Renames[i].Changed = changed[report.Renames[i].Rename]
	}
	return report, nil
}

// linkSupersededTeams writes the links that are missing and returns the renames it wrote,
// in one statement so a lineage pass is all-or-nothing rather than partly applied.
func linkSupersededTeams(ctx context.Context, renames []TeamRename) (map[TeamRename]bool, error) {
	from, to, gender := teamRenameColumns(renames)
	rows, err := Pool.Query(ctx, `
		UPDATE opposition predecessor
		SET canonical_id = successor.id
		FROM unnest($1::text[], $2::text[], $3::text[]) AS rename(from_name, to_name, gender)
		JOIN opposition successor
		  ON successor.opposition_name = rename.to_name AND successor.gender = rename.gender
		WHERE predecessor.opposition_name = rename.from_name
		  AND predecessor.gender = rename.gender
		  AND predecessor.id <> successor.id
		  AND predecessor.canonical_id IS DISTINCT FROM successor.id
		RETURNING rename.from_name, rename.to_name, rename.gender`, from, to, gender)
	if err != nil {
		slog.Error("apply team lineage failed", slog.Int("renames", len(renames)), slog.Any("err", err))
		return nil, fmt.Errorf("apply team lineage: %w", err)
	}
	defer rows.Close()
	changed := map[TeamRename]bool{}
	for rows.Next() {
		var rename TeamRename
		if err := rows.Scan(&rename.FromName, &rename.ToName, &rename.Gender); err != nil {
			slog.Error("apply team lineage scan failed", slog.Any("err", err))
			return nil, fmt.Errorf("apply team lineage: %w", err)
		}
		changed[rename] = true
	}
	if err := rows.Err(); err != nil {
		slog.Error("apply team lineage read failed", slog.Any("err", err))
		return nil, fmt.Errorf("apply team lineage: %w", err)
	}
	return changed, nil
}

// TeamLineageCoverage reports where each rename in the mapping stands, without writing.
//
// This is what /ops/status asks, and it asks the archive rather than the last import: a
// run that aborted before settlement wrote nothing and logged nothing, so the only honest
// source for "is this archive settled?" is the archive.
func TeamLineageCoverage(ctx context.Context, renames []TeamRename) (TeamLineageReport, error) {
	if Pool == nil {
		return TeamLineageReport{}, errors.New("db pool not initialized")
	}
	if len(renames) == 0 {
		return TeamLineageReport{}, nil
	}
	from, to, gender := teamRenameColumns(renames)
	rows, err := Pool.Query(ctx, `
		SELECT rename.from_name, rename.to_name, rename.gender,
		       predecessor.id IS NOT NULL AND successor.id IS NOT NULL AS both_present,
		       predecessor.canonical_id IS NOT DISTINCT FROM successor.id AS linked
		FROM unnest($1::text[], $2::text[], $3::text[])
		     WITH ORDINALITY AS rename(from_name, to_name, gender, position)
		LEFT JOIN opposition predecessor
		  ON predecessor.opposition_name = rename.from_name AND predecessor.gender = rename.gender
		LEFT JOIN opposition successor
		  ON successor.opposition_name = rename.to_name AND successor.gender = rename.gender
		ORDER BY rename.position`, from, to, gender)
	if err != nil {
		slog.Error("team lineage coverage failed", slog.Int("renames", len(renames)), slog.Any("err", err))
		return TeamLineageReport{}, fmt.Errorf("team lineage coverage: %w", err)
	}
	defer rows.Close()
	report := TeamLineageReport{Renames: make([]TeamLineageRename, 0, len(renames))}
	for rows.Next() {
		var rename TeamRename
		var bothPresent, linked bool
		if err := rows.Scan(&rename.FromName, &rename.ToName, &rename.Gender, &bothPresent, &linked); err != nil {
			slog.Error("team lineage coverage scan failed", slog.Any("err", err))
			return TeamLineageReport{}, fmt.Errorf("team lineage coverage: %w", err)
		}
		report.Renames = append(report.Renames, TeamLineageRename{
			Rename: rename,
			State:  teamLineageState(bothPresent, linked),
		})
	}
	if err := rows.Err(); err != nil {
		slog.Error("team lineage coverage read failed", slog.Any("err", err))
		return TeamLineageReport{}, fmt.Errorf("team lineage coverage: %w", err)
	}
	return report, nil
}

// teamLineageState is the one place the three states are decided, so the write path and
// the read-only probe cannot drift into two definitions of "linked".
func teamLineageState(bothPresent, linked bool) TeamLineageState {
	switch {
	case !bothPresent:
		return TeamLineageAbsent
	case linked:
		return TeamLineageLinked
	default:
		return TeamLineageUnlinked
	}
}

// teamRenameColumns turns the renames into the three parallel arrays both statements
// unnest, so one round trip covers the whole mapping however long it grows.
func teamRenameColumns(renames []TeamRename) (from, to, gender []string) {
	from = make([]string, len(renames))
	to = make([]string, len(renames))
	gender = make([]string, len(renames))
	for i, rename := range renames {
		from[i], to[i], gender[i] = rename.FromName, rename.ToName, rename.Gender
	}
	return from, to, gender
}
