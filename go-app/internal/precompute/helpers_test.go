package precompute

import (
	"context"
	"reflect"
	"testing"
)

func TestDiscoverFormatCodes_ReturnsProvidedWhenNonEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	provided := []string{"T20I", "ODI", "TEST"}

	got, err := discoverFormatCodes(ctx, provided)
	if err != nil {
		t.Fatalf("discoverFormatCodes returned error for provided input: %v", err)
	}
	if !reflect.DeepEqual(got, provided) {
		t.Fatalf("expected same contents as provided; got=%v want=%v", got, provided)
	}
}
