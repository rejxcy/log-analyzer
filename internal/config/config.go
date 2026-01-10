package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	OpenSearch OpenSearchConfig `yaml:"opensearch"`
	Query      QueryConfig      `yaml:"query"`
	Analysis   AnalysisConfig   `yaml:"analysis"`
	Output     OutputConfig     `yaml:"output"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// OpenSearchConfig contains OpenSearch connection settings
type OpenSearchConfig struct {
	URL      string   `yaml:"url"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	Indices  []string `yaml:"indices"`
}

// QueryConfig contains query-related settings
type QueryConfig struct {
	MaxResults int           `yaml:"max_results"`
	Timeout    time.Duration `yaml:"timeout"`
}

// AnalysisConfig contains analysis parameters
type AnalysisConfig struct {
	TimeRange  string         `yaml:"time_range"`
	Levels     []string       `yaml:"levels"`
	Keywords   []string       `yaml:"keywords"` // Keywords to search for (e.g., ["error", "warn"])
	SampleSize int            `yaml:"sample_size"`
	Density    DensityConfig  `yaml:"density"`
	Severity   SeverityConfig `yaml:"severity"`
}

// DensityConfig contains time density analysis settings
type DensityConfig struct {
	PeakWindowMinutes        int `yaml:"peak_window_minutes"`
	HighDensityThreshold     int `yaml:"high_density_threshold"`
	CriticalDensityThreshold int `yaml:"critical_density_threshold"`
}

// SeverityConfig contains severity calculation settings
type SeverityConfig struct {
	CountThresholds   CountThresholds   `yaml:"count_thresholds"`
	DensityThresholds DensityThresholds `yaml:"density_thresholds"`
	Trend             TrendConfig       `yaml:"trend"`
}

// CountThresholds defines count-based severity thresholds
type CountThresholds struct {
	High   int `yaml:"high"`
	Medium int `yaml:"medium"`
	Low    int `yaml:"low"`
}

// DensityThresholds defines density-based severity thresholds
type DensityThresholds struct {
	Critical int `yaml:"critical"`
	High     int `yaml:"high"`
	Medium   int `yaml:"medium"`
}

// TrendConfig contains trend analysis settings
type TrendConfig struct {
	AnomalyThreshold float64 `yaml:"anomaly_threshold"`
}

// OutputConfig contains output settings
type OutputConfig struct {
	ReportPath    string `yaml:"report_path"`
	DataPath      string `yaml:"data_path"`
	PendingPath   string `yaml:"pending_path"`
	RetentionDays int    `yaml:"retention_days"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// Load loads configuration from a YAML file with environment variable substitution
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Substitute environment variables
	content := string(data)
	content = substituteEnvVars(content)

	var config Config
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	applyDefaults(&config)

	// Validate configuration
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// substituteEnvVars replaces ${VAR_NAME} with environment variable values
func substituteEnvVars(content string) string {
	for {
		start := strings.Index(content, "${")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "}")
		if end == -1 {
			break
		}
		end += start

		varName := content[start+2 : end]
		envValue := os.Getenv(varName)
		content = content[:start] + envValue + content[end+1:]
	}
	return content
}

// applyDefaults applies default values for optional configuration parameters
func applyDefaults(config *Config) {
	if config.Query.MaxResults == 0 {
		config.Query.MaxResults = 10000
	}
	if config.Query.Timeout == 0 {
		config.Query.Timeout = 30 * time.Second
	}
	if config.Analysis.TimeRange == "" {
		config.Analysis.TimeRange = "24h"
	}
	if config.Analysis.SampleSize == 0 {
		config.Analysis.SampleSize = 5
	}
	if config.Analysis.Density.PeakWindowMinutes == 0 {
		config.Analysis.Density.PeakWindowMinutes = 5
	}
	if config.Analysis.Density.HighDensityThreshold == 0 {
		config.Analysis.Density.HighDensityThreshold = 100
	}
	if config.Analysis.Density.CriticalDensityThreshold == 0 {
		config.Analysis.Density.CriticalDensityThreshold = 500
	}
	if config.Output.ReportPath == "" {
		config.Output.ReportPath = "./reports"
	}
	if config.Output.DataPath == "" {
		config.Output.DataPath = "./data"
	}
	if config.Output.PendingPath == "" {
		config.Output.PendingPath = "./pending"
	}
	if config.Output.RetentionDays == 0 {
		config.Output.RetentionDays = 30
	}
	if len(config.Analysis.Keywords) == 0 {
		config.Analysis.Keywords = []string{"error"} // Default to searching for errors
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
}

// validate checks if the configuration is valid
func validate(config *Config) error {
	if config.OpenSearch.URL == "" {
		return fmt.Errorf("opensearch.url is required")
	}
	if len(config.OpenSearch.Indices) == 0 {
		return fmt.Errorf("opensearch.indices cannot be empty")
	}
	if config.Query.MaxResults <= 0 {
		return fmt.Errorf("query.max_results must be positive")
	}
	if config.Analysis.SampleSize <= 0 {
		return fmt.Errorf("analysis.sample_size must be positive")
	}
	return nil
}
