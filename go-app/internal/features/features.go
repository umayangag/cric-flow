package features

import (
	"context"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ComputeSeasonalFormFmt computes per-player seasonal form metrics (simple averages)
// and upserts into player_form_data_fmt. It is idempotent.
func ComputeSeasonalFormFmt(ctx context.Context, seasonName, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	sid, err := db.GetOrCreateSeason(ctx, seasonName)
	if err != nil {
		return err
	}
	fid, err := db.GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return err
	}

	q := `
		WITH batting AS (
			SELECT bd.player_id, md.season_id, md.format_id, AVG(COALESCE(bd.runs,0))::real AS batting_form
			FROM batting_data bd
			JOIN match_details md ON md.match_id = bd.match_id
			WHERE md.season_id = $1 AND md.format_id = $2
			GROUP BY bd.player_id, md.season_id, md.format_id
		), bowling AS (
			SELECT bw.player_id, md.season_id, md.format_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_form
			FROM bowling_data bw
			JOIN match_details md ON md.match_id = bw.match_id
			WHERE md.season_id = $1 AND md.format_id = $2
			GROUP BY bw.player_id, md.season_id, md.format_id
		)
		INSERT INTO player_form_data_fmt(player_id, season_id, format_id, batting_form, bowling_form)
		SELECT COALESCE(b.player_id, w.player_id) AS player_id,
			   $1, -- season_id
			   $2, -- format_id
			   b.batting_form,
			   w.bowling_form
		FROM batting b
		FULL OUTER JOIN bowling w ON b.player_id = w.player_id
		ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET
			batting_form = COALESCE(EXCLUDED.batting_form, player_form_data_fmt.batting_form),
			bowling_form = COALESCE(EXCLUDED.bowling_form, player_form_data_fmt.bowling_form);
	`
	_, err = db.Pool.Exec(ctx, q, sid, fid)
	return err
}

// ComputeVenueEffectsFmt computes player_venue_data_fmt entries by aggregating performances per venue.
func ComputeVenueEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	fid, err := db.GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return err
	}
	q := `
		WITH batting AS (
			SELECT bd.player_id, md.venue_id, md.format_id, AVG(COALESCE(bd.runs,0))::real AS batting_venue
			FROM batting_data bd
			JOIN match_details md ON md.match_id = bd.match_id
			WHERE md.venue_id IS NOT NULL AND md.format_id = $1
			GROUP BY bd.player_id, md.venue_id, md.format_id
		), bowling AS (
			SELECT bw.player_id, md.venue_id, md.format_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_venue
			FROM bowling_data bw
			JOIN match_details md ON md.match_id = bw.match_id
			WHERE md.venue_id IS NOT NULL AND md.format_id = $1
			GROUP BY bw.player_id, md.venue_id, md.format_id
		)
		INSERT INTO player_venue_data_fmt(player_id, venue_id, format_id, batting_venue, bowling_venue)
		SELECT COALESCE(b.player_id, w.player_id) AS player_id,
			   COALESCE(b.venue_id, w.venue_id) AS venue_id,
			   $1, -- format_id
			   b.batting_venue,
			   w.bowling_venue
		FROM batting b
		FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.venue_id = w.venue_id
		ON CONFLICT (player_id, venue_id, format_id) DO UPDATE SET
			batting_venue = COALESCE(EXCLUDED.batting_venue, player_venue_data_fmt.batting_venue),
			bowling_venue = COALESCE(EXCLUDED.bowling_venue, player_venue_data_fmt.bowling_venue);
	`
	_, err = db.Pool.Exec(ctx, q, fid)
	return err
}

// ComputeOppositionEffectsFmt computes player_opposition_data_fmt by aggregating performances per opposition.
func ComputeOppositionEffectsFmt(ctx context.Context, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	fid, err := db.GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return err
	}
	q := `
		WITH batting AS (
			SELECT bd.player_id, md.opposition_id, md.format_id, AVG(COALESCE(bd.runs,0))::real AS batting_opposition
			FROM batting_data bd
			JOIN match_details md ON md.match_id = bd.match_id
			WHERE md.opposition_id IS NOT NULL AND md.format_id = $1
			GROUP BY bd.player_id, md.opposition_id, md.format_id
		), bowling AS (
			SELECT bw.player_id, md.opposition_id, md.format_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_opposition
			FROM bowling_data bw
			JOIN match_details md ON md.match_id = bw.match_id
			WHERE md.opposition_id IS NOT NULL AND md.format_id = $1
			GROUP BY bw.player_id, md.opposition_id, md.format_id
		)
		INSERT INTO player_opposition_data_fmt(player_id, opposition_id, format_id, batting_opposition, bowling_opposition)
		SELECT COALESCE(b.player_id, w.player_id) AS player_id,
			   COALESCE(b.opposition_id, w.opposition_id) AS opposition_id,
			   $1, -- format_id
			   b.batting_opposition,
			   w.bowling_opposition
		FROM batting b
		FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.opposition_id = w.opposition_id
		ON CONFLICT (player_id, opposition_id, format_id) DO UPDATE SET
			batting_opposition = COALESCE(EXCLUDED.batting_opposition, player_opposition_data_fmt.batting_opposition),
			bowling_opposition = COALESCE(EXCLUDED.bowling_opposition, player_opposition_data_fmt.bowling_opposition);
	`
	_, err = db.Pool.Exec(ctx, q, fid)
	return err
}

// ComputePlayerConsistencyFmt computes player consistency (stddev of runs/wickets)
// for a given season and format, and upserts into player_consistency_data_fmt.
func ComputePlayerConsistencyFmt(ctx context.Context, seasonName, formatCode string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}

	sid, err := db.GetOrCreateSeason(ctx, seasonName)
	if err != nil {
		return err
	}

	fid, err := db.GetOrCreateMatchFormat(ctx, formatCode)
	if err != nil {
		return err
	}

	q := `
		WITH bat AS (
			SELECT
				bd.player_id,
				md.season_id,
				md.format_id,
				COALESCE(stddev_pop(bd.runs)::real, 0) AS batting_stddev
			FROM batting_data bd
			JOIN match_details md ON md.match_id = bd.match_id
			WHERE md.season_id = $1 AND md.format_id = $2
			GROUP BY bd.player_id, md.season_id, md.format_id
		), bowl AS (
			SELECT
				bw.player_id,
				md.season_id,
				md.format_id,
				COALESCE(stddev_pop(bw.wickets)::real, 0) AS bowling_stddev
			FROM bowling_data bw
			JOIN match_details md ON md.match_id = bw.match_id
			WHERE md.season_id = $1 AND md.format_id = $2
			GROUP BY bw.player_id, md.season_id, md.format_id
		)
		INSERT INTO player_consistency_data_fmt (player_id, season_id, format_id, batting_consistency, bowling_consistency)
		SELECT
			COALESCE(bat.player_id, bowl.player_id),
			$1, -- season_id
			$2, -- format_id
			bat.batting_stddev,
			bowl.bowling_stddev
		FROM bat
		FULL OUTER JOIN bowl ON bat.player_id = bowl.player_id
		ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET
			batting_consistency = EXCLUDED.batting_consistency,
			bowling_consistency = EXCLUDED.bowling_consistency
	`

	_, err = db.Pool.Exec(ctx, q, sid, fid)
	return err
}
