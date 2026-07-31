package util

import (
	"strings"
)

// ExtractJSON extracts the first JSON object or array from text that may contain
// non-JSON content (like markdown formatting from LLM responses).
func ExtractJSON(text string) string {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	start = strings.Index(text, "{")
	end = strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

// SafeTruncate truncates a string to maxLen characters without panicking.
func SafeTruncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
