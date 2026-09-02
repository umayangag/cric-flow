package teams_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

func TestIsKnownGender(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		gender string
		want   bool
	}{
		{name: "male", gender: "male", want: true},
		{name: "female", gender: "female", want: true},
		{name: "mixed case is the same side", gender: "Female", want: true},
		{name: "padding is not a distinction the database makes", gender: "  male  ", want: true},
		{name: "empty names no gender", gender: "", want: false},
		{name: "a value the database cannot hold", gender: "mixed", want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, teams.IsKnownGender(tc.gender))
		})
	}
}

func TestSideLabel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		teamName   string
		gender     string
		wantLabel  string
		wantReason string
	}{
		{
			name:       "men",
			teamName:   "India",
			gender:     teams.GenderMale,
			wantLabel:  "India (men)",
			wantReason: "the picker and the response echo spell a side the same way",
		},
		{
			name:      "women",
			teamName:  "India",
			gender:    teams.GenderFemale,
			wantLabel: "India (women)",
		},
		{
			name:       "a gender the system does not store",
			teamName:   "India",
			gender:     "",
			wantLabel:  "India",
			wantReason: "an unlabelled side is named plainly rather than labelled wrongly",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.wantLabel, teams.SideLabel(tc.teamName, tc.gender), tc.wantReason)
		})
	}
}

// The vocabulary is what the generated contract publishes and what ml-service matches on, so
// a value added here without the far side's assertion is the D-9 shape again (H-24).
func TestGenders_IsTheWholeVocabulary(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"male", "female"}, teams.Genders())
}
