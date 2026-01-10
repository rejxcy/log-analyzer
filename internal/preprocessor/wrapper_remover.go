package preprocessor

import (
	"fmt"
	"regexp"
	"strings"
)

// WrapperRemover handles removal of Kubernetes log wrappers
type WrapperRemover struct {
	patterns []*regexp.Regexp
}

// NewWrapperRemover creates a new wrapper remover with common patterns
func NewWrapperRemover() *WrapperRemover {
	patterns := []*regexp.Regexp{
		// Standard Kubernetes format: "TIMESTAMP stderr F JSON"
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s+stderr\s+F\s+(.*)$`),

		// Alternative format: "TIMESTAMP stdout F JSON"
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s+stdout\s+F\s+(.*)$`),

		// Format without microseconds: "TIMESTAMP stderr F JSON"
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\s+stderr\s+F\s+(.*)$`),

		// Format with different stream types
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d*Z\s+(stderr|stdout)\s+[FP]\s+(.*)$`),
	}

	return &WrapperRemover{
		patterns: patterns,
	}
}

// RemoveWrapper attempts to remove wrapper from log message using multiple patterns
func (wr *WrapperRemover) RemoveWrapper(message string) (string, error) {
	// Try each pattern
	for i, pattern := range wr.patterns {
		matches := pattern.FindStringSubmatch(message)
		if len(matches) >= 2 {
			// Found a match, extract the content
			content := matches[len(matches)-1] // Last capture group
			return wr.cleanContent(content), nil
		}

		// For debugging: log which patterns were tried
		_ = i // Use the variable to avoid unused error
	}

	// If no wrapper pattern matches, check if it's already clean JSON
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return trimmed, nil
	}

	return "", fmt.Errorf("no matching wrapper pattern found for message: %s",
		message[:min(100, len(message))])
}

// cleanContent cleans up the extracted content
func (wr *WrapperRemover) cleanContent(content string) string {
	// Remove extra whitespace
	content = strings.TrimSpace(content)

	// Handle escaped quotes in JSON
	content = strings.ReplaceAll(content, `\"`, `"`)

	// Handle other common escape sequences
	content = strings.ReplaceAll(content, `\\`, `\`)

	return content
}

// DetectWrapperType detects the type of wrapper format
func (wr *WrapperRemover) DetectWrapperType(message string) string {
	for i, pattern := range wr.patterns {
		if pattern.MatchString(message) {
			switch i {
			case 0:
				return "kubernetes-stderr-f"
			case 1:
				return "kubernetes-stdout-f"
			case 2:
				return "kubernetes-stderr-f-no-microseconds"
			case 3:
				return "kubernetes-generic"
			default:
				return "unknown-pattern"
			}
		}
	}

	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "{") {
		return "clean-json"
	}

	return "no-wrapper"
}

// GetSupportedFormats returns a list of supported wrapper formats
func (wr *WrapperRemover) GetSupportedFormats() []string {
	return []string{
		"kubernetes-stderr-f",
		"kubernetes-stdout-f",
		"kubernetes-stderr-f-no-microseconds",
		"kubernetes-generic",
		"clean-json",
	}
}

// ValidateExtractedContent validates that the extracted content looks like valid JSON
func (wr *WrapperRemover) ValidateExtractedContent(content string) error {
	trimmed := strings.TrimSpace(content)

	if len(trimmed) == 0 {
		return fmt.Errorf("extracted content is empty")
	}

	if !strings.HasPrefix(trimmed, "{") {
		return fmt.Errorf("extracted content does not start with '{': %s",
			trimmed[:min(50, len(trimmed))])
	}

	if !strings.HasSuffix(trimmed, "}") {
		return fmt.Errorf("extracted content does not end with '}': %s",
			trimmed[max(0, len(trimmed)-50):])
	}

	return nil
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
