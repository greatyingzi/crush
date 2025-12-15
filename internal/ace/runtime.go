package ace

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
)

type Runtime struct {
	cfg       *config.Config
	store     Store
	selector  Selector
	formatter Formatter
	analyzer  *TaskGuidanceAnalyzer

	smallModel fantasy.LanguageModel
	messages   message.Service
}

func NewRuntime(cfg *config.Config) *Runtime {
	return &Runtime{
		cfg:       cfg,
		store:     NewFileStore(),
		selector:  NewDefaultSelector(),
		formatter: NewDefaultFormatter(),
		analyzer:  NewTaskGuidanceAnalyzer(0.5),
	}
}

func (r *Runtime) WithSmallModel(model fantasy.LanguageModel) *Runtime {
	r.smallModel = model
	return r
}

func (r *Runtime) WithMessageService(messages message.Service) *Runtime {
	r.messages = messages
	return r
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

func (r *Runtime) WithAnalyzer(analyzer *TaskGuidanceAnalyzer) *Runtime {
	r.analyzer = analyzer
	return r
}

func (r *Runtime) Prefix(ctx context.Context, basePrefix, sessionID, prompt, workingDir, model, provider string) (string, error) {
	// workingDir, model, and provider are currently unused parameters but kept for API compatibility
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

	// Filter playbook to eligible keypoints for injection (matches selector behavior).
	filtered := Playbook{KeyPoints: make([]KeyPoint, 0, len(pb.KeyPoints))}
	for _, kp := range pb.KeyPoints {
		if kp.Pending || strings.TrimSpace(kp.Text) == "" {
			continue
		}
		if kp.Score < aceCfg.MinScore {
			continue
		}
		filtered.KeyPoints = append(filtered.KeyPoints, kp)
	}

	// Determine optimal temperature based on task characteristics
	optimalTemp := DetermineOptimalTemperature(prompt)
	
	// Use intelligent selector with temperature-driven selection
	intelligentSelector := NewIntelligentSelector(optimalTemp)
	
	// IMPORTANT: selection must be prompt-dependent. The original ACE implementation
	// generates tags from the user's prompt (and optionally via LLM) and selects
	// relevant keypoints based on those tags. Passing playbook tags here makes the
	// selection effectively constant across prompts.
	existingTags := make([]string, 0, len(filtered.KeyPoints)*2)
	for _, kp := range filtered.KeyPoints {
		existingTags = append(existingTags, kp.Tags...)
	}
	existingTags = normalizeTags(existingTags)

	desiredTags := inferTagsFromText(prompt, 6)
	if r.analyzer != nil {
		desiredTags = normalizeTags(r.analyzer.generateTags(prompt, existingTags).FinalTags)
	}

	// If enabled and we have a model, perform ACE's original two-phase workflow:
	// 1) LLM tags + injection settings, 2) LLM guidance recommending KPT IDs.
	taskGuidanceEnabled := aceCfg.TaskGuidance == nil || *aceCfg.TaskGuidance
	if taskGuidanceEnabled && r.smallModel != nil {
		injected, injectedOK := r.twoPhaseInject(ctx, sessionID, prompt, filtered, existingTags)
		if injectedOK {
			if strings.TrimSpace(injected) == "" {
				return basePrefix, nil
			}
			if strings.TrimSpace(basePrefix) == "" {
				return injected, nil
			}
			return strings.TrimSpace(basePrefix) + "\n\n" + injected, nil
		}
	}

	selected := intelligentSelector.Select(filtered, desiredTags, aceCfg.MaxItems)
	if aceCfg.MaxChars > 0 {
		selected = r.formatter.TrimToMaxChars(selected, aceCfg.MaxChars)
	}
	
	// Format selected memory
	memory := strings.TrimSpace(r.formatter.Format(selected))
	if memory == "" {
		return basePrefix, nil
	}
	
	if strings.TrimSpace(basePrefix) == "" {
		return memory, nil
	}
	return strings.TrimSpace(basePrefix) + "\n\n" + memory, nil
}

const taskGuidanceWithKPTsTemplate = `You are analyzing a user request within a multi-agent system that has already matched relevant knowledge points.

## Context

### User Request
` + "```" + `
{prompt}
` + "```" + `

### Conversation History (most recent)
` + "```json" + `
{conversation}
` + "```" + `

### Tags Generated
{tags}

### Previously Matched Key Points
{has_keypoints}

Format of key points: ` + "`ID: content`" + `
{matched_keypoints}

## Your Task

Generate task guidance that is **context-aware** of matched key points. The guidance should:

1. **Leverage Matched Knowledge**: Reference or build upon insights from matched key points when relevant
2. **Avoid Redundancy**: Don't repeat what's already in key points unless it's to emphasize or clarify
3. **Fill Gaps**: Provide guidance that complements key points, not replaces them
4. **Coordinate Multi-Agent Work**: Guide the user on which agents to use and in what sequence

## Guidance Strategy

### If key points were matched:
- Focus on **how to apply** knowledge from those points
- Suggest specific **workflow steps** that incorporate matched insights
- Identify any **missing information** that key points don't cover
- Recommend which **specialized agents** to use based on matched knowledge

### If no key points were matched:
- Provide more **foundational guidance**
- Suggest whether to use **exploration agent** first to understand the codebase
- Recommend whether this is a **simple task** that doesn't need multi-agent coordination

## Output Format

Return a JSON object with this exact structure:

{
  "task_guidance": {
    "complexity": "simple|moderate|complex",
    "brief_guidance": "Concise guidance that builds on or complements matched key points. Focus on actionable steps and agent coordination."
  },
  "recommended_kpt_ids": [
    "kpt_001",
    "kpt_002"
  ]
}

Note for recommended_kpt_ids:
- Return an empty array [] if no key points should be injected
- Include ONLY the kpt IDs of key points that are directly relevant to this specific task
- Prefer fewer, more relevant key points over many loosely related ones
- Maximum 5 recommended kpt IDs
- If multiple key points are very similar, choose only the most relevant one

## Additional Instructions

- Keep guidance **brief and actionable** (2-3 sentences max)
- Mention **specific agents** when relevant (exploration, testing, design, analysis) - use generic descriptions
- Consider **existing key points** as foundational knowledge that doesn't need to be repeated
- Focus on **workflow and process** rather than technical details`

type guidanceWithKPTsResponse struct {
	TaskGuidance       TaskGuidance `json:"task_guidance"`
	RecommendedKPTIDs  []string     `json:"recommended_kpt_ids"`
}

func (r *Runtime) twoPhaseInject(ctx context.Context, sessionID, prompt string, filtered Playbook, existingTags []string) (string, bool) {
	aceCfg := r.cfg.Options.ACE
	if aceCfg == nil {
		return "", false
	}

	conv := r.conversationTurns(ctx, sessionID, 12)

	// Phase 1: generate tags/settings.
	phase1Prompt := GenerateTaskGuidancePrompt(conv, prompt, existingTags)
	phase1Text, err := r.generateWithSmallModel(ctx, phase1Prompt, 1024, 10*time.Second)
	if err != nil {
		return "", false
	}

	var phase1 TaskGuidanceResponse
	if err := json.Unmarshal([]byte(ExtractJSONBlock(phase1Text)), &phase1); err != nil {
		return "", false
	}

	tags := normalizeTags(phase1.Tags.FinalTags)
	temp := phase1.InjectionSettings.Temperature
	if temp <= 0 {
		temp = DetermineOptimalTemperature(prompt)
	}

	selector := NewIntelligentSelector(temp)
	selectedForContext := selector.Select(filtered, tags, 25)

	matchedKeyPointsText := ""
	if len(selectedForContext) > 0 {
		var b strings.Builder
		for _, kp := range selectedForContext {
			line := strings.TrimSpace(kp.Text)
			if line == "" || strings.TrimSpace(kp.Name) == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(kp.Name)
			b.WriteString(": ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		matchedKeyPointsText = strings.TrimSpace(b.String())
	}

	conversationJSON, _ := json.MarshalIndent(conv, "", "  ")
	hasKeyPoints := "No"
	if matchedKeyPointsText != "" {
		hasKeyPoints = "Yes"
	}
	phase2Prompt := strings.NewReplacer(
		"{prompt}", prompt,
		"{conversation}", string(conversationJSON),
		"{tags}", strings.Join(tags, ", "),
		"{has_keypoints}", hasKeyPoints,
		"{matched_keypoints}", matchedKeyPointsText,
	).Replace(taskGuidanceWithKPTsTemplate)

	phase2Text, err := r.generateWithSmallModel(ctx, phase2Prompt, 2048, 10*time.Second)
	if err != nil {
		return "", false
	}

	var phase2 guidanceWithKPTsResponse
	if err := json.Unmarshal([]byte(ExtractJSONBlock(phase2Text)), &phase2); err != nil {
		return "", false
	}

	recommended := make([]KeyPoint, 0, 5)
	byID := make(map[string]KeyPoint, len(selectedForContext))
	for _, kp := range selectedForContext {
		if strings.TrimSpace(kp.Name) != "" {
			byID[kp.Name] = kp
		}
	}
	seen := make(map[string]struct{})
	for _, id := range phase2.RecommendedKPTIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		kp, ok := byID[id]
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		recommended = append(recommended, kp)
		if len(recommended) >= 5 {
			break
		}
	}
	if len(recommended) == 0 {
		return "", true // success, but empty injection
	}

	// Format injected content with guidance + selected memory.
	var out strings.Builder
	out.WriteString("ACE Guidance:\n")
	if len(tags) > 0 {
		out.WriteString("- tags: ")
		out.WriteString(strings.Join(tags, ", "))
		out.WriteString("\n")
	}
	out.WriteString(fmt.Sprintf("- temperature: %.2f\n", temp))
	if g := strings.TrimSpace(phase2.TaskGuidance.BriefGuidance); g != "" {
		out.WriteString("- ")
		out.WriteString(g)
		out.WriteString("\n")
	}

	out.WriteString("\nACE Memory (project-local):\n")
	for _, kp := range recommended {
		line := strings.TrimSpace(kp.Text)
		if line == "" {
			continue
		}
		out.WriteString("- [")
		out.WriteString(kp.Name)
		out.WriteString("] ")
		out.WriteString(line)
		out.WriteString("\n")
	}

	final := strings.TrimSpace(out.String())
	if aceCfg.MaxChars > 0 && len(final) > aceCfg.MaxChars {
		final = strings.TrimSpace(final[:aceCfg.MaxChars])
	}

	return final, true
}

func (r *Runtime) conversationTurns(ctx context.Context, sessionID string, limit int) []ConversationTurn {
	if r.messages == nil || strings.TrimSpace(sessionID) == "" || limit <= 0 {
		return nil
	}
	msgs, err := r.messages.List(ctx, sessionID)
	if err != nil {
		return nil
	}

	turns := make([]ConversationTurn, 0, min(limit, len(msgs)))
	// Take the most recent messages.
	for i := len(msgs) - 1; i >= 0 && len(turns) < limit; i-- {
		m := msgs[i]
		if m.Role == message.Tool || m.Role == message.System {
			continue
		}
		text := strings.TrimSpace(m.Content().Text)
		if text == "" {
			continue
		}
		turns = append(turns, ConversationTurn{Role: string(m.Role), Content: text})
	}

	// Reverse to oldest→newest order.
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns
}

func (r *Runtime) generateWithSmallModel(ctx context.Context, prompt string, maxOutTokens int64, timeout time.Duration) (string, error) {
	if r.smallModel == nil {
		return "", fmt.Errorf("missing small model")
	}
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	agent := fantasy.NewAgent(
		r.smallModel,
		fantasy.WithMaxOutputTokens(maxOutTokens),
	)
	resp, err := agent.Stream(callCtx, fantasy.AgentStreamCall{
		Prompt: prompt,
	})
	if err != nil {
		return "", err
	}
	return resp.Response.Content.Text(), nil
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
