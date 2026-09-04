// Package dbtest holds the gate every database integration suite passes through.
//
// It exists because the gate was copied three times — internal/db, runplan and
// datasetregistry each had their own `guardIntegration` — and only one copy can be
// improved at a time when they are separate. The gate is a safety device, so it has one
// implementation.
package dbtest

import (
	"os"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/db/connection"
)

// runFlag is the environment variable that opts a run into the database suites.
const runFlag = "RUN_DB_TESTS"

// SkipUnlessScratchDatabase skips the calling test unless RUN_DB_TESTS=1, and fails it
// outright when the run is pointed at the working database.
//
// Every fixture in these suites begins with a TRUNCATE. The connection defaults to the
// working database and `go-app/Makefile` exports that name, so the natural way to run
// the suites was also the way to delete an import that takes hours to rebuild. A scratch
// database is one CREATE DATABASE away, which is why this refuses rather than warns.
func SkipUnlessScratchDatabase(t *testing.T) {
	t.Helper()
	if os.Getenv(runFlag) != "1" {
		t.Skipf("integration test skipped; set %s=1 to run", runFlag)
	}
	database := os.Getenv("POSTGRES_DB")
	if database == "" || database == connection.DefaultDatabase {
		t.Fatalf(
			"refusing to run the destructive database integration suite against %q: "+
				"set POSTGRES_DB to a scratch database (see go-app/Makefile, target test-db)",
			connection.DefaultDatabase)
	}
}
