//go:build legacy_cricinfo

package scrape

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/cricinfo"
	"github.com/umayangag/cric-app/go-app/internal/db"
)

// Run scrapes a small window of matches for the given team and date range (YYYY-MM-DD),
// parses scorecards and performs idempotent upserts to the database. It mirrors the
// logic from cmd/scraper but is reusable by the API.
func Run(ctx context.Context, teamName, fromISO, toISO string, limit int) error {
	url, err := buildResultsURL(teamName, fromISO, toISO)
	if err != nil {
		return err
	}
	items, err := cricinfo.ExtractMatchList(url)
	if err != nil {
		return err
	}
	client := cricinfo.NewClient()
	maximum := len(items)
	if limit > 0 && maximum > limit {
		maximum = limit
	}
	for i := 0; i < maximum; i++ {
		it := items[i]
		if it.MatchID == "" {
			continue
		}
		mid, convErr := parseInt64(it.MatchID)
		if convErr != nil {
			log.Printf("warn: cannot parse match id %q: %v", it.MatchID, convErr)
			continue
		}
		if err := db.EnsureMatchByID(ctx, mid); err != nil {
			log.Printf("warn: ensure match_id=%d failed: %v", mid, err)
			continue
		}
		scoreURL := cricinfo.ScorecardURL(it.MatchID)
		body, err := client.Get(scoreURL)
		if err != nil {
			log.Printf("warn: fetch scorecard %s failed: %v", scoreURL, err)
			continue
		}
		mi, batting, bowling, fielding, weather, perr := cricinfo.ParseMatchPage(body, mid)
		_ = body.Close()
		if perr != nil {
			log.Printf("warn: parse scorecard match_id=%d failed: %v", mid, perr)
			continue
		}

		// Update match details (venue/toss/sessions)
		var venueID *int64
		if strings.TrimSpace(mi.Venue) != "" {
			if id, e := db.GetOrCreateVenue(ctx, mi.Venue); e == nil {
				venueID = &id
			} else {
				log.Printf("warn: get/create venue %q failed: %v", mi.Venue, e)
			}
		}
		upd := &db.MatchInfoUpdate{}
		if venueID != nil {
			upd.VenueID = venueID
		}
		if v := strings.TrimSpace(mi.Toss); v != "" {
			upd.Toss = &v
		}
		if v := strings.TrimSpace(mi.BattingSession); v != "" {
			upd.BattingSession = &v
		}
		if v := strings.TrimSpace(mi.BowlingSession); v != "" {
			upd.BowlingSession = &v
		}
		// Opposition (from match list)
		if oppName := strings.TrimSpace(it.Opposition); oppName != "" {
			if oid, e := db.GetOrCreateOpposition(ctx, oppName); e == nil {
				upd.OppositionID = &oid
			} else {
				log.Printf("warn: get/create opposition %q failed: %v", oppName, e)
			}
		}
		// Date and Season
		if d := strings.TrimSpace(it.DateText); d != "" {
			if dateISO, ok := parseDateToISO(d); ok {
				upd.Date = &dateISO
				if season := yearFromISO(dateISO); season != "" {
					if sid, e := db.GetOrCreateSeason(ctx, season); e == nil {
						upd.SeasonID = &sid
					} else {
						log.Printf("warn: get/create season %q failed: %v", season, e)
					}
				}
			}
		}
		// Inning from list if parseable
		if inVal := parseInningNumber(it.Inning); inVal != 0 {
			u := int(inVal)
			upd.Inning = &u
		}
		if err := db.UpdateMatchDetails(ctx, mid, upd); err != nil {
			log.Printf("warn: update match_details for match_id=%d failed: %v", mid, err)
		}

		// Weather
		for _, w := range weather {
			ww := &db.Weather{
				MatchID:   mid,
				Session:   w.Session,
				Temp:      w.Temp,
				Feels:     w.Feels,
				Wind:      w.Wind,
				Gust:      w.Gust,
				Rain:      w.Rain,
				Humidity:  w.Humidity,
				Cloud:     w.Cloud,
				Pressure:  w.Pressure,
				Viscosity: w.Viscosity,
			}
			if err := db.UpsertWeather(ctx, ww); err != nil {
				log.Printf("warn: upsert weather failed match_id=%d session=%s: %v", mid, w.Session, err)
			}
		}

		// Batting
		for idx, br := range batting {
			pid, e := db.GetOrCreateByName(ctx, br.PlayerName)
			if e != nil {
				log.Printf("warn: player get/create %q: %v", br.PlayerName, e)
				continue
			}
			pos := idx + 1
			desc := br.Description
			runs := br.Runs
			balls := br.Balls
			mins := br.Minutes
			fours := br.Fours
			sixes := br.Sixes
			sr := br.StrikeRate
			bb := &db.Batting{
				MatchID:         mid,
				PlayerID:        pid,
				Description:     &desc,
				Runs:            &runs,
				Balls:           &balls,
				Minutes:         &mins,
				Fours:           &fours,
				Sixes:           &sixes,
				StrikeRate:      &sr,
				BattingPosition: &pos,
			}
			if err := db.UpsertBatting(ctx, bb); err != nil {
				log.Printf("warn: upsert batting %q: %v", br.PlayerName, err)
			}
		}

		// Bowling
		for _, bw := range bowling {
			pid, e := db.GetOrCreateByName(ctx, bw.PlayerName)
			if e != nil {
				log.Printf("warn: player get/create %q: %v", bw.PlayerName, e)
				continue
			}
			ov := bw.Overs
			bl := bw.Balls
			md := bw.Maidens
			rn := bw.Runs
			wk := bw.Wickets
			dt := bw.Dots
			fr := bw.Fours
			sx := bw.Sixes
			ec := bw.Econ
			wd := bw.Wides
			nb := bw.NoBalls
			bb := &db.Bowling{
				MatchID:  mid,
				PlayerID: pid,
				Overs:    &ov,
				Balls:    &bl,
				Maidens:  &md,
				Runs:     &rn,
				Wickets:  &wk,
				Dots:     &dt,
				Fours:    &fr,
				Sixes:    &sx,
				Econ:     &ec,
				Wides:    &wd,
				NoBalls:  &nb,
			}
			if err := db.UpsertBowling(ctx, bb); err != nil {
				log.Printf("warn: upsert bowling %q: %v", bw.PlayerName, err)
			}
		}

		// Fielding
		for _, fr := range fielding {
			pid, e := db.GetOrCreateByName(ctx, fr.PlayerName)
			if e != nil {
				log.Printf("warn: player get/create %q: %v", fr.PlayerName, e)
				continue
			}
			ca := fr.Catches
			ro := fr.RunOuts
			dc := fr.DroppedCatches
			mr := fr.MissedRunOuts
			ff := &db.Fielding{
				MatchID:        mid,
				PlayerID:       pid,
				Catches:        &ca,
				RunOuts:        &ro,
				DroppedCatches: &dc,
				MissedRunOuts:  &mr,
			}
			if err := db.UpsertFielding(ctx, ff); err != nil {
				log.Printf("warn: upsert fielding %q: %v", fr.PlayerName, err)
			}
		}
	}
	return nil
}

func buildResultsURL(teamName, from, to string) (string, error) {
	teamID := 8
	if !strings.EqualFold(teamName, "Sri Lanka") {
		log.Printf("note: team %q not in map; defaulting to Sri Lanka (id=8)", teamName)
	}
	fromStr, err := toCricinfoDate(from)
	if err != nil {
		return "", err
	}
	toStr, err := toCricinfoDate(to)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"https://stats.espncricinfo.com/ci/engine/team/%d.html?class=2;spanmax1=%s;spanmin1=%s;spanval1=span;template=results;type=team;view=innings",
		teamID,
		toStr,
		fromStr,
	), nil
}

func toCricinfoDate(iso string) (string, error) {
	// Keep in sync with cmd/scraper
	return cricinfoDate(iso)
}

func cricinfoDate(iso string) (string, error) { return toCricinfoDateImpl(iso) }

// Local helper mirroring cmd/scraper implementation
func toCricinfoDateImpl(iso string) (string, error) {
	// input: YYYY-MM-DD -> output: DD+Mon+YYYY (e.g., 2010-01-01 -> 01+Jan+2010)
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return "", err
	}
	return t.Format("02+Jan+2006"), nil
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// parseDateToISO tries common Cricinfo date formats and returns YYYY-MM-DD.
func parseDateToISO(s string) (string, bool) {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return "", false
	}
	layouts := []string{
		"Jan 2, 2006", // e.g., Jan 5, 2019
		"2 Jan 2006",  // e.g., 5 Jan 2019
		"2006-01-02",  // ISO already
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, ss); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	// try to extract Year if full parse fails
	re := regexp.MustCompile(`(\d{4})`)
	m := re.FindStringSubmatch(ss)
	if len(m) > 1 {
		y := m[1]
		// fallback to Jan 01 of that year
		return y + "-01-01", true
	}
	return "", false
}

// yearFromISO returns YYYY from YYYY-MM-DD input.
func yearFromISO(iso string) string {
	if len(iso) >= 4 {
		return iso[:4]
	}
	return ""
}

// parseInningNumber extracts the first integer from the inning string.
// Examples: "1st innings" -> 1, "Inns 2" -> 2.
func parseInningNumber(s string) int64 {
	re := regexp.MustCompile(`(\d+)`)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		v, _ := strconv.ParseInt(m[1], 10, 64)
		return v
	}
	return 0
}
