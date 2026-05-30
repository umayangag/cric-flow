package evaluate_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/evaluate"

	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		wantErr string
		want    svc.Options
	}{
		{name: "defaults", args: []string{}, want: svc.Options{Season: "demo", Format: "T20"}},
		{
			name: "overrides",
			args: []string{"-season", "2019", "-format", "ODI"},
			want: svc.Options{Season: "2019", Format: "ODI"},
		},
		{name: "missing season", args: []string{"-season", "", "-format", "T20"}, wantErr: "season must not be empty"},
		{name: "missing format", args: []string{"-season", "2019", "-format", ""}, wantErr: "format must not be empty"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			if tc.wantErr != "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseArgs_NilFlagSet(t *testing.T) {
	t.Parallel()
	got, err := svc.ParseArgs(nil, []string{})
	require.NoError(t, err)
	require.Equal(t, "demo", got.Season)
	require.Equal(t, "T20", got.Format)
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	t.Parallel()
	_, err := svc.ParseArgs(nil, []string{"-unknown"})
	require.Error(t, err)
}
