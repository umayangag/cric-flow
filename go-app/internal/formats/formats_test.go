package formats_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/formats"
)

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
