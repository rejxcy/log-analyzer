package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	"log-analyzer/internal/config"
	"log-analyzer/internal/interfaces"
	"log-analyzer/pkg/models"
)

// OpenSearchFetcher implements the Fetcher interface for OpenSearch
type OpenSearchFetcher struct {
	client       *opensearch.Client
	config       *config.OpenSearchConfig
	queryBuilder *QueryBuilder
}

// NewOpenSearchFetcher creates a new OpenSearch fetcher
func NewOpenSearchFetcher(cfg *config.OpenSearchConfig) (*OpenSearchFetcher, error) {
	// For OpenSearch Dashboards API, we don't use the standard client
	// Instead, we'll use HTTP client directly
	return &OpenSearchFetcher{
		client:       nil, // We'll use HTTP client directly
		config:       cfg,
		queryBuilder: NewQueryBuilder(),
	}, nil
}

// Fetch retrieves logs from OpenSearch based on the provided configuration
func (f *OpenSearchFetcher) Fetch(ctx context.Context, config interfaces.FetchConfig) ([]models.RawLog, error) {
	// Use the Dashboards API endpoint we discovered
	return f.fetchViaDashboardsAPI(ctx, config)
}

// fetchViaDashboardsAPI fetches data using the OpenSearch Dashboards internal API
func (f *OpenSearchFetcher) fetchViaDashboardsAPI(ctx context.Context, config interfaces.FetchConfig) ([]models.RawLog, error) {
	// Build the search query for Dashboards API
	searchQuery := f.buildDashboardsQuery(config)

	queryBytes, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Create HTTP request using the correct endpoint with long numerals support
	searchURL := f.config.URL + "/internal/search/opensearch-with-long-numerals"
	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, strings.NewReader(string(queryBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication and headers
	req.SetBasicAuth(f.config.Username, f.config.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("osd-xsrf", "osd-fetch")
	req.Header.Set("osd-version", "3.0.0")

	// Execute request
	client := &http.Client{Timeout: config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	return f.parseDashboardsResponse(resp.Body)
}

// buildDashboardsQuery builds a query for the Dashboards API
// This matches the format used by OpenSearch Dashboards UI exactly
func (f *OpenSearchFetcher) buildDashboardsQuery(config interfaces.FetchConfig) map[string]interface{} {
	// Build filter clauses (for range and other filters)
	filterClauses := []map[string]interface{}{}

	// Add keyword search as multi_match filter
	if len(config.Keywords) > 0 {
		for _, keyword := range config.Keywords {
			filterClauses = append(filterClauses, map[string]interface{}{
				"multi_match": map[string]interface{}{
					"type":    "phrase",
					"query":   keyword,
					"lenient": true,
				},
			})
		}
	}

	// Add time range filter
	filterClauses = append(filterClauses, map[string]interface{}{
		"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{
				"gte":    config.TimeRange.Start.Format(time.RFC3339Nano),
				"lte":    config.TimeRange.End.Format(time.RFC3339Nano),
				"format": "strict_date_optional_time",
			},
		},
	})

	// Add service filter if specified
	if len(config.Services) > 0 {
		if len(config.Services) == 1 {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{
					"fields.servicename": config.Services[0],
				},
			})
		} else {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{
					"fields.servicename": config.Services,
				},
			})
		}
	}

	// Build the complete query matching Dashboards API format
	query := map[string]interface{}{
		"params": map[string]interface{}{
			"index": config.Indices[0], // Use first index (can be extended for multiple)
			"body": map[string]interface{}{
				"sort": []map[string]interface{}{
					{
						"@timestamp": map[string]interface{}{
							"order":         "desc",
							"unmapped_type": "boolean",
						},
					},
				},
				"size":    500, // Match Dashboards API size
				"version": true,
				"aggs": map[string]interface{}{
					"2": map[string]interface{}{
						"date_histogram": map[string]interface{}{
							"field":          "@timestamp",
							"fixed_interval": "30m",
							"time_zone":      "Asia/Taipei",
							"min_doc_count":  1,
						},
					},
				},
				"stored_fields": []string{"*"},
				"script_fields": map[string]interface{}{},
				"docvalue_fields": []map[string]interface{}{
					{
						"field":  "@timestamp",
						"format": "date_time",
					},
				},
				"_source": map[string]interface{}{
					"excludes": []string{},
				},
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"must":     []map[string]interface{}{},
						"filter":   filterClauses,
						"should":   []map[string]interface{}{},
						"must_not": []map[string]interface{}{},
					},
				},
				"highlight": map[string]interface{}{
					"pre_tags":  []string{"@opensearch-dashboards-highlighted-field@"},
					"post_tags": []string{"@/opensearch-dashboards-highlighted-field@"},
					"fields": map[string]interface{}{
						"*": map[string]interface{}{},
					},
					"fragment_size": 2147483647,
				},
			},
		},
		"preference": int64(time.Now().UnixMilli()), // Random preference like Dashboards UI
	}

	return query
}

// fetchSingle performs a single search request
func (f *OpenSearchFetcher) fetchSingle(ctx context.Context, config interfaces.FetchConfig, query map[string]interface{}) ([]models.RawLog, error) {
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Create search request
	req := opensearchapi.SearchRequest{
		Index: config.Indices,
		Body:  strings.NewReader(string(queryBytes)),
	}

	// Execute search with timeout
	searchCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	res, err := req.Do(searchCtx, f.client)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search request returned error: %s", res.Status())
	}

	// Parse response
	return f.parseSearchResponse(res.Body)
}

// fetchConcurrent performs concurrent searches for multiple services
func (f *OpenSearchFetcher) fetchConcurrent(ctx context.Context, config interfaces.FetchConfig, baseQuery map[string]interface{}) ([]models.RawLog, error) {
	var wg sync.WaitGroup
	results := make(chan []models.RawLog, len(config.Services))
	errors := make(chan error, len(config.Services))

	// Launch concurrent searches for each service
	for _, service := range config.Services {
		wg.Add(1)
		go func(serviceName string) {
			defer wg.Done()

			// Create service-specific query
			serviceQuery := f.buildServiceQuery(baseQuery, serviceName)
			serviceConfig := config
			serviceConfig.Services = []string{serviceName}

			logs, err := f.fetchSingle(ctx, serviceConfig, serviceQuery)
			if err != nil {
				errors <- fmt.Errorf("failed to fetch logs for service %s: %w", serviceName, err)
				return
			}

			results <- logs
		}(service)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results
	var allLogs []models.RawLog
	var fetchErrors []error

	for {
		select {
		case logs, ok := <-results:
			if !ok {
				results = nil
			} else {
				allLogs = append(allLogs, logs...)
			}
		case err, ok := <-errors:
			if !ok {
				errors = nil
			} else {
				fetchErrors = append(fetchErrors, err)
			}
		}

		if results == nil && errors == nil {
			break
		}
	}

	// Return results even if some services failed (graceful degradation)
	if len(fetchErrors) > 0 && len(allLogs) == 0 {
		return nil, fmt.Errorf("all service fetches failed: %v", fetchErrors)
	}

	return allLogs, nil
}

// buildServiceQuery creates a query for a specific service
func (f *OpenSearchFetcher) buildServiceQuery(baseQuery map[string]interface{}, service string) map[string]interface{} {
	// Deep copy the base query
	queryBytes, _ := json.Marshal(baseQuery)
	var serviceQuery map[string]interface{}
	json.Unmarshal(queryBytes, &serviceQuery)

	// Update service filter
	serviceFilter := map[string]interface{}{
		"term": map[string]interface{}{
			"fields.servicename": service,
		},
	}

	// Replace or add service filter
	mustFilters := serviceQuery["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{})

	// Find and replace existing service filter, or add new one
	found := false
	for i, filter := range mustFilters {
		if _, hasTerms := filter["terms"]; hasTerms {
			if _, hasServicename := filter["terms"].(map[string]interface{})["fields.servicename"]; hasServicename {
				mustFilters[i] = serviceFilter
				found = true
				break
			}
		}
	}

	if !found {
		mustFilters = append(mustFilters, serviceFilter)
		serviceQuery["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustFilters
	}

	return serviceQuery
}

// parseSearchResponse parses the OpenSearch response and extracts raw logs
func (f *OpenSearchFetcher) parseSearchResponse(body interface{}) ([]models.RawLog, error) {
	// Parse the response body
	var response struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Index  string                  `json:"_index"`
				ID     string                  `json:"_id"`
				Source models.OpenSearchSource `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	// Read and parse JSON response
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// Convert to RawLog format
	var rawLogs []models.RawLog
	for _, hit := range response.Hits.Hits {
		rawLog := models.RawLog{
			Index:     hit.Index,
			ID:        hit.ID,
			Source:    hit.Source,
			Timestamp: hit.Source.Timestamp,
		}
		rawLogs = append(rawLogs, rawLog)
	}

	return rawLogs, nil
}

// TestConnection tests the connection to OpenSearch Dashboards
func (f *OpenSearchFetcher) TestConnection(ctx context.Context) error {
	// Test the Dashboards API endpoint
	testURL := f.config.URL + "/api/saved_objects/_find?type=index-pattern"

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.SetBasicAuth(f.config.Username, f.config.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("osd-xsrf", "true")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("connection test returned error: %s", resp.Status)
	}

	return nil
}

// parseDashboardsResponse parses the OpenSearch Dashboards API response
func (f *OpenSearchFetcher) parseDashboardsResponse(body io.ReadCloser) ([]models.RawLog, error) {
	// Read the response body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the Dashboards API response format
	var response struct {
		RawResponse struct {
			Hits struct {
				Total struct {
					Value int `json:"value"`
				} `json:"total"`
				Hits []struct {
					Index  string `json:"_index"`
					ID     string `json:"_id"`
					Source struct {
						Timestamp time.Time `json:"@timestamp"`
						Message   string    `json:"message"`
						Event     struct {
							Original string `json:"original"`
						} `json:"event"`
						Fields struct {
							ServiceName string `json:"servicename"`
						} `json:"fields"`
						Agent map[string]interface{} `json:"agent,omitempty"`
						Tags  []string               `json:"tags,omitempty"`
						Log   map[string]interface{} `json:"log,omitempty"`
						Host  map[string]interface{} `json:"host,omitempty"`
					} `json:"_source"`
				} `json:"hits"`
			} `json:"hits"`
		} `json:"rawResponse"`
	}

	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// Convert to RawLog format
	var rawLogs []models.RawLog
	for _, hit := range response.RawResponse.Hits.Hits {
		// Create OpenSearchSource based on actual structure
		source := models.OpenSearchSource{
			Message: hit.Source.Message,
			Event: models.EventData{
				Original: hit.Source.Event.Original,
			},
			Fields: models.FieldsData{
				ServiceName: hit.Source.Fields.ServiceName,
			},
			Agent:     hit.Source.Agent,
			Tags:      hit.Source.Tags,
			Log:       hit.Source.Log,
			Host:      hit.Source.Host,
			Timestamp: hit.Source.Timestamp,
		}

		rawLog := models.RawLog{
			Index:     hit.Index,
			ID:        hit.ID,
			Source:    source,
			Timestamp: hit.Source.Timestamp,
		}
		rawLogs = append(rawLogs, rawLog)
	}

	return rawLogs, nil
}
