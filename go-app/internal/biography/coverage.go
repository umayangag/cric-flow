package biography

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CoverageRow is biography coverage for one format and gender, weighted by appearances.
//
// Appearances, not players. A biography for a man who played one match in 2004 is worth
// less than one for a man in every eleven this season, and a player-weighted figure would
// count them the same — which would let a report say "84 % covered" while the models saw a
// gap in every match they care about. Every count here is a count of fielded player-sides
// (rows in match_player), so the ratio is the share of the archive a feature could read.
type CoverageRow struct {
	Format string `json:"format"`
	Gender string `json:"gender"`

	// Appearances is every fielded player-side in this format and gender.
	Appearances int64 `json:"appearances"`
	// Attempted is the appearances whose player has a biography row at all — the pass
	// looked him up. Appearances minus this is "never asked", which is a different gap
	// from "asked and not found" and is fixed by running the backfill, not by curating.
	Attempted int64 `json:"attempted"`
	// Matched is the appearances whose player was found on Wikidata.
	Matched int64 `json:"matched"`
	// BirthDate, BattingHand, BowlingStyle, CareerEnd and Death are the appearances whose
	// player has that fact. BowlingStyle counts only styles the controlled vocabulary
	// could place; a stated style it could not place is not coverage.
	BirthDate    int64 `json:"birth_date"`
	BattingHand  int64 `json:"batting_hand"`
	BowlingStyle int64 `json:"bowling_style"`
	CareerEnd    int64 `json:"career_end"`
	Death        int64 `json:"death"`

	// Players and MatchedPlayers are the same question asked per person, reported beside
	// the weighted figure because the two differ a lot and each answers something the
	// other cannot (H-22's spirit: never a headline number without the thing that
	// qualifies it).
	Players        int64 `json:"players"`
	MatchedPlayers int64 `json:"matched_players"`
}

// UnmatchedPlayer is a player the pass found nothing for, with the appearances that say
// how much fixing him would be worth. The report lists them in that order so a curator
// works down a ranked list rather than through 13,000 names.
type UnmatchedPlayer struct {
	PlayerID    int64  `json:"player_id"`
	Name        string `json:"name"`
	CricsheetID string `json:"cricsheet_id"`
	CricinfoID  string `json:"cricinfo_id,omitempty"`
	Appearances int64  `json:"appearances"`
	// Attempted is false when there is no biography row at all: the pass has not run for
	// him, so he is not a source gap and no override will help.
	Attempted bool `json:"attempted"`
}

// Coverage is the whole measurement: the per-format rows, the totals, the worst gaps, and
// when it was taken.
type Coverage struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Rows        []CoverageRow     `json:"rows"`
	Total       CoverageRow       `json:"total"`
	Unmatched   []UnmatchedPlayer `json:"unmatched"`
	// LastFetchedAt is the newest fetched_at in the table: how fresh the acquisition is.
	// Zero when nothing has been acquired.
	LastFetchedAt *time.Time `json:"last_fetched_at,omitempty"`
	// SourceLicense names the licence the acquired rows carry, so a surface that shows
	// the figure can also say what may be done with the data behind it.
	SourceLicense string `json:"source_license"`
}

// Share returns numerator/denominator as a percentage, and zero when the denominator is.
func Share(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return 100 * float64(numerator) / float64(denominator)
}

// Total folds the rows into one, so the total is arithmetic over the same numbers the
// table shows rather than a second query that could disagree with it.
//
// The player counts are the exception and are left at zero: a player appears in several
// formats, so summing MatchedPlayers across rows would count him several times. The store
// fills those in from its own query.
func Total(rows []CoverageRow) CoverageRow {
	total := CoverageRow{Format: "ALL", Gender: "all"}
	for _, row := range rows {
		total.Appearances += row.Appearances
		total.Attempted += row.Attempted
		total.Matched += row.Matched
		total.BirthDate += row.BirthDate
		total.BattingHand += row.BattingHand
		total.BowlingStyle += row.BowlingStyle
		total.CareerEnd += row.CareerEnd
		total.Death += row.Death
	}
	return total
}

// SortRows orders the rows by format in the canonical order and then by gender, so two
// runs render the same table.
func SortRows(rows []CoverageRow) {
	order := map[string]int{"TEST": 0, "ODI": 1, "T20": 2, "T20I": 3}
	sort.SliceStable(rows, func(i, j int) bool {
		left, leftKnown := order[rows[i].Format]
		right, rightKnown := order[rows[j].Format]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if left != right {
			return left < right
		}
		if rows[i].Format != rows[j].Format {
			return rows[i].Format < rows[j].Format
		}
		return rows[i].Gender < rows[j].Gender
	})
}

// unmatchedListLength is how many gaps the committed report names. Enough to see whether
// the tail is worth curating, short enough that a person reads it.
const unmatchedListLength = 25

// RenderMarkdown writes the coverage report a run commits.
//
// The report is markdown rather than the JSON the ops surface reads because its audience
// is the X-1b gate — a person deciding whether a feature family is worth building — and
// that decision is made from a table, not from a payload.
func RenderMarkdown(coverage Coverage) string {
	var out strings.Builder
	out.WriteString("# Player biography coverage (X-1a)\n\n")
	fmt.Fprintf(&out, "Measured %s from `player_biography`, weighted by appearances\n",
		coverage.GeneratedAt.UTC().Format(time.RFC3339))
	out.WriteString("(fielded player-sides in `match_player`). Source: Wikidata, licence ")
	fmt.Fprintf(&out, "%s, joined through Cricsheet's people register and the ESPNcricinfo\n", coverage.SourceLicense)
	out.WriteString("player id (Wikidata property P2697).\n\n")
	if coverage.LastFetchedAt != nil {
		fmt.Fprintf(&out, "Acquired: %s.\n\n", coverage.LastFetchedAt.UTC().Format(time.RFC3339))
	}

	out.WriteString("## Coverage per format and gender\n\n")
	out.WriteString("| format | gender | appearances | matched | DOB | style | hand | career end | death |\n")
	out.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range coverage.Rows {
		writeCoverageRow(&out, row)
	}
	writeCoverageRow(&out, coverage.Total)
	out.WriteString("\nPlayers, unweighted, for the same rows:\n\n")
	out.WriteString("| format | gender | players | matched players |\n|---|---|---:|---:|\n")
	for _, row := range coverage.Rows {
		fmt.Fprintf(&out, "| %s | %s | %d | %d (%.1f %%) |\n",
			row.Format, row.Gender, row.Players, row.MatchedPlayers,
			Share(row.MatchedPlayers, row.Players))
	}

	out.WriteString("\n## The largest gaps\n\n")
	if len(coverage.Unmatched) == 0 {
		out.WriteString("No player with an appearance is unmatched.\n")
		return out.String()
	}
	out.WriteString("Players with no Wikidata match, by appearances — the order a curated\n")
	out.WriteString("override in `configs/player_biography_overrides.json` should work down.\n\n")
	out.WriteString("| player | cricsheet id | cricinfo id | appearances | looked up |\n")
	out.WriteString("|---|---|---|---:|---|\n")
	for i, player := range coverage.Unmatched {
		if i >= unmatchedListLength {
			break
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %d | %s |\n",
			player.Name, player.CricsheetID, orDash(player.CricinfoID),
			player.Appearances, yesNo(player.Attempted))
	}
	return out.String()
}

func writeCoverageRow(out *strings.Builder, row CoverageRow) {
	fmt.Fprintf(out, "| %s | %s | %d | %.1f %% | %.1f %% | %.1f %% | %.1f %% | %.1f %% | %.1f %% |\n",
		row.Format, row.Gender, row.Appearances,
		Share(row.Matched, row.Appearances),
		Share(row.BirthDate, row.Appearances),
		Share(row.BowlingStyle, row.Appearances),
		Share(row.BattingHand, row.Appearances),
		Share(row.CareerEnd, row.Appearances),
		Share(row.Death, row.Appearances))
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
