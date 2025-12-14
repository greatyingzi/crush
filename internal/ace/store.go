package ace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store interface {
	Load(path string) (Playbook, error)
	SaveAtomic(path string, pb Playbook) error
}

type FileStore struct {
	mu sync.Mutex
	// Simple per-path cache; avoids re-reading the file for every prompt.
	cache map[string]cachedPlaybook
}

type cachedPlaybook struct {
	modTime time.Time
	pb      Playbook
}

func NewFileStore() *FileStore {
	return &FileStore{cache: make(map[string]cachedPlaybook)}
}

func (s *FileStore) Load(path string) (Playbook, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Playbook{Version: "1.0", KeyPoints: []KeyPoint{}}, nil
		}
		return Playbook{}, err
	}

	s.mu.Lock()
	if cached, ok := s.cache[path]; ok && cached.modTime.Equal(info.ModTime()) {
		pb := cached.pb
		s.mu.Unlock()
		return pb, nil
	}
	s.mu.Unlock()

	bts, err := os.ReadFile(path)
	if err != nil {
		return Playbook{}, err
	}

	var pb Playbook
	if err := json.Unmarshal(bts, &pb); err != nil {
		return Playbook{}, err
	}
	pb.Normalize()

	s.mu.Lock()
	s.cache[path] = cachedPlaybook{modTime: info.ModTime(), pb: pb}
	s.mu.Unlock()

	return pb, nil
}

func (s *FileStore) SaveAtomic(path string, pb Playbook) error {
	pb.Normalize()
	pb.LastUpdated = time.Now().Format(time.RFC3339)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	payload := serializePlaybookForSave(pb)
	bts, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	bts = append(bts, '\n')

	tmp, err := os.CreateTemp(dir, ".playbook.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(bts); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	// Update cache on successful save.
	if info, err := os.Stat(path); err == nil {
		s.mu.Lock()
		s.cache[path] = cachedPlaybook{modTime: info.ModTime(), pb: pb}
		s.mu.Unlock()
	}

	return nil
}

func serializePlaybookForSave(pb Playbook) map[string]any {
	existing := make([]any, 0, len(pb.KeyPoints))
	pending := make([]any, 0, 8)

	for _, kp := range pb.KeyPoints {
		if kp.Pending {
			pending = append(pending, serializeKeyPointForSave(kp, true))
			continue
		}
		existing = append(existing, serializeKeyPointForSave(kp, false))
	}

	keyPoints := existing
	if len(pending) > 0 {
		keyPoints = append(keyPoints,
			map[string]any{
				"divider": true,
				"text":    "--- pending key points below ---",
			},
		)
		keyPoints = append(keyPoints, pending...)
	}

	return map[string]any{
		"version":      pb.Version,
		"last_updated": pb.LastUpdated,
		"key_points":   keyPoints,
	}
}

func serializeKeyPointForSave(kp KeyPoint, forcePending bool) map[string]any {
	text := strings.TrimSpace(kp.Text)
	name := strings.TrimSpace(kp.Name)
	tags := normalizeTags(kp.Tags)
	if len(tags) == 0 && text != "" {
		tags = inferTagsFromText(text, 6)
	}

	out := map[string]any{
		"name":  name,
		"text":  text,
		"tags":  tags,
		"score": kp.Score,
	}

	if kp.Pending || forcePending {
		out["pending"] = true
	}

	// Phase 2 emergency fix: validate multi-dimensional fields.
	effect := 0.5
	if kp.EffectRating != nil && 0 <= *kp.EffectRating && *kp.EffectRating <= 1 {
		effect = *kp.EffectRating
	}
	risk := -0.5
	if kp.RiskLevel != nil && -1 <= *kp.RiskLevel && *kp.RiskLevel <= 1 {
		risk = *kp.RiskLevel
	}
	innovation := 0.5
	if kp.InnovationLevel != nil && 0 <= *kp.InnovationLevel && *kp.InnovationLevel <= 1 {
		innovation = *kp.InnovationLevel
	}

	out["effect_rating"] = effect
	out["risk_level"] = risk
	out["innovation_level"] = innovation

	return out
}
