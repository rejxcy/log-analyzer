package reporter

import (
	"testing"
)

func TestExtractCountFromReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		expected string
	}{
		{
			name:     "Chinese format with 發生了",
			reason:   "錯誤在服務 pp-slot-api 中發生了 45 次",
			expected: "45",
		},
		{
			name:     "Chinese format with 發生了 (100)",
			reason:   "錯誤在服務 pp-slot-rpc 中發生了 100 次",
			expected: "100",
		},
		{
			name:     "English format with times",
			reason:   "Error occurred 274 times in service",
			expected: "274",
		},
		{
			name:     "English format with times (single digit)",
			reason:   "Error occurred 5 times",
			expected: "5",
		},
		{
			name:     "No match - should return unknown",
			reason:   "Some random reason without numbers",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCountFromReason(tt.reason)
			if result != tt.expected {
				t.Errorf("extractCountFromReason(%q) = %q, want %q", tt.reason, result, tt.expected)
			}
		})
	}
}
