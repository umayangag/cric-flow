package exportqueries

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// winFeatureCTENames lists the 8 per-player feature CTEs whose aggregation and top-3 stats are
// generated programmatically to avoid repeating the same SQL pattern 16 times.
var winFeatureCTENames = []string{
	"t1_bat_cons", "t1_bowl_cons", "t2_bat_cons", "t2_bowl_cons",
	"t1_bat_form", "t1_bowl_form", "t2_bat_form", "t2_bowl_form",
}

func buildWinAggAndTop3CTEs() string {
	parts := make([]string, 0, len(winFeatureCTENames)+len(winFeatureCTENames))
	for _, name := range winFeatureCTENames {
		parts = append(
			parts,
			fmt.Sprintf(
				"agg_%s AS (SELECT match_id, COALESCE(SUM(v), 0) AS s, COALESCE(AVG(v), 0) AS mean_v, COALESCE(STDDEV_POP(v), 0) AS std_v, COALESCE(MAX(v), 0) AS max_v, COALESCE(MIN(v), 0) AS min_v, COUNT(*) AS cnt FROM %s GROUP BY match_id)",
				name,
				name,
			),
		)
	}
	for _, name := range winFeatureCTENames {
		parts = append(
			parts,
			fmt.Sprintf(
				"top3_%s AS (SELECT match_id, COALESCE(AVG(v), 0) AS top3_mean FROM (SELECT match_id, v, ROW_NUMBER() OVER (PARTITION BY match_id ORDER BY v DESC) AS rn FROM %s) sub WHERE rn <= 3 GROUP BY match_id)",
				name,
				name,
			),
		)
	}
	return strings.Join(parts, ",\n\t")
}

func buildWinFeatureSelectColumns() string {
	cols := make([]string, 0, len(winFeatureCTENames))
	for i, name := range winFeatureCTENames {
		n := i + 1
		cols = append(cols, fmt.Sprintf(
			"COALESCE(a%d.s, 0), COALESCE(a%d.mean_v, 0), COALESCE(a%d.std_v, 0), COALESCE(a%d.max_v, 0), COALESCE(a%d.min_v, 0), COALESCE(t3a%d.top3_mean, 0), COALESCE(a%d.cnt, 0) -- %s",
			n,
			n,
			n,
			n,
			n,
			n,
			n,
			name,
		))
	}
	return strings.Join(cols, ",\n\t\t")
}

func buildWinFeatureJoins() string {
	joins := make([]string, 0, len(winFeatureCTENames)+len(winFeatureCTENames))
	for i, name := range winFeatureCTENames {
		n := i + 1
		joins = append(joins,
			fmt.Sprintf("LEFT JOIN agg_%s a%d ON a%d.match_id = m.match_id", name, n, n),
		)
	}
	for i, name := range winFeatureCTENames {
		n := i + 1
		joins = append(joins,
			fmt.Sprintf("LEFT JOIN top3_%s t3a%d ON t3a%d.match_id = m.match_id", name, n, n),
		)
	}
	return strings.Join(joins, "\n\t")
}

// WinTrainingRows returns match-level rows for win prediction with enhanced per-player feature
// distribution statistics: for each of 8 feature groups (team1/team2 x bat/bowl x consistency/form),
// the export includes sum, mean, std, max, min, top3_mean, and count.
// team1 = batting in inning 1, team2 = bowling in inning 1.
func WinTrainingRows(ctx context.Context, cutoff time.Time) ([][]string, error) {
	return winTrainingRowsImpl(ctx, cutoff, nil)
}

// WinTrainingRowsWithFormat returns win rows filtered by format code.
func WinTrainingRowsWithFormat(ctx context.Context, format string, cutoff time.Time) ([][]string, error) {
	formatIDs, err := db.GetGlobalCache().GetFormatIDsForTrainingBucket(ctx, format)
	if err != nil {
		return nil, err
	}
	return winTrainingRowsImpl(ctx, cutoff, formatIDs)
}

func winTrainingRowsImpl(ctx context.Context, cutoff time.Time, formatIDs []int64) ([][]string, error) {
	q := `WITH matches_filtered AS (
		SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0) AS venue_id,
			mi.batting_team_opposition_id AS team1_opposition_id,
			mi.bowling_team_opposition_id AS team2_opposition_id,
			COALESCE(m.toss_winner_opposition_id, 0) AS toss_winner_opposition_id,
			CASE WHEN m.outcome_winner_opposition_id IS NULL THEN 0 WHEN m.outcome_winner_opposition_id = mi.batting_team_opposition_id THEN 1 ELSE 0 END AS team1_wins,
			COALESCE(mf.code, '') AS format_code,
			COALESCE(w.temp, 0) AS temp, COALESCE(w.wind, 0) AS wind, COALESCE(w.rain, 0) AS rain,
			COALESCE(w.humidity, 0) AS humidity, COALESCE(w.cloud, 0) AS cloud, COALESCE(w.pressure, 0) AS pressure,
			CASE WHEN w.viscosity IS NULL THEN 0 WHEN lower(w.viscosity) = 'dry' THEN 0 WHEN lower(w.viscosity) = 'humid' THEN 1 WHEN lower(w.viscosity) = 'windy' THEN 2 ELSE 0 END AS viscosity,
			m.match_date
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
		LEFT JOIN match_format mf ON m.format_id = mf.id
LEFT JOIN (SELECT DISTINCT ON (match_id) match_id, temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data WHERE session = 'batting' ORDER BY match_id, id DESC) w ON w.match_id = m.match_id
		WHERE m.match_date < $1
	),
	-- Per-player feature values (one row per player per match)
	t1_bat AS (SELECT bd.match_id, bd.player_id, m.format_id, m.match_date FROM batting_data bd JOIN matches_filtered m ON m.match_id = bd.match_id WHERE bd.inning_number = 1),
	t1_bowl AS (SELECT bw.match_id, bw.player_id, m.format_id, m.match_date FROM bowling_data bw JOIN matches_filtered m ON m.match_id = bw.match_id WHERE bw.inning_number = 2),
	t2_bat AS (SELECT bd.match_id, bd.player_id, m.format_id, m.match_date FROM batting_data bd JOIN matches_filtered m ON m.match_id = bd.match_id WHERE bd.inning_number = 2),
	t2_bowl AS (SELECT bw.match_id, bw.player_id, m.format_id, m.match_date FROM bowling_data bw JOIN matches_filtered m ON m.match_id = bw.match_id WHERE bw.inning_number = 1),
	-- Per-player snapshot values via DISTINCT ON (latest as_of_date per player+format+match)
	t1_bat_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.batting_value AS v
		FROM t1_bat p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t1_bowl_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.bowling_value AS v
		FROM t1_bowl p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t2_bat_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.batting_value AS v
		FROM t2_bat p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t2_bowl_cons AS (
		SELECT DISTINCT ON (fcs.player_id, fcs.format_id, p.match_id) p.match_id, fcs.bowling_value AS v
		FROM t2_bowl p
		JOIN feature_consistency_snapshots fcs ON fcs.player_id = p.player_id AND fcs.format_id = p.format_id AND fcs.scope = 'overall' AND fcs.scope_id IS NULL AND fcs.as_of_date <= p.match_date
		ORDER BY fcs.player_id, fcs.format_id, p.match_id, fcs.as_of_date DESC
	),
	t1_bat_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.batting_value AS v
		FROM t1_bat p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t1_bowl_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.bowling_value AS v
		FROM t1_bowl p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t2_bat_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.batting_value AS v
		FROM t2_bat p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	t2_bowl_form AS (
		SELECT DISTINCT ON (ff.player_id, ff.format_id, p.match_id) p.match_id, ff.bowling_value AS v
		FROM t2_bowl p
		JOIN feature_form_snapshots ff ON ff.player_id = p.player_id AND ff.format_id = p.format_id AND ff.scope = 'overall' AND ff.scope_id IS NULL AND ff.as_of_date <= p.match_date
		ORDER BY ff.player_id, ff.format_id, p.match_id, ff.as_of_date DESC
	),
	` + buildWinAggAndTop3CTEs() + `
	SELECT m.match_id, m.format_id, m.venue_id, m.team1_opposition_id, m.team2_opposition_id, m.toss_winner_opposition_id, m.team1_wins, m.format_code,
		m.match_date,
		m.temp, m.wind, m.rain, m.humidity, m.cloud, m.pressure, m.viscosity,
		` + buildWinFeatureSelectColumns() + `
	FROM matches_filtered m
	` + buildWinFeatureJoins() + `
	ORDER BY m.match_date ASC, m.match_id`
	args := []any{cutoff}
	if formatIDs != nil {
		q = strings.Replace(
			q,
			"WHERE m.match_date < $1",
			"WHERE m.format_id = ANY($1::bigint[]) AND m.match_date < $2",
			1,
		)
		args = []any{formatIDs, cutoff}
	}
	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	headers := winEnhancedHeaders()
	out := make([][]string, 0, 256)
	out = append(out, headers)
	for rows.Next() {
		row, err := scanWinEnhancedRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// featureDistStats holds distribution statistics for one feature group (e.g., team1 batting consistency).
type featureDistStats struct {
	sum      float64
	mean     float64
	std      float64
	max      float64
	min      float64
	top3Mean float64
	count    int64
}

func (f featureDistStats) toStrings() []string {
	return []string{
		strconv.FormatFloat(f.sum, 'f', -1, 64),
		strconv.FormatFloat(f.mean, 'f', -1, 64),
		strconv.FormatFloat(f.std, 'f', -1, 64),
		strconv.FormatFloat(f.max, 'f', -1, 64),
		strconv.FormatFloat(f.min, 'f', -1, 64),
		strconv.FormatFloat(f.top3Mean, 'f', -1, 64),
		strconv.FormatInt(f.count, 10),
	}
}

// winFeatureGroupNames lists the 8 feature groups in export order.
var winFeatureGroupNames = []string{
	"team1_bat_consistency",
	"team1_bowl_consistency",
	"team2_bat_consistency",
	"team2_bowl_consistency",
	"team1_bat_form",
	"team1_bowl_form",
	"team2_bat_form",
	"team2_bowl_form",
}

// winDistStatSuffixes lists the distribution stat suffixes in export order.
var winDistStatSuffixes = []string{"_sum", "_mean", "_std", "_max", "_min", "_top3_mean", "_count"}

func winEnhancedHeaders() []string {
	base := make([]string, 0, 17+len(winDistStatSuffixes)*len(winFeatureGroupNames))
	base = append(
		base,
		"match_id",
		"format_id",
		"venue_id",
		"team1_opposition_id",
		"team2_opposition_id",
		"toss_winner_opposition_id",
		"team1_wins",
		"format_code",
		"match_date",
		"match_date_unix",
		"temp",
		"wind",
		"rain",
		"humidity",
		"cloud",
		"pressure",
		"viscosity",
	)
	for _, group := range winFeatureGroupNames {
		for _, suffix := range winDistStatSuffixes {
			base = append(base, group+suffix)
		}
	}
	return base
}

func scanWinEnhancedRow(rows interface{ Scan(dest ...any) error }) ([]string, error) {
	var matchID, formatID, venueID, team1, team2, tossWinner int64
	var team1Wins int
	var formatCode string
	var matchDate time.Time
	var temp, wind, rain, humidity, cloud, pressure, viscosity int

	var groups [8]featureDistStats

	dest := make([]any, 0, 16+7*len(groups))
	dest = append(dest,
		&matchID, &formatID, &venueID, &team1, &team2, &tossWinner, &team1Wins, &formatCode,
		&matchDate,
		&temp, &wind, &rain, &humidity, &cloud, &pressure, &viscosity,
	)
	for i := range groups {
		g := &groups[i] //nolint:gosec // fixed-size array indexed by range
		dest = append(dest,
			&g.sum, &g.mean, &g.std,
			&g.max, &g.min, &g.top3Mean, &g.count,
		)
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	for i := range groups {
		g := &groups[i] //nolint:gosec // fixed-size array indexed by range
		if math.IsNaN(g.std) {
			g.std = 0
		}
	}

	row := []string{
		strconv.FormatInt(matchID, 10),
		strconv.FormatInt(formatID, 10),
		strconv.FormatInt(venueID, 10),
		strconv.FormatInt(team1, 10),
		strconv.FormatInt(team2, 10),
		strconv.FormatInt(tossWinner, 10),
		strconv.Itoa(team1Wins),
		formatCode,
		matchDate.Format("2006-01-02"),
		strconv.FormatInt(matchDate.Unix(), 10),
		strconv.Itoa(temp), strconv.Itoa(wind), strconv.Itoa(rain), strconv.Itoa(humidity),
		strconv.Itoa(cloud), strconv.Itoa(pressure), strconv.Itoa(viscosity),
	}
	for i := range groups {
		g := &groups[i] //nolint:gosec // fixed-size array indexed by range
		row = append(row, g.toStrings()...)
	}
	return row, nil
}
