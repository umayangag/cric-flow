// Package auction is the record of an auction the operator ran (P3-1): who was listed,
// who was sold to whom for how much, who went unsold, and what that leaves the buyer.
//
// Nothing here predicts anything and nothing here chooses anything. The rule the whole
// module is built on is that this is valuation and projection and never XI-picking: the
// system's own record is that optimised selection in domestic T20 is indistinguishable
// from rating order (plan §8.8), the IPL is domestic T20, and so no code path from this
// package reaches the selection objective. The two roles it reads — keeper and bowling
// option — are the objective's own constraint predicates, read off the served rating
// vectors by ml-service, and they are the only thing this module and the selection share.
//
// The package holds the shape of the record, the vocabulary it is spelled in, and the
// arithmetic over it: which slots the buyer has left, and what the remaining pool looks
// like by role. `internal/db` implements the storage; the handlers in `internal/server`
// put it on the wire. It is its own package so that "what an auction is" is stated once,
// away from both the SQL and the HTTP.
package auction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound reports that no auction, or no listed player, matches the id asked for. A
// sentinel rather than a nil result so a handler answers 404 with the id, instead of 200
// with an empty record that reads like an auction nobody has entered anything into.
var ErrNotFound = errors.New("no auction with that id")

// The state a listed player is in. Three, because an auction has three: he is still to
// come, he has been bought, or he was passed over. There is no fourth and none is coming
// — a player put back after a mistyped entry returns to Available rather than gaining a
// state of his own.
//
// Wire vocabulary, declared once in contracts/ops-console.contract.json and asserted from
// every side that reads it (H-24).
const (
	StateAvailable = "available"
	StateSold      = "sold"
	StateUnsold    = "unsold"
)

// States returns every state a listed player can be in, in the order a surface groups
// them.
func States() []string { return []string{StateAvailable, StateSold, StateUnsold} }

// IsKnownState reports whether a state is one this record can hold.
func IsKnownState(state string) bool {
	for _, known := range States() {
		if state == known {
			return true
		}
	}
	return false
}

// The L-1 keys the auction surface labels its numbers under (contracts/ops-console.contract.json).
//
// Every labelled number on a surface must have a glossary entry a reader can open, and
// ml-service's completeness gate asserts these two have one. They are two rather than
// five because the glossary explains a node: one entry covers the open slots and their
// breakdown by constraint, the other covers the remaining pool's counts by role.
const (
	MetricOpenSlots     = "auction_open_slots"
	MetricAvailableRole = "auction_available_by_role"
)

// MetricKeys returns the L-1 keys this module's numbers are reported under.
func MetricKeys() []string { return []string{MetricOpenSlots, MetricAvailableRole} }

// NewID mints the identifier an auction is filed and read back under.
//
// A UUID rather than a sequence, for the reason an issued prediction's is: go-app
// generates it before the insert, so every write that follows names the record it changed
// without a round trip to learn what the database called it.
func NewID() string { return uuid.NewString() }

// Auction is one auction as the record holds it.
type Auction struct {
	ID        string
	CreatedAt time.Time
	Name      string

	// FormatCode is one of the simulator's formats: an auction is for a competition with
	// an innings length, because every later item of Phase 3 projects a total.
	FormatCode string

	// BuyerOppositionID is the side this auction fills a squad for, and BuyerName is that
	// side as the database spells it. A sold row naming this id is in the squad.
	BuyerOppositionID int64
	BuyerName         string

	// VenueIDs are the grounds the auction is for. Ids, not names: a projection at a
	// ground the database cannot resolve is a substitution nobody can see (§8.7).
	VenueIDs []int64

	// The eleven's constraints, which are the auction's. The same three the predict path
	// takes, so an open slot is described in the objective's constraint vocabulary and
	// not a second one invented here.
	SquadSize     int
	MinBowlers    int
	RequireKeeper bool

	// Players is the list, in the order it is read: by name.
	Players []ListedPlayer
}

// ListedPlayer is one player on the list and the state the operator last recorded.
type ListedPlayer struct {
	PlayerID int64
	// ExternalID is the Cricsheet registry id, and it is what ml-service knows a player
	// by: the rating state has been keyed on it since P-1. The role read sends these, not
	// the database ids, which mean nothing outside this repository. Empty for a player
	// this database has no registry id for — the role read reports him unknown rather
	// than guessing, which is the same answer it gives for an id the state has not seen.
	ExternalID string
	PlayerName string
	State      string

	// BuyerName is what the operator typed for the buying franchise, and is present on a
	// sold row only. BuyerOppositionID is set where that buyer is a side this database
	// knows, and is zero otherwise — most franchises in a room are sides the operator
	// names and this system has no id for.
	BuyerName         string
	BuyerOppositionID int64

	// Price is what the buyer paid, in the auction room's own unit. Nil off a sold row,
	// and nil is different from zero: a player bought at the base price is not a player
	// nobody bought.
	Price *int64

	ListedAt       time.Time
	StateChangedAt time.Time
}

// Outcome is one entry the operator makes against a listed player: a sale, an unsold
// result, or an undo back to available.
type Outcome struct {
	PlayerID          int64
	State             string
	BuyerName         string
	BuyerOppositionID int64
	Price             *int64
}

// Validate refuses an entry that is not a state of an auction.
//
// A sale needs a buyer and a price; anything else may carry neither. The operator is
// typing during a live auction, so the one useful thing this can do is refuse the shapes
// that would leave the record holding half of a sale that did not happen — and say which
// half is wrong rather than reporting "invalid".
func (o Outcome) Validate() error {
	if o.PlayerID <= 0 {
		return errors.New("an outcome names a player")
	}
	if !IsKnownState(o.State) {
		return fmt.Errorf("state %q is not one of %s", o.State, strings.Join(States(), ", "))
	}
	if o.State == StateSold {
		if strings.TrimSpace(o.BuyerName) == "" {
			return errors.New("a sale names the buyer")
		}
		if o.Price == nil {
			return errors.New("a sale names the price")
		}
		if *o.Price < 0 {
			return errors.New("a price is not negative")
		}
		return nil
	}
	if strings.TrimSpace(o.BuyerName) != "" || o.BuyerOppositionID != 0 || o.Price != nil {
		return fmt.Errorf("a %s outcome carries no buyer and no price", o.State)
	}
	return nil
}

// Store is the auction record's persistence. Every write returns the auction as it now
// stands, because that is what the operator's next decision is made against — and a
// surface that patched its own copy instead would be one mistyped entry away from showing
// a squad the record does not hold.
type Store interface {
	Create(ctx context.Context, auction Auction) (*Auction, error)
	Get(ctx context.Context, auctionID string) (*Auction, error)
	// List returns every auction, newest first, without its player list: it is the index
	// an operator finds an auction from after a reload, not the record itself.
	List(ctx context.Context) ([]Auction, error)
	// AddPlayers lists players who are not on the list yet. A player already listed is
	// left exactly as he stands: adding him twice is a double-click, not an instruction
	// to forget that he was sold.
	AddPlayers(ctx context.Context, auctionID string, playerIDs []int64) (*Auction, error)
	RecordOutcome(ctx context.Context, auctionID string, outcome Outcome) (*Auction, error)
}
