package riskcontrol

import (
	"fmt"
	"strings"
)

// PrintSummary 结论先行输出
// Part 1: 总排名结论(最先输出)
// Part 2: 按🟢🟡🔴分组明细
func PrintSummary(s *RiskSummary) {
	if s == nil || len(s.Results) == 0 {
		fmt.Println("\n=== AI风控结果 ===")
		fmt.Println("无合格股票")
		return
	}

	green, yellow, red := s.GreenCount(), s.YellowCount(), s.RedCount()

	// ===== Part 1: 总排名结论 =====
	fmt.Println("\n=== AI风控排序结果 ===")
	fmt.Printf("整体: 🟢%d 🟡%d 🔴%d | 共%d只\n", green, yellow, red, s.TotalStocks)

	// 整体建议
	overallSuggestion := calcOverallSuggestion(green, yellow, red, s.Results)
	fmt.Printf("建议: %s\n", overallSuggestion)
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("%-3s %-2s %-8s %-10s %8s  %-14s %-10s\n",
		"排名", "色", "代码", "名称", "价格", "隔夜风险", "爆发力")
	fmt.Println(strings.Repeat("─", 70))
	for i, r := range s.Results {
		riskLabel := fmt.Sprintf("低开%.0f", r.OvernightRisk)
		pulseLabel := fmt.Sprintf("脉冲%.0f", r.PulseScore)
		fmt.Printf("%-3d %-2s %-8s %-10s %8.2f  %-14s %-10s\n",
			i+1, r.Color.Tag(), r.Code, r.Name, r.ClosePrice, riskLabel, pulseLabel)
	}

	// ===== Part 2: 分组明细 =====
	fmt.Println()

	// 🟢组
	if green > 0 {
		fmt.Println("===== 🟢 看涨组 =====")
		for _, r := range s.Results {
			if r.Color != ColorGreen {
				continue
			}
			printStockDetail(r)
		}
		fmt.Println()
	}

	// 🟡组
	if yellow > 0 {
		fmt.Println("===== 🟡 震荡组 =====")
		for _, r := range s.Results {
			if r.Color != ColorYellow {
				continue
			}
			printStockDetail(r)
		}
		fmt.Println()
	}

	// 🔴组
	if red > 0 {
		fmt.Println("===== 🔴 易跌组 =====")
		for _, r := range s.Results {
			if r.Color != ColorRed {
				continue
			}
			printStockDetail(r)
		}
		fmt.Println()
	}
}

func printStockDetail(r *RiskResult) {
	capStr := formatMarketCap(r.MarketCap)
	flagsStr := "无"
	if len(r.RiskFlags) > 0 {
		flagsStr = strings.Join(r.RiskFlags, ", ")
	}

	sectorInfo := r.Sector + "/" + r.Topic
	if r.Topic == "" {
		sectorInfo = r.Sector
	}

	fmt.Printf("\n%s %s | %s | %s\n", r.Color.Tag(), r.Name, sectorInfo, capStr)
	fmt.Printf("评分: %.1f | 建议: %s\n", r.RiskScore, r.Suggestion)
	fmt.Printf("扣分: %s\n", flagsStr)
	if len(r.Warnings) > 0 {
		fmt.Printf("提示: %s\n", strings.Join(r.Warnings, "; "))
	}
	fmt.Println(strings.Repeat("─", 60))
}

// PrintCompactSummary 紧凑格式(适合终端快速浏览)
func PrintCompactSummary(s *RiskSummary) {
	if s == nil || len(s.Results) == 0 {
		return
	}

	fmt.Println("\n=== AI风控排序 ===")
	for i, r := range s.Results {
		fmt.Printf("%2d.%s %-8s %-8s 风险%.0f 脉冲%.0f  %s\n",
			i+1, r.Color.Tag(), r.Code, r.Name,
			r.OvernightRisk, r.PulseScore, r.Suggestion)
	}
}

// formatMarketCap 格式化流通市值
func formatMarketCap(cap float64) string {
	switch {
	case cap >= 1e10:
		return fmt.Sprintf("流通市值%.0f亿", cap/1e8)
	case cap >= 1e8:
		return fmt.Sprintf("流通市值%.1f亿", cap/1e8)
	default:
		return fmt.Sprintf("流通市值%.0f万", cap/1e4)
	}
}

// calcOverallSuggestion 根据分组统计给出整体建议
func calcOverallSuggestion(green, yellow, red int, results []*RiskResult) string {
	if green >= 3 && red == 0 {
		return "积极做多: 多只主线票低开风险低"
	}
	if green >= 2 && yellow <= 2 {
		return "主仓隔夜: 优选🟢票,🟡仅套利"
	}
	if red >= 3 {
		return "高度警惕: 多重高危共振,减少仓位"
	}
	if green == 0 {
		return "轻仓或空仓: 无主线票通过"
	}
	return "精选个股: 🟢主仓 🟡轻仓套利 🔴规避"
}
