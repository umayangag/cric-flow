package scanx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRows struct {
	vals   []int
	idx    int
	errAt  int // 1-based scan call index to return error
	closed bool
}

func (f *fakeRows) Next() bool {
	if f.idx >= len(f.vals) {
		return false
	}
	f.idx++
	return true
}

func (f *fakeRows) Scan(dest ...any) error {
	if f.errAt > 0 && f.idx == f.errAt {
		return errors.New("scan error")
	}
	// idx has already been incremented in Next(), so current value is vals[idx-1]
	v := f.vals[f.idx-1]
	if len(dest) != 1 {
		return errors.New("expected single dest")
	}
	p, ok := dest[0].(*int)
	if !ok {
		return errors.New("dest must be *int")
	}
	*p = v
	return nil
}

func (f *fakeRows) Close() { f.closed = true }

func TestCountReturningOnes_Nil(t *testing.T) {
	t.Parallel()

	n, err := CountReturningOnes(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestCountReturningOnes_Normal(t *testing.T) {
	t.Parallel()

	fr := &fakeRows{vals: []int{1, 1, 1}}
	n, err := CountReturningOnes(fr)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	assert.True(t, fr.closed, "rows should be closed")
}

func TestCountReturningOnes_ScanError(t *testing.T) {
	t.Parallel()

	fr := &fakeRows{vals: []int{1, 1}, errAt: 2}
	_, err := CountReturningOnes(fr)
	require.Error(t, err)
	assert.True(t, fr.closed, "rows should be closed even on error")
}
