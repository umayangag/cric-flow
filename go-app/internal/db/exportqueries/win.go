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

// winFeatureSources maps each of the 8 export feature groups to the squad snapshot CTE it
// reads and the column within it.
//
// There are eight groups but only two snapshot passes. Both of a side's groups are drawn
// from the same squad, so the latest snapshot row per player is looked up once per side
// and four statistics are read off it. Eight separate DISTINCT ON passes over
// feature_raw_stats_snapshots produced identical values for four times the work.
var winFeatureSources = []struct {
	name   string // the agg_/top3_ CTE pair built for this group
	source string // the squad snapshot CTE it aggregates
	column string // the statistic within that CTE
}{
	{"t1_bat_cons", "t1_snapshot", "bat_cons"},
	{"t1_bowl_cons", "t1_snapshot", "bowl_cons"},
	{"t2_bat_cons", "t2_snapshot", "bat_cons"},
	{"t2_bowl_cons", "t2_snapshot", "bowl_cons"},
	{"t1_bat_form", "t1_snapshot", "bat_form"},
	{"t1_bowl_form", "t1_snapshot", "bowl_form"},
	{"t2_bat_form", "t2_snapshot", "bat_form"},
	{"t2_bowl_form", "t2_snapshot", "bowl_form"},
}

// winFeatureCTENames lists the 8 group names in export order.
func winFeatureCTENames() []string {
	names := make([]string, 0, len(winFeatureSources))
	for _, src := range winFeatureSources {
		names = append(names, src.name)
	}
	return names
}

func buildWinAggAndTop3CTEs() string {
	parts := make([]string, 0, 2*len(winFeatureSources))
	for _, src := range winFeatureSources {
		parts = append(
			parts,
			fmt.Sprintf(
				"agg_%s AS (SELECT match_id, COALESCE(SUM(%s), 0) AS s, COALESCE(AVG(%s), 0) AS mean_v, COALESCE(STDDEV_POP(%s), 0) AS std_v, COALESCE(MAX(%s), 0) AS max_v, COALESCE(MIN(%s), 0) AS min_v FROM %s GROUP BY match_id)",
				src.name,
				src.column,
				src.column,
				src.column,
				src.column,
				src.column,
				src.source,
			),
		)
	}
	for _, src := range winFeatureSources {
		parts = append(
			parts,
			fmt.Sprintf(
				"top3_%s AS (SELECT match_id, COALESCE(AVG(v), 0) AS top3_mean FROM (SELECT match_id, %s AS v, ROW_NUMBER() OVER (PARTITION BY match_id ORDER BY %s DESC) AS rn FROM %s) sub WHERE rn <= 3 GROUP BY match_id)",
				src.name,
				src.column,
				src.column,
				src.source,
			),
		)
	}
	// Join with comma between CTEs; do not add trailing comma (would cause "syntax error at or near SELECT").
	return strings.Join(parts, ",\n\t")
}

func buildWinFeatureSelectColumns() string {
	cols := make([]string, 0, len(winFeatureSources))
	for i, name := range winFeatureCTENames() {
		n := i + 1
		cols = append(cols, fmt.Sprintf(
			"COALESCE(a%d.s, 0), COALESCE(a%d.mean_v, 0), COALESCE(a%d.std_v, 0), COALESCE(a%d.max_v, 0), COALESCE(a%d.min_v, 0), COALESCE(t3a%d.top3_mean, 0) /* %s */",
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
	names := winFeatureCTENames()
	joins := make([]string, 0, 2*len(names))
	for i, name := range winFeatureCTENames() {
		n := i + 1
		joins = append(
			joins,
			fmt.Sprintf("LEFT JOIN agg_%s a%d ON a%d.match_id = m.match_id", name, n, n),
		)
	}
	for i, name := range winFeatureCTENames() {
		n := i + 1
		joins = append(
			joins,
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
	q := buildWinEnhancedQuery()
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

// buildWinEnhancedQuery assembles the win export SQL. It is separate from the execution so
// the query's shape -- which population each feature group is aggregated over -- can be
// asserted in a unit test rather than only in an integration run.
func buildWinEnhancedQuery() string {
	return `WITH matches_filtered AS (
		SELECT m.match_id, m.format_id, COALESCE(m.venue_id, 0) AS venue_id,
			mi.batting_team_opposition_id AS team1_opposition_id,
			mi.bowling_team_opposition_id AS team2_opposition_id,
			CASE WHEN m.outcome_winner_opposition_id IS NULL THEN 0 WHEN m.outcome_winner_opposition_id = mi.batting_team_opposition_id THEN 1 ELSE 0 END AS team1_wins,
			COALESCE(mf.code, '') AS format_code,
			m.match_date
		FROM match m
		JOIN match_inning mi ON mi.match_id = m.match_id AND mi.inning_number = 1
		LEFT JOIN match_format mf ON m.format_id = mf.id
		WHERE m.match_date < $1
		-- A match with no recorded squad is excluded, not zero-filled. Emitting it would
		-- produce a side of nobody: every aggregate 0 and a target still saying who won,
		-- which is a row the model would happily learn from.
		AND EXISTS (SELECT 1 FROM match_player mp WHERE mp.match_id = m.match_id AND mp.opposition_id = mi.batting_team_opposition_id)
		AND EXISTS (SELECT 1 FROM match_player mp WHERE mp.match_id = m.match_id AND mp.opposition_id = mi.bowling_team_opposition_id)
	),
	-- The players each side picked, from match_player.
	--
	-- This used to read batting_data and bowling_data -- the scorecard -- and the
	-- scorecard's membership is decided by the result. A side that chases with wickets
	-- in hand sends four batters to the crease; a side bowled out sends eleven. The
	-- count of team-2 batters alone then predicted the match at held-out AUC 0.89-0.94
	-- in the limited-overs formats, against 0.539 in TEST where both sides bat their
	-- innings out regardless of who wins. The model was reading the scoreboard.
	--
	-- Both feature groups for a side now draw from the same squad, which is also what
	-- the serving path does: aggregate_team_features_from_player_maps aggregates every
	-- group over all of the proposed XI, bowling groups included. Training and serving
	-- were computing different functions; now they compute the same one.
	t1_squad AS (SELECT mp.match_id, mp.player_id, m.format_id, m.match_date FROM match_player mp JOIN matches_filtered m ON m.match_id = mp.match_id AND mp.opposition_id = m.team1_opposition_id),
	t2_squad AS (SELECT mp.match_id, mp.player_id, m.format_id, m.match_date FROM match_player mp JOIN matches_filtered m ON m.match_id = mp.match_id AND mp.opposition_id = m.team2_opposition_id),
	-- Per-player snapshot values via DISTINCT ON (latest as_of_date per player+format+match)
	-- One snapshot lookup per side, four statistics read off the row it finds.
	--
	-- LATERAL ... ORDER BY as_of_date DESC LIMIT 1 rather than a join plus DISTINCT ON.
	-- Both keep the latest snapshot strictly before the match, so nothing after the match
	-- date can reach a feature, but they cost wildly different amounts: the DISTINCT ON
	-- form joins every squad row to *every* earlier snapshot for that player and discards
	-- all but one. Measured on this dataset that is 247k rows exploding to 38.5 million and
	-- ~1.7 GB of external merge sort per feature group. The LATERAL walks
	-- idx_feature_raw_stats_player_fmt_date backwards and stops at the first row.
	t1_snapshot AS (
		SELECT p.match_id, s.bat_cons, s.bowl_cons, s.bat_form, s.bowl_form
		FROM t1_squad p
		CROSS JOIN LATERAL (
			SELECT r.batting_std_w10 AS bat_cons, r.bowling_std_w10 AS bowl_cons,
				r.batting_mean_w5 AS bat_form, r.bowling_mean_w5 AS bowl_form
			FROM feature_raw_stats_snapshots r
			WHERE r.player_id = p.player_id AND r.format_id = p.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date < p.match_date
			-- id DESC breaks ties explicitly. feature_raw_stats_snapshots holds 443k
			-- duplicate (player_id, format_id, as_of_date) groups for scope='overall'
			-- because its unique constraint includes scope_id, which is NULL there, and
			-- Postgres treats NULLs as distinct. 2,361 of those groups carry *conflicting*
			-- values, so without a tiebreak "the latest snapshot" is ambiguous and the
			-- export is not reproducible -- 411 matches changed values between two runs of
			-- equivalent queries. The duplicates are a precompute defect to fix separately;
			-- this makes the export deterministic in the meantime.
			ORDER BY r.as_of_date DESC, r.id DESC
			LIMIT 1
		) s
	),
	t2_snapshot AS (
		SELECT p.match_id, s.bat_cons, s.bowl_cons, s.bat_form, s.bowl_form
		FROM t2_squad p
		CROSS JOIN LATERAL (
			SELECT r.batting_std_w10 AS bat_cons, r.bowling_std_w10 AS bowl_cons,
				r.batting_mean_w5 AS bat_form, r.bowling_mean_w5 AS bowl_form
			FROM feature_raw_stats_snapshots r
			WHERE r.player_id = p.player_id AND r.format_id = p.format_id AND r.scope = 'overall' AND r.scope_id IS NULL AND r.as_of_date < p.match_date
			-- id DESC breaks ties explicitly. feature_raw_stats_snapshots holds 443k
			-- duplicate (player_id, format_id, as_of_date) groups for scope='overall'
			-- because its unique constraint includes scope_id, which is NULL there, and
			-- Postgres treats NULLs as distinct. 2,361 of those groups carry *conflicting*
			-- values, so without a tiebreak "the latest snapshot" is ambiguous and the
			-- export is not reproducible -- 411 matches changed values between two runs of
			-- equivalent queries. The duplicates are a precompute defect to fix separately;
			-- this makes the export deterministic in the meantime.
			ORDER BY r.as_of_date DESC, r.id DESC
			LIMIT 1
		) s
	),
	` + buildWinAggAndTop3CTEs() + `
	/* main */
	SELECT m.match_id, m.venue_id, m.team1_opposition_id, m.team2_opposition_id, m.team1_wins, m.format_code,
		m.match_date,
		` + buildWinFeatureSelectColumns() + `
	FROM matches_filtered m
	` + buildWinFeatureJoins() + `
	ORDER BY m.match_date ASC, m.match_id`
}

// featureDistStats holds distribution statistics for one feature group (e.g., team1 batting consistency).
type featureDistStats struct {
	sum      float64
	mean     float64
	std      float64
	max      float64
	min      float64
	top3Mean float64
}

func (f featureDistStats) toStrings() []string {
	return []string{
		strconv.FormatFloat(f.sum, 'f', -1, 64),
		strconv.FormatFloat(f.mean, 'f', -1, 64),
		strconv.FormatFloat(f.std, 'f', -1, 64),
		strconv.FormatFloat(f.max, 'f', -1, 64),
		strconv.FormatFloat(f.min, 'f', -1, 64),
		strconv.FormatFloat(f.top3Mean, 'f', -1, 64),
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
//
// _count is deliberately absent. Now that a group is aggregated over the squad rather
// than the scorecard it is the squad size -- 11 for 43,483 of 44,850 sides, 12 for 1,337
// where a concussion or injury replacement took the field. It carries essentially no
// signal, it is constant at selection time so it cannot separate two candidate XIs, and
// it is the column through which the result leaked in the first place.
var winDistStatSuffixes = []string{"_sum", "_mean", "_std", "_max", "_min", "_top3_mean"}

// winBaseHeaders are the win export's non-feature columns, in emission order.
//
// It is a named list because the header builder and the row scanner must agree on it,
// and for a long time they did not: the scanner emitted a duplicate Unix match_date and
// seven weather zeros that the header never named. Pandas resolves a short header by
// consuming the surplus as an index, so the win CSV silently mapped its eight leading
// data columns onto nothing and shifted every named column eight places -- including
// team1_wins, the training target, which became a constant.
var winBaseHeaders = []string{
	"match_id",
	"venue_id",
	"team1_opposition_id",
	"team2_opposition_id",
	"team1_wins",
	"format_code",
	"match_date",
}

func winEnhancedHeaders() []string {
	base := make([]string, 0, len(winBaseHeaders)+len(winDistStatSuffixes)*len(winFeatureGroupNames))
	base = append(base, winBaseHeaders...)
	for _, group := range winFeatureGroupNames {
		for _, suffix := range winDistStatSuffixes {
			base = append(base, group+suffix)
		}
	}
	return base
}

func scanWinEnhancedRow(rows interface{ Scan(dest ...any) error }) ([]string, error) {
	var matchID, venueID, team1, team2 int64
	var team1Wins int
	var formatCode string
	var matchDate time.Time

	var groups [8]featureDistStats

	dest := make([]any, 0, len(winBaseHeaders)+len(winDistStatSuffixes)*len(groups))
	dest = append(
		dest,
		&matchID, &venueID, &team1, &team2, &team1Wins, &formatCode,
		&matchDate,
	)
	for i := range groups {
		g := &groups[i] //nolint:gosec // fixed-size array indexed by range
		dest = append(
			dest,
			&g.sum, &g.mean, &g.std,
			&g.max, &g.min, &g.top3Mean,
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

	// One value per name in winBaseHeaders, in that order. The two lists are asserted
	// equal in width by TestWinExportRowMatchesItsHeader; they were not, and the eight
	// surplus values silently shifted every column of every row.
	row := []string{
		strconv.FormatInt(matchID, 10),
		strconv.FormatInt(venueID, 10),
		strconv.FormatInt(team1, 10),
		strconv.FormatInt(team2, 10),
		strconv.Itoa(team1Wins),
		formatCode,
		matchDate.Format("2006-01-02"),
	}
	for i := range groups {
		g := &groups[i] //nolint:gosec // fixed-size array indexed by range
		row = append(row, g.toStrings()...)
	}
	return row, nil
}
