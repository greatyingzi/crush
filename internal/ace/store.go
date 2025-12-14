package ace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

	bts, err := json.MarshalIndent(pb, "", "  ")
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
