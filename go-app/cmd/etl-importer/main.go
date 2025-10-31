package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/db"
)

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "src/createdb/data", "directory with curated CSVs from prototype")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Import in this order to satisfy FKs: player (implicit), match_details (ensure), then weather/batting/bowling/fielding
	importWeather(ctx, filepath.Join(dir, "weather_data-batting.csv"), "batting")
	importWeather(ctx, filepath.Join(dir, "weather_data-bowling.csv"), "bowling")
	importBatting(ctx, filepath.Join(dir, "batting_data.csv"))
	importBowling(ctx, filepath.Join(dir, "bowling_data.csv"))
	// fielding CSV not listed explicitly in prototype data dir; add handler if present
	importFielding(ctx, filepath.Join(dir, "fielding_data.csv"))
}

func importWeather(ctx context.Context, path string, defaultSession string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("skip weather: %s not found", path)
			return
		}
		log.Printf("open %s: %v", path, err)
		return
	}
	defer fh.Close()
	log.Printf("importing weather from %s", path)
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		log.Printf("read header %s: %v", path, err)
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		log.Printf("read rows %s: %v", path, err)
		return
	}
	var count int
	for _, rec := range rows {
		get := func(name string) string {
			if i, ok := idx[strings.ToLower(name)]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		matchID := atoi64(get("match_id"))
		if matchID == 0 {
			continue
		}
		session := get("session")
		if session == "" {
			session = defaultSession
		}
		w := &db.Weather{
			MatchID: matchID, Session: session,
			Temp: atoiPtr(
				get("temp"),
			), Feels: atoiPtr(get("feels")), Wind: atoiPtr(get("wind")), Gust: atoiPtr(get("gust")),
			Rain: atoiPtr(
				get("rain"),
			), Humidity: atoiPtr(get("humidity")), Cloud: atoiPtr(get("cloud")), Pressure: atoiPtr(get("pressure")),
		}
		if v := get("viscosity"); v != "" {
			w.Viscosity = &[]string{v}[0]
		}
		if err := db.UpsertWeather(ctx, w); err != nil {
			log.Printf("upsert weather match_id=%d session=%s: %v", matchID, session, err)
			continue
		}
		count++
	}
	log.Printf("weather imported: %d rows from %s", count, filepath.Base(path))
}

func importBatting(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("skip batting: %s not found", path)
			return
		}
		log.Printf("open %s: %v", path, err)
		return
	}
	defer fh.Close()
	log.Printf("importing batting from %s", path)
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		log.Printf("read header %s: %v", path, err)
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		log.Printf("read rows %s: %v", path, err)
		return
	}
	var count int
	for _, rec := range rows {
		get := func(name string) string {
			if i, ok := idx[strings.ToLower(name)]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		matchID := atoi64(get("match_id"))
		playerName := get("player_name")
		if matchID == 0 || playerName == "" {
			continue
		}
		playerID, err := db.GetOrCreateByName(ctx, playerName)
		if err != nil {
			log.Printf("player %q: %v", playerName, err)
			continue
		}
		desc := get("description")
		runs := atoiPtr(get("runs"))
		balls := atoiPtr(get("balls"))
		mins := atoiPtr(get("minutes"))
		fours := atoiPtr(get("fours"))
		sixes := atoiPtr(get("sixes"))
		sr := atof32Ptr(get("strike_rate"))
		pos := atoiPtr(get("batting_position"))
		b := &db.Batting{
			MatchID:         matchID,
			PlayerID:        playerID,
			Description:     strPtrOrNil(desc),
			Runs:            runs,
			Balls:           balls,
			Minutes:         mins,
			Fours:           fours,
			Sixes:           sixes,
			StrikeRate:      sr,
			BattingPosition: pos,
		}
		if err := db.UpsertBatting(ctx, b); err != nil {
			log.Printf("upsert batting mid=%d pid=%d: %v", matchID, playerID, err)
			continue
		}
		count++
	}
	log.Printf("batting imported: %d rows", count)
}

func importBowling(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("skip bowling: %s not found", path)
			return
		}
		log.Printf("open %s: %v", path, err)
		return
	}
	defer fh.Close()
	log.Printf("importing bowling from %s", path)
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		log.Printf("read header %s: %v", path, err)
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		log.Printf("read rows %s: %v", path, err)
		return
	}
	var count int
	for _, rec := range rows {
		get := func(name string) string {
			if i, ok := idx[strings.ToLower(name)]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		matchID := atoi64(get("match_id"))
		playerName := get("player_name")
		if matchID == 0 || playerName == "" {
			continue
		}
		playerID, err := db.GetOrCreateByName(ctx, playerName)
		if err != nil {
			log.Printf("player %q: %v", playerName, err)
			continue
		}
		ov := atof32Ptr(get("overs"))
		bl := atoiPtr(get("balls"))
		md := atoiPtr(get("maidens"))
		rn := atoiPtr(get("runs"))
		wk := atoiPtr(get("wickets"))
		dt := atoiPtr(get("dots"))
		fr := atoiPtr(get("fours"))
		sx := atoiPtr(get("sixes"))
		ec := atof32Ptr(get("econ"))
		wd := atoiPtr(get("wides"))
		nb := atoiPtr(get("no_balls"))
		b := &db.Bowling{
			MatchID:  matchID,
			PlayerID: playerID,
			Overs:    ov,
			Balls:    bl,
			Maidens:  md,
			Runs:     rn,
			Wickets:  wk,
			Dots:     dt,
			Fours:    fr,
			Sixes:    sx,
			Econ:     ec,
			Wides:    wd,
			NoBalls:  nb,
		}
		if err := db.UpsertBowling(ctx, b); err != nil {
			log.Printf("upsert bowling mid=%d pid=%d: %v", matchID, playerID, err)
			continue
		}
		count++
	}
	log.Printf("bowling imported: %d rows", count)
}

func importFielding(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("open %s: %v", path, err)
		return
	}
	defer fh.Close()
	log.Printf("importing fielding from %s", path)
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		log.Printf("read header %s: %v", path, err)
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		log.Printf("read rows %s: %v", path, err)
		return
	}
	var count int
	for _, rec := range rows {
		get := func(name string) string {
			if i, ok := idx[strings.ToLower(name)]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		matchID := atoi64(get("match_id"))
		playerName := get("player_name")
		if matchID == 0 || playerName == "" {
			continue
		}
		playerID, err := db.GetOrCreateByName(ctx, playerName)
		if err != nil {
			log.Printf("player %q: %v", playerName, err)
			continue
		}
		ca := atoiPtr(get("catches"))
		ro := atoiPtr(get("run_outs"))
		dc := atoiPtr(get("dropped_catches"))
		mr := atoiPtr(get("missed_run_outs"))
		f := &db.Fielding{
			MatchID:        matchID,
			PlayerID:       playerID,
			Catches:        ca,
			RunOuts:        ro,
			DroppedCatches: dc,
			MissedRunOuts:  mr,
		}
		if err := db.UpsertFielding(ctx, f); err != nil {
			log.Printf("upsert fielding mid=%d pid=%d: %v", matchID, playerID, err)
			continue
		}
		count++
	}
	log.Printf("fielding imported: %d rows", count)
}

func makeIndex(head []string) map[string]int {
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return idx
}

func atoi64(s string) int64 { v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return v }
func atoiPtr(s string) *int {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &v
}
func atof32Ptr(s string) *float32 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil {
		return nil
	}
	v := float32(f)
	return &v
}
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
