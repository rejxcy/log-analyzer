package models

import (
	"time"
)

// Severity represents error severity levels
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// RawLog represents a raw log entry from OpenSearch
type RawLog struct {
	Index     string           `json:"_index"`
	ID        string           `json:"_id"`
	Source    OpenSearchSource `json:"_source"`
	Timestamp time.Time        `json:"@timestamp"`
}

// OpenSearchSource represents the _source field from OpenSearch
type OpenSearchSource struct {
	Event     EventData              `json:"event"`
	Message   string                 `json:"message"`
	Fields    FieldsData             `json:"fields"`
	Agent     map[string]interface{} `json:"agent,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Log       map[string]interface{} `json:"log,omitempty"`
	Host      map[string]interface{} `json:"host,omitempty"`
	Timestamp time.Time              `json:"@timestamp"`
}

// EventData represents the event field
type EventData struct {
	Original string `json:"original"`
}

// FieldsData represents the fields section
type FieldsData struct {
	ServiceName string `json:"servicename"`
}

// ParsedLog represents a parsed log entry (clean JSON)
type ParsedLog struct {
	Timestamp   time.Time `json:"@timestamp"`
	Caller      string    `json:"caller"`
	Content     string    `json:"content"`
	Level       string    `json:"level"`
	Span        string    `json:"span"`
	Trace       string    `json:"trace"`
	ServiceName string    `json:"service_name"`
}

// PeakWindow represents a time window with high error density
type PeakWindow struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Count   int       `json:"count"`
	Density float64   `json:"density"` // errors per minute
}

// ErrorGroup represents a group of deduplicated errors
type ErrorGroup struct {
	Fingerprint       string         `json:"fingerprint"`
	NormalizedContent string         `json:"normalized_content"`
	ServiceName       string         `json:"service_name"`
	CallerFile        string         `json:"caller_file"`
	TotalCount        int            `json:"total_count"`
	Samples           []ParsedLog    `json:"samples"`
	TimeDistribution  map[string]int `json:"time_distribution"`
	PeakWindow        *PeakWindow    `json:"peak_window"`
}

// TrendAnalysis represents trend comparison with historical data
type TrendAnalysis struct {
	PreviousDayCount int     `json:"previous_day_count"`
	PercentageChange float64 `json:"percentage_change"`
	IsAnomalous      bool    `json:"is_anomalous"`
}

// Analysis represents the analysis result for an error group
type Analysis struct {
	ErrorGroupID     string         `json:"error_group_id"`
	IsKnown          bool           `json:"is_known"`
	IssueID          string         `json:"issue_id,omitempty"`
	Severity         Severity       `json:"severity"`
	Reason           string         `json:"reason"`
	SuggestedActions []string       `json:"suggested_actions"`
	TrendAnalysis    *TrendAnalysis `json:"trend_analysis,omitempty"`
}

// Rule represents a known issue rule
type Rule struct {
	ID               string          `yaml:"id"`
	Name             string          `yaml:"name"`
	Category         string          `yaml:"category"`
	Severity         Severity        `yaml:"severity"`
	ContentPatterns  []string        `yaml:"content_patterns"`
	CallerPatterns   []string        `yaml:"caller_patterns,omitempty"`
	Services         []string        `yaml:"services"`
	Description      string          `yaml:"description"`
	SuggestedActions []string        `yaml:"suggested_actions"`
	AlertThreshold   *AlertThreshold `yaml:"alert_threshold,omitempty"`
}

// AlertThreshold defines when to alert for a rule
type AlertThreshold struct {
	Total     int `yaml:"total"`
	OrDensity int `yaml:"or_density"`
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Report represents the final analysis report
type Report struct {
	GeneratedAt       time.Time     `json:"generated_at"`
	ExecutionTime     time.Duration `json:"execution_time"`
	TotalLogs         int           `json:"total_logs"`
	ErrorGroupCount   int           `json:"error_group_count"`
	HighPriorityCount int           `json:"high_priority_count"`
	NewIssueCount     int           `json:"new_issue_count"`
	ReportPath        string        `json:"report_path"`
	DataSources       []string      `json:"data_sources"`
}
