package precomputefeatures

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAsOf(t *testing.T) {
	testCases := []struct {
		name    string
		in      string
		wantHas bool
		wantYMD string
		wantErr bool
	}{
		{name: "empty -> no date", in: "", wantHas: false, wantYMD: "", wantErr: false},
		{name: "whitespace -> no date", in: "  ", wantHas: false, wantYMD: "", wantErr: false},
		{name: "valid date", in: "2024-02-03", wantHas: true, wantYMD: "2024-02-03", wantErr: false},
		{name: "invalid date", in: "2024-13-40", wantHas: false, wantYMD: "", wantErr: true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got, has, err := parseAsOf(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHas, has)
			if tc.wantHas {
				require.Equal(t, tc.wantYMD, got.Format("2006-01-02"))
			} else {
 			require.True(t, got.IsZero(), "expected zero time when no date, got %v", got)
			}
		})
	}
}

func TestValidateAlpha(t *testing.T) {
	testCases := []struct {
		name string
		v    float64
		ok   bool
	}{
		{"too small", 0.0, false},
		{"negative", -0.1, false},
		{"upper bound ok", 1.0, true},
		{"typical", 0.3, true},
		{"too large", 1.1, false},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			err := validateAlpha(tc.v)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateLastN(t *testing.T) {
	testCases := []struct {
		name string
		n    int
		ok   bool
	}{
		{"negative", -1, false},
		{"zero", 0, true},
		{"positive", 5, true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			err := validateLastN(tc.n)
			require.Equal(t, tc.ok, (err == nil))
		})
	}
}
