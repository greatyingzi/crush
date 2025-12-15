package ace

import (
	"regexp"
	"slices"
	"strings"
)

const maxKeyPoints = 250

func UpdateFromSessionSummary(store Store, playbookPath, sessionTitle, summary string) (bool, error) {
	summary = strings.TrimSpace(summary)
	if len(summary) < 16 {
		return false, nil
	}

	pb, err := store.Load(playbookPath)
	if err != nil {
		return false, err
	}
	pb.Normalize()

	newKeyPoints := extractKeyPointsFromSummary(sessionTitle, summary)
	if len(newKeyPoints) == 0 {
		return false, nil
	}

	changed := false
	for _, text := range newKeyPoints {
		kp := KeyPoint{
			Text: text,
			Tags: append([]string{"ace", "session"}, inferTagsFromTitle(sessionTitle)...),
		}
		kp = evaluateKeyPoint(kp)
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

	if err := store.SaveAtomic(playbookPath, pb); err != nil {
		return false, err
	}
	return true, nil
}

func extractKeyPointsFromSummary(sessionTitle, summary string) []string {
	lines := splitNonEmptyLines(summary)

	var bullets []string
	for _, l := range lines {
		if b, ok := parseBulletLine(l); ok {
			bullets = append(bullets, b)
		}
	}
	if len(bullets) == 0 {
		bullets = splitSentences(summary)
	}

	// Prefix to give provenance in case items are later merged.
	var out []string
	for _, b := range bullets {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if title := strings.TrimSpace(sessionTitle); title != "" {
			b = "[Session] " + title + ": " + b
		}
		out = append(out, b)
		if len(out) >= 12 {
			break
		}
	}
	return slices.Compact(out)
}

func splitNonEmptyLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

var bulletPrefixRx = regexp.MustCompile(`^(\*|-|•|(\d+[\.\)]))\s+`)

func parseBulletLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if !bulletPrefixRx.MatchString(line) {
		return "", false
	}
	trimmed := bulletPrefixRx.ReplaceAllString(line, "")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func splitSentences(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '.', '。', '!', '！', '?', '？', '\n':
			return true
		default:
			return false
		}
	})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) < 20 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func mergeOrAddKeyPoint(pb *Playbook, candidate KeyPoint) bool {
	candidate.Text = strings.TrimSpace(candidate.Text)
	if candidate.Text == "" {
		return false
	}
	candidate.Tags = normalizeTags(candidate.Tags)

	bestIdx := -1
	bestSim := 0.0
	for i, existing := range pb.KeyPoints {
		if existing.Pending || strings.TrimSpace(existing.Text) == "" {
			continue
		}
		sim := lexicalSimilarity(candidate.Text, existing.Text)
		if sim > bestSim {
			bestSim = sim
			bestIdx = i
		}
	}

	const mergeThreshold = 0.8
	if bestIdx >= 0 && bestSim >= mergeThreshold {
		mergedTags := append([]string{}, pb.KeyPoints[bestIdx].Tags...)
		mergedTags = append(mergedTags, candidate.Tags...)
		pb.KeyPoints[bestIdx].Tags = normalizeTags(mergedTags)
		pb.KeyPoints[bestIdx].Score += candidate.Score
		pb.KeyPoints[bestIdx].EffectRating = maxPtr(pb.KeyPoints[bestIdx].EffectRating, candidate.EffectRating)
		pb.KeyPoints[bestIdx].RiskLevel = maxPtr(pb.KeyPoints[bestIdx].RiskLevel, candidate.RiskLevel)
		pb.KeyPoints[bestIdx].InnovationLevel = maxPtr(pb.KeyPoints[bestIdx].InnovationLevel, candidate.InnovationLevel)
		return true
	}

	pb.Add(candidate.Text, candidate.Tags, candidate.Score)
	pb.KeyPoints[len(pb.KeyPoints)-1].EffectRating = candidate.EffectRating
	pb.KeyPoints[len(pb.KeyPoints)-1].RiskLevel = candidate.RiskLevel
	pb.KeyPoints[len(pb.KeyPoints)-1].InnovationLevel = candidate.InnovationLevel
	return true
}

func cleanupKeyPoints(in []KeyPoint) []KeyPoint {
	out := make([]KeyPoint, 0, len(in))
	for _, kp := range in {
		if kp.Score <= -5 {
			continue
		}
		out = append(out, kp)
	}
	return out
}

func inferTagsFromTitle(title string) []string {
	toks := tokenize(title)
	var tags []string
	for _, t := range toks {
		if isStopWord(t) {
			continue
		}
		tags = append(tags, t)
		if len(tags) >= 4 {
			break
		}
	}
	return normalizeTags(tags)
}

func lexicalSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(stripSessionPrefix(a)))
	b = strings.ToLower(strings.TrimSpace(stripSessionPrefix(b)))
	if a == "" && b == "" {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	subBonus := 0.0
	if strings.Contains(a, b) || strings.Contains(b, a) {
		subBonus = 0.7
	}

	setA := make(map[string]struct{})
	for _, t := range tokenize(a) {
		setA[t] = struct{}{}
	}
	toksB := tokenize(b)
	if len(setA) == 0 || len(toksB) == 0 {
		return subBonus
	}

	inter := 0
	union := len(setA)
	for _, t := range toksB {
		if _, ok := setA[t]; ok {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return subBonus
	}
	j := float64(inter) / float64(union)
	if j > subBonus {
		return j
	}
	return subBonus
}

var sessionPrefixRx = regexp.MustCompile(`(?i)^\[session\]\s+[^:]{1,120}:\s+`)

func stripSessionPrefix(s string) string {
	s = strings.TrimSpace(s)
	return sessionPrefixRx.ReplaceAllString(s, "")
}

func evaluateKeyPoint(kp KeyPoint) KeyPoint {
	text := strings.ToLower(strings.TrimSpace(stripSessionPrefix(kp.Text)))

	// Heuristic scoring aligned with ACE semantics:
	// useful +1, harmful -3, neutral 0.
	score := 1
	switch {
	case strings.Contains(text, "hallucinat") || strings.Contains(text, "wrong") || strings.Contains(text, "incorrect"):
		score = -3
	case strings.Contains(text, "unsure") || strings.Contains(text, "unclear") || strings.Contains(text, "maybe"):
		score = 0
	}
	kp.Score = score

	effect := inferEffectRatingFromScore(score)
	kp.EffectRating = &effect
	risk := inferRiskFromText(text)
	kp.RiskLevel = &risk
	innovation := inferInnovationFromText(text)
	kp.InnovationLevel = &innovation

	if len(kp.Tags) == 0 {
		kp.Tags = inferTagsFromText(text, 6)
	} else {
		kp.Tags = normalizeTags(kp.Tags)
	}

	return kp
}

func maxPtr(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b > *a {
		return b
	}
	return a
}
