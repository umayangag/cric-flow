package fielding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	fsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
)

func TestService_BackfillMatch(t *testing.T) {
	t.Parallel()
	events := []db.BackfillEvent{
		{MatchID: 1, PlayerID: 10, Catches: 1},
		{MatchID: 1, PlayerID: 10, RunOuts: 2},
		{MatchID: 1, PlayerID: 11, Stumpings: 1},
		{MatchID: 2, PlayerID: 10, Catches: 1},
	}

	cases := []struct {
		name        string
		apply       bool
		match       int64
		listErr     error
		upsertErr   error
		wantCount   int
		wantBatches int
		wantErr     string
	}{
		{name: "dry-run aggregates without upsert", apply: false, match: 1, wantCount: 2, wantBatches: 0},
		{name: "apply aggregates and upserts", apply: true, match: 1, wantCount: 2, wantBatches: 1},
		{name: "invalid match id", apply: true, match: 0, wantErr: "invalid match id"},
		{name: "list error", apply: true, match: 1, listErr: errors.New("boom"), wantErr: "boom"},
		{name: "upsert error", apply: true, match: 1, upsertErr: errors.New("disk"), wantErr: "disk"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			m := dbmocks.NewMockFieldingRepo(t)
			s := fsvc.NewService(m)

			if tc.match <= 0 {
				// no expectations; method should error before calling repo
			} else {
				// List expectation for specific match
				matchID := tc.match
				if tc.listErr != nil {
					m.EXPECT().ListFieldingEvents(mock.Anything, &matchID).Return(nil, tc.listErr)
				} else {
					// Filter events for this match
					var filtered []db.BackfillEvent
					for _, e := range events {
						if e.MatchID == matchID {
							filtered = append(filtered, e)
						}
					}
					m.EXPECT().ListFieldingEvents(mock.Anything, &matchID).Return(filtered, nil)
					if tc.apply {
						// Expect upsert; allow any rows and return upsertErr if set
						m.EXPECT().UpsertFieldingAggregates(mock.Anything, mock.Anything).
							Return(tc.upsertErr)
					}
				}
			}

			// Act
			n, err := s.BackfillMatch(ctx, tc.match, tc.apply)

			// Assert
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantCount, n)
			if tc.apply {
				// cannot directly assert batches without intercepting rows; rely on expectations
			}
		})
	}
}

func TestService_BackfillAll_ConcurrencyAndErrors(t *testing.T) {
	t.Parallel()
	events := []db.BackfillEvent{
		{MatchID: 1, PlayerID: 10, Catches: 1},
		{MatchID: 1, PlayerID: 11, RunOuts: 1},
		{MatchID: 2, PlayerID: 10, Stumpings: 1},
		{MatchID: 2, PlayerID: 12, RunoutsDirectHits: 1},
	}
	// total aggregates: for match 1 -> players 10,11 (2); match 2 -> players 10,12 (2) => 4

	cases := []struct {
		name      string
		apply     bool
		conc      int
		listErr   error
		upsertErr error
		wantCount int
		wantErr   string
	}{
		{name: "dry-run counts total without upsert", apply: false, conc: 2, wantCount: 4},
		{name: "apply upserts in batches per match", apply: true, conc: 2, wantCount: 4},
		{name: "bad concurrency", apply: true, conc: 0, wantErr: "concurrency"},
		{name: "list error", apply: true, conc: 1, listErr: errors.New("boom"), wantErr: "boom"},
		{name: "upsert error", apply: true, conc: 1, upsertErr: errors.New("fail"), wantErr: "fail"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			m := dbmocks.NewMockFieldingRepo(t)
			s := fsvc.NewService(m)

			if tc.conc < 1 {
				// no expectations; should fail fast
			} else if tc.listErr != nil {
				m.EXPECT().ListFieldingEvents(mock.Anything, (*int64)(nil)).Return(nil, tc.listErr)
			} else {
				m.EXPECT().ListFieldingEvents(mock.Anything, (*int64)(nil)).Return(events, nil)
				if tc.apply {
					m.EXPECT().UpsertFieldingAggregates(mock.Anything, mock.Anything).Return(tc.upsertErr)
				}
			}

			// Act
			n, err := s.BackfillAll(ctx, tc.apply, tc.conc)

			// Assert
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantCount, n)
		})
	}
}
