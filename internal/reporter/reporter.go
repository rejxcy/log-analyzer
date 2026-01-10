package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"log-analyzer/internal/aggregator"
	"log-analyzer/internal/config"
	"log-analyzer/internal/interfaces"
	"log-analyzer/pkg/models"
)

// MarkdownReporter implements the Reporter interface
type MarkdownReporter struct {
	reportPath string
}

// NewMarkdownReporter creates a new markdown reporter
func NewMarkdownReporter(reportPath string) *MarkdownReporter {
	return &MarkdownReporter{
		reportPath: reportPath,
	}
}

// Generate generates a markdown report from analysis results
func (r *MarkdownReporter) Generate(analyses []models.Analysis, stats *interfaces.AggregationResult) (*models.Report, error) {
	// Create report directory if not exists
	if err := os.MkdirAll(r.reportPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create report directory: %w", err)
	}

	// Generate report content
	content := r.generateReportContent(analyses, stats)

	// Generate timestamp for filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("error_analysis_%s.md", timestamp)
	reportPath := filepath.Join(r.reportPath, filename)

	// Write report to file
	if err := os.WriteFile(reportPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write report file: %w", err)
	}

	return &models.Report{
		GeneratedAt:       time.Now(),
		ExecutionTime:     stats.ProcessingTime,
		TotalLogs:         stats.TotalLogs,
		ErrorGroupCount:   stats.TotalErrorGroups,
		HighPriorityCount: countHighPriority(analyses),
		NewIssueCount:     countNewIssues(analyses),
		ReportPath:        reportPath,
		DataSources:       []string{"opensearch"},
	}, nil
}

// generateReportContent generates the markdown content for the report (Engineer-focused)
func (r *MarkdownReporter) generateReportContent(analyses []models.Analysis, stats *interfaces.AggregationResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# 🔍 每日錯誤分析報告\n\n")
	sb.WriteString(fmt.Sprintf("**生成時間**: %s  \n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**分析週期**: 過去 24 小時\n\n"))

	// Count known vs unknown issues
	knownCount := 0
	unknownCount := 0
	for _, a := range analyses {
		if a.IsKnown {
			knownCount++
		} else {
			unknownCount++
		}
	}
	sb.WriteString(fmt.Sprintf("**已知問題**: %d | **新問題**: %d\n\n", knownCount, unknownCount))

	// Sort analyses by severity
	sortedAnalyses := sortBySeverity(analyses)

	// Daily Verdict (3-5 line executive summary)
	r.writeDailyVerdictSection(&sb, sortedAnalyses, stats)

	// Top Problems (max 5 detailed issues)
	r.writeTopProblemsSection(&sb, sortedAnalyses, stats)

	// Secondary Issues (low frequency, summary format)
	r.writeSecondaryIssuesSection(&sb, sortedAnalyses, stats)

	return sb.String()
}

// writeDailyVerdictSection writes the executive summary verdict
func (r *MarkdownReporter) writeDailyVerdictSection(sb *strings.Builder, analyses []models.Analysis, stats *interfaces.AggregationResult) {
	sb.WriteString("## 📊 每日總結\n\n")

	highCount := countHighPriority(analyses)
	totalLogs := stats.TotalLogs

	var verdict string
	if highCount >= 3 {
		verdict = "🔴 **危急** - 檢測到多個高嚴重性問題。需要立即調查。"
	} else if highCount > 0 {
		verdict = "🟡 **警告** - 存在高嚴重性問題。優先修復這些項目。"
	} else {
		verdict = "🟢 **正常** - 沒有危急問題。監控持續進行的模式。"
	}

	sb.WriteString(fmt.Sprintf("%s\n\n", verdict))
	sb.WriteString(fmt.Sprintf("- **總錯誤數**: %d 個錯誤，涉及 %d 個唯一模式\n", totalLogs, stats.TotalErrorGroups))
	sb.WriteString(fmt.Sprintf("- **高優先級問題**: %d 個\n", highCount))
	sb.WriteString(fmt.Sprintf("- **峰值時段**: %02d:00（%d 個錯誤）\n", stats.TimeStats.PeakHour, stats.TimeStats.PeakCount))

	// Show top 2 most urgent problems
	if len(analyses) > 0 {
		sb.WriteString("\n**最緊急的問題**:\n")
		limit := 2
		if len(analyses) < limit {
			limit = len(analyses)
		}
		for i := 0; i < limit; i++ {
			a := analyses[i]
			sb.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, extractProblemName(a), a.Severity))
		}
	}

	sb.WriteString("\n---\n\n")
}

// writeErrorDistributionSection writes error distribution details
func (r *MarkdownReporter) writeTopProblemsSection(sb *strings.Builder, analyses []models.Analysis, stats *interfaces.AggregationResult) {
	sb.WriteString("## 🚨 頂級問題\n\n")

	limit := 5
	if len(analyses) < limit {
		limit = len(analyses)
	}

	registry := config.GetRegistry()

	for i := 0; i < limit; i++ {
		a := analyses[i]
		problemNum := i + 1

		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", problemNum, extractProblemName(a)))

		// Extract error details
		errorMsg := ""
		location := ""
		if len(a.SuggestedActions) > 0 {
			errorMsg = extractErrorMessage(a.SuggestedActions[0])
		}
		if len(a.SuggestedActions) > 1 {
			location = extractLocation(a.SuggestedActions[1])
		}

		// Extract count from reason (e.g., "Error occurred 274 times in service...")
		count := extractCountFromReason(a.Reason)

		sb.WriteString(fmt.Sprintf("**位置**: `%s`  \n", location))
		sb.WriteString(fmt.Sprintf("**發生次數**: %s  \n", count))
		sb.WriteString(fmt.Sprintf("**錯誤訊息**: \n```\n%s\n```\n\n", errorMsg))

		// Show known issue information if applicable
		if a.IsKnown && a.IssueID != "" {
			issue := registry.GetIssueByID(a.IssueID)
			if issue != nil {
				sb.WriteString(fmt.Sprintf("**已知問題**: `%s` - %s  \n", issue.ID, issue.Name))
				sb.WriteString(fmt.Sprintf("**分類**: %s  \n", issue.Category))
			}
		}

		// Determine time pattern
		pattern := determineTimePattern(a, stats)
		sb.WriteString(fmt.Sprintf("**時間模式**: %s  \n", pattern))

		// Severity with reasoning
		severityReason := calculateSeverityReason(a, count)
		sb.WriteString(fmt.Sprintf("**嚴重性**: 🔴 **%s** - %s  \n", strings.ToUpper(string(a.Severity)), severityReason))

		// Engineering suggestion
		suggestion := deriveEngineeringSuggestion(a, pattern)
		sb.WriteString(fmt.Sprintf("**下一步**: %s\n\n", suggestion))
	}

	sb.WriteString("---\n\n")
}

// writeTimeDistributionSection -> writeSecondaryIssuesSection
func (r *MarkdownReporter) writeSecondaryIssuesSection(sb *strings.Builder, analyses []models.Analysis, stats *interfaces.AggregationResult) {
	if len(analyses) <= 5 {
		return
	}

	sb.WriteString("## 📝 其他問題（低頻率）\n\n")
	sb.WriteString("| 問題名稱 | 位置 | 發生次數 | 狀態 | 嚴重性 |\n")
	sb.WriteString("|---------|------|--------|------|-------|\n")

	registry := config.GetRegistry()

	for i := 5; i < len(analyses); i++ {
		a := analyses[i]
		count := extractCountFromReason(a.Reason)
		location := ""
		if len(a.SuggestedActions) > 1 {
			location = extractLocation(a.SuggestedActions[1])
		}

		status := "🆕 新問題"
		if a.IsKnown && a.IssueID != "" {
			issue := registry.GetIssueByID(a.IssueID)
			if issue != nil {
				status = fmt.Sprintf("✅ %s", issue.ID)
			}
		}

		sb.WriteString(fmt.Sprintf("| %s | `%s` | %s | %s | %s |\n",
			extractProblemName(a),
			location,
			count,
			status,
			a.Severity,
		))
	}

	sb.WriteString("\n")
}

// extractErrorMessage extracts the error message from action text
func extractErrorMessage(action string) string {
	// Extract from "Investigate error pattern: <message>"
	if strings.Contains(action, ":") {
		parts := strings.SplitN(action, ": ", 2)
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return action
}

// extractLocation extracts the file location from action text
func extractLocation(action string) string {
	// Extract from "Check logs from caller: <location>"
	if strings.Contains(action, ": ") {
		parts := strings.SplitN(action, ": ", 2)
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return action
}

// writeTimeDistributionSection writes the time distribution chart
func (r *MarkdownReporter) writeTimeDistributionSection(sb *strings.Builder, stats *interfaces.AggregationResult) {
	sb.WriteString("## Time Distribution\n\n")
	sb.WriteString("```\n")

	// Create simple ASCII chart
	maxCount := 0
	for _, count := range stats.TimeStats.HourlyDistribution {
		if count > maxCount {
			maxCount = count
		}
	}

	if maxCount > 0 {
		for hour := 0; hour < 24; hour++ {
			count := stats.TimeStats.HourlyDistribution[hour]
			barLength := (count * 40) / maxCount
			bar := strings.Repeat("█", barLength)
			sb.WriteString(fmt.Sprintf("%02d:00 | %s %d\n", hour, bar, count))
		}
	}

	sb.WriteString("```\n\n")
}

// writeServiceImpactSection -> removed
// All helper functions now included below

// Helper functions

func sortBySeverity(analyses []models.Analysis) []models.Analysis {
	sorted := make([]models.Analysis, len(analyses))
	copy(sorted, analyses)
	sort.Slice(sorted, func(i, j int) bool {
		severityRank := map[models.Severity]int{
			models.SeverityCritical: 0,
			models.SeverityHigh:     1,
			models.SeverityMedium:   2,
			models.SeverityLow:      3,
		}
		return severityRank[sorted[i].Severity] < severityRank[sorted[j].Severity]
	})
	return sorted
}

func extractProblemName(a models.Analysis) string {
	// Extract from reason or error message
	if len(a.SuggestedActions) > 0 {
		msg := extractErrorMessage(a.SuggestedActions[0])
		if len(msg) > 60 {
			return msg[:60] + "..."
		}
		return msg
	}
	return a.Reason
}

func extractCountFromReason(reason string) string {
	// Extract "N times" from reason like "Error occurred 274 times in service..."
	if strings.Contains(reason, " times ") {
		parts := strings.Split(reason, " ")
		for i, p := range parts {
			if p == "times" && i > 0 {
				return parts[i-1]
			}
		}
	}
	return "unknown"
}

func determineTimePattern(a models.Analysis, stats *interfaces.AggregationResult) string {
	// Simplified pattern detection based on severity and count
	// If high count during peak hour, it's burst
	if a.Severity == models.SeverityHigh {
		return "**爆發型** - 在峰值時段集中（需要立即關注）"
	} else if a.Severity == models.SeverityMedium {
		return "**持續型** - 整天分散分佈"
	}
	return "**零星型** - 偶爾發生"
}

func calculateSeverityReason(a models.Analysis, count string) string {
	reasons := []string{
		fmt.Sprintf("高頻率錯誤（%s 次發生）", count),
		"在業務時段集中",
		"可能影響用戶體驗",
	}

	if a.Severity == models.SeverityHigh {
		return strings.Join(reasons, " + ")
	} else if a.Severity == models.SeverityMedium {
		return "中等影響，應該追蹤"
	}
	return "低影響，可以延後處理"
}

func deriveEngineeringSuggestion(a models.Analysis, pattern string) string {
	if strings.Contains(pattern, "爆發型") {
		return "檢查峰值時段附近的最近部署或流量變化"
	} else if strings.Contains(pattern, "持續型") {
		return "建立工單進行根本原因分析和監控"
	}
	return "監控升級情況，暫無需要立即採取行動"
}

// SaveAnalysisJSON saves analysis results as JSON for further processing
func SaveAnalysisJSON(analyses []models.Analysis, stats *interfaces.AggregationResult, outputPath string) error {
	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare data structure
	data := map[string]interface{}{
		"timestamp":   time.Now(),
		"analyses":    analyses,
		"aggregation": stats,
		"agg_stats":   aggregator.GetAggregationStats(stats),
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to file
	filename := time.Now().Format("2006-01-02_15-04-05")
	filepath := filepath.Join(outputPath, fmt.Sprintf("analysis_%s.json", filename))
	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

// countHighPriority counts the number of high priority errors
func countHighPriority(analyses []models.Analysis) int {
	count := 0
	for _, analysis := range analyses {
		if analysis.Severity == models.SeverityHigh || analysis.Severity == models.SeverityCritical {
			count++
		}
	}
	return count
}

// countNewIssues counts the number of new unknown issues
func countNewIssues(analyses []models.Analysis) int {
	count := 0
	for _, analysis := range analyses {
		if !analysis.IsKnown {
			count++
		}
	}
	return count
}
