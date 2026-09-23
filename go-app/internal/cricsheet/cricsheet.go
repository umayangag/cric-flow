// Package cricsheet defines types and helpers for parsing Cricsheet v1.1 match JSON.
package cricsheet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

// Structures matching Cricsheet v1.1 JSON (subset we need)

// Match represents a single CricSheet match with summary Info and Innings.
type Match struct {
	Info    Info      `json:"info"`
	Innings []Innings `json:"innings"`
}

// Info contains general match metadata such as teams, venue, and season.
type Info struct {
	BallsPerOver int      `json:"balls_per_over"`
	Dates        []string `json:"dates"`
	MatchType    string   `json:"match_type"`
	// TeamType is Cricsheet's competition level, "international" or "club". It is what
	// tells a T20 between two national sides from a franchise game -- the archive's
	// `match_type` is "T20" for both -- and it is stored on the match as
	// competition_level, so a Test and a Sheffield Shield round stay distinguishable
	// under the one TEST code they share (IMPORT-09).
	TeamType string `json:"team_type"`
	// MatchTypeNumber is the ICC's running number for an official international of this
	// type (Test no. 2,400; ODI no. 4,700). Present exactly where the match had official
	// status, absent on every club match and on the internationals played before their
	// members' matches carried it.
	MatchTypeNumber *int     `json:"match_type_number"`
	Teams           []string `json:"teams"`
	Venue           string   `json:"venue"`
	City            string   `json:"city"`
	Season          Season   `json:"season"`
	Event           *Event   `json:"event"`
	Toss            *Toss    `json:"toss"`
	Outcome         *Outcome `json:"outcome"`
	Gender          string   `json:"gender"`
	Overs           int      `json:"overs"`

	// Players maps a team name to the players it fielded. This is the only record of
	// who was picked: the scorecard shows only whoever batted or bowled, and both of
	// those are decided by how the match went. See migration 0003_match_player.sql.
	Players map[string][]string `json:"players"`

	// Registry holds Cricsheet's own identifiers for the people named in this file.
	// It is the only thing in the source that tells two people who share a scorecard
	// name apart, and the only thing that recognises one person under two spellings.
	// See migration 0004_identity.sql.
	Registry Registry `json:"registry"`
}

// Registry is info.registry: Cricsheet's person identifiers for one match file.
//
// People maps a name *as spelled in this file* to a dataset-wide identifier. The
// mapping is per file and keyed by name, which is why it cannot separate two namesakes
// who appear in the same match -- see PersonID.
type Registry struct {
	People map[string]string `json:"people"`
}

// PersonIDsByName returns the file's identifiers keyed by trimmed name.
//
// Trimming both sides matters: four registry keys in the current dataset carry a trailing
// space ("Lalchhuanliana ") while the squad and delivery entries naming the same person
// do not, so an exact-match lookup silently drops them onto the name-keyed fallback --
// and two spellings of one name is exactly the split career this work removes. No file
// in the dataset has two identifiers whose names differ only by surrounding space, so
// trimming cannot merge two people; if one ever did, the later key wins and the drop from
// both squads that already covers in-match namesakes applies.
//
// Built once per match rather than looked up per name, because a match resolves the same
// handful of names hundreds of times.
func (r Registry) PersonIDsByName() map[string]string {
	out := make(map[string]string, len(r.People))
	for name, id := range r.People {
		name, id = strings.TrimSpace(name), strings.TrimSpace(id)
		if name == "" || id == "" {
			continue
		}
		out[name] = id
	}
	return out
}

// MatchDate returns the primary match date (the first entry in Dates).
// It defaults to "1970-01-01" if no dates are available.
func (i Info) MatchDate() string {
	if len(i.Dates) > 0 {
		return i.Dates[0]
	}
	return "1970-01-01"
}

// UnmarshalJSON allows Info to flexibly decode from Cricsheet JSON variations.
// It maps both `dates` and `match_date` to the `Dates` field.
func (i *Info) UnmarshalJSON(data []byte) error {
	type Alias Info
	aux := &struct {
		MatchDate []string `json:"match_date"`
		*Alias
	}{
		Alias: (*Alias)(i),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	// If dates is missing but match_date is present, use match_date
	if len(i.Dates) == 0 && len(aux.MatchDate) > 0 {
		i.Dates = aux.MatchDate
	}
	return nil
}

// Event contains optional tournament information.
//
// Stage and Group are the two fields that say which *part* of a competition a match
// belonged to, and the reason the pair is stored rather than derived: an event name names
// the tournament, never the round. Stage is the round as Cricsheet spells it ("Final",
// "Qualifier 1", "3rd Place Play-Off", "Group Stage"), Group the pool of a group-stage
// match. Both are absent from most files and stay empty when they are -- X-3 labels what
// the archive states and leaves the rest unlabelled.
type Event struct {
	Name        string      `json:"name"`
	MatchNumber *int        `json:"match_number"`
	Stage       string      `json:"stage"`
	Group       FlexibleTag `json:"group"`
}

// FlexibleTag is a short label Cricsheet writes as either a string or a bare number --
// group "A" in one file, group 1 in the next. It decodes both to text.
type FlexibleTag string

// UnmarshalJSON decodes a string, a number or null into text; anything else becomes "".
func (t *FlexibleTag) UnmarshalJSON(data []byte) error {
	*t = FlexibleTag(flexibleString(data))
	return nil
}

// Toss records which team won the toss and the decision.
type Toss struct {
	Winner   string `json:"winner"`
	Decision string `json:"decision"`
}

// Outcome is info.outcome: how the match was decided, in the shapes Cricsheet writes it.
//
// A match that was won outright carries Winner (and usually By, the margin). A match that
// was not carries Result instead -- "draw", "no result" or "tie" -- and *never* Winner. A
// tie that was then settled by a tie-breaker keeps Result "tie" and names the side that
// won the tie-breaker in Eliminator (a super over: 109 files in the current archive) or
// BowlOut (2 files, both from 2007). Method is the rule that adjusted or awarded the
// result: "D/L" on 1,018 files, "VJD", "Awarded", and once "Lost fewer wickets".
//
// Until IMPORT-02 only Winner and By were read, so a super-over win landed with no winner
// at all -- the same record as an abandoned match -- and the rating pass excluded it. See
// WinningTeam for the rule that reads these together.
type Outcome struct {
	Winner     string     `json:"winner"`
	By         *OutcomeBy `json:"by,omitempty"`
	Result     string     `json:"result"`
	Method     string     `json:"method"`
	Eliminator string     `json:"eliminator"`
	BowlOut    string     `json:"bowl_out"`
}

// WinningTeam returns the side the match went to: the outright winner, else the side that
// won the tie-breaker (a super over, else a bowl-out), else "" for a draw, a no-result or
// a tie that was left as one. Nil-safe, so a file with no outcome at all reads "".
//
// A tie-breaker win is a win: the fixture has a result and a side that took it, which is
// what the competition records, what a forecast of the match is scored against, and what
// a team's Elo and form should see. What the record keeps as well is that the match itself
// was tied (Result stays "tie" beside the winner), so a reader that wants to weight such a
// win differently can tell it from an outright one without going back to the file.
func (o *Outcome) WinningTeam() string {
	if o == nil {
		return ""
	}
	for _, side := range []string{o.Winner, o.Eliminator, o.BowlOut} {
		if side = strings.TrimSpace(side); side != "" {
			return side
		}
	}
	return ""
}

// OutcomeBy holds margin details (runs or wickets).
type OutcomeBy struct {
	Runs    *int `json:"runs,omitempty"`
	Wickets *int `json:"wickets,omitempty"`
}

// Innings represents a team's innings containing overs and deliveries.
//
// The four flags after Overs are what Cricsheet says about the innings *as a whole*, and
// they are decoded because the innings list is not always a list of innings of the match.
// SuperOver marks a tie-breaker: a one-over shoot-out Cricsheet appends after the second
// innings (226 such entries in the current archive, always at index two or later, each of
// exactly one over), whose deliveries belong to no innings anyone bats a career in. See
// PlayedInnings. Declared and Forfeited say why a first-class innings ended short of ten
// wickets, and Target is what the chasing side was set -- Cricsheet's own figure, which is
// the revised one in a rain-shortened chase. They are read so the record can tell a short
// innings from a truncated file; nothing derived from them is stored yet.
type Innings struct {
	Team      string  `json:"team"`
	Overs     []Over  `json:"overs"`
	SuperOver bool    `json:"super_over"`
	Declared  bool    `json:"declared"`
	Forfeited bool    `json:"forfeited"`
	Target    *Target `json:"target"`
}

// Target is the chase target Cricsheet writes on a second innings.
//
// Overs is a float64, not an int: 158 innings in the current archive carry a
// rain-revised target such as 12.4 overs -- overs and balls, in the scorer's notation --
// and decoding that as an integer would fail the parse of every one of those files.
type Target struct {
	Overs float64 `json:"overs"`
	Runs  int     `json:"runs"`
}

// PlayedInnings returns the innings of the match in the order they were played, leaving
// out super overs.
//
// A super over is not an innings of the match: it decides a tie, its runs do not count
// towards either side's total, and its deliveries are not part of anyone's career. Stored
// as innings 3 and 4 they were exactly that -- career balls, runs and dismissals, a
// batting position for a batter who did not bat in the match, and an "innings 3" total
// (IMPORT-01). Every consumer of a match's innings reads this and not Innings directly,
// so the two write paths -- the scorecard aggregates and the ball-by-ball rows -- cannot
// disagree about what an innings is, and the innings numbers they write are the same.
//
// The fact that a match was decided by a super over is not lost: it is a property of the
// outcome, which is where Cricsheet records it (info.outcome), and not of the innings.
func (m *Match) PlayedInnings() []Innings {
	played := make([]Innings, 0, len(m.Innings))
	for i := range m.Innings {
		if m.Innings[i].SuperOver {
			continue
		}
		played = append(played, m.Innings[i])
	}
	return played
}

// Over groups deliveries and indicates the over number.
type Over struct {
	Over       int        `json:"over"`
	Deliveries []Delivery `json:"deliveries"`
}

// Delivery represents a single ball with runs, extras and optional wicket info.
type Delivery struct {
	Batter       string          `json:"batter"`
	Bowler       string          `json:"bowler"`
	NonStriker   string          `json:"non_striker"`
	Runs         RunInfo         `json:"runs"`
	Extras       ExtrasBreakdown `json:"extras"`
	Wickets      *Wickets        `json:"wickets,omitempty"`
	Replacements *Replacements   `json:"replacements,omitempty"`
}

// Replacements is a delivery's `replacements` object: who came in at this ball, and why.
//
// Cricsheet keeps this on the delivery rather than in info because that is where it
// happened; nothing in info says which of a twelve-man list joined after the start. Two
// lists, and only Match is a change to the side: a `role` entry is a substitute finishing
// an injured bowler's over or running for a batter, which changes nobody's membership
// and is not read. See Match.ReplacementPlayers.
type Replacements struct {
	Match []MatchReplacement `json:"match"`
}

// MatchReplacement is one player joining a side after the match started, in the place of
// another. Every one of the 1,364 entries in the current archive carries all four fields;
// Reason is Cricsheet's word for it -- impact_player, concussion_substitute, supersub,
// injury_substitute, covid_replacement, national_callup, national_release, unknown -- and
// is read for the log line only: whatever the reason, the player who came in did not start.
type MatchReplacement struct {
	In     string `json:"in"`
	Out    string `json:"out"`
	Team   string `json:"team"`
	Reason string `json:"reason"`
}

// ExtrasBreakdown is a delivery's extras by kind, as Cricsheet records them under
// `extras`. The archive uses exactly these five keys and no others; a delivery with no
// extras has no `extras` object at all, which decodes to the zero value. A delivery can
// carry more than one kind -- a no-ball with leg-byes off it, a penalty beside a wide --
// and each kind is kept, because which of them the bowler is charged with is a question
// the summary in ball_event.extras_kind cannot answer (IMPORT-04).
type ExtrasBreakdown struct {
	Wides   int `json:"wides"`
	NoBalls int `json:"noballs"`
	Byes    int `json:"byes"`
	LegByes int `json:"legbyes"`
	Penalty int `json:"penalty"`
}

// RunsConcededByBowler is the part of a delivery's total that is charged to the bowler:
// the batter's runs plus wides and no-balls. Byes and leg-byes are the fielding side's
// fault, not the bowler's, and penalty runs are awarded against the side for conduct; the
// scorecard credits none of them to him, so neither does the bowling record.
func (d Delivery) RunsConcededByBowler() int {
	return d.Runs.Total - d.Extras.Byes - d.Extras.LegByes - d.Extras.Penalty
}

// IsLegal reports whether the delivery is one of the over's balls: neither a wide nor a
// no-ball, each of which the bowler must bowl again. It is the bowler's count -- his balls
// and overs, the innings' balls bowled, ball_seq and ball_event.is_legal -- and not the
// batter's, who faces a no-ball (FacedByBatter).
func (d Delivery) IsLegal() bool {
	return d.Extras.Wides == 0 && d.Extras.NoBalls == 0
}

// FacedByBatter reports whether the striker faced the delivery: every ball but a wide,
// which passes out of his reach and is not one he could have played. A no-ball is faced --
// he may hit it, and is out to a run-out off it -- so it is in his balls and his strike
// rate while it is not one of the bowler's six (IsLegal). Until IMPORT-05 was fixed the
// batter was counted by the bowler's rule and every no-ball he faced was missing from
// batting_data.balls, while the rating source counted wides as faced, the opposite error;
// this is the one rule, and ml.xi.sources.faced_by_batter is the same rule for the rating
// pass, so the scorecard and the balls_faced target agree delivery for delivery.
func (d Delivery) FacedByBatter() bool {
	return d.Extras.Wides == 0
}

type (
	// Wickets is a list of wicket events for a delivery.
	Wickets []Wicket
	// RunInfo holds per-delivery run breakdown (batter, extras, total).
	RunInfo struct {
		Batter int `json:"batter"`
		Extras int `json:"extras"`
		Total  int `json:"total"`
	}
)

// Wicket represents a dismissal event with player out, kind and optional fielders.
type Wicket struct {
	PlayerOut string      `json:"player_out"`
	Kind      string      `json:"kind"`
	Fielders  *Collection `json:"fielders,omitempty"`
}

// Collection is a flexible list of names (e.g., fielders) parsed from Cricsheet.
type Collection []string

// Season is a normalized season identifier (e.g., "2012" or "2007/08").
type Season string

// UnmarshalJSON allows Season to decode from string, number, or null.
// - "2007/08" -> "2007/08"
// - 2012 -> "2012"
// - 2012.0 -> "2012"
// - null -> ""
func (s *Season) UnmarshalJSON(data []byte) error {
	*s = Season(flexibleString(data))
	return nil
}

// flexibleString decodes one JSON scalar Cricsheet spells inconsistently into text:
// a string as itself, an integer or an integral float without its ".0", any other number
// in its shortest exact form, and null or anything unreadable as "".
func flexibleString(data []byte) string {
	if string(data) == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		return str
	}
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		return strconv.Itoa(i)
	}
	var f float64
	if err := json.Unmarshal(data, &f); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		if float64(int(f)) == f {
			return strconv.Itoa(int(f))
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

// UnmarshalJSON allows `Collection` to flexibly decode from various Cricsheet encodings:
// - null or missing -> nil
// - ["A", "B"] -> []string{"A", "B"}
// - [{"name":"A"}, {"name":"B"}] -> []string{"A", "B"}
// - {"name":"A"} -> []string{"A"}
// - "A" -> []string{"A"}
// Some datasets mix strings and objects within the same array; we will
// extract any available string or object.name values and ignore the rest.
func (c *Collection) UnmarshalJSON(data []byte) error {
	// Handle null explicitly
	if string(data) == "null" {
		*c = nil
		return nil
	}
	// Try simple []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*c = Collection(ss)
		return nil
	}
	// Try []object with name field
	type named struct {
		Name string `json:"name"`
	}
	var objs []named
	if err := json.Unmarshal(data, &objs); err == nil {
		out := make([]string, 0, len(objs))
		for _, o := range objs {
			if o.Name != "" {
				out = append(out, o.Name)
			}
		}
		*c = Collection(out)
		return nil
	}
	// Try single object
	var one named
	if err := json.Unmarshal(data, &one); err == nil && one.Name != "" {
		*c = Collection{one.Name}
		return nil
	}
	// Try single string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = Collection{s}
		return nil
	}
	// Try generic []interface{} and pull what we can
	var anyArr []any
	if err := json.Unmarshal(data, &anyArr); err == nil {
		out := make([]string, 0, len(anyArr))
		for _, v := range anyArr {
			switch t := v.(type) {
			case string:
				if t != "" {
					out = append(out, t)
				}
			case map[string]any:
				if n, ok := t["name"].(string); ok && n != "" {
					out = append(out, n)
				}
			}
		}
		*c = Collection(out)
		return nil
	}
	// Fallback: empty collection
	*c = Collection{}
	return nil
}

// Parse decodes a Cricsheet JSON match from reader.
func Parse(r io.Reader) (*Match, error) {
	dec := json.NewDecoder(r)
	var m Match
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// derivedMatchIDBase is the low end of the space match ids get when they have to be
// derived rather than read from the source. Cricsheet's own match ids are six and seven
// digits and have been growing by roughly 20,000 a year since 2004, so the two spaces
// cannot meet, and an id below this bound is a number you can look the match up by.
const derivedMatchIDBase = 100000000000

// SourceRef returns the identifier a Cricsheet file carries in its name: "1130677" for
// 1130677.json. That name is Cricsheet's own match id, and it is the only match identity
// the source publishes -- nothing inside the JSON names the match.
func SourceRef(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// MatchIDFromSource returns the match_id for one Cricsheet file.
//
// It is the file's own Cricsheet match id whenever that id is a number, which is 22,709
// of the 22,734 files in the current dataset. Deriving the id from the *content* instead
// -- a hash of date and team names, which is what this did until now -- is not an identity
// at all: two sides can play twice in a day, and 309 files in the dataset share a
// (date, team, team) with another. Every one of those pairs collapsed into a single match
// row whose squad and scorecard were whichever file's transaction committed last, so the
// database held 22,425 matches for 22,734 files and two imports of the same directory
// could disagree about which match a row described.
//
// The remaining 25 files are named with a prefix -- "wi_211824" -- and cannot be a bigint.
// Those fall back to a hash, which now includes the file identifier, so it distinguishes
// two matches the old key could not. The fallback is logged: it is the path that would
// quietly go back to inventing identities if the archive changed shape.
func MatchIDFromSource(sourceRef, dateISO, teamA, teamB string) int64 {
	if id, err := strconv.ParseInt(sourceRef, 10, 64); err == nil && id > 0 && id < derivedMatchIDBase {
		return id
	}
	slog.Warn("cricsheet: file name is not a Cricsheet match id, deriving one",
		slog.String("source_ref", sourceRef),
		slog.String("match_date", dateISO),
		slog.String("teams", teamA+" vs "+teamB))
	return derivedMatchID(sourceRef, dateISO, teamA, teamB)
}

// derivedMatchID hashes the fields that identify a match when the file name cannot.
func derivedMatchID(sourceRef, dateISO, teamA, teamB string) int64 {
	arr := []byte(fmt.Sprintf("%s|%s|%s|%s", sourceRef, dateISO, teamA, teamB))
	h := sha256.Sum256(arr)
	hex10 := hex.EncodeToString(h[:])[:10]
	var v uint64
	if _, err := fmt.Sscanf(hex10, "%x", &v); err != nil {
		// Fallback to zero if parsing fails; unlikely given fixed hex source
		v = 0
	}
	// bound into the derived space, which starts above every Cricsheet match id
	v = (v % 900000000000) + derivedMatchIDBase
	// guard uint64 -> int64 conversion (gosec G115)
	if v > math.MaxInt64 {
		v = uint64(math.MaxInt64)
	}
	return int64(v)
}
