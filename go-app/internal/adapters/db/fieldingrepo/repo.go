package fieldingrepo

import (
	"context"

	appdb "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Repo implements appdb.FieldingRepo using the application's DB helpers.
// It provides a thin façade for the fielding backfill service; logic remains
// in the service layer to keep this adapter minimal and easy to test/mock.
type Repo struct{}

func New() *Repo { return &Repo{} }

var _ appdb.FieldingRepo = (*Repo)(nil)

// ListFieldingEvents returns fielding events filtered by match when provided.
// It maps each fielding_event row to a BackfillEvent with a single count set,
// preserving enough information for the service-level aggregation.
func (r *Repo) ListFieldingEvents(ctx context.Context, matchID *int64) ([]appdb.BackfillEvent, error) {
	if appdb.Pool == nil {
		if _, err := appdb.Connect(ctx); err != nil { // ensure connection when run from cmd
			return nil, err
		}
	}
	var (
		rows appdb.Rows
		err  error
	)
	if matchID != nil {
		rows, err = appdb.Pool.Query(ctx, `
			SELECT match_id, fielder_id, kind, is_direct_hit
			FROM fielding_event
			WHERE match_id=$1 AND fielder_id IS NOT NULL
		`, *matchID)
	} else {
		rows, err = appdb.Pool.Query(ctx, `
			SELECT match_id, fielder_id, kind, is_direct_hit
			FROM fielding_event
			WHERE fielder_id IS NOT NULL
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]appdb.BackfillEvent, 0, 2048)
	for rows.Next() {
		var (
			mid  int64
			pid  int64
			kind string
			isDH bool
		)
		if err := rows.Scan(&mid, &pid, &kind, &isDH); err != nil {
			return nil, err
		}
		be := appdb.BackfillEvent{MatchID: mid, PlayerID: pid}
		switch kind {
		case "caught":
			be.Catches = 1
		case "run_out":
			be.RunOuts = 1
			if isDH {
				be.RunoutsDirectHits = 1
			}
		case "stumped":
			be.Stumpings = 1
		default:
			// other kinds are not counted toward aggregates; skip
			continue
		}
		out = append(out, be)
	}
	return out, nil
}

// UpsertFieldingAggregates persists aggregated rows into fielding_data.
func (r *Repo) UpsertFieldingAggregates(ctx context.Context, rows []appdb.FieldingAggregateRow) error {
	for i := range rows {
		c, ro, s, dh := rows[i].Catches, rows[i].RunOuts, rows[i].Stumpings, rows[i].RunoutsDirectHits
		if err := appdb.UpsertFielding(ctx, &appdb.Fielding{
			MatchID:           rows[i].MatchID,
			PlayerID:          rows[i].PlayerID,
			Catches:           &c,
			RunOuts:           &ro,
			DroppedCatches:    nil,
			MissedRunOuts:     nil,
			Stumpings:         &s,
			RunoutsDirectHits: &dh,
		}); err != nil {
			return err
		}
	}
	return nil
}
