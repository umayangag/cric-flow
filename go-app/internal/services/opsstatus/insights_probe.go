package opsstatus

import (
	"context"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// NewProductionInsightsProbe returns the default production implementation of InsightsProbe.
func NewProductionInsightsProbe() InsightsProbe { return productionInsightsProbe{} }

type productionInsightsProbe struct{}

// errDBNotInitialized signals db pool not initialized.
var errDBNotInitialized = fmtError("db pool not initialized")

type fmtError string

func (e fmtError) Error() string { return string(e) }

func (productionInsightsProbe) LatestMatchDateByFormat(ctx context.Context, format string) (time.Time, error) {
	if db.Pool == nil {
		return time.Time{}, errDBNotInitialized
	}
	var ts time.Time
	if err := db.Pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(m.match_date), DATE '0001-01-01')
        FROM match m
        JOIN match_format mf ON m.format_id = mf.id
        WHERE mf.code = $1
    `, format).Scan(&ts); err != nil {
		return time.Time{}, err
	}
	if ts.IsZero() {
		return time.Time{}, nil
	}
	return ts.UTC(), nil
}

func (productionInsightsProbe) CountMatchesByFormat(ctx context.Context, format string) (int64, error) {
	if db.Pool == nil {
		return 0, errDBNotInitialized
	}
	var n int64
	if err := db.Pool.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM match m
        JOIN match_format mf ON m.format_id = mf.id
        WHERE mf.code = $1
    `, format).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (productionInsightsProbe) CountMatchesSinceByFormat(
	ctx context.Context,
	format string,
	since time.Time,
) (int64, error) {
	if db.Pool == nil {
		return 0, errDBNotInitialized
	}
	var n int64
	if err := db.Pool.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM match m
        JOIN match_format mf ON m.format_id = mf.id
        WHERE mf.code = $1 AND m.match_date >= $2::date
    `, format, since).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
