package auction_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
)

// A small auction the arithmetic can be read off by eye: the buying side is club 1, a
// rival is club 2, and six players sit in the states an auction leaves them in.
//
//	1 keeper, sold to the buyer
//	2 bowling option, sold to the rival
//	3 bowling option, available
//	4 keeper and bowling option, available
//	5 neither predicate, available (a batter by elimination)
//	6 unknown to the served state, available
func listedAuction() (auction.Auction, map[int64]auction.PlayerRoles) {
	price := int64(100)
	record := auction.Auction{
		ID:                "a-1",
		FormatCode:        "T20",
		BuyerOppositionID: 1,
		SquadSize:         4,
		MinBowlers:        2,
		RequireKeeper:     true,
		Players: []auction.ListedPlayer{
			{PlayerID: 1, State: auction.StateSold, BuyerOppositionID: 1, BuyerName: "Buyer", Price: &price},
			{PlayerID: 2, State: auction.StateSold, BuyerOppositionID: 2, BuyerName: "Rival", Price: &price},
			{PlayerID: 3, State: auction.StateAvailable},
			{PlayerID: 4, State: auction.StateAvailable},
			{PlayerID: 5, State: auction.StateAvailable},
			{PlayerID: 6, State: auction.StateAvailable},
		},
	}
	roles := map[int64]auction.PlayerRoles{
		1: {Known: true, Keeper: true},
		2: {Known: true, BowlingOption: true},
		3: {Known: true, BowlingOption: true},
		4: {Known: true, Keeper: true, BowlingOption: true},
		5: {Known: true},
		6: {Known: false},
	}
	return record, roles
}

func TestSquadOf_IsTheSoldRowsBoughtByTheAuctionsOwnSide(t *testing.T) {
	t.Parallel()
	record, _ := listedAuction()

	squad := auction.SquadOf(record)

	require.Len(t, squad.Players, 1)
	assert.Equal(t, int64(1), squad.Players[0].PlayerID,
		"a rival's purchase is a fact the operator recorded, not a place in this buyer's squad")
}

func TestSlotsFor_CountsTheOpenPlacesAndTheConstraintsTheyMustStillSatisfy(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()

	slots := auction.SlotsFor(record, roles)

	assert.Equal(t, 4, slots.SquadSize)
	assert.Equal(t, 1, slots.Filled)
	assert.Equal(t, 3, slots.Open)
	require.NotNil(t, slots.ByRole)
	assert.Equal(t, 1, slots.ByRole.Keepers)
	assert.False(t, slots.ByRole.KeeperNeeded, "the squad already holds the keeper the eleven requires")
	assert.Equal(t, 0, slots.ByRole.BowlingOptions)
	assert.Equal(t, 2, slots.ByRole.BowlingOptionsShort, "two were asked for and none is bought")
}

func TestSlotsFor_WithNoRoleReadCountsThePlacesAndGuessesNoConstraint(t *testing.T) {
	t.Parallel()
	record, _ := listedAuction()

	slots := auction.SlotsFor(record, nil)

	assert.Equal(t, 3, slots.Open, "squad size minus the squad is arithmetic over the record alone")
	assert.Nil(t, slots.ByRole,
		"which places still need a keeper is the model's answer, and a refused read shows no number")
}

func TestSlotsFor_CannotReportMorePlacesThanTheSquadHolds(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()
	record.SquadSize = 1

	slots := auction.SlotsFor(record, roles)

	assert.Equal(t, 0, slots.Open, "a squad already at its size has no open places, and never a negative count")
}

func TestSlotsFor_CountsASquadMemberTheStateHasNeverSeenApartFromBothRoles(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()
	record.Players[5].State = auction.StateSold
	record.Players[5].BuyerOppositionID = 1
	record.Players[5].BuyerName = "Buyer"

	slots := auction.SlotsFor(record, roles)

	require.NotNil(t, slots.ByRole)
	assert.Equal(t, 1, slots.ByRole.UnknownRoles,
		"a squad that looks a bowler short may not be, and this is the number that says so")
	assert.Equal(t, 0, slots.ByRole.BowlingOptions,
		"no role is invented for a player the model has never read")
}

func TestDistributionOf_CountsTheStillAvailablePlayersByThePredicatesTheModelReads(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()

	distribution := auction.DistributionOf(record, roles)

	assert.Equal(t, auction.Distribution{
		Available: 4, Keepers: 1, BowlingOptions: 2, Batters: 1, Unknown: 1,
	}, distribution)
}

func TestDistributionOf_CountsAKeeperWhoAlsoBowlsInBoth(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()

	distribution := auction.DistributionOf(record, roles)

	assert.Equal(t, 1+2, distribution.Keepers+distribution.BowlingOptions,
		"the two counts overlap because they are two independent predicates and not a partition")
	assert.Equal(t, distribution.Available, distribution.Batters+distribution.Unknown+3-1,
		"only the batter and unknown counts are exclusive of everything else")
}

func TestDistributionOf_MovesWhenASaleIsRecorded(t *testing.T) {
	t.Parallel()
	record, roles := listedAuction()
	before := auction.DistributionOf(record, roles)
	price := int64(700)
	record.Players[3].State = auction.StateSold
	record.Players[3].BuyerOppositionID = 2
	record.Players[3].BuyerName = "Rival"
	record.Players[3].Price = &price

	after := auction.DistributionOf(record, roles)

	assert.Equal(t, before.Available-1, after.Available)
	assert.Equal(t, before.Keepers-1, after.Keepers, "the keeper who was sold is off the remaining pool")
	assert.Equal(t, before.BowlingOptions-1, after.BowlingOptions, "and he was a bowling option too")
}
