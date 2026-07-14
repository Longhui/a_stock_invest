package main

import (
	"fmt"
	"time"
	"stock-strategy/internal/provider"
	"stock-strategy/internal/selector"
)

const tdxDir = "D:/Programs/tdx"

func main() {
	p := provider.NewProvider(tdxDir)
	defer p.Close()

	targetDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	data, err := p.GetStockData("300663", 100)
	if err != nil || len(data.Klines) == 0 {
		fmt.Printf("获取数据失败: %v\n", err)
		return
	}

	fmt.Printf("股票: %s %s\n", data.Code, data.Name)
	fmt.Printf("流通股本: %.0f\n", data.CirculateShares)
	fmt.Printf("K线总数: %d\n", len(data.Klines))

	// 截断到目标日期
	var filtered []provider.Kline
	for _, k := range data.Klines {
		if !k.Date.After(targetDate) {
			filtered = append(filtered, k)
		}
	}
	fmt.Printf("截断后: %d 根K线(需≥60)\n", len(filtered))
	if len(filtered) < 60 {
		fmt.Println("K线不足60根，跳过")
		return
	}
	data.Klines = filtered

	result, ctx := selector.ScreenStock(data)
	if result != nil {
		fmt.Printf("✓ 通过! 价格: %.2f, 评分: %.0f, 条件: %v\n", result.ClosePrice, result.Score, result.Reasons)
	} else {
		fmt.Println("✗ 未通过策略条件")
	}

	n := ctx.N
	fmt.Printf("\n最新索引: %d\n", n)
	fmt.Printf("=== 技术指标 ===\n")
	fmt.Printf("最新价: %.2f (前一根: %.2f)\n", ctx.Close[n], ctx.Close[n-1])
	fmt.Printf("MA5: %.4f\n", ctx.MA5[n])
	fmt.Printf("MA10: %.4f\n", ctx.MA10[n])
	fmt.Printf("Volume: %.0f, VOL5: %.0f, 放量: %v\n", filtered[n].Volume, ctx.VOL5[n], ctx.VOL5[n] < filtered[n].Volume)
	fmt.Printf("DIF: %.4f, DEA: %.4f, MACDBar: %.4f\n", ctx.DIF[n], ctx.DEA[n], ctx.MACDBar[n])
	fmt.Printf("K: %.2f, D: %.2f, J: %.2f\n", ctx.K[n-1], ctx.D[n-1], ctx.J[n-1])
	fmt.Printf("换手率: %.2f%%\n", ctx.TurnRate(n))
	fmt.Printf("筹码获利比例(Winner250): %.2f%%\n", ctx.Winner(n, 250))
	fmt.Printf("涨跌幅(Diff): %.2f%%\n", ctx.Diff[n])

	// 检查各条件
	fmt.Println("\n=== 条件逐项检查 ===")
	isYang := ctx.Close[n] > ctx.Open[n]
	fmt.Printf("1. 阳线(Close>Open): %v (%.2f>%.2f)\n", isYang, ctx.Close[n], ctx.Open[n])

	onMA5 := ctx.Close[n] >= ctx.MA5[n]
	fmt.Printf("2. MA5之上: %v (%.2f>=%.2f)\n", onMA5, ctx.Close[n], ctx.MA5[n])

	isVolumeUp := ctx.VOL5[n] < filtered[n].Volume
	fmt.Printf("3. 放量(VOL5<Vol): %v (%.0f<%.0f)\n", isVolumeUp, ctx.VOL5[n], filtered[n].Volume)

	isMACDBull := ctx.DIF[n] > ctx.DEA[n] || ctx.MACDBar[n] >= ctx.MACDBar[n-1]
	fmt.Printf("4. MACD多头: %v (DIF=%.4f, DEA=%.4f, Bar=%.4f)\n", isMACDBull, ctx.DIF[n], ctx.DEA[n], ctx.MACDBar[n])

	isKDJOk := ctx.J[n-1] < 80
	fmt.Printf("5. KDJ未超买(J<80): %v (J=%.2f)\n", isKDJOk, ctx.J[n-1])

	winner := ctx.Winner(n, 250)
	isWinnerOk := winner > 15.0 && winner <= 85.0
	fmt.Printf("6. 获利比例适中(15-85%%): %v (%.2f%%)\n", isWinnerOk, winner)

	turnRate := ctx.TurnRate(n)
	isTurnOk := turnRate > 3.5 && turnRate <= 15
	fmt.Printf("7. 换手率(3.5-15%%): %v (%.2f%%)\n", isTurnOk, turnRate)

	// 阴线后连阳
	fmt.Println("\n8. 阴线后连阳(最后6根):")
	start := n - 5
	if start < 0 {
		start = 0
	}
	for i := start; i <= n; i++ {
		yang := ""
		if ctx.Close[i] > ctx.Open[i] {
			yang = "阳"
		} else {
			yang = "阴"
		}
		fmt.Printf("   [%d] O:%.2f C:%.2f %s  Diff:%.2f%%\n", i, ctx.Open[i], ctx.Close[i], yang, ctx.Diff[i])
	}
}
