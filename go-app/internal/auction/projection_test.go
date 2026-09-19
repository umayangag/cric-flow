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

// The projection's rules and its one piece of arithmetic (P3-2).
//
// The mixture is the part worth testing hardest: it is the only number this module
// computes rather than relays, and the whole reason it is computed from draws is that the
// obvious wrong answer — averaging the grounds' quantiles — is arithmetic nobody would
// notice was wrong. So it is checked against draws small enough to invert by hand.

// namedPlayers builds a list of n players with registry ids, for the eleven rules.
func namedPlayers(n int) []auction.NamedPlayer {
	players := make([]auction.NamedPlayer, 0, n)
	for i := 1; i <= n; i++ {
		players = append(players, auction.NamedPlayer{
			PlayerID:   int64(i),
			ExternalID: "reg" + string(rune('a'+i-1)),
			PlayerName: "Player " + string(rune('A'+i-1)),
		})
	}
	return players
}

func TestIntervalSources_AreTheOnesTheContractPublishes(t *testing.T) {
	t.Parallel()
	var contract struct {
		AuctionIntervalSources []string `json:"auction_interval_sources"`
	}
	raw, err := os.ReadFile(filepath.Clean(contractPath))
	require.NoError(t, err, "contract file missing; regenerate it with TestPipelineContract -update")
	require.NoError(t, json.Unmarshal(raw, &contract))

	assert.Equal(t, auction.IntervalSources(), contract.AuctionIntervalSources,
		"an interval names where it came from, and both sides spell the source the same way")
	assert.Equal(t, []string{"l2b_quantiles", "simulator_draws"}, auction.IntervalSources())
}

func TestProjectedEleven_TakesTheCandidateIntoTheOpenPlace(t *testing.T) {
	t.Parallel()
	candidate := auction.NamedPlayer{PlayerID: 99, ExternalID: "reg99", PlayerName: "Candidate"}

	eleven, err := auction.ProjectedEleven(namedPlayers(10), candidate)

	require.NoError(t, err)
	require.Len(t, eleven, 11)
	assert.Equal(t, candidate, eleven[10], "the candidate fills the eleventh place")
}

func TestProjectedEleven_ProjectsAnElevenThatAlreadyNamesHimAsItStands(t *testing.T) {
	t.Parallel()
	likely := namedPlayers(11)

	eleven, err := auction.ProjectedEleven(likely, likely[3])

	require.NoError(t, err)
	assert.Equal(t, likely, eleven,
		"an operator who has already put the candidate in his likely eleven gets that eleven, not twelve men")
}

func TestProjectedEleven_RefusesASideThatIsNotAnEleven(t *testing.T) {
	t.Parallel()
	outsider := auction.NamedPlayer{PlayerID: 99, ExternalID: "reg99", PlayerName: "Candidate"}

	testCases := []struct {
		name     string
		likelyXI []auction.NamedPlayer
		wantHave int
	}{
		{name: "nine named and the candidate is a ten-man side", likelyXI: namedPlayers(9), wantHave: 10},
		{name: "a full eleven plus an outsider is twelve men", likelyXI: namedPlayers(11), wantHave: 12},
		{name: "an empty likely eleven is one man", likelyXI: nil, wantHave: 1},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			eleven, err := auction.ProjectedEleven(testCase.likelyXI, outsider)

			assert.Nil(t, eleven, "a refused eleven is not a shorter eleven; nothing is scored")
			var incomplete *auction.IncompleteElevenError
			require.ErrorAs(t, err, &incomplete)
			assert.Equal(t, testCase.wantHave, incomplete.Have)
			assert.Equal(t, 11, incomplete.Need)
		})
	}
}

// TestRefuseSharedPlayers_RefusesAProjectionWhoseTwoElevensShareAPlayer: both elevens are
// the operator's own, so the module has nothing to pick a side with and refuses rather than
// dropping him from one of them (GO-04, §8.7).
func TestRefuseSharedPlayers_RefusesAProjectionWhoseTwoElevensShareAPlayer(t *testing.T) {
	t.Parallel()
	eleven := namedPlayers(11)
	opposition := oppositionEleven()
	opposition[4] = eleven[2]

	err := auction.RefuseSharedPlayers(eleven, opposition)

	var shared *auction.SharedPlayerError
	require.ErrorAs(t, err, &shared)
	assert.Equal(t, []auction.NamedPlayer{eleven[2]}, shared.Players)
	assert.Contains(t, shared.Error(), "nobody plays both sides")
	assert.Contains(t, shared.Error(), eleven[2].PlayerName)
}

func TestRefuseSharedPlayers_AcceptsTwoDifferentElevens(t *testing.T) {
	t.Parallel()

	err := auction.RefuseSharedPlayers(namedPlayers(11), oppositionEleven())

	assert.NoError(t, err)
}

// oppositionEleven is an eleven of its own, sharing no player with namedPlayers.
func oppositionEleven() []auction.NamedPlayer {
	players := make([]auction.NamedPlayer, 0, 11)
	for i := 1; i <= 11; i++ {
		players = append(players, auction.NamedPlayer{
			PlayerID:   int64(100 + i),
			ExternalID: "opp" + string(rune('a'+i-1)),
			PlayerName: "Opponent " + string(rune('A'+i-1)),
		})
	}
	return players
}

func TestRegistryKeys_RefusesASideHoldingAPlayerTheRegistryDoesNotKnow(t *testing.T) {
	t.Parallel()
	side := namedPlayers(3)
	side[1].ExternalID = ""

	keys, err := auction.RegistryKeys(side, "the likely eleven")

	assert.Nil(t, keys, "the two who resolved are not scored: that would answer for a side nobody named")
	var unregistered *auction.UnregisteredPlayerError
	require.ErrorAs(t, err, &unregistered)
	assert.Equal(t, []int64{2}, unregistered.PlayerIDs)
	assert.Contains(t, unregistered.Error(), "the likely eleven")
}

func TestAssumptionsChangeValidate_RefusesWhatCouldNotBeProjectedUnder(t *testing.T) {
	t.Parallel()
	elevenIDs := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	tenIDs := elevenIDs[:10]
	twelveIDs := append(append([]int64(nil), elevenIDs...), 12)
	repeated := []int64{1, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11}

	testCases := []struct {
		name    string
		change  auction.AssumptionsChange
		wantErr string
	}{
		{
			name:   "ten named with a place open for the candidate is the usual state",
			change: auction.AssumptionsChange{LikelyXIPlayerIDs: &tenIDs},
		},
		{
			name:   "eleven named is the other one",
			change: auction.AssumptionsChange{LikelyXIPlayerIDs: &elevenIDs},
		},
		{
			name: "the opposition is a whole eleven and a side",
			change: auction.AssumptionsChange{
				Opposition: &auction.OppositionChange{OppositionID: 7, PlayerIDs: elevenIDs},
			},
		},
		{
			name:    "a likely eleven of twelve is not an eleven any candidate joins",
			change:  auction.AssumptionsChange{LikelyXIPlayerIDs: &twelveIDs},
			wantErr: "at most 11",
		},
		{
			name:    "one player fills one place",
			change:  auction.AssumptionsChange{LikelyXIPlayerIDs: &repeated},
			wantErr: "twice",
		},
		{
			name: "an opposition with no side would make every ground read alike",
			change: auction.AssumptionsChange{
				Opposition: &auction.OppositionChange{PlayerIDs: elevenIDs},
			},
			wantErr: "the opposition names a side",
		},
		{
			name: "a ten-man opposition is a side nobody plays",
			change: auction.AssumptionsChange{
				Opposition: &auction.OppositionChange{OppositionID: 7, PlayerIDs: tenIDs},
			},
			wantErr: "the opposition is an eleven",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.change.Validate()

			if testCase.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.wantErr)
		})
	}
}

func TestMixtureQuantiles_InvertsThePooledDrawsAndNotTheGroundsQuantiles(t *testing.T) {
	t.Parallel()
	// Two grounds, five draws each, weighted 3:1. Pooled masses are 0.15 a draw at the
	// first ground and 0.05 at the second, so the cumulative mass over the sorted pool is:
	//
	//   100 .15 | 110 .30 | 120 .45 | 130 .60 | 140 .75 | 200 .80 | 210 .85 | 220 .90 |
	//   230 .95 | 240 1.00
	//
	// q10 is the first value reaching 0.10 (100), the median the first reaching 0.50 (130)
	// and q90 the first reaching 0.90 (220). Averaging the two grounds' own q90s would give
	// (140 + 240) / 2 = 190, a total neither ground drew and the mixture does not have.
	components := []auction.MixtureComponent{
		{Draws: []float64{100, 110, 120, 130, 140}, Weight: 0.75},
		{Draws: []float64{200, 210, 220, 230, 240}, Weight: 0.25},
	}

	quantiles, err := auction.MixtureQuantiles(components)

	require.NoError(t, err)
	assert.Equal(t, 100.0, quantiles.Q10)
	assert.Equal(t, 130.0, quantiles.Median)
	assert.Equal(t, 220.0, quantiles.Q90)
	assert.NotEqual(t, 190.0, quantiles.Q90,
		"the mixture's q90 is not the mean of its parts' q90s, which is why the draws are pooled")
}

func TestMixtureQuantiles_HandlesTheEdgesTheOperatorCanReach(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		components []auction.MixtureComponent
		want       auction.Quantiles
		wantErr    error
	}{
		{
			name:       "one ground at full weight is that ground's own distribution",
			components: []auction.MixtureComponent{{Draws: []float64{10, 20, 30, 40, 50}, Weight: 1}},
			want:       auction.Quantiles{Q10: 10, Median: 30, Q90: 50},
		},
		{
			name: "weights need not sum to one: they are normalised by their total",
			components: []auction.MixtureComponent{
				{Draws: []float64{10, 20, 30, 40, 50}, Weight: 3},
				{Draws: []float64{10, 20, 30, 40, 50}, Weight: 1},
			},
			want: auction.Quantiles{Q10: 10, Median: 30, Q90: 50},
		},
		{
			name: "a ground weighted zero is not in the mixture at all",
			components: []auction.MixtureComponent{
				{Draws: []float64{10, 20, 30, 40, 50}, Weight: 1},
				{Draws: []float64{900, 900, 900, 900, 900}, Weight: 0},
			},
			want: auction.Quantiles{Q10: 10, Median: 30, Q90: 50},
		},
		{
			name:       "nothing to pool is refused rather than answered with zeroes",
			components: []auction.MixtureComponent{{Draws: nil, Weight: 1}},
			wantErr:    auction.ErrEmptyMixture,
		},
		{
			name:       "every weight zero is refused too",
			components: []auction.MixtureComponent{{Draws: []float64{1, 2}, Weight: 0}},
			wantErr:    auction.ErrEmptyMixture,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			quantiles, err := auction.MixtureQuantiles(testCase.components)

			if testCase.wantErr != nil {
				assert.ErrorIs(t, err, testCase.wantErr)
				assert.Equal(t, auction.Quantiles{}, quantiles)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.want, quantiles)
		})
	}
}
