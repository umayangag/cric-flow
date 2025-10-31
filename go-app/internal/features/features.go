package features

import (
	"context"
	"fmt"

	"github.com/umayangag/cric-app/go-app/internal/db"
)

// ComputeSeasonalForm computes per-player seasonal form metrics (simple averages)
// and upserts into player_form_data. It mirrors the spirit of the prototype's
// seasonal form, using available tables. It is idempotent.
func ComputeSeasonalForm(ctx context.Context, seasonName string) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	// Ensure season exists and get its id when provided; if blank, compute for all seasons present in match_details.
	var seasonIDFilter string
	if seasonName != "" {
		sid, err := db.GetOrCreateSeason(ctx, seasonName)
		if err != nil {
			return err
		}
		seasonIDFilter = fmt.Sprintf("AND md.season_id = %d", sid)
	}
	// Compute batting_form as AVG(runs) per player x season; bowling_form as AVG(wickets) per player x season.
	q := fmt.Sprintf(`
	WITH batting AS (
		SELECT bd.player_id, md.season_id, AVG(COALESCE(bd.runs,0))::real AS batting_form
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.season_id IS NOT NULL %s
		GROUP BY bd.player_id, md.season_id
	), bowling AS (
		SELECT bw.player_id, md.season_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_form
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.season_id IS NOT NULL %s
		GROUP BY bw.player_id, md.season_id
	)
	INSERT INTO player_form_data(player_id, season_id, batting_form, bowling_form)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.season_id, w.season_id) AS season_id,
	       b.batting_form,
	       w.bowling_form
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.season_id = w.season_id
	ON CONFLICT (player_id, season_id) DO UPDATE SET
		batting_form = COALESCE(EXCLUDED.batting_form, player_form_data.batting_form),
		bowling_form = COALESCE(EXCLUDED.bowling_form, player_form_data.bowling_form);
	`, seasonIDFilter, seasonIDFilter)
	_, err := db.Pool.Exec(ctx, q)
	return err
}

// ComputeVenueEffects computes player_venue_data entries by aggregating performances per venue.
// batting_venue: AVG(runs) for batter at venue; bowling_venue: AVG(wickets) for bowler at venue.
func ComputeVenueEffects(ctx context.Context) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	q := `
	WITH batting AS (
		SELECT bd.player_id, md.venue_id, AVG(COALESCE(bd.runs,0))::real AS batting_venue
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.venue_id IS NOT NULL
		GROUP BY bd.player_id, md.venue_id
	), bowling AS (
		SELECT bw.player_id, md.venue_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_venue
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.venue_id IS NOT NULL
		GROUP BY bw.player_id, md.venue_id
	)
	INSERT INTO player_venue_data(player_id, venue_id, batting_venue, bowling_venue)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.venue_id, w.venue_id) AS venue_id,
	       b.batting_venue,
	       w.bowling_venue
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.venue_id = w.venue_id
	ON CONFLICT (player_id, venue_id) DO UPDATE SET
		batting_venue = COALESCE(EXCLUDED.batting_venue, player_venue_data.batting_venue),
		bowling_venue = COALESCE(EXCLUDED.bowling_venue, player_venue_data.bowling_venue);
	`
	_, err := db.Pool.Exec(ctx, q)
	return err
}

// ComputeOppositionEffects computes player_opposition_data by aggregating performances per opposition.
// batting_opposition: AVG(runs); bowling_opposition: AVG(wickets).
func ComputeOppositionEffects(ctx context.Context) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	q := `
	WITH batting AS (
		SELECT bd.player_id, md.opposition_id, AVG(COALESCE(bd.runs,0))::real AS batting_opposition
		FROM batting_data bd
		JOIN match_details md ON md.match_id = bd.match_id
		WHERE md.opposition_id IS NOT NULL
		GROUP BY bd.player_id, md.opposition_id
	), bowling AS (
		SELECT bw.player_id, md.opposition_id, AVG(COALESCE(bw.wickets,0))::real AS bowling_opposition
		FROM bowling_data bw
		JOIN match_details md ON md.match_id = bw.match_id
		WHERE md.opposition_id IS NOT NULL
		GROUP BY bw.player_id, md.opposition_id
	)
	INSERT INTO player_opposition_data(player_id, opposition_id, batting_opposition, bowling_opposition)
	SELECT COALESCE(b.player_id, w.player_id) AS player_id,
	       COALESCE(b.opposition_id, w.opposition_id) AS opposition_id,
	       b.batting_opposition,
	       w.bowling_opposition
	FROM batting b
	FULL OUTER JOIN bowling w ON b.player_id = w.player_id AND b.opposition_id = w.opposition_id
	ON CONFLICT (player_id, opposition_id) DO UPDATE SET
		batting_opposition = COALESCE(EXCLUDED.batting_opposition, player_opposition_data.batting_opposition),
		bowling_opposition = COALESCE(EXCLUDED.bowling_opposition, player_opposition_data.bowling_opposition);
	`
	_, err := db.Pool.Exec(ctx, q)
	return err
}

// UpdatePlayerConsistency updates player.batting_consistency and player.bowling_consistency
// using population standard deviation of runs/wickets across all matches for the player.
func UpdatePlayerConsistency(ctx context.Context) error {
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	// Lower stddev implies more consistency; we can store inverse or keep raw stddev.
	// Here we keep raw stddev and let downstream consumers transform as needed.
	q := `
	WITH bat AS (
		SELECT bd.player_id, COALESCE(stddev_pop(bd.runs)::real, 0) AS batting_stddev
		FROM batting_data bd
		GROUP BY bd.player_id
	), bowl AS (
		SELECT bw.player_id, COALESCE(stddev_pop(bw.wickets)::real, 0) AS bowling_stddev
		FROM bowling_data bw
		GROUP BY bw.player_id
	)
	UPDATE player p SET
		batting_consistency = COALESCE(bat.batting_stddev, p.batting_consistency),
		bowling_consistency = COALESCE(bowl.bowling_stddev, p.bowling_consistency)
	FROM bat LEFT JOIN bowl ON bat.player_id = bowl.player_id
	WHERE p.id = COALESCE(bat.player_id, bowl.player_id);
	`
	_, err := db.Pool.Exec(ctx, q)
	return err
}
