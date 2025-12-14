package ace

import (
	"strings"
)

type Formatter interface {
	Format(selected []KeyPoint) string
	TrimToMaxChars(selected []KeyPoint, maxChars int) []KeyPoint
}

type DefaultFormatter struct{}

func NewDefaultFormatter() DefaultFormatter {
	return DefaultFormatter{}
}

func (DefaultFormatter) Format(selected []KeyPoint) string {
	if len(selected) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("ACE Memory (project-local):\n")
	for _, kp := range selected {
		line := strings.TrimSpace(kp.Text)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		if len(kp.Tags) > 0 {
			b.WriteString("  (tags: ")
			b.WriteString(strings.Join(kp.Tags, ", "))
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (f DefaultFormatter) TrimToMaxChars(selected []KeyPoint, maxChars int) []KeyPoint {
	if maxChars <= 0 {
		return selected
	}
	var out []KeyPoint
	for _, kp := range selected {
		out = append(out, kp)
		if len(f.Format(out)) > maxChars {
			out = out[:len(out)-1]
			break
		}
	}
	return out
}

func trimToMaxChars(selected []KeyPoint, maxChars int) []KeyPoint {
	return NewDefaultFormatter().TrimToMaxChars(selected, maxChars)
}
