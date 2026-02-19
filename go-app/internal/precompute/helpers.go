package precompute

import (
	"context"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// discoverFormatCodes returns the list of format codes to process. If the
// provided slice is non-empty, it is returned as-is. Otherwise, the codes are
// discovered from the database in stable order. Uses db.Query when available (e.g. tests with SetDB); else db.Pool.
func discoverFormatCodes(ctx context.Context, provided []string) ([]string, error) {
	if len(provided) > 0 {
		return provided, nil
	}
	var rows db.Rows
	var err error
	switch {
	case db.Available():
		rows, err = db.Query(ctx, `SELECT code FROM match_format ORDER BY id`)
	case db.Pool != nil:
		rows, err = db.Pool.Query(ctx, `SELECT code FROM match_format ORDER BY id`)
	default:
		return nil, fmt.Errorf("list formats: no database")
	}
	if err != nil {
		return nil, fmt.Errorf("list formats: %w", err)
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return codes, nil
}
