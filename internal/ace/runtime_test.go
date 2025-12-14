package ace

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultSelector_SelectForPrompt(t *testing.T) {
	t.Parallel()

	pb := Playbook{
		Version: "1.0",
		KeyPoints: []KeyPoint{
			{Name: "kpt_001", Text: "Prefer gofumpt formatting.", Tags: []string{"go", "format"}, Score: 2},
			{Name: "kpt_002", Text: "Use ripgrep (rg) for searching.", Tags: []string{"tools"}, Score: 1},
			{Name: "kpt_003", Text: "Write code in Python.", Tags: []string{"python"}, Score: 2},
		},
	}

	sel := NewDefaultSelector()
	selected := sel.Select(pb, "Please format this Go code (gofumpt).", SelectOptions{
		MaxItems: 2,
		MinScore: 0,
		MaxChars: 0,
	})
	require.NotEmpty(t, selected)
	require.Equal(t, "kpt_001", selected[0].Name)
}
