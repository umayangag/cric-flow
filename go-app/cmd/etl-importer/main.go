// Command etl-importer imports encoded CSV features into the database.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var dir string
	// Resolve default input directory (curated CSVs) with precedence: flag > env > config > built-in
	defDir := os.Getenv("GO_APP_INPUT_DIR")
	if defDir == "" {
		defDir = config.DefaultEtlDir()
	}
	flag.StringVar(
		&dir,
		"dir",
		defDir,
		"directory with curated CSVs (default from env GO_APP_INPUT_DIR or config.json)",
	)
	flag.Parse()

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
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
			slog.Info("skip weather: not found", slog.String("path", path))
			return
		}
		slog.Error("open failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	slog.Info("importing weather", slog.String("path", path))
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		slog.Error("read header failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		slog.Error("read rows failed", slog.String("path", path), slog.Any("err", err))
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
			slog.Warn(
				"upsert weather failed",
				slog.Int64("match_id", matchID),
				slog.String("session", session),
				slog.Any("err", err),
			)
			continue
		}
		count++
	}
	slog.Info("weather imported", slog.Int("rows", count), slog.String("file", filepath.Base(path)))
}

func importBatting(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("skip batting: not found", slog.String("path", path))
			return
		}
		slog.Error("open failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	slog.Info("importing batting", slog.String("path", path))
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		slog.Error("read header failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		slog.Error("read rows failed", slog.String("path", path), slog.Any("err", err))
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
			slog.Warn("get/create player failed", slog.String("name", playerName), slog.Any("err", err))
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
			slog.Warn(
				"upsert batting failed",
				slog.Int64("match_id", matchID),
				slog.Int64("player_id", playerID),
				slog.Any("err", err),
			)
			continue
		}
		count++
	}
	slog.Info("batting imported", slog.Int("rows", count))
}

func importBowling(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("skip bowling: not found", slog.String("path", path))
			return
		}
		slog.Error("open failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	slog.Info("importing bowling", slog.String("path", path))
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		slog.Error("read header failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		slog.Error("read rows failed", slog.String("path", path), slog.Any("err", err))
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
			slog.Warn("get/create player failed", slog.String("name", playerName), slog.Any("err", err))
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
			slog.Warn(
				"upsert bowling failed",
				slog.Int64("match_id", matchID),
				slog.Int64("player_id", playerID),
				slog.Any("err", err),
			)
			continue
		}
		count++
	}
	slog.Info("bowling imported", slog.Int("rows", count))
}

func importFielding(ctx context.Context, path string) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		slog.Error("open failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	slog.Info("importing fielding", slog.String("path", path))
	r := csv.NewReader(bufio.NewReader(fh))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		slog.Error("read header failed", slog.String("path", path), slog.Any("err", err))
		return
	}
	idx := makeIndex(head)
	rows, err := r.ReadAll()
	if err != nil {
		slog.Error("read rows failed", slog.String("path", path), slog.Any("err", err))
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
			slog.Warn("get/create player failed", slog.String("name", playerName), slog.Any("err", err))
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
			slog.Warn(
				"upsert fielding failed",
				slog.Int64("match_id", matchID),
				slog.Int64("player_id", playerID),
				slog.Any("err", err),
			)
			continue
		}
		count++
	}
	slog.Info("fielding imported", slog.Int("rows", count))
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
