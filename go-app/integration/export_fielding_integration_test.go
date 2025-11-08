package integration

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// TestExportFieldingEndToEnd performs an end-to-end ingest of a tiny Cricsheet fixture
// and validates fielding_event rows, aggregated fielding_data, and exporter headers/values.
func TestExportFieldingEndToEnd(t *testing.T) {
	ctx := context.Background()
	if _, err := db.Connect(ctx); err != nil {
		t.Skipf("skipping: cannot connect to DB: %v", err)
	}
	// Run migrations
	migrationsDir := filepath.Join("..", "migrations")
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Ingest the sample fixture directory
	fixturesDir := filepath.Join("..", "..", "tests", "fixtures", "cricsheet")
	if _, err := os.Stat(fixturesDir); err != nil {
		t.Fatalf("fixture dir missing: %s: %v", fixturesDir, err)
	}
	if _, err := cricsheet.ImportDir(ctx, fixturesDir, &cricsheet.Options{}); err != nil {
		t.Fatalf("import fixture failed: %v", err)
	}

	// Compute expected match_id using same helper as ingest
	mid := cricsheet.StableMatchID("2020-01-01", "Alpha", "Beta")

	// Validate fielding_event counts by kind
	row := db.Pool.QueryRow(ctx, `SELECT 
		COALESCE(SUM(CASE WHEN kind='caught' THEN 1 ELSE 0 END),0) AS c,
		COALESCE(SUM(CASE WHEN kind='run_out' THEN 1 ELSE 0 END),0) AS r,
		COALESCE(SUM(CASE WHEN kind='stumped' THEN 1 ELSE 0 END),0) AS s
		FROM fielding_event WHERE match_id=$1`, mid)
	var caught, runOut, stumped int
	if err := row.Scan(&caught, &runOut, &stumped); err != nil {
		t.Fatalf("scan fielding_event counts: %v", err)
	}
	if caught != 1 { t.Fatalf("expected 1 caught, got %d", caught) }
	if runOut != 2 { t.Fatalf("expected 2 run_out events (two fielders credited), got %d", runOut) }
	if stumped != 1 { t.Fatalf("expected 1 stumped, got %d", stumped) }

	// Recompute aggregates explicitly (ingest already calls it, but repeat to be safe)
	if err := db.RecomputeFieldingAggregates(ctx, mid); err != nil {
		t.Fatalf("recompute aggregates: %v", err)
	}

	rows, err := db.Pool.Query(ctx, `SELECT COALESCE(SUM(catches),0), COALESCE(SUM(run_outs),0), COALESCE(SUM(stumpings),0), COALESCE(SUM(runouts_direct_hits),0) FROM fielding_data WHERE match_id=$1`, mid)
	if err != nil { t.Fatalf("query fielding_data sums: %v", err) }
	defer rows.Close()
	if !rows.Next() { t.Fatalf("no aggregate row returned") }
	var sumC, sumR, sumS, sumDH int
	if err := rows.Scan(&sumC, &sumR, &sumS, &sumDH); err != nil { t.Fatalf("scan aggregates: %v", err) }
	if sumC != 1 { t.Fatalf("expected total catches=1, got %d", sumC) }
	if sumR != 2 { t.Fatalf("expected total run_outs=2, got %d", sumR) }
	if sumS != 1 { t.Fatalf("expected total stumpings=1, got %d", sumS) }
	if sumDH != 0 { t.Fatalf("expected total runouts_direct_hits=0 by default, got %d", sumDH) }

	// Invoke unified exporter to generate CSVs
	outDir := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/export-dataset", "--unified", "--out", outDir)
	cmd.Env = append(os.Environ(), "GO_APP_OUTPUT_DIR="+outDir)
	cmd.Dir = filepath.Join("..") // go-app directory root
	if err := runWithTimeout(cmd, 60*time.Second); err != nil {
		t.Fatalf("export unified failed: %v", err)
	}

	batCSV := filepath.Join(outDir, "batting_encoded_all.csv")
	if _, err := os.Stat(batCSV); err != nil {
		t.Fatalf("expected unified batting csv not found: %v", err)
	}
	f, err := os.Open(batCSV)
	if err != nil { t.Fatalf("open csv: %v", err) }
	defer f.Close()
	cr := csv.NewReader(f)
	rec, err := cr.Read()
	if err != nil { t.Fatalf("read header: %v", err) }
	// Verify header contains fielding columns in order
	want := []string{"catches", "run_outs", "stumpings", "runouts_direct_hits", "fielding_involvements"}
	idxs := make([]int, len(want))
	for i, col := range want {
		idxs[i] = indexOf(rec, col)
		if idxs[i] < 0 {
			t.Fatalf("missing column %q in header: %v", col, rec)
		}
		// ensure order
		if i > 0 && !(idxs[i] > idxs[i-1]) {
			t.Fatalf("columns out of order for %q, got indexes %v in header %v", col, idxs, rec)
		}
	}
	// Locate player_name column
	pidx := indexOf(rec, "player_name")
	if pidx < 0 { t.Fatalf("missing player_name in header") }

	// Read rows and find any with non-zero fielding_involvements; cross-check against DB for that player
	// Collect DB expected counts per player_name for this match
	dbRows, err := db.Pool.Query(ctx, `SELECT p.player_name, fd.catches, fd.run_outs, fd.stumpings, fd.runouts_direct_hits FROM fielding_data fd JOIN player p ON p.id = fd.player_id WHERE fd.match_id=$1`, mid)
	if err != nil { t.Fatalf("query per-player aggregates: %v", err) }
	expects := map[string][4]int{}
	for dbRows.Next() {
		var name string
		var c, r, s, dh int
		if err := dbRows.Scan(&name, &c, &r, &s, &dh); err != nil { t.Fatalf("scan per-player: %v", err) }
		expects[name] = [4]int{c, r, s, dh}
	}
	dbRows.Close()

	matched := 0
	for {
		rec, err = cr.Read()
		if err != nil { break }
		name := rec[pidx]
		exp, ok := expects[name]
		if !ok { continue }
		c := mustAtoi(rec[idxs[0]])
		r := mustAtoi(rec[idxs[1]])
		s := mustAtoi(rec[idxs[2]])
		dh := mustAtoi(rec[idxs[3]])
		fi := mustAtoi(rec[idxs[4]])
		if c == exp[0] && r == exp[1] && s == exp[2] && dh == exp[3] && fi == (exp[0]+exp[1]+exp[2]) {
			matched++
		}
	}
	if matched == 0 {
		t.Fatalf("no CSV rows matched expected fielding aggregates from DB; header=%v expects=%v", rec, expects)
	}
}

func runWithTimeout(cmd *exec.Cmd, d time.Duration) error {
	c := make(chan error, 1)
	go func() { c <- cmd.Run() }()
	select {
	case err := <-c:
		return err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return context.DeadlineExceeded
	}
}

func indexOf(sl []string, s string) int {
	for i, v := range sl {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return i
		}
	}
	return -1
}

func mustAtoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" { return 0 }
	n := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' { return 0 }
		n = n*10 + int(ch-'0')
	}
	return n
}
