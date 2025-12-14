package ace

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// TaskGuidanceAnalyzer implements intelligent task analysis and guidance generation
type TaskGuidanceAnalyzer struct {
	temperature float64
}

func NewTaskGuidanceAnalyzer(temperature float64) *TaskGuidanceAnalyzer {
	return &TaskGuidanceAnalyzer{temperature: temperature}
}

// AnalyzeTask performs comprehensive task analysis based on conversation and prompt
func (t *TaskGuidanceAnalyzer) AnalyzeTask(conversation []ConversationTurn, prompt string, existingTags []string) (TaskGuidanceResponse, error) {
	// Part 1: Context continuity analysis
	contextAnalysis := t.analyzeContextContinuity(conversation, prompt)
	
	// Part 2: Tag generation
	tags := t.generateTags(prompt, existingTags)
	
	// Part 3: Injection settings (temperature)
	injectionSettings := t.determineInjectionSettings(prompt)
	
	// Part 4: Task guidance
	guidance := t.generateTaskGuidance(contextAnalysis.TaskRelevance, prompt)
	
	return TaskGuidanceResponse{
		ContextAnalysis:   contextAnalysis,
		Tags:             tags,
		InjectionSettings: injectionSettings,
		TaskGuidance:      guidance,
	}, nil
}

type ConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (t *TaskGuidanceAnalyzer) analyzeContextContinuity(conversation []ConversationTurn, prompt string) ContextAnalysis {
	// Extract current engineering context from recent conversation
	currentContext := t.extractCurrentContext(conversation)
	
	// Calculate continuity score
	continuityScore := calculateContinuityScore(currentContext, prompt)
	
	// Determine task relevance level
	taskRelevance := "HIGH"
	if continuityScore >= 0.8 {
		taskRelevance = "HIGH"
	} else if continuityScore >= 0.4 {
		taskRelevance = "MEDIUM"
	} else {
		taskRelevance = "LOW"
	}
	
	reasoning := fmt.Sprintf("Continuity score %.2f based on lexical similarity and topic overlap", continuityScore)
	
	return ContextAnalysis{
		ContinuityScore: continuityScore,
		TaskRelevance:   taskRelevance,
		CurrentContext:  currentContext,
		Reasoning:       reasoning,
	}
}

func (t *TaskGuidanceAnalyzer) extractCurrentContext(conversation []ConversationTurn) string {
	// Extract the most recent technical/engineering content
	var technicalTurns []string
	for i := len(conversation) - 1; i >= 0 && i >= len(conversation)-6; i-- {
		turn := conversation[i]
		if turn.Role == "assistant" && t.isTechnicalContent(turn.Content) {
			technicalTurns = append(technicalTurns, turn.Content)
		}
	}
	
	if len(technicalTurns) == 0 {
		return ""
	}
	
	// Return the most recent technical context (truncate if too long)
	context := technicalTurns[0]
	if len(context) > 200 {
		context = context[:200] + "..."
	}
	return context
}

func (t *TaskGuidanceAnalyzer) isTechnicalContent(content string) bool {
	technicalKeywords := []string{
		"implement", "code", "function", "class", "method", "algorithm",
		"bug", "fix", "error", "test", "debug", "refactor",
		"api", "database", "query", "server", "client",
		"interface", "design", "pattern", "architecture",
	}
	
	content = strings.ToLower(content)
	for _, keyword := range technicalKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func (t *TaskGuidanceAnalyzer) generateTags(prompt string, existingTags []string) TagsResponse {
	// Extract potential tags from prompt
	promptTokens := tokenize(prompt)
	
	var finalTags []string
	
	// First, try to reuse existing tags (80%+ semantic similarity)
	for _, existingTag := range existingTags {
		for _, promptToken := range promptTokens {
			if t.semanticSimilarity(existingTag, promptToken) >= 0.8 {
				if !slices.Contains(finalTags, existingTag) {
					finalTags = append(finalTags, existingTag)
				}
			}
		}
	}
	
	// Add new tags if we have fewer than 5
	if len(finalTags) < 5 {
		for _, token := range promptTokens {
			if len(token) < 3 || t.isStopWord(token) {
				continue
			}
			
			// Check if similar to existing tags
			isSimilar := false
			for _, existingTag := range existingTags {
				if t.semanticSimilarity(existingTag, token) >= 0.8 {
					isSimilar = true
					break
				}
			}
			
			if !isSimilar && !slices.Contains(finalTags, token) {
				finalTags = append(finalTags, token)
				if len(finalTags) >= 5 {
					break
				}
			}
		}
	}
	
	// Limit to 3-5 tags
	if len(finalTags) > 5 {
		finalTags = finalTags[:5]
	} else if len(finalTags) < 3 && len(existingTags) > 0 {
		// If we have too few tags, add some from existing
		for _, tag := range existingTags {
			if !slices.Contains(finalTags, tag) {
				finalTags = append(finalTags, tag)
				if len(finalTags) >= 3 {
					break
				}
			}
		}
	}
	
	reasoning := fmt.Sprintf("Generated %d tags based on semantic similarity to existing tags", len(finalTags))
	
	return TagsResponse{
		FinalTags: finalTags,
		Reasoning: reasoning,
	}
}

func (t *TaskGuidanceAnalyzer) semanticSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	
	if a == b {
		return 1.0
	}
	
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 0.9 // High similarity for substring matches
	}
	
	// Simple token overlap for basic semantic similarity
	aTokens := tokenize(a)
	bTokens := tokenize(b)
	
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0.0
	}
	
	aSet := make(map[string]struct{})
	for _, token := range aTokens {
		aSet[token] = struct{}{}
	}
	
	intersection := 0
	for _, token := range bTokens {
		if _, ok := aSet[token]; ok {
			intersection++
		}
	}
	
	union := len(aSet) + len(bTokens) - intersection
	if union == 0 {
		return 0.0
	}
	
	return float64(intersection) / float64(union)
}

func (t *TaskGuidanceAnalyzer) isStopWord(token string) bool {
	stopWords := []string{
		"a", "an", "and", "are", "as", "at", "be", "but", "by",
		"for", "from", "how", "i", "if", "in", "is", "it", "of", "on",
		"or", "please", "the", "this", "to", "use", "we", "what", "when",
		"with", "you", "your",
	}
	
	return slices.Contains(stopWords, strings.ToLower(token))
}

func (t *TaskGuidanceAnalyzer) determineInjectionSettings(prompt string) InjectionSettings {
	// Determine optimal temperature based on task characteristics
	temperature := DetermineOptimalTemperature(prompt)
	
	reasoning := fmt.Sprintf("Temperature %.1f selected based on task characteristics and urgency level", temperature)
	
	return InjectionSettings{
		Temperature: temperature,
		Reasoning:  reasoning,
	}
}

func (t *TaskGuidanceAnalyzer) generateTaskGuidance(relevance, prompt string) TaskGuidance {
	// Determine task complexity
	complexity := t.assessComplexity(prompt)
	
	// Generate brief guidance
	briefGuidance := t.generateBriefGuidance(complexity, prompt)
	
	var contextReminder *string
	var userChoicePrompt *string
	
	// Generate context-aware messages based on relevance
	switch relevance {
	case "HIGH":
		// No interruption needed for high continuity
	case "MEDIUM":
		// Context bridge for medium continuity
		if len(prompt) > 20 { // Only for substantial requests
			bridge := GenerateContextBridge("current engineering work", prompt)
			userChoicePrompt = &bridge
		}
	case "LOW":
		// Gentle reminder for low continuity
		if len(prompt) > 20 { // Only for substantial requests
			reminder := GenerateContextReminder("current engineering task", prompt)
			contextReminder = &reminder
		}
	}
	
	return TaskGuidance{
		Complexity:       complexity,
		BriefGuidance:    briefGuidance,
		ContextReminder:  contextReminder,
		UserChoicePrompt: userChoicePrompt,
	}
}

func (t *TaskGuidanceAnalyzer) assessComplexity(prompt string) string {
	prompt = strings.ToLower(prompt)
	
	// Simple keywords for complexity assessment
	simpleKeywords := []string{"fix", "add", "show", "get", "list", "check", "run", "execute"}
	moderateKeywords := []string{"implement", "refactor", "optimize", "improve", "update", "modify"}
	complexKeywords := []string{"design", "analyze", "comprehensive", "system", "architecture", "framework"}
	
	for _, keyword := range simpleKeywords {
		if strings.Contains(prompt, keyword) {
			return "simple"
		}
	}
	
	for _, keyword := range moderateKeywords {
		if strings.Contains(prompt, keyword) {
			return "moderate"
		}
	}
	
	for _, keyword := range complexKeywords {
		if strings.Contains(prompt, keyword) {
			return "complex"
		}
	}
	
	// Default to moderate for ambiguous requests
	if strings.Contains(prompt, "optimize") || strings.Contains(prompt, "improve") || strings.Contains(prompt, "enhance") {
		return "moderate"
	}
	
	return "simple"
}

func (t *TaskGuidanceAnalyzer) generateBriefGuidance(complexity, prompt string) string {
	switch complexity {
	case "simple":
		return "Directly execute the requested operation using appropriate tools."
	case "moderate":
		return "Break down the task into steps: analyze requirements, implement solution, verify results."
	case "complex":
		return "Use systematic approach: clarify requirements, explore codebase, design solution, implement in phases, verify thoroughly."
	default:
		return "Proceed with structured analysis and implementation."
	}
}

// GenerateTaskGuidancePrompt creates the full prompt for task guidance analysis
func GenerateTaskGuidancePrompt(conversation []ConversationTurn, prompt string, existingTags []string) string {
	// Convert conversation to JSON
	conversationJSON, _ := json.MarshalIndent(conversation, "", "  ")
	
	// Convert existing tags to JSON
	existingTagsJSON, _ := json.Marshal(existingTags)
	
	// Use the template from task_guidance.txt
	template := `# Task Guidance Template

Analyze user's request to: 1) assess context continuity with current engineering work, 2) derive concise tags, 3) generate structured thinking guidance, and 4) assess task characteristics for optimal knowledge injection.

## Core Principle: Engineering Context Protection
**CRITICAL**: Your primary responsibility is maintaining engineering workflow continuity. While being responsive to all requests, you must proactively detect and manage context shifts that could fragment attention or derail ongoing work. Balance responsiveness with focus protection through intelligent guidance.

# Conversation (recent messages)
%s

# Pending Prompt (highest priority)
%s

# Part 1: Tag Generation
## Existing Playbook Tags for Reference
%s

## Tag Generation Rules
- Limit final_tags to 3-5 most relevant tags
- Prefer existing tags when they match (80%+ similarity)
- Create new tags only when necessary
- Standardize format: lowercase, ascii, max 64 chars
- All tags must be single words or simple hyphenated terms

# Part 2: Task Type Assessment for Knowledge Injection

## Context Continuity Detection
Before generating guidance, assess if user's request maintains continuity with current engineering work:

**Detection Factors:**
- **Relevance Score**: How related is this request to previous engineering tasks (0.0-1.0)?
- **Task Shift**: Is this a major departure from current work context? (creative/personal vs technical)
- **Urgency Level**: Is there an immediate need vs casual exploration?
- **Session State**: Are we in middle of active engineering work?

**Continuity Assessment:**
- HIGH (0.8-1.0): Direct continuation of current engineering work
- MEDIUM (0.4-0.7): Related but tangential to current work
- LOW (0.0-0.3): Major departure from engineering context

## Intelligent Guidance Mechanism
Based on continuity assessment, choose appropriate response strategy:

### HIGH Continuity (Engineering Focus):
- Proceed with normal task guidance
- No interruption needed
- Maintain engineering workflow

### MEDIUM Continuity (Context Bridge):
- Brief acknowledgment of context shift
- Offer choice: continue current work or address new request
- Example: "I notice we're shifting from [current task] to [new request]. Should we pause current work to address this, or continue with our original task?"

### LOW Continuity (Focus Protection):
- **GENTLE REMINDER** required for major task shifts
- Provide clear options for user choice
- Protect against attention fragmentation

## Temperature Parameter (0.0-1.0)
Assess optimal knowledge injection temperature based on task characteristics:

**Temperature Guidelines:**
- 0.1-0.3: Emergency fixes, critical bugs, precise technical tasks - rely on verified knowledge only
- 0.4-0.6: Routine development, moderate complexity - balance precision with exploration
- 0.7-1.0: Learning, exploration, research, creative tasks - encourage diverse knowledge discovery

**Temperature Selection Rules:**
- Look for urgent keywords: "fix", "bug", "error", "urgent" → Use low temperature (0.1-0.3)
- Look for exploratory keywords: "explore", "learn", "alternative", "research" → Use high temperature (0.7-1.0)
- Consider task complexity: simple → 0.3, moderate → 0.5, complex → 0.7
- Consider project stage: maintenance → 0.3, development → 0.6, innovation → 0.9

# Part 2: Structured Thinking Guidance
## Your Role: Main Agent Orchestrator
**MANDATORY: You are coordinator of a multi-agent system. You MUST proactively plan execution flows:**

**IMPORTANT**: The multi-agent system may have user-defined configurations with custom agent names and capabilities. Be flexible and adapt your guidance to work with available agents rather than assuming fixed agent names.

You are about to receive a user request. Before responding, you need to:

1. **CLARIFY AMBIGUOUS REQUIREMENTS** - Ask questions for ANY uncertainty
2. **PLAN AGENT CALL CHAINS** - Design A→B→C execution flows using appropriate specialized agents (adapting to user configuration)
3. **EXECUTE IN PARALLEL** - Use parallel tool calls whenever possible

### Flexible Execution Patterns (Adapt based on user-defined agent configurations):
- Code understanding: Use exploration agent to analyze → Main Agent synthesizes → [Verification agent]
- New features: Use exploration + design agents → Main Agent implements → Verification agent
- Bug investigation: Use exploration + analysis agents → Main Agent fixes
- UI/UX work: Use design agent + exploration agent → Main Agent implements
- **Note**: Agent availability and names may vary based on user configuration. Adapt patterns accordingly.

### CRITICAL: When to Ask Questions:
- CRITICAL: Seeing vague words like "optimize", "improve", "enhance"
- Multiple valid implementation approaches exist
- Technical choices affect user experience
- Scope is unclear

## Guidance Structure Requirements:
**ALWAYS use imperative language: "You should first X, then Y"**
**CLEARLY specify call order: "dispatch exploration agent → analyze results → implement"**
**MUST ask clarifying questions for ambiguous tasks**
**Break complex tasks into phases: exploration → design → execution → verification**

## Task Complexity Assessment
- **Simple**: Single-step, clear requirements (fix, add, show, get, list)
- **Moderate**: Multi-step with some ambiguity (implement, refactor, optimize)
- **Complex**: Unclear scope, deep analysis needed (design, analyze, comprehensive)

## When to Show Guidance:
- Complex multi-step tasks requiring systematic breakdown
- Ambiguous requests needing clarification
- High-complexity tasks benefitting from structured approach
- Tasks where thinking process should be visible to user

## When to Hide Guidance:
- Simple, direct commands with obvious implementation
- Well-defined single-step operations
- Routine information requests

## Required JSON Output Format
{
  "context_analysis": {
    "continuity_score": 0.X,
    "task_relevance": "HIGH|MEDIUM|LOW",
    "current_engineering_context": "Brief description of current work if applicable",
    "reasoning": "Explanation of continuity assessment"
  },
  "tags": {
    "final_tags": ["tag1", "tag2", "tag3"],
    "reasoning": "Brief explanation of tag selection"
  },
  "injection_settings": {
    "temperature": 0.X,
    "reasoning": "Brief explanation of temperature selection based on task characteristics"
  },
  "task_guidance": {
    "complexity": "simple|moderate|complex",
    "brief_guidance": "2-3 sentences maximum guidance for self-approach",
    "context_reminder": "Optional gentle reminder for low continuity tasks (null if not needed)",
    "user_choice_prompt": "Optional choice prompt for context shifts (null if not needed)"
  }
}

Always respond with valid JSON only.`
	
	return fmt.Sprintf(template, string(conversationJSON), prompt, string(existingTagsJSON))
}