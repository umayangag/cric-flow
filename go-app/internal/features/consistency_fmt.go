// Package features: consistency computations with parity to Python formulas.
package features

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ComputeConsistencyFmt computes batting and bowling consistency per player for the
// given season and format, using the exact coefficients and thresholds from the
// Python implementation. Note: The original Python computes consistency from ALL
// innings for a player (no season/format filter) and then writes the same value
// into the season+format row. We mirror that behavior for parity.
func ComputeConsistencyFmt(ctx context.Context, seasonName string, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("features.ComputeConsistencyFmt Connect failed", slog.Any("err", err))
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		slog.Error(
			"features.ComputeConsistencyFmt GetMatchFormatIDByCode failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return err
	}
	var seasonIDCond string
	if seasonName != "" {
		sid, err := db.GetOrCreateSeason(ctx, seasonName)
		if err != nil {
			slog.Error(
				"features.ComputeConsistencyFmt GetOrCreateSeason failed",
				slog.String("season", seasonName),
				slog.Any("err", err),
			)
			return err
		}
		seasonIDCond = fmt.Sprintf("AND season_id = %d", sid)
	}

	// Batting consistency:
	// 0.4262*avg_score + 0.2566*innings + 0.1510*avg_sr + 0.0787*centuries + 0.0556*fifties - 0.0328*ducks
	// threshold: innings > 10
	batting := fmt.Sprintf(`
	WITH agg AS (
		SELECT bd.player_id,
			COUNT(*) AS innings,
			AVG(COALESCE(bd.strike_rate,0)) AS avg_sr,
			SUM(COALESCE(bd.runs,0)) AS sum_runs,
			SUM(CASE WHEN bd.description = 'not out' THEN 1 ELSE 0 END) AS not_outs,
			SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS centuries,
			SUM(CASE WHEN bd.runs >= 50 THEN 1 ELSE 0 END) - SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS fifties,
			SUM(CASE WHEN bd.runs = 0 THEN 1 ELSE 0 END) AS ducks
		FROM batting_data bd
		-- Note: Python version does NOT filter by season/format for consistency
		GROUP BY bd.player_id
	), cons AS (
		SELECT player_id,
			(0.4262 * (CASE WHEN innings - not_outs > 0 THEN (sum_runs::real)/(innings - not_outs) ELSE sum_runs::real END)
			 + 0.2566 * innings
			 + 0.1510 * avg_sr
			 + 0.0787 * centuries
			 + 0.0556 * fifties
			 - 0.0328 * ducks)::real AS batting_consistency
		FROM agg WHERE innings > 10
	)
	INSERT INTO player_consistency_data_fmt(player_id, season_id, format_id, batting_consistency, bowling_consistency)
	SELECT c.player_id, s.season_id, %d AS format_id, c.batting_consistency, COALESCE(pcdf.bowling_consistency, NULL)
	FROM cons c
	JOIN (
		SELECT DISTINCT player_id, season_id FROM batting_data bd
		JOIN match m ON m.match_id = bd.match_id
		WHERE m.format_id = %d %s
	) s ON s.player_id = c.player_id
	LEFT JOIN player_consistency_data_fmt pcdf ON pcdf.player_id = c.player_id AND pcdf.season_id = s.season_id AND pcdf.format_id = %d
	ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET
		batting_consistency = EXCLUDED.batting_consistency;`, fmtID, fmtID, seasonIDCond, fmtID)

	if _, err := db.Pool.Exec(ctx, batting); err != nil {
		slog.Error(
			"features.ComputeConsistencyFmt batting Exec failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return err
	}

	// Bowling consistency:
	// 0.4174*total_overs + 0.2634*innings + 0.1602*strike_rate + 0.0975*average + 0.0615*five_fors
	// threshold: innings > 10
	bowling := fmt.Sprintf(`
	WITH agg AS (
		SELECT bw.player_id,
			COUNT(*) AS innings,
			SUM(COALESCE(bw.balls,0)) AS sum_balls,
			SUM(COALESCE(bw.runs,0)) AS sum_runs,
			SUM(COALESCE(bw.wickets,0)) AS sum_wkts,
			SUM(CASE WHEN bw.wickets >= 5 THEN 1 ELSE 0 END) AS five_fors
		FROM bowling_data bw
		GROUP BY bw.player_id
	), cons AS (
		SELECT player_id,
			(0.4174 * ((sum_balls::real)/6.0)
			 + 0.2634 * innings
			 + 0.1602 * (CASE WHEN sum_wkts > 0 THEN (sum_balls::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0975 * (CASE WHEN sum_wkts > 0 THEN (sum_runs::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0615 * five_fors)::real AS bowling_consistency
		FROM agg WHERE innings > 10
	)
	UPDATE player_consistency_data_fmt pcdf SET bowling_consistency = cons.bowling_consistency
	FROM cons
	WHERE pcdf.player_id = cons.player_id AND pcdf.format_id = %d %s;`, fmtID, seasonIDCond)

	_, err = db.Pool.Exec(ctx, bowling)
	if err != nil {
		slog.Error(
			"features.ComputeConsistencyFmt bowling Exec failed",
			slog.String("format", formatCode),
			slog.Any("err", err),
		)
		return err
	}
	return nil
}
