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

	// 3. Query MAX(date) for relevant tables using a single UNION ALL query to optimize.
	type queryPart struct {
		tName    string
		colIdent string
		tblIdent string
	}
	var unionParts []queryPart
	for tName, col := range tableDateCol {
		unionParts = append(
			unionParts,
			queryPart{
				tName:    tName,
				colIdent: pgx.Identifier{col}.Sanitize(),
				tblIdent: pgx.Identifier{tName}.Sanitize(),
			},
		)
	}

	if len(unionParts) > 0 {
		fullQuery := ""
		var params []any
		for i, p := range unionParts {
			if i > 0 {
				fullQuery += " UNION ALL "
			}
			// Use pgx.Identifier for table and column names to prevent SQL injection.
			// Values retrieved from information_schema are used to build the query parts.
			fullQuery += fmt.Sprintf(
				"SELECT $%d as tname, MAX(%s)::text as max_val FROM %s",
				len(params)+1,
				p.colIdent,
				p.tblIdent,
			)
			params = append(params, p.tName)
		}

		mRows, err := PoolAPI.Query(ctx, fullQuery, params...)
		if err != nil {
			return nil, fmt.Errorf("query max dates: %w", err)
		}
		defer mRows.Close()

		maxDates := make(map[string]string)
		for mRows.Next() {
			var tName string
			var maxVal *string
			if err := mRows.Scan(&tName, &maxVal); err != nil {
				return nil, err
			}
			if maxVal != nil {
				val := *maxVal
				if len(val) > 10 {
					val = val[:10]
				}
				maxDates[tName] = val
			}
		}

		for i := range stats {
			if val, ok := maxDates[stats[i].TableName]; ok {
				stats[i].LastRecord = &val
			}
		}
	}

	return stats, nil
}
