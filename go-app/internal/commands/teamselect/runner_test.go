package teamselect_test

import (
	"context"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teamselect"
	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

type assertFn func(t *testing.T, team []ts.Player, err error)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ []ts.Player, err error) {
		s := ""; if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 { t.Fatalf("want err containing %q got %v", sub, err) }
	}
}

func assertNoErrorSize(n int) assertFn {
	return func(t *testing.T, team []ts.Player, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if len(team) != n { t.Fatalf("want size=%d got %d", n, len(team)) }
	}
}

func indexOf(s, sub string) int { for i:=0; i+len(sub)<=len(s); i++ { ok:=true; for j:=0; j<len(sub); j++ { if s[i+j]!=sub[j] { ok=false; break } }; if ok { return i } }; return -1 }

func TestRunner_Run_Table(t *testing.T) {
	t.Parallel()
	mk := func(name string, bat, bowl float64, isBow, isKeep bool) ts.Player {
		return ts.Player{Name: name, BatScore: bat, BowlScore: bowl, IsBowler: isBow, IsKeeper: isKeep}
	}
	pool := []ts.Player{
		mk("A", 0.9, 0.1, false, false),
		mk("B", 0.7, 0.8, true, false),
		mk("C", 0.6, 0.7, true, false),
		mk("D", 0.5, 0.2, false, false),
		mk("K", 0.4, 0.3, false, true),
	}
	r := cmd.NewRunner()
	cases := []struct{
		name string
		opts cli.Options
		pool []ts.Player
		assert assertFn
	}{
		{"nil runner", cli.Options{MatchID:1,Format:"T20",Season:"2019",Size:3}, nil, func(t *testing.T, _ []ts.Player, _ error){
			var nr *cmd.Runner
			_, err := nr.Run(context.Background(), cli.Options{}, nil)
			assertErrContains("nil runner")(t, nil, err)
		}},
		{"invalid opts", cli.Options{MatchID:0,Format:"T20",Season:"2019",Size:3}, pool, assertErrContains("invalid options")},
		{"insufficient pool", cli.Options{MatchID:1,Format:"T20",Season:"2019",Size:10}, pool, assertErrContains("insufficient pool")},
		{"happy path", cli.Options{MatchID:1,Format:"T20",Season:"2019",Size:3,MinBowlers:1,RequireKeeper:true}, pool, assertNoErrorSize(3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			team, err := r.Run(context.Background(), tc.opts, tc.pool)
			tc.assert(t, team, err)
		})
	}
}
