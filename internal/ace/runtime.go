package ace

import (
	"context"
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
		return basePrefix, nil
	}

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
		return basePrefix, nil
	}
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
