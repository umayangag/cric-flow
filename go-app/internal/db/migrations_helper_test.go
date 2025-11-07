package db

import "testing"

func TestComputePendingMigrations_SortsAndSkipsApplied(t *testing.T) {
	files := []string{"002_b.sql", "001_a.sql", "readme.md", "003_c.SQL"}
	applied := []string{"001_a.sql"}
	got := ComputePendingMigrations(files, applied)
	want := []string{"002_b.sql", "003_c.SQL"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pending mismatch: got %#v want %#v", got, want)
	}
}

func TestComputePendingMigrations_EmptyOrNoSQL(t *testing.T) {
	if got := ComputePendingMigrations(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty for nil inputs, got %#v", got)
	}
	files := []string{"notes.txt", "migrate.sh"}
	if got := ComputePendingMigrations(files, nil); len(got) != 0 {
		t.Fatalf("expected empty for non-sql files, got %#v", got)
	}
}
