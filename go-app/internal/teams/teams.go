// Package teams holds the vocabulary of a team's *side*.
//
// A team in this system is (name, gender): 130 of the 394 names in the dataset are used by
// both a men's and a women's side, so "India" is not an identity and never was. The gender
// half of that identity is a literal that crosses two boundaries -- the frontend names a
// side to go-app with it, and ml-service matches on the value go-app wrote into
// `match.gender` when it groups context baselines (E7) -- so under H-24 it is declared once,
// here, generated into contracts/ops-console.contract.json, and asserted from every side.
package teams

import "strings"

// The gender vocabulary. These are Cricsheet's own `info.gender` values, which the importer
// stores verbatim in `opposition.gender` and `match.gender`; nothing translates them on the
// way in, so this is the whole set the database can hold.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// Genders returns the vocabulary in a fixed order, for the generated contract and for the
// error that lists what a caller could have said instead.
func Genders() []string {
	return []string{GenderMale, GenderFemale}
}

// NormalizeGender trims and lower-cases a gender as a caller spelled it. "Male" and "male"
// name the same side, and refusing the first would be a distinction the database does not
// make.
func NormalizeGender(gender string) string {
	return strings.ToLower(strings.TrimSpace(gender))
}

// IsKnownGender reports whether a gender is one this system stores. A request carrying
// anything else is refused rather than resolved to nothing: an unknown gender matching no
// side would reach the caller as "no such team", which is a different and misleading answer.
func IsKnownGender(gender string) bool {
	switch NormalizeGender(gender) {
	case GenderMale, GenderFemale:
		return true
	default:
		return false
	}
}

// SideLabel is how one side is named to a person: "India (men)", "India (women)".
//
// It lives beside the vocabulary rather than in the frontend so that the picker, the
// prediction's echo of what it scored, and the error listing the candidates of an ambiguous
// name all spell a side the same way -- H-24's discipline applied to prose.
func SideLabel(name, gender string) string {
	switch NormalizeGender(gender) {
	case GenderMale:
		return name + " (men)"
	case GenderFemale:
		return name + " (women)"
	default:
		return name
	}
}
