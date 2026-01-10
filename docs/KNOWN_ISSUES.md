# 已知問題判斷系統

## 概述

系統現在包含一個智能的**已知問題匹配引擎**，可以自動識別錯誤是否是已知的問題。

## 運作原理

### 1. 已知問題登記表 (Known Issues Registry)

位置：`internal/config/known_issues.go`

系統維護一個全域的已知問題登記表，每個已知問題包含：

- **ID**: 唯一識別碼（ISSUE-001 ~ ISSUE-010）
- **Name**: 人類可讀的問題名稱（中文）
- **Category**: 問題分類（logic, parsing, infrastructure, security 等）
- **Severity**: 嚴重級別（low, medium, high）
- **Pattern**: 用於匹配錯誤內容的正則表達式
- **Services**: 適用的服務列表
- **Description**: 詳細描述
- **SuggestedActions**: 建議的修復步驟
- **AlertThreshold**: 告警閾值

### 2. 已知問題列表

位置：`configs/known-issues/known-issues.yaml`

當前包含 10 個已知問題：

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

### 3. 匹配流程

當分析錯誤群組時：

```
錯誤群組 (NormalizedContent + ServiceName)
         ↓
    [已知問題匹配]
         ↓
     ┌───┴────┐
     ↓        ↓
   已知    未知
   (IsKnown=true)  (IsKnown=false)
   有 IssueID       無 IssueID
```

匹配邏輯：
1. 檢查錯誤內容是否符合任何已知問題的正則表達式模式
2. 如果符合，驗證該服務是否在該問題的適用服務列表中
3. 如果兩者都符合，標記為已知問題，並設定對應的 Issue ID

## 使用方式

### 查詢已知問題

```go
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

## 報告中的顯示

在生成的 Markdown 報告中，已知問題會顯示 Issue ID：

```markdown
### 1. 索引不匹配錯誤

**位置**: `logic/game_service_logic.go:175`  
**發生次數**: 2172  
**已知問題**: ISSUE-001
**嚴重性**: 🔴 **HIGH** - ...
```

## 擴展已知問題

### 方法 1：更新代碼中的預定義列表

編輯 `internal/config/known_issues.go` 中的 `initializePredefinedIssues()` 函數。

### 方法 2：從 YAML 加載（未來功能）

計畫實現從 `configs/known-issues/known-issues.yaml` 動態加載已知問題。

## 優勢

✅ **自動化識別** - 無需手動檢查，系統自動識別已知問題  
✅ **模式匹配** - 使用正則表達式進行靈活的模式識別  
✅ **服務過濾** - 可根據服務名稱進行過濾  
✅ **易於擴展** - 新增已知問題只需添加到列表  
✅ **性能優化** - 使用編譯的正則表達式，查詢快速  
✅ **中文本地化** - 所有描述和建議均為中文  

## 未來改進

- [ ] 從 YAML 檔案動態加載已知問題
- [ ] 實現正則表達式規則的熱重載
- [ ] 添加告警閾值檢查
- [ ] 實現機器學習模型進行自動分類
- [ ] 支援自定義已知問題匹配器
