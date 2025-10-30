package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/cricinfo"
	"github.com/umayangag/cric-app/go-app/internal/db"
)

var (
	team   = flag.String("team", "Sri Lanka", "Team name (as shown on Cricinfo)")
	from   = flag.String("from", "2010-01-01", "From date (YYYY-MM-DD)")
	toDate = flag.String("to", "2020-01-01", "To date (YYYY-MM-DD)")
)

func main() {
	flag.Parse()
	log.Printf("scraper starting team=%s from=%s to=%s", *team, *from, *toDate)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	url, err := buildResultsURL(*team, *from, *toDate)
	if err != nil {
		log.Fatalf("failed to build results URL: %v", err)
	}
	log.Printf("fetching: %s", url)
	items, err := cricinfo.ExtractMatchList(url)
	if err != nil {
		log.Fatalf("extract match list failed: %v", err)
	}
	log.Printf("parsed %d innings rows", len(items))

	// Minimal persistence: ensure match_ids exist in DB, then fetch scorecards and upsert parsed data
	client := cricinfo.NewClient()
	max := len(items)
	if max > 5 { max = 5 } // limit initial run
	for i := 0; i < max; i++ {
		it := items[i]
		if it.MatchID == "" { continue }
		mid, convErr := parseInt64(it.MatchID)
		if convErr != nil {
			log.Printf("warn: cannot parse match id %q: %v", it.MatchID, convErr)
			continue
		}
		if err := db.EnsureMatchByID(ctx, mid); err != nil {
			log.Printf("warn: ensure match_id=%d failed: %v", mid, err)
			continue
		}

		// Fetch scorecard and parse
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

		// Upsert high-level match info where available (venue/toss/sessions)
		var venueID *int64
		if strings.TrimSpace(mi.Venue) != "" {
			if id, e := db.GetOrCreateVenue(ctx, mi.Venue); e == nil {
				venueID = &id
			} else {
				log.Printf("warn: get/create venue %q failed: %v", mi.Venue, e)
			}
		}
		upd := &db.MatchInfoUpdate{}
		if venueID != nil { upd.VenueID = venueID }
		if v := strings.TrimSpace(mi.Toss); v != "" { upd.Toss = &v }
		if v := strings.TrimSpace(mi.BattingSession); v != "" { upd.BattingSession = &v }
		if v := strings.TrimSpace(mi.BowlingSession); v != "" { upd.BowlingSession = &v }
		if err := db.UpdateMatchDetails(ctx, mid, upd); err != nil {
			log.Printf("warn: update match_details for match_id=%d failed: %v", mid, err)
		}

		// Upsert weather
		for _, w := range weather {
			ww := &db.Weather{MatchID: mid, Session: w.Session, Temp: w.Temp, Feels: w.Feels, Wind: w.Wind, Gust: w.Gust, Rain: w.Rain, Humidity: w.Humidity, Cloud: w.Cloud, Pressure: w.Pressure, Viscosity: w.Viscosity}
			if err := db.UpsertWeather(ctx, ww); err != nil {
				log.Printf("warn: upsert weather failed match_id=%d session=%s: %v", mid, w.Session, err)
			}
		}

		// Upsert batting
		for idx, br := range batting {
			pid, e := db.GetOrCreateByName(ctx, br.PlayerName)
			if e != nil { log.Printf("warn: player get/create %q: %v", br.PlayerName, e); continue }
			pos := idx + 1
			desc := br.Description
			runs := br.Runs; balls := br.Balls; mins := br.Minutes; fours := br.Fours; sixes := br.Sixes
			sr := br.StrikeRate
			bb := &db.Batting{MatchID: mid, PlayerID: pid, Description: &desc, Runs: &runs, Balls: &balls, Minutes: &mins, Fours: &fours, Sixes: &sixes, StrikeRate: &sr, BattingPosition: &pos}
			if err := db.UpsertBatting(ctx, bb); err != nil { log.Printf("warn: upsert batting %q: %v", br.PlayerName, err) }
		}

		// Upsert bowling
		for _, bw := range bowling {
			pid, e := db.GetOrCreateByName(ctx, bw.PlayerName)
			if e != nil { log.Printf("warn: player get/create %q: %v", bw.PlayerName, e); continue }
			ov := bw.Overs; bl := bw.Balls; md := bw.Maidens; rn := bw.Runs; wk := bw.Wickets; dt := bw.Dots; fr := bw.Fours; sx := bw.Sixes; ec := bw.Econ; wd := bw.Wides; nb := bw.NoBalls
			bb := &db.Bowling{MatchID: mid, PlayerID: pid, Overs: &ov, Balls: &bl, Maidens: &md, Runs: &rn, Wickets: &wk, Dots: &dt, Fours: &fr, Sixes: &sx, Econ: &ec, Wides: &wd, NoBalls: &nb}
			if err := db.UpsertBowling(ctx, bb); err != nil { log.Printf("warn: upsert bowling %q: %v", bw.PlayerName, err) }
		}

		// Upsert fielding
		for _, fr := range fielding {
			pid, e := db.GetOrCreateByName(ctx, fr.PlayerName)
			if e != nil { log.Printf("warn: player get/create %q: %v", fr.PlayerName, e); continue }
			ca := fr.Catches; ro := fr.RunOuts; dc := fr.DroppedCatches; mr := fr.MissedRunOuts
			ff := &db.Fielding{MatchID: mid, PlayerID: pid, Catches: &ca, RunOuts: &ro, DroppedCatches: &dc, MissedRunOuts: &mr}
			if err := db.UpsertFielding(ctx, ff); err != nil { log.Printf("warn: upsert fielding %q: %v", fr.PlayerName, err) }
		}
	}

	log.Println("scraper finished")
}

func buildResultsURL(teamName, from, to string) (string, error) {
	// Map team to Cricinfo team id (extend as needed). Sri Lanka=8 matches the prototype.
	teamID := 8
	if !strings.EqualFold(teamName, "Sri Lanka") {
		log.Printf("note: team %q not in map; defaulting to Sri Lanka (id=8)", teamName)
	}
	fromStr, err := toCricinfoDate(from)
	if err != nil { return "", err }
	toStr, err := toCricinfoDate(to)
	if err != nil { return "", err }
	return fmt.Sprintf("https://stats.espncricinfo.com/ci/engine/team/%d.html?class=2;spanmax1=%s;spanmin1=%s;spanval1=span;template=results;type=team;view=innings", teamID, toStr, fromStr), nil
}

func toCricinfoDate(iso string) (string, error) {
	// input: YYYY-MM-DD -> output: DD+Mon+YYYY (e.g., 2010-01-01 -> 01+Jan+2010)
	t, err := time.Parse("2006-01-02", iso)
	if err != nil { return "", err }
	return t.Format("02+Jan+2006"), nil
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscan(s, &v)
	return v, err
}
