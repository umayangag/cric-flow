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
		assert      assertFn
	}{
		{"dry-run returns count", nil, nil, 7, false, assertNoErrorCount(2, 0)},
		{"apply upserts all", nil, nil, 7, true, assertNoErrorCount(2, 2)},
		{"invalid match id", nil, nil, 0, true, assertErrContains("invalid match id")},
		{"Provider error", errors.New("boom"), nil, 5, true, assertErrContains("boom")},
		{"repo error", nil, errors.New("disk"), 5, true, assertErrContains("disk")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := mocks.NewMockProvider(t)
			fr := mocks.NewMockRepository(t)

			// Set provider expectation when matchID is valid; otherwise, service should short-circuit.
			if tc.matchID > 0 {
				fp.EXPECT().Fetch(mock.Anything, tc.matchID).
					Return(baseRecs, tc.providerErr)
			}

			// Set repository expectations only when apply=true and provider has no error.
			if tc.apply && tc.providerErr == nil && tc.matchID > 0 {
				if tc.repoErr != nil {
					// Expect first upsert to error; service should stop after first failure.
					fr.EXPECT().UpsertWeather(mock.Anything, baseRecs[0]).Return(tc.repoErr)
				} else {
					// Expect upsert for each record.
					for _, r := range baseRecs {
						fr.EXPECT().UpsertWeather(mock.Anything, r).Return(nil)
					}
				}
			}

			s := weatherimport.NewService(fp, fr)
			n, err := s.Import(context.Background(), tc.matchID, tc.apply)
			tc.assert(t, n, err, fr)
		})
	}
}
