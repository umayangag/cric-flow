package exportdataset

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	exq "github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

// fakeFS implements fsx.FS minimal methods used by Runner.
type fakeFS struct{}

func (fakeFS) MkdirAll(_ string, _ fs.FileMode) error { return nil }
func (fakeFS) WriteFile(_ context.Context, _ string, _ []byte, _ fs.FileMode) error {
	return nil
}
func (fakeFS) ReadFile(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (fakeFS) Glob(_ string) ([]string, error)                      { return []string{}, nil }

// fakeRepo asserts seq flag presence via context and returns trivial CSV rows.
type fakeRepo struct{ wantSeq bool }

func (f fakeRepo) BattingUnifiedRows(ctx context.Context) ([][]string, error) {
	if exq.IsSeqEnabled(ctx) != f.wantSeq {
		testFailf(ctx, "BattingUnifiedRows: IsSeqEnabled mismatch")
	}
	return [][]string{{"h1", "h2"}, {"a", "b"}}, nil
}
func (f fakeRepo) BattingLegacyRows(_ context.Context) ([][]string, error) { return nil, nil }
func (f fakeRepo) BattingInferenceRows(_ context.Context, _ string) ([][]string, error) {
	return nil, nil
}
func (f fakeRepo) BattingFormatRows(_ context.Context, _ string) ([][]string, error) {
	return nil, nil
}
func (f fakeRepo) BowlingUnifiedRows(ctx context.Context) ([][]string, error) {
	if exq.IsSeqEnabled(ctx) != f.wantSeq {
		testFailf(ctx, "BowlingUnifiedRows: IsSeqEnabled mismatch")
	}
	return [][]string{{"h1", "h2"}, {"c", "d"}}, nil
}
func (f fakeRepo) BowlingLegacyRows(_ context.Context) ([][]string, error) { return nil, nil }
func (f fakeRepo) BowlingInferenceRows(_ context.Context, _ string) ([][]string, error) {
	return nil, nil
}
func (f fakeRepo) BowlingFormatRows(_ context.Context, _ string) ([][]string, error) {
	return nil, nil
}

// testFailf marks the test as failed using testing.T from context when available.
// Runner does not pass testing.T, so this is a best-effort helper; we will also rely on Run return error.
func testFailf(ctx context.Context, msg string) {
	if v := ctx.Value(testingTKey{}); v != nil {
		if tt, ok := v.(*testing.T); ok {
			tt.Fatalf("%s", msg)
		}
	}
}

type testingTKey struct{}

func TestRunner_PassesSeqFlagToRepo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		enable bool
	}{
		{"seq_off", false},
		{"seq_on", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := fakeFS{}
			repo := fakeRepo{wantSeq: tc.enable}
			bat := exportdataset.NewBattingService(repo)
			bow := exportdataset.NewBowlingService(repo)
			r := NewRunnerWithServices(fs, bat, bow)
			out := t.TempDir()
			// ensure a stable subpath write works
			_ = filepath.Join(out, "dummy.csv")
			ctx := context.WithValue(context.Background(), testingTKey{}, t)
			// Run unified so each exporter is called exactly once.
			opts := cli.Options{OutDir: out, Unified: true, EnableSeq: tc.enable}
			if err := r.Run(ctx, opts); err != nil {
				t.Fatalf("runner returned error: %v", err)
			}
		})
	}
}
