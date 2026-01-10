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
	"path/filepath"
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
	// OpenSearch parameters
	fetchFromOpenSearch := flag.Bool("fetch", false, "Fetch logs from OpenSearch instead of using saved files")
	timeRange := flag.String("time", "1h", "Time range for OpenSearch query (e.g., '1h', '24h')")
	keyword := flag.String("keyword", "error", "Search keyword for OpenSearch query")
	indices := flag.String("indices", "pp-slot-api-log*", "OpenSearch indices to query (comma-separated)")
	windowSize := flag.String("window", "30m", "Time window size for fetching (default: 30 minutes)")

	// File parameters
	inputDir := flag.String("input", "./data/opensearch-responses", "Directory containing saved OpenSearch JSON files")
	outputDir := flag.String("output", "./reports", "Directory to save generated reports")
	flag.Parse()

	fmt.Println("🚀 啟動日誌分析管道（支援時間窗口）")
	fmt.Println()

	// Step 0: Fetch from OpenSearch if requested
	var rawLogs []models.RawLog
	var err error

	if *fetchFromOpenSearch {
		fmt.Printf("📡 第 0 步：從 OpenSearch 獲取日誌（過去 %s，窗口大小：%s）...\n", *timeRange, *windowSize)
		rawLogs, err = fetchFromOpenSearchWithWindows(*timeRange, *keyword, *indices, *windowSize)
		if err != nil {
			log.Fatalf("無法從 OpenSearch 獲取：%v", err)
		}
		fmt.Printf("✅ 從多個時間窗口成功獲取 %d 條日誌\n\n", len(rawLogs))

		// If fetch mode but no data, show warning and exit (don't use mock)
		if len(rawLogs) == 0 {
			fmt.Println("⚠️  指定時間範圍內 OpenSearch 中找不到日誌。")
			fmt.Println("   提示：嘗試更長的時間範圍或不同的搜尋關鍵字")
			fmt.Println("   範例：-time 48h -keyword warning")
			os.Exit(0)
		}
	} else {
		// Step 1: Load raw logs from JSON
		fmt.Println("📥 第 1 步：從 JSON 檔案加載原始日誌...")
		rawLogs, err = loadRawLogsFromJSON(*inputDir)
		if err != nil {
			log.Fatalf("無法加載原始日誌：%v", err)
		}
		fmt.Printf("✅ 成功加載 %d 條原始日誌\n\n", len(rawLogs))

		// If file mode and no data, offer to use mock for demonstration
		if len(rawLogs) == 0 {
			fmt.Println("⚠️  目錄中找不到日誌。建立示範用的模擬數據...")
			rawLogs = createMockData()
			fmt.Printf("✅ 已建立 %d 條模擬日誌用於測試\n\n", len(rawLogs))
		}
	}

	// Step 2: Preprocess logs
	fmt.Println("🔄 第 2 步：預處理日誌...")
	preprocessor := preprocessor.NewLogPreprocessor()
	parsedLogs, err := preprocessor.Process(rawLogs)
	if err != nil {
		log.Fatalf("無法預處理日誌：%v", err)
	}
	fmt.Printf("✅ 成功解析 %d 條日誌\n\n", len(parsedLogs))

	// Step 3: Normalize and group by fingerprint
	fmt.Println("🔐 第 3 步：正規化和分組錯誤...")
	norm := normalizer.NewLogNormalizer()
	errorGroups, err := norm.Normalize(parsedLogs)
	if err != nil {
		log.Fatalf("無法正規化日誌：%v", err)
	}
	normStats := normalizer.GetNormalizationStats(len(parsedLogs), errorGroups)
	fmt.Printf("✅ 分組為 %d 個唯一錯誤模式（%.1f%% 重複率）\n\n",
		len(errorGroups), normStats.DuplicationRate*100)

	// Step 4: Aggregate statistics
	fmt.Println("📊 第 4 步：聚合統計資訊...")
	agg := aggregator.NewLogAggregator()
	aggResult, err := agg.Aggregate(errorGroups)
	if err != nil {
		log.Fatalf("無法聚合：%v", err)
	}
	aggStats := aggregator.GetAggregationStats(aggResult)
	fmt.Printf("✅ 聚合完成：\n")
	fmt.Printf("   - 總錯誤數：%d\n", aggStats.TotalLogs)
	fmt.Printf("   - 服務總數：%d\n", aggStats.TotalServices)
	fmt.Printf("   - 峰值時段：%02d:00（%d 個錯誤）\n", aggStats.PeakHour, aggStats.PeakCount)
	fmt.Printf("   - 平均密度：%.2f 錯誤/分鐘\n\n", aggStats.AverageDensity)

	// Step 5: Generate analyses from actual error groups
	fmt.Println("🔍 第 5 步：分析錯誤模式...")
	analyses := createAnalysesFromErrorGroups(errorGroups)
	fmt.Printf("✅ 從實際數據建立了 %d 個分析結果\n\n", len(analyses))

	// Step 6: Generate report
	fmt.Println("📄 第 6 步：生成 Markdown 報告...")
	rep := reporter.NewMarkdownReporter(*outputDir)
	report, err := rep.Generate(analyses, aggResult)
	if err != nil {
		log.Fatalf("無法生成報告：%v", err)
	}
	fmt.Printf("✅ 報告已生成：%s\n\n", report.ReportPath)

	// Step 7: Save analysis JSON
	fmt.Println("💾 第 7 步：將分析結果保存為 JSON...")
	if err := reporter.SaveAnalysisJSON(analyses, aggResult, *outputDir); err != nil {
		log.Fatalf("無法保存分析 JSON：%v", err)
	}
	fmt.Println("✅ 分析 JSON 已保存")

	// Summary
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✨ 完整管道測試成功完成！")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\n📊 最終統計資訊：\n")
	fmt.Printf("   輸入日誌數：%d\n", len(rawLogs))
	fmt.Printf("   解析日誌數：%d\n", len(parsedLogs))
	fmt.Printf("   錯誤群組數：%d\n", len(errorGroups))
	fmt.Printf("   受影響服務數：%d\n", len(aggResult.ServiceStats))
	fmt.Printf("   處理時間：%dms\n\n", aggResult.ProcessingTime.Milliseconds())

	fmt.Printf("📁 輸出檔案：\n")
	fmt.Printf("   報告：%s\n", report.ReportPath)
	fmt.Printf("   分析 JSON：%s/analysis_*.json\n\n", *outputDir)

	fmt.Println("✅ 您現在可以查看生成的報告和分析 JSON 檔案！")
}

// fetchFromOpenSearchWithWindows fetches logs with time window splitting
func fetchFromOpenSearchWithWindows(timeRangeStr, keyword, indicesStr, windowSizeStr string) ([]models.RawLog, error) {
	// Parse time range and window size
	duration, err := time.ParseDuration(timeRangeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	windowDuration, err := time.ParseDuration(windowSizeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid window size: %w", err)
	}

	endTime := time.Now()

	// Calculate number of windows
	numWindows := int(duration / windowDuration)
	if numWindows == 0 {
		numWindows = 1
	}

	fmt.Printf("   📊 Fetching logs across %d time windows (%.0f minutes each)\n", numWindows, windowDuration.Minutes())
	fmt.Println()

	var allLogs []models.RawLog

	// Fetch data for each window
	for i := 0; i < numWindows; i++ {
		windowEnd := endTime.Add(-time.Duration(i) * windowDuration)
		windowStart := windowEnd.Add(-windowDuration)

		fmt.Printf("   🕐 Window %d/%d: %s to %s... ", i+1, numWindows,
			windowStart.Format("15:04:05"), windowEnd.Format("15:04:05"))

		logs, err := fetchFromOpenSearchDashboards(windowStart, windowEnd, keyword, indicesStr)
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("✅ %d logs\n", len(logs))
		allLogs = append(allLogs, logs...)
	}

	fmt.Println()
	return allLogs, nil
}

// fetchFromOpenSearchDashboards fetches logs from a specific time window
func fetchFromOpenSearchDashboards(startTime, endTime time.Time, keyword, indicesStr string) ([]models.RawLog, error) {
	// Load config
	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Build query
	query := buildDashboardsQuery(startTime, endTime, keyword)

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}

	indices := strings.Split(indicesStr, ",")
	if len(indices) == 0 {
		indices = []string{"pp-slot-api-log*"}
	}

	var allLogs []models.RawLog

	for _, index := range indices {
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
			return nil, fmt.Errorf("failed to fetch from OpenSearch: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("OpenSearch API returned %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
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

// loadRawLogsFromJSON loads raw logs from saved JSON files
func loadRawLogsFromJSON(inputDir string) ([]models.RawLog, error) {
	var allLogs []models.RawLog

	files, err := filepath.Glob(filepath.Join(inputDir, "all-documents_*.json"))
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("⚠️  Warning: Failed to read file %s: %v\n", file, err)
			continue
		}

		var response map[string]interface{}
		if err := json.Unmarshal(data, &response); err != nil {
			fmt.Printf("⚠️  Warning: Failed to unmarshal file %s: %v\n", file, err)
			continue
		}

		if docs, ok := response["documents"].([]interface{}); ok {
			for _, doc := range docs {
				docBytes, _ := json.Marshal(doc)
				var rawLog models.RawLog
				if err := json.Unmarshal(docBytes, &rawLog); err != nil {
					continue
				}
				allLogs = append(allLogs, rawLog)
			}
		}
	}

	return allLogs, nil
}

// createMockData creates mock log data for testing
func createMockData() []models.RawLog {
	now := time.Now()

	t1 := now.Add(-3 * time.Hour).Truncate(time.Hour).Add(30*time.Minute + 45*time.Second)

	mockLogs := []models.RawLog{
		{
			Index: "pp-slot-api-log*",
			ID:    "mock-1",
			Source: models.OpenSearchSource{
				Message: fmt.Sprintf(`%s stderr F {"@timestamp":"%s","caller":"api/handler.go:123","content":"Connection timeout","level":"error","span":"span-123","trace":"trace-456","servicename":"pp-slot-api"}`,
					t1.Format("2006-01-02T15:04:05.000Z"), t1.Format(time.RFC3339)),
				Fields: models.FieldsData{
					ServiceName: "pp-slot-api",
				},
				Timestamp: t1,
			},
		},
	}

	return mockLogs
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
