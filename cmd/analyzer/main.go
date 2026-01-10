package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"log-analyzer/internal/aggregator"
	"log-analyzer/internal/config"
	"log-analyzer/internal/normalizer"
	"log-analyzer/internal/preprocessor"
	"log-analyzer/internal/reporter"
	"log-analyzer/pkg/models"
)

func main() {
	// Only one parameter: time range
	timeRange := flag.String("time", "24h", "Time range for OpenSearch query (e.g., '1h', '24h', '7d')")
	flag.Parse()

	fmt.Println("🚀 啟動日誌分析管道")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		log.Fatalf("❌ 無法加載配置：%v", err)
	}

	// Step 0: Fetch from OpenSearch with time windows
	fmt.Printf("📡 第 0 步：從 OpenSearch 獲取日誌（過去 %s）...\n", *timeRange)
	rawLogs, err := fetchFromOpenSearchWithWindows(cfg, *timeRange)
	if err != nil {
		log.Fatalf("❌ 無法從 OpenSearch 獲取：%v", err)
	}

	if len(rawLogs) == 0 {
		fmt.Println("⚠️  指定時間範圍內找不到日誌。")
		fmt.Println("   提示：嘗試更長的時間範圍（例如：-time 48h）")
		os.Exit(0)
	}

	fmt.Printf("✅ 成功獲取 %d 條原始日誌\n", len(rawLogs))

	// Show service distribution from raw logs
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

	// Step 1: Preprocess logs
	fmt.Println("🔄 第 1 步：預處理日誌...")
	prep := preprocessor.NewLogPreprocessor()
	parsedLogs, err := prep.Process(rawLogs)
	if err != nil {
		log.Fatalf("❌ 無法預處理日誌：%v", err)
	}
	fmt.Printf("✅ 成功解析 %d 條日誌\n\n", len(parsedLogs))

	// Step 2: Normalize and group by fingerprint
	fmt.Println("🔐 第 2 步：正規化和分組錯誤...")
	norm := normalizer.NewLogNormalizer()
	errorGroups, err := norm.Normalize(parsedLogs)
	if err != nil {
		log.Fatalf("❌ 無法正規化日誌：%v", err)
	}
	normStats := normalizer.GetNormalizationStats(len(parsedLogs), errorGroups)
	fmt.Printf("✅ 分組為 %d 個唯一錯誤模式（%.1f%% 重複率）\n\n",
		len(errorGroups), normStats.DuplicationRate*100)

	// Step 3: Aggregate statistics
	fmt.Println("📊 第 3 步：聚合統計資訊...")
	agg := aggregator.NewLogAggregator()
	aggResult, err := agg.Aggregate(errorGroups)
	if err != nil {
		log.Fatalf("❌ 無法聚合：%v", err)
	}
	aggStats := aggregator.GetAggregationStats(aggResult)
	fmt.Printf("✅ 聚合完成：\n")
	fmt.Printf("   - 總錯誤數：%d\n", aggStats.TotalLogs)
	fmt.Printf("   - 服務總數：%d\n", aggStats.TotalServices)
	fmt.Printf("   - 峰值時段：%02d:00（%d 個錯誤）\n", aggStats.PeakHour, aggStats.PeakCount)
	fmt.Printf("   - 平均密度：%.2f 錯誤/分鐘\n\n", aggStats.AverageDensity)

	// Step 4: Generate analyses from actual error groups
	fmt.Println("🔍 第 4 步：分析錯誤模式...")
	analyses := createAnalysesFromErrorGroups(errorGroups)
	fmt.Printf("✅ 從實際數據建立了 %d 個分析結果\n\n", len(analyses))

	// Step 5: Generate reports (one per service)
	fmt.Println("📄 第 5 步：為每個服務生成 Markdown 報告...")

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

	rep := reporter.NewMarkdownReporter(cfg.Output.ReportDir)

	// Generate one report per service
	for service, serviceAnalyses := range analysesByService {
		report, err := rep.GeneratePerService(serviceAnalyses, aggResult, service)
		if err != nil {
			fmt.Printf("❌ 無法生成 %s 的報告：%v\n", service, err)
			continue
		}
		fmt.Printf("✅ %s 報告已生成：%s\n", service, report.ReportPath)
	}
	fmt.Println()

	// Step 6: Save analysis JSON
	fmt.Println("💾 第 6 步：將分析結果保存為 JSON...")
	if err := reporter.SaveAnalysisJSON(analyses, aggResult, cfg.Output.ReportDir); err != nil {
		log.Fatalf("❌ 無法保存分析 JSON：%v", err)
	}
	fmt.Println("✅ 分析 JSON 已保存")

	// Summary
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✨ 完整管道分析成功完成！")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\n📊 最終統計資訊：\n")
	fmt.Printf("   輸入日誌數：%d\n", len(rawLogs))
	fmt.Printf("   解析日誌數：%d\n", len(parsedLogs))
	fmt.Printf("   錯誤群組數：%d\n", len(errorGroups))
	fmt.Printf("   受影響服務數：%d\n", len(aggResult.ServiceStats))
	fmt.Printf("   處理時間：%dms\n\n", aggResult.ProcessingTime.Milliseconds())

	fmt.Printf("📁 輸出檔案：\n")
	fmt.Printf("   報告目錄：%s\n", cfg.Output.ReportDir)
	fmt.Printf("   分析 JSON：%s/analysis_*.json\n\n", cfg.Output.ReportDir)

	fmt.Println("✅ 您現在可以查看生成的報告和分析 JSON 檔案！")
}

// fetchFromOpenSearchWithWindows fetches logs with time window splitting
func fetchFromOpenSearchWithWindows(cfg *config.Config, timeRangeStr string) ([]models.RawLog, error) {
	// Parse time range and window size
	duration, err := time.ParseDuration(timeRangeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	windowDuration, err := time.ParseDuration("30m") // Fixed window size from config
	if err != nil {
		return nil, fmt.Errorf("invalid window size: %w", err)
	}

	endTime := time.Now()

	// Calculate number of windows
	numWindows := int(duration / windowDuration)
	if numWindows == 0 {
		numWindows = 1
	}

	fmt.Printf("   📊 跨 %d 個時間窗口獲取日誌（每個 %.0f 分鐘）\n", numWindows, windowDuration.Minutes())
	fmt.Println()

	var allLogs []models.RawLog

	// Fetch data for each window
	for i := 0; i < numWindows; i++ {
		windowEnd := endTime.Add(-time.Duration(i) * windowDuration)
		windowStart := windowEnd.Add(-windowDuration)

		fmt.Printf("   🕐 窗口 %d/%d：%s 到 %s\n", i+1, numWindows,
			windowStart.Format("15:04:05"), windowEnd.Format("15:04:05"))

		logs, err := fetchFromOpenSearchDashboards(cfg, windowStart, windowEnd)
		if err != nil {
			fmt.Printf("      ❌ 錯誤：%v\n", err)
			continue
		}

		fmt.Printf("      ✅ 共 %d 條日誌\n", len(logs))
		allLogs = append(allLogs, logs...)
	}

	fmt.Println()
	return allLogs, nil
}

// fetchFromOpenSearchDashboards fetches logs from a specific time window
func fetchFromOpenSearchDashboards(cfg *config.Config, startTime, endTime time.Time) ([]models.RawLog, error) {
	// Build query
	query := buildDashboardsQuery(startTime, endTime, cfg.Query.Keyword)

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}

	var allLogs []models.RawLog

	for _, index := range cfg.OpenSearch.Indices {
		index = strings.TrimSpace(index)

		body := map[string]interface{}{
			"params": map[string]interface{}{
				"index": index,
				"body":  query,
			},
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		url := fmt.Sprintf("%s/internal/search/opensearch-with-long-numerals", cfg.OpenSearch.URL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+basicAuth(cfg.OpenSearch.Username, cfg.OpenSearch.Password))
		req.Header.Set("osd-xsrf", "osd-fetch")
		req.Header.Set("osd-version", "3.0.0")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("         [%s] ❌ 連接失敗：%v\n", index, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			fmt.Printf("         [%s] ❌ API 返回 %d：%s\n", index, resp.StatusCode, string(bodyBytes))
			continue
		}

		// Parse response
		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			fmt.Printf("         [%s] ❌ 解析響應失敗：%v\n", index, err)
			continue
		}

		// Extract hits
		var hitsArray []interface{}
		if rawResp, ok := response["rawResponse"].(map[string]interface{}); ok {
			if hits, ok := rawResp["hits"].(map[string]interface{}); ok {
				if hits, ok := hits["hits"].([]interface{}); ok {
					hitsArray = hits
				}
			}
		}

		// Process hits
		logsFromIndex := 0
		for _, hit := range hitsArray {
			hitMap := hit.(map[string]interface{})

			source, ok := hitMap["_source"].(map[string]interface{})
			if !ok {
				continue
			}

			sourceBytes, _ := json.Marshal(source)
			var openSearchSource models.OpenSearchSource
			if err := json.Unmarshal(sourceBytes, &openSearchSource); err != nil {
				continue
			}

			rawLog := models.RawLog{
				Index:  index,
				ID:     hitMap["_id"].(string),
				Source: openSearchSource,
			}

			allLogs = append(allLogs, rawLog)
			logsFromIndex++
		}

		if logsFromIndex > 0 {
			fmt.Printf("         [%s] ✅ %d 條\n", index, logsFromIndex)
		}
	}

	return allLogs, nil
}

// buildDashboardsQuery builds a query for specific time window
func buildDashboardsQuery(startTime, endTime time.Time, keyword string) map[string]interface{} {
	return map[string]interface{}{
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order":         "desc",
					"unmapped_type": "boolean",
				},
			},
		},
		"size": 500,
		"_source": map[string]interface{}{
			"excludes": []string{},
		},
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{},
				"filter": []interface{}{
					map[string]interface{}{
						"multi_match": map[string]interface{}{
							"type":    "phrase",
							"query":   keyword,
							"lenient": true,
						},
					},
					map[string]interface{}{
						"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte":    startTime.Format(time.RFC3339),
								"lte":    endTime.Format(time.RFC3339),
								"format": "strict_date_optional_time",
							},
						},
					},
				},
				"should":   []interface{}{},
				"must_not": []interface{}{},
			},
		},
	}
}

// basicAuth creates a basic auth header value
func basicAuth(username, password string) string {
	credentials := fmt.Sprintf("%s:%s", username, password)
	return base64.StdEncoding.EncodeToString([]byte(credentials))
}

// createAnalysesFromErrorGroups creates analysis results from actual error groups
func createAnalysesFromErrorGroups(groups []models.ErrorGroup) []models.Analysis {
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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
