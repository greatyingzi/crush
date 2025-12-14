package ace

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// UpdatePlaybookData is a close port of
// agentic_context_engineering/src/hooks/playbook_engine.py:update_playbook_data.
func UpdatePlaybookData(pb Playbook, extraction ReflectionExtraction) Playbook {
	pb.Normalize()

	applyRatingEvaluations(&pb, extraction.Evaluations)

	if extraction.MergedKeyPoints != nil {
		pb = applyMergedKeyPoints(pb, extraction.MergedKeyPoints)
	} else {
		pb = applyNewKeyPointsAsPending(pb, extraction.NewKeyPoints)
	}

	// Drop low-score items (strictly greater than -5).
	out := pb.KeyPoints[:0]
	for _, kp := range pb.KeyPoints {
		if kp.Score > -5 {
			out = append(out, kp)
		}
	}
	pb.KeyPoints = out

	// Enforce a hard cap by score to keep playbook size bounded.
	if len(pb.KeyPoints) > maxKeyPoints {
		slices.SortFunc(pb.KeyPoints, func(a, b KeyPoint) int {
			if a.Score != b.Score {
				if a.Score > b.Score {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		})
		pb.KeyPoints = pb.KeyPoints[:maxKeyPoints]
	}

	// Renumber sequentially to keep identifiers compact.
	for idx := range pb.KeyPoints {
		pb.KeyPoints[idx].Name = fmt.Sprintf("kpt_%03d", idx+1)
	}

	pb.Normalize()
	return pb
}

func applyRatingEvaluations(pb *Playbook, evaluations []RatingEvaluation) {
	if pb == nil || len(evaluations) == 0 {
		return
	}

	// Enhanced 7-level rating system (matches Python).
	ratingDelta := map[string]int{
		"highly_effective":   3,
		"moderately_useful":  2,
		"slightly_useful":    1,
		"neutral":            0,
		"slightly_harmful":   -1,
		"moderately_harmful": -2,
		"highly_dangerous":   -4,
	}

	nameToIndex := make(map[string]int, len(pb.KeyPoints))
	for i, kp := range pb.KeyPoints {
		if strings.TrimSpace(kp.Name) != "" {
			nameToIndex[kp.Name] = i
		}
	}

	for _, ev := range evaluations {
		name := strings.TrimSpace(ev.Name)
		if name == "" {
			continue
		}
		idx, ok := nameToIndex[name]
		if !ok {
			continue
		}
		delta, ok := ratingDelta[strings.TrimSpace(ev.Rating)]
		if !ok || delta == 0 {
			continue
		}
		pb.KeyPoints[idx].Score += delta
	}
}

func applyMergedKeyPoints(pb Playbook, merged []MergedKeyPoint) Playbook {
	// If model proposes fewer merged KPTs than existing ones, treat them as
	// additions instead of replacing the playbook to avoid accidental shrinking.
	if len(merged) > 0 && len(merged) < len(pb.KeyPoints) {
		existingNames := make(map[string]struct{})
		existingTexts := make(map[string]struct{})
		nameIndex := make(map[string]KeyPoint, len(pb.KeyPoints))

		for _, kp := range pb.KeyPoints {
			if kp.Name != "" {
				existingNames[kp.Name] = struct{}{}
				nameIndex[kp.Name] = kp
			}
			if kp.Text != "" {
				existingTexts[kp.Text] = struct{}{}
			}
		}

		for _, item := range merged {
			text := strings.TrimSpace(item.Text)
			if text == "" {
				continue
			}
			if _, ok := existingTexts[text]; ok {
				continue
			}

			tags := normalizeTags(item.Tags)
			sources := item.Sources

			var sourceKPs []KeyPoint
			for _, s := range sources {
				if kp, ok := nameIndex[s]; ok {
					sourceKPs = append(sourceKPs, kp)
				}
			}
			totalScore := 0
			var fallbackTags []string
			for _, kp := range sourceKPs {
				totalScore += kp.Score
				if len(fallbackTags) == 0 && len(kp.Tags) > 0 {
					fallbackTags = kp.Tags
				}
			}

			if len(tags) == 0 {
				tags = normalizeTags(fallbackTags)
			}
			if len(tags) == 0 {
				tags = inferTagsFromText(text, 6)
			}

			name := generateKeyPointName(existingNames)
			existingNames[name] = struct{}{}
			existingTexts[text] = struct{}{}
			pb.KeyPoints = append(pb.KeyPoints, KeyPoint{
				Name:    name,
				Text:    text,
				Score:   totalScore,
				Tags:    tags,
				Pending: false,
			})
		}
	}

	// Rebuild indices after any additions so downstream merge logic has mappings.
	existingNames := make(map[string]struct{})
	textIndex := make(map[string]KeyPoint, len(pb.KeyPoints))
	nameIndex := make(map[string]KeyPoint, len(pb.KeyPoints))
	for _, kp := range pb.KeyPoints {
		if kp.Name != "" {
			existingNames[kp.Name] = struct{}{}
			nameIndex[kp.Name] = kp
		}
		if kp.Text != "" {
			textIndex[kp.Text] = kp
		}
	}

	var mergedList []KeyPoint
	seenTexts := make(map[string]struct{})
	usedNames := make(map[string]struct{})

	for _, item := range merged {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		if _, ok := seenTexts[text]; ok {
			continue
		}

		tags := normalizeTags(item.Tags)
		sources := item.Sources

		var matchedSources []KeyPoint
		for _, s := range sources {
			if kp, ok := nameIndex[s]; ok {
				matchedSources = append(matchedSources, kp)
				usedNames[s] = struct{}{}
			}
		}

		var sourceKp *KeyPoint
		if len(matchedSources) > 0 {
			sourceKp = &matchedSources[0]
		} else if kp, ok := textIndex[text]; ok {
			sourceKp = &kp
		}

		totalScore := 0
		var fallbackTags []string
		if len(matchedSources) > 0 {
			for _, kp := range matchedSources {
				totalScore += kp.Score
				if len(fallbackTags) == 0 && len(kp.Tags) > 0 {
					fallbackTags = kp.Tags
				}
			}
		} else if sourceKp != nil {
			totalScore = sourceKp.Score
			fallbackTags = sourceKp.Tags
			usedNames[sourceKp.Name] = struct{}{}
		}

		name := ""
		if sourceKp != nil && strings.TrimSpace(sourceKp.Name) != "" {
			name = sourceKp.Name
		} else {
			name = generateKeyPointName(existingNames)
		}

		if anyName(mergedList, name) {
			name = generateKeyPointName(existingNames)
		}

		if len(tags) == 0 {
			tags = normalizeTags(fallbackTags)
		}
		if len(tags) == 0 {
			tags = inferTagsFromText(text, 6)
		}

		mergedList = append(mergedList, KeyPoint{
			Name:    name,
			Text:    text,
			Score:   totalScore,
			Tags:    tags,
			Pending: false,
			// Intentionally not propagating multi-dimensional fields here; the
			// original Python implementation infers them on load/save.
		})
		seenTexts[text] = struct{}{}
		existingNames[name] = struct{}{}
	}

	// Preserve any existing items that were not part of the merged output.
	for _, kp := range pb.KeyPoints {
		if _, ok := usedNames[kp.Name]; ok {
			continue
		}
		if _, ok := seenTexts[kp.Text]; ok {
			continue
		}
		mergedList = append(mergedList, kp)
		seenTexts[kp.Text] = struct{}{}
	}

	pb.KeyPoints = mergedList
	return pb
}

func applyNewKeyPointsAsPending(pb Playbook, newKeyPoints []KeyPoint) Playbook {
	if len(newKeyPoints) == 0 {
		return pb
	}
	existingNames := make(map[string]struct{})
	existingTexts := make(map[string]struct{})
	for _, kp := range pb.KeyPoints {
		if kp.Name != "" {
			existingNames[kp.Name] = struct{}{}
		}
		if kp.Text != "" {
			existingTexts[kp.Text] = struct{}{}
		}
	}

	for _, item := range newKeyPoints {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		if _, ok := existingTexts[text]; ok {
			continue
		}

		name := generateKeyPointName(existingNames)
		existingNames[name] = struct{}{}
		existingTexts[text] = struct{}{}

		effect := 0.5
		if item.EffectRating != nil {
			effect = *item.EffectRating
		}
		risk := -0.5
		if item.RiskLevel != nil {
			risk = *item.RiskLevel
		}
		innovation := 0.5
		if item.InnovationLevel != nil {
			innovation = *item.InnovationLevel
		}

		initialScore := int(math.Round(effect*2 + innovation*1 - risk*1))
		if initialScore < -2 {
			initialScore = -2
		}
		if initialScore > 3 {
			initialScore = 3
		}

		tags := normalizeTags(item.Tags)
		if len(tags) == 0 {
			tags = inferTagsFromText(text, 6)
		}

		pb.KeyPoints = append(pb.KeyPoints, KeyPoint{
			Name:            name,
			Text:            text,
			Score:           initialScore,
			Tags:            tags,
			Pending:         true,
			EffectRating:    ptrFloat(clamp01(effect)),
			RiskLevel:       ptrFloat(clamp11(risk)),
			InnovationLevel: ptrFloat(clamp01(innovation)),
		})
	}

	return pb
}

func generateKeyPointName(existingNames map[string]struct{}) string {
	maxNum := 0
	for name := range existingNames {
		if !strings.HasPrefix(name, "kpt_") {
			continue
		}
		num, err := strconv.Atoi(strings.TrimPrefix(name, "kpt_"))
		if err != nil {
			continue
		}
		if num > maxNum {
			maxNum = num
		}
	}
	return fmt.Sprintf("kpt_%03d", maxNum+1)
}

func anyName(kps []KeyPoint, name string) bool {
	for _, kp := range kps {
		if kp.Name == name {
			return true
		}
	}
	return false
}

func ptrFloat(v float64) *float64 {
	return &v
}

func clamp11(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

