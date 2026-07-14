package tail_20_2

import (
	"time"

	"stock-strategy/internal/selector"
)

// ============================================================
// K线数据
// ============================================================

// Kline 简化K线（用于内存缓存）
type Kline struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// StockDataCache 单只股票的日K线缓存
type StockDataCache struct {
	Name            string
	Klines          []Kline
	CirculateShares float64
}

// ============================================================
// 选股候选结果
// ============================================================

// CandidateResult 选股通过 + 完整技术上下文
// 由 SelectCandidates 产生，供 BuildRiskInputs 使用
type CandidateResult struct {
	Result *selector.StockResult
	Ctx    *selector.Context
	Code   string
}

// ============================================================
// 卖出决策
// ============================================================

// SellDecision 单只股票的卖出决策
type SellDecision struct {
	ShouldSell bool
	SellPrice  float64
	Reason     string // "止损" / "止盈" / "收盘卖出"
}
