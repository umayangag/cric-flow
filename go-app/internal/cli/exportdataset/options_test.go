package exportdataset_test

import (
    "flag"
    "sort"
    "testing"

    cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
)

type assertFn func(t *testing.T, got cli.Options, err error)

func assertNoErrorFormats(want []string) assertFn {
    return func(t *testing.T, got cli.Options, err error) {
        if err != nil {
            t.Fatalf("unexpected err: %v", err)
        }
        if len(got.Formats) != len(want) {
            t.Fatalf("want %d formats, got %d (%v)", len(want), len(got.Formats), got.Formats)
        }
        // Compare as sets (order-insensitive) for robustness
        gotCopy := append([]string(nil), got.Formats...)
        wantCopy := append([]string(nil), want...)
        sort.Strings(gotCopy)
        sort.Strings(wantCopy)
        for i := range wantCopy {
            if wantCopy[i] != gotCopy[i] {
                t.Fatalf("want formats sorted[%d]=%q, got %q (got=%v)", i, wantCopy[i], gotCopy[i], got.Formats)
            }
        }
    }
}

func assertHasUnified(v bool) assertFn {
    return func(t *testing.T, got cli.Options, err error) {
        if err != nil {
            t.Fatalf("unexpected err: %v", err)
        }
        if got.Unified != v {
            t.Fatalf("want unified=%v got %v", v, got.Unified)
        }
    }
}

func TestParse_BasicFlags(t *testing.T) {
    t.Parallel()

    cases := []struct{
        name string
        args []string
        assert assertFn
    }{
        {
            name: "single format",
            args: []string{"-format", "ODI"},
            assert: assertNoErrorFormats([]string{"ODI"}),
        },
        {
            name: "multiple formats",
            args: []string{"-formats", "TEST,T20I"},
            assert: assertNoErrorFormats([]string{"TEST", "T20I"}),
        },
        {
            name: "all formats",
            args: []string{"-all-formats"},
            assert: assertNoErrorFormats([]string{"TEST", "ODI", "T20", "T20I"}),
        },
        {
            name: "unified flag",
            args: []string{"-unified"},
            assert: assertHasUnified(true),
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // Ensure a fresh FlagSet environment for each test
            fs := flag.NewFlagSet("test", flag.ContinueOnError)
            opts, err := cli.ParseArgs(fs, tc.args)
            tc.assert(t, opts, err)
        })
    }
}
