package cricsheet

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// emptyToNil lets a case omit wantAmbiguous when nothing is contested.
func emptyToNil(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func TestSquadFromInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		info          Info
		wantMembers   []SquadMember
		wantAmbiguous []string
		wantErr       string
		wantNoSquad   bool
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
			// The side is not in doubt, so the player is kept once rather than dropped.
			name: "the same player twice in one team is deduplicated",
			info: Info{
				Teams:   []string{"India"},
				Players: map[string][]string{"India": {"V Kohli", "V Kohli"}},
			},
			wantMembers: []SquadMember{{Team: "India", Player: "V Kohli"}},
		},
		{
			// Two people sharing a scorecard name. Cricsheet's registry collapses them
			// into one identifier, so which side each played for is unknowable.
			name: "a player named on both teams is dropped from both",
			info: Info{
				Teams: []string{"India", "Sri Lanka"},
				Players: map[string][]string{
					"India":     {"V Kohli", "R Sharma"},
					"Sri Lanka": {"V Kohli", "K Mendis"},
				},
			},
			wantMembers: []SquadMember{
				{Team: "India", Player: "R Sharma"},
				{Team: "Sri Lanka", Player: "K Mendis"},
			},
			wantAmbiguous: []string{"V Kohli"},
		},
		{
			name: "a squad of only contested names is the same as no squad",
			info: Info{
				Teams: []string{"India", "Sri Lanka"},
				Players: map[string][]string{
					"India":     {"J Butler"},
					"Sri Lanka": {"J Butler"},
				},
			},
			wantNoSquad:   true,
			wantAmbiguous: []string{"J Butler"},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got, ambiguous, err := SquadFromInfo(tc.info)

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
			assert.Equal(t, tc.wantAmbiguous, emptyToNil(ambiguous))
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

func (s *stubSquadResolver) PlayerID(_ context.Context, name string) (int64, error) {
	s.playerCalls++
	if name == s.failPlayer {
		return 0, errStubResolver
	}
	return s.playerIDs[name], nil
}

func (s *stubSquadResolver) OppositionID(_ context.Context, name string) (int64, error) {
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
		rows, err := buildMatchPlayerRows(
			context.Background(),
			resolver,
			&Match{Info: info},
			99,
			"f.json",
			"2024-01-01",
		)

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
		_, err := buildMatchPlayerRows(context.Background(), resolver, &Match{Info: info}, 99, "f.json", "2024-01-01")

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
			context.Background(), resolver, &Match{Info: Info{Teams: []string{"India"}}}, 99, "f.json", "2024-01-01",
		)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Zero(t, resolver.playerCalls, "nothing to resolve")
	})

	t.Run("an empty player name fails the import rather than recording a partial squad", func(t *testing.T) {
		t.Parallel()
		// Arrange
		bad := Info{Teams: []string{"India"}, Players: map[string][]string{"India": {"V Kohli", " "}}}
		resolver := &stubSquadResolver{}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, &Match{Info: bad}, 99, "f.json", "2024-01-01")

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read info.players")
		assert.Nil(t, rows)
	})

	t.Run("a player named on both teams costs one row, not the match", func(t *testing.T) {
		t.Parallel()
		// Arrange
		namesake := Info{
			Teams: []string{"India", "Sri Lanka"},
			Players: map[string][]string{
				"India":     {"V Kohli", "R Sharma"},
				"Sri Lanka": {"V Kohli", "K Mendis"},
			},
		}
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		rows, err := buildMatchPlayerRows(
			context.Background(),
			resolver,
			&Match{Info: namesake},
			99,
			"f.json",
			"2024-01-01",
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []db.MatchPlayer{
			{MatchID: 99, PlayerID: 2, OppositionID: 10},
			{MatchID: 99, PlayerID: 3, OppositionID: 20},
		}, rows)
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
		rows, err := buildMatchPlayerRows(
			context.Background(),
			resolver,
			&Match{Info: info},
			99,
			"f.json",
			"2024-01-01",
		)

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
		rows, err := buildMatchPlayerRows(
			context.Background(),
			resolver,
			&Match{Info: info},
			99,
			"f.json",
			"2024-01-01",
		)

		// Assert
		require.ErrorIs(t, err, errStubResolver)
		assert.Contains(t, err.Error(), `"Sri Lanka"`)
		assert.Nil(t, rows)
	})

	t.Run("the man who came in is flagged and the eleven who started are not", func(t *testing.T) {
		t.Parallel()
		// Arrange: a twelve-man list whose twelfth, R Sharma, came in for V Kohli mid-match.
		match := &Match{
			Info: info,
			Innings: []Innings{
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("R Sharma", "V Kohli"))),
			},
		}
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, match, 99, "f.json", "2024-01-01")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []db.MatchPlayer{
			{MatchID: 99, PlayerID: 1, OppositionID: 10},
			{MatchID: 99, PlayerID: 2, OppositionID: 10, IsReplacement: true},
			{MatchID: 99, PlayerID: 3, OppositionID: 20},
		}, rows)
	})

	t.Run("a replacement the side does not list flags nobody and costs nothing", func(t *testing.T) {
		t.Parallel()
		// Arrange: a name the entry gives that no side lists.
		match := &Match{
			Info: info,
			Innings: []Innings{
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("Someone Else", "V Kohli"))),
			},
		}
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, match, 99, "f.json", "2024-01-01")

		// Assert
		require.NoError(t, err)
		for i := range rows {
			assert.False(t, rows[i].IsReplacement, "row %d", i)
		}
		assert.Len(t, rows, 3)
	})

	t.Run("a replacement named for one side is not looked for in the other", func(t *testing.T) {
		t.Parallel()
		// Arrange: the archive's 1537342, whose entry names as the man who came in for one
		// side a player listed for the other. He started for his own side and stays.
		match := &Match{
			Info: info,
			Innings: []Innings{
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("K Mendis", "V Kohli"))),
			},
		}
		resolver := &stubSquadResolver{
			playerIDs:     map[string]int64{"V Kohli": 1, "R Sharma": 2, "K Mendis": 3},
			oppositionIDs: map[string]int64{"India": 10, "Sri Lanka": 20},
		}

		// Act
		rows, err := buildMatchPlayerRows(context.Background(), resolver, match, 99, "f.json", "2024-01-01")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []db.MatchPlayer{
			{MatchID: 99, PlayerID: 1, OppositionID: 10},
			{MatchID: 99, PlayerID: 2, OppositionID: 10},
			{MatchID: 99, PlayerID: 3, OppositionID: 20},
		}, rows)
	})
}

// matchReplacement is one replacements.match entry: in for out.
func matchReplacement(in, out string) MatchReplacement {
	return MatchReplacement{In: in, Out: out, Team: "India", Reason: "concussion_substitute"}
}

// deliveryWithReplacements is one delivery carrying the given match replacements.
func deliveryWithReplacements(entries ...MatchReplacement) Delivery {
	return Delivery{
		Batter:       "V Kohli",
		Bowler:       "K Mendis",
		NonStriker:   "R Sharma",
		Replacements: &Replacements{Match: entries},
	}
}

// inningsWithReplacements is one innings of one over holding the given deliveries.
func inningsWithReplacements(deliveries ...Delivery) Innings {
	return Innings{Team: "India", Overs: []Over{{Over: 0, Deliveries: deliveries}}}
}

func TestReplacementPlayers(t *testing.T) {
	t.Parallel()

	india := func(player string) SquadMember { return SquadMember{Team: "India", Player: player} }
	testCases := []struct {
		name  string
		match Match
		want  []SquadMember
	}{
		{
			name:  "a file with no replacements names nobody",
			match: Match{Innings: []Innings{inningsWithReplacements(Delivery{Batter: "a"})}},
			want:  []SquadMember{},
		},
		{
			name: "a concussion substitute is the man who came in, for the side he came in for",
			match: Match{Innings: []Innings{
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("R Sharma", "V Kohli"))),
			}},
			want: []SquadMember{india("R Sharma")},
		},
		{
			// 1234909 in the archive: a covid stand-in went back out and the man he stood
			// in for came back. The side that started is the one without the stand-in.
			name: "a player who went out and came back is not a replacement",
			match: Match{Innings: []Innings{
				inningsWithReplacements(
					deliveryWithReplacements(matchReplacement("BG Lister", "MS Chapman")),
					deliveryWithReplacements(matchReplacement("MS Chapman", "BG Lister")),
				),
			}},
			want: []SquadMember{india("BG Lister")},
		},
		{
			name: "replacements are read from every innings in playing order",
			match: Match{Innings: []Innings{
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("A12", "A1"))),
				inningsWithReplacements(deliveryWithReplacements(matchReplacement("B12", "B1"))),
			}},
			want: []SquadMember{india("A12"), india("B12")},
		},
		{
			name: "the same entry twice names the player once",
			match: Match{Innings: []Innings{
				inningsWithReplacements(
					deliveryWithReplacements(matchReplacement("A12", "A1")),
					deliveryWithReplacements(matchReplacement("A12", "A1")),
				),
			}},
			want: []SquadMember{india("A12")},
		},
		{
			name: "names are trimmed and an empty one is skipped",
			match: Match{Innings: []Innings{
				inningsWithReplacements(
					deliveryWithReplacements(matchReplacement("  A12 ", "A1"), matchReplacement("", "A2")),
				),
			}},
			want: []SquadMember{india("A12")},
		},
		{
			// A substitute finishing an injured bowler's over: the file records it under
			// `role`, which is not decoded, and nobody's membership changes.
			name:  "a delivery with a replacements object but no match entries names nobody",
			match: Match{Innings: []Innings{inningsWithReplacements(deliveryWithReplacements())}},
			want:  []SquadMember{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := tc.match.ReplacementPlayers()

			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParse_ReadsADeliverysMatchReplacements(t *testing.T) {
	t.Parallel()
	// Arrange: the shape Cricsheet writes, a role entry beside the match entry.
	const file = `{"info": {"teams": ["A", "B"]}, "innings": [{"team": "A", "overs": [{"over": 3, "deliveries": [
		{"batter": "A1", "bowler": "B1", "non_striker": "A2", "runs": {"batter": 0, "extras": 0, "total": 0},
		 "replacements": {"match": [{"in": "A12", "out": "A3", "reason": "concussion_substitute", "team": "A"}],
		                  "role": [{"in": "B11", "reason": "injury", "role": "bowler"}]}}]}]}]}`

	// Act
	m, err := Parse(strings.NewReader(file))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []SquadMember{{Team: "A", Player: "A12"}}, m.ReplacementPlayers())
	assert.Equal(t,
		[]MatchReplacement{{In: "A12", Out: "A3", Team: "A", Reason: "concussion_substitute"}},
		m.Innings[0].Overs[0].Deliveries[0].Replacements.Match)
}
