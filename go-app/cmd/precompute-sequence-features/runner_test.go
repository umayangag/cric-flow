package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_DryRun_AllTargets(t *testing.T) {
	var out bytes.Buffer
	args := []string{"-format=T20", "-targets=all", "-dry-run"}
	if err := run(args, &out); err != nil {
		t.Fatalf("run error: %v", err)
	}
	got := out.String()
	// Should mention dry-run header and at least a couple of targets
	if !strings.Contains(got, "precompute (dry-run)") {
		t.Fatalf("expected dry-run header, got: %s", got)
	}
	for _, want := range []string{"bat_transitions", "bowl_sequences", "player_windows"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got: %s", want, got)
		}
	}
}

func TestRun_DryRun_SelectedTargets(t *testing.T) {
	var out bytes.Buffer
	args := []string{"-format=ODI", "-targets=bowl_sequences,overpos", "-dry-run"}
	if err := run(args, &out); err != nil {
		t.Fatalf("run error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "format=ODI") {
		t.Fatalf("expected format ODI in output, got: %s", got)
	}
	if !strings.Contains(got, "bowl_sequences") || !strings.Contains(got, "overpos") {
		t.Fatalf("expected selected targets in output, got: %s", got)
	}
}
