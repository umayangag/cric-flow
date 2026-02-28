package db_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestGetMatchFeatureContext_PoolNil(t *testing.T) {
	// When defaultDB is not set, GetMatchFeatureContext returns error.
	db.SetDB(nil)
	defer func() { db.SetDB(nil) }() // leave clean for other tests

	ctx := context.Background()
	got, err := db.GetMatchFeatureContext(ctx, 1)
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "db pool not initialized")
}

func TestGetMatchFeatureContext_HappyPath(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	db.SetDB(mockDB{pool: mock})

	// Match row: format_id=2, venue_id=10, season_id=5 (use nil for nullable to avoid Scan ptr issues in mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT format_id, venue_id, season_id FROM match WHERE match_id = $1`)).
		WithArgs(int64(100)).
		WillReturnRows(pgxmock.NewRows([]string{"format_id", "venue_id", "season_id"}).
			AddRow(int64(2), nil, nil))

	// Batting: two players (mock returns nil for *int64 columns to satisfy Scan)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bd.player_id, mi.bowling_team_opposition_id
		FROM batting_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		WHERE bd.match_id = $1`)).
		WithArgs(int64(100)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "bowling_team_opposition_id"}).
			AddRow(int64(1), nil).
			AddRow(int64(2), nil))

	// Bowling: same two players
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bd.player_id, mi.batting_team_opposition_id
		FROM bowling_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		WHERE bd.match_id = $1`)).
		WithArgs(int64(100)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "batting_team_opposition_id"}).
			AddRow(int64(1), nil).
			AddRow(int64(2), nil))

	got, err := db.GetMatchFeatureContext(ctx, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(2), got.FormatID)
	require.Nil(t, got.VenueID) // mock returned nil for nullable columns
	require.Nil(t, got.SeasonID)
	require.Len(t, got.PlayerOpps, 2)
	byPID := make(map[int64]db.PlayerOpposition)
	for _, po := range got.PlayerOpps {
		byPID[po.PlayerID] = po
	}
	// Opposition IDs were nil in mock (Scan into *int64 works with nil)
	require.Nil(t, byPID[1].BattingOppositionID)
	require.Nil(t, byPID[1].BowlingOppositionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMatchFeatureContext_MatchNotFound(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	db.SetDB(mockDB{pool: mock})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT format_id, venue_id, season_id FROM match WHERE match_id = $1`)).
		WithArgs(int64(999)).
		WillReturnError(errors.New("no rows"))

	got, err := db.GetMatchFeatureContext(ctx, 999)
	require.Error(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMatchPlayerTeams_PoolNil(t *testing.T) {
	db.SetDB(nil)
	defer func() { db.SetDB(nil) }()

	ctx := context.Background()
	got, err := db.GetMatchPlayerTeams(ctx, 1)
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "db pool not initialized")
}

func TestGetMatchPlayerTeams_HappyPath(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	db.SetDB(mockDB{pool: mock})

	// UNION: batting gives player_id, team; bowling gives player_id, team. First occurrence wins.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM batting_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.batting_team_opposition_id
		WHERE bd.match_id = $1
		UNION
		SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM bowling_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.bowling_team_opposition_id
		WHERE bd.match_id = $2`)).
		WithArgs(int64(50), int64(50)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "opposition_name"}).
			AddRow(int64(1), "IND").
			AddRow(int64(2), "AUS"))

	// Field-only query: no field-only players in this test
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT fd.player_id, COALESCE(
			(SELECT o2.opposition_name FROM fielding_event fe
			 JOIN match_inning mi2 ON mi2.match_id = fe.match_id AND mi2.inning_number = fe.innings
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE fe.match_id = fd.match_id AND fe.fielder_id = fd.player_id
			 LIMIT 1),
			(SELECT o2.opposition_name FROM match_inning mi2
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE mi2.match_id = fd.match_id
			 ORDER BY mi2.inning_number LIMIT 1),
			''
		)
		FROM fielding_data fd
		WHERE fd.match_id = $1
		  AND NOT EXISTS (SELECT 1 FROM batting_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)
		  AND NOT EXISTS (SELECT 1 FROM bowling_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)`)).
		WithArgs(int64(50)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "opposition_name"}))

	got, err := db.GetMatchPlayerTeams(ctx, 50)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "IND", got[1])
	require.Equal(t, "AUS", got[2])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMatchPlayerTeams_IncludesFieldOnly(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	db.SetDB(mockDB{pool: mock})

	// Batting/bowling: players 1 (IND), 2 (AUS)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM batting_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.batting_team_opposition_id
		WHERE bd.match_id = $1
		UNION
		SELECT bd.player_id, COALESCE(o.opposition_name, '')
		FROM bowling_data bd
		JOIN match_inning mi ON mi.match_id = bd.match_id AND mi.inning_number = bd.inning_number
		JOIN opposition o ON o.id = mi.bowling_team_opposition_id
		WHERE bd.match_id = $2`)).
		WithArgs(int64(50), int64(50)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "opposition_name"}).
			AddRow(int64(1), "IND").
			AddRow(int64(2), "AUS"))

	// Field-only: player 3 (IND) - included so predicted scorecard has full squad
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT fd.player_id, COALESCE(
			(SELECT o2.opposition_name FROM fielding_event fe
			 JOIN match_inning mi2 ON mi2.match_id = fe.match_id AND mi2.inning_number = fe.innings
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE fe.match_id = fd.match_id AND fe.fielder_id = fd.player_id
			 LIMIT 1),
			(SELECT o2.opposition_name FROM match_inning mi2
			 JOIN opposition o2 ON o2.id = mi2.bowling_team_opposition_id
			 WHERE mi2.match_id = fd.match_id
			 ORDER BY mi2.inning_number LIMIT 1),
			''
		)
		FROM fielding_data fd
		WHERE fd.match_id = $1
		  AND NOT EXISTS (SELECT 1 FROM batting_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)
		  AND NOT EXISTS (SELECT 1 FROM bowling_data bd WHERE bd.match_id = fd.match_id AND bd.player_id = fd.player_id)`)).
		WithArgs(int64(50)).
		WillReturnRows(pgxmock.NewRows([]string{"player_id", "opposition_name"}).
			AddRow(int64(3), "IND"))

	got, err := db.GetMatchPlayerTeams(ctx, 50)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "IND", got[1])
	require.Equal(t, "AUS", got[2])
	require.Equal(t, "IND", got[3])
	require.NoError(t, mock.ExpectationsWereMet())
}
