package scanx

import (
	"errors"
	"testing"
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
	n, err := CountReturningOnes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}

func TestCountReturningOnes_Normal(t *testing.T) {
	fr := &fakeRows{vals: []int{1, 1, 1}}
	n, err := CountReturningOnes(fr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3, got %d", n)
	}
	if !fr.closed {
		t.Fatalf("rows should be closed")
	}
}

func TestCountReturningOnes_ScanError(t *testing.T) {
	fr := &fakeRows{vals: []int{1, 1}, errAt: 2}
	_, err := CountReturningOnes(fr)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !fr.closed {
		t.Fatalf("rows should be closed even on error")
	}
}
