package ace

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ContextAnalysis represents the analysis of user request for context continuity
type ContextAnalysis struct {
	ContinuityScore    float64 `json:"continuity_score"`
	TaskRelevance      string  `json:"task_relevance"`
	CurrentContext     string  `json:"current_engineering_context"`
	Reasoning          string  `json:"reasoning"`
}

// TaskGuidance represents structured guidance for the agent
type TaskGuidance struct {
	Complexity       string  `json:"complexity"`
	BriefGuidance    string  `json:"brief_guidance"`
	ContextReminder  *string `json:"context_reminder"`
	UserChoicePrompt *string `json:"user_choice_prompt"`
}

// InjectionSettings represents optimal knowledge injection settings
type InjectionSettings struct {
	Temperature float64 `json:"temperature"`
	Reasoning  string  `json:"reasoning"`
}

// TaskGuidanceResponse represents the complete analysis response
type TaskGuidanceResponse struct {
	ContextAnalysis   ContextAnalysis   `json:"context_analysis"`
	Tags             TagsResponse      `json:"tags"`
	InjectionSettings InjectionSettings `json:"injection_settings"`
	TaskGuidance      TaskGuidance      `json:"task_guidance"`
}

type TagsResponse struct {
	FinalTags []string `json:"final_tags"`
	Reasoning string   `json:"reasoning"`
}

// IntelligentSelector implements the sophisticated selection algorithm from Python
type IntelligentSelector struct {
	temperature float64
}

func NewIntelligentSelector(temperature float64) *IntelligentSelector {
	return &IntelligentSelector{temperature: temperature}
}

func (s *IntelligentSelector) Select(pb Playbook, tags []string, limit int) []KeyPoint {
	if limit <= 0 {
		return nil
	}
	if len(pb.KeyPoints) == 0 {
		return nil
	}

	// Apply dual-layer classification with temperature-driven allocation
	return s.dualLayerSelection(pb, tags, limit)
}

// dualLayerSelection implements the core Python selection logic
func (s *IntelligentSelector) dualLayerSelection(pb Playbook, tags []string, limit int) []KeyPoint {
	desiredTags := s.normalizeTags(tags)
	promptTagSet := make(map[string]struct{}, len(desiredTags))
	for _, tag := range desiredTags {
		promptTagSet[tag] = struct{}{}
	}

	// Constants from Python implementation
	HIGH_CONFIDENCE_THRESHOLD := 2.0

	// Classification phase: separate into two distinct layers
	var highConfidenceLayer []scoredKeyPoint
	var recommendationLayer []scoredKeyPoint

	for _, kp := range pb.KeyPoints {
		kpTags := normalizeTags(kp.Tags)
		score, coverage, promptHits := s.scoreAndCoverage(kpTags, desiredTags, promptTagSet)
		
		// Skip negative scoring items entirely
		if kp.Score < 0 || score <= 0 || coverage <= 0 {
			continue
		}

		baseWeight := 10*coverage + 3*score + 5*promptHits + kp.Score

		// Layer-specific temperature application
		layerType := ""
		var tempMultiplier float64

		if float64(kp.Score) >= HIGH_CONFIDENCE_THRESHOLD {
			// Layer 1: High-Confidence Matching
			layerType = "HIGH_CONFIDENCE"
			
			// Temperature affects high-confidence items INVERSELY
			// Low temperature = HIGH weight for proven solutions
			tempMultiplier = 2.5 - s.temperature*1.5

			// Additional layer-specific adjustments
			if s.temperature <= 0.3 {
				tempMultiplier += 0.5 // Conservative boost for proven items
			} else if s.temperature >= 0.7 {
				tempMultiplier -= 0.3 // Exploratory mode reduces proven item weight
			}
		} else {
			// Layer 2: Recommendation-Based
			layerType = "RECOMMENDATION"
			
			// Temperature affects recommendation items DIRECTLY
			// High temperature = HIGH weight for exploration
			tempMultiplier = s.temperature * 2.0

			// Additional layer-specific adjustments
			if s.temperature <= 0.3 {
				tempMultiplier *= 0.3 // Conservative suppresses recommendations
			} else if s.temperature >= 0.7 {
				tempMultiplier += 0.5 // Exploratory boosts recommendations
			}
		}

		// Apply multi-dimensional adjustments
		effectRating := s.getOrDefault(kp.EffectRating, 0.5)
		riskLevel := s.getOrDefault(kp.RiskLevel, -0.5)
		innovationLevel := s.getOrDefault(kp.InnovationLevel, 0.5)

		// Context-aware parameter weights
		effectWeight, innovationWeight, riskThreshold := s.getContextualWeights(layerType, desiredTags)

		if layerType == "HIGH_CONFIDENCE" {
			// High confidence items get effectiveness boost
			tempMultiplier += effectRating * effectWeight
			// Risk reduction for proven items
			if riskLevel < riskThreshold {
				tempMultiplier += 0.2
			}
		} else {
			// Recommendations get innovation boost
			tempMultiplier += innovationLevel * innovationWeight
			// Risk awareness for new ideas
			if riskLevel > riskThreshold {
				tempMultiplier *= 0.8
			}
		}

		// Create scored keypoint
		scoredKp := scoredKeyPoint{
			KeyPoint:        kp,
			BaseWeight:      float64(baseWeight),
			TempMultiplier:  tempMultiplier,
			TotalMatch:      float64(baseWeight) * tempMultiplier,
			MatchScore:      score,
			MatchCoverage:    coverage,
			PromptHits:      promptHits,
			LayerType:       layerType,
		}

		// Classify into correct layer
		if layerType == "HIGH_CONFIDENCE" {
			highConfidenceLayer = append(highConfidenceLayer, scoredKp)
		} else {
			recommendationLayer = append(recommendationLayer, scoredKp)
		}
	}

	// Temperature-based allocation phase
	highConfidenceLimit, recommendationLimit := s.calculateAllocation(s.temperature, limit)

	// Sort each layer internally
	sortedHighConfidence := s.sortLayer(highConfidenceLayer, highConfidenceLimit)
	sortedRecommendations := s.sortLayer(recommendationLayer, recommendationLimit)

	// Merge with layer priority
	finalSelection := make([]KeyPoint, 0, limit)
	
	// Layer-specific ranking: interleave high-confidence and recommendations
	maxItems := max(len(sortedHighConfidence), len(sortedRecommendations))
	for i := 0; i < maxItems && len(finalSelection) < limit; i++ {
		if i < len(sortedHighConfidence) {
			finalSelection = append(finalSelection, sortedHighConfidence[i].KeyPoint)
		}
		if i < len(sortedRecommendations) && len(finalSelection) < limit {
			finalSelection = append(finalSelection, sortedRecommendations[i].KeyPoint)
		}
	}

	return finalSelection
}

type scoredKeyPoint struct {
	KeyPoint
	BaseWeight     float64
	TempMultiplier float64
	TotalMatch     float64
	MatchScore     int
	MatchCoverage   int
	PromptHits     int
	LayerType      string
}

func (s *IntelligentSelector) normalizeTags(tags []string) []string {
	var normalized []string
	for _, tag := range tags {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			normalized = append(normalized, tag)
		}
	}
	return normalized
}

func (s *IntelligentSelector) scoreAndCoverage(kpTags []string, desiredTags []string, promptTagSet map[string]struct{}) (int, int, int) {
	best := 0
	matched := make(map[string]struct{})
	promptHits := 0

	for _, kpTag := range kpTags {
		kpNorm := strings.ToLower(kpTag)
		for _, desired := range desiredTags {
			score := s.tagMatchScore(kpNorm, desired)
			if score > 0 {
				matched[desired] = struct{}{}
				if _, ok := promptTagSet[desired]; ok {
					promptHits++
				}
				best = max(best, score)
				if best == 3 && len(matched) == len(desiredTags) {
					return best, len(matched), promptHits
				}
			}
		}
	}

	return best, len(matched), promptHits
}

// tagMatchScore implements Python's three-tier matching: exact=3, substring=2, token=1
func (s *IntelligentSelector) tagMatchScore(kpTag, desired string) int {
	kpTag = strings.ToLower(kpTag)
	desired = strings.ToLower(desired)

	if kpTag == desired {
		return 3 // Exact match
	}

	if strings.Contains(kpTag, desired) || strings.Contains(desired, kpTag) {
		return 2 // Substring match
	}

	// Token overlap
	kpTokens := s.tokenize(kpTag)
	desiredTokens := s.tokenize(desired)
	if len(kpTokens) == 0 || len(desiredTokens) == 0 {
		return 0
	}

	for _, kpTok := range kpTokens {
		for _, desiredTok := range desiredTokens {
			if kpTok == desiredTok {
				return 1 // Token match
			}
		}
	}

	return 0
}

var tokenRegex = regexp.MustCompile(`[a-z0-9]+`)

func (s *IntelligentSelector) tokenize(text string) []string {
	matches := tokenRegex.FindAllString(strings.ToLower(text), -1)
	var tokens []string
	for _, match := range matches {
		if match != "" {
			tokens = append(tokens, match)
		}
	}
	return tokens
}

func (s *IntelligentSelector) getOrDefault(ptr *float64, defaultValue float64) float64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

func (s *IntelligentSelector) getContextualWeights(layerType string, tags []string) (effectWeight, innovationWeight float64, riskThreshold float64) {
	// Default weights
	effectWeight = 0.3
	innovationWeight = 0.2
	riskThreshold = -0.3

	// Context-aware adjustments based on tags
	tagsText := strings.Join(tags, " ")
	
	if strings.Contains(tagsText, "security") || strings.Contains(tagsText, "safety") {
		effectWeight = 0.5
		riskThreshold = -0.7 // Higher safety requirements
		innovationWeight = 0.1
	} else if strings.Contains(tagsText, "performance") || strings.Contains(tagsText, "optimization") {
		effectWeight = 0.4
		innovationWeight = 0.3
		riskThreshold = -0.2 // More risk tolerant for performance
	} else if strings.Contains(tagsText, "experimental") || strings.Contains(tagsText, "research") {
		effectWeight = 0.2
		innovationWeight = 0.5
		riskThreshold = 0.0 // Risk-neutral for research
	}

	return effectWeight, innovationWeight, riskThreshold
}

func (s *IntelligentSelector) calculateAllocation(temperature float64, limit int) (highConfidenceLimit, recommendationLimit int) {
	if temperature <= 0.3 {
		// CONSERVATIVE: Prioritize proven solutions
		highConfidenceLimit = max(4, int(float64(limit)*0.7)) // Up to 70%
		recommendationLimit = max(1, limit-highConfidenceLimit)   // At least 1
	} else if temperature >= 0.7 {
		// EXPLORATORY: Prioritize new ideas
		recommendationLimit = max(4, int(float64(limit)*0.7)) // Up to 70%
		highConfidenceLimit = max(1, limit-recommendationLimit) // At least 1
	} else {
		// BALANCED: Equal allocation
		highConfidenceLimit = limit / 2
		recommendationLimit = limit - highConfidenceLimit
	}

	return highConfidenceLimit, recommendationLimit
}

func (s *IntelligentSelector) sortLayer(layer []scoredKeyPoint, limit int) []scoredKeyPoint {
	sort.Slice(layer, func(i, j int) bool {
		return layer[i].TotalMatch > layer[j].TotalMatch
	})

	if len(layer) <= limit {
		return layer
	}
	return layer[:limit]
}

// AnalyzeContextContinuity evaluates the continuity between current work and new request
func AnalyzeContextContinuity(currentContext, newRequest string) ContextAnalysis {
	// Simple heuristic implementation - could be enhanced with more sophisticated NLP
	continuityScore := calculateContinuityScore(currentContext, newRequest)
	
	taskRelevance := "HIGH"
	if continuityScore >= 0.8 {
		taskRelevance = "HIGH"
	} else if continuityScore >= 0.4 {
		taskRelevance = "MEDIUM"
	} else {
		taskRelevance = "LOW"
	}

	return ContextAnalysis{
		ContinuityScore: continuityScore,
		TaskRelevance:   taskRelevance,
		CurrentContext:  currentContext,
		Reasoning:       "Based on lexical similarity and topic overlap",
	}
}

// calculateContinuityScore computes continuity between contexts
func calculateContinuityScore(current, new string) float64 {
	if current == "" && new == "" {
		return 1.0
	}
	if current == "" || new == "" {
		return 0.0
	}

	currentTokens := tokenize(strings.ToLower(current))
	newTokens := tokenize(strings.ToLower(new))

	if len(currentTokens) == 0 && len(newTokens) == 0 {
		return 1.0
	}
	if len(currentTokens) == 0 || len(newTokens) == 0 {
		return 0.0
	}

	// Calculate Jaccard similarity
	currentSet := make(map[string]struct{})
	for _, token := range currentTokens {
		currentSet[token] = struct{}{}
	}

	intersection := 0
	for _, token := range newTokens {
		if _, ok := currentSet[token]; ok {
			intersection++
		}
	}

	union := len(currentSet) + len(newTokens) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// DetermineOptimalTemperature selects temperature based on task characteristics
func DetermineOptimalTemperature(prompt string) float64 {
	prompt = strings.ToLower(prompt)
	
	// Look for urgent keywords - low temperature
	urgentKeywords := []string{"fix", "bug", "error", "urgent", "critical", "emergency"}
	for _, keyword := range urgentKeywords {
		if strings.Contains(prompt, keyword) {
			return 0.2 // Low temperature for urgent fixes
		}
	}

	// Look for exploratory keywords - high temperature
	exploratoryKeywords := []string{"explore", "learn", "alternative", "research", "experiment", "creative", "brainstorm"}
	for _, keyword := range exploratoryKeywords {
		if strings.Contains(prompt, keyword) {
			return 0.8 // High temperature for exploration
		}
	}

	// Consider task complexity
	if strings.Contains(prompt, "complex") || strings.Contains(prompt, "comprehensive") || strings.Contains(prompt, "design") {
		return 0.6 // Moderate-high for complex tasks
	}

	return 0.5 // Default balanced temperature
}

// GenerateContextReminder creates a gentle reminder for low continuity tasks
func GenerateContextReminder(currentTask, newRequest string) string {
	return fmt.Sprintf("Context Reminder: We're currently working on %s.\nYour new request '%s' appears to be unrelated.\n\nWould you like to:\n1. Continue with current engineering work\n2. Pause and address this new request\n3. Save current progress and start fresh\n4. Merge both if related", 
		currentTask, newRequest)
}

// GenerateContextBridge creates a bridge message for medium continuity tasks
func GenerateContextBridge(currentTask, newRequest string) string {
	return fmt.Sprintf("I notice we're shifting from %s to %s. Should we pause current work to address this, or continue with our original task?", 
		currentTask, newRequest)
}