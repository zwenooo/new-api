package oai_cc

import (
	"regexp"
	"strings"
)

var reCch = regexp.MustCompile(`(?i)\bcch=[^;\n]+`)

func normalizeDynamicTimeLines(s string) string {
	if s == "" {
		return ""
	}

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "current time:"):
			lines[i] = "  Current time: <stable>"
		case strings.HasPrefix(lower, "current date:"):
			lines[i] = "  Current date: <stable>"
		case strings.HasPrefix(lower, "current datetime:"):
			lines[i] = "  Current datetime: <stable>"
		case strings.HasPrefix(lower, "today's date:") || strings.HasPrefix(lower, "today’s date:"):
			lines[i] = "  Today's date: <stable>"
		}
	}
	return strings.Join(lines, "\n")
}

