package weatherimport_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport/internal/mocks"
)

type assertFn func(t *testing.T, n int, err error, fr *mocks.MockRepository)
type arrangeFn func(fp *mocks.MockProvider, fr *mocks.MockRepository)

func assertNoErrorCount(want int, wantUpserts int) assertFn {
	return func(t *testing.T, n int, err error, fr *mocks.MockRepository) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != want {
			t.Fatalf("want count=%d got %d", want, n)
		}
		fr.AssertNumberOfCalls(t, "UpsertWeather", wantUpserts)
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ int, err error, _ *mocks.MockRepository) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q got %v", sub, err)
		}
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func TestService_Import_Table(t *testing.T) {
	t.Parallel()
	baseRecs := []models.WeatherData{{MatchID: 1, Session: "batting", Temp: 25}, {MatchID: 1, Session: "bowling", Temp: 23}}

	cases := []struct {
		name        string
		providerErr error
		repoErr     error
		matchID     int64
		apply       bool
		arrange     arrangeFn
		assert      assertFn
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
			assert: assertNoErrorCount(2, 0),
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
			assert: assertNoErrorCount(2, 2),
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
			assert: assertErrContains("invalid match id"),
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
			assert: assertErrContains("boom"),
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
			assert: assertErrContains("disk"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := mocks.NewMockProvider(t)
			fr := mocks.NewMockRepository(t)

			// Arrange expectations for this case.
			tc.arrange(fp, fr)

			s := weatherimport.NewService(fp, fr)
			n, err := s.Import(context.Background(), tc.matchID, tc.apply)
			tc.assert(t, n, err, fr)
		})
	}
}
