package ace

import (
	"context"
	"log/slog"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

type Summarizer interface {
	Summarize(ctx context.Context, sessionID string) error
}

type Observer struct {
	cfg       *config.Config
	sessions  session.Service
	messages  message.Service
	summarize Summarizer
	model     fantasy.LanguageModel
	store     Store
}

func NewObserver(cfg *config.Config, sessions session.Service, messages message.Service, summarizer Summarizer, model fantasy.LanguageModel) *Observer {
	return &Observer{
		cfg:             cfg,
		sessions:        sessions,
		messages:        messages,
		summarize:       summarizer,
		model:           model,
		store:           NewFileStore(),
	}
}

func (o *Observer) OnShutdown(ctx context.Context) {
	if !o.enabled() || !boolVal(o.cfg.Options.ACE.UpdateOnExit) {
		return
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	sessions, err := o.sessions.List(shutdownCtx)
	if err != nil || len(sessions) == 0 {
		return
	}
	sess := sessions[0]

	o.runUpdate(shutdownCtx, sess.ID, sess.Title, "exit", true)
}

func (o *Observer) OnSessionEnd(ctx context.Context, sessionID, reason string) {
	if !o.enabled() || !boolVal(o.cfg.Options.ACE.UpdateOnSessionEnd) {
		return
	}

	endCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	sess, err := o.sessions.Get(endCtx, sessionID)
	if err != nil {
		return
	}

	o.runUpdate(endCtx, sessionID, sess.Title, reason, true)
}

func (o *Observer) OnPreCompact(ctx context.Context, sessionID string) {
	if !o.enabled() || !boolVal(o.cfg.Options.ACE.UpdateOnPreCompact) {
		return
	}

	preCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	sess, err := o.sessions.Get(preCtx, sessionID)
	if err != nil {
		return
	}
	o.runUpdate(preCtx, sessionID, sess.Title, "precompact", true)
}

func (o *Observer) enabled() bool {
	return o.cfg != nil && o.cfg.Options != nil && o.cfg.Options.ACE != nil && o.cfg.Options.ACE.Enabled
}

func boolVal(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}

func (o *Observer) runUpdate(ctx context.Context, sessionID, sessionTitle, reason string, allowSummarizeFallback bool) {
	playbookPath := PlaybookPath(o.cfg)

	msgs, err := o.messages.List(ctx, sessionID)
	if err != nil || len(msgs) == 0 {
		slog.Debug("ACE update skipped: no messages", "session_id", sessionID, "reason", reason, "error", err)
		return
	}

	pbBefore, _ := o.store.Load(playbookPath)
	beforeTotal := len(pbBefore.KeyPoints)
	beforePending := countPending(pbBefore.KeyPoints)

	// Preferred: LLM extraction + evaluation from the session transcript.
	if o.model != nil {
		gen := NewFantasyTextGenerator(o.model)
		changed, updateErr := UpdateFromSessionMessages(ctx, o.store, playbookPath, sessionTitle, msgs, gen)
		if updateErr == nil {
			if changed {
				pbAfter, _ := o.store.Load(playbookPath)
				slog.Info(
					"ACE playbook updated (LLM)",
					"session_id", sessionID,
					"reason", reason,
					"messages", len(msgs),
					"playbook_path", playbookPath,
					"before_total", beforeTotal,
					"after_total", len(pbAfter.KeyPoints),
					"before_pending", beforePending,
					"after_pending", countPending(pbAfter.KeyPoints),
				)
			} else {
				slog.Debug("ACE update: no changes (LLM)", "session_id", sessionID, "reason", reason, "messages", len(msgs))
			}
			return
		}
		slog.Debug("ACE update failed (LLM)", "reason", reason, "error", updateErr)
	}

	// Optional fallback: use Crush summarization + heuristic extraction.
	if !allowSummarizeFallback || o.summarize == nil {
		return
	}
	slog.Debug("ACE update: running summarize fallback", "session_id", sessionID, "reason", reason)
	if err := o.summarize.Summarize(ctx, sessionID); err != nil {
		slog.Debug("ACE summarize fallback failed", "reason", reason, "error", err)
		return
	}
	updatedSession, err := o.sessions.Get(ctx, sessionID)
	if err != nil || updatedSession.SummaryMessageID == "" {
		slog.Debug("ACE summarize fallback missing summary", "session_id", sessionID, "reason", reason, "error", err)
		return
	}
	summaryMsg, err := o.messages.Get(ctx, updatedSession.SummaryMessageID)
	if err != nil {
		slog.Debug("ACE summarize fallback: summary message load failed", "session_id", sessionID, "reason", reason, "error", err)
		return
	}
	summaryText := summaryMsg.Content().Text
	if summaryText == "" {
		slog.Debug("ACE summarize fallback: empty summary", "session_id", sessionID, "reason", reason)
		return
	}
	changed, err := UpdateFromSessionSummary(o.store, playbookPath, updatedSession.Title, summaryText)
	if err != nil {
		slog.Debug("ACE update failed (summary fallback)", "reason", reason, "error", err)
		return
	}
	if changed {
		pbAfter, _ := o.store.Load(playbookPath)
		slog.Info(
			"ACE playbook updated (summary fallback)",
			"session_id", sessionID,
			"reason", reason,
			"playbook_path", playbookPath,
			"before_total", beforeTotal,
			"after_total", len(pbAfter.KeyPoints),
			"before_pending", beforePending,
			"after_pending", countPending(pbAfter.KeyPoints),
		)
	} else {
		slog.Debug("ACE update: no changes (summary fallback)", "session_id", sessionID, "reason", reason)
	}
}

func countPending(kps []KeyPoint) int {
	n := 0
	for _, kp := range kps {
		if kp.Pending {
			n++
		}
	}
	return n
}
