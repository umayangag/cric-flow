package etlrepo

import (
	"context"

	appdb "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Repo is a thin adapter that satisfies appdb.EtlRepo. In this phase it
// intentionally delegates to future SQL helpers (to be added) or acts as a
// no-op on upserts when used in dry-run flows. This keeps the adapter simple
// and allows unit tests to focus on the service layer without DB.
//
// Behavior preservation: apply=false (dry-run) paths never call this adapter in
// tests; in production, wiring can evolve to real upserts without changing the
// public interface.
type Repo struct{}

func New() *Repo { return &Repo{} }

func (r *Repo) UpsertBatting(_ context.Context, _ []appdb.EtlBattingRow) error {
	// TODO: implement using appdb helpers/queries; no-op for now
	return nil
}

func (r *Repo) UpsertBowling(_ context.Context, _ []appdb.EtlBowlingRow) error {
	// TODO: implement using appdb helpers/queries; no-op for now
	return nil
}
