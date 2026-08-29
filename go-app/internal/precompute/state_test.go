package precompute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetState clears the global status so tests are independent.
func resetState(t *testing.T) {
	t.Helper()
	mu.Lock()
	currentStat = Status{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		currentStat = Status{}
		mu.Unlock()
	})
}

func TestSetStart(t *testing.T) {
	testCases := []struct {
		name    string
		season  string
		formats []string
	}{
		{
			name:    "single_format",
			season:  "2024",
			formats: []string{"T20I"},
		},
		{
			name:    "multiple_formats",
			season:  "2025",
			formats: []string{"TEST", "ODI", "T20I"},
		},
		{
			name:    "empty_formats",
			season:  "2023",
			formats: []string{},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			resetState(t)
			before := time.Now().UTC()

			setStart(tc.season, tc.formats)

			st := GetStatus()
			assert.True(t, st.Running)
			assert.Equal(t, tc.season, st.Season)
			assert.Equal(t, tc.formats, st.Formats)
			assert.Equal(t, "starting", st.Phase)
			assert.Empty(t, st.LastError)
			require.False(t, st.StartedAt.IsZero())
			assert.True(t, !st.StartedAt.Before(before), "StartedAt should be >= before")
		})
	}
}

func TestSetStart_DoesNotShareSlice(t *testing.T) {
	resetState(t)
	original := []string{"T20I", "ODI"}
	setStart("2024", original)

	// Mutate the original slice — status should be unaffected.
	original[0] = "MUTATED"
	st := GetStatus()
	assert.Equal(t, "T20I", st.Formats[0])
}

func TestSetPhase(t *testing.T) {
	testCases := []struct {
		name  string
		phase string
	}{
		{name: "form_phase", phase: "form"},
		{name: "consistency_phase", phase: "consistency"},
		{name: "done_phase", phase: "done"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{"T20I"})

			setPhase(tc.phase)

			st := GetStatus()
			assert.Equal(t, tc.phase, st.Phase)
		})
	}
}

func TestSetCurrentFormat(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{name: "t20i", code: "T20I"},
		{name: "odi", code: "ODI"},
		{name: "test", code: "TEST"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{tc.code})
			before := time.Now().UTC()

			setCurrentFormat(tc.code)

			st := GetStatus()
			assert.Equal(t, tc.code, st.CurrentFormat)
			require.False(t, st.FormatStartedAt.IsZero())
			assert.True(t, !st.FormatStartedAt.Before(before))
		})
	}
}

func TestSetDone(t *testing.T) {
	testCases := []struct {
		name          string
		runErr        error
		wantPhase     string
		wantLastError string
		wantSucceeded bool
	}{
		{
			name:          "successful run reports done",
			runErr:        nil,
			wantPhase:     PhaseDone,
			wantSucceeded: true,
		},
		{
			name:          "failed run reports failed",
			runErr:        errors.New("upsert raw stats venue pid=61"),
			wantPhase:     PhaseFailed,
			wantLastError: "upsert raw stats venue pid=61",
		},
		{
			name:          "cancelled run reports failed",
			runErr:        context.Canceled,
			wantPhase:     PhaseFailed,
			wantLastError: context.Canceled.Error(),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{"T20I"})
			setPhase("form")
			setCurrentFormat("T20I")

			setDone(tc.runErr)

			st := GetStatus()
			assert.False(t, st.Running)
			assert.Equal(t, tc.wantPhase, st.Phase)
			assert.Equal(t, tc.wantLastError, st.LastError)
			assert.Equal(t, tc.wantSucceeded, st.Succeeded())
			assert.Empty(t, st.CurrentFormat)
			assert.True(t, st.FormatStartedAt.IsZero())
			require.False(t, st.FinishedAt.IsZero(), "a finished run records when it ended, however it ended")
		})
	}
}

// TestStatus_Succeeded_RunningOrUnfinished guards the two states that are neither a
// success nor a failure, and which an "is FinishedAt set?" check used to conflate.
func TestStatus_Succeeded_RunningOrUnfinished(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		status Status
	}{
		{"never run", Status{}},
		{"still running", Status{Running: true, Phase: PhaseDone, FinishedAt: time.Now()}},
		{"finished but unmarked", Status{FinishedAt: time.Now()}},
		{"phase done without a finish time", Status{Phase: PhaseDone}},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, tc.status.Succeeded())
		})
	}
}

// TestSetStart_ClearsThePreviousRunsError covers the case that makes LastError
// meaningful: a new run must not inherit the last one's failure.
func TestSetStart_ClearsThePreviousRunsError(t *testing.T) {
	resetState(t)
	setStart("2024", []string{"T20I"})
	setDone(errors.New("something went wrong"))

	setStart("2025", []string{"ODI"})

	st := GetStatus()
	assert.Empty(t, st.LastError)
	assert.True(t, st.Running)
	assert.False(t, st.Succeeded())
}

func TestGetStatus_ReturnsIndependentCopy(t *testing.T) {
	resetState(t)
	setStart("2024", []string{"T20I", "ODI"})

	st1 := GetStatus()
	setPhase("form")
	st2 := GetStatus()

	// st1 should still have "starting", not "form"
	assert.Equal(t, "starting", st1.Phase)
	assert.Equal(t, "form", st2.Phase)
}

func TestFullLifecycle(t *testing.T) {
	resetState(t)

	// Initially idle.
	st := GetStatus()
	assert.False(t, st.Running)

	// Start.
	setStart("2025", []string{"TEST", "ODI"})
	st = GetStatus()
	assert.True(t, st.Running)
	assert.Equal(t, "starting", st.Phase)

	// Phase transition.
	setPhase("form")
	setCurrentFormat("TEST")
	st = GetStatus()
	assert.Equal(t, "form", st.Phase)
	assert.Equal(t, "TEST", st.CurrentFormat)

	// Error + done.
	setDone(errors.New("timeout"))
	st = GetStatus()
	assert.False(t, st.Running)
	assert.Equal(t, PhaseFailed, st.Phase)
	assert.Equal(t, "timeout", st.LastError)
	assert.False(t, st.Succeeded())
	assert.Empty(t, st.CurrentFormat)
}
