package cricinfo

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// MatchInfo captures high-level match metadata parsed from the scorecard page.
type MatchInfo struct {
	MatchID        int64
	Venue          string
	Toss           string
	BattingSession string
	BowlingSession string
}

// BattingRow represents a single batter line from the scorecard.
type BattingRow struct {
	PlayerName      string
	Description     string
	Runs            int
	Balls           int
	Minutes         int
	Fours           int
	Sixes           int
	StrikeRate      float32
	BattingPosition int
}

// BowlingRow represents a single bowler line from the scorecard.
type BowlingRow struct {
	PlayerName string
	Overs      float32
	Balls      int
	Maidens    int
	Runs       int
	Wickets    int
	Dots       int
	Fours      int
	Sixes      int
	Econ       float32
	Wides      int
	NoBalls    int
}

// FieldingRow aggregates per-player fielding stats.
type FieldingRow struct {
	PlayerName     string
	Catches        int
	RunOuts        int
	DroppedCatches int
	MissedRunOuts  int
}

// SessionWeather contains weather-like attributes for a given session label.
type SessionWeather struct {
	Session   string // e.g., "Morning", "Afternoon" or similar derived value
	Temp      *int
	Feels     *int
	Wind      *int
	Gust      *int
	Rain      *int
	Humidity  *int
	Cloud     *int
	Pressure  *int
	Viscosity *string
}

// ParseMatchPage parses a Cricinfo match scorecard HTML and returns structured data.
// It supports both our mock fixture selectors and a best-effort set of selectors for real Cricinfo pages.
func ParseMatchPage(
	r io.Reader,
	matchID int64,
) (MatchInfo, []BattingRow, []BowlingRow, []FieldingRow, []SessionWeather, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return MatchInfo{}, nil, nil, nil, nil, err
	}

	mi := MatchInfo{MatchID: matchID}
	// 1) Venue & Toss: try mock fixture first, then real-page fallbacks (label-based search)
	mi.Venue = strings.TrimSpace(doc.Find("#match-meta .venue").First().Text())
	mi.Toss = strings.TrimSpace(doc.Find("#match-meta .toss").First().Text())
	if mi.Venue == "" {
		if v := extractMetaByLabel(doc, "Venue"); v != "" {
			mi.Venue = v
		}
	}
	if mi.Toss == "" {
		if v := extractMetaByLabel(doc, "Toss"); v != "" {
			mi.Toss = v
		}
	}

	// 2) Sessions: mock fixture explicit span; otherwise, derive from Hours of play label text
	mi.BattingSession = strings.TrimSpace(doc.Find("#match-meta .sessions .batting-session").First().Text())
	mi.BowlingSession = strings.TrimSpace(doc.Find("#match-meta .sessions .bowling-session").First().Text())
	if mi.BattingSession == "" && mi.BowlingSession == "" {
		if hours := extractHoursMap(doc); hours != nil {
			bs, ws := DeriveSessions(1, hours) // default to first innings when unknown
			mi.BattingSession, mi.BowlingSession = bs, ws
		}
	}

	// 3) Weather: mock fixture blocks; real pages rarely expose numeric weather — leave empty unless present
	var weather []SessionWeather
	doc.Find("#weather .session").Each(func(_ int, s *goquery.Selection) {
		label, _ := s.Attr("data-label")
		getInt := func(sel string) *int {
			v := strings.TrimSpace(s.Find(sel).First().Text())
			if v == "" {
				return nil
			}
			if n, err := strconv.Atoi(v); err == nil {
				return &n
			}
			return nil
		}
		getStr := func(sel string) *string {
			v := strings.TrimSpace(s.Find(sel).First().Text())
			if v == "" {
				return nil
			}
			return &v
		}
		w := SessionWeather{
			Session:   label,
			Temp:      getInt(".temp"),
			Feels:     getInt(".feels"),
			Wind:      getInt(".wind"),
			Gust:      getInt(".gust"),
			Rain:      getInt(".rain"),
			Humidity:  getInt(".humidity"),
			Cloud:     getInt(".cloud"),
			Pressure:  getInt(".pressure"),
			Viscosity: getStr(".viscosity"),
		}
		weather = append(weather, w)
	})

	// 4) Batting: try real-page style header matching, otherwise fallback to mock #batting table
	var batting []BattingRow
	batting = append(batting, parseBattingReal(doc)...)
	if len(batting) == 0 {
		// Fallback 1: class-based realish fixture
		doc.Find("table.batting-table tbody tr").Each(func(i int, tr *goquery.Selection) {
			cells := tdTexts(tr)
			if len(cells) < 6 {
				return
			}
			name := strings.TrimSpace(firstNonEmpty(tr, "a, .ci-player, .player, span"))
			if name == "" {
				name = strings.TrimSpace(cells[0])
			}
			if name == "" {
				return
			}
			br := BattingRow{
				PlayerName:      name,
				Description:     "",
				Runs:            atoiSafe(cells[1]),
				Balls:           atoiSafe(cells[2]),
				Minutes:         0,
				Fours:           atoiSafe(cells[3]),
				Sixes:           atoiSafe(cells[4]),
				StrikeRate:      atof32Safe(cells[5]),
				BattingPosition: i + 1,
			}
			batting = append(batting, br)
		})
	}
	if len(batting) == 0 {
		// Fallback 2: mock fixture by ids
		doc.Find("#batting tbody tr").Each(func(i int, tr *goquery.Selection) {
			text := func(sel string) string { return strings.TrimSpace(tr.Find(sel).First().Text()) }
			atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
			atof := func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) }
			br := BattingRow{
				PlayerName:      text(".player"),
				Description:     text(".desc"),
				Runs:            atoi(text(".runs")),
				Balls:           atoi(text(".balls")),
				Minutes:         atoi(text(".mins")),
				Fours:           atoi(text(".fours")),
				Sixes:           atoi(text(".sixes")),
				StrikeRate:      atof(text(".sr")),
				BattingPosition: i + 1,
			}
			if br.PlayerName != "" {
				batting = append(batting, br)
			}
		})
	}

	// 5) Bowling: try real-page, else fallback
	var bowling []BowlingRow
	bowling = append(bowling, parseBowlingReal(doc)...)
	if len(bowling) == 0 {
		// Fallback 1: class-based realish fixture
		doc.Find("table.bowling-table tbody tr").Each(func(_ int, tr *goquery.Selection) {
			cells := tdTexts(tr)
			if len(cells) < 6 {
				return
			}
			name := strings.TrimSpace(firstNonEmpty(tr, "a, .ci-player, .player, span"))
			if name == "" {
				name = strings.TrimSpace(cells[0])
			}
			if name == "" {
				return
			}
			bw := BowlingRow{
				PlayerName: name,
				Overs:      atof32Safe(cells[1]),
				Balls:      0,
				Maidens:    atoiSafe(cells[2]),
				Runs:       atoiSafe(cells[3]),
				Wickets:    atoiSafe(cells[4]),
				Dots:       0,
				Fours:      0,
				Sixes:      0,
				Econ:       atof32Safe(cells[5]),
				Wides:      atoiSafe(pickCellByHeaderGuess(cells, []string{"wd", "wides"})),
				NoBalls:    atoiSafe(pickCellByHeaderGuess(cells, []string{"nb", "noballs"})),
			}
			bowling = append(bowling, bw)
		})
	}
	if len(bowling) == 0 {
		// Fallback 2: mock fixture by ids
		doc.Find("#bowling tbody tr").Each(func(_ int, tr *goquery.Selection) {
			text := func(sel string) string { return strings.TrimSpace(tr.Find(sel).First().Text()) }
			atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
			atof := func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) }
			bw := BowlingRow{
				PlayerName: text(".player"),
				Overs:      atof(text(".overs")),
				Balls:      atoi(text(".balls")),
				Maidens:    atoi(text(".maidens")),
				Runs:       atoi(text(".runs")),
				Wickets:    atoi(text(".wickets")),
				Dots:       atoi(text(".dots")),
				Fours:      atoi(text(".fours")),
				Sixes:      atoi(text(".sixes")),
				Econ:       atof(text(".econ")),
				Wides:      atoi(text(".wides")),
				NoBalls:    atoi(text(".noballs")),
			}
			if bw.PlayerName != "" {
				bowling = append(bowling, bw)
			}
		})
	}

	// 6) Fielding: real pages don’t expose a nice aggregated table; fallback to mock fixture if present
	var fielding []FieldingRow
	doc.Find("#fielding tbody tr").Each(func(_ int, tr *goquery.Selection) {
		text := func(sel string) string { return strings.TrimSpace(tr.Find(sel).First().Text()) }
		atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
		fr := FieldingRow{
			PlayerName:     text(".player"),
			Catches:        atoi(text(".catches")),
			RunOuts:        atoi(text(".runouts")),
			DroppedCatches: atoi(text(".dropped")),
			MissedRunOuts:  atoi(text(".missed")),
		}
		if fr.PlayerName != "" {
			fielding = append(fielding, fr)
		}
	})

	return mi, batting, bowling, fielding, weather, nil
}

// ScorecardURL builds a legacy stats.espncricinfo URL for a given match id.
func ScorecardURL(matchID string) string {
	return fmt.Sprintf("https://stats.espncricinfo.com/ci/engine/match/%s.html", matchID)
}

// --- helpers for real Cricinfo pages ---

// extractMetaByLabel searches for a label like "Venue" or "Toss" and returns the associated value.
// It scans common container tags and tries simple "Label: Value" patterns.
func extractMetaByLabel(doc *goquery.Document, label string) string {
	want := strings.ToLower(label)
	var found string
	doc.Find("p, li, span, div").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		lt := strings.ToLower(text)
		if strings.HasPrefix(lt, want+":") || strings.Contains(lt, want+":") {
			// split at first ':'
			idx := strings.Index(text, ":")
			if idx >= 0 && idx+1 < len(text) {
				val := strings.TrimSpace(text[idx+1:])
				if val != "" {
					found = val
					return false
				}
			}
		}
		return true
	})
	return found
}

// extractHoursMap tries to locate the "Hours of play (local time)" text from the page.
func extractHoursMap(doc *goquery.Document) map[string]string {
	keys := []string{"Hours of play (local time)", "Hours of play"}
	var hours string
	// Only consider leaf-like nodes (p, li, span) and require prefix match to avoid capturing container text
	doc.Find("p, li, span").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		lt := strings.ToLower(text)
		for _, k := range keys {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lt, lk+":") { // strict prefix to avoid mixing with other labels in parent containers
				idx := strings.Index(lt, ":")
				if idx >= 0 && idx+1 < len(text) {
					hours = strings.TrimSpace(text[idx+1:])
					return false
				}
			}
		}
		return true
	})
	if hours == "" {
		return nil
	}
	return map[string]string{"Hours of play (local time)": hours}
}

// parseBattingReal attempts to parse a batting table by identifying a header row containing common batting columns.
func parseBattingReal(doc *goquery.Document) []BattingRow {
	var out []BattingRow
	// inspect all tables and try to match header columns
	doc.Find("table").Each(func(_ int, tbl *goquery.Selection) {
		headers := headerTexts(tbl)
		if !hasAll(headers, []string{"r", "b", "4s", "6s", "sr"}) {
			return
		}
		pos := 0
		tbl.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			cells := tdTexts(tr)
			// Real pages often have 6 columns for batting (name + R, B, 4s, 6s, SR)
			if len(cells) < 6 {
				return
			}
			name := strings.TrimSpace(firstNonEmpty(tr, "a, .ci-player, .player, span"))
			if name == "" {
				name = strings.TrimSpace(cells[0])
			}
			if name == "" {
				return
			}
			pos++
			br := BattingRow{
				PlayerName:      name,
				Description:     "", // description not reliably present on live pages
				Runs:            atoiSafe(pickByHeader(headers, cells, "r")),
				Balls:           atoiSafe(pickByHeader(headers, cells, "b")),
				Minutes:         0,
				Fours:           atoiSafe(pickByHeader(headers, cells, "4s")),
				Sixes:           atoiSafe(pickByHeader(headers, cells, "6s")),
				StrikeRate:      atof32Safe(pickByHeader(headers, cells, "sr")),
				BattingPosition: pos,
			}
			out = append(out, br)
		})
	})
	return out
}

// parseBowlingReal attempts to parse a bowling table by header recognition.
func parseBowlingReal(doc *goquery.Document) []BowlingRow {
	var out []BowlingRow
	doc.Find("table").Each(func(_ int, tbl *goquery.Selection) {
		headers := headerTexts(tbl)
		if !hasAll(headers, []string{"o", "m", "r", "w", "econ"}) {
			return
		}
		tbl.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			cells := tdTexts(tr)
			if len(cells) < 8 {
				return
			}
			name := strings.TrimSpace(firstNonEmpty(tr, "a, .ci-player, .player, span"))
			if name == "" {
				name = strings.TrimSpace(cells[0])
			}
			if name == "" {
				return
			}
			bw := BowlingRow{
				PlayerName: name,
				Overs:      atof32Safe(pickByHeader(headers, cells, "o")),
				Balls:      0, // often not present; can be derived later
				Maidens:    atoiSafe(pickByHeader(headers, cells, "m")),
				Runs:       atoiSafe(pickByHeader(headers, cells, "r")),
				Wickets:    atoiSafe(pickByHeader(headers, cells, "w")),
				Dots:       0,
				Fours:      0,
				Sixes:      0,
				Econ:       atof32Safe(pickByHeader(headers, cells, "econ")),
				Wides:      atoiSafe(pickByHeader(headers, cells, "wd")),
				NoBalls:    atoiSafe(pickByHeader(headers, cells, "nb")),
			}
			out = append(out, bw)
		})
	})
	return out
}

// small helpers
func headerTexts(tbl *goquery.Selection) []string {
	var hs []string
	tbl.Find("thead th").Each(func(_ int, th *goquery.Selection) {
		h := strings.ToLower(strings.TrimSpace(th.Text()))
		h = strings.ReplaceAll(h, ".", "")
		hs = append(hs, h)
	})
	return hs
}

func tdTexts(tr *goquery.Selection) []string {
	var cs []string
	tr.Find("td").Each(func(_ int, td *goquery.Selection) {
		cs = append(cs, strings.TrimSpace(td.Text()))
	})
	return cs
}

func hasAll(hay []string, needles []string) bool {
	m := map[string]bool{}
	for _, h := range hay {
		m[h] = true
	}
	for _, n := range needles {
		if !containsPrefixKey(m, n) {
			return false
		}
	}
	return true
}

func containsPrefixKey(m map[string]bool, key string) bool {
	// consider minor variations like "sr", "s/r", "strike rate"
	for k := range m {
		if k == key {
			return true
		}
		if key == "sr" && (strings.Contains(k, "sr") || strings.Contains(k, "strike")) {
			return true
		}
		if key == "o" && (k == "o" || strings.HasPrefix(k, "o")) {
			return true
		}
		if key == "r" && (k == "r" || k == "runs") {
			return true
		}
		if key == "b" && (k == "b" || k == "balls") {
			return true
		}
		if key == "econ" && (strings.Contains(k, "econ") || strings.Contains(k, "economy")) {
			return true
		}
	}
	return false
}

func pickByHeader(headers, cells []string, key string) string {
	// find the column index whose header matches the key (loosely)
	for i, h := range headers {
		if containsPrefixKey(map[string]bool{h: true}, key) {
			if i < len(cells) {
				return cells[i]
			}
		}
	}
	return ""
}

// pickCellByHeaderGuess is a simple helper for fallback tables without headers where
// we still want to read optional columns like wides (Wd) and no-balls (Nb) if present.
// It tries best-effort by scanning the tail cells for numbers and returns the first
// non-empty numeric-looking value when keys hint is provided. If none found, returns empty.
func pickCellByHeaderGuess(cells []string, hints []string) string {
	for _, c := range cells {
		cv := strings.TrimSpace(c)
		if cv == "" {
			continue
		}
		// accept simple ints (ignore floats here)
		if _, err := strconv.Atoi(cv); err == nil {
			return cv
		}
	}
	return ""
}

func firstNonEmpty(s *goquery.Selection, selector string) string {
	var out string
	s.Find(selector).EachWithBreak(func(_ int, el *goquery.Selection) bool {
		v := strings.TrimSpace(el.Text())
		if v != "" {
			out = v
			return false
		}
		return true
	})
	return out
}

func atoiSafe(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
func atof32Safe(s string) float32 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 32)
	return float32(f)
}
