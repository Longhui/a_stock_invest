package main

import (
	"fmt"
	"log"
	"os"
	"stock-strategy/internal/provider"
	"stock-strategy/internal/selector"
	"sync"
	"sync/atomic"
	"time"
)

const (
	tdxInstallDir = "D:/Programs/tdx"
	minKlines     = 100
	workerCount   = 8
)

// 条件计数器
var stats struct {
	total       int64
	dayOk       int64 // 本地.day文件读取成功
	circOk      int64 // 流通股本>0
	circZero    int64 // 流通股本为0
	errCount    int64 // GetStockData报错
	yangLine    int64
	maCond      int64
	volCond     int64
	turnCond    int64
	macdCond    int64
	kdjCond     int64
	zdfCond     int64
	notBadFilt  int64
	notFilt1    int64
	notFilt2    int64
	notDanger   int64
	huanShou    int64
	chipCond    int64
	xg1         int64

	jc1         int64
	jc2         int64
	jc3         int64
	jc4         int64
	xg2Turn     int64
	xg2Dif      int64
	xg2         int64

	notLastZT   int64
	cond3Y      int64
	cond4Y      int64
	klineStruct int64

	final       int64
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	fmt.Println("=== 选股策略 Debug 诊断 ===")

	p := provider.NewProvider(tdxInstallDir)
	defer p.Close()

	stocks, err := p.GetAllStocks()
	if err != nil {
		log.Fatalf("获取股票列表失败: %v", err)
	}
	fmt.Printf("共获取 %d 只股票\n\n", len(stocks))

	startTime := time.Now()

	// 并行处理
	var wg sync.WaitGroup
	jobCh := make(chan string, len(stocks))
	sampleMu := sync.Mutex{}
	sampleCount := 0

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range jobCh {
				debugStock(code, p, &sampleMu, &sampleCount)
			}
		}()
	}
	for _, code := range stocks {
		jobCh <- code
	}
	close(jobCh)
	wg.Wait()

	elapsed := time.Since(startTime)
	fmt.Printf("\n=== 诊断完成 (耗时 %v) ===\n", elapsed)
	printStats()
}

func debugStock(code string, p *provider.Provider, sampleMu *sync.Mutex, sampleCount *int) {
	data, err := p.GetStockData(code, minKlines)
	if err != nil {
		atomic.AddInt64(&stats.errCount, 1)
		return
	}
	atomic.AddInt64(&stats.dayOk, 1)
	if data.CirculateShares <= 0 {
		atomic.AddInt64(&stats.circZero, 1)
		return
	}
	atomic.AddInt64(&stats.circOk, 1)

	ctx := selector.NewContext(data)
	if ctx == nil {
		return
	}

	atomic.AddInt64(&stats.total, 1)
	n := ctx.N

	turnRate := ctx.TurnRate(n)

	// 1. yangLine
	yl := ctx.Close[n] > ctx.Open[n] && ctx.Close[n] > ctx.Close[n-1]
	if yl {
		atomic.AddInt64(&stats.yangLine, 1)
	}

	// 2. maCond
	maC := ctx.Close[n] > ctx.MA5[n]
	if maC {
		atomic.AddInt64(&stats.maCond, 1)
	}

	// 3. volCond
	volN := ctx.Volume[n] > ctx.VOL5[n] && turnRate > 3.5
	turnC := turnRate > 5 && turnRate < 15
	volC := volN || turnC
	if volC {
		atomic.AddInt64(&stats.volCond, 1)
	}

	// 4. macdCond
	macdBarLimit := ctx.MACDBar[n] > -0.7
	diffNotDown := ctx.DIF[n] >= ctx.DIF[n-1]-0.02
	macdBarUp := ctx.MACDBar[n] > ctx.MACDBar[n-1]
	macdC := macdBarLimit && macdBarUp && diffNotDown
	if macdC {
		atomic.AddInt64(&stats.macdCond, 1)
	}

	// 5. kdjCond
	kdjC := ctx.J[n] < 88
	if kdjC {
		atomic.AddInt64(&stats.kdjCond, 1)
	}

	// 6. zdfCond
	zdf := ctx.Diff[n]
	var zdfC bool
	if turnRate > 5 {
		zdfC = zdf < 5.2
	} else if turnRate > 4 {
		zdfC = zdf < 4.6
	} else {
		zdfC = zdf < 4.1
	}
	if zdfC {
		atomic.AddInt64(&stats.zdfCond, 1)
	}

	// 7. badFilter
	kdjCross := selector.Cross(ctx.K, ctx.D, n)
	badF := ctx.DIF[n] > 0 && ctx.DEA[n] > 0 && ctx.MACDBar[n] < 0 && kdjCross && ctx.D[n] > 60
	if !badF {
		atomic.AddInt64(&stats.notBadFilt, 1)
	}

	// 8. filter1
	var f1 bool
	if n >= 5 {
		macdJC3 := selector.Cross(ctx.DIF, ctx.DEA, n-3)
		macdSpec := ctx.MACDBar[n] > 0 && ctx.MACDBar[n] < 0.2 && ctx.DIF[n] < -0.3
		f1 = macdJC3 && ctx.MACDBar[n-2] > 0 && ctx.MACDBar[n-2] < 0.2 &&
			ctx.DIF[n-2] < -0.3 &&
			ctx.MACDBar[n-1] > 0 && ctx.MACDBar[n-1] < 0.2 &&
			ctx.DIF[n-1] < -0.3 &&
			macdSpec
	}
	if !f1 {
		atomic.AddInt64(&stats.notFilt1, 1)
	}

	// 9. filter2
	var f2 bool
	if n >= 3 {
		macdJC2 := selector.Cross(ctx.DIF, ctx.DEA, n-2)
		macdSpec := ctx.MACDBar[n] > 0 && ctx.MACDBar[n] < 0.2 && ctx.DIF[n] < -0.3
		f2 = macdJC2 && ctx.MACDBar[n-1] > 0 && ctx.MACDBar[n-1] < 0.2 &&
			ctx.DIF[n-1] < -0.3 &&
			macdSpec
	}
	if !f2 {
		atomic.AddInt64(&stats.notFilt2, 1)
	}

	// 10. dangerTop
	var danger bool
	if n >= 4 {
		aJ := ctx.J[n-3]
		bJ := ctx.J[n-2]
		cJ := ctx.J[n-1]
		dJ := ctx.J[n]
		v1 := aJ > bJ && bJ > cJ && cJ < dJ
		v2 := aJ > bJ && bJ < cJ && cJ < dJ
		v3 := aJ > bJ && bJ < cJ && cJ > dJ
		v4 := aJ < bJ && bJ > cJ && cJ < dJ
		fourV := v1 || v2 || v3 || v4
		highOver := max(max(aJ, bJ), max(cJ, dJ)) > 95
		danger = fourV && highOver
	}
	if !danger {
		atomic.AddInt64(&stats.notDanger, 1)
	}

	// 11. huanShouFilter
	if turnRate > 3.5 {
		atomic.AddInt64(&stats.huanShou, 1)
	}

	// 12. chipCond
	chipProfit := ctx.Winner(n, 250)
	chipC := chipProfit > 15 && chipProfit < 85
	if chipC {
		atomic.AddInt64(&stats.chipCond, 1)
	}

	// XG1
	xg1 := yl && maC && volC && macdC && kdjC && zdfC && !badF && !f1 && !f2 && !danger && turnRate > 3.5 && chipC
	if xg1 {
		atomic.AddInt64(&stats.xg1, 1)
	}

	// XG2 sub-conditions
	jc1 := selector.Cross(ctx.MA5, ctx.MA10, n) && ctx.D[n] < 60
	jc2 := kdjCross && ctx.D[n] < 60
	jc3 := selector.Cross(ctx.DIF, ctx.DEA, n)
	jc4 := selector.Cross(ctx.K, ctx.D, n-1) && ctx.D[n-1] < 60
	if jc1 { atomic.AddInt64(&stats.jc1, 1) }
	if jc2 { atomic.AddInt64(&stats.jc2, 1) }
	if jc3 { atomic.AddInt64(&stats.jc3, 1) }
	if jc4 { atomic.AddInt64(&stats.jc4, 1) }

	hasJc := jc1 || jc2 || jc3 || jc4
	turnOk := turnRate > 3.5 && turnRate < 15
	difOk := ctx.DIF[n] > -1
	if turnOk { atomic.AddInt64(&stats.xg2Turn, 1) }
	if difOk { atomic.AddInt64(&stats.xg2Dif, 1) }
	xg2 := hasJc && turnOk && difOk
	if xg2 {
		atomic.AddInt64(&stats.xg2, 1)
	}

	// 昨日涨停
	var lastZT bool
	if n >= 1 {
		code := ctx.Last().Code
		isChiNext := isChiNextStock(code)
		lastZT = selector.CheckLastZT(ctx.Diff[n-1], isChiNext)
	}
	if !lastZT {
		atomic.AddInt64(&stats.notLastZT, 1)
	}

	// K线结构
	var cond3Y, cond4Y bool
	if n >= 4 {
		cond3Y = countOpen(ctx.Close, ctx.Open, n-2, 3) && ctx.Close[n-3] <= ctx.Open[n-3]
	}
	if n >= 5 {
		cond4Y = countOpen(ctx.Close, ctx.Open, n-3, 4) && ctx.Close[n-4] <= ctx.Open[n-4]
	}
	if cond3Y { atomic.AddInt64(&stats.cond3Y, 1) }
	if cond4Y { atomic.AddInt64(&stats.cond4Y, 1) }
	klineStruct := cond3Y || cond4Y
	if klineStruct {
		atomic.AddInt64(&stats.klineStruct, 1)
	}

	if xg1 && xg2 && !lastZT && klineStruct {
		atomic.AddInt64(&stats.final, 1)
	}

	// 打印前20只通过klineStruct的样本
	if klineStruct {
		sampleMu.Lock()
		if *sampleCount < 20 {
			*sampleCount++
			fmt.Printf("\n=== 样本%d: %s ===\n", *sampleCount, code)
			fmt.Printf("  n=%d, close=%.2f, open=%.2f\n", n, ctx.Close[n], ctx.Open[n])
			fmt.Printf("  yangLine=%v, maCond=%v, volCond=%v\n", yl, maC, volC)
			fmt.Printf("  macdCond=%v, kdjCond=%v, zdfCond=%v\n", macdC, kdjC, zdfC)
			fmt.Printf("  badF=%v, f1=%v, f2=%v, danger=%v\n", badF, f1, f2, danger)
			fmt.Printf("  turnRate=%.2f, chipProfit=%.2f, xg1=%v\n", turnRate, chipProfit, xg1)
			fmt.Printf("  MA5=%.2f, MA10=%.2f\n", ctx.MA5[n], ctx.MA10[n])
			fmt.Printf("  DIF=%.4f, DEA=%.4f, MACDBar=%.4f\n", ctx.DIF[n], ctx.DEA[n], ctx.MACDBar[n])
			fmt.Printf("  K=%.2f, D=%.2f, J=%.2f\n", ctx.K[n], ctx.D[n], ctx.J[n])
			fmt.Printf("  jc1=%v, jc2=%v, jc3=%v, jc4=%v, xg2=%v\n", jc1, jc2, jc3, jc4, xg2)
			fmt.Printf("  lastZT=%v, klineStruct=%v\n", lastZT, klineStruct)
			fmt.Printf("  cond3Y=%v (n-2=%d, close[n-3]=%.2f, open[n-3]=%.2f)\n", cond3Y, n-2, ctx.Close[n-3], ctx.Open[n-3])
			fmt.Printf("  cond4Y=%v (n-3=%d, close[n-4]=%.2f, open[n-4]=%.2f)\n", cond4Y, n-3, ctx.Close[n-4], ctx.Open[n-4])
			// 打印最近10根K线
			fmt.Printf("  最近10根K线(C/O):")
			for i := n - 9; i <= n; i++ {
				if i >= 0 {
					fmt.Printf(" [%d]%.2f/%.2f", i, ctx.Close[i], ctx.Open[i])
				}
			}
			fmt.Println()
			// 打印最近10根K线的Diff
			fmt.Printf("  最近10根Diff:")
			for i := n - 9; i <= n; i++ {
				if i >= 0 {
					fmt.Printf(" [%d]%.2f%%", i, ctx.Diff[i])
				}
			}
			fmt.Println()
			// 打印最近10根K线的Volume
			fmt.Printf("  最近10根VOL:")
			for i := n - 9; i <= n; i++ {
				if i >= 0 {
					fmt.Printf(" [%d]%.0f", i, ctx.Volume[i])
				}
			}
			fmt.Println()
		}
		sampleMu.Unlock()
	}
}

func printStats() {
	total := stats.total
	fmt.Println("\n====== 数据加载统计 ======")
	fmt.Printf("GetStockData报错: %d\n", stats.errCount)
	fmt.Printf(".day读取成功:   %d\n", stats.dayOk)
	fmt.Printf("流通股本为0:    %d\n", stats.circZero)
	fmt.Printf("流通股本>0:     %d\n", stats.circOk)
	fmt.Printf("进入策略扫描:   %d\n", total)
	fmt.Println()
	fmt.Println("====== 条件通过统计 ======")
	fmt.Printf("总扫描(有数据): %d\n", total)
	fmt.Println()
	fmt.Printf("  yangLine       : %6d (%.1f%%) - 阳线\n", stats.yangLine, pct(stats.yangLine, total))
	fmt.Printf("  maCond         : %6d (%.1f%%) - 收盘在MA5之上\n", stats.maCond, pct(stats.maCond, total))
	fmt.Printf("  volCond        : %6d (%.1f%%) - 放量\n", stats.volCond, pct(stats.volCond, total))
	fmt.Printf("  macdCond       : %6d (%.1f%%) - MACD多头\n", stats.macdCond, pct(stats.macdCond, total))
	fmt.Printf("  kdjCond        : %6d (%.1f%%) - KDJ未超买\n", stats.kdjCond, pct(stats.kdjCond, total))
	fmt.Printf("  zdfCond        : %6d (%.1f%%) - 涨幅约束\n", stats.zdfCond, pct(stats.zdfCond, total))
	fmt.Printf("  !badFilter     : %6d (%.1f%%) - 非风险过滤\n", stats.notBadFilt, pct(stats.notBadFilt, total))
	fmt.Printf("  !filter1       : %6d (%.1f%%) - 非MACD特殊条件1\n", stats.notFilt1, pct(stats.notFilt1, total))
	fmt.Printf("  !filter2       : %6d (%.1f%%) - 非MACD特殊条件2\n", stats.notFilt2, pct(stats.notFilt2, total))
	fmt.Printf("  !dangerTop     : %6d (%.1f%%) - 非KDJ见顶\n", stats.notDanger, pct(stats.notDanger, total))
	fmt.Printf("  huanShouFilter : %6d (%.1f%%) - 换手率>3.5\n", stats.huanShou, pct(stats.huanShou, total))
	fmt.Printf("  chipCond       : %6d (%.1f%%) - 获利比例15-85\n", stats.chipCond, pct(stats.chipCond, total))
	fmt.Printf("  ====================================\n")
	fmt.Printf("  XG1            : %6d (%.1f%%) - 第一组合\n", stats.xg1, pct(stats.xg1, total))
	fmt.Println()
	fmt.Printf("  jc1(MA金叉)    : %6d (%.1f%%)\n", stats.jc1, pct(stats.jc1, total))
	fmt.Printf("  jc2(KDJ金叉)   : %6d (%.1f%%)\n", stats.jc2, pct(stats.jc2, total))
	fmt.Printf("  jc3(MACD金叉)  : %6d (%.1f%%)\n", stats.jc3, pct(stats.jc3, total))
	fmt.Printf("  jc4(KDJ隔日)   : %6d (%.1f%%)\n", stats.jc4, pct(stats.jc4, total))
	fmt.Printf("  turnOk(3.5-15) : %6d (%.1f%%)\n", stats.xg2Turn, pct(stats.xg2Turn, total))
	fmt.Printf("  difOk(DIF>-1)  : %6d (%.1f%%)\n", stats.xg2Dif, pct(stats.xg2Dif, total))
	fmt.Printf("  ====================================\n")
	fmt.Printf("  XG2            : %6d (%.1f%%) - 第二组合\n", stats.xg2, pct(stats.xg2, total))
	fmt.Println()
	fmt.Printf("  !lastZT        : %6d (%.1f%%) - 非昨日涨停\n", stats.notLastZT, pct(stats.notLastZT, total))
	fmt.Printf("  cond3Y(3连阳)  : %6d (%.1f%%) - 阴线后3连阳\n", stats.cond3Y, pct(stats.cond3Y, total))
	fmt.Printf("  cond4Y(4连阳)  : %6d (%.1f%%) - 阴线后4连阳\n", stats.cond4Y, pct(stats.cond4Y, total))
	fmt.Printf("  klineStruct    : %6d (%.1f%%) - 阴线后连阳\n", stats.klineStruct, pct(stats.klineStruct, total))
	fmt.Println()
	fmt.Printf("  ★ FINAL PASS   : %6d (%.1f%%) - 全部通过\n", stats.final, pct(stats.final, total))
}

func pct(a, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(a) / float64(total) * 100
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// countOpen 同 selector.countOpen
func countOpen(close, open []float64, startIdx int, count int) bool {
	if startIdx < 0 || startIdx+count > len(close) {
		return false
	}
	for i := 0; i < count; i++ {
		if close[startIdx+i] <= open[startIdx+i] {
			return false
		}
	}
	return true
}

func isChiNextStock(code string) bool {
	if len(code) >= 8 {
		code = code[2:]
	}
	if len(code) >= 6 {
		return code[:2] == "30" || code[:3] == "688"
	}
	return false
}

func init() {
	f, err := os.OpenFile("debug_strategy.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(f)
	}
}
