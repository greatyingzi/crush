package ace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		systemPrompt: llmSystemPrompt,
		maxOutTokens: 2000,
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
	KeyPoints []KeyPoint `json:"key_points"`
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

	prompt, conv := buildExtractionPrompt(sessionTitle, msgs)
	respText, err := gen.Generate(ctx, prompt)
	if err != nil {
		return false, err
	}

	extracted, err := parseKeyPointsFromLLM(respText)
	if err != nil {
		return false, fmt.Errorf("parse LLM extraction: %w", err)
	}
	if len(extracted) == 0 {
		_ = conv
		return false, nil
	}

	pb, err := store.Load(playbookPath)
	if err != nil {
		return false, err
	}
	pb.Normalize()

	changed := false
	for _, kp := range extracted {
		kp.Text = strings.TrimSpace(kp.Text)
		if kp.Text == "" {
			continue
		}
		kp.Tags = normalizeTags(kp.Tags)
		if len(kp.Tags) == 0 {
			kp.Tags = inferTagsFromText(kp.Text, 6)
		}
		if kp.EffectRating == nil {
			v := inferEffectRatingFromScore(kp.Score)
			kp.EffectRating = &v
		}
		if kp.RiskLevel == nil {
			v := inferRiskFromText(kp.Text)
			kp.RiskLevel = &v
		}
		if kp.InnovationLevel == nil {
			v := inferInnovationFromText(kp.Text)
			kp.InnovationLevel = &v
		}

		if mergeOrAddKeyPoint(&pb, kp) {
			changed = true
		}
	}

	if !changed {
		return false, nil
	}

	pb.KeyPoints = cleanupKeyPoints(pb.KeyPoints)
	if len(pb.KeyPoints) > maxKeyPoints {
		pb.Sort()
		pb.KeyPoints = pb.KeyPoints[:maxKeyPoints]
	}
	return true, store.SaveAtomic(playbookPath, pb)
}

type conversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildExtractionPrompt(sessionTitle string, msgs []message.Message) (prompt string, conv []conversationTurn) {
	const maxTurns = 24

	start := 0
	if len(msgs) > maxTurns {
		start = len(msgs) - maxTurns
	}
	for _, m := range msgs[start:] {
		text := strings.TrimSpace(m.Content().Text)
		if text == "" {
			continue
		}
		conv = append(conv, conversationTurn{
			Role:    string(m.Role),
			Content: text,
		})
	}

	convJSON, _ := json.MarshalIndent(conv, "", "  ")
	title := strings.TrimSpace(sessionTitle)
	if title == "" {
		title = "Untitled"
	}

	var b strings.Builder
	b.WriteString("Session title: ")
	b.WriteString(title)
	b.WriteString("\n\nConversation (JSON):\n")
	b.Write(convJSON)
	b.WriteString("\n\nReturn JSON only.")
	return b.String(), conv
}

func parseKeyPointsFromLLM(respText string) ([]KeyPoint, error) {
	respText = strings.TrimSpace(respText)
	if respText == "" {
		return nil, errors.New("empty response")
	}

	jsonText := extractJSONBlock(respText)
	var r llmExtractionResponse
	if err := json.Unmarshal([]byte(jsonText), &r); err != nil {
		return nil, err
	}

	out := make([]KeyPoint, 0, len(r.KeyPoints))
	for _, kp := range r.KeyPoints {
		kp.Text = strings.TrimSpace(kp.Text)
		if kp.Text == "" {
			continue
		}
		out = append(out, kp)
	}
	return out, nil
}

func extractJSONBlock(s string) string {
	// Prefer fenced ```json blocks.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	// Fallback to any fenced ``` block.
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + len("```")
		if end := strings.Index(s[start:], "```"); end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}

	// Otherwise, try to locate the first '{' and last '}'.
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first >= 0 && last > first {
		return strings.TrimSpace(s[first : last+1])
	}
	return strings.TrimSpace(s)
}

const llmSystemPrompt = `
You are an Agentic Context Engineering (ACE) memory engine.

Task: extract reusable project key points from the conversation.

Output JSON ONLY with this shape:
{
  "key_points": [
    {
      "text": "string, imperative and reusable, no markdown bullets",
      "tags": ["lower_snake_or_kebab_case"],
      "score": -3|0|1,
      "effect_rating": number 0..1,
      "risk_level": number -1..1,
      "innovation_level": number 0..1,
      "pending": false
    }
  ]
}

Scoring: useful=1, neutral=0, harmful=-3.
Keep at most 12 key_points. Prefer deduplicated, concrete guidance.
`
