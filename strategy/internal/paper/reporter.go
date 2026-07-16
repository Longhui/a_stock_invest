package paper

import (
	"fmt"
	"log"
	"os"
)

// ============================================================
// PrintStatus — 当前持仓与综合状态
// ============================================================

// PrintStatus 输出模拟盘综合状态
func (e *Engine) PrintStatus() {
	var err error
	e.state, err = loadState(e.cfg)
	if err != nil {
		log.Fatalf("加载状态失败: %v", err)
	}

	fmt.Println("=== 模拟盘状态 ===")

	// 基本信息
	initCap := e.cfg.InitCapital
	total := e.currentTotal()
	returnPct := (total - initCap) / initCap * 100
	runningDays := len(e.state.DailyValues)

	fmt.Printf("运行天数: %d\n", runningDays)
	fmt.Printf("初始本金: %s\n", formatAmount(initCap))
	fmt.Printf("当前总资产: %s", formatAmount(total))
	if returnPct >= 0 {
		fmt.Printf(" (+%.2f%%)\n", returnPct)
	} else {
		fmt.Printf(" (%.2f%%)\n", returnPct)
	}
	fmt.Printf("最大回撤: %.2f%%\n", e.state.MaxDrawdown)
	fmt.Printf("总手续费: %s\n", formatAmount(e.state.TotalFee))
	fmt.Println()

	// 交易统计
	totalTrades := len(e.state.Trades)
	buyCount, sellCount := 0, 0
	for _, t := range e.state.Trades {
		switch t.Dir {
		case "买入":
			buyCount++
		case "卖出":
			sellCount++
		}
	}
	winRate := 0.0
	if e.state.WinCount+e.state.LoseCount > 0 {
		winRate = float64(e.state.WinCount) / float64(e.state.WinCount+e.state.LoseCount) * 100
	}
	fmt.Printf("交易次数: %d (买入%d次, 卖出%d次)\n", totalTrades, buyCount, sellCount)
	fmt.Printf("胜率: %.1f%% (胜%d次 / 负%d次)\n", winRate, e.state.WinCount, e.state.LoseCount)
	fmt.Println()

	// 当前持仓
	if len(e.state.Positions) > 0 {
		fmt.Println("--- 当前持仓 ---")
		fmt.Printf("%-8s %-10s %8s %6s %8s %10s\n", "代码", "名称", "买入价", "股数", "成本", "日期")
		for _, p := range e.state.Positions {
			cost := p.BuyPrice * float64(p.Shares)
			fmt.Printf("%-8s %-10s %8.2f %6d %8s %10s\n",
				p.Code, p.Name, p.BuyPrice, p.Shares,
				formatAmount(cost), p.BuyDate.Format("01-02"))
		}
		fmt.Printf("\n现金: %s | 持仓成本: %s | 总资产: %s\n",
			formatAmount(e.state.Cash),
			formatAmount(e.positionCost()),
			formatAmount(total))
	} else {
		fmt.Println("当前无持仓")
		fmt.Printf("可用现金: %s\n", formatAmount(e.state.Cash))
	}
	fmt.Println()
}

// ============================================================
// PrintTrades — 历史成交明细
// ============================================================

// PrintTrades 输出历史成交记录
func (e *Engine) PrintTrades() {
	var err error
	e.state, err = loadState(e.cfg)
	if err != nil {
		log.Fatalf("加载状态失败: %v", err)
	}

	fmt.Println("=== 成交记录 ===")
	if len(e.state.Trades) == 0 {
		fmt.Println("暂无交易记录")
		return
	}

	fmt.Printf("%-8s %-4s %-8s %-10s %8s %6s %10s %8s %s\n",
		"日期", "方向", "代码", "名称", "价格", "股数", "成交额", "手续费", "原因")
	for _, t := range e.state.Trades {
		fmt.Printf("%-8s %-4s %-8s %-10s %8.2f %6d %10.2f %8.2f %s\n",
			t.Date.Format("01-02"), t.Dir, t.Code, t.Name,
			t.Price, t.Shares, t.Amount, t.Fee, t.Reason)
	}
	fmt.Println()
}

// ============================================================
// PrintDailyValues — 每日净值曲线
// ============================================================

// PrintDailyValues 输出每日净值记录
func (e *Engine) PrintDailyValues() {
	var err error
	e.state, err = loadState(e.cfg)
	if err != nil {
		log.Fatalf("加载状态失败: %v", err)
	}

	fmt.Println("--- 每日净值 ---")
	if len(e.state.DailyValues) == 0 {
		fmt.Println("暂无数据")
		return
	}

	initCap := e.cfg.InitCapital
	fmt.Printf("%-8s %12s %10s %12s %5s %8s\n", "日期", "总资产", "现金", "持仓市值", "持仓", "收益%")
	for _, dv := range e.state.DailyValues {
		ret := (dv.TotalValue - initCap) / initCap * 100
		sign := "+"
		if ret < 0 {
			sign = ""
		}
		fmt.Printf("%-8s %12.0f %10.0f %12.0f %5d %7s%.2f%%\n",
			dv.Date.Format("01-02"), dv.TotalValue, dv.Cash,
			dv.PositionValue, dv.PositionCount, sign, ret)
	}
	fmt.Println()
}

// ============================================================
// formatSummary — 单日运行汇总(简短一行)
// ============================================================

func (e *Engine) formatSummary() string {
	initCap := e.cfg.InitCapital
	total := e.currentTotal()
	retPct := (total - initCap) / initCap * 100
	pos := len(e.state.Positions)

	sign := "+"
	if retPct < 0 {
		sign = ""
	}

	// 交易汇总
	var sellCount, buyCount int
	for i := len(e.state.Trades) - 1; i >= 0; i-- {
		t := e.state.Trades[i]
		if t.Dir == "买入" {
			buyCount++
		} else {
			sellCount++
		}
		if buyCount > 0 && sellCount > 0 {
			break
		}
	}

	return fmt.Sprintf("=== 汇总: 总资产 %s (%s%.2f%%) | 持仓%d只 | 当日买入%d 卖出%d ===\n",
		formatAmount(total), sign, retPct, pos, buyCount, sellCount)
}

// ============================================================
// Reset — 清空状态
// ============================================================

// Reset 删除状态文件，重置模拟盘
func (e *Engine) Reset() {
	if err := os.Remove(e.cfg.StateFile); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("状态文件不存在，无需重置")
		} else {
			log.Fatalf("删除状态文件失败: %v", err)
		}
		return
	}
	fmt.Printf("已重置模拟盘 (%s)\n", e.cfg.StateFile)
}
