package biography_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// TestMapBowlingStyle_PlacesTheLabelsWikidataActuallyUses covers the vocabulary against
// the labels observed on P2545 in the real dataset, plus the spellings its editors use
// interchangeably.
func TestMapBowlingStyle_PlacesTheLabelsWikidataActuallyUses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		label string
		want  string
	}{
		{
			name:  "left-arm orthodox spin, the most common label in the dataset",
			label: "left-arm orthodox spin",
			want:  biography.StyleLeftArmOrthodox,
		},
		{
			name:  "slow left-arm orthodox, the same style spelled differently",
			label: "Slow left-arm orthodox",
			want:  biography.StyleLeftArmOrthodox,
		},
		{name: "leg break", label: "leg break", want: biography.StyleLegSpin},
		{name: "leg spin", label: "leg spin", want: biography.StyleLegSpin},
		{name: "googly, a delivery that identifies the style", label: "googly", want: biography.StyleLegSpin},
		{name: "off break", label: "off break", want: biography.StyleOffSpin},
		{name: "off spin", label: "off spin", want: biography.StyleOffSpin},
		{name: "chinaman, the left-arm wrist spinner", label: "chinaman", want: biography.StyleLeftArmWristSpin},
		{name: "left-arm unorthodox spin", label: "left-arm unorthodox spin", want: biography.StyleLeftArmWristSpin},
		{name: "fast bowling", label: "fast bowling", want: biography.StylePace},
		{name: "seam bowling", label: "seam bowling", want: biography.StyleMedium},
		{name: "hyphens and case are folded", label: "LEG-BREAK", want: biography.StyleLegSpin},
		{name: "a family without a member stays unknown", label: "spin bowling", want: biography.StyleUnknown},
		{name: "an arm without a style stays unknown", label: "right arm", want: biography.StyleUnknown},
		{name: "nothing stated is not the same as unknown", label: "   ", want: ""},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, biography.MapBowlingStyle(testCase.label))
		})
	}
}

// TestMapBattingHand_ReadsTheHandednessLabels checks the two values and refuses a third.
func TestMapBattingHand_ReadsTheHandednessLabels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		label string
		want  string
	}{
		{name: "right-handedness", label: "right-handedness", want: biography.HandRight},
		{name: "left-handedness", label: "left-handedness", want: biography.HandLeft},
		{name: "left handed, spelled as two words", label: "Left handed", want: biography.HandLeft},
		{name: "ambidexterity has no single hand", label: "ambidexterity", want: ""},
		{name: "absent", label: "", want: ""},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, biography.MapBattingHand(testCase.label))
		})
	}
}

// TestRecordHasStyle_DoesNotCountAnUnplaceableStyle is the distinction the coverage
// figure rests on: X-1b cannot build a matchup feature out of "some kind of spin", so a
// style the vocabulary could not place is not coverage.
func TestRecordHasStyle_DoesNotCountAnUnplaceableStyle(t *testing.T) {
	t.Parallel()

	assert.True(t, biography.Record{BowlingStyle: biography.StyleLegSpin}.HasStyle())
	assert.False(t, biography.Record{BowlingStyle: biography.StyleUnknown}.HasStyle())
	assert.False(t, biography.Record{}.HasStyle())
}

// TestRecordMatched_ReadsTheQID separates "asked and not found" from "found".
func TestRecordMatched_ReadsTheQID(t *testing.T) {
	t.Parallel()

	assert.True(t, biography.Record{WikidataQID: "Q9200"}.Matched())
	assert.False(t, biography.Record{CricinfoID: "35320"}.Matched())
}

// TestStyles_ListsTheVocabularyWithUnknownLast fixes the order two reports of the same
// data are rendered in.
func TestStyles_ListsTheVocabularyWithUnknownLast(t *testing.T) {
	t.Parallel()

	styles := biography.Styles()

	require.NotEmpty(t, styles)
	assert.Equal(t, biography.StyleUnknown, styles[len(styles)-1])
	assert.Len(t, styles, 7)
}

// TestNormalizeLabel_FoldsSeparatorsAndWhitespace is why one alias entry covers the
// several spellings the source carries.
func TestNormalizeLabel_FoldsSeparatorsAndWhitespace(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "left arm orthodox spin", biography.NormalizeLabel("  Left-Arm   Orthodox\tSpin "))
	assert.Equal(t, "", biography.NormalizeLabel("   "))
}
