package weatherimport_test

import (
	"context"
	"errors"
	"testing"

	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherimport"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/wx"
)

type fakeProvider struct{ recs []wx.Record; err error }

func (p *fakeProvider) Fetch(_ context.Context, _ int64) ([]wx.Record, error) {
	if p.err != nil { return nil, p.err }
	out := make([]wx.Record, len(p.recs))
	copy(out, p.recs)
	return out, nil
}

type fakeRepo struct{ upserts []wx.Record; err error }

func (r *fakeRepo) UpsertWeather(_ context.Context, rec wx.Record) error {
	if r.err != nil { return r.err }
	r.upserts = append(r.upserts, rec)
	return nil
}

type assertFn func(t *testing.T, n int, err error, fr *fakeRepo)

func assertNoErrorCount(want int, wantUpserts int) assertFn {
	return func(t *testing.T, n int, err error, fr *fakeRepo) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if n != want { t.Fatalf("want count=%d got %d", want, n) }
		if len(fr.upserts) != wantUpserts { t.Fatalf("want upserts=%d got %d", wantUpserts, len(fr.upserts)) }
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ int, err error, _ *fakeRepo) {
		s := ""; if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 { t.Fatalf("want err containing %q got %v", sub, err) }
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ { if s[i+j] != sub[j] { ok = false; break } }
		if ok { return i }
	}
	return -1
}

func TestService_Import_Table(t *testing.T) {
	t.Parallel()
 baseRecs := []wx.Record{{MatchID: 1, Session: "batting", Temp: 25}, {MatchID: 1, Session: "bowling", Temp: 23}}

	cases := []struct{
		name string
		providerErr error
		repoErr error
		matchID int64
		apply bool
		assert assertFn
	}{
		{"dry-run returns count", nil, nil, 7, false, assertNoErrorCount(2, 0)},
		{"apply upserts all", nil, nil, 7, true, assertNoErrorCount(2, 2)},
		{"invalid match id", nil, nil, 0, true, assertErrContains("invalid match id")},
		{"provider error", errors.New("boom"), nil, 5, true, assertErrContains("boom")},
		{"repo error", nil, errors.New("disk"), 5, true, assertErrContains("disk")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakeProvider{recs: baseRecs, err: tc.providerErr}
			fr := &fakeRepo{err: tc.repoErr}
			s := svc.NewService(fp, fr)
			n, err := s.Import(context.Background(), tc.matchID, tc.apply)
			tc.assert(t, n, err, fr)
		})
	}
}
