package precompute

import (
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
	tests := []struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			before := time.Now().UTC()

			setStart(tt.season, tt.formats)

			st := GetStatus()
			assert.True(t, st.Running)
			assert.Equal(t, tt.season, st.Season)
			assert.Equal(t, tt.formats, st.Formats)
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
	tests := []struct {
		name  string
		phase string
	}{
		{name: "form_phase", phase: "form"},
		{name: "consistency_phase", phase: "consistency"},
		{name: "done_phase", phase: "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{"T20I"})

			setPhase(tt.phase)

			st := GetStatus()
			assert.Equal(t, tt.phase, st.Phase)
		})
	}
}

func TestSetCurrentFormat(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "t20i", code: "T20I"},
		{name: "odi", code: "ODI"},
		{name: "test", code: "TEST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{tt.code})
			before := time.Now().UTC()

			setCurrentFormat(tt.code)

			st := GetStatus()
			assert.Equal(t, tt.code, st.CurrentFormat)
			require.False(t, st.FormatStartedAt.IsZero())
			assert.True(t, !st.FormatStartedAt.Before(before))
		})
	}
}

func TestSetDone(t *testing.T) {
	resetState(t)
	setStart("2024", []string{"T20I"})
	setPhase("form")
	setCurrentFormat("T20I")

	setDone()

	st := GetStatus()
	assert.False(t, st.Running)
	assert.Equal(t, "done", st.Phase)
	assert.Empty(t, st.CurrentFormat)
	assert.True(t, st.FormatStartedAt.IsZero())
	require.False(t, st.FinishedAt.IsZero())
}

func TestSetLastError(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
	}{
		{name: "with_error", errMsg: "something went wrong"},
		{name: "empty_clears", errMsg: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			setStart("2024", []string{"T20I"})

			setLastError(tt.errMsg)

			st := GetStatus()
			assert.Equal(t, tt.errMsg, st.LastError)
		})
	}
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
	setLastError("timeout")
	setDone()
	st = GetStatus()
	assert.False(t, st.Running)
	assert.Equal(t, "done", st.Phase)
	assert.Equal(t, "timeout", st.LastError)
	assert.Empty(t, st.CurrentFormat)
}
