// 模拟盘交易系统
//
// 用法:
//
// # 单次操作
//
//	go run cmd/paper/main.go sell                 # 检查止盈止损，触发的卖出 (手动)
//	go run cmd/paper/main.go buy                  # 选股买入 (14:50)
//	go run cmd/paper/main.go settle               # 选股买入 + 记账 (14:50, 卖出由monitor处理)
//	go run cmd/paper/main.go run                  # 等价于 settle
//
// # 持续监控（推荐）
//
//	go run cmd/paper/main.go monitor              # 09:35~14:50持续监控，自动清仓+买入
//
// # 信息查询
//
//	go run cmd/paper/main.go status               # 当前持仓状态
//	go run cmd/paper/main.go list                 # 成交记录
//	go run cmd/paper/main.go nav                  # 每日净值
//
// # 管理
//
//	go run cmd/paper/main.go reset                # 重置模拟盘
//
// # 指定日期
//
//	go run cmd/paper/main.go settle -date 2026-07-15
//	go run cmd/paper/main.go buy    -date 2026-07-15
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"stock-strategy/internal/paper"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		printUsage()
		return
	}

	cfg := paper.DefaultPaperConfig("D:/Programs/tdx")
	cmd := args[0]

	switch cmd {
	case "run":
		withDate(args[1:], cfg, (*paper.Engine).Run)

	case "sell":
		withDate(args[1:], cfg, (*paper.Engine).RunSell)

	case "buy":
		withDate(args[1:], cfg, (*paper.Engine).RunBuy)

	case "settle":
		withDate(args[1:], cfg, (*paper.Engine).RunSettle)

	case "monitor":
		// monitor 不支持 -date (实时监控)
		eng := paper.NewEngine(cfg)
		eng.RunMonitor()

	case "status":
		eng := paper.NewEngine(cfg)
		eng.PrintStatus()

	case "list":
		eng := paper.NewEngine(cfg)
		eng.PrintTrades()

	case "nav":
		eng := paper.NewEngine(cfg)
		eng.PrintDailyValues()

	case "reset":
		fmt.Print("确认重置模拟盘? 输入 YES 确认: ")
		var s string
		fmt.Scanln(&s)
		if s != "YES" {
			fmt.Println("取消")
			return
		}
		eng := paper.NewEngine(cfg)
		eng.Reset()

	default:
		fmt.Printf("未知命令: %s\n", cmd)
		printUsage()
	}
}

// withDate 解析 -date 参数，创建 Engine 并执行方法
func withDate(args []string, cfg paper.PaperConfig, fn func(*paper.Engine)) {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	dateStr := fs.String("date", "", "指定日期(2006-01-02，默认今天)")
	_ = fs.Parse(args)

	eng := paper.NewEngine(cfg)
	if *dateStr != "" {
		d, err := time.Parse("2006-01-02", *dateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "日期格式错误: %v\n", err)
			os.Exit(1)
		}
		eng.SetDate(d)
	}
	fn(eng)
}

func printUsage() {
	fmt.Println("模拟盘交易系统")
	fmt.Println()
	fmt.Println("单次操作:")
	fmt.Println("  sell       止盈止损检查 (手动)")
	fmt.Println("  buy        选股买入 (14:50)")
	fmt.Println("  settle     选股买入+记账 (14:50, 卖出由monitor处理)")
	fmt.Println("  run        等价于 settle")
	fmt.Println()
	fmt.Println("持续监控(推荐):")
	fmt.Println("  monitor    09:35~14:50 自动监控卖出, 尾盘自动清算买入")
	fmt.Println()
	fmt.Println("信息查询:")
	fmt.Println("  status     当前持仓状态")
	fmt.Println("  list       成交记录")
	fmt.Println("  nav        每日净值")
	fmt.Println()
	fmt.Println("管理:")
	fmt.Println("  reset      重置模拟盘")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  go run cmd/paper/main.go monitor    # 全自动: 盯盘卖出+尾盘买入")
	fmt.Println("  go run cmd/paper/main.go settle     # 仅买入 (14:50)")
	fmt.Println("  go run cmd/paper/main.go sell       # 手动检查止盈止损")
	fmt.Println("  go run cmd/paper/main.go settle -date 2026-07-15")
}
