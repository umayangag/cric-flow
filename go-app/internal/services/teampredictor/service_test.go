package teampredictor_test

import (
	"context"
	"errors"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

type fakeML struct {
	resp mlclient.PredictResponse
	err  error
	last mlclient.PredictRequest
}

func (f *fakeML) PredictTeam(_ context.Context, in mlclient.PredictRequest) (mlclient.PredictResponse, error) {
	f.last = in
	return f.resp, f.err
}
func (f *fakeML) Reload(_ context.Context) error { return nil }

type assertFn func(t *testing.T, out mlclient.PredictResponse, err error, f *fakeML)

func assertNoErrorPlayers(want []string) assertFn {
	return func(t *testing.T, out mlclient.PredictResponse, err error, f *fakeML) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if len(out.Players) != len(want) { t.Fatalf("want %d players got %d", len(want), len(out.Players)) }
		for i := range want {
			if out.Players[i] != want[i] { t.Fatalf("player %d: want %q got %q", i, want[i], out.Players[i]) }
		}
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ mlclient.PredictResponse, err error, _ *fakeML) {
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

func TestService_Predict_Table(t *testing.T) {
	t.Parallel()
	cases := []struct{
		name string
		opts cli.Options
		ml   *fakeML
		assert assertFn
	}{
		{
			name: "happy path",
			opts: cli.Options{MatchID: 1, Format: "T20", Season: "2019", Bat: 6, Bowl: 5},
			ml:   &fakeML{resp: mlclient.PredictResponse{Players: []string{"A","B","C"}}},
			assert: assertNoErrorPlayers([]string{"A","B","C"}),
		},
		{
			name: "ml error surfaces",
			opts: cli.Options{MatchID: 2, Format: "ODI", Season: "2011"},
			ml:   &fakeML{err: errors.New("ml down")},
			assert: assertErrContains("ml down"),
		},
		{
			name: "invalid options",
			opts: cli.Options{MatchID: 0, Format: "T20", Season: "2019"},
			ml:   &fakeML{},
			assert: assertErrContains("invalid options"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			s := svc.NewService(tc.ml)
			out, err := s.Predict(context.Background(), tc.opts)
			tc.assert(t, out, err, tc.ml)
		})
	}
}
