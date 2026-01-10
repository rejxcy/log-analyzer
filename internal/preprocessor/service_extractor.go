package preprocessor

import (
	"fmt"
	"regexp"
	"strings"

	"log-analyzer/pkg/models"
)

// ServiceExtractor handles extraction of service names from various sources
type ServiceExtractor struct {
	servicePatterns []*regexp.Regexp
}

// NewServiceExtractor creates a new service extractor
func NewServiceExtractor() *ServiceExtractor {
	// Patterns to extract service names from various formats
	patterns := []*regexp.Regexp{
		// Extract from container names like "lc-jade-prod_pp-slot-rpc-dd4bcd599-vlkp5"
		regexp.MustCompile(`([a-zA-Z0-9-_]+)(?:-[a-f0-9]{8,10}-[a-z0-9]{5})?$`),

		// Extract from service names with environment prefixes
		regexp.MustCompile(`^(?:lc-jade-prod_)?([a-zA-Z0-9-_]+)`),

		// Extract from Kubernetes pod names
		regexp.MustCompile(`^([a-zA-Z0-9-]+)-[a-f0-9]+-[a-z0-9]+$`),

		// Generic service name pattern
		regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9-_]*[a-zA-Z0-9])$`),
	}

	return &ServiceExtractor{
		servicePatterns: patterns,
	}
}

// ExtractServiceName extracts service name from raw log
func (se *ServiceExtractor) ExtractServiceName(rawLog models.RawLog) (string, error) {
	// Primary source: fields.servicename
	if rawLog.Source.Fields.ServiceName != "" {
		return se.normalizeServiceName(rawLog.Source.Fields.ServiceName), nil
	}

	// Secondary sources: try to extract from other fields
	sources := []string{
		rawLog.Source.Host["name"].(string), // This might cause panic, need to handle safely
	}

	// Safely extract host name
	if hostMap, ok := rawLog.Source.Host["name"]; ok {
		if hostName, ok := hostMap.(string); ok {
			sources = append(sources, hostName)
		}
	}

	// Try agent name
	if agentMap, ok := rawLog.Source.Agent["name"]; ok {
		if agentName, ok := agentMap.(string); ok {
			sources = append(sources, agentName)
		}
	}

	// Try to extract from log file path
	if logMap, ok := rawLog.Source.Log["file"]; ok {
		if fileMap, ok := logMap.(map[string]interface{}); ok {
			if filePath, ok := fileMap["path"].(string); ok {
				if serviceName := se.extractFromFilePath(filePath); serviceName != "" {
					return se.normalizeServiceName(serviceName), nil
				}
			}
		}
	}

	// Try each source
	for _, source := range sources {
		if source != "" {
			if serviceName := se.extractFromString(source); serviceName != "" {
				return se.normalizeServiceName(serviceName), nil
			}
		}
	}

	return "", fmt.Errorf("unable to extract service name from raw log")
}

// extractFromString attempts to extract service name from a string using patterns
func (se *ServiceExtractor) extractFromString(input string) string {
	if input == "" {
		return ""
	}

	// Try each pattern
	for _, pattern := range se.servicePatterns {
		matches := pattern.FindStringSubmatch(input)
		if len(matches) >= 2 {
			serviceName := matches[1]
			if se.isValidServiceName(serviceName) {
				return serviceName
			}
		}
	}

	// If no pattern matches, try to clean the input directly
	cleaned := se.cleanServiceName(input)
	if se.isValidServiceName(cleaned) {
		return cleaned
	}

	return ""
}

// extractFromFilePath extracts service name from file path
func (se *ServiceExtractor) extractFromFilePath(filePath string) string {
	// Example: /var/lib/docker/containers/lc-jade-prod_pp-slot-rpc-dd4bcd599-vlkp5_f0b48562.../pp-slot-rpc/0.log

	// Look for service name in the path
	pathParts := strings.Split(filePath, "/")

	for _, part := range pathParts {
		if part == "" {
			continue
		}

		// Skip common path components
		if part == "var" || part == "lib" || part == "docker" || part == "containers" {
			continue
		}

		// Try to extract service name from this part
		if serviceName := se.extractFromString(part); serviceName != "" {
			return serviceName
		}
	}

	return ""
}

// normalizeServiceName normalizes the service name
func (se *ServiceExtractor) normalizeServiceName(serviceName string) string {
	// Remove common prefixes
	prefixes := []string{
		"lc-jade-prod_",
		"lc-jade-staging_",
		"lc-jade-dev_",
		"prod_",
		"staging_",
		"dev_",
	}

	normalized := serviceName
	for _, prefix := range prefixes {
		if strings.HasPrefix(normalized, prefix) {
			normalized = strings.TrimPrefix(normalized, prefix)
			break
		}
	}

	// Clean up the name
	normalized = se.cleanServiceName(normalized)

	return normalized
}

// cleanServiceName cleans up the service name
func (se *ServiceExtractor) cleanServiceName(serviceName string) string {
	// Convert to lowercase
	cleaned := strings.ToLower(serviceName)

	// Replace underscores with hyphens for consistency
	cleaned = strings.ReplaceAll(cleaned, "_", "-")

	// Remove invalid characters (keep only alphanumeric and hyphens)
	var result strings.Builder
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	cleaned = result.String()

	// Remove leading/trailing hyphens
	cleaned = strings.Trim(cleaned, "-")

	// Replace multiple consecutive hyphens with single hyphen
	for strings.Contains(cleaned, "--") {
		cleaned = strings.ReplaceAll(cleaned, "--", "-")
	}

	return cleaned
}

// isValidServiceName validates if the extracted name looks like a valid service name
func (se *ServiceExtractor) isValidServiceName(serviceName string) bool {
	if len(serviceName) < 2 || len(serviceName) > 100 {
		return false
	}

	// Must start and end with alphanumeric
	if !((serviceName[0] >= 'a' && serviceName[0] <= 'z') ||
		(serviceName[0] >= '0' && serviceName[0] <= '9')) {
		return false
	}

	lastChar := serviceName[len(serviceName)-1]
	if !((lastChar >= 'a' && lastChar <= 'z') ||
		(lastChar >= '0' && lastChar <= '9')) {
		return false
	}

	// Check for invalid patterns
	invalidPatterns := []string{
		"filebeat",
		"logstash",
		"fluentd",
		"unknown",
		"default",
		"system",
		"kernel",
	}

	for _, invalid := range invalidPatterns {
		if serviceName == invalid {
			return false
		}
	}

	return true
}

// GetServiceNameVariations returns possible variations of a service name
func (se *ServiceExtractor) GetServiceNameVariations(serviceName string) []string {
	variations := []string{serviceName}

	// Add variations with different separators
	if strings.Contains(serviceName, "-") {
		variations = append(variations, strings.ReplaceAll(serviceName, "-", "_"))
	}
	if strings.Contains(serviceName, "_") {
		variations = append(variations, strings.ReplaceAll(serviceName, "_", "-"))
	}

	// Add variations with common prefixes
	prefixes := []string{
		"lc-jade-prod_",
		"lc-jade-staging_",
		"prod_",
	}

	for _, prefix := range prefixes {
		variations = append(variations, prefix+serviceName)
	}

	return variations
}

// ValidateServiceName validates that a service name meets requirements
func (se *ServiceExtractor) ValidateServiceName(serviceName string) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	if !se.isValidServiceName(serviceName) {
		return fmt.Errorf("invalid service name format: %s", serviceName)
	}

	return nil
}
