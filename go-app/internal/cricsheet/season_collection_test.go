package cricsheet_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
)

func TestSeason_UnmarshalJSON_VariousTypes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{"string year", `"2019"`, "2019"},
		{"string range", `"2007/08"`, "2007/08"},
		{"int year", `2012`, "2012"},
		{"float intish", `2012.0`, "2012"},
		{"float non-int", `2012.5`, "2012.5"},
		{"null", `null`, ""},
	}
	for _, tc := range cases {
		var s cricsheet.Season
		err := s.UnmarshalJSON([]byte(tc.in))
		assert.NoError(t, err, tc.name)
		assert.Equal(t, tc.out, string(s), tc.name)
	}
}

func TestCollection_UnmarshalJSON_Forms(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  []string
	}{
		{"array strings", `["A","B"]`, []string{"A", "B"}},
		{"array objects", `[{"name":"A"},{"name":"B"}]`, []string{"A", "B"}},
		{"single object", `{"name":"A"}`, []string{"A"}},
		{"single string", `"A"`, []string{"A"}},
		{"mixed array", `[{"name":"A"},"B",{"name":""},123]`, []string{"A", "B"}},
		{"null", `null`, nil},
	}
	for _, tc := range cases {
		var c cricsheet.Collection
		err := c.UnmarshalJSON([]byte(tc.in))
		assert.NoError(t, err, tc.name)
		assert.Equal(t, len(tc.out), len(c), tc.name)
		for i := range tc.out {
			assert.Equal(t, tc.out[i], c[i], "%s idx %d", tc.name, i)
		}
	}
}

func TestCollection_UnmarshalJSON_GarbageFallback(t *testing.T) {
	var c cricsheet.Collection
	err := c.UnmarshalJSON([]byte(`{"foo":"bar"}`))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(c))
}

func TestParse_DoesNotPanicOnMinimalJSON(t *testing.T) {
	data := []byte(`{"info":{"teams":["A","B"],"match_type":"T20","season":"2019"},"innings":[]}`)
	m, err := cricsheet.Parse(bytes.NewReader(data))
	assert.NoError(t, err)
	assert.NotNil(t, m)
}

func TestParse_ErrorOnBadJSON(t *testing.T) {
	_, err := cricsheet.Parse(bytes.NewReader([]byte("not-json")))
	assert.Error(t, err)
}
