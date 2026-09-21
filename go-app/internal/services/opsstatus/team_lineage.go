package opsstatus

import (
	"context"
	"log/slog"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/teamlineage"
)

// TeamLineageProbe reports how much of the reviewed rename mapping the archive has applied.
type TeamLineageProbe interface {
	TeamLineageCoverage(ctx context.Context) (db.TeamLineageReport, error)
}

// Statuses the team lineage section reports.
const (
	teamLineageStatusOK         = "ok"
	teamLineageStatusIncomplete = "incomplete"
	teamLineageStatusUnknown    = "unknown"
)

// BuildTeamLineageSection says how many renamed clubs the archive has joined back up.
//
// It exists because the import could not answer the question. Linking runs at the end of a
// run, an aborted run used to skip it, and the function that does the linking reported only
// a row count -- zero, whether it had nothing to do or never ran (IMPORT-07). So the surface
// asks the archive instead: for each rename in the reviewed mapping, are both clubs in the
// data, and does the superseded row point at the current one?
//
// `incomplete` is the value to act on: both rows of a rename are present and the link was
// never written, so that club is two clubs, with two Elo histories, two form series and two
// head-to-head records, and nothing else in the system will say so. The fix is to re-run the
// import, which is idempotent for this. `absent` is not a fault -- the mapping describes
// cricket, not one import, and a dataset may stop before a club renamed.
func BuildTeamLineageSection(ctx context.Context, probe TeamLineageProbe) map[string]any {
	if probe == nil {
		return map[string]any{"status": teamLineageStatusUnknown}
	}
	report, err := probe.TeamLineageCoverage(ctx)
	if err != nil {
		slog.Error("ops-status: could not read team lineage coverage", slog.Any("err", err))
		return map[string]any{"status": teamLineageStatusUnknown}
	}
	unlinked := report.InState(db.TeamLineageUnlinked)
	status := teamLineageStatusOK
	if len(unlinked) > 0 {
		status = teamLineageStatusIncomplete
	}
	return map[string]any{
		"status":             status,
		"renames_configured": report.Requested(),
		"linked":             report.Count(db.TeamLineageLinked),
		"unlinked":           len(unlinked),
		"absent":             report.Count(db.TeamLineageAbsent),
		"unlinked_renames":   unlinked,
	}
}

// NewProductionTeamLineageProbe returns the probe that reads the committed mapping and the
// live archive.
func NewProductionTeamLineageProbe() TeamLineageProbe { return productionTeamLineageProbe{} }

type productionTeamLineageProbe struct{}

func (productionTeamLineageProbe) TeamLineageCoverage(ctx context.Context) (db.TeamLineageReport, error) {
	mapping, err := teamlineage.Load()
	if err != nil {
		return db.TeamLineageReport{}, err
	}
	renames := make([]db.TeamRename, 0, len(mapping.Renames))
	for _, rename := range mapping.Renames {
		renames = append(renames, db.TeamRename{FromName: rename.From, ToName: rename.To, Gender: rename.Gender})
	}
	return db.TeamLineageCoverage(ctx, renames)
}
