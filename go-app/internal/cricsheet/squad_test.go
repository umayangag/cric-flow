package cricsheet

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestSquadFromInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		info        Info
		wantMembers []SquadMember
		wantErr     string
		wantNoSquad bool
	}{
		{
			name: "both teams in info.teams order regardless of map order",
			info: Info{
				Teams: []string{"Australia", "South Africa"},
				Players: map[string][]string{
					"South Africa": {"SC Cook", "D Elgar"},
					"Australia":    {"DA Warner", "SE Marsh"},
				},
			},
			wantMembers: []SquadMember{
				{Team: "Australia", Player: "DA Warner"},
				{Team: "Australia", Player: "SE Marsh"},
				{Team: "South Africa", Player: "SC Cook"},
				{Team: "South Africa", Player: "D Elgar"},
			},
		},
		{
			name: "a twelfth player is kept, not trimmed to eleven",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}},
			},
			wantMembers: []SquadMember{
				{Team: "India", Player: "a"},
				{Team: "India", Player: "b"},
				{Team: "India", Player: "c"},
				{Team: "India", Player: "d"},
				{Team: "India", Player: "e"},
				{Team: "India", Player: "f"},
				{Team: "India", Player: "g"},
				{Team: "India", Player: "h"},
				{Team: "India", Player: "i"},
				{Team: "India", Player: "j"},
				{Team: "India", Player: "k"},
				{Team: "India", Player: "l"},
			},
		},
		{
			name: "names are trimmed",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {"  V Kohli  "}},
			},
			wantMembers: []SquadMember{{Team: "India", Player: "V Kohli"}},
		},
		{
			name: "a team missing from info.teams is still recorded",
			info: Info{
				Teams: []string{"India"},
				Players: map[string][]string{
					"India":     {"V Kohli"},
					"Sri Lanka": {"K Mendis"},
				},
			},
			wantMembers: []SquadMember{
				{Team: "India", Player: "V Kohli"},
				{Team: "Sri Lanka", Player: "K Mendis"},
			},
		},
		{
			name:        "no info.players is a named condition",
			info:        Info{Teams: []string{"India", "Sri Lanka"}},
			wantNoSquad: true,
		},
		{
			name: "an empty player list is the same as no squad",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {}},
			},
			wantNoSquad: true,
		},
		{
			name: "an empty player name is refused",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {"V Kohli", "  "}},
			},
			wantErr: `team "India" lists an empty player name`,
		},
		{
			name: "the same player twice in one team is refused",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {"V Kohli", "V Kohli"}},
			},
			wantErr: `player "V Kohli" is listed twice for team "India"`,
		},
		{
			name: "a player on both teams is refused",
			info: Info{
				Teams: []string{"India", "Sri Lanka"},
				Players: map[string][]string{
					"India":     {"V Kohli"},
					"Sri Lanka": {"V Kohli"},
				},
			},
			wantErr: `player "V Kohli" is listed for both "India" and "Sri Lanka"`,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got, err := SquadFromInfo(tc.info)

			// Assert
			switch {
			case tc.wantNoSquad:
				require.ErrorIs(t, err, ErrNoSquad)
				assert.Nil(t, got)
			case tc.wantErr != "":
				require.Error(t, err)
				assert.EqualError(t, err, tc.wantErr)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.wantMembers, got)
			}
		})
	}
}

// stubSquadResolver hands back deterministic IDs and can be told to fail for one name.
type stubSquadResolver struct {
	playerIDs     map[string]int64
	oppositionIDs map[string]int64
	failPlayer    string
	failTeam      string
	playerCalls   int
	teamCalls     int
}

var errStubResolver = errors.New("resolver unavailable")

func (s *stubSquadResolver) GetPlayerID(_ context.Context, name string) (int64, error) {
	s.playerCalls++
	if name == s.failPlayer {
		return 0, errStubResolver
	}
	return s.playerIDs[name], nil
}

func (s *stubSquadResolver) GetOppositionID(_ context.Context, name string) (int64, error) {
	s.teamCalls++
	if name == s.failTeam {
		return 0, errStubResolver
	}
	return s.oppositionIDs[name], nil
}

func TestBuildMatchPlayerRows(t *testing.T) {
	t.Parallel()

	info := Info{
		Teams: []string{"India", "Sri Lanka"},
		Players: map[string][]string{
			"India":     {"V Kohli", "R Sharma"},
			"Sri Lanka": {"K Mendis"},
		},
	}

	t.Run("resolves every member to its own team", func(t *testing.T) {
		t.Parallel()
		// Arrange
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, info, 99, "f.json", "2024-01-01")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []db.MatchPlayer{
			{MatchID: 99, PlayerID: 1, OppositionID: 10},
			{MatchID: 99, PlayerID: 2, OppositionID: 10},
			{MatchID: 99, PlayerID: 3, OppositionID: 20},
		}, rows)
	})

	t.Run("resolves each team once", func(t *testing.T) {
		t.Parallel()
		// Arrange
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		_, err := buildMatchPlayerRows(context.Background(), resolver, info, 99, "f.json", "2024-01-01")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 2, resolver.teamCalls, "one lookup per team, not per player")
		assert.Equal(t, 3, resolver.playerCalls)
	})

	t.Run("a match with no squad yields no rows and no error", func(t *testing.T) {
		t.Parallel()
		// Arrange
		resolver := &stubSquadResolver{}

		// Act
		rows, err := buildMatchPlayerRows(
			context.Background(), resolver, Info{Teams: []string{"India"}}, 99, "f.json", "2024-01-01",
		)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Zero(t, resolver.playerCalls, "nothing to resolve")
	})

	t.Run("an unusable squad fails the import rather than recording a partial one", func(t *testing.T) {
		t.Parallel()
		// Arrange
		bad := Info{Teams: []string{"India"}, Players: map[string][]string{"India": {"V Kohli", "V Kohli"}}}
		resolver := &stubSquadResolver{}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, bad, 99, "f.json", "2024-01-01")

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read info.players")
		assert.Nil(t, rows)
	})

	t.Run("a failed player lookup is returned, not skipped", func(t *testing.T) {
		t.Parallel()
		// Arrange
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
			failPlayer:    "R Sharma",
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, info, 99, "f.json", "2024-01-01")

		// Assert
		require.ErrorIs(t, err, errStubResolver)
		assert.Contains(t, err.Error(), `"R Sharma"`)
		assert.Nil(t, rows)
	})

	t.Run("a failed team lookup is returned, not skipped", func(t *testing.T) {
		t.Parallel()
		// Arrange
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10},
			failTeam:      "Sri Lanka",
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, info, 99, "f.json", "2024-01-01")

		// Assert
		require.ErrorIs(t, err, errStubResolver)
		assert.Contains(t, err.Error(), `"Sri Lanka"`)
		assert.Nil(t, rows)
	})
}
