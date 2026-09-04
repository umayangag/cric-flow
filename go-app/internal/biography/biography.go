// Package biography acquires player biographies — date of birth, batting handedness,
// bowling style and career end — from Wikidata, and reports how much of the archive they
// actually cover.
//
// X-1a in docs/EXTERNAL_DATA_PLAN.md. Acquisition and measurement only: nothing here is
// read by a model, and no feature is derived from it. X-1b decides, against the coverage
// this package measures, whether any of it is worth putting in front of a model.
//
// The join is Cricsheet's people register → ESPNcricinfo player id → Wikidata property
// P2697. It is the only free, licence-clean bridge between the two: Cricsheet publishes
// the register under ODbL and Wikidata's data is CC0, and no scraping or account-gated
// source is involved.
//
// The package is split so the network is at one edge and everything else is pure: the
// register parser, the SPARQL result decoder, the vocabulary mapping and the coverage
// arithmetic are all functions over values, which is what makes them testable without a
// fixture server or a database.
package biography

import (
	"strings"
	"time"
)

// SourceLicense is the licence Wikidata publishes its data under. It is recorded per row
// rather than assumed, because the answer would differ for any second source.
const SourceLicense = "CC0-1.0"

// Row sources. A hand-curated override must be distinguishable from an acquired fact:
// a coverage figure that counted the two together would report the operator's typing as
// the source's coverage.
const (
	// SourceWikidata is a row written by the Wikidata pass.
	SourceWikidata = "wikidata"
	// SourceOverride is a row written from configs/player_biography_overrides.json.
	SourceOverride = "override"
)

// Bowling styles: the controlled vocabulary X-1a fixes so X-1b has a small, stable set of
// labels to build matchup features from.
//
// Six styles plus unknown. The split is the one that matters to a batter facing the ball
// — the direction it turns or swings and the speed it arrives at — not the finer
// distinctions a scorecard draws (fast vs fast-medium vs medium-fast). Anything the
// source states too coarsely to place ("spin bowling", "right arm") maps to
// StyleUnknown with its raw label kept, because a coarse truth must not be sharpened
// into a specific claim.
const (
	StylePace             = "pace"
	StyleMedium           = "medium"
	StyleOffSpin          = "off-spin"
	StyleLegSpin          = "leg-spin"
	StyleLeftArmOrthodox  = "left-arm-orthodox"
	StyleLeftArmWristSpin = "left-arm-wrist"
	StyleUnknown          = "unknown"
)

// Batting hands. Two values and nothing else: an unknown hand is an absent value, not a
// third category, so a null in the column and a null in a feature frame mean the same
// thing.
const (
	HandLeft  = "left"
	HandRight = "right"
)

// Styles returns the controlled vocabulary in a fixed order, StyleUnknown last. Callers
// that report per style iterate this so two reports of the same data list their rows in
// the same order.
func Styles() []string {
	return []string{
		StylePace,
		StyleMedium,
		StyleOffSpin,
		StyleLegSpin,
		StyleLeftArmOrthodox,
		StyleLeftArmWristSpin,
		StyleUnknown,
	}
}

// Record is one player's acquired biography, as it is stored.
//
// Every field but PlayerID may be absent, and absence is the normal case for all but
// BirthDate. Dates are *time.Time rather than time.Time so "no career end date is known"
// stays distinguishable from "the career ended at the zero time", which is the whole
// difference between an unavailable criterion and a corroborated one.
type Record struct {
	PlayerID int64
	// CricinfoID is the register's key_cricinfo, the value the join was attempted on.
	// It is set even when the lookup found nothing, so a miss can be re-checked by hand.
	CricinfoID string
	// WikidataQID is the item the join landed on, empty when no item carries the id.
	WikidataQID string
	BirthDate   *time.Time
	// BattingHand is HandLeft, HandRight or empty.
	BattingHand string
	// BowlingStyle is one of Styles(), or empty when the source stated nothing at all.
	// StyleUnknown means the source stated something this vocabulary cannot place.
	BowlingStyle string
	// BowlingStyleRaw is the label BowlingStyle was mapped from, kept so a mapping can be
	// corrected later without re-fetching.
	BowlingStyleRaw string
	// CareerEndDate is Wikidata's end of work period (P2032), where stated. Never
	// inferred from inactivity, from a death, or from anything else.
	CareerEndDate *time.Time
	// DeathDate is P570. It is recorded as itself and never written into CareerEndDate:
	// a player who died in 2022 may have retired in 2007, and the ledger's career-end
	// criterion must not read one as the other.
	DeathDate *time.Time
	// Source is SourceWikidata or SourceOverride.
	Source string
	// License is the licence the values were acquired under.
	License string
	// FetchedAt is when the pass wrote the row.
	FetchedAt time.Time
}

// Matched reports whether the join found a Wikidata item for this player.
func (r Record) Matched() bool { return strings.TrimSpace(r.WikidataQID) != "" }

// HasStyle reports whether a bowling style was placed in the vocabulary. StyleUnknown is
// a stated style this vocabulary could not place, and it is deliberately not counted as
// coverage: X-1b cannot build a matchup feature out of "some kind of spin".
func (r Record) HasStyle() bool {
	return r.BowlingStyle != "" && r.BowlingStyle != StyleUnknown
}

// styleAliases maps the Wikidata labels seen on P2545 to the controlled vocabulary.
//
// Keys are lower-cased and hyphen-normalised by NormalizeLabel before lookup, so one
// entry covers "leg-break", "Leg Break" and "leg break". Labels that state a family
// without stating which member ("spin bowling") are absent on purpose: MapBowlingStyle
// returns StyleUnknown for them, which records that the source said something rather
// than nothing without inventing the missing half.
var styleAliases = map[string]string{
	"fast bowling":                StylePace,
	"fast bowler":                 StylePace,
	"pace bowling":                StylePace,
	"right arm fast":              StylePace,
	"left arm fast":               StylePace,
	"fast medium":                 StylePace,
	"right arm fast medium":       StylePace,
	"left arm fast medium":        StylePace,
	"medium pace":                 StyleMedium,
	"medium pace bowling":         StyleMedium,
	"seam bowling":                StyleMedium,
	"swing bowling":               StyleMedium,
	"medium fast":                 StyleMedium,
	"right arm medium":            StyleMedium,
	"left arm medium":             StyleMedium,
	"off spin":                    StyleOffSpin,
	"off break":                   StyleOffSpin,
	"off spin bowling":            StyleOffSpin,
	"right arm off break":         StyleOffSpin,
	"finger spin":                 StyleOffSpin,
	"leg spin":                    StyleLegSpin,
	"leg break":                   StyleLegSpin,
	"leg spin bowling":            StyleLegSpin,
	"googly":                      StyleLegSpin,
	"right arm leg break":         StyleLegSpin,
	"wrist spin":                  StyleLegSpin,
	"left arm orthodox spin":      StyleLeftArmOrthodox,
	"slow left arm orthodox":      StyleLeftArmOrthodox,
	"left arm orthodox":           StyleLeftArmOrthodox,
	"left arm spin":               StyleLeftArmOrthodox,
	"left arm unorthodox spin":    StyleLeftArmWristSpin,
	"slow left arm unorthodox":    StyleLeftArmWristSpin,
	"left arm wrist spin":         StyleLeftArmWristSpin,
	"left arm chinaman":           StyleLeftArmWristSpin,
	"chinaman":                    StyleLeftArmWristSpin,
	"left arm unorthodox":         StyleLeftArmWristSpin,
	"left arm wrist spin bowling": StyleLeftArmWristSpin,
}

// handAliases maps Wikidata handedness labels to HandLeft / HandRight. Ambidexterity is
// absent on purpose: a batter who is recorded as ambidextrous has no single hand for a
// left–right balance feature to read, and an arbitrary choice would be a fabricated fact.
var handAliases = map[string]string{
	"left handedness":  HandLeft,
	"left handed":      HandLeft,
	"left hand":        HandLeft,
	"right handedness": HandRight,
	"right handed":     HandRight,
	"right hand":       HandRight,
}

// NormalizeLabel lower-cases a source label and folds hyphens and runs of whitespace to
// single spaces, so one alias entry covers the several spellings Wikidata's editors use.
func NormalizeLabel(label string) string {
	folded := strings.Map(func(r rune) rune {
		switch r {
		case '-', '‐', '‑', '‒', '–', '—', '/', '_':
			return ' '
		default:
			return r
		}
	}, strings.ToLower(strings.TrimSpace(label)))
	return strings.Join(strings.Fields(folded), " ")
}

// MapBowlingStyle places a source label in the controlled vocabulary.
//
// It returns the empty string for an empty label — the source said nothing — and
// StyleUnknown for a label it cannot place. The two are different answers and both are
// stored: "no statement" and "a statement we cannot use" are different gaps, and only the
// second is a mapping this package could improve.
func MapBowlingStyle(label string) string {
	normalized := NormalizeLabel(label)
	if normalized == "" {
		return ""
	}
	if style, ok := styleAliases[normalized]; ok {
		return style
	}
	return StyleUnknown
}

// MapBattingHand places a handedness label on a hand, returning empty when the label is
// absent or is not one this package recognises.
func MapBattingHand(label string) string {
	return handAliases[NormalizeLabel(label)]
}
