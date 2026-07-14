// 通用回测入口 — 直接使用 tail_20_2 策略
//
// 运行:
//
//	go run cmd/backtest/main.go
//
// 策略专属入口（推荐）:
//
//	go run cmd/tail_20_2/main.go
package main

import (
	"fmt"
	"time"

	"stock-strategy/internal/backtest"
	"stock-strategy/internal/strategy/tail_20_2"
)

func main() {
	scfg := tail_20_2.DefaultConfig("D:/Programs/tdx")

	btCfg := backtest.BacktestConfig{
		StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local),
		EndDate:      time.Date(2024, 12, 31, 0, 0, 0, 0, time.Local),
		InitCapital:  scfg.InitCapital,
		FeeRate:      scfg.FeeRate,
		PositionPct:  scfg.PositionPct,
		MaxPositions: scfg.MaxPositions,
		StopLossPct:  scfg.StopLossPct,
		TDXDir:       scfg.TDXDir,
	}

	engine := backtest.NewEngine(btCfg)
	engine.SetDebug(false)
	summary := engine.Run()

	summary.PrintReport()
	summary.PrintMonthlyReport()
	summary.PrintCompactSummary()

	fmt.Printf("\n策略: %s | 区间: %s ~ %s 回测完成。\n",
		scfg.String(),
		btCfg.StartDate.Format("2006-01-02"),
		btCfg.EndDate.Format("2006-01-02"))
}
