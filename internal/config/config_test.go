package config

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file for testing
	configContent := `
opensearch:
  url: "https://test.com:9200"
  username: "testuser"
  password: "testpass"
  indices:
    - "test-log*"
query:
  max_results: 5000
  timeout: "15s"
analysis:
  time_range: "12h"
  sample_size: 3
output:
  report_path: "./test-reports"
logging:
  level: "debug"
`

	tmpFile, err := os.CreateTemp("", "config-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	config, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded values
	if config.OpenSearch.URL != "https://test.com:9200" {
		t.Errorf("Expected URL 'https://test.com:9200', got '%s'", config.OpenSearch.URL)
	}

	if config.Query.MaxResults != 5000 {
		t.Errorf("Expected MaxResults 5000, got %d", config.Query.MaxResults)
	}

	if config.Query.Timeout != 15*time.Second {
		t.Errorf("Expected Timeout 15s, got %v", config.Query.Timeout)
	}
}

func TestEnvironmentVariableSubstitution(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_USER", "envuser")
	os.Setenv("TEST_PASS", "envpass")
	defer func() {
		os.Unsetenv("TEST_USER")
		os.Unsetenv("TEST_PASS")
	}()

	configContent := `
opensearch:
  url: "https://test.com:9200"
  username: "${TEST_USER}"
  password: "${TEST_PASS}"
  indices:
    - "test-log*"
`

	tmpFile, err := os.CreateTemp("", "config-env-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	config, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.OpenSearch.Username != "envuser" {
		t.Errorf("Expected username 'envuser', got '%s'", config.OpenSearch.Username)
	}

	if config.OpenSearch.Password != "envpass" {
		t.Errorf("Expected password 'envpass', got '%s'", config.OpenSearch.Password)
	}
}

func TestDefaults(t *testing.T) {
	configContent := `
opensearch:
  url: "https://test.com:9200"
  indices:
    - "test-log*"
`

	tmpFile, err := os.CreateTemp("", "config-defaults-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	config, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults are applied
	if config.Query.MaxResults != 10000 {
		t.Errorf("Expected default MaxResults 10000, got %d", config.Query.MaxResults)
	}

	if config.Query.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", config.Query.Timeout)
	}

	if config.Analysis.SampleSize != 5 {
		t.Errorf("Expected default SampleSize 5, got %d", config.Analysis.SampleSize)
	}

	if config.Output.RetentionDays != 30 {
		t.Errorf("Expected default RetentionDays 30, got %d", config.Output.RetentionDays)
	}
}

// **Feature: log-analyzer, Property 18: Configuration loading robustness**
// Property 18: Configuration loading robustness
// For any valid YAML configuration file, all settings should be correctly loaded
// and environment variable substitutions should be properly resolved.
func TestConfigurationLoadingRobustness(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("configuration loading should be robust for valid configs", prop.ForAll(
		func(url, username, password string, maxResults int, timeoutSecs int) bool {
			// Generate valid configuration content
			if maxResults <= 0 {
				maxResults = 1000
			}
			if timeoutSecs <= 0 {
				timeoutSecs = 30
			}

			configContent := fmt.Sprintf(`
opensearch:
  url: "%s"
  username: "%s"
  password: "%s"
  indices:
    - "test-log*"
query:
  max_results: %d
  timeout: "%ds"
`, url, username, password, maxResults, timeoutSecs)

			// Create temporary file
			tmpFile, err := os.CreateTemp("", "config-property-test-*.yaml")
			if err != nil {
				return false
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(configContent); err != nil {
				return false
			}
			tmpFile.Close()

			// Load configuration
			config, err := Load(tmpFile.Name())
			if err != nil {
				return false
			}

			// Verify all settings are correctly loaded
			return config.OpenSearch.URL == url &&
				config.OpenSearch.Username == username &&
				config.OpenSearch.Password == password &&
				config.Query.MaxResults == maxResults &&
				config.Query.Timeout == time.Duration(timeoutSecs)*time.Second
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.IntRange(1, 50000),
		gen.IntRange(1, 300),
	))

	properties.Property("environment variable substitution should work correctly", prop.ForAll(
		func(envVarName, envVarValue string) bool {
			if len(envVarName) == 0 || len(envVarValue) == 0 {
				return true // Skip empty values
			}

			// Set environment variable
			os.Setenv(envVarName, envVarValue)
			defer os.Unsetenv(envVarName)

			configContent := fmt.Sprintf(`
opensearch:
  url: "https://test.com:9200"
  username: "${%s}"
  password: "static-password"
  indices:
    - "test-log*"
`, envVarName)

			tmpFile, err := os.CreateTemp("", "config-env-property-test-*.yaml")
			if err != nil {
				return false
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(configContent); err != nil {
				return false
			}
			tmpFile.Close()

			config, err := Load(tmpFile.Name())
			if err != nil {
				return false
			}

			return config.OpenSearch.Username == envVarValue
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	properties.TestingRun(t)
}

// **Feature: log-analyzer, Property 19: Default value application**
// Property 19: Default value application
// For any configuration with missing optional parameters, the system should apply
// sensible defaults and continue operation without errors.
func TestDefaultValueApplication(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("missing optional parameters should get sensible defaults", prop.ForAll(
		func(url string, hasMaxResults, hasTimeout, hasSampleSize, hasRetentionDays bool) bool {
			if len(url) == 0 {
				url = "https://test.com:9200"
			}

			// Build minimal config with some optional fields missing
			configContent := fmt.Sprintf(`
opensearch:
  url: "%s"
  indices:
    - "test-log*"
`, url)

			// Conditionally add optional fields
			if hasMaxResults || hasTimeout {
				configContent += `
query:`
				if hasMaxResults {
					configContent += `
  max_results: 5000`
				}
				if hasTimeout {
					configContent += `
  timeout: "45s"`
				}
			}
			if hasSampleSize {
				configContent += `
analysis:
  sample_size: 3`
			}
			if hasRetentionDays {
				configContent += `
output:
  retention_days: 60`
			}

			tmpFile, err := os.CreateTemp("", "config-defaults-property-test-*.yaml")
			if err != nil {
				return false
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(configContent); err != nil {
				return false
			}
			tmpFile.Close()

			config, err := Load(tmpFile.Name())
			if err != nil {
				return false
			}

			// Verify defaults are applied correctly
			expectedMaxResults := 10000
			if hasMaxResults {
				expectedMaxResults = 5000
			}

			expectedTimeout := 30 * time.Second
			if hasTimeout {
				expectedTimeout = 45 * time.Second
			}

			expectedSampleSize := 5
			if hasSampleSize {
				expectedSampleSize = 3
			}

			expectedRetentionDays := 30
			if hasRetentionDays {
				expectedRetentionDays = 60
			}

			return config.Query.MaxResults == expectedMaxResults &&
				config.Query.Timeout == expectedTimeout &&
				config.Analysis.SampleSize == expectedSampleSize &&
				config.Output.RetentionDays == expectedRetentionDays &&
				config.Analysis.TimeRange == "24h" && // Always default
				config.Logging.Level == "info" // Always default
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.Bool(),
		gen.Bool(),
		gen.Bool(),
		gen.Bool(),
	))

	properties.Property("completely minimal config should work with all defaults", prop.ForAll(
		func(url string) bool {
			if len(url) == 0 {
				url = "https://test.com:9200"
			}

			// Absolute minimal config
			configContent := fmt.Sprintf(`
opensearch:
  url: "%s"
  indices:
    - "test-log*"
`, url)

			tmpFile, err := os.CreateTemp("", "config-minimal-property-test-*.yaml")
			if err != nil {
				return false
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(configContent); err != nil {
				return false
			}
			tmpFile.Close()

			config, err := Load(tmpFile.Name())
			if err != nil {
				return false
			}

			// All defaults should be applied
			return config.Query.MaxResults == 10000 &&
				config.Query.Timeout == 30*time.Second &&
				config.Analysis.TimeRange == "24h" &&
				config.Analysis.SampleSize == 5 &&
				config.Analysis.Density.PeakWindowMinutes == 5 &&
				config.Analysis.Density.HighDensityThreshold == 100 &&
				config.Analysis.Density.CriticalDensityThreshold == 500 &&
				config.Output.ReportPath == "./reports" &&
				config.Output.DataPath == "./data" &&
				config.Output.PendingPath == "./pending" &&
				config.Output.RetentionDays == 30 &&
				config.Logging.Level == "info"
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}
