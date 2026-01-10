package preprocessor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JSONParser handles parsing of inner JSON log content
type JSONParser struct {
	timeFormats []string
}

// NewJSONParser creates a new JSON parser with common time formats
func NewJSONParser() *JSONParser {
	timeFormats := []string{
		time.RFC3339,                       // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano,                   // "2006-01-02T15:04:05.999999999Z07:00"
		"2006-01-02T15:04:05.999Z07:00",    // Common format with milliseconds
		"2006-01-02T15:04:05.999999Z07:00", // Common format with microseconds
		"2006-01-02T15:04:05Z",             // Without timezone
		"2006-01-02T15:04:05.999Z",         // With milliseconds, no timezone
	}

	return &JSONParser{
		timeFormats: timeFormats,
	}
}

// LogFields represents the expected fields in the inner JSON
type LogFields struct {
	Timestamp string `json:"@timestamp"`
	Caller    string `json:"caller"`
	Content   string `json:"content"`
	Level     string `json:"level"`
	Span      string `json:"span"`
	Trace     string `json:"trace"`

	// Additional fields that might be present
	Message   string `json:"message,omitempty"`
	Logger    string `json:"logger,omitempty"`
	Thread    string `json:"thread,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ParsedFields represents the parsed and validated log fields
type ParsedFields struct {
	Timestamp time.Time
	Caller    string
	Content   string
	Level     string
	Span      string
	Trace     string

	// Additional parsed fields
	Message   string
	Logger    string
	Thread    string
	RequestID string
}

// ParseJSON parses the JSON content and extracts log fields
func (jp *JSONParser) ParseJSON(jsonContent string) (*ParsedFields, error) {
	// Clean the JSON content
	cleanJSON := jp.cleanJSONContent(jsonContent)

	// Parse into LogFields struct
	var fields LogFields
	if err := json.Unmarshal([]byte(cleanJSON), &fields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w, content: %s",
			err, cleanJSON[:min(200, len(cleanJSON))])
	}

	// Convert to ParsedFields with validation
	parsed, err := jp.convertToParsedFields(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to convert fields: %w", err)
	}

	return parsed, nil
}

// cleanJSONContent cleans up the JSON content for parsing
func (jp *JSONParser) cleanJSONContent(content string) string {
	// Trim whitespace
	content = strings.TrimSpace(content)

	// Handle double-escaped quotes (common in log forwarding)
	content = strings.ReplaceAll(content, `\"`, `"`)

	// Handle escaped backslashes
	content = strings.ReplaceAll(content, `\\`, `\`)

	// Remove any trailing commas before closing braces (invalid JSON)
	content = strings.ReplaceAll(content, `,}`, `}`)
	content = strings.ReplaceAll(content, `,]`, `]`)

	return content
}

// convertToParsedFields converts LogFields to ParsedFields with validation
func (jp *JSONParser) convertToParsedFields(fields LogFields) (*ParsedFields, error) {
	parsed := &ParsedFields{
		Caller:    fields.Caller,
		Content:   fields.Content,
		Level:     strings.ToLower(fields.Level), // Normalize level to lowercase
		Span:      fields.Span,
		Trace:     fields.Trace,
		Message:   fields.Message,
		Logger:    fields.Logger,
		Thread:    fields.Thread,
		RequestID: fields.RequestID,
	}

	// Parse timestamp
	timestamp, err := jp.parseTimestamp(fields.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp '%s': %w", fields.Timestamp, err)
	}
	parsed.Timestamp = timestamp

	// Use message as content if content is empty but message exists
	if parsed.Content == "" && parsed.Message != "" {
		parsed.Content = parsed.Message
	}

	// Validate required fields
	if err := jp.validateRequiredFields(parsed); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return parsed, nil
}

// parseTimestamp attempts to parse timestamp using multiple formats
func (jp *JSONParser) parseTimestamp(timestampStr string) (time.Time, error) {
	if timestampStr == "" {
		return time.Time{}, fmt.Errorf("timestamp string is empty")
	}

	// Try each time format
	for _, format := range jp.timeFormats {
		if t, err := time.Parse(format, timestampStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp with any known format: %s", timestampStr)
}

// validateRequiredFields validates that required fields are present
func (jp *JSONParser) validateRequiredFields(fields *ParsedFields) error {
	if fields.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	if fields.Content == "" {
		return fmt.Errorf("content is required")
	}

	if fields.Level == "" {
		return fmt.Errorf("level is required")
	}

	// Validate log level
	validLevels := map[string]bool{
		"error":   true,
		"err":     true,
		"warn":    true,
		"warning": true,
		"info":    true,
		"debug":   true,
		"trace":   true,
	}

	if !validLevels[fields.Level] {
		return fmt.Errorf("invalid log level: %s", fields.Level)
	}

	return nil
}

// ExtractAdditionalFields extracts additional fields from raw JSON
func (jp *JSONParser) ExtractAdditionalFields(jsonContent string) (map[string]interface{}, error) {
	var rawFields map[string]interface{}

	cleanJSON := jp.cleanJSONContent(jsonContent)
	if err := json.Unmarshal([]byte(cleanJSON), &rawFields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal for additional fields: %w", err)
	}

	// Remove known fields to get additional ones
	knownFields := map[string]bool{
		"@timestamp": true,
		"caller":     true,
		"content":    true,
		"level":      true,
		"span":       true,
		"trace":      true,
		"message":    true,
		"logger":     true,
		"thread":     true,
		"request_id": true,
	}

	additionalFields := make(map[string]interface{})
	for key, value := range rawFields {
		if !knownFields[key] {
			additionalFields[key] = value
		}
	}

	return additionalFields, nil
}

// GetSupportedTimeFormats returns the list of supported time formats
func (jp *JSONParser) GetSupportedTimeFormats() []string {
	return jp.timeFormats
}

// NormalizeLogLevel normalizes log level to standard values
func (jp *JSONParser) NormalizeLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))

	switch level {
	case "err":
		return "error"
	case "warning":
		return "warn"
	default:
		return level
	}
}
