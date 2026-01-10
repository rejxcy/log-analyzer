# Log Analyzer

🔍 OpenSearch 日誌分析工具，自動識別錯誤模式、已知問題匹配與生成工程友善報告。

## 快速開始

```bash
go run cmd/analyzer/main.go -time 24h
```

## 📋 專案結構

```
log-analyzer/
├── cmd/
│   └── analyzer/
│       └── main.go              # 主 CLI 程式
├── internal/
│   ├── config/                  # 配置與已知問題系統
│   │   ├── config.go            # OpenSearch 連接配置
│   │   └── known_issues.go      # 已知問題匹配引擎
│   ├── fetcher/                 # 數據獲取
│   │   └── opensearch.go        # OpenSearch API 調用
│   ├── preprocessor/            # 數據預處理
│   │   ├── processor.go         # JSON 解析、服務提取
│   │   └── service_extractor.go # 服務名稱提取
│   ├── normalizer/              # 正規化與去重
│   │   └── normalizer.go        # Error Fingerprint 計算
│   ├── aggregator/              # 數據聚合
│   │   └── aggregator.go        # 時間統計、服務分類
│   └── reporter/                # 報告生成
│       └── reporter.go          # 每個服務獨立的 Markdown 報告
├── pkg/
│   └── models/                  # 數據結構定義
│       └── types.go
├── configs/
│   ├── config.yaml              # OpenSearch 連接配置
│   └── known-issues/            # 已知問題系統文檔
│       └── README.md
├── reports/                     # 報告輸出目錄
├── ARCHITECTURE.md              # 系統架構設計文檔
└── go.mod
```

## 🎯 核心功能

- ✅ **OpenSearch 集成** - 直接從 OpenSearch Dashboards API 獲取日誌
- ✅ **自動去重** - SHA256 Fingerprint 計算，識別重複錯誤模式
- ✅ **已知問題匹配** - 10 個預定義的已知問題，自動識別與分類
- ✅ **工程友善報告** - Daily Verdict + Top 5 Problems + Secondary Issues
- ✅ **完全中文本地化** - 繁體中文界面與報告輸出
- ✅ **獨立服務報告** - 每個服務生成單獨報告，清晰易讀
- ✅ **JSON 導出** - 詳細分析結果 JSON 供進階分析

## ⚙️ 配置

編輯 `configs/config.yaml`，配置 OpenSearch 連接信息。所有其他配置都有默認值。

## 📊 輸出文件

運行分析後在 `./reports` 生成：

```
reports/
├── 2026-01-11_pp-slot-api_02-16-12.md      # API 服務報告
├── 2026-01-11_pp-slot-rpc_02-16-12.md      # RPC 服務報告  
├── 2026-01-11_pp-slot-math_02-16-12.md     # Math 服務報告
└── analysis_2026-01-11_02-16-12.json       # 完整分析 JSON
```

## � 支持的時間範圍

```bash
go run cmd/analyzer/main.go -time 1h      # 過去 1 小時
go run cmd/analyzer/main.go -time 24h     # 過去 24 小時
go run cmd/analyzer/main.go -time 7d      # 過去 7 天
go run cmd/analyzer/main.go -time 48h     # 過去 48 小時
```

## 📚 文檔

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - 系統架構設計
- **[configs/known-issues/README.md](./configs/known-issues/README.md)** - 已知問題系統

## 🚀 系統工作流程

```
OpenSearch 數據獲取（時間窗口分割）
   ↓
預處理（JSON 解析、服務提取）
   ↓
正規化（Error Fingerprint、去重）
   ↓
聚合（時間統計、服務分類）
   ↓
分析（已知問題匹配、嚴重級別評估）
   ↓
報告生成（每服務獨立 Markdown + JSON）
```

## 📝 已知問題系統

系統預置了 10 個常見遊戲服務錯誤的已知問題模式，會在報告中自動標示。

詳見 [configs/known-issues/README.md](./configs/known-issues/README.md)
