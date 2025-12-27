package importkeepers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	mocks "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/importkeepers/mocks"
)

func TestRunner_Preview(t *testing.T) {
	type args struct {
		targets    map[string]int
		zeroOthers bool
	}
	cases := []struct {
		name    string
		arrange func(m *mocks.MockKeeperRepository)
		args    args
		wantErr error
	}{
		{
			name: "happy path counts",
			args: args{
				targets:    map[string]int{"a": 1, "b": 0},
				zeroOthers: true,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().CountPlayersByLowerName(mock.Anything, "a").Return(int64(2), nil)
				m.EXPECT().CountPlayersByLowerName(mock.Anything, "b").Return(int64(1), nil)
			},
		},
		{
			name: "error on count",
			args: args{
				targets:    map[string]int{"john": 1},
				zeroOthers: false,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().CountPlayersByLowerName(mock.Anything, "john").Return(int64(0), errors.New("db down"))
			},
			wantErr: errors.New("db down"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			m := mocks.NewMockKeeperRepository(t)
			if tc.arrange != nil {
				tc.arrange(m)
			}
			r := NewRunner(m)

			// Act
			err := r.Preview(ctx, tc.args.targets, tc.args.zeroOthers)

			// Assert
			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRunner_Apply(t *testing.T) {
	type args struct {
		targets    map[string]int
		zeroOthers bool
	}
	cases := []struct {
		name    string
		arrange func(m *mocks.MockKeeperRepository)
		args    args
		wantErr error
	}{
		{
			name: "happy no zero others",
			args: args{
				targets:    map[string]int{"sam": 1, "max": 0},
				zeroOthers: false,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "sam").Return(int64(1), nil)
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "max").Return(int64(1), nil)
			},
		},
		{
			name: "happy with zero others",
			args: args{
				targets:    map[string]int{"x": 1, "y": 1, "z": 0},
				zeroOthers: true,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "x").Return(int64(1), nil)
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "y").Return(int64(1), nil)
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "z").Return(int64(1), nil)
				m.EXPECT().ZeroKeepersExcept(mock.Anything, mock.MatchedBy(func(names []string) bool {
					set := map[string]struct{}{}
					for _, n := range names {
						set[n] = struct{}{}
					}
					_, hasX := set["x"]
					_, hasY := set["y"]
					_, hasZ := set["z"]
					return hasX && hasY && hasZ && len(names) == 3
				})).Return(int64(10), nil)
			},
		},
		{
			name: "error on update",
			args: args{
				targets:    map[string]int{"alice": 1, "bob": 0},
				zeroOthers: false,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().SetIsWicketKeeperByLowerName(
					mock.Anything,
					mock.AnythingOfType("int"),
					mock.AnythingOfType("string"),
				).Return(int64(0), errors.New("update failed"))
			},
			wantErr: errors.New("update failed"),
		},
		{
			name: "error on zero others",
			args: args{
				targets:    map[string]int{"a": 1, "b": 0},
				zeroOthers: true,
			},
			arrange: func(m *mocks.MockKeeperRepository) {
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "a").Return(int64(1), nil)
				m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "b").Return(int64(1), nil)
				m.EXPECT().ZeroKeepersExcept(mock.Anything, mock.Anything).Return(int64(0), errors.New("zero failed"))
			},
			wantErr: errors.New("zero failed"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			m := mocks.NewMockKeeperRepository(t)
			if tc.arrange != nil {
				tc.arrange(m)
			}
			r := NewRunner(m)

			// Act
			err := r.Apply(ctx, tc.args.targets, tc.args.zeroOthers)

			// Assert
			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
