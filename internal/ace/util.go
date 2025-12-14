package ace

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func isStopWord(tok string) bool {
	switch tok {
	case "a", "an", "and", "are", "as", "at", "be", "but", "by",
		"for", "from", "how", "i", "if", "in", "is", "it", "of", "on",
		"or", "please", "the", "this", "to", "use", "we", "what", "when",
		"with", "you", "your":
		return true
	default:
		return false
	}
}

func inferEffectRatingFromScore(score int) float64 {
	return clamp01(0.5 + float64(score)*0.1)
}

func inferRiskFromText(text string) float64 {
	text = strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(text, "rm -rf") ||
		strings.Contains(text, "production") ||
		strings.Contains(text, "security") ||
		strings.Contains(text, "danger") {
		return 0.7
	}
	return 0
}

func inferInnovationFromText(text string) float64 {
	text = strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(text, "experimental") ||
		strings.Contains(text, "novel") ||
		strings.Contains(text, "new approach") {
		return 0.7
	}
	return 0.2
}

func nextKeyPointName(existing []KeyPoint) string {
	maxNum := 0
	for _, kp := range existing {
		if !strings.HasPrefix(kp.Name, "kpt_") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(kp.Name, "kpt_"))
		if err != nil {
			continue
		}
		maxNum = max(maxNum, n)
	}
	return fmt.Sprintf("kpt_%03d", maxNum+1)
}

var tagNormalizeTokenRx = regexp.MustCompile(`[\p{L}\p{N}_-]+`)

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		for _, m := range tagNormalizeTokenRx.FindAllString(t, -1) {
			if m != "" {
				out = append(out, m)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func inferTagsFromText(text string, maxTags int) []string {
	toks := tokenize(text)
	var tags []string
	for _, t := range toks {
		if isStopWord(t) {
			continue
		}
		tags = append(tags, t)
		if maxTags > 0 && len(tags) >= maxTags {
			break
		}
	}
	return normalizeTags(tags)
}

func (pb *Playbook) Add(text string, tags []string, score int) KeyPoint {
	kp := KeyPoint{
		Name:  nextKeyPointName(pb.KeyPoints),
		Text:  strings.TrimSpace(text),
		Tags:  normalizeTags(tags),
		Score: score,
	}
	pb.KeyPoints = append(pb.KeyPoints, kp)
	return kp
}
