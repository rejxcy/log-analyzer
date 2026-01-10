package aggregator

import (
	"fmt"
	"sort"
	"time"

	"log-analyzer/internal/interfaces"
	"log-analyzer/pkg/models"
)

// LogAggregator implements the Aggregator interface
type LogAggregator struct{}

// NewLogAggregator creates a new log aggregator
func NewLogAggregator() *LogAggregator {
	return &LogAggregator{}
}

// Aggregate performs statistical analysis on error groups
func (a *LogAggregator) Aggregate(groups []models.ErrorGroup) (*interfaces.AggregationResult, error) {
	startTime := time.Now()

	result := &interfaces.AggregationResult{
		ServiceStats:     make(map[string]*interfaces.ServiceStats),
		TimeStats:        &interfaces.TimeStats{},
		TotalErrorGroups: len(groups),
		ProcessingTime:   0,
	}

	// Initialize time stats
	result.TimeStats.HourlyDistribution = make(map[int]int)

	// Process each error group
	var totalLogs int
	for _, group := range groups {
		totalLogs += group.TotalCount

		// Update service stats
		if _, exists := result.ServiceStats[group.ServiceName]; !exists {
			result.ServiceStats[group.ServiceName] = &interfaces.ServiceStats{
				ServiceName:       group.ServiceName,
				ErrorGroupCount:   0,
				TotalErrors:       0,
				HighPriorityCount: 0,
				PeakDensity:       0,
			}
		}

		serviceStats := result.ServiceStats[group.ServiceName]
		serviceStats.ErrorGroupCount++
		serviceStats.TotalErrors += group.TotalCount

		// Update peak density for this service
		if group.PeakWindow != nil && group.PeakWindow.Density > serviceStats.PeakDensity {
			serviceStats.PeakDensity = group.PeakWindow.Density
		}

		// Update hourly distribution
		for hourKey, count := range group.TimeDistribution {
			// hourKey is already in format "HH:00"
			var hour int
			fmt.Sscanf(hourKey, "%02d:00", &hour)
			result.TimeStats.HourlyDistribution[hour] += count
		}
	}

	result.TotalLogs = totalLogs

	// Calculate hourly distribution peaks
	a.calculateHourlyStats(result.TimeStats)

	// Calculate average density
	if len(groups) > 0 {
		var totalDensity float64
		for _, group := range groups {
			if group.PeakWindow != nil {
				totalDensity += group.PeakWindow.Density
			}
		}
		result.TimeStats.AverageDensity = totalDensity / float64(len(groups))
	}

	result.ProcessingTime = time.Since(startTime)

	return result, nil
}

// calculateHourlyStats calculates peak hours and average density
func (a *LogAggregator) calculateHourlyStats(timeStats *interfaces.TimeStats) {
	if len(timeStats.HourlyDistribution) == 0 {
		return
	}

	maxCount := 0
	peakHour := 0

	for hour, count := range timeStats.HourlyDistribution {
		if count > maxCount {
			maxCount = count
			peakHour = hour
		}
	}

	timeStats.PeakHour = peakHour
	timeStats.PeakCount = maxCount
}

// AggregationStats contains statistics about aggregation results
type AggregationStats struct {
	TotalErrorGroups int                       `json:"total_error_groups"`
	TotalLogs        int                       `json:"total_logs"`
	TotalServices    int                       `json:"total_services"`
	PeakHour         int                       `json:"peak_hour"`
	PeakCount        int                       `json:"peak_count"`
	AverageDensity   float64                   `json:"average_density"`
	ServicesSorted   []interfaces.ServiceStats `json:"services_sorted"`
	ProcessingTimeMs int64                     `json:"processing_time_ms"`
}

// GetAggregationStats returns statistics about the aggregation operation
func GetAggregationStats(result *interfaces.AggregationResult) AggregationStats {
	stats := AggregationStats{
		TotalErrorGroups: result.TotalErrorGroups,
		TotalLogs:        result.TotalLogs,
		TotalServices:    len(result.ServiceStats),
		PeakHour:         result.TimeStats.PeakHour,
		PeakCount:        result.TimeStats.PeakCount,
		AverageDensity:   result.TimeStats.AverageDensity,
		ProcessingTimeMs: result.ProcessingTime.Milliseconds(),
		ServicesSorted:   make([]interfaces.ServiceStats, 0),
	}

	// Sort services by error count
	for _, svc := range result.ServiceStats {
		stats.ServicesSorted = append(stats.ServicesSorted, *svc)
	}
	sort.Slice(stats.ServicesSorted, func(i, j int) bool {
		return stats.ServicesSorted[i].TotalErrors > stats.ServicesSorted[j].TotalErrors
	})

	return stats
}
