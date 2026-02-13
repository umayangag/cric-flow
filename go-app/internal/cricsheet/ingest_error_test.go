package cricsheet_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet/mocks"
)

func TestImportDir_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	
	// Setup mocks
	mdb := &tmocks.CricsheetDBMock{}
	mweather := &tmocks.WeatherClientMock{}
	
	prevDB := cricsheet.GetCricsheetDB()
	prevWeather := cricsheet.GetWeatherClient()
	cricsheet.SetCricsheetDB(mdb)
	cricsheet.SetWeatherClient(mweather)
	t.Cleanup(func() {
		cricsheet.SetCricsheetDB(prevDB)
		cricsheet.SetWeatherClient(prevWeather)
	})

	// Prepare temp dir with two files
	tmpDir, err := os.MkdirTemp("", "import-dir-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "match1.json")
	file2 := filepath.Join(tmpDir, "match2.json")
	
	require.NoError(t, os.WriteFile(file1, []byte(`{"info":{"match_type":"T20","teams":["A","B"],"dates":["2024-01-01"]}}`), 0600))
	require.NoError(t, os.WriteFile(file2, []byte(`{"info":{"match_type":"T20","teams":["C","D"],"dates":["2024-01-02"]}}`), 0600))

	t.Run("FailFast_Enabled", func(t *testing.T) {
		// Expect calls to fail. Since it is concurrent, we might get multiple calls before cancellation takes effect.
		mdb.On("EnsureMatchWithFormat", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))
		
		opts := &cricsheet.Options{FailFast: true}
		_, err := cricsheet.ImportDir(ctx, tmpDir, opts)
		
		require.Error(t, err)
	})

	t.Run("FailFast_Disabled", func(t *testing.T) {
		// Reset mocks
		mdb.ExpectedCalls = nil
		mdb.On("EnsureMatchWithFormat", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))
		
		opts := &cricsheet.Options{FailFast: false}
		count, err := cricsheet.ImportDir(ctx, tmpDir, opts)
		
		// In current implementation, this will FAIL and return error because it uses errgroup.WithContext
		require.NoError(t, err, "Should not return error when FailFast is disabled")
		require.Equal(t, 0, count, "Count should be 0 as both failed, but process should have continued")
	})
}
