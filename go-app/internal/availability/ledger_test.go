package availability_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/availability/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// TestLedgerFlag_PromotesOnlyWhenACriterionCorroborates is the rule the whole ledger
// exists for: a user's claim never becomes a fact about the player on its own.
func TestLedgerFlag_PromotesOnlyWhenACriterionCorroborates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		lastPlayed    time.Time
		wantPromoted  bool
		wantCriterion string
	}{
		{
			name:          "a player away from every format for a decade is corroborated",
			lastPlayed:    date(2015, 3, 1),
			wantPromoted:  true,
			wantCriterion: availability.CriterionInactivity,
		},
		{
			name:       "a player who played last month is not, whatever the user says",
			lastPlayed: date(2026, 8, 20),
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			store := mocks.NewMockStore(t)
			store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
				Return(availability.Evidence{PlayerID: 7, LastPlayed: testCase.lastPlayed}, nil)
			var saved availability.Flag
			store.EXPECT().SaveFlag(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, flag availability.Flag) error {
					saved = flag
					return nil
				})
			ledger := availability.NewLedger(store, availability.Criteria(nil))

			result, err := ledger.Flag(context.Background(), "kate", 7, "ODI", now)

			require.NoError(t, err)
			assert.Equal(t, testCase.wantPromoted, result.Flag.Promoted())
			assert.Equal(t, testCase.wantCriterion, result.Flag.Criterion)
			assert.Equal(t, saved, result.Flag, "the flag returned is the flag stored")
			assert.Equal(t, "kate", saved.Actor)
			assert.Equal(t, now, saved.FlaggedAt)
		})
	}
}

// TestLedgerFlag_AnUncorroboratedClaimStillHidesHimFromThatUser states the other half of
// the rule: the flag is not nothing. It excludes the player from the flagging user's own
// default pools, and from nobody else's.
func TestLedgerFlag_AnUncorroboratedClaimStillHidesHimFromThatUser(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{PlayerID: 7, LastPlayed: date(2026, 8, 20)}, nil)
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).Return(nil)
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	result, err := ledger.Flag(context.Background(), "kate", 7, "ODI", now)

	require.NoError(t, err)
	assert.False(t, result.Flag.Promoted())
	assert.Equal(t, availability.ReasonUserFlagged, result.Flag.Reason())
}

// TestLedgerFlag_ReportsWhatItCouldNotCheck keeps the answer honest while X-1a is
// outstanding: two of the three criteria have no evidence to read, and the response says
// so rather than implying they were checked and said no.
func TestLedgerFlag_ReportsWhatItCouldNotCheck(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{PlayerID: 7, LastPlayed: date(2026, 8, 20)}, nil)
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).Return(nil)
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	result, err := ledger.Flag(context.Background(), "kate", 7, "ODI", now)

	require.NoError(t, err)
	assert.Equal(t,
		[]string{availability.CriterionCareerEnd, availability.CriterionAgeAndInactivity},
		result.Unchecked)
	assert.Len(t, result.Notes, 3, "every criterion that ran records what it read")
}

// TestLedgerFlag_StopsAtTheFirstCorroboratingCriterion pins the recorded reason to one
// criterion. A promotion carries the evidence that allowed it, and a list of every rule
// that happened to agree would say nothing about which one decided.
func TestLedgerFlag_StopsAtTheFirstCorroboratingCriterion(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{
			PlayerID:   7,
			LastPlayed: date(2015, 1, 1),
			BirthDate:  date(1975, 1, 1),
			CareerEnd:  date(2016, 1, 1),
		}, nil)
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).Return(nil)
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	result, err := ledger.Flag(context.Background(), "kate", 7, "TEST", now)

	require.NoError(t, err)
	assert.Equal(t, availability.CriterionInactivity, result.Flag.Criterion)
	assert.Len(t, result.Notes, 1, "the criteria after the deciding one are not run")
}

// TestLedgerFlag_HonoursTheConfiguredCriteria shows the list is injected, which is what
// makes it pluggable: X-1a adds an entry and nothing here changes.
func TestLedgerFlag_HonoursTheConfiguredCriteria(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Pool.Retirement.InactiveYears = 1
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{PlayerID: 7, LastPlayed: date(2024, 1, 1)}, nil)
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).Return(nil)
	ledger := availability.NewLedger(store, availability.Criteria(cfg))

	result, err := ledger.Flag(context.Background(), "kate", 7, "ODI", now)

	require.NoError(t, err)
	assert.True(t, result.Flag.Promoted(), "two years away clears a one-year bound")
}

// TestLedgerFlag_FallsBackToTheDefaultActor keeps one deployment's single user addressing
// one ledger, however the request spelled it.
func TestLedgerFlag_FallsBackToTheDefaultActor(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{PlayerID: 7}, nil)
	var saved availability.Flag
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, flag availability.Flag) error {
			saved = flag
			return nil
		})
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	_, err := ledger.Flag(context.Background(), "  ", 7, "ODI", now)

	require.NoError(t, err)
	assert.Equal(t, availability.DefaultActor, saved.Actor)
}

// TestLedgerFlag_ReturnsTheStoreError never swallows a failed write: a flag the user was
// told about but that was not stored is worse than a refusal.
func TestLedgerFlag_ReturnsTheStoreError(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().RetirementEvidence(mock.Anything, int64(7)).
		Return(availability.Evidence{PlayerID: 7}, nil)
	store.EXPECT().SaveFlag(mock.Anything, mock.Anything).Return(errors.New("write failed"))
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	_, err := ledger.Flag(context.Background(), "kate", 7, "ODI", now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

// TestLedgerUnflag_ReportsTheDemotionItCaused is the reverse rule: withdrawing a claim
// that had raised the stored fact lowers it again, and the caller is told, so a surface
// can say what undoing did rather than only that it happened.
func TestLedgerUnflag_ReportsTheDemotionItCaused(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		removed     availability.Flag
		existed     bool
		wantDemoted bool
	}{
		{
			name: "withdrawing a promoted claim demotes the fact",
			removed: availability.Flag{
				PlayerID:   7,
				PromotedAt: now,
				Criterion:  availability.CriterionInactivity,
			},
			existed:     true,
			wantDemoted: true,
		},
		{
			name:    "withdrawing a bare claim demotes nothing",
			removed: availability.Flag{PlayerID: 7},
			existed: true,
		},
		{
			name:    "there was nothing to withdraw",
			existed: false,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			store := mocks.NewMockStore(t)
			store.EXPECT().DeleteFlag(mock.Anything, "kate", int64(7)).
				Return(testCase.removed, testCase.existed, nil)
			ledger := availability.NewLedger(store, availability.Criteria(nil))

			removed, existed, err := ledger.Unflag(context.Background(), "kate", 7)

			require.NoError(t, err)
			assert.Equal(t, testCase.existed, existed)
			assert.Equal(t, testCase.wantDemoted, existed && removed.Promoted())
		})
	}
}

// TestLedgerFlags_ReadsOneActorsClaims keeps a flag scoped to the user who set it, which
// is the difference between the claim and the fact.
func TestLedgerFlags_ReadsOneActorsClaims(t *testing.T) {
	t.Parallel()
	store := mocks.NewMockStore(t)
	store.EXPECT().ListFlags(mock.Anything, "kate").
		Return(map[int64]availability.Flag{7: {PlayerID: 7, Actor: "kate"}}, nil)
	ledger := availability.NewLedger(store, availability.Criteria(nil))

	flags, err := ledger.Flags(context.Background(), "kate")

	require.NoError(t, err)
	assert.Len(t, flags, 1)
	assert.Equal(t, availability.ReasonUserFlagged, flags[7].Reason())
}

// TestFlagReason_NamesTheFactWhenPromoted keeps the two exclusion reasons distinct on the
// wire: a corroborated claim is reported as the fact it became, not as one user's opinion.
func TestFlagReason_NamesTheFactWhenPromoted(t *testing.T) {
	t.Parallel()

	assert.Equal(t, availability.ReasonRetired,
		availability.Flag{PromotedAt: now}.Reason())
	assert.Equal(t, availability.ReasonUserFlagged,
		availability.Flag{}.Reason())
}
