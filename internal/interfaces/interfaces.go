package interfaces

import (
	"context"
	"time"

	"log-analyzer/pkg/models"
)

// FetchConfig contains configuration for data fetching
type FetchConfig struct {
	TimeRange  models.TimeRange
	Services   []string
	Indices    []string
	Keywords   []string // Keywords to search for (e.g., ["error", "warn"])
	MaxResults int
	Timeout    time.Duration
}

// AggregationResult contains statistical analysis results
type AggregationResult struct {
	ServiceStats     map[string]*ServiceStats
	TimeStats        *TimeStats
	TrendAnalysis    *models.TrendAnalysis
	TotalErrorGroups int
	TotalLogs        int
	ProcessingTime   time.Duration
}

// ServiceStats contains statistics for a specific service
type ServiceStats struct {
	ServiceName       string
	ErrorGroupCount   int
	TotalErrors       int
	HighPriorityCount int
	PeakDensity       float64
}

// TimeStats contains time-based statistics
type TimeStats struct {
	HourlyDistribution map[int]int // hour -> count
	PeakHour           int
	PeakCount          int
	AverageDensity     float64

	// Time range information
	EarliestLogTime time.Time     // 最早日誌時間
	LatestLogTime   time.Time     // 最晚日誌時間
	QueryDuration   time.Duration // 查詢時間範圍

	// Window-based peak statistics (30-minute granularity)
	PeakWindowStart time.Time // 峰值 30 分鐘窗口的起始時間
	PeakWindowEnd   time.Time // 峰值 30 分鐘窗口的結束時間
	PeakWindowCount int       // 峰值窗口的錯誤數
}

// Fetcher interface for retrieving log data from OpenSearch
type Fetcher interface {
	Fetch(ctx context.Context, config FetchConfig) ([]models.RawLog, error)
}

// Preprocessor interface for processing raw logs
type Preprocessor interface {
	Process(rawLogs []models.RawLog) ([]models.ParsedLog, error)
}

// Normalizer interface for content normalization and fingerprinting
type Normalizer interface {
	Normalize(logs []models.ParsedLog) ([]models.ErrorGroup, error)
}

// Aggregator interface for statistical analysis
type Aggregator interface {
	Aggregate(groups []models.ErrorGroup) (*AggregationResult, error)
}

// RuleEngine interface for pattern matching and severity calculation
type RuleEngine interface {
	LoadRules(rulesDir string) error
	Analyze(groups []models.ErrorGroup, stats *AggregationResult) ([]models.Analysis, error)
}

// Reporter interface for generating reports
type Reporter interface {
	Generate(analyses []models.Analysis, stats *AggregationResult) (*models.Report, error)
}

// ErrorHandler interface for handling various types of errors
type ErrorHandler interface {
	HandleFetchError(err error, service string) error
	HandleParseError(err error, rawLog string) error
	HandleRuleError(err error, ruleFile string) error
	HandleStorageError(err error, operation string) error
}
