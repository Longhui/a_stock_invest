package backtest

import (
	"fmt"
	"time"
)

// PrintReport 打印回测报告
func (s *BacktestSummary) PrintReport() {
	fmt.Println("\n========================================")
	fmt.Println("  回测报告")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Printf("区间: %s ~ %s\n",
		s.Config.StartDate.Format("2006-01-02"),
		s.Config.EndDate.Format("2006-01-02"))
	fmt.Printf("初始资金: %.0f\n", s.Config.InitCapital)

	// 期末资金
	finalValue := s.finalValue()
	fmt.Printf("期末资金: %.0f", finalValue)
	if s.TotalReturn >= 0 {
		fmt.Printf(" (+%.2f%%)\n", s.TotalReturn)
	} else {
		fmt.Printf(" (%.2f%%)\n", s.TotalReturn)
	}

	fmt.Printf("总手续费: %.2f\n", s.TotalFee)
	fmt.Printf("最大回撤: %.2f%%\n", s.MaxDrawdown)
	fmt.Println()

	// 交易统计
	fmt.Printf("交易次数: %d (买入%d次, 卖出%d次)\n", s.TradeCount, s.BuyCount, s.SellCount)
	fmt.Printf("胜率: %.1f%% (止盈%d次, 止损%d次)\n", s.WinRate, s.WinCount, s.LoseCount)
	fmt.Println()

	// 逐笔交易明细
	fmt.Println("--- 逐笔交易明细 ---")
	fmt.Printf("%-8s %-4s %-8s %-10s %8s %6s %10s %8s %s\n",
		"日期", "方向", "代码", "名称", "价格", "股数", "成交额", "手续费", "原因")
	for _, t := range s.Trades {
		fmt.Printf("%-8s %-4s %-8s %-10s %8.2f %6d %10.2f %8.2f %s\n",
			t.Date.Format("01-02"), t.Dir, t.Code, t.Name,
			t.Price, t.Shares, t.Amount, t.Fee, t.Reason)
	}
	fmt.Println()

	// 每日净值
	fmt.Println("--- 每日净值 ---")
	fmt.Printf("%-8s %12s %10s %12s %5s\n", "日期", "总资产", "现金", "持仓市值", "持仓")
	for _, dv := range s.DailyValues {
		fmt.Printf("%-8s %12.0f %10.0f %12.0f %5d\n",
			dv.Date.Format("01-02"), dv.TotalValue, dv.Cash, dv.PositionValue, dv.PositionCount)
	}
	fmt.Println()
}

// finalValue 获取最终总资产
func (s *BacktestSummary) finalValue() float64 {
	if len(s.DailyValues) > 0 {
		return s.DailyValues[len(s.DailyValues)-1].TotalValue
	}
	return s.Config.InitCapital
}

// // MaxDrawdown 计算最大回撤
// func (s *BacktestSummary) MaxDrawdown() float64 {
// 	if len(s.DailyValues) == 0 {
// 		return 0
// 	}
// 	peak := s.DailyValues[0].TotalValue
// 	maxDD := 0.0
// 	for _, dv := range s.DailyValues {
// 		if dv.TotalValue > peak {
// 			peak = dv.TotalValue
// 		}
// 		dd := (peak - dv.TotalValue) / peak * 100
// 		if dd > maxDD {
// 			maxDD = dd
// 		}
// 	}
// 	return maxDD
// }

// // WinRate 计算胜率
// func (s *BacktestSummary) calcWinRate() float64 {
// 	total := s.WinCount + s.LoseCount
// 	if total == 0 {
// 		return 0
// 	}
// 	return float64(s.WinCount) / float64(total) * 100
// }

// PrintCompactSummary 打印简洁版收益曲线(纯文本,用于快速查看)
func (s *BacktestSummary) PrintCompactSummary() {
	fmt.Println("\n=== 收益摘要 ===")
	fmt.Printf("区间: %s ~ %s\n",
		s.Config.StartDate.Format("01-02"),
		s.Config.EndDate.Format("01-02"))
	fmt.Printf("初始: %.0f → 期末: %.0f (%+.2f%%)\n",
		s.Config.InitCapital, s.finalValue(), s.TotalReturn)
	fmt.Printf("手续费: %.0f | 最大回撤: %.2f%% | 交易: %d笔 | 胜率: %.1f%%\n",
		s.TotalFee, s.MaxDrawdown, s.TradeCount, s.WinRate)

	// 简易净值曲线
	fmt.Println("\n--- 净值曲线(每5日) ---")
	for i, dv := range s.DailyValues {
		if i%5 == 0 || i == len(s.DailyValues)-1 {
			barLen := int(dv.TotalValue / s.Config.InitCapital * 50)
			if barLen < 0 {
				barLen = 0
			}
			bar := ""
			for j := 0; j < barLen && j < 50; j++ {
				bar += "█"
			}
			returnPct := (dv.TotalValue - s.Config.InitCapital) / s.Config.InitCapital * 100
			fmt.Printf("%s %10.0f %+6.2f%% %s\n",
				dv.Date.Format("01-02"), dv.TotalValue, returnPct, bar)
		}
	}
	fmt.Println()
}

// PrintMonthlyReport 打印月度收益统计
func (s *BacktestSummary) PrintMonthlyReport() {
	if len(s.DailyValues) == 0 {
		return
	}

	// 统计每月交易笔数
	tradesByMonth := make(map[string]int)
	for _, t := range s.Trades {
		key := fmt.Sprintf("%d-%02d", t.Date.Year(), t.Date.Month())
		tradesByMonth[key]++
	}

	// 按月分组 DailyValues
	type monthInfo struct {
		startVal float64
		endVal   float64
		days     int
		label    string
		key      string
	}
	var months []monthInfo

	startIdx := 0
	for startIdx < len(s.DailyValues) {
		dv := s.DailyValues[startIdx]
		yr, mo := dv.Date.Year(), dv.Date.Month()

		// 找到同月的最后一天
		endIdx := startIdx
		for endIdx+1 < len(s.DailyValues) &&
			s.DailyValues[endIdx+1].Date.Year() == yr &&
			s.DailyValues[endIdx+1].Date.Month() == mo {
			endIdx++
		}

		startVal := s.DailyValues[startIdx].TotalValue
		endVal := s.DailyValues[endIdx].TotalValue
		days := endIdx - startIdx + 1
		key := fmt.Sprintf("%d-%02d", yr, mo)

		months = append(months, monthInfo{
			startVal: startVal,
			endVal:   endVal,
			days:     days,
			label:    dv.Date.Format("2006-01"),
			key:      key,
		})
		startIdx = endIdx + 1
	}

	fmt.Println("\n--- 月度收益统计 ---")
	fmt.Printf("%-8s %8s %12s %10s %8s\n", "月份", "交易天数", "月初资产", "月收益%", "交易笔数")

	for _, m := range months {
		ret := (m.endVal - m.startVal) / m.startVal * 100
		sign := "+"
		if ret < 0 {
			sign = ""
		}
		fmt.Printf("%-8s %8d %12.0f %9s%.2f%% %8d\n",
			m.label, m.days, m.startVal, sign, ret, tradesByMonth[m.key])
	}

	// 汇总
	initCap := s.Config.InitCapital
	finalVal := s.finalValue()
	totalRet := (finalVal - initCap) / initCap * 100
	sign := "+"
	if totalRet < 0 {
		sign = ""
	}
	fmt.Printf("%-8s %8s %12.0f %9s%.2f%% %8d\n",
		"合计", "", initCap, sign, totalRet, len(s.Trades))
	fmt.Println()
}

// init 确保time包被使用(避免空导入)
var _ = time.Now
