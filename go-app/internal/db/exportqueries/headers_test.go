package exportqueries

import (
	"context"
	"testing"
)

func TestAppendSeqIfEnabled_Bowling(t *testing.T) {
	base := []string{"c1", "c2"}
	seq := BowlingSeqHeaders()
	// OFF → unchanged
	got := AppendSeqIfEnabled(context.Background(), base, seq)
	if len(got) != len(base) {
		t.Fatalf("OFF: expected len=%d got %d", len(base), len(got))
	}
	// ON → base + seq
	ctx := WithSeqEnabled(context.Background(), true)
	got2 := AppendSeqIfEnabled(ctx, base, seq)
	if len(got2) != len(base)+len(seq) {
		t.Fatalf("ON: expected len=%d got %d", len(base)+len(seq), len(got2))
	}
	for i := range base {
		if got2[i] != base[i] {
			t.Fatalf("prefix mismatch at %d: %q vs %q", i, got2[i], base[i])
		}
	}
}

func TestAppendSeqIfEnabled_Batting(t *testing.T) {
	base := []string{"h1"}
	seq := BattingSeqHeaders()
	// OFF
	got := AppendSeqIfEnabled(context.Background(), base, seq)
	if len(got) != 1 || got[0] != "h1" {
		t.Fatalf("OFF: unexpected result: %v", got)
	}
	// ON
	ctx := WithSeqEnabled(context.Background(), true)
	got2 := AppendSeqIfEnabled(ctx, base, seq)
	if len(got2) != 1+len(seq) {
		t.Fatalf("ON: wrong length: %d", len(got2))
	}
}
