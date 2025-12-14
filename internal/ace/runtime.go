package ace

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

type Runtime struct {
	cfg       *config.Config
	store     Store
	selector  Selector
	formatter Formatter
}

func NewRuntime(cfg *config.Config) *Runtime {
	return &Runtime{
		cfg:       cfg,
		store:     NewFileStore(),
		selector:  NewDefaultSelector(),
		formatter: NewDefaultFormatter(),
	}
}

func (r *Runtime) WithStore(store Store) *Runtime {
	r.store = store
	return r
}

func (r *Runtime) WithSelector(selector Selector) *Runtime {
	r.selector = selector
	return r
}

func (r *Runtime) WithFormatter(formatter Formatter) *Runtime {
	r.formatter = formatter
	return r
}

func (r *Runtime) Prefix(_ context.Context, basePrefix, sessionID, prompt, workingDir, model, provider string) (string, error) {
	_ = sessionID
	_ = workingDir
	_ = model
	_ = provider
	aceCfg := r.cfg.Options.ACE
	if aceCfg == nil || !aceCfg.Enabled {
		return basePrefix, nil
	}

	path := PlaybookPath(r.cfg)
	pb, err := r.store.Load(path)
	if err != nil {
		slog.Debug("ACE prefix: playbook load failed", "path", path, "error", err)
		return basePrefix, nil
	}
	slog.Debug(
		"ACE prefix: start",
		"playbook_path", path,
		"key_points", len(pb.KeyPoints),
		"max_items", aceCfg.MaxItems,
		"min_score", aceCfg.MinScore,
		"max_chars", aceCfg.MaxChars,
		"prompt_chars", len(prompt),
	)

	selected := r.selector.Select(pb, prompt, SelectOptions{
		MaxItems: aceCfg.MaxItems,
		MinScore: aceCfg.MinScore,
		MaxChars: aceCfg.MaxChars,
	})
	if aceCfg.MaxChars > 0 {
		selected = r.formatter.TrimToMaxChars(selected, aceCfg.MaxChars)
	}
	memory := strings.TrimSpace(r.formatter.Format(selected))
	if memory == "" {
		slog.Debug("ACE prefix: no memory selected", "selected", len(selected))
		return basePrefix, nil
	}
	slog.Debug("ACE prefix: injected", "selected", len(selected), "memory_chars", len(memory))
	if strings.TrimSpace(basePrefix) == "" {
		return memory, nil
	}
	return strings.TrimSpace(basePrefix) + "\n\n" + memory, nil
}

func PlaybookPath(cfg *config.Config) string {
	aceCfg := cfg.Options.ACE
	if aceCfg == nil || strings.TrimSpace(aceCfg.PlaybookPath) == "" {
		return filepath.Join(cfg.Options.DataDirectory, "ace", "playbook.json")
	}

	pth := strings.TrimSpace(aceCfg.PlaybookPath)
	if filepath.IsAbs(pth) {
		return pth
	}
	return filepath.Join(cfg.Options.DataDirectory, pth)
}
