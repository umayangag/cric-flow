package exportdataset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
)

type assertFn func(t *testing.T, err error)

func TestRunner_Run_MkdirAndValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arrange func() (*cmd.Runner, cli.Options)
		assert  assertFn
	}{
		{
			name: "errors on empty outdir",
			arrange: func() (*cmd.Runner, cli.Options) {
				r := cmd.NewRunner()
				return r, cli.Options{OutDir: ""}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "output directory")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts := tc.arrange()
			err := r.Run(context.Background(), opts)
			tc.assert(t, err)
		})
	}
}
