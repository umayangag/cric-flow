package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// The auction record on the wire (P3-1).
//
// One shape, returned by every read and by every write, because the operator's next
// decision is made against the whole record: a write that answered with only the row it
// changed would leave the surface patching its own copy of a squad, and one mistyped entry
// away from showing a squad the record does not hold.

// auctionResponse is an auction as it now stands, with what the model says about it.
type auctionResponse struct {
	Auction auctionRecord `json:"auction"`
	Squad   auctionSquad  `json:"squad"`
	Slots   auctionSlots  `json:"slots"`
	// Distribution is the remaining pool by role. Absent where the role read was refused
	// — the counts come off the model, and a refused read shows no number rather than a
	// zero (§8.7). `Roles` says why.
	Distribution *auctionDistribution `json:"distribution,omitempty"`
	Roles        auctionRolesBlock    `json:"roles"`
}

// auctionRecord is what the operator entered, and nothing derived.
type auctionRecord struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Format        string          `json:"format"`
	CreatedAt     string          `json:"created_at"`
	Buyer         auctionBuyer    `json:"buyer"`
	VenueIDs      []int64         `json:"venue_ids"`
	SquadSize     int             `json:"squad_size"`
	MinBowlers    int             `json:"min_bowlers"`
	RequireKeeper bool            `json:"require_keeper"`
	Players       []auctionPlayer `json:"players"`
}

// auctionBuyer is the side this auction fills a squad for.
type auctionBuyer struct {
	ClubID int64  `json:"club_id"`
	Name   string `json:"name"`
}

// auctionSummary is one auction on the index: enough to find it again after a reload, and
// no list.
type auctionSummary struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Format    string       `json:"format"`
	CreatedAt string       `json:"created_at"`
	Buyer     auctionBuyer `json:"buyer"`
	SquadSize int          `json:"squad_size"`
}

// auctionPlayer is one listed player: the record's facts, and the model's roles beside
// them.
type auctionPlayer struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	// State is "available", "sold" or "unsold" — the vocabulary declared in
	// contracts/ops-console.contract.json and asserted from every side (H-24).
	State string `json:"state"`
	// BuyerName, BuyerClubID and Price are the sale, and are absent off a sold row.
	// Price is a pointer because nil and zero are different facts: a player bought at the
	// base price is not a player nobody bought.
	BuyerName      string `json:"buyer_name,omitempty"`
	BuyerClubID    int64  `json:"buyer_club_id,omitempty"`
	Price          *int64 `json:"price,omitempty"`
	StateChangedAt string `json:"state_changed_at"`
	// Roles is what the served rating vectors say about him. Absent — not empty — where
	// the role read was refused: "the model was not asked" and "the model says he neither
	// keeps nor bowls" are different answers and the surface must be able to tell them
	// apart.
	Roles *auctionPlayerRoles `json:"roles,omitempty"`
}

// auctionPlayerRoles is the model's answer about one player.
type auctionPlayerRoles struct {
	// Known is false where the served rating state has never seen him. His Roles are then
	// empty and mean nothing; the surface shows him as unknown and not as a batter.
	Known bool `json:"known"`
	// Roles is a subset of the contract's `selection_roles`. Empty for a known player
	// means neither predicate holds — a batter by elimination, which is what the label
	// means and what the glossary entry says it means.
	Roles []string `json:"roles"`
}

// auctionSquad is what the buyer has bought.
type auctionSquad struct {
	Size      int             `json:"size"`
	PlayerIDs []int64         `json:"player_ids"`
	Players   []auctionPlayer `json:"players"`
}

// auctionSlots is what the buyer has left to fill.
type auctionSlots struct {
	SquadSize int `json:"squad_size"`
	Filled    int `json:"filled"`
	Open      int `json:"open"`
	// ByRole is absent where the role read was refused: the total is arithmetic over the
	// record and needs no model, but which places still need a keeper is the model's
	// answer and is not guessed.
	ByRole *auctionSlotsByRole `json:"by_role,omitempty"`
}

// auctionSlotsByRole reads the same places through the eleven's constraints.
type auctionSlotsByRole struct {
	Keepers             int  `json:"keepers"`
	BowlingOptions      int  `json:"bowling_options"`
	KeeperNeeded        bool `json:"keeper_needed"`
	MinBowlers          int  `json:"min_bowlers"`
	BowlingOptionsShort int  `json:"bowling_options_short"`
	// UnknownRoles is how many squad members the served state has never seen. A squad
	// that looks a bowler short may not be, and this is the number that says so.
	UnknownRoles int `json:"unknown_roles"`
}

// auctionDistribution is the still-available players by role.
type auctionDistribution struct {
	Available int `json:"available"`
	// Keepers and BowlingOptions overlap: two independent predicates, not a partition.
	// Batters and Unknown are exclusive of everything else.
	Keepers        int `json:"keepers"`
	BowlingOptions int `json:"bowling_options"`
	Batters        int `json:"batters"`
	Unknown        int `json:"unknown"`
}

// auctionRolesBlock says whether the model answered, and — either way — what it answered
// with (§8.7).
type auctionRolesBlock struct {
	Available bool `json:"available"`
	// RunID and RatingsThrough are the rating state that served the roles, off the answer
	// itself (P1-5). Present only where it answered.
	RunID          string `json:"run_id,omitempty"`
	RatingsThrough string `json:"ratings_through,omitempty"`
	// Code, Message and Hint are ml-service's own refusal, kept by name — RATINGS_STALE
	// for a state past H-11's limit, XI_MODEL_UNAVAILABLE for nothing loaded,
	// ML_UNREACHABLE for a service that did not answer.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// respondWithAuction reads the roles for the auction's list and writes the whole answer.
//
// The role read is allowed to fail. The record is facts the operator typed and reading it
// back needs no model at all, so a stale registry refuses the roles and the counts read
// off them, and leaves the list exactly as entered — with the refusal named on the wire
// where a reader will meet it, never only in a log.
func (a *App) respondWithAuction(w http.ResponseWriter, r *http.Request, record *auction.Auction) {
	roles, block := a.readAuctionRoles(r.Context(), record)
	writeJSON(w, http.StatusOK, newAuctionResponse(*record, roles, block))
}

// readAuctionRoles asks ml-service what the served vectors say about the listed players.
//
// A player with no registry id is not sent: ml-service knows players by that id and by
// nothing else, so asking under a database id would be asking about somebody else. He
// comes back unknown, which is the honest answer and the same one an id the rating state
// has never seen gets.
func (a *App) readAuctionRoles(
	ctx context.Context,
	record *auction.Auction,
) (map[int64]auction.PlayerRoles, auctionRolesBlock) {
	keyByPlayerID := make(map[int64]string, len(record.Players))
	keys := make([]string, 0, len(record.Players))
	for _, player := range record.Players {
		if player.ExternalID == "" {
			continue
		}
		keyByPlayerID[player.PlayerID] = player.ExternalID
		keys = append(keys, player.ExternalID)
	}

	if len(keys) == 0 {
		// Nothing was read, so nothing is stamped. An empty list's distribution would be
		// all zeroes and would look exactly like a served answer with no keepers left in
		// it; "the model was not asked" is a different statement and this is it (§8.7).
		return nil, auctionRolesBlock{
			Available: false,
			Code:      rolesNotReadCode,
			Message:   "no player on this list carries a registry id the served ratings could be asked about",
			Hint:      "add players to the list; the roles are read on every read of the auction",
		}
	}

	result, err := a.mlClient.PlayerRoles(ctx, record.FormatCode, keys)
	if err != nil {
		refusal := refusalFrom(err)
		slog.Warn("auction: the role read was refused; the record is served without it",
			slog.String("auction_id", record.ID),
			slog.String("code", refusal.Code),
			slog.Any("err", err))
		return nil, auctionRolesBlock{
			Available: false,
			Code:      refusal.Code,
			Message:   refusal.Message,
			Hint:      refusal.Hint,
		}
	}

	roles := make(map[int64]auction.PlayerRoles, len(record.Players))
	for _, player := range record.Players {
		key, hasKey := keyByPlayerID[player.PlayerID]
		if !hasKey {
			roles[player.PlayerID] = auction.PlayerRoles{}
			continue
		}
		roles[player.PlayerID] = result.Roles[key]
	}
	return roles, auctionRolesBlock{
		Available:      true,
		RunID:          result.Served.RunID,
		RatingsThrough: result.Served.RatingsThrough,
	}
}

// newAuctionResponse renders the record, the squad, the slots and the distribution.
func newAuctionResponse(
	record auction.Auction,
	roles map[int64]auction.PlayerRoles,
	block auctionRolesBlock,
) auctionResponse {
	squad := auction.SquadOf(record)
	slots := auction.SlotsFor(record, roles)

	response := auctionResponse{
		Auction: newAuctionRecord(record, roles),
		Squad:   newAuctionSquad(squad, roles),
		Slots:   newAuctionSlots(slots),
		Roles:   block,
	}
	if roles == nil {
		return response
	}
	distribution := auction.DistributionOf(record, roles)
	response.Distribution = &auctionDistribution{
		Available:      distribution.Available,
		Keepers:        distribution.Keepers,
		BowlingOptions: distribution.BowlingOptions,
		Batters:        distribution.Batters,
		Unknown:        distribution.Unknown,
	}
	return response
}

func newAuctionRecord(record auction.Auction, roles map[int64]auction.PlayerRoles) auctionRecord {
	venues := record.VenueIDs
	if venues == nil {
		venues = []int64{}
	}
	return auctionRecord{
		ID:            record.ID,
		Name:          record.Name,
		Format:        record.FormatCode,
		CreatedAt:     record.CreatedAt.UTC().Format(time.RFC3339),
		Buyer:         auctionBuyer{ClubID: record.BuyerOppositionID, Name: record.BuyerName},
		VenueIDs:      venues,
		SquadSize:     record.SquadSize,
		MinBowlers:    record.MinBowlers,
		RequireKeeper: record.RequireKeeper,
		Players:       newAuctionPlayers(record.Players, roles),
	}
}

func newAuctionPlayers(
	listed []auction.ListedPlayer,
	roles map[int64]auction.PlayerRoles,
) []auctionPlayer {
	players := make([]auctionPlayer, 0, len(listed))
	for _, player := range listed {
		players = append(players, newAuctionPlayer(player, roles))
	}
	return players
}

func newAuctionPlayer(listed auction.ListedPlayer, roles map[int64]auction.PlayerRoles) auctionPlayer {
	player := auctionPlayer{
		PlayerID:       listed.PlayerID,
		PlayerName:     listed.PlayerName,
		State:          listed.State,
		BuyerName:      listed.BuyerName,
		BuyerClubID:    listed.BuyerOppositionID,
		Price:          listed.Price,
		StateChangedAt: listed.StateChangedAt.UTC().Format(time.RFC3339),
	}
	if roles == nil {
		return player
	}
	read := roles[listed.PlayerID]
	player.Roles = &auctionPlayerRoles{Known: read.Known, Roles: roleNames(read)}
	return player
}

// roleNames spells the two predicates in the contract's vocabulary, in its order.
func roleNames(read auction.PlayerRoles) []string {
	names := make([]string, 0, 2)
	if !read.Known {
		return names
	}
	if read.Keeper {
		names = append(names, predictteam.RoleKeeper)
	}
	if read.BowlingOption {
		names = append(names, predictteam.RoleBowlingOption)
	}
	return names
}

func newAuctionSquad(squad auction.Squad, roles map[int64]auction.PlayerRoles) auctionSquad {
	playerIDs := make([]int64, 0, len(squad.Players))
	for _, player := range squad.Players {
		playerIDs = append(playerIDs, player.PlayerID)
	}
	return auctionSquad{
		Size:      len(squad.Players),
		PlayerIDs: playerIDs,
		Players:   newAuctionPlayers(squad.Players, roles),
	}
}

func newAuctionSlots(slots auction.Slots) auctionSlots {
	rendered := auctionSlots{SquadSize: slots.SquadSize, Filled: slots.Filled, Open: slots.Open}
	if slots.ByRole == nil {
		return rendered
	}
	rendered.ByRole = &auctionSlotsByRole{
		Keepers:             slots.ByRole.Keepers,
		BowlingOptions:      slots.ByRole.BowlingOptions,
		KeeperNeeded:        slots.ByRole.KeeperNeeded,
		MinBowlers:          slots.ByRole.MinBowlers,
		BowlingOptionsShort: slots.ByRole.BowlingOptionsShort,
		UnknownRoles:        slots.ByRole.UnknownRoles,
	}
	return rendered
}

// newAuctionSummaries renders the index.
func newAuctionSummaries(auctions []auction.Auction) []auctionSummary {
	summaries := make([]auctionSummary, 0, len(auctions))
	for _, record := range auctions {
		summaries = append(summaries, auctionSummary{
			ID:        record.ID,
			Name:      record.Name,
			Format:    record.FormatCode,
			CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
			Buyer:     auctionBuyer{ClubID: record.BuyerOppositionID, Name: record.BuyerName},
			SquadSize: record.SquadSize,
		})
	}
	return summaries
}

// playerSearchResponse is what a cross-club name search found.
type playerSearchResponse struct {
	Players []playerSearchResult `json:"players"`
}

// playerSearchResult is one player the search found, with what an operator needs in order
// to know he is the right man: who he has played for and in which formats.
type playerSearchResult struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	// IsWicketKeeper is `player.is_wicket_keeper` — the *database's* flag, set from an
	// import's name sets. It is named as the database's flag everywhere it is shown,
	// because it is not the model's keeper role: that is read off the served vectors and
	// arrives on the auction's own rows.
	IsWicketKeeper bool `json:"is_wicket_keeper"`
	// LastPlayed is YYYY-MM-DD, absent where this database holds no appearance for him.
	LastPlayed string   `json:"last_played,omitempty"`
	Clubs      []string `json:"clubs"`
	Formats    []string `json:"formats"`
	// Excluded is the retirement ledger's verdict, shown and never applied: an auction
	// list is what the room is selling, so the ledger's opinion belongs beside a name and
	// not instead of one.
	Excluded bool   `json:"excluded"`
	Reason   string `json:"reason,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func newPlayerSearchResults(found []db.PlayerSearchRow) []playerSearchResult {
	results := make([]playerSearchResult, 0, len(found))
	for _, row := range found {
		result := playerSearchResult{
			PlayerID:       row.PlayerID,
			PlayerName:     row.PlayerName,
			IsWicketKeeper: row.IsWicketKeeper,
			Clubs:          row.Clubs,
			Formats:        row.Formats,
			Excluded:       row.Excluded,
			Reason:         row.Reason,
			Detail:         row.Detail,
		}
		if !row.LastPlayed.IsZero() {
			result.LastPlayed = row.LastPlayed.Format(time.DateOnly)
		}
		if result.Clubs == nil {
			result.Clubs = []string{}
		}
		if result.Formats == nil {
			result.Formats = []string{}
		}
		results = append(results, result)
	}
	return results
}
