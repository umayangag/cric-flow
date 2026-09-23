package venues_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umayangag/cric-flow/go-app/internal/venues"
)

func TestNormalizeName_FoldsPunctuationAndCaseOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		given string
		want  string
	}{
		{
			name:  "a plain name folds to itself in lower case",
			given: "Adelaide Oval",
			want:  "adelaide oval",
		},
		{
			name:  "a full stop standing in for a space carries no identity",
			given: "M.Chinnaswamy Stadium",
			want:  "m chinnaswamy stadium",
		},
		{
			name:  "the same ground spelled with a space folds to the same key",
			given: "M Chinnaswamy Stadium",
			want:  "m chinnaswamy stadium",
		},
		{
			name:  "a full stop where the archive elsewhere wrote a comma",
			given: "Gahanga International Cricket Stadium. Rwanda",
			want:  "gahanga international cricket stadium rwanda",
		},
		{
			name:  "the comma spelling of the same ground",
			given: "Gahanga International Cricket Stadium, Rwanda",
			want:  "gahanga international cricket stadium rwanda",
		},
		{
			name:  "a hyphen between initials is punctuation like any other",
			given: "Dr. Y.S. Rajasekhara Reddy ACA-VDCA Cricket Stadium",
			want:  "dr y s rajasekhara reddy aca vdca cricket stadium",
		},
		{
			name:  "accents decompose and their marks are dropped",
			given: "Café Oval, Málaga",
			want:  "cafe oval malaga",
		},
		{
			name:  "runs of whitespace and leading or trailing space collapse",
			given: "  Spaced   Out , Ground ",
			want:  "spaced out ground",
		},
		{
			name:  "a name with no letters or digits identifies nothing",
			given: "...",
			want:  "",
		},
		{
			name:  "the empty name identifies nothing",
			given: "",
			want:  "",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := venues.NormalizeName(testCase.given)

			assert.Equal(t, testCase.want, got)
		})
	}
}

// TestNormalizeName_KeepsGroundsApartThatShareANameStem is the guard on the fold's
// conservatism. "County Ground" names nine different grounds in the Cricsheet archive, and
// a rule keyed on the first comma-part -- or on the first comma-part when the trailing
// parts are a known city, as Bristol and Derby are -- would make those nine one venue with
// one scoring baseline. Every spelling here must keep its own key.
func TestNormalizeName_KeepsGroundsApartThatShareANameStem(t *testing.T) {
	t.Parallel()

	countyGrounds := []string{
		"County Ground",
		"County Ground, Bristol",
		"County Ground, Chelmsford",
		"County Ground, Derby",
		"County Ground, Hove",
		"County Ground, New Road",
		"County Ground, New Road, Worcester",
		"County Ground, Northampton",
		"County Ground, Taunton",
	}

	keys := make(map[string]string, len(countyGrounds))
	for i := range countyGrounds {
		keys[venues.NormalizeName(countyGrounds[i])] = countyGrounds[i]
	}

	assert.Len(t, keys, len(countyGrounds), "nine grounds must fold to nine keys")
}

// TestNormalizeName_LeavesSpellingsThatDifferByMoreThanPunctuation is the other side of
// that guard: the four "Kensington Oval" rows are one ground under four names, and this
// fold deliberately does not merge them. Doing so needs coordinates rather than a string
// rule, and is DATA-02's subject. If a later change makes these collide, it has widened
// the fold past what this function promises.
func TestNormalizeName_LeavesSpellingsThatDifferByMoreThanPunctuation(t *testing.T) {
	t.Parallel()

	kensingtonOvals := []string{
		"Kensington Oval",
		"Kensington Oval, Barbados",
		"Kensington Oval, Bridgetown",
		"Kensington Oval, Bridgetown, Barbados",
	}

	keys := make(map[string]struct{}, len(kensingtonOvals))
	for i := range kensingtonOvals {
		keys[venues.NormalizeName(kensingtonOvals[i])] = struct{}{}
	}

	assert.Len(t, keys, len(kensingtonOvals))
}
