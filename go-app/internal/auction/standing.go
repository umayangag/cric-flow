package auction

// PlayerRoles is what the served rating vectors say about one player, as ml-service read
// them (P3-1 step 3).
//
// Two predicates and a flag, because that is all the model has. `Keeper` is the
// objective's own `is_keeper` — the served state has credited the player with a stumping
// — and `BowlingOption` is `contract.is_bowling_option` on his expected balls bowled
// against the format's threshold. They are the same two predicates the selection's
// `selection_reasons` reports, extracted into one definition so the two cannot drift.
//
// `Known` is false where the served rating state has never seen the player. No role is
// invented for him and he is counted apart from the batters: "the model has nothing to
// say about this man" and "the model says he neither keeps nor bowls" are different
// answers, and reporting the first as the second is exactly the silent substitution §8.7
// forbids.
type PlayerRoles struct {
	Known         bool
	Keeper        bool
	BowlingOption bool
}

// isBatterByElimination reports whether the model, having seen this player, says he
// neither keeps wicket nor is a bowling option.
//
// "Batter" is a label by elimination and nothing more. The system has measured no other
// role vocabulary — A-3's phase-matchup family was a recorded null — so it is not that
// the model called him a batter; it is that neither of the two predicates it does have
// is true of him.
func (r PlayerRoles) isBatterByElimination() bool {
	return r.Known && !r.Keeper && !r.BowlingOption
}

// Squad is the buyer's own purchases: the sold rows whose buyer is the auction's side.
type Squad struct {
	Players []ListedPlayer
}

// Slots is what the buyer has left to fill.
//
// `ByRole` is nil where the role read was refused — a stale registry, an unreachable
// ml-service. The total is still counted, because a squad size minus a squad is
// arithmetic over the record alone and needs no model; what cannot be said without the
// model is which of those places still needs a keeper.
type Slots struct {
	SquadSize int
	Filled    int
	Open      int
	ByRole    *SlotsByRole
}

// SlotsByRole is the same count read through the eleven's constraints.
type SlotsByRole struct {
	// Keepers and BowlingOptions are how many of each the squad already holds.
	Keepers        int
	BowlingOptions int
	// KeeperNeeded is true where the auction requires a keeper and the squad holds none.
	KeeperNeeded bool
	// MinBowlers is the constraint, and BowlingOptionsShort is how many more the squad
	// needs to meet it — zero once it does.
	MinBowlers          int
	BowlingOptionsShort int
	// UnknownRoles is how many squad members the served state has never seen, and so
	// could contribute to neither count. A squad that looks one bowler short may not be,
	// and this is the number that says so.
	UnknownRoles int
}

// Distribution is the remaining pool by role: what is still available to buy.
//
// The counts overlap by construction — a keeper who is also a bowling option is in both —
// because they are two independent predicates and not a partition. Only `Batters` and
// `Unknown` are exclusive of everything else, and the surface says as much.
type Distribution struct {
	Available      int
	Keepers        int
	BowlingOptions int
	Batters        int
	Unknown        int
}

// SquadOf returns the players this auction's own side has bought.
func SquadOf(auction Auction) Squad {
	squad := Squad{Players: make([]ListedPlayer, 0, len(auction.Players))}
	for _, player := range auction.Players {
		if player.State == StateSold && player.BuyerOppositionID == auction.BuyerOppositionID {
			squad.Players = append(squad.Players, player)
		}
	}
	return squad
}

// SlotsFor counts the buyer's open places, and — where the roles were served — which
// constraints those places still have to satisfy.
//
// Pass a nil `roles` map for a refused role read; the total is still counted and the
// breakdown is absent rather than guessed.
func SlotsFor(auction Auction, roles map[int64]PlayerRoles) Slots {
	squad := SquadOf(auction)
	filled := len(squad.Players)
	open := auction.SquadSize - filled
	if open < 0 {
		open = 0
	}
	slots := Slots{SquadSize: auction.SquadSize, Filled: filled, Open: open}
	if roles == nil {
		return slots
	}

	byRole := SlotsByRole{MinBowlers: auction.MinBowlers}
	for _, player := range squad.Players {
		role, seen := roles[player.PlayerID]
		if !seen || !role.Known {
			byRole.UnknownRoles++
			continue
		}
		if role.Keeper {
			byRole.Keepers++
		}
		if role.BowlingOption {
			byRole.BowlingOptions++
		}
	}
	byRole.KeeperNeeded = auction.RequireKeeper && byRole.Keepers == 0
	if short := auction.MinBowlers - byRole.BowlingOptions; short > 0 {
		byRole.BowlingOptionsShort = short
	}
	slots.ByRole = &byRole
	return slots
}

// DistributionOf counts the still-available players by role.
func DistributionOf(auction Auction, roles map[int64]PlayerRoles) Distribution {
	var distribution Distribution
	for _, player := range auction.Players {
		if player.State != StateAvailable {
			continue
		}
		distribution.Available++
		role, seen := roles[player.PlayerID]
		if !seen || !role.Known {
			distribution.Unknown++
			continue
		}
		if role.Keeper {
			distribution.Keepers++
		}
		if role.BowlingOption {
			distribution.BowlingOptions++
		}
		if role.isBatterByElimination() {
			distribution.Batters++
		}
	}
	return distribution
}
