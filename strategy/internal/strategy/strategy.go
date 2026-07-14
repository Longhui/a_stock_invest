// Package strategy 交易策略定义总览
//
// 本包定义策略框架，具体策略实现位于子包中:
//
//	tail_20_2/  — 隔夜尾盘买入策略（20%仓位 × 4只 × 2%止损）
//
// 策略公共特性:
//   - T日尾盘买入 → T+1日卖出（隔夜持仓）
//   - 上证指数60分钟MACD红柱过滤
//   - 全市场选股 + 五步风控流水线
//   - 卖出: -2%止损 / +2%止盈(回落1.5%确认) / 收盘兜底
package strategy

import "time"

// 时间线常量
const (
	MacdCheckHour   = 9  // 大盘MACD检查 09:55
	MacdCheckMinute = 55
	BuyHour         = 14 // 选股买入 14:50
	BuyMinute       = 50
)

// BoardFilterType 板块过滤类型
type BoardFilterType int

const (
	BoardFilterAll    BoardFilterType = iota // 沪深主板 + 创业板 + 科创板
	BoardFilterMain                          // 仅沪深主板
	BoardFilterChiNext                       // 仅创业板 + 科创板
)

// 大盘指数参数
const (
	IndexCode        = "000001" // 上证指数代码
	Min60MinBars     = 30       // 最少需要的60分钟K线数
	MacdFastPeriod   = 12       // MACD快线周期
	MacdSlowPeriod   = 26       // MACD慢线周期
	MacdSignalPeriod = 9        // MACD信号线周期
)

// 交易日判定（周一到周五）
func IsTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w >= time.Monday && w <= time.Friday
}
