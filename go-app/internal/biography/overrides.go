package biography

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Override is one hand-curated biography, keyed by the Cricsheet registry identifier.
//
// The pattern is the venue-geocoding one: an acquisition pass gets most of the way, the
// coverage report names the players it missed in descending order of how much they matter,
// and a handful of curated rows closes the top of that list. It is a small file a person
// reads and edits, not a second import.
//
// A field left out is left alone: an override that names only a bowling style does not
// erase the date of birth Wikidata supplied. That is what makes the file safe to use for
// a single correction.
type Override struct {
	// CricsheetID is player.external_id — the identity the rest of the system uses. The
	// file is keyed by it rather than by name because a name covers two people often
	// enough that the identity migration exists.
	CricsheetID string `json:"cricsheet_id"`
	// Note says where the fact came from and who checked it. It is required: a curated
	// row with no provenance is an assertion, and this package refuses to store one.
	Note string `json:"note"`

	WikidataQID   string `json:"wikidata_qid,omitempty"`
	BirthDate     string `json:"birth_date,omitempty"`
	BattingHand   string `json:"batting_hand,omitempty"`
	BowlingStyle  string `json:"bowling_style,omitempty"`
	CareerEndDate string `json:"career_end_date,omitempty"`
	DeathDate     string `json:"death_date,omitempty"`
}

// OverrideFile is the on-disk shape of configs/player_biography_overrides.json.
type OverrideFile struct {
	Comment string     `json:"$comment,omitempty"`
	Players []Override `json:"players"`
}

// ParseOverrides reads the curated file and validates every row, keyed by Cricsheet id.
//
// It validates rather than tolerates. A typo in a controlled-vocabulary value or a date
// would otherwise reach the database as a fact nothing rejects and the coverage report
// would count it, which is the failure mode a curated file exists to avoid: hand-entered
// data that nobody checks is worse than no data, because it looks the same as measured
// data afterwards.
func ParseOverrides(r io.Reader) (map[string]Override, error) {
	var file OverrideFile
	if err := json.NewDecoder(r).Decode(&file); err != nil {
		return nil, fmt.Errorf("decoding the overrides file: %w", err)
	}

	overrides := make(map[string]Override, len(file.Players))
	for i := range file.Players {
		override := file.Players[i]
		id := strings.TrimSpace(override.CricsheetID)
		if id == "" {
			return nil, fmt.Errorf("override %d names no cricsheet_id", i)
		}
		if strings.TrimSpace(override.Note) == "" {
			return nil, fmt.Errorf("override %s has no note saying where the fact came from", id)
		}
		if _, ok := overrides[id]; ok {
			return nil, fmt.Errorf("override %s appears twice", id)
		}
		if err := validateOverride(override); err != nil {
			return nil, fmt.Errorf("override %s: %w", id, err)
		}
		overrides[id] = override
	}
	return overrides, nil
}

// validateOverride checks the vocabulary values and the dates.
func validateOverride(override Override) error {
	if hand := strings.TrimSpace(override.BattingHand); hand != "" {
		if hand != HandLeft && hand != HandRight {
			return fmt.Errorf("batting_hand %q is neither %q nor %q", hand, HandLeft, HandRight)
		}
	}
	if style := strings.TrimSpace(override.BowlingStyle); style != "" && !contains(Styles(), style) {
		return fmt.Errorf("bowling_style %q is not in the controlled vocabulary %v", style, Styles())
	}
	for name, value := range map[string]string{
		"birth_date":      override.BirthDate,
		"career_end_date": override.CareerEndDate,
		"death_date":      override.DeathDate,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return fmt.Errorf("%s %q is not a YYYY-MM-DD date", name, value)
		}
	}
	return nil
}

// Apply lays a curated row over an acquired one, returning the result.
//
// Only the fields the override names change, and the row's source becomes SourceOverride
// so the coverage report can say how much of its figure is curated. A row that was never
// matched keeps its empty QID unless the override supplies one — a curated date of birth
// is a date of birth, not a Wikidata match, and reporting it as one would inflate the
// figure the X-1b gate reads.
func Apply(record Record, override Override) Record {
	record.Source = SourceOverride
	record.License = ""
	if qid := strings.TrimSpace(override.WikidataQID); qid != "" {
		record.WikidataQID = qid
	}
	if date := parseOverrideDate(override.BirthDate); date != nil {
		record.BirthDate = date
	}
	if hand := strings.TrimSpace(override.BattingHand); hand != "" {
		record.BattingHand = hand
	}
	if style := strings.TrimSpace(override.BowlingStyle); style != "" {
		record.BowlingStyle = style
		record.BowlingStyleRaw = "override: " + override.Note
	}
	if date := parseOverrideDate(override.CareerEndDate); date != nil {
		record.CareerEndDate = date
	}
	if date := parseOverrideDate(override.DeathDate); date != nil {
		record.DeathDate = date
	}
	return record
}

// parseOverrideDate reads a validated YYYY-MM-DD date, returning nil for an absent one.
func parseOverrideDate(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := time.Parse(time.DateOnly, trimmed)
	if err != nil {
		return nil
	}
	return &parsed
}
