package evaluate_test

import (
	"testing"

	cli "github.com/umayangag/cric-flow/go-app/internal/cli/evaluate"
)

func TestParseArgs_WithNilFlagSet_UsesDefaults(t *testing.T) {
	t.Parallel()
	got, err := cli.ParseArgs(nil, []string{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Season != "demo" {
		t.Fatalf("want default season demo got %q", got.Season)
	}
	if got.Format != "T20" {
		t.Fatalf("want default format T20 got %q", got.Format)
	}
}

func TestParseArgs_ParseError_UnknownFlag(t *testing.T) {
	t.Parallel()
	_, err := cli.ParseArgs(nil, []string{"-unknown"})
	if err == nil {
		t.Fatalf("expected parse error for unknown flag")
	}
}
