package ace

import (
	"regexp"
	"sort"
	"strings"
)

type SelectOptions struct {
	MaxItems int
	MinScore int
	MaxChars int
}

type Selector interface {
	Select(pb Playbook, prompt string, opts SelectOptions) []KeyPoint
}

type DefaultSelector struct{}

func NewDefaultSelector() DefaultSelector {
	return DefaultSelector{}
}

func (DefaultSelector) Select(pb Playbook, prompt string, opts SelectOptions) []KeyPoint {
	if opts.MaxItems <= 0 {
		return nil
	}

	promptTokens := tokenize(prompt)
	if len(promptTokens) == 0 {
		return nil
	}

	promptTags := inferTagsFromText(prompt, 6)
	promptTokenSet := make(map[string]struct{}, len(promptTokens)+len(promptTags))
	for _, t := range promptTokens {
		promptTokenSet[t] = struct{}{}
	}
	for _, t := range promptTags {
		promptTokenSet[t] = struct{}{}
	}

	type scored struct {
		kp    KeyPoint
		score float64
	}
	var candidates []scored
	for _, kp := range pb.KeyPoints {
		if kp.Pending || strings.TrimSpace(kp.Text) == "" {
			continue
		}
		if kp.Score < opts.MinScore {
			continue
		}

		textTokens := tokenize(kp.Text)
		tokenOverlap := overlapCount(promptTokenSet, textTokens)
		tagOverlap := overlapCount(promptTokenSet, kp.Tags)

		// Tags should dominate selection; text overlap is a weaker signal.
		score := float64(kp.Score) + float64(tagOverlap)*2.0 + float64(tokenOverlap)*0.4
		if score <= 0 {
			continue
		}
		candidates = append(candidates, scored{kp: kp, score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].kp.Name < candidates[j].kp.Name
		}
		return candidates[i].score > candidates[j].score
	})

	selected := make([]KeyPoint, 0, min(opts.MaxItems, len(candidates)))
	for _, c := range candidates {
		if len(selected) >= opts.MaxItems {
			break
		}
		selected = append(selected, c.kp)
	}

	if opts.MaxChars > 0 {
		selected = trimToMaxChars(selected, opts.MaxChars)
	}

	return selected
}

var tagTokenRx = regexp.MustCompile(`[\p{L}\p{N}_-]+`)

func tokenize(s string) []string {
	s = strings.ToLower(s)
	toks := tagTokenRx.FindAllString(s, -1)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		t = strings.TrimSpace(t)
		if len(t) < 2 {
			continue
		}
		out = append(out, t)
	}
	return out
}

func overlapCount(set map[string]struct{}, items []string) int {
	n := 0
	for _, it := range items {
		if it == "" {
			continue
		}
		if _, ok := set[strings.ToLower(it)]; ok {
			n++
		}
	}
	return n
}
