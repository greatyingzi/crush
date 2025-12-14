package ace

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	pb Playbook
}

func (s *memoryStore) Load(path string) (Playbook, error) {
	_ = path
	return s.pb, nil
}

func (s *memoryStore) SaveAtomic(path string, pb Playbook) error {
	_ = path
	s.pb = pb
	return nil
}

func TestUpdateFromSessionSummary_ExtractsMultipleKeyPoints(t *testing.T) {
	t.Parallel()

	store := &memoryStore{pb: Playbook{Version: "1.0", KeyPoints: []KeyPoint{}}}
	changed, err := UpdateFromSessionSummary(store, "ignored.json", "My Session", `
- Use rg for searching.
- Prefer gofumpt for formatting.
`)
	require.NoError(t, err)
	require.True(t, changed)
	require.GreaterOrEqual(t, len(store.pb.KeyPoints), 2)
	require.Contains(t, store.pb.KeyPoints[0].Text, "[Session] My Session:")
	require.NotNil(t, store.pb.KeyPoints[0].EffectRating)
	require.NotNil(t, store.pb.KeyPoints[0].RiskLevel)
	require.NotNil(t, store.pb.KeyPoints[0].InnovationLevel)
}

func TestUpdateFromSessionSummary_MergesSimilarKeyPoints(t *testing.T) {
	t.Parallel()

	store := &memoryStore{pb: Playbook{
		Version: "1.0",
		KeyPoints: []KeyPoint{
			{Name: "kpt_001", Text: "[Session] Old: Prefer gofumpt formatting", Tags: []string{"go"}, Score: 1},
		},
	}}
	changed, err := UpdateFromSessionSummary(store, "ignored.json", "New", `
- Prefer gofumpt formatting.
`)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, store.pb.KeyPoints, 1)
	require.GreaterOrEqual(t, store.pb.KeyPoints[0].Score, 2)
	require.NotNil(t, store.pb.KeyPoints[0].EffectRating)
}
