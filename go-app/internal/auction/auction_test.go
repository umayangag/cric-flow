package auction_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
)

// contractPath is the repo-root contract every side of a wire literal is asserted against
// (H-24).
const contractPath = "../../../contracts/ops-console.contract.json"

func TestStates_AreTheThreeAnAuctionHas(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"available", "sold", "unsold"}, auction.States())
	assert.True(t, auction.IsKnownState(auction.StateSold))
	assert.False(t, auction.IsKnownState("withdrawn"),
		"a state this record cannot hold is refused rather than stored and rendered as a blank")
}

func TestStatesAndMetricKeys_AreTheOnesTheContractPublishes(t *testing.T) {
	t.Parallel()
	var contract struct {
		AuctionPlayerStates []string `json:"auction_player_states"`
		AuctionMetricKeys   []string `json:"auction_metric_keys"`
	}
	raw, err := os.ReadFile(filepath.Clean(contractPath))
	require.NoError(t, err, "contract file missing; regenerate it with TestPipelineContract -update")
	require.NoError(t, json.Unmarshal(raw, &contract))

	assert.Equal(t, auction.States(), contract.AuctionPlayerStates,
		"the state vocabulary is declared once and matched from every side that reads it")
	assert.Equal(t, auction.MetricKeys(), contract.AuctionMetricKeys,
		"every labelled number on the Auction tab opens an L-1 explainer under one of these keys")
}

func TestNewID_MintsADistinctIdentifierPerAuction(t *testing.T) {
	t.Parallel()

	first, second := auction.NewID(), auction.NewID()

	assert.NotEmpty(t, first)
	assert.NotEqual(t, first, second)
}

func TestOutcomeValidate_AcceptsTheThreeEntriesAnOperatorMakes(t *testing.T) {
	t.Parallel()
	price := int64(1200)
	zero := int64(0)

	testCases := []struct {
		name    string
		outcome auction.Outcome
	}{
		{
			name: "a sale names the buyer and the price",
			outcome: auction.Outcome{
				PlayerID: 7, State: auction.StateSold, BuyerName: "Rival", Price: &price,
			},
		},
		{
			name: "a sale at the base price is still a sale",
			outcome: auction.Outcome{
				PlayerID: 7, State: auction.StateSold, BuyerName: "Rival", Price: &zero,
			},
		},
		{
			name:    "an unsold outcome carries neither buyer nor price",
			outcome: auction.Outcome{PlayerID: 7, State: auction.StateUnsold},
		},
		{
			name:    "an undo puts him back with nothing attached",
			outcome: auction.Outcome{PlayerID: 7, State: auction.StateAvailable},
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.NoError(t, testCase.outcome.Validate())
		})
	}
}

func TestOutcomeValidate_RefusesEveryShapeThatIsNotAStateOfAnAuction(t *testing.T) {
	t.Parallel()
	price := int64(1200)
	negative := int64(-1)

	testCases := []struct {
		name    string
		outcome auction.Outcome
		wantErr string
	}{
		{
			name:    "an outcome names a player",
			outcome: auction.Outcome{State: auction.StateUnsold},
			wantErr: "an outcome names a player",
		},
		{
			name:    "a state the record cannot hold is refused by name",
			outcome: auction.Outcome{PlayerID: 7, State: "withdrawn"},
			wantErr: `state "withdrawn" is not one of available, sold, unsold`,
		},
		{
			name:    "a sale with no buyer is half a sale",
			outcome: auction.Outcome{PlayerID: 7, State: auction.StateSold, Price: &price},
			wantErr: "a sale names the buyer",
		},
		{
			name:    "a sale with no price is the other half",
			outcome: auction.Outcome{PlayerID: 7, State: auction.StateSold, BuyerName: "Rival"},
			wantErr: "a sale names the price",
		},
		{
			name: "nobody pays a negative price",
			outcome: auction.Outcome{
				PlayerID: 7, State: auction.StateSold, BuyerName: "Rival", Price: &negative,
			},
			wantErr: "a price is not negative",
		},
		{
			name: "an undo that still carries a price is two contradictory facts",
			outcome: auction.Outcome{
				PlayerID: 7, State: auction.StateAvailable, Price: &price,
			},
			wantErr: `an outcome of "available" carries no buyer and no price`,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.outcome.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wantErr)
		})
	}
}
