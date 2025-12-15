package ace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/message"
)

type TextGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type FantasyTextGenerator struct {
	model         fantasy.LanguageModel
	systemPrompt  string
	maxOutTokens  int64
	providerOpts  fantasy.ProviderOptions
	temperature   *float64
	disableThink  bool
	responseGuard string
}

func NewFantasyTextGenerator(model fantasy.LanguageModel) *FantasyTextGenerator {
	return &FantasyTextGenerator{
		model:        model,
		systemPrompt: "",
		maxOutTokens: 4096,
	}
}

func (g *FantasyTextGenerator) Generate(ctx context.Context, prompt string) (string, error) {
	if g.model == nil {
		return "", errors.New("missing model")
	}
	agent := fantasy.NewAgent(
		g.model,
		fantasy.WithSystemPrompt(g.systemPrompt),
		fantasy.WithMaxOutputTokens(g.maxOutTokens),
	)
	resp, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt:          prompt,
		ProviderOptions: g.providerOpts,
		Temperature:     g.temperature,
	})
	if err != nil {
		return "", err
	}
	return resp.Response.Content.Text(), nil
}

type llmExtractionResponse struct {
	// Mirrors agentic_context_engineering/src/prompts/reflection.txt.
	MergedKeyPoints []MergedKeyPoint   `json:"merged_key_points"`
	NewKeyPoints    []KeyPoint         `json:"new_key_points"`
	Evaluations     []RatingEvaluation `json:"evaluations"`
	ScoreChanges    []RatingEvaluation `json:"score_changes"` // legacy field name
}

type RatingEvaluation struct {
	Name   string `json:"name"`
	Rating string `json:"rating"`
}

type MergedKeyPoint struct {
	Text            string   `json:"text"`
	Tags            []string `json:"tags"`
	Sources         []string `json:"sources"`
	EffectRating    *float64 `json:"effect_rating,omitempty"`
	RiskLevel       *float64 `json:"risk_level,omitempty"`
	InnovationLevel *float64 `json:"innovation_level,omitempty"`
}

type ReflectionExtraction struct {
	// Nil means field missing from JSON (important: matches Python's `get()` behavior).
	MergedKeyPoints []MergedKeyPoint
	NewKeyPoints    []KeyPoint
	Evaluations     []RatingEvaluation
}

func (r ReflectionExtraction) Empty() bool {
	return r.MergedKeyPoints == nil && len(r.NewKeyPoints) == 0 && len(r.Evaluations) == 0
}

func UpdateFromSessionMessages(
	ctx context.Context,
	store Store,
	playbookPath string,
	sessionTitle string,
	msgs []message.Message,
	gen TextGenerator,
) (bool, error) {
	if gen == nil {
		return false, errors.New("missing generator")
	}
	if len(msgs) == 0 {
		return false, nil
	}

	pb, err := store.Load(playbookPath)
	if err != nil {
		return false, err
	}
	pb.Normalize()

	// sessionTitle is intentionally unused as original ACE doesn't include title in reflection prompt
	_ = sessionTitle
	prompt, _ := buildReflectionPrompt(msgs, pb)
	respText, err := gen.Generate(ctx, prompt)
	if err != nil {
		return false, err
	}

	extraction, err := parseReflectionResponse(respText)
	if err != nil {
		return false, fmt.Errorf("parse LLM extraction: %w", err)
	}
	if extraction.Empty() {
		return false, nil
	}

	updated := UpdatePlaybookData(pb, extraction)
	if playbooksSemanticallyEqual(pb, updated) {
		return false, nil
	}
	return true, store.SaveAtomic(playbookPath, updated)
}

type conversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildReflectionPrompt(msgs []message.Message, pb Playbook) (prompt string, conv []conversationTurn) {
	for _, m := range msgs {
		text := strings.TrimSpace(m.Content().Text)
		if text == "" {
			continue
		}
		conv = append(conv, conversationTurn{
			Role:    string(m.Role),
			Content: text,
		})
	}

	trajectoriesJSON, _ := json.MarshalIndent(conv, "", "  ")

	existing := make(map[string]string)
	pending := make(map[string]string)
	tagSet := make(map[string]struct{})

	for _, kp := range pb.KeyPoints {
		for _, t := range kp.Tags {
			if strings.TrimSpace(t) != "" {
				tagSet[t] = struct{}{}
			}
		}
		if strings.TrimSpace(kp.Name) == "" || strings.TrimSpace(kp.Text) == "" {
			continue
		}
		if kp.Pending {
			pending[kp.Name] = kp.Text
		} else {
			existing[kp.Name] = kp.Text
		}
	}

	existingJSON, _ := json.MarshalIndent(existing, "", "  ")
	pendingJSON, _ := json.MarshalIndent(pending, "", "  ")

	existingTags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		existingTags = append(existingTags, t)
	}
	slices.Sort(existingTags)
	existingTagsJSON, _ := json.Marshal(existingTags)

	// Matches common.py: existing_tags_context = "\n\nExisting tags in playbook: [...]"
	existingTagsContext := "\n\nExisting tags in playbook: " + string(existingTagsJSON)

	replacer := strings.NewReplacer(
		"{trajectories}", string(trajectoriesJSON),
		"{existing_playbook}", string(existingJSON),
		"{pending_playbook}", string(pendingJSON),
		"{existing_tags_context}", existingTagsContext,
	)
	return replacer.Replace(reflectionPromptTemplate), conv
}

func parseReflectionResponse(respText string) (ReflectionExtraction, error) {
	respText = strings.TrimSpace(respText)
	if respText == "" {
		return ReflectionExtraction{}, errors.New("empty response")
	}

	jsonText := ExtractJSONBlock(respText)
	var r llmExtractionResponse
	if err := json.Unmarshal([]byte(jsonText), &r); err != nil {
		return ReflectionExtraction{}, err
	}

	evals := r.Evaluations
	if len(evals) == 0 && len(r.ScoreChanges) > 0 {
		evals = r.ScoreChanges
	}
	return ReflectionExtraction{
		MergedKeyPoints: r.MergedKeyPoints,
		NewKeyPoints:    r.NewKeyPoints,
		Evaluations:     evals,
	}, nil
}


func playbooksSemanticallyEqual(a, b Playbook) bool {
	if len(a.KeyPoints) != len(b.KeyPoints) {
		return false
	}
	for i := range a.KeyPoints {
		ak := a.KeyPoints[i]
		bk := b.KeyPoints[i]
		if ak.Name != bk.Name || ak.Text != bk.Text || ak.Score != bk.Score || ak.Pending != bk.Pending {
			return false
		}
		if !slices.Equal(ak.Tags, bk.Tags) {
			return false
		}
	}
	return true
}
