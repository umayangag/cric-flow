package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type TableStat struct {
	TableName  string  `json:"table_name"`
	RowCount   int64   `json:"row_count"`
	LastRecord *string `json:"last_record,omitempty"` // YYYY-MM-DD
}

// GetTableStats returns row counts and approximate last record dates for all public tables.
func GetTableStats(ctx context.Context) ([]TableStat, error) {
	if PoolAPI == nil {
		return nil, fmt.Errorf("db pool not initialized")
	}

	// 1. Get row counts from pg_stat_user_tables
	rows, err := PoolAPI.Query(ctx, `
		SELECT relname, n_live_tup 
		FROM pg_stat_user_tables 
		WHERE schemaname = 'public' 
		ORDER BY relname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TableStat
	for rows.Next() {
		var s TableStat
		if err := rows.Scan(&s.TableName, &s.RowCount); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Identify date columns for each table
	// We check for columns that likely contain the business date of the record.
	// Priorities: as_of_date > match_date > date > started_at
	dateColsQuery := `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND column_name IN ('as_of_date', 'date', 'match_date', 'started_at')
	`
	cRows, err := PoolAPI.Query(ctx, dateColsQuery)
	if err != nil {
		return nil, err
	}
	defer cRows.Close()

	tableDateCol := make(map[string]string)
	for cRows.Next() {
		var tName, cName string
		if err := cRows.Scan(&tName, &cName); err != nil {
			return nil, err
		}

		// Priority logic
		current, ok := tableDateCol[tName]
		if !ok {
			tableDateCol[tName] = cName
		} else {
			switch cName {
			case "as_of_date":
				tableDateCol[tName] = cName
			case "match_date":
				if current != "as_of_date" {
					tableDateCol[tName] = cName
				}
			case "date":
				if current != "as_of_date" && current != "match_date" {
					tableDateCol[tName] = cName
				}
			}
		}
	}
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	// 3. Query MAX(date) for relevant tables
	for i := range stats {
		s := &stats[i]
		if col, ok := tableDateCol[s.TableName]; ok {
			// Construct query using identifier sanitization to avoid SQL injection via identifiers.
			// Table and column names originate from the database catalog, but we still sanitize.
			colIdent := pgx.Identifier{col}.Sanitize()
			tblIdent := pgx.Identifier{s.TableName}.Sanitize()
			q := fmt.Sprintf("SELECT MAX(%s)::text FROM %s", colIdent, tblIdent)

			// We use QueryRow. Since we are in a loop, this is N queries.
			// For ~20 tables this is negligible.
			var maxVal *string
			// Scan into *string to handle NULLs
			if err := PoolAPI.QueryRow(ctx, q).Scan(&maxVal); err == nil && maxVal != nil {
				// Truncate to YYYY-MM-DD if longer (e.g. timestamp)
				val := *maxVal
				if len(val) > 10 {
					val = val[:10]
				}
				s.LastRecord = &val
			}
		}
	}

	return stats, nil
}
