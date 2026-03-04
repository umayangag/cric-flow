package evaluate_test

import (
	"flag"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/evaluate"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantErr string
		want    svc.Options
	}{
		{name: "defaults", args: []string{}, want: svc.Options{Season: "demo", Format: "T20"}},
		{name: "overrides", args: []string{"-season", "2019", "-format", "ODI"}, want: svc.Options{Season: "2019", Format: "ODI"}},
		{name: "missing season", args: []string{"-season", "", "-format", "T20"}, wantErr: "season must not be empty"},
		{name: "missing format", args: []string{"-season", "2019", "-format", ""}, wantErr: "format must not be empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := svc.ParseArgs(fs, tc.args)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %+v got %+v", tc.want, got)
			}
		})
	}
}

func TestParseArgs_NilFlagSet(t *testing.T) {
	t.Parallel()
	got, err := svc.ParseArgs(nil, []string{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Season != "demo" || got.Format != "T20" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	t.Parallel()
	_, err := svc.ParseArgs(nil, []string{"-unknown"})
	if err == nil {
		t.Fatalf("expected parse error for unknown flag")
	}
}
