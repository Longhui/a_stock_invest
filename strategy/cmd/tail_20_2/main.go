// tail_20_2 策略回测入口
//
// 运行:
//
//	go run cmd/tail_20_2/main.go
//
// 修改回测区间直接改下面的 StartDate / EndDate 即可。
package main

import (
	"fmt"
	"time"

	"stock-strategy/internal/backtest"
	"stock-strategy/internal/strategy/tail_20_2"
)

func main() {
	// =========================================
	// tail_20_2 策略配置
	// =========================================
	scfg := tail_20_2.DefaultConfig("D:/Programs/tdx")

	// 回测区间（按需修改）
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.Local)

	btCfg := backtest.BacktestConfig{
		StartDate:    startDate,
		EndDate:      endDate,
		InitCapital:  scfg.InitCapital,
		FeeRate:      scfg.FeeRate,
		PositionPct:  scfg.PositionPct,
		MaxPositions: scfg.MaxPositions,
		StopLossPct:  scfg.StopLossPct,
		TDXDir:       scfg.TDXDir,
	}

	// 运行回测
	engine := backtest.NewEngine(btCfg)
	engine.SetDebug(false)
	summary := engine.Run()

	// 输出报告
	summary.PrintReport()
	summary.PrintMonthlyReport()
	summary.PrintCompactSummary()

	fmt.Printf("\n策略: %s | 区间: %s ~ %s 回测完成。\n",
		scfg.String(),
		btCfg.StartDate.Format("2006-01-02"),
		btCfg.EndDate.Format("2006-01-02"))
}
