package biography_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// registerCSV is a trimmed people register with the columns in a different order from the
// real file, which is the point: the parser reads the header, not fixed positions.
const registerCSV = `name,key_pulse,identifier,key_cricinfo,key_cricinfo_2
SR Tendulkar,1,35320,35320,
V Kohli,2,ba607b88,253802,253803
No Cricinfo Person,3,deadbeef,,
`

// TestParseRegister_KeysByIdentifierAndCollectsCricinfoIDs is the join the whole item
// rests on: Cricsheet identifier in, ESPNcricinfo ids out.
func TestParseRegister_KeysByIdentifierAndCollectsCricinfoIDs(t *testing.T) {
	t.Parallel()

	entries, err := biography.ParseRegister(strings.NewReader(registerCSV))

	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, []string{"253802", "253803"}, entries["ba607b88"].CricinfoIDs,
		"a person ESPNcricinfo lists twice contributes both ids, in preference order")
	assert.Equal(t, "V Kohli", entries["ba607b88"].Name)
	assert.Empty(t, entries["deadbeef"].CricinfoIDs,
		"a person ESPNcricinfo does not list has no ids, which is a miss rather than an error")
}

// TestParseRegister_ToleratesAShortRow keeps a truncated third-party line from costing
// the whole run.
func TestParseRegister_ToleratesAShortRow(t *testing.T) {
	t.Parallel()

	entries, err := biography.ParseRegister(strings.NewReader(
		"identifier,name,key_cricinfo\nabc123,Short Row\n"))

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Empty(t, entries["abc123"].CricinfoIDs)
}

// TestParseRegister_RefusesAFileWithNoIdentifierColumn fails loudly rather than
// returning an empty map that would read as "nobody is in the register".
func TestParseRegister_RefusesAFileWithNoIdentifierColumn(t *testing.T) {
	t.Parallel()

	_, err := biography.ParseRegister(strings.NewReader("name,key_cricinfo\nX,1\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "identifier")
}

// TestParseRegister_RefusesAnEmptyFile treats a zero-byte register as the failed
// download it almost always is.
func TestParseRegister_RefusesAnEmptyFile(t *testing.T) {
	t.Parallel()

	_, err := biography.ParseRegister(strings.NewReader(""))

	require.Error(t, err)
}

// TestParseRegister_SkipsARowWithNoIdentifier keeps a blank key out of the map, where it
// would silently match every player whose external_id is empty.
func TestParseRegister_SkipsARowWithNoIdentifier(t *testing.T) {
	t.Parallel()

	entries, err := biography.ParseRegister(strings.NewReader(
		"identifier,name,key_cricinfo\n,Nameless,1\nabc,Real,2\n"))

	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Contains(t, entries, "abc")
}
