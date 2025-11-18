package weatherimport_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport/internal/mocks"
)

func TestService_Import_Table(t *testing.T) {
	t.Parallel()
	baseRecs := []models.WeatherData{
		{
			MatchID: 1,
			Session: "batting",
			Temp:    25,
		}, {
			MatchID: 1,
			Session: "bowling",
			Temp:    23,
		},
	}

	cases := []struct {
		name    string
		matchID int64
		apply   bool
		arrange func(ctx context.Context, provider *mocks.MockProvider, repository *mocks.MockRepository)
		assert  func(t *testing.T, n int, err error, repository *mocks.MockRepository)
	}{
		{
			name:    "dry-run returns count",
			matchID: 7,
			apply:   false,
			arrange: func(ctx context.Context, provider *mocks.MockProvider, _ *mocks.MockRepository) {
				provider.EXPECT().
					Fetch(ctx, int64(7)).
					Return(baseRecs, nil)
			},
			assert: func(t *testing.T, n int, err error, repository *mocks.MockRepository) {
				require.NoError(t, err)
				require.Equal(t, 2, n)
				repository.AssertNumberOfCalls(t, "UpsertWeather", 0)
			},
		},
		{
			name:    "apply upserts all",
			matchID: 7,
			apply:   true,
			arrange: func(ctx context.Context, provider *mocks.MockProvider, repository *mocks.MockRepository) {
				provider.EXPECT().
					Fetch(ctx, int64(7)).
					Return(baseRecs, nil)
				for _, r := range baseRecs {
					repository.EXPECT().
						UpsertWeather(ctx, r).
						Return(nil)
				}
			},
			assert: func(t *testing.T, n int, err error, repository *mocks.MockRepository) {
				require.NoError(t, err)
				require.Equal(t, 2, n)
				repository.AssertNumberOfCalls(t, "UpsertWeather", 2)
			},
		},
		{
			name:    "invalid match id",
			matchID: 0,
			apply:   true,
			arrange: func(_ context.Context, _ *mocks.MockProvider, _ *mocks.MockRepository) {
				// No expectations: service should short-circuit before calling provider.
			},
			assert: func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid match id")
			},
		},
		{
			name:    "Provider error",
			matchID: 5,
			apply:   true,
			arrange: func(ctx context.Context, provider *mocks.MockProvider, _ *mocks.MockRepository) {
				provider.EXPECT().
					Fetch(ctx, int64(5)).
					Return(baseRecs, errors.New("boom"))
			},
			assert: func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
				require.Error(t, err)
				require.ErrorContains(t, err, "boom")
			},
		},
		{
			name:    "repo error",
			matchID: 5,
			apply:   true,
			arrange: func(ctx context.Context, provider *mocks.MockProvider, repository *mocks.MockRepository) {
				provider.EXPECT().
					Fetch(ctx, int64(5)).
					Return(baseRecs, nil)
				repository.EXPECT().
					UpsertWeather(ctx, baseRecs[0]).
					Return(errors.New("disk"))
			},
			assert: func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
				require.Error(t, err)
				require.ErrorContains(t, err, "disk")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := mocks.NewMockProvider(t)
			repository := mocks.NewMockRepository(t)
			ctx := context.Background()

			// Arrange expectations for this case.
			tc.arrange(ctx, provider, repository)

			s := weatherimport.NewService(provider, repository)
			n, err := s.Import(ctx, tc.matchID, tc.apply)
			tc.assert(t, n, err, repository)
		})
	}
}
