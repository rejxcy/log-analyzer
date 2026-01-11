package fetcher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log-analyzer/internal/config"
	"log-analyzer/pkg/models"
)

// Fetcher fetches logs from OpenSearch
type Fetcher struct {
	config *config.Config
}

// NewFetcher creates a new fetcher
func NewFetcher(cfg *config.Config) *Fetcher {
	return &Fetcher{
		config: cfg,
	}
}

// FetchWithTimeWindows fetches logs with time window splitting to avoid 500-hit limit
func (f *Fetcher) FetchWithTimeWindows(timeRangeStr string) ([]models.RawLog, error) {
	// Parse time range and window size
	duration, err := time.ParseDuration(timeRangeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	windowDuration, err := time.ParseDuration("30m") // Fixed window size
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

		logs, err := f.fetchFromWindow(windowStart, windowEnd)
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

// fetchFromWindow fetches logs from a specific time window
func (f *Fetcher) fetchFromWindow(startTime, endTime time.Time) ([]models.RawLog, error) {
	// Build query
	query := f.buildDashboardsQuery(startTime, endTime)

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}

	var allLogs []models.RawLog

	for _, index := range f.config.OpenSearch.Indices {
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

		url := fmt.Sprintf("%s/internal/search/opensearch-with-long-numerals", f.config.OpenSearch.URL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Basic "+basicAuth(f.config.OpenSearch.Username, f.config.OpenSearch.Password))
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
func (f *Fetcher) buildDashboardsQuery(startTime, endTime time.Time) map[string]interface{} {
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
							"query":   f.config.Query.Keyword,
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
