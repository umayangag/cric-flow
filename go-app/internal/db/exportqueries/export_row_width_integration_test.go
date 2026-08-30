package exportqueries_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	exq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

// The unit tests in export_headers_test.go compare header lists against each other.
// Nothing compared a header list against the query and the row builder that have to
// fill it, which is the gap that let four separate exports ship broken:
//
//	extras.go, win.go   outer SELECT reading weather columns from a CTE that no
//	                    longer projected them -- "column m.temp does not exist"
//	fielding.go         explicit rows.Scan destinations left behind after the
//	                    SELECT shrank
//	innings.go          both of the above at once, undetected for months because
//	                    innings is the one export cmd/export-dataset does not
//	                    write, so only the training-data API ever runs its query
//
// Every one of those is invisible to a unit test: the SQL is a string until a
// database parses it. So this guard runs each export for real and asserts the two
// properties a caller depends on -- the query executes, and every row it produces is
// exactly as wide as the header row that names its columns.
//
// Needs a populated database (RUN_DB_TESTS=1, `make go-test-int`). An early cutoff
// keeps each query to the first few years of matches: enough rows to make the width
// assertion meaningful, few enough that six exports in two variants stay quick.

const rowWidthGuardFormat = "ODI"

var rowWidthGuardCutoff = time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)

// trainingExport is one export reduced to the shape every export shares: a call that
// returns the header row followed by data rows.
type trainingExport func(ctx context.Context, cutoff time.Time) ([][]string, error)

// byFormat adapts a WithFormat variant to trainingExport by pinning the format, so
// both variants of every export can share one table. The two differ in their WHERE
// clause and parameter numbering, so a mistake in one does not show up in the other.
func byFormat(fn func(ctx context.Context, format string, cutoff time.Time) ([][]string, error)) trainingExport {
	return func(ctx context.Context, cutoff time.Time) ([][]string, error) {
		return fn(ctx, rowWidthGuardFormat, cutoff)
	}
}

func TestTrainingExportRowsMatchTheirHeaderWidth_Integration(t *testing.T) {
	if os.Getenv("RUN_DB_TESTS") != "1" {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	testCases := []struct {
		name   string
		export trainingExport
	}{
		{name: "batting training", export: exq.BattingTrainingRows},
		{name: "batting training by format", export: byFormat(exq.BattingTrainingRowsWithFormat)},
		{name: "bowling training", export: exq.BowlingTrainingRows},
		{name: "bowling training by format", export: byFormat(exq.BowlingTrainingRowsWithFormat)},
		{name: "fielding training", export: exq.FieldingTrainingRows},
		{name: "fielding training by format", export: byFormat(exq.FieldingTrainingRowsWithFormat)},
		{name: "extras training", export: exq.ExtrasTrainingRows},
		{name: "extras training by format", export: byFormat(exq.ExtrasTrainingRowsWithFormat)},
		{name: "win training", export: exq.WinTrainingRows},
		{name: "win training by format", export: byFormat(exq.WinTrainingRowsWithFormat)},
		{name: "innings training", export: exq.InningsTrainingRows},
		{name: "innings training by format", export: byFormat(exq.InningsTrainingRowsWithFormat)},
	}

	for i := range testCases {
		t.Run(testCases[i].name, func(t *testing.T) {
			out, err := testCases[i].export(ctx, rowWidthGuardCutoff)
			require.NoError(t, err, "the export's SQL must run against the real schema")
			require.NotEmpty(t, out, "an export always returns at least its header row")

			headers, dataRows := out[0], out[1:]
			require.NotEmpty(t, headers, "header row must name the columns")
			require.NotEmpty(
				t,
				dataRows,
				"no rows before %s: this guard needs a populated database, run `make seed-fixtures` or import first",
				rowWidthGuardCutoff.Format(time.RFC3339),
			)
			assertRowsMatchHeaderWidth(t, headers, dataRows)
		})
	}
}

// assertRowsMatchHeaderWidth fails on the first row whose field count disagrees with
// the header row, naming both counts. A narrower header is the dangerous direction:
// pandas silently absorbs the surplus leading fields as an index and shifts every
// named column left, so the training run reads the wrong columns without complaint.
func assertRowsMatchHeaderWidth(t *testing.T, headers []string, dataRows [][]string) {
	t.Helper()
	for i := range dataRows {
		require.Lenf(
			t,
			dataRows[i],
			len(headers),
			"row %d has %d fields but the header names %d columns; the SELECT, the scan "+
				"destinations and the header list have drifted apart",
			i,
			len(dataRows[i]),
			len(headers),
		)
	}
}
