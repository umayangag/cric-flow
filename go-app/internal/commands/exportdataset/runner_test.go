package exportdataset_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	cli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	cmd "github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/commands/exportdataset/mocks"
)

type assertFn func(t *testing.T, err error)

func TestRunner_Run_MkdirAndValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arrange func(t *testing.T) (*cmd.Runner, cli.Options)
		assert  assertFn
	}{
		{
			name: "nil_runner_returns_error",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				return nil, cli.Options{OutDir: t.TempDir()}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "nil runner")
			},
		},
		{
			name: "errors_on_empty_outdir",
			arrange: func(_ *testing.T) (*cmd.Runner, cli.Options) {
				return cmd.NewRunner(), cli.Options{OutDir: ""}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "output directory")
			},
		},
		{
			name: "writable_dir_no_services_succeeds",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				return cmd.NewRunner(), cli.Options{OutDir: t.TempDir()}
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "unified_with_mocks_succeeds",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				r := cmd.NewRunnerWithServices(bat, bow, nil, nil, nil)
				return r, cli.Options{OutDir: t.TempDir(), Unified: true}
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "unified_bat_export_error_returns_error",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(errors.New("bat export failed"))
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				r := cmd.NewRunnerWithServices(bat, bow, nil, nil, nil)
				return r, cli.Options{OutDir: t.TempDir(), Unified: true}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "bat export failed")
			},
		},
		{
			name: "write_fails_when_outdir_readonly",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				root := t.TempDir()
				readOnly := filepath.Join(root, "readonly")
				require.NoError(t, os.MkdirAll(readOnly, 0o755))
				require.NoError(t, os.Chmod(readOnly, 0o555))
				t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				r := cmd.NewRunnerWithServices(bat, bow, nil, nil, nil)
				return r, cli.Options{OutDir: readOnly, Unified: true}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "unified_with_extras_win_mocks_succeeds",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				extras := mocks.NewMockExtrasExporter(t)
				win := mocks.NewMockWinExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				extras.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				win.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bat.EXPECT().ExportFormat(mock.Anything, "TEST", mock.Anything).Return(nil)
				bow.EXPECT().ExportFormat(mock.Anything, "TEST", mock.Anything).Return(nil)
				extras.EXPECT().ExportFormat(mock.Anything, "TEST", mock.Anything).Return(nil)
				win.EXPECT().ExportFormat(mock.Anything, "TEST", mock.Anything).Return(nil)
				r := cmd.NewRunnerWithServices(bat, bow, nil, extras, win)
				return r, cli.Options{OutDir: t.TempDir(), Unified: true, Formats: []string{"TEST"}}
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "unified_extras_export_error_returns_error",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				extras := mocks.NewMockExtrasExporter(t)
				win := mocks.NewMockWinExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				extras.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(errors.New("extras export failed"))
				win.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				r := cmd.NewRunnerWithServices(bat, bow, nil, extras, win)
				return r, cli.Options{OutDir: t.TempDir(), Unified: true}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "extras export failed")
			},
		},
		{
			name: "unified_win_performat_error_returns_error",
			arrange: func(t *testing.T) (*cmd.Runner, cli.Options) {
				bat := mocks.NewMockBattingExporter(t)
				bow := mocks.NewMockBowlingExporter(t)
				extras := mocks.NewMockExtrasExporter(t)
				win := mocks.NewMockWinExporter(t)
				bat.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bow.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				extras.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				win.EXPECT().ExportUnified(mock.Anything, mock.Anything).Return(nil)
				bat.EXPECT().ExportFormat(mock.Anything, "ODI", mock.Anything).Return(nil)
				bow.EXPECT().ExportFormat(mock.Anything, "ODI", mock.Anything).Return(nil)
				extras.EXPECT().ExportFormat(mock.Anything, "ODI", mock.Anything).Return(nil)
				win.EXPECT().ExportFormat(mock.Anything, "ODI", mock.Anything).Return(errors.New("win format export failed"))
				r := cmd.NewRunnerWithServices(bat, bow, nil, extras, win)
				return r, cli.Options{OutDir: t.TempDir(), Unified: true, Formats: []string{"ODI"}}
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "win format export failed")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts := tc.arrange(t)
			err := r.Run(context.Background(), opts)
			tc.assert(t, err)
		})
	}
}
