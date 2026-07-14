package backtest

import (
	"fmt"
	"time"
)

// BacktestConfig 回测配置
type BacktestConfig struct {
	StartDate    time.Time // 回测起始日期
	EndDate      time.Time // 回测结束日期
	InitCapital  float64   // 初始本金(元)
	FeeRate      float64   // 手续费率(万分之1=0.0001)
	PositionPct  float64   // 每只仓位比例(0.20=20%)
	MaxPositions int       // 最大持仓数(4)
	StopLossPct  float64   // 止损比例(0.02=2%)
	TDXDir       string    // 通达信安装目录
}

func (c *BacktestConfig) String() string {
	return fmt.Sprintf("区间:%s~%s 本金:%.0f 手续费:%.04f%% 每只:%.0f%% 最多:%d只 止损:%.0f%%",
		c.StartDate.Format("2006-01-02"),
		c.EndDate.Format("2006-01-02"),
		c.InitCapital,
		c.FeeRate*100,
		c.PositionPct*100,
		c.MaxPositions,
		c.StopLossPct*100)
}

// Position 持仓记录
type Position struct {
	Code     string
	Name     string
	BuyDate  time.Time
	BuyPrice float64
	Shares   int
}

// Trade 成交记录
type Trade struct {
	Date   time.Time
	Code   string
	Name   string
	Dir    string  // "买入" / "卖出"
	Price  float64
	Shares int
	Amount float64
	Fee    float64
	Reason string // 策略买入 / 止盈 / 止损 / 收盘卖出
}

// DailyValue 每日净值
type DailyValue struct {
	Date          time.Time
	Cash          float64
	PositionValue float64
	TotalValue    float64
	PositionCount int
}

// BacktestSummary 回测结果汇总
type BacktestSummary struct {
	Config      BacktestConfig
	Trades      []Trade
	DailyValues []DailyValue

	TotalReturn float64 // 总收益率
	MaxDrawdown float64 // 最大回撤
	TotalFee    float64 // 总手续费

	TradeCount int
	BuyCount   int
	SellCount  int
	WinCount   int // 止盈次数
	LoseCount  int // 止损次数
	WinRate    float64
}

// ============================================================
// 辅助函数
// ============================================================

// isTradingDay 简单判断交易日(周一到周五)
func isTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w >= time.Monday && w <= time.Friday
}

// countTradingDays 统计区间内交易日数
func countTradingDays(start, end time.Time) int {
	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if isTradingDay(d) {
			count++
		}
	}
	return count
}

// formatWan 格式化金额（万/亿）
func formatWan(v float64) string {
	if v >= 1e8 {
		return fmt.Sprintf("%.2f亿", v/1e8)
	}
	return fmt.Sprintf("%.2f万", v/1e4)
}
