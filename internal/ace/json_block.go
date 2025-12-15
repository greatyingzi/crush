package ace

import "strings"

// ExtractJSONBlock extracts the JSON payload from a model response that may
// include fenced code blocks or surrounding text.
func ExtractJSONBlock(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

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

