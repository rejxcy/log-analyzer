# Log Analyzer 使用指南

本文檔說明如何使用 Log Analyzer 系統進行日誌分析。

## 快速開始

### 方式 1：使用 Mock 數據演示（最快）

```bash
cd ./log-analyzer
go run cmd/analyzer/main.go
```

這會使用示範性的 mock 日誌數據運行完整的分析管道，並在 `./reports` 目錄生成報告。

**預期輸出時間**：< 1 秒  
**輸出位置**：`./reports/error_analysis_*.md`

---

### 方式 2：從本地保存的 OpenSearch 數據運行

如果你已經用 `test-pagination` 或 `test-dashboards-api-file` 保存了 JSON 文件：

```bash
go run cmd/analyzer/main.go \
  -input ./data/opensearch-responses \
  -output ./reports
```

**參數說明**：
- `-input` 目錄：包含 `all-documents_*.json` 或其他 OpenSearch 回應文件的目錄
- `-output` 目錄：生成報告的目標目錄

---

### 方式 3：直接從 OpenSearch Dashboards 實時獲取（推薦用於生產）

```bash
go run cmd/analyzer/main.go \
  -fetch \
  -time 24h \
  -keyword error \
  -indices "pp-slot-api-log*,pp-slot-rpc-log*,pp-slot-math-log*"
```

**參數說明**：
- `-fetch`：啟用 OpenSearch 實時獲取模式
- `-time`：查詢時間範圍（例如：`1h`, `24h`, `7d`）
- `-keyword`：搜尋關鍵字（預設：`error`）
- `-indices`：逗號分隔的索引列表（預設：`pp-slot-api-log*`）
- `-output`：報告輸出目錄（預設：`./reports`）

**前置條件**：
- `configs/config.yaml` 必須包含有效的 OpenSearch 連接信息

**預期執行時間**：3-15 秒（取決於日誌數量）

---

## 配置 OpenSearch 連接

編輯 `configs/config.yaml`：

```yaml
opensearch:
  url: "{{opensearch_url}}"
  username: "{username}"
  password: "{{password}}"
  indices:
    - "{{log-service}}-log*"
    - "{{log-service}}-log*"
```

> ⚠️ **安全提示**：不要在代碼庫中提交明文密碼。建議使用環境變量替換。

---

## 輸出文件說明

運行分析後，會生成以下文件：

```
reports/
├── error_analysis_2026-01-10_23-43-28.md    # 主分析報告（Markdown）
├── analysis_2026-01-10_23-43-28.json        # 詳細分析數據（JSON）
└── pagination-summary_*.json                 # （如果使用分頁工具）
```

### 報告文件格式

生成的 Markdown 報告包含：

1. **執行摘要**
   - 查詢時間範圍
   - 分析生成時間
   - 總計統計數字

2. **錯誤摘要表格**
   - 錯誤指紋（SHA256）
   - 發生次數
   - 影響的服務
   - 嚴重級別
   - 狀態（已知/未知）

3. **時間分佈圖表**
   - ASCII 圖表顯示按小時分佈的錯誤

4. **服務影響統計**
   - 各服務的錯誤計數

5. **詳細錯誤信息**
   - 每個錯誤群組的示範日誌

---

## 完整的管道工作流

```
┌─────────────────────────────────┐
│ 1. 數據獲取 (Fetcher)           │
│   - OpenSearch API 或本地文件   │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│ 2. 預處理 (Preprocessor)        │
│   - 解析 JSON                   │
│   - 移除 Kubernetes wrapper     │
│   - 提取服務名稱                │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│ 3. 正規化 (Normalizer)          │
│   - 標準化內容                  │
│   - 計算 Error Fingerprint      │
│   - 按指紋去重                  │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│ 4. 聚合 (Aggregator)            │
│   - 計時統計                    │
│   - 密度計算                    │
│   - 服務分類                    │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│ 5. 報告生成 (Reporter)          │
│   - 生成 Markdown 報告          │
│   - 保存分析結果 JSON           │
└─────────────────────────────────┘
```

---

## 故障排除

### 問題 1：`config.yaml` 文件未找到

**解決方案**：確保在正確的目錄運行命令

```bash
cd ./log-analyzer
```

### 問題 2：OpenSearch 連接失敗

**檢查清單**：
- 確認 OpenSearch Dashboards URL 是否正確
- 驗證用戶名和密碼
- 檢查網絡連接

```bash
# 測試連接
go run cmd/test-connection/main.go
```

### 問題 3：查詢返回 0 結果

**可能原因**：
- 指定的時間範圍內沒有錯誤日誌
- 關鍵字不匹配任何日誌
- 索引名稱不正確

**解決方案**：
- 試試更長的時間範圍（例如：`-time 7d`）
- 檢查 OpenSearch Dashboards UI 確認日誌存在

---

## 高級用法

### 保存 OpenSearch 回應用於離線分析

```bash
# 步驟 1：從 OpenSearch 獲取數據並保存為 JSON
go run cmd/test-pagination/main.go \
  -output ./data/opensearch-responses \
  -max-batches 10

# 步驟 2：稍後從本地文件進行分析
go run cmd/analyzer/main.go \
  -input ./data/opensearch-responses \
  -output ./my-reports
```

### 批量分析多個索引

```bash
go run cmd/analyzer/main.go \
  -fetch \
  -time 24h \
  -keyword error \
  -indices "{{log-service}}-log*,{{log-service}}-log*" \
  -output ./reports/batch-analysis
```

