package venues_test

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/venues"
)

// geocodingTable is reference-data/venue-geocoding.csv as this package finds it from the
// test's own directory. The file is hand-curated (rows are added and corrected by a
// person, which is the documented flow), so the committed file itself is under test.
const geocodingTable = "../../../reference-data/venue-geocoding.csv"

// readGeocodingTable returns the curated table's rows keyed by header name.
func readGeocodingTable(t *testing.T) []map[string]string {
	t.Helper()

	file, err := os.Open(filepath.Clean(geocodingTable))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, file.Close()) })

	records, err := csv.NewReader(file).ReadAll()
	require.NoError(t, err)
	require.Greater(t, len(records), 1, "the curated table has a header and at least one row")

	header := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		require.Len(t, record, len(header))
		row := make(map[string]string, len(header))
		for i := range header {
			row[header[i]] = record[i]
		}
		rows = append(rows, row)
	}
	return rows
}

// TestTheCuratedTablesVenueKeyIsThisPackagesFold pins the cross-language half of the
// venue identity rule (IMPORT-08): `venue_key` is computed in Python by
// ml/weather/venues.venue_key, and the importer resolves the same ground through
// venues.NormalizeName here. Nothing joined the two before, so the two folds could drift
// apart silently and the coordinates would stop joining to the grounds they describe.
// Python's half of this pairing is ml-service/tests/test_reference_data.py.
func TestTheCuratedTablesVenueKeyIsThisPackagesFold(t *testing.T) {
	t.Parallel()

	rows := readGeocodingTable(t)

	for _, row := range rows {
		assert.Equal(t, row["venue_key"], venues.NormalizeName(row["venue"]),
			"venue %q: the CSV's Python-computed key and NormalizeName must agree", row["venue"])
	}
}

// TestTheCuratedTablesKeysAreUnique: the key is what a coordinate row is joined on, so two
// rows sharing one makes the join ambiguous and one of the two grounds unreachable.
func TestTheCuratedTablesKeysAreUnique(t *testing.T) {
	t.Parallel()

	rows := readGeocodingTable(t)

	keys := make(map[string]string, len(rows))
	for _, row := range rows {
		previous, seen := keys[row["venue_key"]]
		assert.False(t, seen, "venue key %q is shared by %q and %q", row["venue_key"], previous, row["venue"])
		keys[row["venue_key"]] = row["venue"]
	}
	assert.Len(t, keys, len(rows))
}
