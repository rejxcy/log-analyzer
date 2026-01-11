package pipeline

import (
	"fmt"

	"log-analyzer/internal/aggregator"
	"log-analyzer/internal/config"
	"log-analyzer/internal/fetcher"
	"log-analyzer/internal/interfaces"
	"log-analyzer/internal/normalizer"
	"log-analyzer/internal/preprocessor"
	"log-analyzer/internal/reporter"
	"log-analyzer/pkg/models"
)

// Pipeline orchestrates the entire log analysis workflow
type Pipeline struct {
	fetcher      *fetcher.Fetcher
	preprocessor *preprocessor.LogPreprocessor
	normalizer   *normalizer.LogNormalizer
	aggregator   *aggregator.LogAggregator
	reporter     *reporter.MarkdownReporter
	config       *config.Config
}

// NewPipeline creates a new pipeline
func NewPipeline(cfg *config.Config) *Pipeline {
	return &Pipeline{
		fetcher:      fetcher.NewFetcher(cfg),
		preprocessor: preprocessor.NewLogPreprocessor(),
		normalizer:   normalizer.NewLogNormalizer(),
		aggregator:   aggregator.NewLogAggregator(),
		reporter:     reporter.NewMarkdownReporter(cfg.Output.ReportDir),
		config:       cfg,
	}
}

// PipelineResult represents the result of running the pipeline
type PipelineResult struct {
	RawLogs           []models.RawLog
	ParsedLogs        []models.ParsedLog
	ErrorGroups       []models.ErrorGroup
	Analyses          []models.Analysis
	AggregationResult *interfaces.AggregationResult
	Reports           map[string]*models.Report
}

// Run executes the entire pipeline
func (p *Pipeline) Run(timeRangeStr string) (*PipelineResult, error) {
	result := &PipelineResult{
		Reports: make(map[string]*models.Report),
	}

	// Step 0: Fetch from OpenSearch
	fmt.Printf("📡 第 0 步：從 OpenSearch 獲取日誌（過去 %s）...\n", timeRangeStr)
	rawLogs, err := p.fetcher.FetchWithTimeWindows(timeRangeStr)
	if err != nil {
		return nil, fmt.Errorf("fetching failed: %w", err)
	}

	if len(rawLogs) == 0 {
		fmt.Println("⚠️  指定時間範圍內找不到日誌。")
		fmt.Println("   提示：嘗試更長的時間範圍（例如：-time 48h）")
		return result, nil
	}

	fmt.Printf("✅ 成功獲取 %d 條原始日誌\n", len(rawLogs))
	result.RawLogs = rawLogs

	// Show service distribution
	p.printServiceDistribution(rawLogs)

	// Step 1: Preprocess
	fmt.Println("🔄 第 1 步：預處理日誌...")
	parsedLogs, err := p.preprocessor.Process(rawLogs)
	if err != nil {
		return nil, fmt.Errorf("preprocessing failed: %w", err)
	}
	fmt.Printf("✅ 成功解析 %d 條日誌\n\n", len(parsedLogs))
	result.ParsedLogs = parsedLogs

	// Step 2: Normalize
	fmt.Println("🔐 第 2 步：正規化和分組錯誤...")
	errorGroups, err := p.normalizer.Normalize(parsedLogs)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}
	normStats := normalizer.GetNormalizationStats(len(parsedLogs), errorGroups)
	fmt.Printf("✅ 分組為 %d 個唯一錯誤模式（%.1f%% 重複率）\n\n",
		len(errorGroups), normStats.DuplicationRate*100)
	result.ErrorGroups = errorGroups

	// Step 3: Aggregate
	fmt.Println("📊 第 3 步：聚合統計資訊...")
	aggResult, err := p.aggregator.Aggregate(errorGroups)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}
	aggStats := aggregator.GetAggregationStats(aggResult)
	fmt.Printf("✅ 聚合完成：\n")
	fmt.Printf("   - 總錯誤數：%d\n", aggStats.TotalLogs)
	fmt.Printf("   - 服務總數：%d\n", aggStats.TotalServices)
	fmt.Printf("   - 峰值時段：%02d:00（%d 個錯誤）\n", aggStats.PeakHour, aggStats.PeakCount)
	fmt.Printf("   - 平均密度：%.2f 錯誤/分鐘\n\n", aggStats.AverageDensity)
	result.AggregationResult = aggResult

	// Step 4: Analyze
	fmt.Println("🔍 第 4 步：分析錯誤模式...")
	analyses := p.createAnalysesFromErrorGroups(errorGroups)
	fmt.Printf("✅ 從實際數據建立了 %d 個分析結果\n\n", len(analyses))
	result.Analyses = analyses

	// Step 5: Generate reports (one per service)
	fmt.Println("📄 第 5 步：為每個服務生成 Markdown 報告...")
	if err := p.generatePerServiceReports(analyses, errorGroups, aggResult, result); err != nil {
		return nil, fmt.Errorf("report generation failed: %w", err)
	}
	fmt.Println()

	// Step 6: Save JSON
	fmt.Println("💾 第 6 步：將分析結果保存為 JSON...")
	if err := reporter.SaveAnalysisJSON(analyses, aggResult, p.config.Output.ReportDir); err != nil {
		return nil, fmt.Errorf("saving JSON failed: %w", err)
	}
	fmt.Println("✅ 分析 JSON 已保存")

	return result, nil
}

// printServiceDistribution prints service distribution from raw logs
func (p *Pipeline) printServiceDistribution(rawLogs []models.RawLog) {
	serviceDistribution := make(map[string]int)
	for _, log := range rawLogs {
		serviceName := log.Source.Fields.ServiceName
		if serviceName == "" {
			serviceName = "unknown"
		}
		serviceDistribution[serviceName]++
	}
	fmt.Println("   服務分佈：")
	for service, count := range serviceDistribution {
		fmt.Printf("   - %s: %d 條日誌\n", service, count)
	}
	fmt.Println()
}

// createAnalysesFromErrorGroups creates analysis results from error groups
func (p *Pipeline) createAnalysesFromErrorGroups(groups []models.ErrorGroup) []models.Analysis {
	var analyses []models.Analysis
	registry := config.GetRegistry()

	for _, group := range groups {
		severity := models.SeverityLow
		if group.TotalCount >= 50 {
			severity = models.SeverityHigh
		} else if group.TotalCount >= 10 {
			severity = models.SeverityMedium
		}

		// Try to match against known issues
		var isKnown bool
		var issueID string
		matchedIssue := registry.MatchContentAndService(group.NormalizedContent, group.ServiceName)
		if matchedIssue != nil {
			isKnown = true
			issueID = matchedIssue.ID
		}

		analysis := models.Analysis{
			ErrorGroupID: group.Fingerprint[:8],
			IsKnown:      isKnown,
			Severity:     severity,
			Reason:       fmt.Sprintf("錯誤在服務 %s 中發生了 %d 次", group.ServiceName, group.TotalCount),
			SuggestedActions: []string{
				fmt.Sprintf("調查錯誤模式：%s", truncateString(group.NormalizedContent, 60)),
				fmt.Sprintf("檢查來自調用者的日誌：%s", group.CallerFile),
				fmt.Sprintf("與部署或配置變更相關聯"),
			},
		}

		if isKnown {
			analysis.IssueID = issueID
		}

		analyses = append(analyses, analysis)
	}

	return analyses
}

// generatePerServiceReports generates reports for each service
func (p *Pipeline) generatePerServiceReports(analyses []models.Analysis, errorGroups []models.ErrorGroup,
	aggResult *interfaces.AggregationResult, result *PipelineResult) error {

	// Group analyses by service
	analysesByService := make(map[string][]models.Analysis)
	for _, analysis := range analyses {
		// Find the service for this analysis from errorGroups
		for _, group := range errorGroups {
			if group.Fingerprint[:8] == analysis.ErrorGroupID {
				analysesByService[group.ServiceName] = append(analysesByService[group.ServiceName], analysis)
				break
			}
		}
	}

	// Generate one report per service
	for service, serviceAnalyses := range analysesByService {
		report, err := p.reporter.GeneratePerService(serviceAnalyses, aggResult, service)
		if err != nil {
			fmt.Printf("❌ 無法生成 %s 的報告：%v\n", service, err)
			continue
		}
		fmt.Printf("✅ %s 報告已生成：%s\n", service, report.ReportPath)
		result.Reports[service] = report
	}

	return nil
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
