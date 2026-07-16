package paper

import (
	"fmt"
	"time"
)

// PaperConfig 模拟盘配置
type PaperConfig struct {
	StateFile    string  // 状态文件路径
	TDXDir       string  // 通达信目录
	InitCapital  float64 // 初始本金(元)
	PositionPct  float64 // 每只仓位比例(0.20=20%)
	MaxPositions int     // 最大持仓数
	StopLossPct  float64 // 止损比例(0.02=2%)
	FeeRate      float64 // 手续费率(0.0001=万分之一)
}

// DefaultPaperConfig 返回默认模拟盘配置
func DefaultPaperConfig(tdxDir string) PaperConfig {
	return PaperConfig{
		StateFile:    "paper_state.json",
		TDXDir:       tdxDir,
		InitCapital:  1_000_000,
		PositionPct:  0.20,
		MaxPositions: 4,
		StopLossPct:  0.02,
		FeeRate:      0.0001,
	}
}

func (c PaperConfig) String() string {
	return fmt.Sprintf("仓位%.0f%%×%d只 止损%.0f%% 手续费%.04f%%",
		c.PositionPct*100, c.MaxPositions, c.StopLossPct*100, c.FeeRate*100)
}

// StoredPosition 持久化持仓
type StoredPosition struct {
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	BuyDate  time.Time `json:"buy_date"`
	BuyPrice float64   `json:"buy_price"`
	Shares   int       `json:"shares"`
}

// StoredTrade 持久化成交记录
type StoredTrade struct {
	Date   time.Time `json:"date"`
	Code   string    `json:"code"`
	Name   string    `json:"name"`
	Dir    string    `json:"dir"`
	Price  float64   `json:"price"`
	Shares int       `json:"shares"`
	Amount float64   `json:"amount"`
	Fee    float64   `json:"fee"`
	Reason string    `json:"reason"`
}

// DailyRecord 每日净值记录
type DailyRecord struct {
	Date          time.Time `json:"date"`
	Cash          float64   `json:"cash"`
	PositionValue float64   `json:"position_value"`
	TotalValue    float64   `json:"total_value"`
	PositionCount int       `json:"position_count"`
}

// PaperState 模拟盘完整状态
type PaperState struct {
	Config      PaperConfig      `json:"config"`
	Cash        float64          `json:"cash"`
	Positions   []StoredPosition `json:"positions"`
	Trades      []StoredTrade    `json:"trades"`
	DailyValues []DailyRecord    `json:"daily_values"`
	TotalFee    float64          `json:"total_fee"`
	WinCount    int              `json:"win_count"`
	LoseCount   int              `json:"lose_count"`
	PeakCapital float64          `json:"peak_capital"`
	MaxDrawdown float64          `json:"max_drawdown"`
}

// isTradingDay 简单交易日判断
func isTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w >= time.Monday && w <= time.Friday
}

// formatAmount 格式化金额(万/亿)
func formatAmount(v float64) string {
	if v >= 1e8 {
		return fmt.Sprintf("%.2f亿", v/1e8)
	}
	if v >= 1e4 {
		return fmt.Sprintf("%.2f万", v/1e4)
	}
	return fmt.Sprintf("%.2f", v)
}
