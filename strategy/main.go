// tail_20_2 策略实时选股入口
//
// 用法:
//
//	go run main.go                          # 默认日期 2026-07-10
//	go run main.go -date 2026-07-14         # 指定日期
//	go run main.go -skip-macd               # 跳过大盘MACD检查
//	go run main.go -debug                   # 调试模式
//	go run main.go -output json             # JSON 输出（供 Python 执行层调用）
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"stock-strategy/internal/provider"
	"stock-strategy/internal/riskcontrol"
	"stock-strategy/internal/strategy/tail_20_2"
)

const tdxInstallDir = "D:/Programs/tdx"

var (
	targetDate    time.Time
	debugMode     bool
	skipMACD      bool
	targetDateStr string
	outputMode    string // "text" 或 "json"
)

func main() {
	flag.BoolVar(&debugMode, "debug", false, "调试模式")
	flag.BoolVar(&skipMACD, "skip-macd", false, "跳过大盘MACD检查")
	flag.StringVar(&targetDateStr, "date", "2026-07-10", "目标日期")
	flag.StringVar(&outputMode, "output", "text", "输出模式: text=控制台, json=结构化JSON")
	flag.Parse()

	targetDate, _ = time.Parse("2006-01-02", targetDateStr)
	log.SetFlags(log.Ltime | log.Lshortfile)

	isJSON := outputMode == "json"

	if !isJSON {
		fmt.Println("=== tail_20_2 实时选股系统 ===")
		fmt.Printf("目标日期: %s\n", targetDateStr)
		if skipMACD {
			fmt.Println("MACD检查: 跳过")
		}
		fmt.Println()
	}

	startTime := time.Now()

	// 初始化数据提供者
	p := provider.NewProvider(tdxInstallDir)
	defer p.Close()

	// 获取全市场股票并过滤
	if !isJSON {
		fmt.Println("正在获取全市场股票列表...")
	}
	allCodes, err := tail_20_2.GetAllStockCodes(p)
	if err != nil {
		maybeJSONError(isJSON, "获取股票列表失败", err)
		log.Fatalf("获取股票列表失败: %v", err)
	}

	// MACD检查
	macdOK := true
	if !skipMACD {
		if !isJSON {
			fmt.Println("\n正在检查大盘60分钟MACD...")
		}
		marketUp, err := tail_20_2.CheckMACDLive(p)
		if err != nil {
			if !isJSON {
				fmt.Printf("⚠ 获取大盘MACD失败: %v (忽略检查)\n", err)
			}
			marketUp = true
		}
		macdOK = marketUp
		if !isJSON {
			if marketUp {
				fmt.Println("✓ 大盘60分钟MACD翻红，允许入场")
			} else {
				fmt.Println("✗ 大盘60分钟MACD为绿，禁止入场")
				return
			}
		}
	}

	if !macdOK && isJSON {
		out := BuildBridgeOutput(targetDateStr, false, nil)
		out.OutputJSON()
		return
	}

	// 实时选股
	if !isJSON {
		fmt.Println("\n正在执行选股策略(逐只获取数据，较慢)...")
	}
	candidates := tail_20_2.SelectCandidatesLive(allCodes, p, targetDate, 100)

	elapsed := time.Since(startTime)
	if !isJSON {
		fmt.Printf("\n=== 选股完成 (耗时 %v) ===\n", elapsed)
		fmt.Printf("总扫描: %d 只, 符合条件: %d 只\n", len(allCodes), len(candidates))
	}

	if len(candidates) == 0 {
		if !isJSON {
			fmt.Println("\n当前没有符合策略条件的股票。")
		} else {
			out := BuildBridgeOutput(targetDateStr, macdOK, nil)
			out.OutputJSON()
		}
		return
	}

	// 打印选股结果（仅 text 模式）
	if !isJSON {
		fmt.Println("\n=== 符合条件的股票列表 ===")
		fmt.Printf("%-4s %-8s %-10s %8s %-6s %s\n", "序号", "代码", "名称", "价格", "评分", "条件")
		for i, c := range candidates {
			fmt.Printf("%-4d %-8s %-10s %8.2f %-6.0f %v\n",
				i+1, c.Result.Code, c.Result.Name, c.Result.ClosePrice, c.Result.Score, c.Result.Reasons)
		}
	}

	// 风控筛选
	if !isJSON {
		fmt.Println("\n=== 风控筛选 ===")
	}
	inputs := tail_20_2.BuildRiskInputs(candidates, p, targetDate)
	summary := riskcontrol.ProcessAll(inputs)

	if isJSON {
		out := BuildBridgeOutput(targetDateStr, macdOK, summary)
		out.OutputJSON()
	} else {
		riskcontrol.PrintSummary(summary)
		riskcontrol.PrintCompactSummary(summary)
	}

	_ = debugMode
	_ = elapsed
}

// maybeJSONError 在 JSON 模式下输出错误 JSON 并退出
func maybeJSONError(isJSON bool, msg string, err error) {
	if !isJSON {
		return
	}
	out := struct {
		Error string `json:"error"`
	}{
		Error: msg + ": " + err.Error(),
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
	os.Exit(1)
}

func init() {
	f, err := os.OpenFile("strategy.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(f)
	}
}
