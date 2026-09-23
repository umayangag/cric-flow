package cricsheet

import (
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

// DetectFormat returns one of the canonical codes TEST, ODI, T20 or T20I for a Cricsheet
// `match_type`, or "" when the type is unrecognised.
//
//   - "Test" or "MDM"  -> TEST
//   - "ODI"  or "ODM"  -> ODI
//   - "T20I" or "IT20" -> T20I
//   - "T20"            -> T20I when the competition level is international, else T20
//
// The competition level is Cricsheet's `info.team_type` (formats.ParseCompetitionLevel).
// Cricsheet writes every twenty-over match as "T20", national sides included, so the level
// is the only thing in the file that separates a T20I from a franchise game. Until
// IMPORT-09 the split was a hand-maintained list of twelve team names, which took the
// 3,888 T20s between other national sides -- World Cup qualifiers, and the World Cup
// matches those sides played against the twelve -- for club cricket.
//
// MDM and ODM stay pooled with TEST and ODI here; the level is recorded beside the code
// so that pooling can be measured and, if it is wrong, undone without a re-import.
func DetectFormat(matchType, competitionLevel string) string {
	switch strings.ToUpper(strings.TrimSpace(matchType)) {
	case "TEST", "MDM":
		return formats.CodeTest
	case "ODI", "ODM":
		return formats.CodeODI
	case "T20I", "IT20":
		return formats.CodeT20I
	case "T20":
		if competitionLevel == formats.CompetitionInternational {
			return formats.CodeT20I
		}
		return formats.CodeT20
	default:
		return ""
	}
}
