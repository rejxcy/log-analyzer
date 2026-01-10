# Log Analyzer 架構設計文檔

## 系統概覽

Log Analyzer 是一個簡化的日誌分析系統，專為快速診斷 OpenSearch 中的錯誤模式而設計。

**核心理念**：最小化參數、預設行為、開箱即用。

```bash
go run cmd/analyzer/main.go -time 24h
```

## 數據流向

```
OpenSearch Dashboards
      ↓
[時間窗口分割] 
      ↓ (每 30 分鐘一個窗口)
[原始日誌獲取] 604 條日誌
      ↓
[預處理] 
  • JSON 解析
  • 服務名稱提取
  • Wrapper 移除
      ↓ 解析成功: 604 條
[正規化]
  • 計算 Error Fingerprint (SHA256)
  • 按指紋去重
      ↓ 得到 23 個唯一模式
[聚合]
  • 時間統計 (按小時分佈)
  • 服務分類
  • 峰值計算
      ↓ 3 個服務
[分析]
  • 已知問題匹配
  • 嚴重級別評估
      ↓ 10 個已知 + 13 個新問題
[報告生成]
  • 為每個服務生成獨立報告
  • 生成 JSON 分析結果
      ↓
reports/
  ├── 2026-01-11_pp-slot-api_02-16-12.md
  ├── 2026-01-11_pp-slot-rpc_02-16-12.md
  ├── 2026-01-11_pp-slot-math_02-16-12.md
  └── analysis_2026-01-11_02-16-12.json
```

## 模塊說明

### 1. 數據獲取 (OpenSearch)

**文件**: `cmd/analyzer/main.go` → `fetchFromOpenSearchWithWindows()`

**策略**：時間窗口分割

```
-time 24h 會分割成 48 個 30 分鐘的窗口

時間軸：
現在 ← | 30m | 30m | 30m | ... | 30m | ← 24h 前

每個窗口獨立查詢，避免 500 筆限制導致數據丟失
```

### 2. 預處理 (Preprocessor)

**文件**: `internal/preprocessor/processor.go`

**職責**：
- 解析 JSON 消息
- 提取服務名稱（多種來源備用）
- 移除 Kubernetes wrapper

**輸入**: `RawLog[]` (604 條)  
**輸出**: `ParsedLog[]` (604 條)

### 3. 正規化 (Normalizer)

**文件**: `internal/normalizer/normalizer.go`

**職責**：
- 計算 Error Fingerprint（SHA256）
- 按指紋聚合重複錯誤

**輸入**: `ParsedLog[]` (604 條)  
**輸出**: `ErrorGroup[]` (23 個唯一模式)  
**去重率**: 99.5%

### 4. 聚合 (Aggregator)

**文件**: `internal/aggregator/aggregator.go`

**職責**：
- 時間分佈統計（按小時）
- 服務統計
- 峰值計算

**輸出示例**：
```
- 總錯誤數：604
- 服務總數：3
- 峰值時段：00:00（175 個錯誤）
- 平均密度：0.00 錯誤/分鐘
```

### 5. 分析 (Analysis)

**文件**: `cmd/analyzer/main.go` → `createAnalysesFromErrorGroups()`

**職責**：
- 從 ErrorGroup 創建 Analysis 對象
- 已知問題匹配
- 嚴重級別評估

### 6. 報告生成 (Reporter)

**文件**: `internal/reporter/reporter.go`

**職責**：
- 按服務分組生成獨立報告
- Markdown 格式
- 包含 JSON 導出

**報告名稱格式**: `日期_服務_時間.md`

## 已知問題系統

**文件**: `internal/config/known_issues.go`

**功能**：
- 10 個預定義的已知問題
- 正則表達式匹配
- 線程安全（RWMutex）

**在報告中的顯示**：
- 頂部統計：`已知問題: 10 | 新問題: 13`
- 頂級問題：標示 Issue ID 和分類
- 其他問題：✅ ISSUE-XXX 或 🆕 新問題

## 配置系統

**文件**: `configs/config.yaml`

**必需配置**：
```yaml
opensearch:
  url: "http://..."
  username: "..."
  password: "..."
```

**自動配置**（默認值）：
- 查詢關鍵字：`error`
- 時間窗口：`30m`
- 輸出目錄：`./reports`
- 索引列表：4 個預設服務

## 簡化的設計決策

### ✅ 已移除

| 功能 | 原因 |
|------|------|
| `-fetch/-input` 開關 | 預設始終從 OpenSearch 獲取 |
| `-output` 參數 | 固定使用 `./reports` |
| `-keyword` 參數 | 從 config.yaml 讀取 |
| `-indices` 參數 | 從 config.yaml 讀取 |
| `-window` 參數 | 固定 30 分鐘 |
| 本地文件加載 | 簡化邏輯，專注 OpenSearch |
| Mock 數據生成 | 移除測試干擾 |

### ✅ 保留

| 參數 | 用途 |
|------|------|
| `-time` | 唯一必需參數，控制查詢時間範圍 |

## 線程安全性

**當前實現**：完全單線程，無並發問題

```
main() → 順序執行所有步驟 → 順序寫入文件
```

**RWMutex 使用**：
- `known_issues.go` 中的 `KnownIssuesRegistry` 使用 RWMutex
- 允許多個並發讀操作
- 支持未來的並發擴展

## 性能特徵

```
典型運行（24 小時，~600 個錯誤）：
- 時間窗口分割：48 個
- 數據獲取：8-10 秒
- 預處理：< 1 秒
- 正規化：< 1 秒
- 聚合：< 1 秒
- 分析：< 1 秒
- 報告生成：< 1 秒

總耗時：~10-15 秒
```

## 未來改進空間

1. **增量更新去重**
   - 記錄上次運行時間戳
   - 僅查詢新日誌
   - 合併歷史數據避免重複計算

2. **並發優化**
   - 多線程日誌處理
   - 使用 Mutex/Channel 保護共享數據
   - 服務並行報告生成

3. **動態規則引擎**
   - 從 YAML 文件動態加載規則
   - 支持熱重載（不重啟）
   - 分類、嚴重級別、建議行動自定義

4. **定時任務**
   - Cron 表達式支持
   - 定時自動執行分析
   - 結果持久化與趨勢分析

## 總結

| 特性 | 狀態 |
|------|------|
| OpenSearch 集成 | ✅ 完成 |
| 時間窗口分割 | ✅ 完成 |
| 錯誤去重 | ✅ 完成 |
| 已知問題匹配 | ✅ 完成 |
| 獨立服務報告 | ✅ 完成 |
| 中文本地化 | ✅ 完成 |
| 增量更新 | ⏳ 計劃中 |
| 並發優化 | ⏳ 計劃中 |
| 定時任務 | ⏳ 計劃中 |

## 整體工作流程

```
┌─────────────────────────────────────────────────────────────────┐
│                      三階段日誌分析流程                          │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────┐      ┌─────────────────┐      ┌──────────────┐
│  第 1 階段      │      │  第 2 階段      │      │  第 3 階段   │
│  數據獲取       │ ───→ │  數據保存       │ ───→ │  數據分析    │
│  (Fetch)       │      │  (Storage)      │      │  (Analysis)  │
└─────────────────┘      └─────────────────┘      └──────────────┘
```

---

## 詳細流程說明

### 階段 1：數據獲取 (Fetcher)

#### 🔍 獲取邏輯

```
時間軸（現在往回看）
├─ 現在
├─ 1 小時前 ──┐
├─ 2 小時前  │ 查詢範圍
├─ 3 小時前  │ (last 3h)
└─ 3.5 小時前─┘

OpenSearch API 特性：
• 單次查詢最多返回 500 筆
• 支持 search_after 分頁（無限滾動）
• 按 @timestamp desc 排序
```

#### 📦 分批獲取流程

**目前實現（簡單版本）**：

```
go run cmd/test-pagination/main.go -max-batches 5

執行流程：
┌──────────────────────────────────────┐
│ Batch 1: 查詢 last 1h，返回 0-500 筆  │ ←─ search_after: null
└──────────────────────────────────────┘
                    ↓
           有返回 500 筆？ ─ 是 ─→
                    │
                    否
                    ↓
            ✅ 完成，共 1 批次

┌──────────────────────────────────────┐
│ 如果要查詢更多（假設第 1 批返回 500） │
└──────────────────────────────────────┘

Batch 1: 返回 500 筆
         最後一筆: {timestamp: "2026-01-10T10:00:00Z", _id: "doc-500"}
                        ↓
Batch 2: search_after: ["2026-01-10T10:00:00Z", "doc-500"]
         返回 500 筆
                        ↓
Batch 3: search_after: ["2026-01-10T09:30:00Z", "doc-1000"]
         返回 200 筆 (終止)
```

#### ⚠️ 關鍵問題：時間窗口分割

**當前實現的限制**：

```
查詢： -time 24h -keyword error

如果過去 24 小時有 2000 筆錯誤日誌：

❌ 問題：
┌─────────────────────────────┐
│ 單次查詢會丟失 1500 筆！     │
│ (OpenSearch 最多返回 500筆) │
└─────────────────────────────┘

✅ 解決方案（需實現）：
┌──────────────────────────────────────┐
│ 將 24h 分割成多個小時間窗口          │
├──────────────────────────────────────┤
│ 時間窗口 1: 現在 → 23 小時前          │
│   查詢結果: 150 筆 ✅                 │
│                                      │
│ 時間窗口 2: 23 小時前 → 22 小時前    │
│   查詢結果: 180 筆 ✅                 │
│                                      │
│ 時間窗口 3: 22 小時前 → 21 小時前    │
│   查詢結果: 200 筆 ✅                 │
│                                      │
│ ... (共 24 個時間窗口)                │
│                                      │
│ 總計: 2000 筆 ✅ (無丟失)             │
└──────────────────────────────────────┘
```

---

### 階段 2：數據保存 (Storage)

#### 💾 保存格式

```
./data/opensearch-responses/
├── all-documents_2026-01-10_23-19-38.json
│   {
│     "documents": [
│       {
│         "index": "pp-slot-api-log*",
│         "id": "doc-1",
│         "source": { ... JSON 原始數據 ... }
│       },
│       ...
│     ],
│     "total_documents": 2000
│   }
│
├── pagination-summary_2026-01-10_23-19-38.json
│   {
│     "timestamp_start": "2026-01-09T23:19:38+08:00",
│     "timestamp_end": "2026-01-10T23:19:38+08:00",
│     "total_hits": 2000,
│     "collected_docs": 2000,
│     "batches": 4,
│     "timed_out": false
│   }
│
└── raw-response_2026-01-10_23-13-34.json
    (第一批原始 API 回應)
```

#### 🔄 保存邏輯

```
數據獲取 (test-pagination/main.go)
         ↓
    逐批次接收
         ↓
    累積到內存
         ↓
    完成所有批次
         ↓
    一次性保存到 JSON 文件
         
特點：
• 單線程，不存在並發問題
• 以時間戳命名，避免覆蓋
• 包含摘要信息（便於追蹤）
```

---

### 階段 3：數據分析 (Analysis)

#### 🔐 分析管道

```
讀取保存的 JSON
       ↓
┌─────────────────────────────────┐
│ Step 1: Preprocessor            │
│ 輸入: RawLog[]                  │
│ 動作: 解析 JSON + 提取字段      │
│ 輸出: ParsedLog[]               │
│ 線程: 單線程，順序處理          │
└─────────────────────────────────┘
       ↓
┌─────────────────────────────────┐
│ Step 2: Normalizer              │
│ 輸入: ParsedLog[]               │
│ 動作: 計算指紋 + 分組            │
│ 輸出: ErrorGroup[]{             │
│         Fingerprint: "abc123",  │
│         Count: 10,              │
│         Logs: [...]             │
│       }                         │
│ 線程: 單線程，順序處理          │
│ 去重: 按 Fingerprint 聚合       │
└─────────────────────────────────┘
       ↓
┌─────────────────────────────────┐
│ Step 3: Aggregator              │
│ 輸入: ErrorGroup[]              │
│ 動作: 時間統計 + 密度計算        │
│ 輸出: AggregationResult{        │
│         HourlyDistribution: {}, │
│         ServiceStats: {},       │
│         ...                     │
│       }                         │
│ 線程: 單線程，順序處理          │
└─────────────────────────────────┘
       ↓
┌─────────────────────────────────┐
│ Step 4: Reporter                │
│ 輸入: Analysis[], AggResult     │
│ 動作: 生成 Markdown 報告         │
│ 輸出: error_analysis_*.md       │
│ 線程: 單線程，順序處理          │
└─────────────────────────────────┘
       ↓
生成報告完成 ✅
```

---

## 競爭與重複計算問題分析

### ❓ 會不會有競爭問題？

**答案：目前 NO，但有潛在風險**

#### 當前架構（安全）

```
單線程順序執行：

main.go
  ├─ loadRawLogsFromJSON()          ← 讀取 JSON（順序）
  ├─ preprocessor.Process()         ← 解析（順序）
  ├─ normalizer.Normalize()         ← 去重（順序）
  ├─ aggregator.Aggregate()         ← 聚合（順序）
  └─ reporter.Generate()            ← 生成（順序）

特點：
✅ 無並發
✅ 無競爭
✅ 無死鎖
✅ 易於調試
```

#### 未來隱患（多線程時）

```
如果要並行化（例如：多個服務同時分析）

❌ 錯誤示例：
goroutine 1: normalizer.Normalize(logs[0:500])
goroutine 2: normalizer.Normalize(logs[500:1000])
goroutine 3: normalizer.Normalize(logs[1000:])
                    ↓
           共享的 fingerprint map
                    ↓
           🚨 Map 並發寫入 → panic!

✅ 解決方案：
1. 使用 sync.Mutex 保護共享數據
2. 使用 sync.RWMutex 讀寫分離
3. 使用 channel 匯聚結果
4. 使用 atomic 操作計數
```

### ❓ 會不會有重複計算？

**答案：YES，需要注意去重策略**

#### 重複日誌來源

```
場景 1：增量更新時重複
┌─────────────────────────┐
│ 第 1 次執行              │
│ 查詢: last 24h           │
│ 獲得: 日誌 A, B, C       │
│ 保存: all-documents_*.json
└─────────────────────────┘
            ↓
        1 小時後
            ↓
┌─────────────────────────┐
│ 第 2 次執行              │
│ 查詢: last 24h           │
│ 獲得: 日誌 A, B, C, D, E │ ← A, B, C 重複了！
│ 保存: all-documents_*.json (新文件)
└─────────────────────────┘

問題：
• A, B, C 被重複分析
• 統計數據會疊加（計數翻倍）
```

#### 當前解決方案

```
使用 Error Fingerprint 去重：

日誌 A: "Connection timeout" → Fingerprint: "abc123"
日誌 B: "Connection timeout to 192.168.x.x" → Fingerprint: "abc123"
日誌 C: "RPC call failed" → Fingerprint: "def456"

分組結果：
{
  "abc123": {
    count: 2,           ← 去重後只算 1 個指紋
    logs: [A, B],       ← 保存所有副本
  },
  "def456": {
    count: 1,
    logs: [C],
  }
}

優點：
✅ 同一錯誤只計算一次
✅ 保留所有日誌副本便於分析
✅ 易於識別相同根因的不同表現形式
```

#### 未來改進：增量更新去重

```
需要實現的邏輯：

第 2 次運行時：
1. 記錄前一次運行的時間戳
   last_run_time = "2026-01-10T23:00:00Z"

2. 只查詢新數據
   query: @timestamp > last_run_time

3. 與歷史數據對比
   - 如果 fingerprint 在舊數據中存在 → 更新計數
   - 如果 fingerprint 新出現 → 新增警報

4. 合併結果
   new_stats = old_stats ∪ new_data
```

---

## 運行模式對比

### 模式 A：完整分析（推薦用於日報）

```bash
go run cmd/analyzer/main.go -fetch -time 24h

流程：
1. 從 OpenSearch 查詢過去 24 小時
2. 自動分批（當前簡單實現，未來改進為時間窗口分割）
3. 保存 JSON
4. 立即分析
5. 輸出報告

時間：5-15 秒
適合：晨報、定時任務、手動調查
```

### 模式 B：增量分析（推薦用於實時監控）

```bash
# 第 1 次（完整）
go run cmd/test-pagination/main.go \
  -time 1h -output ./data

# 第 2 次（1 小時後，增量）
go run cmd/test-pagination/main.go \
  -time 10m -output ./data

# 分析
go run cmd/analyzer/main.go -input ./data

改進空間：
• 記錄 last_run_time
• 自動計算增量時間
• 合併去重歷史數據
• 檢測新的錯誤模式
```

### 模式 C：本地重複分析

```bash
# 保存一次
go run cmd/test-pagination/main.go -time 24h -output ./data

# 可以多次分析而不再查詢 OpenSearch
go run cmd/analyzer/main.go -input ./data -output ./report1
go run cmd/analyzer/main.go -input ./data -output ./report2
go run cmd/analyzer/main.go -input ./data -output ./report3

優點：
✅ 不影響 OpenSearch
✅ 快速重複分析
✅ 嘗試不同參數
```

---

## 數據流向圖

```
┌──────────────────────────────────────────────────────────────┐
│                    OpenSearch Dashboards                      │
│                   (2000 筆錯誤日誌)                           │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ↓ (分批查詢)
        ┌───────────────────────────────┐
        │ test-pagination/main.go        │
        │ • 第 1 批: 0-500 筆           │
        │ • 第 2 批: 500-1000 筆        │
        │ • 第 3 批: 1000-1500 筆       │
        │ • 第 4 批: 1500-2000 筆       │
        └───────────┬───────────────────┘
                    │
                    ↓ (逐批保存)
        ┌───────────────────────────────────────────┐
        │ ./data/opensearch-responses/               │
        │ └─ all-documents_*.json (2000 筆)         │
        │ └─ pagination-summary_*.json              │
        └───────────┬─────────────────────────────┘
                    │
                    ↓ (一次性讀取)
        ┌───────────────────────────────┐
        │ analyzer/main.go               │
        │ Step 1: Preprocessor (解析)    │
        │   2000 → 2000 ParsedLog        │
        │ Step 2: Normalizer (去重)      │
        │   2000 → 150 ErrorGroup        │
        │ Step 3: Aggregator (聚合)      │
        │   150 → AggregationResult      │
        │ Step 4: Reporter (報告)        │
        │   結果 → Markdown              │
        └───────────┬───────────────────┘
                    │
                    ↓
        ┌───────────────────────────────┐
        │ ./reports/                     │
        │ └─ error_analysis_*.md         │
        │ └─ analysis_*.json             │
        └───────────────────────────────┘
```

---

## 總結

### ✅ 你的理解正確

| 階段 | 名稱 | 職責 | 當前狀態 |
|------|------|------|--------|
| 1 | 獲取 | 從 OpenSearch 分批獲取日誌 | ✅ 實現 |
| 2 | 保存 | 保存到本地 JSON 文件 | ✅ 實現 |
| 3 | 分析 | 預處理→去重→聚合→報告 | ✅ 實現 |

### ⚠️ 需要改進

| 項目 | 現況 | 問題 | 優先級 |
|------|------|------|--------|
| **時間窗口分割** | 簡單實現 | 超過 500 筆日誌會丟失 | 🔴 高 |
| **增量更新去重** | 無 | 重複執行會重複計算 | 🟡 中 |
| **並發安全性** | 單線程 | 無法並行化 | 🟡 中 |
| **性能優化** | 基礎版 | 大規模日誌較慢 | 🟢 低 |

### 🎯 後續改進順序

1. **優先**：實現時間窗口分割（避免數據丟失）
2. **次要**：實現增量去重（支持增量更新）
3. **最後**：並發優化（性能提升）
