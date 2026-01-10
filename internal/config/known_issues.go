package config

import (
	"regexp"
	"strings"
	"sync"
)

// KnownIssue represents a known issue pattern
type KnownIssue struct {
	ID              string
	Name            string
	Category        string
	Severity        string
	Pattern         string
	compiledRegex   *regexp.Regexp
	Services        []string
	Description     string
	SuggestedActions []string
	AlertThreshold  struct {
		Total   int
		Density int
	}
}

// KnownIssuesRegistry manages known issue patterns
type KnownIssuesRegistry struct {
	issues map[string]*KnownIssue
	mu     sync.RWMutex
}

var (
	// Global registry instance
	globalRegistry *KnownIssuesRegistry
	regOnce        sync.Once
)

// GetRegistry returns the global known issues registry
func GetRegistry() *KnownIssuesRegistry {
	regOnce.Do(func() {
		globalRegistry = &KnownIssuesRegistry{
			issues: make(map[string]*KnownIssue),
		}
		// Initialize with predefined issues
		initializePredefinedIssues(globalRegistry)
	})
	return globalRegistry
}

// initializePredefinedIssues initializes the registry with predefined known issues
func initializePredefinedIssues(reg *KnownIssuesRegistry) {
	issues := []KnownIssue{
		{
			ID:       "ISSUE-001",
			Name:     "索引不匹配錯誤",
			Category: "logic",
			Severity: "high",
			Pattern:  "mismatch index|index out of range",
			Services: []string{"pp-slot-api", "pp-slot-replay"},
		},
		{
			ID:       "ISSUE-002",
			Name:     "JSON 解析錯誤",
			Category: "parsing",
			Severity: "high",
			Pattern:  "unexpected end of json input|invalid json|unmarshal error",
			Services: []string{"pp-slot-api", "pp-slot-session"},
		},
		{
			ID:       "ISSUE-003",
			Name:     "遊戲點數不足",
			Category: "business_logic",
			Severity: "medium",
			Pattern:  "insufficient points|balance not enough|insufficient funds",
			Services: []string{"pp-slot-api"},
		},
		{
			ID:       "ISSUE-004",
			Name:     "會話密鑰為空",
			Category: "authentication",
			Severity: "high",
			Pattern:  "empty mgckey|invalid mgckey|mgckey not found",
			Services: []string{"pp-slot-api", "pp-slot-session"},
		},
		{
			ID:       "ISSUE-005",
			Name:     "Redis 快取取得失敗",
			Category: "infrastructure",
			Severity: "high",
			Pattern:  "redis message is nil|redis connection refused|redis timeout",
			Services: []string{"pp-slot-api", "pp-slot-index"},
		},
		{
			ID:       "ISSUE-006",
			Name:     "玩家記錄未找到",
			Category: "data",
			Severity: "medium",
			Pattern:  "player not found|account not found|no such account",
			Services: []string{"pp-slot-api", "pp-slot-session"},
		},
		{
			ID:       "ISSUE-007",
			Name:     "遊戲配置缺失",
			Category: "configuration",
			Severity: "medium",
			Pattern:  "game config does not exist|game not found|invalid game id",
			Services: []string{"pp-slot-api"},
		},
		{
			ID:       "ISSUE-008",
			Name:     "帳戶被鎖定",
			Category: "security",
			Severity: "high",
			Pattern:  "account is locked|account suspended|login blocked",
			Services: []string{"pp-slot-session"},
		},
		{
			ID:       "ISSUE-009",
			Name:     "不支援的遊戲類型組合",
			Category: "business_logic",
			Severity: "low",
			Pattern:  "does not support spin type|unsupported game mode|invalid configuration",
			Services: []string{"pp-slot-api"},
		},
		{
			ID:       "ISSUE-010",
			Name:     "錢包操作失敗",
			Category: "payment",
			Severity: "high",
			Pattern:  "wallet fail|wallet error|transaction failed|insufficient balance",
			Services: []string{"pp-slot-api"},
		},
	}

	for i := range issues {
		issue := &issues[i]
		// Compile regex pattern
		if issue.Pattern != "" {
			// Split pattern by | (OR) and create regex
			patterns := strings.Split(issue.Pattern, "|")
			if len(patterns) > 0 {
				// Create case-insensitive regex pattern
				regexStr := "(?i)(" + strings.Join(patterns, "|") + ")"
				if compiled, err := regexp.Compile(regexStr); err == nil {
					issue.compiledRegex = compiled
				}
			}
		}
		reg.issues[issue.ID] = issue
	}
}

// MatchContent tries to match error content against known issue patterns
// Returns the matched issue, if any
func (r *KnownIssuesRegistry) MatchContent(content string) *KnownIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, issue := range r.issues {
		if issue.compiledRegex != nil && issue.compiledRegex.MatchString(content) {
			return issue
		}
	}
	return nil
}

// MatchContentAndService matches both content and service name
func (r *KnownIssuesRegistry) MatchContentAndService(content, serviceName string) *KnownIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, issue := range r.issues {
		// Check if content matches
		if issue.compiledRegex != nil && issue.compiledRegex.MatchString(content) {
			// Check if service matches (if specified)
			if len(issue.Services) == 0 {
				// No service filter, match based on content only
				return issue
			}

			for _, svc := range issue.Services {
				if strings.Contains(serviceName, svc) || svc == "*" {
					return issue
				}
			}
		}
	}
	return nil
}

// GetIssueByID retrieves a known issue by its ID
func (r *KnownIssuesRegistry) GetIssueByID(id string) *KnownIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.issues[id]
}

// GetAllIssues returns all known issues
func (r *KnownIssuesRegistry) GetAllIssues() []*KnownIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()

	issues := make([]*KnownIssue, 0, len(r.issues))
	for _, issue := range r.issues {
		issues = append(issues, issue)
	}
	return issues
}
