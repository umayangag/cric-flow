package opsstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// test helper: write a file with N lines and fixed modtime
func writeFileWithLines(t *testing.T, dir, name string, lines int) {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	for i := 0; i < lines; i++ {
		if _, err := f.WriteString("row\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mt := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestBuildExportsSection_Table(t *testing.T) {
	type assertion func(t *testing.T, sec map[string]any)

	newTmp := func(t *testing.T) string { return t.TempDir() }

	assertEmptyFormats := func(root string) assertion {
		return func(t *testing.T, sec map[string]any) {
			if sec["root"] != root {
				t.Fatalf("root mismatch: got %v", sec["root"])
			}
			fm, ok := sec["formats"].(map[string]any)
			if !ok {
				t.Fatalf("formats wrong type: %T", sec["formats"])
			}
			for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
				vf, ok := fm[f].(map[string]any)
				if !ok {
					t.Fatalf("missing format %s", f)
				}
				if arr, ok := vf["files"].([]map[string]any); ok {
					if len(arr) != 0 {
						t.Fatalf("files not empty for %s", f)
					}
				} else if arrAny, ok := vf["files"].([]any); ok {
					if len(arrAny) != 0 {
						t.Fatalf("files not empty(any) for %s", f)
					}
				} else {
					t.Fatalf("files wrong type: %T", vf["files"])
				}
			}
		}
	}

	assertUnifiedApplied := func() assertion {
		return func(t *testing.T, sec map[string]any) {
			fm := sec["formats"].(map[string]any)
			for _, f := range []string{"TEST", "ODI", "T20I", "T20"} {
				files := fm[f].(map[string]any)["files"].([]map[string]any)
				var seenBat, seenBowl bool
				for _, e := range files {
					switch e["name"] {
					case "batting_on.csv":
						seenBat = e["exists"].(bool)
						// rows and modified are set
						if rows, ok := e["rows"].(int); ok && rows == 0 {
							t.Fatalf("batting rows should be >0")
						}
						if _, ok := e["modified"].(string); !ok {
							t.Fatalf("modified missing")
						}
					case "bowling_on.csv":
						seenBowl = e["exists"].(bool)
					}
				}
				if !seenBat || !seenBowl {
					t.Fatalf("unified not applied for %s", f)
				}
			}
		}
	}

	assertPerFormat := func() assertion {
		return func(t *testing.T, sec map[string]any) {
			fm := sec["formats"].(map[string]any)
			// ODI has batting file
			var hasODI bool
			for _, e := range fm["ODI"].(map[string]any)["files"].([]map[string]any) {
				if e["name"] == "my_batting_ODI_2020.csv" && e["exists"].(bool) {
					hasODI = true
				}
			}
			if !hasODI {
				t.Fatalf("ODI batting not detected")
			}
			// T20I has bowling file
			var hasT20I bool
			for _, e := range fm["T20I"].(map[string]any)["files"].([]map[string]any) {
				if e["name"] == "stats_bowling_T20I.csv" && e["exists"].(bool) {
					hasT20I = true
				}
			}
			if !hasT20I {
				t.Fatalf("T20I bowling not detected")
			}
		}
	}

	tests := []struct {
		name   string
		setup  func(t *testing.T) string // returns root
		assert assertion
	}{
		{
			name:  "missing_dir_returns_empty_structure",
			setup: func(t *testing.T) string { return filepath.Join(newTmp(t), "does-not-exist") },
			assert: func(t *testing.T, sec map[string]any) {
				// pass root for root equality check
				root := sec["root"].(string)
				assertEmptyFormats(root)(t, sec)
			},
		},
		{
			name: "unified_files_applied_to_all_formats",
			setup: func(t *testing.T) string {
				root := newTmp(t)
				writeFileWithLines(t, root, "batting_on.csv", 3)
				writeFileWithLines(t, root, "bowling_on.csv", 2)
				return root
			},
			assert: assertUnifiedApplied(),
		},
		{
			name: "per_format_files_detected",
			setup: func(t *testing.T) string {
				root := newTmp(t)
				writeFileWithLines(t, root, "my_batting_ODI_2020.csv", 4)
				writeFileWithLines(t, root, "stats_bowling_T20I.csv", 5)
				return root
			},
			assert: assertPerFormat(),
		},
		{
			name: "T20_and_T20I_do_not_cross_match",
			setup: func(t *testing.T) string {
				root := newTmp(t)
				writeFileWithLines(t, root, "batting_encoded_T20.csv", 100)
				writeFileWithLines(t, root, "bowling_encoded_T20.csv", 200)
				writeFileWithLines(t, root, "batting_encoded_T20I.csv", 300)
				writeFileWithLines(t, root, "bowling_encoded_T20I.csv", 400)
				return root
			},
			assert: func(t *testing.T, sec map[string]any) {
				fm := sec["formats"].(map[string]any)
				// T20 must have only T20 files (row sum 100+200=300)
				t20Files := fm["T20"].(map[string]any)["files"].([]map[string]any)
				if len(t20Files) != 2 {
					t.Fatalf("T20 should have 2 files, got %d", len(t20Files))
				}
				var t20Rows int
				for _, e := range t20Files {
					if r, ok := e["rows"].(int); ok {
						t20Rows += r
					}
				}
				if t20Rows != 300 {
					t.Fatalf("T20 row sum should be 300, got %d", t20Rows)
				}
				// T20I must have only T20I files (row sum 300+400=700)
				t20iFiles := fm["T20I"].(map[string]any)["files"].([]map[string]any)
				if len(t20iFiles) != 2 {
					t.Fatalf("T20I should have 2 files, got %d", len(t20iFiles))
				}
				var t20iRows int
				for _, e := range t20iFiles {
					if r, ok := e["rows"].(int); ok {
						t20iRows += r
					}
				}
				if t20iRows != 700 {
					t.Fatalf("T20I row sum should be 700, got %d", t20iRows)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.setup(t)
			sec := BuildExportsSection(root)
			// debug dump on failure convenience
			_, _ = json.Marshal(sec)
			tc.assert(t, sec)
		})
	}
}
