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

type SessionEndObserver struct {
	cfg       *config.Config
	sessions  session.Service
	messages  message.Service
	summarize Summarizer
	model     fantasy.LanguageModel
	store     Store
}

func NewSessionEndObserver(cfg *config.Config, sessions session.Service, messages message.Service, summarizer Summarizer, model fantasy.LanguageModel) *SessionEndObserver {
	return &SessionEndObserver{
		cfg:       cfg,
		sessions:  sessions,
		messages:  messages,
		summarize: summarizer,
		model:     model,
		store:     NewFileStore(),
	}
}

func (o *SessionEndObserver) OnShutdown(ctx context.Context) {
	if o.cfg == nil || o.cfg.Options == nil || o.cfg.Options.ACE == nil {
		return
	}
	if !o.cfg.Options.ACE.Enabled || !o.cfg.Options.ACE.UpdateOnExit {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	sessions, err := o.sessions.List(shutdownCtx)
	if err != nil || len(sessions) == 0 {
		return
	}
	sess := sessions[0]

	playbookPath := PlaybookPath(o.cfg)

	// Preferred: LLM extraction + evaluation from the session transcript.
	if o.model != nil {
		msgs, err := o.messages.List(shutdownCtx, sess.ID)
		if err == nil && len(msgs) > 0 {
			gen := NewFantasyTextGenerator(o.model)
			changed, updateErr := UpdateFromSessionMessages(shutdownCtx, o.store, playbookPath, sess.Title, msgs, gen)
			if updateErr == nil && changed {
				return
			}
			if updateErr != nil {
				slog.Debug("ACE session-end LLM update failed", "error", updateErr)
			}
		}
	}

	// Fallback: use Crush summarization (if available) + heuristic extraction.
	if o.summarize == nil {
		return
	}
	if err := o.summarize.Summarize(shutdownCtx, sess.ID); err != nil {
		slog.Debug("ACE session-end summarize failed", "error", err)
		return
	}
	updatedSession, err := o.sessions.Get(shutdownCtx, sess.ID)
	if err != nil || updatedSession.SummaryMessageID == "" {
		return
	}
	summaryMsg, err := o.messages.Get(shutdownCtx, updatedSession.SummaryMessageID)
	if err != nil {
		return
	}
	summaryText := summaryMsg.Content().Text
	if summaryText == "" {
		return
	}
	if _, err := UpdateFromSessionSummary(o.store, playbookPath, updatedSession.Title, summaryText); err != nil {
		slog.Debug("ACE session-end update failed", "error", err)
	}
}
