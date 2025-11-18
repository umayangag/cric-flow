package weatherimport_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport/internal/mocks"
)

func TestService_Import_Table(t *testing.T) {
	t.Parallel()
	baseRecs := []models.WeatherData{{MatchID: 1, Session: "batting", Temp: 25}, {MatchID: 1, Session: "bowling", Temp: 23}}

	cases := []struct {
		name        string
		providerErr error
		repoErr     error
		matchID     int64
		apply       bool
		arrange     func(fp *mocks.MockProvider, fr *mocks.MockRepository)
		assert      func(t *testing.T, n int, err error, fr *mocks.MockRepository)
	}{
		{
			name:        "dry-run returns count",
			providerErr: nil,
			repoErr:     nil,
			matchID:     7,
			apply:       false,
			arrange: func(fp *mocks.MockProvider, _ *mocks.MockRepository) {
				fp.EXPECT().Fetch(mock.Anything, int64(7)).Return(baseRecs, nil)
			},
			assert: func(t *testing.T, n int, err error, fr *mocks.MockRepository) {
				require.NoError(t, err)
				require.Equal(t, 2, n)
				fr.AssertNumberOfCalls(t, "UpsertWeather", 0)
			},
		},
		{
			name:        "apply upserts all",
			providerErr: nil,
			repoErr:     nil,
			matchID:     7,
			apply:       true,
			arrange: func(fp *mocks.MockProvider, fr *mocks.MockRepository) {
				fp.EXPECT().Fetch(mock.Anything, int64(7)).Return(baseRecs, nil)
				for _, r := range baseRecs {
					fr.EXPECT().UpsertWeather(mock.Anything, r).Return(nil)
				}
			},
			assert: func(t *testing.T, n int, err error, fr *mocks.MockRepository) {
				require.NoError(t, err)
				require.Equal(t, 2, n)
				fr.AssertNumberOfCalls(t, "UpsertWeather", 2)
			},
		},
		{
			name:        "invalid match id",
			providerErr: nil,
			repoErr:     nil,
			matchID:     0,
			apply:       true,
			arrange: func(_ *mocks.MockProvider, _ *mocks.MockRepository) {
				// No expectations: service should short-circuit before calling provider.
			},
			assert: func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid match id")
			},
		},
		{
			name:        "Provider error",
			providerErr: errors.New("boom"),
			repoErr:     nil,
			matchID:     5,
			apply:       true,
			arrange: func(fp *mocks.MockProvider, _ *mocks.MockRepository) {
				fp.EXPECT().Fetch(mock.Anything, int64(5)).Return(baseRecs, errors.New("boom"))
			},
			assert: func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
				require.Error(t, err)
				require.ErrorContains(t, err, "boom")
			},
		},
		{
			name:        "repo error",
			providerErr: nil,
			repoErr:     errors.New("disk"),
			matchID:     5,
			apply:       true,
			arrange: func(fp *mocks.MockProvider, fr *mocks.MockRepository) {
				fp.EXPECT().Fetch(mock.Anything, int64(5)).Return(baseRecs, nil)
				fr.EXPECT().UpsertWeather(mock.Anything, baseRecs[0]).Return(errors.New("disk"))
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

			// Arrange expectations for this case.
			tc.arrange(provider, repository)

			s := weatherimport.NewService(provider, repository)
			n, err := s.Import(context.Background(), tc.matchID, tc.apply)
			tc.assert(t, n, err, repository)
		})
	}
}
