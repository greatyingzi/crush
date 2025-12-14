package ace

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlaybookUnmarshal_StringsAndObjectsAndDivider(t *testing.T) {
	t.Parallel()

	effect := 0.9
	risk := 0.2
	innovation := 0.8

	raw := map[string]any{
		"version": "1.0",
		"key_points": []any{
			"remember to run gofmt",
			map[string]any{"divider": true},
			map[string]any{
				"name":             "kpt_001",
				"text":             "prefer gofumpt",
				"tags":             []any{"Go", "format"},
				"score":            2,
				"effect_rating":    effect,
				"risk_level":       risk,
				"innovation_level": innovation,
			},
			map[string]any{"text": "   "}, // ignored
			42,                            // ignored
		},
	}
	bts, err := json.Marshal(raw)
	require.NoError(t, err)

	var pb Playbook
	require.NoError(t, json.Unmarshal(bts, &pb))
	require.Equal(t, "1.0", pb.Version)
	require.Len(t, pb.KeyPoints, 2)

	require.Equal(t, "remember to run gofmt", pb.KeyPoints[0].Text)
	require.Equal(t, "prefer gofumpt", pb.KeyPoints[1].Text)
	require.Equal(t, "kpt_001", pb.KeyPoints[1].Name)
	require.Equal(t, []string{"format", "go"}, pb.KeyPoints[1].Tags)
	require.Equal(t, 2, pb.KeyPoints[1].Score)
	require.NotNil(t, pb.KeyPoints[1].EffectRating)
	require.InEpsilon(t, effect, *pb.KeyPoints[1].EffectRating, 1e-6)
	require.NotNil(t, pb.KeyPoints[1].RiskLevel)
	require.InEpsilon(t, risk, *pb.KeyPoints[1].RiskLevel, 1e-6)
	require.NotNil(t, pb.KeyPoints[1].InnovationLevel)
	require.InEpsilon(t, innovation, *pb.KeyPoints[1].InnovationLevel, 1e-6)
}
