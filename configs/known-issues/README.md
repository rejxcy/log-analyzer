# 已知問題系統

## 概述

Log Analyzer 包含一個**智能的已知問題匹配引擎**，可以自動識別錯誤是否為已知的問題。系統在分析時會將錯誤與預定義的已知問題進行匹配，並在報告中清楚地標示。

## 運作原理

### 已知問題登記表 (Known Issues Registry)

位置：`internal/config/known_issues.go`

系統維護一個全域的已知問題登記表，每個已知問題包含：

| 欄位 | 說明 |
|------|------|
| **ID** | 唯一識別碼（ISSUE-001 ~ ISSUE-010） |
| **Name** | 人類可讀的問題名稱（中文） |
| **Category** | 問題分類（logic, parsing, infrastructure, security 等） |
| **Severity** | 嚴重級別（low, medium, high） |
| **Pattern** | 用於匹配錯誤內容的正則表達式 |
| **Services** | 適用的服務列表 |
| **Description** | 詳細描述 |
| **SuggestedActions** | 建議的修復步驟 |
| **AlertThreshold** | 告警閾值 |

### 預定義的已知問題

系統包含 10 個遊戲服務常見的已知問題：

| ID | 名稱 | 嚴重性 | 描述 |
|----|------|--------|------|
| ISSUE-001 | 索引不匹配錯誤 | HIGH | 遊戲邏輯中的索引計算錯誤 |
| ISSUE-002 | JSON 解析錯誤 | HIGH | JSON 數據格式不完整或損壞 |
| ISSUE-003 | 遊戲點數不足 | MEDIUM | 玩家遊戲點數不足 |
| ISSUE-004 | 會話密鑰為空 | HIGH | 使用者會話加密密鑰遺失 |
| ISSUE-005 | Redis 快取取得失敗 | HIGH | Redis 無法響應 |
| ISSUE-006 | 玩家記錄未找到 | MEDIUM | 不存在的玩家記錄 |
| ISSUE-007 | 遊戲配置缺失 | MEDIUM | 遊戲配置不存在 |
| ISSUE-008 | 帳戶被鎖定 | HIGH | 帳戶因安全原因被鎖定 |
| ISSUE-009 | 不支援的遊戲類型組合 | LOW | 不支援的遊戲配置 |
| ISSUE-010 | 錢包操作失敗 | HIGH | 錢包系統操作失敗 |

### 匹配流程

當分析錯誤群組時的匹配邏輯：

```
錯誤群組 (NormalizedContent + ServiceName)
         ↓
    [已知問題匹配引擎]
         ↓
     ┌───┴────┐
     ↓        ↓
   已知      未知
   (IsKnown=true)    (IsKnown=false)
   有 IssueID         無 IssueID
```

**匹配步驟**：

1. 檢查錯誤內容是否符合任何已知問題的正則表達式模式
2. 如果符合，驗證該服務是否在該問題的適用服務列表中
3. 如果兩者都符合，標記為已知問題，並設定對應的 Issue ID

## 在代碼中使用

### Go 代碼示例

```go
import "log-analyzer/internal/config"

// 獲取已知問題登記表
registry := config.GetRegistry()

// 方式 1：根據錯誤內容和服務名稱匹配
matchedIssue := registry.MatchContentAndService(
    "unexpected end of json input",
    "pp-slot-api",
)

// 方式 2：只根據內容匹配
matchedIssue := registry.MatchContent(
    "mismatch index [123], expected [456]",
)

// 方式 3：根據 Issue ID 查詢
issue := registry.GetIssueByID("ISSUE-001")

// 方式 4：取得所有已知問題
allIssues := registry.GetAllIssues()
```

## 在報告中的顯示

### 報告頂部統計

```
**已知問題**: 8 | **新問題**: 4
```

### 頂級問題詳細信息

如果是已知問題，會在報告中顯示：

```markdown
### 1. 索引不匹配錯誤

**位置**: `logic/game_service_logic.go:175`  
**發生次數**: 2172  
**已知問題**: `ISSUE-001` - 索引不匹配錯誤  
**分類**: logic  
**時間模式**: **爆發型** (10:00-11:00 集中 2100 次)  
**嚴重性**: 🔴 **HIGH** - 高頻率錯誤 + 業務時段集中 + 可能影響用戶體驗
```

### 其他問題表格

低頻率問題顯示狀態欄：

```
| 問題名稱 | 位置 | 發生次數 | 狀態 | 嚴重性 |
|---------|------|--------|------|-------|
| player not found | ... | 247 | ✅ ISSUE-006 | medium |
| game config error | ... | 4 | 🆕 新問題 | low |
```

狀態欄顯示：
- `✅ ISSUE-XXX` - 已知問題（帶 Issue ID）
- `🆕 新問題` - 未識別的新問題

## 擴展已知問題

### 方式：修改代碼中的預定義問題

編輯 `internal/config/known_issues.go` 中的 `initializePredefinedIssues()` 函數，添加新的已知問題：

```go
func initializePredefinedIssues(reg *KnownIssuesRegistry) {
    // ... 現有問題 ...
    
    // 添加新問題
    reg.issues["ISSUE-011"] = &KnownIssue{
        ID:              "ISSUE-011",
        Name:            "新的問題名稱",
        Category:        "logic",
        Severity:        "high",
        Pattern:         "正則表達式模式",
        compiledRegex:   regexp.MustCompile("(?i)正則表達式模式"),
        Services:        []string{"pp-slot-api", "pp-slot-rpc"},
        Description:     "問題描述",
        SuggestedActions: []string{"行動 1", "行動 2"},
        AlertThreshold:  100,
    }
}
```

然後重新編譯即可。

## 常見問題

### Q: 如何驗證已知問題匹配是否正常工作？

A: 運行分析後檢查生成的報告，查看：
1. 報告頂部的已知/新問題統計數字
2. 頂級問題中是否顯示 Issue ID
3. 其他問題表格的狀態欄

### Q: 如何添加新的已知問題？

A: 修改 `internal/config/known_issues.go` 中的 `initializePredefinedIssues()` 函數，重新編譯運行。

### Q: 已知問題匹配失敗怎麼辦？

A: 檢查：
1. 正則表達式模式是否正確（使用在線正則工具測試）
2. 服務名稱是否完全匹配
3. 錯誤內容是否包含預期的關鍵字

## 技術細節

### 線程安全性

已知問題登記表使用 `sync.RWMutex` 保護，支持併發讀取操作：

```go
type KnownIssuesRegistry struct {
    issues    map[string]*KnownIssue
    mu        sync.RWMutex
    regOnce   sync.Once
}
```

### 性能優化

- 正則表達式在初始化時編譯一次，避免重複編譯開銷
- 使用 RWMutex 而非普通 Mutex，允許多個並發讀操作

## 相關文檔

- [README.md](../../README.md) - 項目總覽
- [ARCHITECTURE.md](../../ARCHITECTURE.md) - 系統架構