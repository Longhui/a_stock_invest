// Package tail_20_2 隔夜尾盘买入策略（20%仓位 × 4只 × 2%止损）
//
// 策略名称: tail_20_2
// 全称:     Tail Buy 20% Position 2% Stop-Loss
// 类型:     T日尾盘买入 → T+1日卖出（隔夜持仓）
//
// 本包包含:
//   config.go   — 策略配置参数
//   types.go    — 策略数据类型的定义
//   market.go   — 市场数据加载（全市场股票、日K线、指数K线）
//   strategy.go — 核心决策逻辑（大盘MACD、选股、风控、卖出判断）
package tail_20_2

import "fmt"

// ============================================================
// 策略配置
// ============================================================

// Config tail_20_2 策略专属配置
type Config struct {
	InitCapital           float64 // 初始本金（元）
	PositionPct           float64 // 每只仓位比例（占当时总资产）
	MaxPositions          int     // 最大同时持仓数量
	StopLossPct           float64 // 止损比例（如0.02=-2%）
	FeeRate               float64 // 手续费率（如0.0001=万分之一）
	TakeProfitActivatePct float64 // 止盈激活涨幅（如0.02=+2%）
	TakeProfitRetracePct  float64 // 止盈回落比例（如0.985=回落1.5%）
	TDXDir                string  // 通达信安装目录
}

// DefaultConfig 返回 tail_20_2 默认配置（经历史回测验证）
func DefaultConfig(tdxDir string) Config {
	return Config{
		InitCapital:           1_000_000,
		PositionPct:           0.20,
		MaxPositions:          4,
		StopLossPct:           0.02,
		FeeRate:               0.0001,
		TakeProfitActivatePct: 0.02,
		TakeProfitRetracePct:  0.985,
		TDXDir:                tdxDir,
	}
}

func (c Config) String() string {
	return fmt.Sprintf("tail_20_2 仓位%.0f%%×%d只 止损%.0f%% 手续费%.04f%%",
		c.PositionPct*100, c.MaxPositions, c.StopLossPct*100, c.FeeRate*100)
}
