package fetcher

import (
	"fmt"
	"testing"
	"time"

	"log-analyzer/internal/config"
	"log-analyzer/internal/interfaces"
	"log-analyzer/pkg/models"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestNewOpenSearchFetcher(t *testing.T) {
	cfg := &config.OpenSearchConfig{
		URL:      "https://test.example.com:9200",
		Username: "testuser",
		Password: "testpass",
		Indices:  []string{"test-log*"},
	}

	fetcher, err := NewOpenSearchFetcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create fetcher: %v", err)
	}

	if fetcher == nil {
		t.Fatal("Fetcher is nil")
	}

	if fetcher.client == nil {
		t.Fatal("Client is nil")
	}

	if fetcher.config != cfg {
		t.Fatal("Config not set correctly")
	}
}

func TestBuildQuery(t *testing.T) {
	cfg := &config.OpenSearchConfig{
		URL:     "https://test.example.com:9200",
		Indices: []string{"test-log*"},
	}

	fetcher, err := NewOpenSearchFetcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create fetcher: %v", err)
	}

	// Test basic query
	fetchConfig := interfaces.FetchConfig{
		TimeRange: models.TimeRange{
			Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
		},
		Services:   []string{"service1", "service2"},
		Indices:    []string{"test-log*"},
		MaxResults: 1000,
		Timeout:    30 * time.Second,
	}

	query := fetcher.buildDashboardsQuery(fetchConfig)

	// Verify query structure
	if query["size"] != 1000 {
		t.Errorf("Expected size 1000, got %v", query["size"])
	}

	// Verify time range
	queryMap := query["query"].(map[string]interface{})
	boolMap := queryMap["bool"].(map[string]interface{})
	mustArray := boolMap["must"].([]map[string]interface{})

	// Should have time range and service filter
	if len(mustArray) != 2 {
		t.Errorf("Expected 2 must clauses, got %d", len(mustArray))
	}

	// Check for range query
	hasRange := false
	hasTerms := false
	for _, clause := range mustArray {
		if _, ok := clause["range"]; ok {
			hasRange = true
		}
		if _, ok := clause["terms"]; ok {
			hasTerms = true
		}
	}

	if !hasRange {
		t.Error("Missing range query for timestamp")
	}
	if !hasTerms {
		t.Error("Missing terms query for services")
	}
}

func TestBuildServiceQuery(t *testing.T) {
	cfg := &config.OpenSearchConfig{
		URL:     "https://test.example.com:9200",
		Indices: []string{"test-log*"},
	}

	fetcher, err := NewOpenSearchFetcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create fetcher: %v", err)
	}

	baseQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte": "2026-01-10T00:00:00Z",
								"lte": "2026-01-10T23:59:59Z",
							},
						},
					},
				},
			},
		},
	}

	serviceQuery := fetcher.buildServiceQuery(baseQuery, "test-service")

	// Verify service filter was added
	queryMap := serviceQuery["query"].(map[string]interface{})
	boolMap := queryMap["bool"].(map[string]interface{})
	mustArray := boolMap["must"].([]map[string]interface{})

	// Should have range and service term
	if len(mustArray) != 2 {
		t.Errorf("Expected 2 must clauses, got %d", len(mustArray))
	}

	// Check for service term
	hasServiceTerm := false
	for _, clause := range mustArray {
		if termMap, ok := clause["term"]; ok {
			if serviceMap, ok := termMap.(map[string]interface{}); ok {
				if service, ok := serviceMap["fields.servicename"]; ok && service == "test-service" {
					hasServiceTerm = true
					break
				}
			}
		}
	}

	if !hasServiceTerm {
		t.Error("Missing service term filter")
	}
}

// **Feature: log-analyzer, Property 1: Time range query consistency**
// Property 1: Time range query consistency
// For any valid time range configuration, the OpenSearch query parameters should correctly
// reflect the specified 24-hour period and all returned logs should fall within that timeframe.
func TestTimeRangeQueryConsistency(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("time range should be correctly reflected in query", prop.ForAll(
		func(startHour, endHour int) bool {
			// Ensure valid hour range
			if startHour < 0 || startHour > 23 || endHour < 0 || endHour > 23 || startHour >= endHour {
				return true // Skip invalid ranges
			}

			// Create test time range
			baseDate := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
			timeRange := models.TimeRange{
				Start: baseDate.Add(time.Duration(startHour) * time.Hour),
				End:   baseDate.Add(time.Duration(endHour) * time.Hour),
			}

			// Create fetch config
			fetchConfig := interfaces.FetchConfig{
				TimeRange:  timeRange,
				Services:   []string{"test-service"},
				Indices:    []string{"test-log*"},
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			// Build query using query builder
			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Extract time range from query
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			// Find range clause
			var rangeClause map[string]interface{}
			for _, clause := range mustArray {
				if rangeMap, ok := clause["range"]; ok {
					rangeClause = rangeMap.(map[string]interface{})
					break
				}
			}

			if rangeClause == nil {
				return false // No range clause found
			}

			// Verify timestamp range
			timestampRange, ok := rangeClause["@timestamp"].(map[string]interface{})
			if !ok {
				return false
			}

			gteStr, ok := timestampRange["gte"].(string)
			if !ok {
				return false
			}

			lteStr, ok := timestampRange["lte"].(string)
			if !ok {
				return false
			}

			// Parse and verify times
			gte, err := time.Parse(time.RFC3339, gteStr)
			if err != nil {
				return false
			}

			lte, err := time.Parse(time.RFC3339, lteStr)
			if err != nil {
				return false
			}

			// Verify the query time range matches the input
			return gte.Equal(timeRange.Start) && lte.Equal(timeRange.End)
		},
		gen.IntRange(0, 22), // Start hour
		gen.IntRange(1, 23), // End hour
	))

	properties.Property("query should contain all required time range components", prop.ForAll(
		func(dayOffset int) bool {
			// Generate different days to test
			if dayOffset < 0 || dayOffset > 30 {
				return true // Skip invalid offsets
			}

			baseDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			startTime := baseDate.AddDate(0, 0, dayOffset)
			endTime := startTime.Add(24 * time.Hour)

			timeRange := models.TimeRange{
				Start: startTime,
				End:   endTime,
			}

			fetchConfig := interfaces.FetchConfig{
				TimeRange:  timeRange,
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Verify query structure
			if _, ok := query["query"]; !ok {
				return false
			}

			if _, ok := query["size"]; !ok {
				return false
			}

			if _, ok := query["sort"]; !ok {
				return false
			}

			// Verify the time range is exactly 24 hours
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			for _, clause := range mustArray {
				if rangeMap, ok := clause["range"]; ok {
					timestampRange := rangeMap.(map[string]interface{})["@timestamp"].(map[string]interface{})
					gteStr := timestampRange["gte"].(string)
					lteStr := timestampRange["lte"].(string)

					gte, _ := time.Parse(time.RFC3339, gteStr)
					lte, _ := time.Parse(time.RFC3339, lteStr)

					// Verify it's a 24-hour period
					duration := lte.Sub(gte)
					return duration == 24*time.Hour
				}
			}

			return false
		},
		gen.IntRange(0, 30),
	))

	properties.TestingRun(t)
}

// **Feature: log-analyzer, Property 2: Concurrent service query completeness**
// Property 2: Concurrent service query completeness
// For any list of configured services, the system should successfully query all services
// and aggregate results from each service without data loss.
func TestConcurrentServiceQueryCompleteness(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("query should handle multiple services correctly", prop.ForAll(
		func(serviceCount int) bool {
			// Limit service count to reasonable range
			if serviceCount < 1 || serviceCount > 10 {
				return true // Skip invalid counts
			}

			// Generate service names
			services := make([]string, serviceCount)
			for i := 0; i < serviceCount; i++ {
				services[i] = fmt.Sprintf("service-%d", i)
			}

			// Create fetch config
			fetchConfig := interfaces.FetchConfig{
				TimeRange: models.TimeRange{
					Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
				},
				Services:   services,
				Indices:    []string{"test-log*"},
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			// Build query using query builder
			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Extract service filter from query
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			// Find service filter clause
			var serviceClause map[string]interface{}
			for _, clause := range mustArray {
				if termsMap, ok := clause["terms"]; ok {
					if servicenameTerms, ok := termsMap.(map[string]interface{})["fields.servicename"]; ok {
						serviceClause = clause
						_ = servicenameTerms // Use the variable to avoid unused error
						break
					}
				} else if termMap, ok := clause["term"]; ok {
					if _, ok := termMap.(map[string]interface{})["fields.servicename"]; ok {
						serviceClause = clause
						break
					}
				}
			}

			if serviceClause == nil {
				return false // No service clause found
			}

			// Verify service filter contains all services
			if serviceCount == 1 {
				// Single service uses "term" query
				termMap := serviceClause["term"].(map[string]interface{})
				serviceName := termMap["fields.servicename"].(string)
				return serviceName == services[0]
			} else {
				// Multiple services use "terms" query
				termsMap := serviceClause["terms"].(map[string]interface{})
				serviceList := termsMap["fields.servicename"].([]string)

				// Verify all services are included
				if len(serviceList) != len(services) {
					return false
				}

				serviceSet := make(map[string]bool)
				for _, service := range serviceList {
					serviceSet[service] = true
				}

				for _, expectedService := range services {
					if !serviceSet[expectedService] {
						return false
					}
				}

				return true
			}
		},
		gen.IntRange(1, 10),
	))

	properties.Property("service-specific queries should be generated correctly", prop.ForAll(
		func(serviceName string) bool {
			if len(serviceName) == 0 {
				return true // Skip empty service names
			}

			// Create base config
			baseConfig := interfaces.FetchConfig{
				TimeRange: models.TimeRange{
					Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
				},
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			// Build service-specific query
			qb := NewQueryBuilder()
			query := qb.BuildServiceSpecificQuery(baseConfig, serviceName)

			// Verify the query contains the specific service
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			// Find service filter
			for _, clause := range mustArray {
				if termMap, ok := clause["term"]; ok {
					if serviceField, ok := termMap.(map[string]interface{})["fields.servicename"]; ok {
						return serviceField.(string) == serviceName
					}
				}
			}

			return false
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
	))

	properties.Property("concurrent query structure should be consistent", prop.ForAll(
		func(services []string) bool {
			// Filter out empty services and limit count
			validServices := []string{}
			for _, service := range services {
				if len(service) > 0 && len(service) < 50 {
					validServices = append(validServices, service)
				}
				if len(validServices) >= 5 { // Limit to 5 services for testing
					break
				}
			}

			if len(validServices) == 0 {
				return true // Skip empty service lists
			}

			// Create fetch config
			fetchConfig := interfaces.FetchConfig{
				TimeRange: models.TimeRange{
					Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
				},
				Services:   validServices,
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			qb := NewQueryBuilder()

			// Test that individual service queries are consistent with multi-service query
			multiQuery := qb.BuildSearchQuery(fetchConfig)

			// Verify multi-service query structure
			if _, ok := multiQuery["query"]; !ok {
				return false
			}
			if _, ok := multiQuery["size"]; !ok {
				return false
			}
			if _, ok := multiQuery["sort"]; !ok {
				return false
			}

			// Test individual service queries
			for _, service := range validServices {
				singleQuery := qb.BuildServiceSpecificQuery(fetchConfig, service)

				// Verify single service query structure
				if _, ok := singleQuery["query"]; !ok {
					return false
				}
				if _, ok := singleQuery["size"]; !ok {
					return false
				}
				if _, ok := singleQuery["sort"]; !ok {
					return false
				}
			}

			return true
		},
		gen.SliceOf(gen.AlphaString()),
	))

	properties.TestingRun(t)
}

// **Feature: log-analyzer, Property 3: Error handling resilience**
// Property 3: Error handling resilience
// For any combination of successful and failed OpenSearch responses, the system should
// continue processing available data and provide meaningful error information for failures.
func TestErrorHandlingResilience(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("invalid configuration should be handled gracefully", prop.ForAll(
		func(url, username, password string) bool {
			// Test with potentially invalid configurations
			cfg := &config.OpenSearchConfig{
				URL:      url,
				Username: username,
				Password: password,
				Indices:  []string{"test-log*"},
			}

			// Creating the fetcher should not panic, even with invalid config
			fetcher, err := NewOpenSearchFetcher(cfg)

			// If URL is empty, we expect an error or the creation to succeed but fail later
			if url == "" {
				// Empty URL might be handled differently, but shouldn't panic
				return true
			}

			// If fetcher creation succeeds, it should have proper structure
			if err == nil && fetcher != nil {
				return fetcher.client != nil && fetcher.config == cfg && fetcher.queryBuilder != nil
			}

			// If there's an error, it should be meaningful (not nil)
			return err != nil
		},
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.Property("query validation should catch malformed queries", prop.ForAll(
		func(maxResults int, timeoutSecs int) bool {
			// Test with various parameter ranges
			if maxResults < -1000 || maxResults > 100000 || timeoutSecs < -100 || timeoutSecs > 3600 {
				return true // Skip extreme values
			}

			// Create fetch config with potentially problematic values
			fetchConfig := interfaces.FetchConfig{
				TimeRange: models.TimeRange{
					Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
				},
				Services:   []string{"test-service"},
				Keywords:   []string{"error"},
				MaxResults: maxResults,
				Timeout:    time.Duration(timeoutSecs) * time.Second,
			}

			// Build query - should handle edge cases gracefully
			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Query should always have basic structure
			if _, ok := query["query"]; !ok {
				return false
			}

			// Size should be set (even if input was invalid)
			if _, ok := query["size"]; !ok {
				return false
			}

			// Validate the query structure
			err := qb.ValidateQuery(query)
			return err == nil // Query should be valid after building
		},
		gen.IntRange(-100, 50000),
		gen.IntRange(-10, 300),
	))

	properties.Property("empty or invalid service lists should be handled", prop.ForAll(
		func(services []string) bool {
			// Filter services to create various edge cases
			var testServices []string
			for _, service := range services {
				// Include various edge cases
				if len(service) <= 100 { // Reasonable length limit
					testServices = append(testServices, service)
				}
				if len(testServices) >= 20 { // Limit for testing
					break
				}
			}

			fetchConfig := interfaces.FetchConfig{
				TimeRange: models.TimeRange{
					Start: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2026, 1, 10, 23, 59, 59, 0, time.UTC),
				},
				Services:   testServices,
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Query should be buildable regardless of service list
			if _, ok := query["query"]; !ok {
				return false
			}

			// Verify query structure
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			// Should have at least time range and keyword filters
			hasTimeRange := false
			hasKeyword := false
			hasService := false

			for _, clause := range mustArray {
				if _, ok := clause["range"]; ok {
					hasTimeRange = true
				}
				if _, ok := clause["match"]; ok {
					hasKeyword = true
				}
				if _, ok := clause["terms"]; ok {
					hasService = true
				}
				if _, ok := clause["term"]; ok {
					hasService = true
				}
			}

			// Time range and keyword should always be present
			if !hasTimeRange || !hasKeyword {
				return false
			}

			// Service filter should be present only if services were provided
			if len(testServices) > 0 {
				return hasService
			} else {
				return !hasService // No service filter if no services
			}
		},
		gen.SliceOf(gen.AlphaString()),
	))

	properties.Property("time range edge cases should be handled", prop.ForAll(
		func(startYear, endYear int, startMonth, endMonth int) bool {
			// Limit to reasonable year range
			if startYear < 2020 || startYear > 2030 || endYear < 2020 || endYear > 2030 {
				return true
			}
			if startMonth < 1 || startMonth > 12 || endMonth < 1 || endMonth > 12 {
				return true
			}

			// Create potentially problematic time ranges
			startTime := time.Date(startYear, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
			endTime := time.Date(endYear, time.Month(endMonth), 1, 0, 0, 0, 0, time.UTC)

			// Skip invalid ranges where start > end
			if startTime.After(endTime) {
				return true
			}

			timeRange := models.TimeRange{
				Start: startTime,
				End:   endTime,
			}

			fetchConfig := interfaces.FetchConfig{
				TimeRange:  timeRange,
				Keywords:   []string{"error"},
				MaxResults: 1000,
				Timeout:    30 * time.Second,
			}

			qb := NewQueryBuilder()
			query := qb.BuildSearchQuery(fetchConfig)

			// Should be able to build query with any valid time range
			queryMap := query["query"].(map[string]interface{})
			boolMap := queryMap["bool"].(map[string]interface{})
			mustArray := boolMap["must"].([]map[string]interface{})

			// Find and verify time range
			for _, clause := range mustArray {
				if rangeMap, ok := clause["range"]; ok {
					timestampRange := rangeMap.(map[string]interface{})["@timestamp"].(map[string]interface{})
					gteStr := timestampRange["gte"].(string)
					lteStr := timestampRange["lte"].(string)

					// Should be able to parse the generated time strings
					gte, err1 := time.Parse(time.RFC3339, gteStr)
					lte, err2 := time.Parse(time.RFC3339, lteStr)

					if err1 != nil || err2 != nil {
						return false
					}

					// Verify the times match our input
					return gte.Equal(startTime) && lte.Equal(endTime)
				}
			}

			return false
		},
		gen.IntRange(2020, 2030),
		gen.IntRange(2020, 2030),
		gen.IntRange(1, 12),
		gen.IntRange(1, 12),
	))

	properties.TestingRun(t)
}
