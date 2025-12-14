package ace

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

type fakeGen struct {
	out string
}

func (g fakeGen) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return g.out, nil
}

func TestUpdateFromSessionMessages_LLMPipeline(t *testing.T) {
	t.Parallel()

	store := &memoryStore{pb: Playbook{Version: "1.0", KeyPoints: []KeyPoint{}}}
	gen := fakeGen{out: `{
  "new_key_points": [
    {
      "text": "Prefer gofumpt for formatting.",
      "tags": ["go","format"],
      "effect_rating": 0.8,
      "risk_level": -0.9,
      "innovation_level": 0.2,
      "pending": false
    }
  ],
  "evaluations": []
}`}
	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "format this go code"}}},
		{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "ok"}}},
	}
	changed, err := UpdateFromSessionMessages(t.Context(), store, "ignored.json", "Title", msgs, gen)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, store.pb.KeyPoints, 1)
	require.Equal(t, "Prefer gofumpt for formatting.", store.pb.KeyPoints[0].Text)
	require.True(t, store.pb.KeyPoints[0].Pending)
	require.Equal(t, 3, store.pb.KeyPoints[0].Score)
	require.NotNil(t, store.pb.KeyPoints[0].EffectRating)
}
