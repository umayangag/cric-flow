package db

import "context"

// EtlBattingRow is a minimal DTO representing a curated batting CSV row ready to upsert.
// Adjust fields as the existing cmd/etl-importer requires when wiring the adapter.
type EtlBattingRow struct {
	PlayerName string
	Season    string
	Format    string
	Runs      int
	Balls     int
	Fours     int
	Sixes     int
	Position  int
}

// EtlBowlingRow is a minimal DTO representing a curated bowling CSV row ready to upsert.
type EtlBowlingRow struct {
	PlayerName string
	Season    string
	Format    string
	Overs     float64
	Balls     int
	Maidens   int
	Runs      int
	Wickets   int
	Economy   float64
}

// EtlRepo defines the minimal persistence API for the ETL importer.
//
//go:generate mockery --name EtlRepo --output internal/mocks --case underscore
type EtlRepo interface {
	UpsertBatting(ctx context.Context, rows []EtlBattingRow) error
	UpsertBowling(ctx context.Context, rows []EtlBowlingRow) error
}
