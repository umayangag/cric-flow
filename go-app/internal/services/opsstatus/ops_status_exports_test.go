package opsstatus

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeFileWithLines writes a file with N lines and a fixed modtime.
func writeFileWithLines(t *testing.T, dir, name string, lines int) {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()
	for i := 0; i < lines; i++ {
		_, err := f.WriteString("row\n")
		require.NoError(t, err)
	}
	mt := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(p, mt, mt))
}

func TestBuildExportsSection_Table(t *testing.T) {
	testCases := []struct {
		name   string
		setup  func(t *testing.T) string
		assert func(t *testing.T, sec map[string]any)
	}{
		{
			name:  "missing dir returns empty structure",
			setup: func(t *testing.T) string { return filepath.Join(t.TempDir(), "does-not-exist") },
			assert: func(t *testing.T, sec map[string]any) {
				t.Helper()
				root := sec["root"].(string)
				require.NotEmpty(t, root)
				fm, ok := sec["formats"].(map[string]any)
				require.True(t, ok)
				for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
					vf, ok := fm[f].(map[string]any)
					require.True(t, ok, "format %s missing", f)
					switch files := vf["files"].(type) {
					case []map[string]any:
						require.Empty(t, files, "format %s should have no files", f)
					case []any:
						require.Empty(t, files, "format %s should have no files", f)
					default:
						require.Fail(t, "files wrong type", "%T for format %s", vf["files"], f)
					}
				}
			},
		},
		{
			name: "unified files applied to all formats",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				writeFileWithLines(t, root, "batting_on.csv", 3)
				writeFileWithLines(t, root, "bowling_on.csv", 2)
				return root
			},
			assert: func(t *testing.T, sec map[string]any) {
				t.Helper()
				fm := sec["formats"].(map[string]any)
				for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
					files, ok := fm[f].(map[string]any)["files"].([]map[string]any)
					require.True(t, ok, "files not []map[string]any for format %s", f)
					var seenBat, seenBowl bool
					for _, e := range files {
						switch e["name"] {
						case "batting_on.csv":
							seenBat = e["exists"].(bool)
							if rows, ok := e["rows"].(int); ok {
								require.Positive(t, rows, "batting rows should be >0 for format %s", f)
							}
							_, ok := e["modified"].(string)
							require.True(t, ok, "modified missing for format %s", f)
						case "bowling_on.csv":
							seenBowl = e["exists"].(bool)
						}
					}
					require.True(t, seenBat, "batting_on.csv not found for format %s", f)
					require.True(t, seenBowl, "bowling_on.csv not found for format %s", f)
				}
			},
		},
		{
			name: "per format files detected",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				writeFileWithLines(t, root, "my_batting_ODI_2020.csv", 4)
				writeFileWithLines(t, root, "stats_bowling_T20I.csv", 5)
				return root
			},
			assert: func(t *testing.T, sec map[string]any) {
				t.Helper()
				fm := sec["formats"].(map[string]any)
				var hasODI bool
				for _, e := range fm["ODI"].(map[string]any)["files"].([]map[string]any) {
					if e["name"] == "my_batting_ODI_2020.csv" && e["exists"].(bool) {
						hasODI = true
					}
				}
				require.True(t, hasODI, "ODI batting file not detected")
				var hasT20I bool
				for _, e := range fm["T20I"].(map[string]any)["files"].([]map[string]any) {
					if e["name"] == "stats_bowling_T20I.csv" && e["exists"].(bool) {
						hasT20I = true
					}
				}
				require.True(t, hasT20I, "T20I bowling file not detected")
			},
		},
		{
			name: "T20 and T20I do not cross match",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				writeFileWithLines(t, root, "batting_encoded_T20.csv", 100)
				writeFileWithLines(t, root, "bowling_encoded_T20.csv", 200)
				writeFileWithLines(t, root, "batting_encoded_T20I.csv", 300)
				writeFileWithLines(t, root, "bowling_encoded_T20I.csv", 400)
				return root
			},
			assert: func(t *testing.T, sec map[string]any) {
				t.Helper()
				fm := sec["formats"].(map[string]any)
				t20Files := fm["T20"].(map[string]any)["files"].([]map[string]any)
				require.Len(t, t20Files, 2, "T20 should have 2 files")
				var t20Rows int
				for _, e := range t20Files {
					if r, ok := e["rows"].(int); ok {
						t20Rows += r
					}
				}
				require.Equal(t, 300, t20Rows)

				t20iFiles := fm["T20I"].(map[string]any)["files"].([]map[string]any)
				require.Len(t, t20iFiles, 2, "T20I should have 2 files")
				var t20iRows int
				for _, e := range t20iFiles {
					if r, ok := e["rows"].(int); ok {
						t20iRows += r
					}
				}
				require.Equal(t, 700, t20iRows)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			root := tc.setup(t)
			sec := BuildExportsSection(root)
			tc.assert(t, sec)
		})
	}
}
