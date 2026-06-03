package teampredictor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

func TestParseFlags_BlankFormatOrSeasonErrors(t *testing.T) {
	cfg := &config.Config{}

	testCases := []struct {
		name string
		args []string
	}{
		{"blank format", []string{"-match=10", "-format= ", "-season=2025"}},
		{"blank season", []string{"-match=10", "-format=T20", "-season=   "}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFlags(tc.args, cfg)
			require.Error(t, err)
		})
	}
}

func TestParseFlags_SuccessAndDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.Team.DefaultBatters = 6
	cfg.Team.DefaultBowlers = 5
	cfg.Team.MinBowlers = 5

	testCases := []struct {
		name string
		args []string
		want options
	}{
		{
			name: "explicit bat bowl kept",
			args: []string{"-match=100", "-format=T20", "-season=2025", "-bat=7", "-bowl=6"},
			want: options{matchID: 100, formatCode: "T20", seasonName: "2025", batters: 7, bowlers: 6},
		},
		{
			name: "defaults applied when zero",
			args: []string{"-match=1", "-format=ODI", "-season=2019"},
			want: options{matchID: 1, formatCode: "ODI", seasonName: "2019", batters: 6, bowlers: 5},
		},
		{
			name: "min bowlers enforced",
			args: []string{"-match=2", "-format=T20", "-season=2024", "-bowl=3"},
			want: options{matchID: 2, formatCode: "T20", seasonName: "2024", batters: 6, bowlers: 5},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFlags(tc.args, cfg)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseFlags_Errors(t *testing.T) {
	cfg := &config.Config{}

	testCases := []struct {
		name string
		args []string
	}{
		{"no args", []string{}},
		{"zero match", []string{"-match=0", "-format=T20", "-season=2025"}},
		{"empty format", []string{"-match=10", "-format=", "-season=2025"}},
		{"empty season", []string{"-match=10", "-format=T20", "-season="}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFlags(tc.args, cfg)
			require.Error(t, err)
		})
	}
}
