package selectionbacktest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
	sb "github.com/umayangag/cric-flow/go-app/internal/services/selectionbacktest"
)

func greedyArm() sb.Arm {
	return sb.Arm{Name: "greedy", Mode: predictteam.SelectionModeGreedy}
}

func winProbArm() sb.Arm {
	return sb.Arm{Name: "winprob", Mode: predictteam.SelectionModeWinProbability}
}

func match(id int64, winner string, fielded ...int64) sb.Match {
	return sb.Match{
		MatchID:          id,
		Format:           "T20",
		Team1:            "A",
		Team2:            "B",
		ActualWinner:     winner,
		FieldedPlayerIDs: fielded,
	}
}

func TestRun_SelectsEveryMatchUnderEveryArm(t *testing.T) {
	t.Parallel()

	// Arrange
	matches := []sb.Match{match(1, "A"), match(2, "B")}
	arms := []sb.Arm{greedyArm(), winProbArm()}
	var seen []predictteam.SelectionMode
	selector := func(_ context.Context, _ sb.Match, mode predictteam.SelectionMode) (sb.ArmSelection, error) {
		seen = append(seen, mode)
		return sb.ArmSelection{Team1WinProbability: 0.5}, nil
	}

	// Act
	selections := sb.Run(context.Background(), matches, arms, selector)

	// Assert
	require.Len(t, selections, 4)
	assert.Equal(t, []predictteam.SelectionMode{
		predictteam.SelectionModeGreedy, predictteam.SelectionModeWinProbability,
		predictteam.SelectionModeGreedy, predictteam.SelectionModeWinProbability,
	}, seen, "arms run per match, so a run stopped halfway still holds a comparable sample")
}

func TestRun_RecordsAFailureRatherThanAbandoningTheBacktest(t *testing.T) {
	t.Parallel()

	// Arrange
	matches := []sb.Match{match(1, "A"), match(2, "B")}
	selector := func(_ context.Context, m sb.Match, _ predictteam.SelectionMode) (sb.ArmSelection, error) {
		if m.MatchID == 1 {
			return sb.ArmSelection{}, errors.New("no squad for this match")
		}
		return sb.ArmSelection{Team1WinProbability: 0.7}, nil
	}

	// Act
	selections := sb.Run(context.Background(), matches, []sb.Arm{greedyArm()}, selector)

	// Assert
	require.Len(t, selections, 2)
	require.Error(t, selections[0].Err)
	require.NoError(t, selections[1].Err)
}

func TestRun_StopsWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	selector := func(context.Context, sb.Match, predictteam.SelectionMode) (sb.ArmSelection, error) {
		t.Fatal("selector must not run after cancellation")
		return sb.ArmSelection{}, nil
	}

	// Act
	selections := sb.Run(ctx, []sb.Match{match(1, "A")}, []sb.Arm{greedyArm()}, selector)

	// Assert
	assert.Empty(t, selections)
}

func TestSummarize_ScoresWinnerAccuracyAgainstTheRealResult(t *testing.T) {
	t.Parallel()

	// Arrange: two right, one wrong.
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, "A"), Selection: sb.ArmSelection{PredictedWinner: "A"}},
		{MatchID: 2, Arm: "greedy", Match: match(2, "B"), Selection: sb.ArmSelection{PredictedWinner: "B"}},
		{MatchID: 3, Arm: "greedy", Match: match(3, "A"), Selection: sb.ArmSelection{PredictedWinner: "B"}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	require.Len(t, report.Arms, 1)
	require.NotNil(t, report.Arms[0].WinnerAccuracy)
	assert.InDelta(t, 2.0/3.0, *report.Arms[0].WinnerAccuracy, 1e-9)
	assert.Equal(t, 3, report.Arms[0].DecidedMatches)
}

func TestSummarize_ExcludesUndecidedMatchesFromAccuracy(t *testing.T) {
	t.Parallel()

	// Arrange: a match with no result and one where the model had no preference.
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, ""), Selection: sb.ArmSelection{PredictedWinner: "A"}},
		{MatchID: 2, Arm: "greedy", Match: match(2, "B"), Selection: sb.ArmSelection{PredictedWinner: ""}},
		{MatchID: 3, Arm: "greedy", Match: match(3, "A"), Selection: sb.ArmSelection{PredictedWinner: "A"}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Equal(t, 1, report.Arms[0].DecidedMatches, "a no-result match cannot score a prediction")
	assert.Equal(t, 3, report.Arms[0].Matches)
}

func TestSummarize_LeavesAccuracyUnsetWhenNothingWasDecided(t *testing.T) {
	t.Parallel()

	// Arrange
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, ""), Selection: sb.ArmSelection{}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert: nil, not a misleading 0.0.
	assert.Nil(t, report.Arms[0].WinnerAccuracy)
}

func TestSummarize_CountsFailuresSeparatelyFromMatches(t *testing.T) {
	t.Parallel()

	// Arrange
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, "A"), Err: errors.New("boom")},
		{MatchID: 2, Arm: "greedy", Match: match(2, "A"), Selection: sb.ArmSelection{Team1WinProbability: 0.8}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Equal(t, 1, report.Arms[0].Failed)
	assert.Equal(t, 1, report.Arms[0].Matches)
	require.NotNil(t, report.Arms[0].MeanTeam1WinProbability)
	assert.InDelta(t, 0.8, *report.Arms[0].MeanTeam1WinProbability, 1e-9,
		"a failed match must not be averaged in as a zero")
}

func TestSummarize_ReportsWhenTheTwoArmsPickTheSameTeam(t *testing.T) {
	t.Parallel()

	// Arrange: identical selections. If the optimiser returns the greedy XI, the search
	// is not doing anything, and no other metric changes that.
	selections := []sb.Selection{
		{
			MatchID:   1,
			Arm:       "greedy",
			Match:     match(1, "A"),
			Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1, 2, 3}},
		},
		{
			MatchID:   1,
			Arm:       "winprob",
			Match:     match(1, "A"),
			Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{3, 2, 1}},
		},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	require.Len(t, report.Divergences, 1)
	assert.Equal(t, 1, report.Divergences[0].Matches)
	assert.Equal(t, 1, report.Divergences[0].IdenticalXIs, "order must not count as a difference")
	assert.Zero(t, report.Divergences[0].MeanDifferentPlayers)
}

func TestSummarize_CountsPlayersTheArmsDisagreeOn(t *testing.T) {
	t.Parallel()

	// Arrange: one player swapped each way.
	selections := []sb.Selection{
		{
			MatchID:   1,
			Arm:       "greedy",
			Match:     match(1, "A"),
			Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1, 2, 3}},
		},
		{
			MatchID:   1,
			Arm:       "winprob",
			Match:     match(1, "A"),
			Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1, 2, 4}},
		},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Equal(t, 0, report.Divergences[0].IdenticalXIs)
	assert.InDelta(t, 2.0, report.Divergences[0].MeanDifferentPlayers, 1e-9, "3 is gone and 4 is new")
}

func TestSummarize_DivergenceIgnoresMatchesOnlyOneArmSelected(t *testing.T) {
	t.Parallel()

	// Arrange: winprob failed on match 1, so there is nothing to compare there.
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, "A"), Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1}}},
		{MatchID: 1, Arm: "winprob", Match: match(1, "A"), Err: errors.New("boom")},
		{MatchID: 2, Arm: "greedy", Match: match(2, "A"), Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1}}},
		{MatchID: 2, Arm: "winprob", Match: match(2, "A"), Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{2}}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Equal(t, 1, report.Divergences[0].Matches)
}

func TestSummarize_ReportsOverlapWithTheFieldedXI(t *testing.T) {
	t.Parallel()

	// Arrange: picked two of the four who actually played.
	selections := []sb.Selection{
		{
			MatchID:   1,
			Arm:       "greedy",
			Match:     match(1, "A", 1, 2, 3, 4),
			Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1, 2, 9}},
		},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	require.NotNil(t, report.Arms[0].MeanFieldedOverlap)
	assert.InDelta(t, 0.5, *report.Arms[0].MeanFieldedOverlap, 1e-9)
}

func TestSummarize_LeavesOverlapUnsetWhenTheFieldedXIIsUnknown(t *testing.T) {
	t.Parallel()

	// Arrange
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, "A"), Selection: sb.ArmSelection{SelectedPlayerIDs: []int64{1}}},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Nil(t, report.Arms[0].MeanFieldedOverlap)
}

func TestSummarize_CountsDistinctMatches(t *testing.T) {
	t.Parallel()

	// Arrange: two arms over two matches is two matches, not four.
	selections := []sb.Selection{
		{MatchID: 1, Arm: "greedy", Match: match(1, "A")},
		{MatchID: 1, Arm: "winprob", Match: match(1, "A")},
		{MatchID: 2, Arm: "greedy", Match: match(2, "A")},
		{MatchID: 2, Arm: "winprob", Match: match(2, "A")},
	}

	// Act
	report := sb.Summarize(selections)

	// Assert
	assert.Equal(t, 2, report.Matches)
	assert.Equal(t, []string{"greedy", "winprob"}, sb.SortedArmNames(report))
}
