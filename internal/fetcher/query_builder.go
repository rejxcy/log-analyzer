package fetcher

import (
	"fmt"
	"time"

	"log-analyzer/internal/interfaces"
	"log-analyzer/pkg/models"
)

// QueryBuilder helps construct OpenSearch queries
type QueryBuilder struct{}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// BuildSearchQuery constructs a complete search query
func (qb *QueryBuilder) BuildSearchQuery(config interfaces.FetchConfig) map[string]interface{} {
	query := map[string]interface{}{
		"query": qb.buildBoolQuery(config),
		"size":  config.MaxResults,
		"sort":  qb.buildSortClause(),
	}

	return query
}

// buildBoolQuery constructs the bool query with must clauses
func (qb *QueryBuilder) buildBoolQuery(config interfaces.FetchConfig) map[string]interface{} {
	mustClauses := []map[string]interface{}{}

	// Add time range filter
	mustClauses = append(mustClauses, qb.buildTimeRangeFilter(config.TimeRange))

	// Add service filter if specified
	if len(config.Services) > 0 {
		mustClauses = append(mustClauses, qb.buildServiceFilter(config.Services))
	}

	// Add keyword search filter if specified
	if len(config.Keywords) > 0 {
		mustClauses = append(mustClauses, qb.buildKeywordFilter(config.Keywords))
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"must": mustClauses,
		},
	}
}

// buildTimeRangeFilter creates a time range filter
func (qb *QueryBuilder) buildTimeRangeFilter(timeRange models.TimeRange) map[string]interface{} {
	return map[string]interface{}{
		"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{
				"gte": timeRange.Start.Format(time.RFC3339),
				"lte": timeRange.End.Format(time.RFC3339),
			},
		},
	}
}

// buildServiceFilter creates a service filter
func (qb *QueryBuilder) buildServiceFilter(services []string) map[string]interface{} {
	if len(services) == 1 {
		// Use term query for single service
		return map[string]interface{}{
			"term": map[string]interface{}{
				"fields.servicename": services[0],
			},
		}
	}

	// Use terms query for multiple services
	return map[string]interface{}{
		"terms": map[string]interface{}{
			"fields.servicename": services,
		},
	}
}

// buildKeywordFilter creates a filter to search for keywords in the message field
func (qb *QueryBuilder) buildKeywordFilter(keywords []string) map[string]interface{} {
	if len(keywords) == 1 {
		// Single keyword - use match query
		return map[string]interface{}{
			"match": map[string]interface{}{
				"message": keywords[0],
			},
		}
	}

	// Multiple keywords - use bool should query (OR logic)
	shouldClauses := []map[string]interface{}{}
	for _, keyword := range keywords {
		shouldClauses = append(shouldClauses, map[string]interface{}{
			"match": map[string]interface{}{
				"message": keyword,
			},
		})
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"should":               shouldClauses,
			"minimum_should_match": 1,
		},
	}
}

// buildSortClause creates the sort clause
func (qb *QueryBuilder) buildSortClause() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"@timestamp": map[string]interface{}{
				"order": "desc",
			},
		},
	}
}

// BuildServiceSpecificQuery creates a query for a specific service
func (qb *QueryBuilder) BuildServiceSpecificQuery(baseConfig interfaces.FetchConfig, service string) map[string]interface{} {
	// Create a new config with single service
	serviceConfig := baseConfig
	serviceConfig.Services = []string{service}

	return qb.BuildSearchQuery(serviceConfig)
}

// AddLogLevelFilter adds log level filtering to an existing query
func (qb *QueryBuilder) AddLogLevelFilter(query map[string]interface{}, levels []string) map[string]interface{} {
	if len(levels) == 0 {
		return query
	}

	// Extract existing must clauses
	queryMap := query["query"].(map[string]interface{})
	boolMap := queryMap["bool"].(map[string]interface{})
	mustClauses := boolMap["must"].([]map[string]interface{})

	// Add level filter
	levelFilter := map[string]interface{}{
		"terms": map[string]interface{}{
			"_source.level": levels, // This will need to be adjusted based on actual data structure
		},
	}

	mustClauses = append(mustClauses, levelFilter)
	boolMap["must"] = mustClauses

	return query
}

// BuildSearchQueryWithPagination constructs a search query with search_after pagination
func (qb *QueryBuilder) BuildSearchQueryWithPagination(config interfaces.FetchConfig, searchAfter []interface{}) map[string]interface{} {
	query := qb.BuildSearchQuery(config)

	// Add search_after if provided
	if len(searchAfter) > 0 {
		query["search_after"] = searchAfter
	}

	return query
}

// BuildDashboardsQuery constructs a query compatible with OpenSearch Dashboards API
func (qb *QueryBuilder) BuildDashboardsQuery(config interfaces.FetchConfig, searchAfter []interface{}) map[string]interface{} {
	// Build filter clauses (separate from must)
	filterClauses := []map[string]interface{}{}

	// Add keyword filter
	if len(config.Keywords) > 0 {
		filterClauses = append(filterClauses, qb.buildKeywordFilterForDashboards(config.Keywords))
	}

	// Add time range filter
	filterClauses = append(filterClauses, qb.buildTimeRangeFilter(config.TimeRange))

	// Add service filter if specified
	if len(config.Services) > 0 {
		filterClauses = append(filterClauses, qb.buildServiceFilter(config.Services))
	}

	// Build the query body
	body := map[string]interface{}{
		"sort":          qb.buildSortClause(),
		"size":          config.MaxResults,
		"version":       true,
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
			"pre_tags":      []string{"@opensearch-dashboards-highlighted-field@"},
			"post_tags":     []string{"@/opensearch-dashboards-highlighted-field@"},
			"fields":        map[string]interface{}{"*": map[string]interface{}{}},
			"fragment_size": 2147483647,
		},
	}

	// Add search_after if provided for pagination
	if len(searchAfter) > 0 {
		body["search_after"] = searchAfter
	}

	// Add aggregations for date histogram
	body["aggs"] = map[string]interface{}{
		"2": map[string]interface{}{
			"date_histogram": map[string]interface{}{
				"field":          "@timestamp",
				"fixed_interval": "30m",
				"time_zone":      "Asia/Taipei",
				"min_doc_count":  1,
			},
		},
	}

	return map[string]interface{}{
		"params": map[string]interface{}{
			"index": config.Indices[0], // Use first index
			"body":  body,
		},
		"preference": time.Now().UnixMilli(),
	}
}

// buildKeywordFilterForDashboards creates a filter for Dashboards API format
func (qb *QueryBuilder) buildKeywordFilterForDashboards(keywords []string) map[string]interface{} {
	// Use multi_match for Dashboards API compatibility
	query := ""
	if len(keywords) > 0 {
		query = keywords[0]
	}

	return map[string]interface{}{
		"multi_match": map[string]interface{}{
			"type":    "phrase",
			"query":   query,
			"lenient": true,
		},
	}
}

// ValidateQuery performs basic validation on the query structure
func (qb *QueryBuilder) ValidateQuery(query map[string]interface{}) error {
	// Basic validation - ensure required fields exist
	if _, ok := query["query"]; !ok {
		return fmt.Errorf("query missing 'query' field")
	}

	if _, ok := query["size"]; !ok {
		return fmt.Errorf("query missing 'size' field")
	}

	return nil
}
