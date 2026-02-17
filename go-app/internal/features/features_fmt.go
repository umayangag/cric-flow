// Package features contains helpers for computing per-player features.
package features

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ComputeSeasonalFormFmt computes seasonal form per player per season per format
// using the exact weighted formulas and thresholds from the Python implementation
// and upserts into player_form_data_fmt. If seasonName is empty, computes for all seasons present.
func ComputeSeasonalFormFmt(ctx context.Context, seasonName string, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("features.ComputeSeasonalFormFmt Connect failed", slog.Any("err", err))
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		slog.Error("features.ComputeSeasonalFormFmt GetMatchFormatIDByCode failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	var seasonIDFilter string
	if seasonName != "" {
		sid, err := db.GetOrCreateSeason(ctx, seasonName)
		if err != nil {
			slog.Error("features.ComputeSeasonalFormFmt GetOrCreateSeason failed", slog.String("season", seasonName), slog.Any("err", err))
			return err
		}
		seasonIDFilter = fmt.Sprintf("AND m.season_id = %d", sid)
	}
	// Batting form formula:
	// 0.4262*avg_score + 0.2566*innings + 0.1510*avg_sr + 0.0787*centuries + 0.0556*fifties - 0.0328*ducks
	// threshold: innings > 5
	q := fmt.Sprintf(`
	WITH batting_base AS (
		SELECT bd.player_id, m.season_id, m.format_id,
			COUNT(*) AS innings,
			AVG(COALESCE(bd.strike_rate,0)) AS avg_sr,
			SUM(COALESCE(bd.runs,0)) AS sum_runs,
			SUM(CASE WHEN bd.description = 'not out' THEN 1 ELSE 0 END) AS not_outs,
			SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS centuries,
			SUM(CASE WHEN bd.runs >= 50 THEN 1 ELSE 0 END) - SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS fifties,
			SUM(CASE WHEN bd.runs = 0 THEN 1 ELSE 0 END) AS ducks
		FROM batting_data bd
		JOIN match m ON m.match_id = bd.match_id
		WHERE m.season_id IS NOT NULL AND m.format_id = %d %s
		GROUP BY bd.player_id, m.season_id, m.format_id
	), batting AS (
		SELECT player_id, season_id, format_id,
			innings,
			CASE WHEN innings - not_outs > 0 THEN (sum_runs::real)/(innings - not_outs) ELSE sum_runs::real END AS avg_score,
			avg_sr::real, centuries::real, fifties::real, ducks::real,
			(0.4262 * (CASE WHEN innings - not_outs > 0 THEN (sum_runs::real)/(innings - not_outs) ELSE sum_runs::real END)
			 + 0.2566 * innings
			 + 0.1510 * avg_sr
			 + 0.0787 * centuries
			 + 0.0556 * fifties
			 - 0.0328 * ducks)::real AS batting_form
		FROM batting_base
		WHERE innings > 5
	), bowling_base AS (
		SELECT bw.player_id, m.season_id, m.format_id,
			COUNT(*) AS innings,
			SUM(COALESCE(bw.balls,0)) AS sum_balls,
			SUM(COALESCE(bw.runs,0)) AS sum_runs,
			SUM(COALESCE(bw.wickets,0)) AS sum_wkts,
			SUM(CASE WHEN bw.wickets >= 5 THEN 1 ELSE 0 END) AS five_fors
		FROM bowling_data bw
		JOIN match m ON m.match_id = bw.match_id
		WHERE m.season_id IS NOT NULL AND m.format_id = %d %s
		GROUP BY bw.player_id, m.season_id, m.format_id
	), bowling AS (
		SELECT player_id, season_id, format_id,
			innings,
			(sum_balls::real)/6.0 AS total_overs,
			CASE WHEN sum_wkts > 0 THEN (sum_balls::real)/(sum_wkts::real) ELSE NULL END AS strike_rate,
			CASE WHEN sum_wkts > 0 THEN (sum_runs::real)/(sum_wkts::real) ELSE NULL END AS average,
			five_fors::real,
			(0.4174 * ((sum_balls::real)/6.0)
			 + 0.2634 * innings
			 + 0.1602 * (CASE WHEN sum_wkts > 0 THEN (sum_balls::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0975 * (CASE WHEN sum_wkts > 0 THEN (sum_runs::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0615 * five_fors)::real AS bowling_form
		FROM bowling_base
		WHERE innings > 5
	)
	INSERT INTO player_form_data_fmt(player_id, season_id, format_id, batting_form, bowling_form)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.season_id, w.season_id) AS season_id,
	       COALESCE(b.format_id, w.format_id) AS format_id,
	       b.batting_form,
	       w.bowling_form
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.season_id = w.season_id AND b.format_id = w.format_id
	ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET
		batting_form = COALESCE(EXCLUDED.batting_form, player_form_data_fmt.batting_form),
		bowling_form = COALESCE(EXCLUDED.bowling_form, player_form_data_fmt.bowling_form);
	`, fmtID, seasonIDFilter, fmtID, seasonIDFilter)
	_, err = db.Pool.Exec(ctx, q)
	if err != nil {
		slog.Error("features.ComputeSeasonalFormFmt Exec failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	return nil
}

// ComputeVenueEffectsFmt computes venue effects per player per venue per format using
// the exact weighted formulas and thresholds from the Python implementation.
func ComputeVenueEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("features.ComputeVenueEffectsFmt Connect failed", slog.Any("err", err))
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		slog.Error("features.ComputeVenueEffectsFmt GetMatchFormatIDByCode failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	q := fmt.Sprintf(`
	WITH batting_base AS (
		SELECT bd.player_id, m.venue_id, m.format_id,
			COUNT(*) AS innings,
			AVG(COALESCE(bd.strike_rate,0)) AS avg_sr,
			SUM(COALESCE(bd.runs,0)) AS sum_runs,
			SUM(CASE WHEN bd.description = 'not out' THEN 1 ELSE 0 END) AS not_outs,
			SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS centuries,
			SUM(CASE WHEN bd.runs >= 50 THEN 1 ELSE 0 END) - SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS fifties,
			MAX(COALESCE(bd.runs,0)) AS highest_score
		FROM batting_data bd
		JOIN match m ON m.match_id = bd.match_id
		WHERE m.venue_id IS NOT NULL AND m.format_id = %d
		GROUP BY bd.player_id, m.venue_id, m.format_id
	), batting AS (
		SELECT player_id, venue_id, format_id,
			(0.4262 * (CASE WHEN innings - not_outs > 0 THEN (sum_runs::real)/(innings - not_outs) ELSE sum_runs::real END)
			 + 0.2566 * innings
			 + 0.1510 * avg_sr
			 + 0.0787 * centuries
			 + 0.0556 * fifties
			 + 0.0328 * highest_score)::real AS batting_venue
		FROM batting_base
		WHERE innings > 5
	), bowling_base AS (
		SELECT bw.player_id, m.venue_id, m.format_id,
			COUNT(*) AS innings,
			SUM(COALESCE(bw.balls,0)) AS sum_balls,
			SUM(COALESCE(bw.runs,0)) AS sum_runs,
			SUM(COALESCE(bw.wickets,0)) AS sum_wkts,
			SUM(CASE WHEN bw.wickets >= 5 THEN 1 ELSE 0 END) AS five_fors
		FROM bowling_data bw
		JOIN match m ON m.match_id = bw.match_id
		WHERE m.venue_id IS NOT NULL AND m.format_id = %d
		GROUP BY bw.player_id, m.venue_id, m.format_id
	), bowling AS (
		SELECT player_id, venue_id, format_id,
			(0.4174 * ((sum_balls::real)/6.0)
			 + 0.2634 * innings
			 + 0.1602 * (CASE WHEN sum_wkts > 0 THEN (sum_balls::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0975 * (CASE WHEN sum_wkts > 0 THEN (sum_runs::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0615 * five_fors)::real AS bowling_venue
		FROM bowling_base
		WHERE innings > 5
	)
	INSERT INTO player_venue_data_fmt(player_id, venue_id, format_id, batting_venue, bowling_venue)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.venue_id, w.venue_id) AS venue_id,
	       COALESCE(b.format_id, w.format_id) AS format_id,
	       b.batting_venue,
	       w.bowling_venue
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.venue_id = w.venue_id AND b.format_id = w.format_id
	ON CONFLICT (player_id, venue_id, format_id) DO UPDATE SET
		batting_venue = COALESCE(EXCLUDED.batting_venue, player_venue_data_fmt.batting_venue),
		bowling_venue = COALESCE(EXCLUDED.bowling_venue, player_venue_data_fmt.bowling_venue);
	`, fmtID, fmtID)
	_, err = db.Pool.Exec(ctx, q)
	if err != nil {
		slog.Error("features.ComputeVenueEffectsFmt Exec failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	return nil
}

// ComputeOppositionEffectsFmt computes opposition effects per player per opposition per format using
// the exact weighted formulas and thresholds from the Python implementation.
func ComputeOppositionEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("features.ComputeOppositionEffectsFmt Connect failed", slog.Any("err", err))
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		slog.Error("features.ComputeOppositionEffectsFmt GetMatchFormatIDByCode failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	q := fmt.Sprintf(`
	WITH batting_base AS (
		SELECT bd.player_id, mi.bowling_team_opposition_id, m.format_id,
			COUNT(*) AS innings,
			AVG(COALESCE(bd.strike_rate,0)) AS avg_sr,
			SUM(COALESCE(bd.runs,0)) AS sum_runs,
			SUM(CASE WHEN bd.description = 'not out' THEN 1 ELSE 0 END) AS not_outs,
			SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS centuries,
			SUM(CASE WHEN bd.runs >= 50 THEN 1 ELSE 0 END) - SUM(CASE WHEN bd.runs >= 100 THEN 1 ELSE 0 END) AS fifties,
			MAX(COALESCE(bd.runs,0)) AS highest_score
		FROM batting_data bd
		JOIN match m ON m.match_id = bd.match_id
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		WHERE mi.bowling_team_opposition_id IS NOT NULL AND m.format_id = %d
		GROUP BY bd.player_id, mi.bowling_team_opposition_id, m.format_id
	), batting AS (
		SELECT player_id, bowling_team_opposition_id AS opposition_id, format_id,
			(0.4262 * (CASE WHEN innings - not_outs > 0 THEN (sum_runs::real)/(innings - not_outs) ELSE sum_runs::real END)
			 + 0.2566 * innings
			 + 0.1510 * avg_sr
			 + 0.0787 * centuries
			 + 0.0556 * fifties
			 + 0.0328 * highest_score)::real AS batting_opposition
		FROM batting_base
		WHERE innings > 5
	), bowling_base AS (
		SELECT bw.player_id, mi.batting_team_opposition_id, m.format_id,
			COUNT(*) AS innings,
			SUM(COALESCE(bw.balls,0)) AS sum_balls,
			SUM(COALESCE(bw.runs,0)) AS sum_runs,
			SUM(COALESCE(bw.wickets,0)) AS sum_wkts,
			SUM(CASE WHEN bw.wickets >= 5 THEN 1 ELSE 0 END) AS five_fors
		FROM bowling_data bw
		JOIN match m ON m.match_id = bw.match_id
		JOIN match_inning mi ON mi.match_id = bw.match_id AND mi.inning_number = bw.inning_number
		WHERE mi.batting_team_opposition_id IS NOT NULL AND m.format_id = %d
		GROUP BY bw.player_id, mi.batting_team_opposition_id, m.format_id
	), bowling AS (
		SELECT player_id, batting_team_opposition_id AS opposition_id, format_id,
			(0.4174 * ((sum_balls::real)/6.0)
			 + 0.2634 * innings
			 + 0.1602 * (CASE WHEN sum_wkts > 0 THEN (sum_balls::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0975 * (CASE WHEN sum_wkts > 0 THEN (sum_runs::real)/(sum_wkts::real) ELSE 0 END)
			 + 0.0615 * five_fors)::real AS bowling_opposition
		FROM bowling_base
		WHERE innings > 5
	)
	INSERT INTO player_opposition_data_fmt(player_id, opposition_id, format_id, batting_opposition, bowling_opposition)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.opposition_id, w.opposition_id) AS opposition_id,
	       COALESCE(b.format_id, w.format_id) AS format_id,
	       b.batting_opposition,
	       w.bowling_opposition
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.opposition_id = w.opposition_id AND b.format_id = w.format_id
	ON CONFLICT (player_id, opposition_id, format_id) DO UPDATE SET
		batting_opposition = COALESCE(EXCLUDED.batting_opposition, player_opposition_data_fmt.batting_opposition),
		bowling_opposition = COALESCE(EXCLUDED.bowling_opposition, player_opposition_data_fmt.bowling_opposition);
	`, fmtID, fmtID)
	_, err = db.Pool.Exec(ctx, q)
	if err != nil {
		slog.Error("features.ComputeOppositionEffectsFmt Exec failed", slog.String("format", formatCode), slog.Any("err", err))
		return err
	}
	return nil
}
