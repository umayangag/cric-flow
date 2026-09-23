// Package venues holds the identity rule for a cricket ground: the one function that says
// when two spellings name the same place.
//
// It is its own package because three layers need the same answer and none of them owns it
// -- the importer, which creates a ground the first time a match file names it; the
// prediction path, which resolves a ground a request names and may not create one (GO-08);
// and `reference-data/venue-geocoding.csv`, whose `venue_key` column is this same fold
// computed in Python (`ml/weather/venues.py`). The archive's 896 stored spellings fold to
// the CSV's 892 keys exactly, which is what keeps the coordinates joinable to the rows.
package venues

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// nonKeyRun matches any run of characters that carries no identity: everything outside
// [a-z0-9] once the name has been folded to lower-case ASCII.
var nonKeyRun = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeName folds one venue spelling to its identity key: accents decomposed and
// dropped, case folded, and every run of punctuation or whitespace collapsed to a single
// space. "M Chinnaswamy Stadium" and "M.Chinnaswamy Stadium" share a key; so do
// "Gahanga International Cricket Stadium, Rwanda" and "... Stadium. Rwanda".
//
// It folds nothing else, and that restraint is deliberate. "County Ground, Bristol" and
// "County Ground, Derby" are two different grounds, as are the nine "County Ground"
// spellings the archive holds, so a rule that dropped the part after the comma -- or
// dropped it when the trailing part happens to be a known city -- would merge nine
// unrelated venues into one. Spellings that differ by more than punctuation, such as the
// four "Kensington Oval" variants, stay distinct here; folding those is DATA-02's subject
// and needs coordinates, not a string rule.
//
// The empty string folds to the empty string: a match file that names no ground has no
// ground, and the caller must not invent one for it.
func NormalizeName(name string) string {
	decomposed := norm.NFKD.String(name)
	var ascii strings.Builder
	ascii.Grow(len(decomposed))
	for _, r := range decomposed {
		if r < 128 {
			ascii.WriteRune(r)
		}
	}
	lowered := strings.ToLower(ascii.String())
	return strings.TrimSpace(nonKeyRun.ReplaceAllString(lowered, " "))
}
