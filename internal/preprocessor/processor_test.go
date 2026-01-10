package preprocessor

import (
	"testing"
	"time"

	"log-analyzer/pkg/models"
)

func TestNewLogPreprocessor(t *testing.T) {
	processor := NewLogPreprocessor()
	if processor == nil {
		t.Fatal("Processor is nil")
	}

	if processor.wrapperRegex == nil {
		t.Fatal("Wrapper regex is nil")
	}
}

func TestRemoveWrapper(t *testing.T) {
	processor := NewLogPreprocessor()

	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "Valid Kubernetes wrapper",
			input:    `2026-01-10T11:30:32.804760259Z stderr F {"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test message","level":"error"}`,
			expected: `{"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test message","level":"error"}`,
			hasError: false,
		},
		{
			name:     "Already clean JSON",
			input:    `{"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test message","level":"error"}`,
			expected: `{"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test message","level":"error"}`,
			hasError: false,
		},
		{
			name:     "Invalid format",
			input:    `invalid log format`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.removeWrapper(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

func TestParseInnerJSON(t *testing.T) {
	processor := NewLogPreprocessor()

	validJSON := `{"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test message","level":"error","span":"test-span","trace":"test-trace"}`

	result, err := processor.parseInnerJSON(validJSON)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Content != "test message" {
		t.Errorf("Expected content 'test message', got %q", result.Content)
	}

	if result.Level != "error" {
		t.Errorf("Expected level 'error', got %q", result.Level)
	}

	if result.Caller != "logic/spin_logic.go:110" {
		t.Errorf("Expected caller 'logic/spin_logic.go:110', got %q", result.Caller)
	}
}

func TestValidateParsedLog(t *testing.T) {
	processor := NewLogPreprocessor()

	validLog := &models.ParsedLog{
		Timestamp:   time.Now(),
		Caller:      "test.go:123",
		Content:     "test content",
		Level:       "error",
		Span:        "test-span",
		Trace:       "test-trace",
		ServiceName: "test-service",
	}

	err := processor.validateParsedLog(validLog)
	if err != nil {
		t.Errorf("Valid log should not produce error: %v", err)
	}

	// Test missing required fields
	invalidLog := &models.ParsedLog{}
	err = processor.validateParsedLog(invalidLog)
	if err == nil {
		t.Error("Invalid log should produce error")
	}
}

func TestProcessRawLog(t *testing.T) {
	processor := NewLogPreprocessor()

	// Create a test raw log based on the actual OpenSearch format
	rawLog := models.RawLog{
		Index: "test-log-2026.01.10",
		ID:    "test-id",
		Source: models.OpenSearchSource{
			Message: `2026-01-10T11:30:32.804760259Z stderr F {"@timestamp":"2026-01-10T19:30:32.804+08:00","caller":"logic/spin_logic.go:110","content":"test error message","level":"error","span":"test-span","trace":"test-trace"}`,
			Fields: models.FieldsData{
				ServiceName: "test-service",
			},
		},
		Timestamp: time.Now(),
	}

	result, err := processor.processRawLog(rawLog)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Result is nil")
	}

	if result.Content != "test error message" {
		t.Errorf("Expected content 'test error message', got %q", result.Content)
	}

	if result.ServiceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got %q", result.ServiceName)
	}

	if result.Level != "error" {
		t.Errorf("Expected level 'error', got %q", result.Level)
	}
}

// Placeholder for property-based tests that will be implemented in subtasks
func TestPreprocessorPropertyBased(t *testing.T) {
	// Property-based tests will be implemented in tasks 3.1, 3.2, 3.3
	t.Skip("Property-based tests will be implemented in subtasks")
}
