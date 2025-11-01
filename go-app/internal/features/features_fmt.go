package features

import (
	"context"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ComputeSeasonalFormFmt computes seasonal form per player per season per format
// and upserts into player_form_data_fmt. If seasonName is empty, computes for all seasons present.
func ComputeSeasonalFormFmt(ctx context.Context, seasonName string, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		return err
	}
	var seasonIDFilter string
	if seasonName != "" {
		sid, err := db.GetOrCreateSeason(ctx, seasonName)
		if err != nil {
			return err
		}
		seasonIDFilter = fmt.Sprintf("AND md.season_id = %d", sid)
	}
	q := fmt.Sprintf(`
	WITH batting AS (
		SELECT bd.player_id, md.season_id, md.format_id,
			AVG(COALESCE(bd.runs,0))::real AS batting_form
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.season_id IS NOT NULL AND md.format_id = %d %s
		GROUP BY bd.player_id, md.season_id, md.format_id
	), bowling AS (
		SELECT bw.player_id, md.season_id, md.format_id,
			AVG(COALESCE(bw.wickets,0))::real AS bowling_form
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.season_id IS NOT NULL AND md.format_id = %d %s
		GROUP BY bw.player_id, md.season_id, md.format_id
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
	return err
}

// ComputeVenueEffectsFmt computes venue effects per player per venue per format.
func ComputeVenueEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`
	WITH batting AS (
		SELECT bd.player_id, md.venue_id, md.format_id,
			AVG(COALESCE(bd.runs,0))::real AS batting_venue
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.venue_id IS NOT NULL AND md.format_id = %d
		GROUP BY bd.player_id, md.venue_id, md.format_id
	), bowling AS (
		SELECT bw.player_id, md.venue_id, md.format_id,
			AVG(COALESCE(bw.wickets,0))::real AS bowling_venue
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.venue_id IS NOT NULL AND md.format_id = %d
		GROUP BY bw.player_id, md.venue_id, md.format_id
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
	return err
}

// ComputeOppositionEffectsFmt computes opposition effects per player per opposition per format.
func ComputeOppositionEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	fmtID, err := db.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`
	WITH batting AS (
		SELECT bd.player_id, md.opposition_id, md.format_id,
			AVG(COALESCE(bd.runs,0))::real AS batting_opposition
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.opposition_id IS NOT NULL AND md.format_id = %d
		GROUP BY bd.player_id, md.opposition_id, md.format_id
	), bowling AS (
		SELECT bw.player_id, md.opposition_id, md.format_id,
			AVG(COALESCE(bw.wickets,0))::real AS bowling_opposition
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.opposition_id IS NOT NULL AND md.format_id = %d
		GROUP BY bw.player_id, md.opposition_id, md.format_id
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
	return err
}
