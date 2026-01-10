package preprocessor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"log-analyzer/pkg/models"
)

// LogPreprocessor implements the Preprocessor interface
type LogPreprocessor struct {
	wrapperRegex *regexp.Regexp
}

// NewLogPreprocessor creates a new log preprocessor
func NewLogPreprocessor() *LogPreprocessor {
	// Regex to match Kubernetes wrapper format: "TIMESTAMP stderr F JSON_CONTENT"
	wrapperRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s+stderr\s+F\s+(.*)$`)

	return &LogPreprocessor{
		wrapperRegex: wrapperRegex,
	}
}

// Process processes raw logs and extracts structured data
func (p *LogPreprocessor) Process(rawLogs []models.RawLog) ([]models.ParsedLog, error) {
	var parsedLogs []models.ParsedLog
	var processingErrors []error

	for i, rawLog := range rawLogs {
		parsedLog, err := p.processRawLog(rawLog)
		if err != nil {
			// Log the error but continue processing other logs
			processingErrors = append(processingErrors, fmt.Errorf("failed to process log %d: %w", i, err))
			continue
		}

		if parsedLog != nil {
			parsedLogs = append(parsedLogs, *parsedLog)
		}
	}

	// Return results even if some logs failed to process (graceful degradation)
	if len(parsedLogs) == 0 && len(processingErrors) > 0 {
		return nil, fmt.Errorf("failed to process any logs: %v", processingErrors)
	}

	return parsedLogs, nil
}

// processRawLog processes a single raw log entry
func (p *LogPreprocessor) processRawLog(rawLog models.RawLog) (*models.ParsedLog, error) {
	// Extract the message content from the raw log
	messageContent := ""

	// Try to get message from different possible sources
	if rawLog.Source.Message != "" {
		messageContent = rawLog.Source.Message
	} else if rawLog.Source.Event.Original != "" {
		messageContent = rawLog.Source.Event.Original
	} else {
		return nil, fmt.Errorf("no message content found in raw log")
	}

	// Remove Kubernetes wrapper if present
	innerJSON, err := p.removeWrapper(messageContent)
	if err != nil {
		return nil, fmt.Errorf("failed to remove wrapper: %w", err)
	}

	// Parse the inner JSON
	innerLog, err := p.parseInnerJSON(innerJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inner JSON: %w", err)
	}

	// Extract service name from the raw log using ServiceExtractor
	extractor := NewServiceExtractor()
	serviceName, err := extractor.ExtractServiceName(rawLog)
	if err != nil {
		// Fallback: try to get from Fields.ServiceName
		serviceName = rawLog.Source.Fields.ServiceName
		if serviceName == "" {
			// Last resort: extract from rawLog index name
			serviceName = extractServiceFromIndex(rawLog.Index)
			if serviceName == "" {
				return nil, fmt.Errorf("unable to extract service name from log")
			}
		}
	}

	// Create parsed log
	parsedLog := &models.ParsedLog{
		Timestamp:   innerLog.Timestamp,
		Caller:      innerLog.Caller,
		Content:     innerLog.Content,
		Level:       innerLog.Level,
		Span:        innerLog.Span,
		Trace:       innerLog.Trace,
		ServiceName: serviceName,
	}

	// Validate required fields
	if err := p.validateParsedLog(parsedLog); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return parsedLog, nil
}

// removeWrapper removes the Kubernetes wrapper and extracts the inner JSON
func (p *LogPreprocessor) removeWrapper(message string) (string, error) {
	// Check if the message has the Kubernetes wrapper format
	matches := p.wrapperRegex.FindStringSubmatch(message)
	if len(matches) >= 2 {
		// Extract the JSON part (group 1)
		return matches[1], nil
	}

	// If no wrapper found, assume the message is already clean JSON
	// This handles cases where logs might not have the wrapper
	if strings.HasPrefix(strings.TrimSpace(message), "{") {
		return message, nil
	}

	return "", fmt.Errorf("message does not match expected format: %s", message[:min(100, len(message))])
}

// InnerLogData represents the structure of the inner JSON log
type InnerLogData struct {
	Timestamp time.Time `json:"@timestamp"`
	Caller    string    `json:"caller"`
	Content   string    `json:"content"`
	Level     string    `json:"level"`
	Span      string    `json:"span"`
	Trace     string    `json:"trace"`
}

// parseInnerJSON parses the inner JSON content
func (p *LogPreprocessor) parseInnerJSON(jsonStr string) (*InnerLogData, error) {
	// Clean up the JSON string (handle escaped quotes)
	cleanJSON := strings.ReplaceAll(jsonStr, `\"`, `"`)

	var innerLog InnerLogData
	if err := json.Unmarshal([]byte(cleanJSON), &innerLog); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w, content: %s", err, jsonStr[:min(200, len(jsonStr))])
	}

	return &innerLog, nil
}

// validateParsedLog validates that the parsed log has required fields
func (p *LogPreprocessor) validateParsedLog(log *models.ParsedLog) error {
	if log.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	if log.Content == "" {
		return fmt.Errorf("content is required")
	}

	if log.Level == "" {
		return fmt.Errorf("level is required")
	}

	if log.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	// Validate log level
	validLevels := map[string]bool{
		"error": true,
		"warn":  true,
		"info":  true,
		"debug": true,
	}

	if !validLevels[strings.ToLower(log.Level)] {
		return fmt.Errorf("invalid log level: %s", log.Level)
	}

	return nil
}

// GetProcessingStats returns statistics about the preprocessing operation
func (p *LogPreprocessor) GetProcessingStats(rawLogs []models.RawLog, parsedLogs []models.ParsedLog) ProcessingStats {
	stats := ProcessingStats{
		TotalRawLogs:       len(rawLogs),
		SuccessfullyParsed: len(parsedLogs),
		Failed:             len(rawLogs) - len(parsedLogs),
	}

	if len(rawLogs) > 0 {
		stats.SuccessRate = float64(len(parsedLogs)) / float64(len(rawLogs))
	}

	// Count by log level
	stats.LevelCounts = make(map[string]int)
	for _, log := range parsedLogs {
		stats.LevelCounts[log.Level]++
	}

	return stats
}

// extractServiceFromIndex extracts service name from OpenSearch index name
// e.g., "pp-slot-api-log*" -> "pp-slot-api"
func extractServiceFromIndex(indexName string) string {
	// Remove wildcard
	indexName = strings.TrimSuffix(indexName, "*")

	// Remove common suffixes
	suffixes := []string{"-log", "-prod", "-staging"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(indexName, suffix) {
			indexName = strings.TrimSuffix(indexName, suffix)
		}
	}

	return indexName
}

// ProcessingStats contains statistics about the preprocessing operation
type ProcessingStats struct {
	TotalRawLogs       int            `json:"total_raw_logs"`
	SuccessfullyParsed int            `json:"successfully_parsed"`
	Failed             int            `json:"failed"`
	SuccessRate        float64        `json:"success_rate"`
	LevelCounts        map[string]int `json:"level_counts"`
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
