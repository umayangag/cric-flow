package db

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildPlayedMatchesFiltersQuery(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	te := time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		format    string
		team1     string
		team2     string
		start     time.Time
		end       time.Time
		order     string
		limit     int
		wantParts []string
		notParts  []string
		wantArgs  []any
	}{
		{
			name:      "no filters => asc order, no limit",
			order:     "",
			limit:     0,
			wantParts: []string{"WHERE m.match_date < NOW()", "ORDER BY m.match_date ASC"},
			notParts:  []string{" LIMIT $"},
			wantArgs:  []any{},
		},
		{
			name:      "format only",
			format:    "odi",
			wantParts: []string{"mf.code = $1", "ORDER BY m.match_date ASC"},
			wantArgs:  []any{"odi"},
		},
		{
			name:      "start only",
			start:     ts,
			wantParts: []string{"m.match_date >= $1"},
			wantArgs:  []any{ts},
		},
		{
			name:      "end only",
			end:       te,
			wantParts: []string{"m.match_date <= $1"},
			wantArgs:  []any{te},
		},
		{
			name:      "start and end",
			start:     ts,
			end:       te,
			wantParts: []string{"m.match_date >= $1", "m.match_date <= $2"},
			wantArgs:  []any{ts, te},
		},
		{
			name:      "single team filter",
			team1:     "IND",
			wantParts: []string{"(mt.team_a = $1", "OR mt.team_b = $2)"},
			wantArgs:  []any{"IND", "IND"},
		},
		{
			name:      "two teams filter (order-insensitive)",
			team1:     "IND",
			team2:     "AUS",
			wantParts: []string{"((mt.team_a = $1", "mt.team_b = $2)", "OR (mt.team_a = $3", "mt.team_b = $4)"},
			wantArgs:  []any{"IND", "AUS", "AUS", "IND"},
		},
		{
			name:      "desc order with limit",
			order:     "desc",
			limit:     10,
			wantParts: []string{"ORDER BY m.match_date DESC", " LIMIT $1"},
			wantArgs:  []any{10},
		},
		{
			name:   "combined filters with proper arg order",
			format: "t20",
			team1:  "IND",
			team2:  "AUS",
			start:  ts,
			end:    te,
			order:  "desc",
			limit:  25,
			wantParts: []string{
				"mf.code = $1",
				"m.match_date >= $2",
				"m.match_date <= $3",
				"ORDER BY m.match_date DESC",
				" LIMIT $",
			},
			wantArgs: []any{"t20", ts, te, "IND", "AUS", "AUS", "IND", 25},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Act
			sql, args := buildPlayedMatchesFiltersQuery(
				tc.format,
				tc.team1,
				tc.team2,
				tc.start,
				tc.end,
				tc.order,
				tc.limit,
			)

			// Assert
			require.Contains(t, sql, "WHERE m.match_date < NOW()")
			for _, p := range tc.wantParts {
				require.True(t, strings.Contains(sql, p), "SQL missing expected part %q\nSQL: %s", p, sql)
			}
			for _, np := range tc.notParts {
				require.False(t, strings.Contains(sql, np), "SQL should not contain %q\nSQL: %s", np, sql)
			}
			require.Equal(t, tc.wantArgs, args)
		})
	}
}
