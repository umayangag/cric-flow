package formats

import (
	"testing"
)

func TestGetHierarchy(t *testing.T) {
	hierarchy := GetHierarchy()
	if len(hierarchy) != 3 {
		t.Fatalf("expected 3 root nodes, got %d", len(hierarchy))
	}

	// Check MDM -> TEST
	if hierarchy[0].Code != "MDM" {
		t.Errorf("expected hierarchy[0] to be MDM, got %s", hierarchy[0].Code)
	}
	if len(hierarchy[0].Children) != 1 || hierarchy[0].Children[0].Code != CodeTest {
		t.Errorf("expected MDM to have child TEST, got %v", hierarchy[0].Children)
	}

	// Check ODM -> ODI
	if hierarchy[1].Code != "ODM" {
		t.Errorf("expected hierarchy[1] to be ODM, got %s", hierarchy[1].Code)
	}
	if len(hierarchy[1].Children) != 1 || hierarchy[1].Children[0].Code != CodeODI {
		t.Errorf("expected ODM to have child ODI, got %v", hierarchy[1].Children)
	}

	// Check T20 Bucket
	if hierarchy[2].Name != "T20 (Bucket)" {
		t.Errorf("expected hierarchy[2] to be T20 (Bucket), got %s", hierarchy[2].Name)
	}
}
