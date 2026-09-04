package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// PlayerBiographyStore is the persistence for X-1a's acquired biographies (migration
// 0010). It implements biography.Store; the acquisition rules live in that package, so
// this file holds SQL and nothing else.
type PlayerBiographyStore struct{}

// NewPlayerBiographyStore returns the store backed by the process's connection pool.
func NewPlayerBiographyStore() *PlayerBiographyStore { return &PlayerBiographyStore{} }

// unmatchedListLimit is how many gaps Coverage reports. The list exists to be worked
// down by hand, so it is bounded at a length a person will actually read.
const unmatchedListLimit = 50

// ListPlayers returns every player with his Cricsheet registry identifier and how many
// times he has been fielded. The appearance count comes from match_player — the fielded
// elevens — because that is the weight the coverage report is expressed in.
func (s *PlayerBiographyStore) ListPlayers(ctx context.Context) ([]biography.Player, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT p.id, COALESCE(p.external_id, ''), p.player_name, COUNT(mp.match_id)
		FROM player p
		LEFT JOIN match_player mp ON mp.player_id = p.id
		GROUP BY p.id, p.external_id, p.player_name
		ORDER BY COUNT(mp.match_id) DESC, p.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []biography.Player
	for rows.Next() {
		var player biography.Player
		if err := rows.Scan(&player.ID, &player.ExternalID, &player.Name, &player.Appearances); err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	return players, rows.Err()
}

// UpsertBiographies writes one row per player, replacing what was there.
//
// One transaction for the whole pass: a half-written biography table would make the
// coverage report — the only artefact X-1a produces — a measurement of an interrupted
// run rather than of the source, and the X-1b gate reads that figure.
func (s *PlayerBiographyStore) UpsertBiographies(ctx context.Context, records []biography.Record) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	if len(records) == 0 {
		return nil
	}
	return withTx(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for i := range records {
			record := records[i]
			batch.Queue(`
				INSERT INTO player_biography (
				  player_id, cricinfo_id, wikidata_qid, birth_date, batting_hand,
				  bowling_style, bowling_style_raw, career_end_date, death_date,
				  source, source_license, fetched_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
				ON CONFLICT (player_id) DO UPDATE SET
				  cricinfo_id = EXCLUDED.cricinfo_id,
				  wikidata_qid = EXCLUDED.wikidata_qid,
				  birth_date = EXCLUDED.birth_date,
				  batting_hand = EXCLUDED.batting_hand,
				  bowling_style = EXCLUDED.bowling_style,
				  bowling_style_raw = EXCLUDED.bowling_style_raw,
				  career_end_date = EXCLUDED.career_end_date,
				  death_date = EXCLUDED.death_date,
				  source = EXCLUDED.source,
				  source_license = EXCLUDED.source_license,
				  fetched_at = EXCLUDED.fetched_at
			`,
				record.PlayerID, nullString(record.CricinfoID), nullString(record.WikidataQID),
				record.BirthDate, nullString(record.BattingHand), nullString(record.BowlingStyle),
				nullString(record.BowlingStyleRaw), record.CareerEndDate, record.DeathDate,
				record.Source, nullString(record.License), record.FetchedAt)
		}
		results := tx.SendBatch(ctx, batch)
		for range records {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		return results.Close()
	})
}

// nullString turns an empty string into a NULL, so "we have no value" is stored as the
// absence of one rather than as an empty string a COUNT(...) would happily count.
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Coverage measures what is stored, weighted by appearances, per format and gender.
func (s *PlayerBiographyStore) Coverage(ctx context.Context) (biography.Coverage, error) {
	if Pool == nil {
		return biography.Coverage{}, errors.New("db pool not initialized")
	}
	coverage := biography.Coverage{
		GeneratedAt:   time.Now().UTC(),
		SourceLicense: biography.SourceLicense,
	}
	rows, err := coverageRows(ctx)
	if err != nil {
		return biography.Coverage{}, err
	}
	biography.SortRows(rows)
	coverage.Rows = rows
	coverage.Total = biography.Total(rows)
	if err := fillTotalPlayers(ctx, &coverage.Total); err != nil {
		return biography.Coverage{}, err
	}
	if coverage.Unmatched, err = unmatchedPlayers(ctx); err != nil {
		return biography.Coverage{}, err
	}
	if coverage.LastFetchedAt, err = lastFetchedAt(ctx); err != nil {
		return biography.Coverage{}, err
	}
	return coverage, nil
}

// coverageRows counts fielded player-sides per format and gender, and how many of them
// have each fact.
//
// The counts are over appearances rather than players by design: see
// biography.CoverageRow. A style counts only when it is in the controlled vocabulary —
// 'unknown' is a stated style this system cannot place and is not coverage.
func coverageRows(ctx context.Context) ([]biography.CoverageRow, error) {
	rows, err := Pool.Query(ctx, `
		SELECT f.code,
		       COALESCE(m.gender, 'unknown'),
		       COUNT(*),
		       COUNT(*) FILTER (WHERE b.player_id IS NOT NULL),
		       COUNT(*) FILTER (WHERE b.wikidata_qid IS NOT NULL),
		       COUNT(*) FILTER (WHERE b.birth_date IS NOT NULL),
		       COUNT(*) FILTER (WHERE b.batting_hand IS NOT NULL),
		       COUNT(*) FILTER (WHERE b.bowling_style IS NOT NULL AND b.bowling_style <> $1),
		       COUNT(*) FILTER (WHERE b.career_end_date IS NOT NULL),
		       COUNT(*) FILTER (WHERE b.death_date IS NOT NULL),
		       COUNT(DISTINCT mp.player_id),
		       COUNT(DISTINCT mp.player_id) FILTER (WHERE b.wikidata_qid IS NOT NULL)
		FROM match_player mp
		JOIN match m ON m.match_id = mp.match_id
		JOIN match_format f ON f.id = m.format_id
		LEFT JOIN player_biography b ON b.player_id = mp.player_id
		GROUP BY f.code, COALESCE(m.gender, 'unknown')
	`, biography.StyleUnknown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var coverageRows []biography.CoverageRow
	for rows.Next() {
		var row biography.CoverageRow
		if err := rows.Scan(
			&row.Format, &row.Gender, &row.Appearances, &row.Attempted, &row.Matched,
			&row.BirthDate, &row.BattingHand, &row.BowlingStyle, &row.CareerEnd, &row.Death,
			&row.Players, &row.MatchedPlayers,
		); err != nil {
			return nil, err
		}
		coverageRows = append(coverageRows, row)
	}
	return coverageRows, rows.Err()
}

// fillTotalPlayers counts distinct players once across every format, because a player who
// appears in three formats would otherwise be counted three times by summing the rows.
func fillTotalPlayers(ctx context.Context, total *biography.CoverageRow) error {
	return Pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.player_id),
		       COUNT(DISTINCT mp.player_id) FILTER (WHERE b.wikidata_qid IS NOT NULL)
		FROM match_player mp
		LEFT JOIN player_biography b ON b.player_id = mp.player_id
	`).Scan(&total.Players, &total.MatchedPlayers)
}

// unmatchedPlayers lists the players nothing was found for, worst first.
func unmatchedPlayers(ctx context.Context) ([]biography.UnmatchedPlayer, error) {
	rows, err := Pool.Query(ctx, `
		SELECT p.id, p.player_name, COALESCE(p.external_id, ''),
		       COALESCE(b.cricinfo_id, ''), COUNT(mp.match_id), b.player_id IS NOT NULL
		FROM player p
		JOIN match_player mp ON mp.player_id = p.id
		LEFT JOIN player_biography b ON b.player_id = p.id
		WHERE b.player_id IS NULL OR b.wikidata_qid IS NULL
		GROUP BY p.id, p.player_name, p.external_id, b.cricinfo_id, b.player_id
		ORDER BY COUNT(mp.match_id) DESC, p.id
		LIMIT $1
	`, unmatchedListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var unmatched []biography.UnmatchedPlayer
	for rows.Next() {
		var player biography.UnmatchedPlayer
		if err := rows.Scan(&player.PlayerID, &player.Name, &player.CricsheetID,
			&player.CricinfoID, &player.Appearances, &player.Attempted); err != nil {
			return nil, err
		}
		unmatched = append(unmatched, player)
	}
	return unmatched, rows.Err()
}

// lastFetchedAt reports how fresh the acquisition is, or nil when it has never run.
func lastFetchedAt(ctx context.Context) (*time.Time, error) {
	var fetchedAt *time.Time
	if err := Pool.QueryRow(ctx,
		`SELECT MAX(fetched_at) FROM player_biography`).Scan(&fetchedAt); err != nil {
		return nil, err
	}
	return fetchedAt, nil
}
