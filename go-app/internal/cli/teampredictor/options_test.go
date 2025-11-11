package teampredictor_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
)

type assertFn func(t *testing.T, got cli.Options, err error)

type assertErrFn func(t *testing.T, err error)

func assertNoErrorOpts(want cli.Options) assertFn {
	return func(t *testing.T, got cli.Options, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if got.MatchID != want.MatchID { t.Fatalf("want MatchID=%d got %d", want.MatchID, got.MatchID) }
		if got.Format != want.Format { t.Fatalf("want Format=%q got %q", want.Format, got.Format) }
		if got.Season != want.Season { t.Fatalf("want Season=%q got %q", want.Season, got.Season) }
		if got.Bat != want.Bat { t.Fatalf("want Bat=%d got %d", want.Bat, got.Bat) }
		if got.Bowl != want.Bowl { t.Fatalf("want Bowl=%d got %d", want.Bowl, got.Bowl) }
	}
}

func assertErrorContains(sub string) assertFn {
	return func(t *testing.T, _ cli.Options, err error) {
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

func TestParseArgs_Basic(t *testing.T) {
	t.Parallel()
	cases := []struct{
		name string
		setup func()
		args []string
		assert assertFn
	}{
		{
			name: "happy path explicit flags",
			setup: func(){ os.Unsetenv("TEAM_PREDICTOR_MATCH"); os.Unsetenv("TEAM_PREDICTOR_FORMAT"); os.Unsetenv("TEAM_PREDICTOR_SEASON") },
			args: []string{"-match","1193505","-format","T20","-season","2019","-bat","6","-bowl","5"},
			assert: assertNoErrorOpts(cli.Options{MatchID:1193505,Format:"T20",Season:"2019",Bat:6,Bowl:5}),
		},
		{
			name: "env defaults used",
			setup: func(){ os.Setenv("TEAM_PREDICTOR_MATCH","1193505"); os.Setenv("TEAM_PREDICTOR_FORMAT","ODI"); os.Setenv("TEAM_PREDICTOR_SEASON","2011") },
			args: []string{"-bat","4","-bowl","6"},
			assert: assertNoErrorOpts(cli.Options{MatchID:1193505,Format:"ODI",Season:"2011",Bat:4,Bowl:6}),
		},
		{
			name: "invalid match",
			setup: func(){},
			args: []string{"-match","0","-format","T20","-season","2019"},
			assert: assertErrorContains("invalid match"),
		},
		{
			name: "invalid format",
			setup: func(){},
			args: []string{"-match","1","-format","X","-season","2019"},
			assert: assertErrorContains("invalid format"),
		},
		{
			name: "missing season",
			setup: func(){},
			args: []string{"-match","1","-format","T20"},
			assert: assertErrorContains("season"),
		},
		{
			name: "invalid bat",
			setup: func(){},
			args: []string{"-match","1","-format","T20","-season","2019","-bat","-1"},
			assert: assertErrorContains("invalid bat"),
		},
		{
			name: "invalid bowl",
			setup: func(){},
			args: []string{"-match","1","-format","T20","-season","2019","-bowl","-1"},
			assert: assertErrorContains("invalid bowl"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			os.Unsetenv("TEAM_PREDICTOR_MATCH"); os.Unsetenv("TEAM_PREDICTOR_FORMAT"); os.Unsetenv("TEAM_PREDICTOR_SEASON")
			if tc.setup != nil { tc.setup() }
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			tc.assert(t, got, err)
		})
	}
}
