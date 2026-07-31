package util

import (
	"encoding/json"
	"strings"
)

// ExtractJSON extracts the first valid JSON object or array from text that may contain
// markdown formatting (e.g. ```json ... ```) or conversational preamble/postscript.
func ExtractJSON(text string) string {
	cleaned := strings.TrimSpace(text)

	// Strip markdown code fences if present
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	// Try direct validation first
	if json.Valid([]byte(cleaned)) {
		return cleaned
	}

	// Search for JSON Object {...}
	objStart := strings.Index(cleaned, "{")
	objEnd := strings.LastIndex(cleaned, "}")
	if objStart != -1 && objEnd != -1 && objEnd > objStart {
		candidate := cleaned[objStart : objEnd+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	// Search for JSON Array [...]
	arrStart := strings.Index(cleaned, "[")
	arrEnd := strings.LastIndex(cleaned, "]")
	if arrStart != -1 && arrEnd != -1 && arrEnd > arrStart {
		candidate := cleaned[arrStart : arrEnd+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	// Fallbacks if formatting is slightly loose
	if objStart != -1 && objEnd != -1 && objEnd > objStart {
		return cleaned[objStart : objEnd+1]
	}
	if arrStart != -1 && arrEnd != -1 && arrEnd > arrStart {
		return cleaned[arrStart : arrEnd+1]
	}

	return cleaned
}

// SafeTruncate truncates a string to maxLen characters without panicking.
func SafeTruncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// SliceLimit returns at most limit elements from a string slice.
func SliceLimit(slice []string, limit int) []string {
	if len(slice) > limit {
		return slice[:limit]
	}
	return slice
}
