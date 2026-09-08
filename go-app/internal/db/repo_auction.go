package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
)

// AuctionStore is the auction record's persistence (migration 0015). It implements
// auction.Store; the rules live in that package, so this file holds SQL and nothing else.
type AuctionStore struct{}

// NewAuctionStore returns the record backed by the process's connection pool.
func NewAuctionStore() *AuctionStore { return &AuctionStore{} }

// Create files a new auction and returns it as the record now holds it.
//
// One transaction, because an auction and the grounds it is for are one thing the
// operator entered: an auction whose venues did not land would be a record of a decision
// half made, and the next read could not tell that from a decision to name no ground.
func (s *AuctionStore) Create(ctx context.Context, created auction.Auction) (*auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	err := withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO auction
			  (id, name, format_code, buyer_opposition_id, squad_size, min_bowlers, require_keeper)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, created.ID, created.Name, created.FormatCode, created.BuyerOppositionID,
			created.SquadSize, created.MinBowlers, created.RequireKeeper); err != nil {
			return fmt.Errorf("create auction: %w", err)
		}
		for _, venueID := range created.VenueIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO auction_venue (auction_id, venue_id) VALUES ($1, $2)
				 ON CONFLICT DO NOTHING`, created.ID, venueID); err != nil {
				return fmt.Errorf("create auction venue %d: %w", venueID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, created.ID)
}

// Get returns one auction whole: its own row, its grounds and its list.
func (s *AuctionStore) Get(ctx context.Context, auctionID string) (*auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	record, err := s.readAuctionRow(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if record.VenueIDs, err = s.readVenueIDs(ctx, auctionID); err != nil {
		return nil, err
	}
	if record.Players, err = s.readPlayers(ctx, auctionID); err != nil {
		return nil, err
	}
	if record.LikelyXI, err = s.readNamedPlayers(ctx, auctionID, "auction_likely_xi"); err != nil {
		return nil, err
	}
	if record.Opposition, err = s.readOpposition(ctx, auctionID); err != nil {
		return nil, err
	}
	return record, nil
}

// readNamedPlayers reads one of the projection's assumption lists, in the order the
// operator entered it (P3-2).
//
// The table name is interpolated and is never caller-supplied: the two assumption tables
// are the same shape and reading them through one query keeps their two orderings from
// drifting apart. The only two call sites pass the two literals below.
func (s *AuctionStore) readNamedPlayers(
	ctx context.Context,
	auctionID string,
	table string,
) ([]auction.NamedPlayer, error) {
	rows, err := Pool.Query(ctx, `
		SELECT x.player_id, COALESCE(p.external_id, ''), p.player_name
		FROM `+table+` x
		JOIN player p ON p.id = x.player_id
		WHERE x.auction_id = $1
		ORDER BY x.position, x.player_id
	`, auctionID)
	if err != nil {
		return nil, fmt.Errorf("read %s %s: %w", table, auctionID, err)
	}
	defer rows.Close()

	players := make([]auction.NamedPlayer, 0, auction.TeamSize)
	for rows.Next() {
		var player auction.NamedPlayer
		if err := rows.Scan(&player.PlayerID, &player.ExternalID, &player.PlayerName); err != nil {
			return nil, fmt.Errorf("read %s %s: %w", table, auctionID, err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s %s: %w", table, auctionID, err)
	}
	return players, nil
}

// readOpposition reads the side a projection is against, or nil where the operator has not
// named one. Nil and not an empty eleven: "no opposition named" is what a projection is
// refused for, and an empty side would be a side nobody plays rather than a missing answer.
func (s *AuctionStore) readOpposition(ctx context.Context, auctionID string) (*auction.Opposition, error) {
	var opposition auction.Opposition
	err := Pool.QueryRow(ctx, `
		SELECT ao.opposition_id, COALESCE(o.opposition_name, '')
		FROM auction_opposition ao
		LEFT JOIN opposition o ON o.id = ao.opposition_id
		WHERE ao.auction_id = $1
	`, auctionID).Scan(&opposition.OppositionID, &opposition.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read auction opposition %s: %w", auctionID, err)
	}
	if opposition.Players, err = s.readNamedPlayers(ctx, auctionID, "auction_opposition_player"); err != nil {
		return nil, err
	}
	return &opposition, nil
}

// readAuctionRow reads the auction's own row, with the buying side named.
func (s *AuctionStore) readAuctionRow(ctx context.Context, auctionID string) (*auction.Auction, error) {
	var record auction.Auction
	err := Pool.QueryRow(ctx, `
		SELECT a.id, a.created_at, a.name, a.format_code, a.buyer_opposition_id,
		       COALESCE(o.opposition_name, ''), a.squad_size, a.min_bowlers, a.require_keeper
		FROM auction a
		LEFT JOIN opposition o ON o.id = a.buyer_opposition_id
		WHERE a.id = $1
	`, auctionID).Scan(&record.ID, &record.CreatedAt, &record.Name, &record.FormatCode,
		&record.BuyerOppositionID, &record.BuyerName, &record.SquadSize,
		&record.MinBowlers, &record.RequireKeeper)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auction.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read auction %s: %w", auctionID, err)
	}
	return &record, nil
}

// readVenueIDs reads the grounds one auction is for.
func (s *AuctionStore) readVenueIDs(ctx context.Context, auctionID string) ([]int64, error) {
	rows, err := Pool.Query(ctx,
		`SELECT venue_id FROM auction_venue WHERE auction_id = $1 ORDER BY venue_id`, auctionID)
	if err != nil {
		return nil, fmt.Errorf("read auction venues %s: %w", auctionID, err)
	}
	defer rows.Close()

	venueIDs := make([]int64, 0, 8)
	for rows.Next() {
		var venueID int64
		if err := rows.Scan(&venueID); err != nil {
			return nil, fmt.Errorf("read auction venues %s: %w", auctionID, err)
		}
		venueIDs = append(venueIDs, venueID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read auction venues %s: %w", auctionID, err)
	}
	return venueIDs, nil
}

// readPlayers reads the list, name-ordered, which is how a person reads a team sheet.
func (s *AuctionStore) readPlayers(ctx context.Context, auctionID string) ([]auction.ListedPlayer, error) {
	rows, err := Pool.Query(ctx, `
		SELECT ap.player_id, COALESCE(p.external_id, ''), p.player_name, ap.state,
		       COALESCE(ap.buyer_name, ''), COALESCE(ap.buyer_opposition_id, 0),
		       ap.price, ap.listed_at, ap.state_changed_at
		FROM auction_player ap
		JOIN player p ON p.id = ap.player_id
		WHERE ap.auction_id = $1
		ORDER BY p.player_name, ap.player_id
	`, auctionID)
	if err != nil {
		return nil, fmt.Errorf("read auction list %s: %w", auctionID, err)
	}
	defer rows.Close()

	players := make([]auction.ListedPlayer, 0, 64)
	for rows.Next() {
		var player auction.ListedPlayer
		if err := rows.Scan(&player.PlayerID, &player.ExternalID, &player.PlayerName, &player.State,
			&player.BuyerName, &player.BuyerOppositionID, &player.Price,
			&player.ListedAt, &player.StateChangedAt); err != nil {
			return nil, fmt.Errorf("read auction list %s: %w", auctionID, err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read auction list %s: %w", auctionID, err)
	}
	return players, nil
}

// List returns every auction, newest first, without its list.
func (s *AuctionStore) List(ctx context.Context) ([]auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT a.id, a.created_at, a.name, a.format_code, a.buyer_opposition_id,
		       COALESCE(o.opposition_name, ''), a.squad_size, a.min_bowlers, a.require_keeper
		FROM auction a
		LEFT JOIN opposition o ON o.id = a.buyer_opposition_id
		ORDER BY a.created_at DESC, a.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list auctions: %w", err)
	}
	defer rows.Close()

	auctions := make([]auction.Auction, 0, 16)
	for rows.Next() {
		var record auction.Auction
		if err := rows.Scan(&record.ID, &record.CreatedAt, &record.Name, &record.FormatCode,
			&record.BuyerOppositionID, &record.BuyerName, &record.SquadSize,
			&record.MinBowlers, &record.RequireKeeper); err != nil {
			return nil, fmt.Errorf("list auctions: %w", err)
		}
		auctions = append(auctions, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list auctions: %w", err)
	}
	return auctions, nil
}

// AddPlayers lists players who are not on the list yet and returns the auction as it now
// stands.
//
// `ON CONFLICT DO NOTHING` rather than an upsert: a player already on the list keeps the
// state he is in. Re-adding a sold player is a double-click, and quietly returning him to
// available would erase a sale nobody asked to undo.
func (s *AuctionStore) AddPlayers(
	ctx context.Context,
	auctionID string,
	playerIDs []int64,
) (*auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if _, err := s.readAuctionRow(ctx, auctionID); err != nil {
		return nil, err
	}
	err := withTx(ctx, func(tx pgx.Tx) error {
		for _, playerID := range playerIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO auction_player (auction_id, player_id, state)
				VALUES ($1, $2, $3)
				ON CONFLICT (auction_id, player_id) DO NOTHING
			`, auctionID, playerID, auction.StateAvailable); err != nil {
				return fmt.Errorf("list player %d on auction %s: %w", playerID, auctionID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, auctionID)
}

// RecordOutcome writes one entry the operator made — a sale, an unsold result, or an undo
// — and returns the auction as it now stands.
//
// An undo clears the buyer and the price with the state, in one statement, because a
// player back on the list who still carries what somebody paid for him is a record of two
// contradictory facts. The write is refused where the player is not on this list: an
// outcome for a player nobody listed is a typo, and answering it with a silent no-op would
// leave the operator believing the room had moved on.
func (s *AuctionStore) RecordOutcome(
	ctx context.Context,
	auctionID string,
	outcome auction.Outcome,
) (*auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if err := outcome.Validate(); err != nil {
		return nil, err
	}
	var buyerOppositionID *int64
	var buyerName *string
	if outcome.State == auction.StateSold {
		buyerName = &outcome.BuyerName
		if outcome.BuyerOppositionID > 0 {
			buyerOppositionID = &outcome.BuyerOppositionID
		}
	}
	tag, err := Pool.Exec(ctx, `
		UPDATE auction_player
		SET state = $3, buyer_name = $4, buyer_opposition_id = $5, price = $6,
		    state_changed_at = now()
		WHERE auction_id = $1 AND player_id = $2
	`, auctionID, outcome.PlayerID, outcome.State, buyerName, buyerOppositionID, outcome.Price)
	if err != nil {
		return nil, fmt.Errorf("record outcome for player %d on auction %s: %w",
			outcome.PlayerID, auctionID, err)
	}
	if tag.RowsAffected() == 0 {
		return nil, auction.ErrNotFound
	}
	return s.Get(ctx, auctionID)
}

// SetAssumptions replaces the projection's named assumptions and returns the auction as it
// now stands (P3-2).
//
// Replace rather than merge: an eleven is a list, and merging a shorter list into a longer
// one would leave the record holding a player the operator has just removed. One
// transaction, because a half-written eleven read back mid-write would be an assumption
// nobody made, and every projection under it would be labelled with it.
func (s *AuctionStore) SetAssumptions(
	ctx context.Context,
	auctionID string,
	change auction.AssumptionsChange,
) (*auction.Auction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if err := change.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.readAuctionRow(ctx, auctionID); err != nil {
		return nil, err
	}
	err := withTx(ctx, func(tx pgx.Tx) error {
		if change.LikelyXIPlayerIDs != nil {
			if err := replaceAssumptionList(
				ctx, tx, "auction_likely_xi", auctionID, *change.LikelyXIPlayerIDs); err != nil {
				return err
			}
		}
		if change.Opposition == nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auction_opposition (auction_id, opposition_id)
			VALUES ($1, $2)
			ON CONFLICT (auction_id) DO UPDATE SET opposition_id = EXCLUDED.opposition_id
		`, auctionID, change.Opposition.OppositionID); err != nil {
			return fmt.Errorf("set auction opposition %s: %w", auctionID, err)
		}
		return replaceAssumptionList(
			ctx, tx, "auction_opposition_player", auctionID, change.Opposition.PlayerIDs)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, auctionID)
}

// replaceAssumptionList rewrites one assumption's eleven inside the caller's transaction.
func replaceAssumptionList(
	ctx context.Context,
	tx pgx.Tx,
	table string,
	auctionID string,
	playerIDs []int64,
) error {
	if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE auction_id = $1`, auctionID); err != nil {
		return fmt.Errorf("clear %s %s: %w", table, auctionID, err)
	}
	for position, playerID := range playerIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO `+table+` (auction_id, player_id, position) VALUES ($1, $2, $3)
		`, auctionID, playerID, position); err != nil {
			return fmt.Errorf("write %s %s player %d: %w", table, auctionID, playerID, err)
		}
	}
	return nil
}
