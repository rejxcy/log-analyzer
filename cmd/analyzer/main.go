package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"log-analyzer/internal/config"
	"log-analyzer/internal/pipeline"
)

func main() {
	// Only one parameter: time range
	timeRange := flag.String("time", "24h", "Time range for OpenSearch query (e.g., '1h', '24h', '7d')")
	flag.Parse()

	fmt.Println("🚀 啟動日誌分析管道")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		log.Fatalf("❌ 無法加載配置：%v", err)
	}

	// Create and run pipeline
	pipe := pipeline.NewPipeline(cfg)
	result, err := pipe.Run(*timeRange)
	if err != nil {
		log.Fatalf("❌ 管道執行失敗：%v", err)
	}

	if len(result.RawLogs) == 0 {
		os.Exit(0)
	}

	// Print summary
	printSummary(result, cfg)
}

// printSummary prints a summary of the pipeline execution
func printSummary(result *pipeline.PipelineResult, cfg *config.Config) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✨ 完整管道分析成功完成！")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\n📊 最終統計資訊：\n")
	fmt.Printf("   輸入日誌數：%d\n", len(result.RawLogs))
	fmt.Printf("   解析日誌數：%d\n", len(result.ParsedLogs))
	fmt.Printf("   錯誤群組數：%d\n", len(result.ErrorGroups))
	fmt.Printf("   受影響服務數：%d\n", len(result.AggregationResult.ServiceStats))
	fmt.Printf("   處理時間：%dms\n\n", result.AggregationResult.ProcessingTime.Milliseconds())

	fmt.Printf("📁 輸出檔案：\n")
	fmt.Printf("   報告目錄：%s\n", cfg.Output.ReportDir)
	fmt.Printf("   分析 JSON：%s/analysis_*.json\n\n", cfg.Output.ReportDir)

	fmt.Println("✅ 您現在可以查看生成的報告和分析 JSON 檔案！")
}
