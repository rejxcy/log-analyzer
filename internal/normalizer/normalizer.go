package normalizer

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"log-analyzer/pkg/models"
)

// LogNormalizer implements the Normalizer interface
type LogNormalizer struct {
	uuidRegex *regexp.Regexp
}

// NewLogNormalizer creates a new log normalizer
func NewLogNormalizer() *LogNormalizer {
	// Regex to match common UUID patterns
	uuidRegex := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\b[0-9a-f]{32}\b`)

	return &LogNormalizer{
		uuidRegex: uuidRegex,
	}
}

// NormalizationConfig contains configuration for normalization
type NormalizationConfig struct {
	// ReplaceLiterals replaces specific literals with placeholders
	ReplaceLiterals map[string]string
	// MinSamplesPerGroup minimum samples to keep per error group
	MinSamplesPerGroup int
	// MaxSamplesPerGroup maximum samples to keep per error group
	MaxSamplesPerGroup int
}

// DefaultNormalizationConfig returns default configuration
func DefaultNormalizationConfig() NormalizationConfig {
	return NormalizationConfig{
		MinSamplesPerGroup: 3,
		MaxSamplesPerGroup: 5,
		ReplaceLiterals:    make(map[string]string),
	}
}

// Normalize processes logs and groups them by fingerprint
func (n *LogNormalizer) Normalize(logs []models.ParsedLog) ([]models.ErrorGroup, error) {
	config := DefaultNormalizationConfig()
	return n.NormalizeWithConfig(logs, config)
}

// NormalizeWithConfig processes logs with custom configuration
func (n *LogNormalizer) NormalizeWithConfig(logs []models.ParsedLog, config NormalizationConfig) ([]models.ErrorGroup, error) {
	// Group logs by fingerprint
	groupMap := make(map[string]*models.ErrorGroup)
	timeDistribution := make(map[string]map[string]int) // fingerprint -> hour -> count

	for _, log := range logs {
		// Normalize content
		normalizedContent := n.normalizeContent(log.Content, config)

		// Calculate fingerprint
		fingerprint := n.calculateFingerprint(normalizedContent, log.ServiceName, log.Caller)

		// Initialize group if not exists
		if _, exists := groupMap[fingerprint]; !exists {
			groupMap[fingerprint] = &models.ErrorGroup{
				Fingerprint:       fingerprint,
				NormalizedContent: normalizedContent,
				ServiceName:       log.ServiceName,
				CallerFile:        log.Caller,
				TotalCount:        0,
				Samples:           []models.ParsedLog{},
				TimeDistribution:  make(map[string]int),
			}
			timeDistribution[fingerprint] = make(map[string]int)
		}

		group := groupMap[fingerprint]
		group.TotalCount++

		// Add sample if under max limit
		if len(group.Samples) < config.MaxSamplesPerGroup {
			group.Samples = append(group.Samples, log)
		}

		// Update time distribution (by hour)
		hour := log.Timestamp.Hour()
		hourKey := fmt.Sprintf("%02d:00", hour)
		group.TimeDistribution[hourKey]++
	}

	// Convert map to slice
	var errorGroups []models.ErrorGroup
	for _, group := range groupMap {
		// Sort samples by timestamp for consistency
		sort.Slice(group.Samples, func(i, j int) bool {
			return group.Samples[i].Timestamp.Before(group.Samples[j].Timestamp)
		})

		// Ensure minimum samples
		if len(group.Samples) > config.MinSamplesPerGroup {
			group.Samples = group.Samples[:config.MinSamplesPerGroup]
		}

		// Calculate peak window
		group.PeakWindow = n.calculatePeakWindow(group.TimeDistribution)

		errorGroups = append(errorGroups, *group)
	}

	// Sort by total count descending for consistent ordering
	sort.Slice(errorGroups, func(i, j int) bool {
		return errorGroups[i].TotalCount > errorGroups[j].TotalCount
	})

	return errorGroups, nil
}

// normalizeContent normalizes log content for fingerprinting
func (n *LogNormalizer) normalizeContent(content string, config NormalizationConfig) string {
	// Convert to lowercase
	normalized := strings.ToLower(content)

	// Replace UUIDs with placeholder
	normalized = n.uuidRegex.ReplaceAllString(normalized, "[UUID]")

	// Replace numbers with placeholder (to group similar errors with different IDs)
	numRegex := regexp.MustCompile(`\b\d+\b`)
	normalized = numRegex.ReplaceAllString(normalized, "[NUM]")

	// Replace custom literals
	for literal, placeholder := range config.ReplaceLiterals {
		normalized = strings.ReplaceAll(normalized, strings.ToLower(literal), placeholder)
	}

	// Collapse multiple spaces
	spaceRegex := regexp.MustCompile(`\s+`)
	normalized = spaceRegex.ReplaceAllString(normalized, " ")

	// Trim spaces
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// calculateFingerprint calculates a SHA256 fingerprint for error grouping
func (n *LogNormalizer) calculateFingerprint(normalizedContent, serviceName, caller string) string {
	// Combine normalized content with service name and caller
	combined := fmt.Sprintf("%s|%s|%s", normalizedContent, serviceName, caller)

	// Calculate SHA256 hash
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash)
}

// calculatePeakWindow finds the time window with highest error density
func (n *LogNormalizer) calculatePeakWindow(distribution map[string]int) *models.PeakWindow {
	if len(distribution) == 0 {
		return nil
	}

	// Find the hour with maximum count
	var peakHour string
	maxCount := 0

	for hour, count := range distribution {
		if count > maxCount {
			maxCount = count
			peakHour = hour
		}
	}

	if peakHour == "" {
		return nil
	}

	// Parse the peak hour
	peakTime, err := time.Parse("2006-01-02T15:00:00Z", peakHour)
	if err != nil {
		return nil
	}

	// Calculate density (errors per minute in that hour)
	density := float64(maxCount) / 60.0

	return &models.PeakWindow{
		Start:   peakTime,
		End:     peakTime.Add(1 * time.Hour),
		Count:   maxCount,
		Density: density,
	}
}

// NormalizationStats contains statistics about normalization
type NormalizationStats struct {
	TotalLogs       int     `json:"total_logs"`
	UniqueGroups    int     `json:"unique_groups"`
	DuplicationRate float64 `json:"duplication_rate"`
}

// GetNormalizationStats returns statistics about the normalization operation
func GetNormalizationStats(originalCount int, groups []models.ErrorGroup) NormalizationStats {
	stats := NormalizationStats{
		TotalLogs:    originalCount,
		UniqueGroups: len(groups),
	}

	if originalCount > 0 {
		stats.DuplicationRate = 1.0 - (float64(len(groups)) / float64(originalCount))
	}

	return stats
}
