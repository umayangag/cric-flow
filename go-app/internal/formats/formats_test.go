package formats_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

func TestCanonicalCodes(t *testing.T) {
	codes := formats.CanonicalCodes()
	require.Equal(t, []string{formats.CodeTest, formats.CodeODI, formats.CodeT20, formats.CodeT20I}, codes)
}

func TestNormalizeCode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"t20", "T20"},
		{" odi ", "ODI"},
		{"test", "TEST"},
		{"T20I", "T20I"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := formats.NormalizeCode(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCanonicalizeCode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"MDM", formats.CodeTest},
		{"mdm", formats.CodeTest},
		{"ODM", formats.CodeODI},
		{"IT20", formats.CodeT20I},
		{"it20", formats.CodeT20I},
		{"T20", formats.CodeT20},
		{"ODI", formats.CodeODI},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := formats.CanonicalizeCode(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIDForCode(t *testing.T) {
	tests := []struct {
		code    string
		wantID  int
		wantErr bool
	}{
		{"TEST", formats.IDTest, false},
		{"ODI", formats.IDODI, false},
		{"T20", formats.IDT20, false},
		{"T20I", formats.IDT20I, false},
		{"t20", formats.IDT20, false},
		{"UNKNOWN", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			id, err := formats.IDForCode(tt.code)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unknown format")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantID, id)
		})
	}
}

func TestMapFormatIDs(t *testing.T) {
	tests := []struct {
		code string
		want []int
	}{
		{"", []int{formats.IDT20, formats.IDT20I}},
		{"T20", []int{formats.IDT20, formats.IDT20I}},
		{"T20I", []int{formats.IDT20, formats.IDT20I}},
		{"ODI", []int{formats.IDODI}},
		{"TEST", []int{formats.IDTest}},
		{"UNKNOWN", []int{formats.IDT20, formats.IDT20I}},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := formats.MapFormatIDs(tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetHierarchy(t *testing.T) {
	hierarchy := formats.GetHierarchy()

	tests := []struct {
		name          string
		index         int
		expectedCode  string
		expectedName  string
		expectedChild *formats.FormatHierarchyNode
	}{
		{
			name:         "MDM Node",
			index:        0,
			expectedCode: "MDM",
			expectedName: "Multi-Day Match",
			expectedChild: &formats.FormatHierarchyNode{
				Code: formats.CodeTest,
				Name: "Test Matches",
			},
		},
		{
			name:         "ODM Node",
			index:        1,
			expectedCode: "ODM",
			expectedName: "One Day Match",
			expectedChild: &formats.FormatHierarchyNode{
				Code: formats.CodeODI,
				Name: "One Day International",
			},
		},
		{
			name:         "T20 Bucket Node",
			index:        2,
			expectedCode: formats.CodeT20,
			expectedName: "T20 (Bucket)",
		},
	}

	require.Equal(t, 3, len(hierarchy), "Hierarchy should have 3 root nodes")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := hierarchy[tt.index]
			require.Equal(t, tt.expectedCode, node.Code)
			require.Equal(t, tt.expectedName, node.Name)

			if tt.expectedChild != nil {
				require.NotEmpty(t, node.Children)
				require.Equal(t, tt.expectedChild.Code, node.Children[0].Code)
				require.Equal(t, tt.expectedChild.Name, node.Children[0].Name)
			}
		})
	}
}
