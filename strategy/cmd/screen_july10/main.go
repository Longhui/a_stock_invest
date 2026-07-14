package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"stock-strategy/internal/provider"
	"stock-strategy/internal/reader"
	"stock-strategy/internal/riskcontrol"
	"stock-strategy/internal/selector"
	"stock-strategy/internal/sellcondition"
)

const (
	tdxInstallDir = "D:/Programs/tdx"
	minKlines     = 100
	workerCount   = 8
	targetDateStr = "2026-07-10"
)

var targetDate time.Time

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	targetDate, _ = time.Parse("2006-01-02", targetDateStr)

	fmt.Println("=== 策略选股回测 ===")
	fmt.Printf("目标日期: %s\n", targetDateStr)
	fmt.Printf("并发数: %d\n", workerCount)
	fmt.Println()

	startTime := time.Now()

	// 初始化数据提供者
	p := provider.NewProvider(tdxInstallDir)
	defer p.Close()

	// 获取全市场股票
	fmt.Println("正在获取全市场股票列表...")
	allStocks, err := p.GetAllStocks()
	if err != nil {
		log.Fatalf("获取股票列表失败: %v", err)
	}
	fmt.Printf("全市场: %d 只\n", len(allStocks))

	// 板块过滤: 只保留沪深主板(60/00)、创业板(30)、科创板(688)
	stocks := filterByBoard(allStocks)
	fmt.Printf("过滤后: %d 只(沪深主板+创业板+科创板)\n\n", len(stocks))

	// 并发选股
	fmt.Println("正在执行选股策略...")
	pairs := batchSelect(stocks, p, workerCount)

	elapsed := time.Since(startTime)
	fmt.Printf("\n=== 选股完成 (耗时 %v) ===\n", elapsed)
	fmt.Printf("扫描: %d 只, 选中: %d 只\n\n", len(stocks), len(pairs))

	if len(pairs) == 0 {
		fmt.Println("当前没有符合策略的股票。")
		return
	}

	// ===== 风控筛选 =====
	inputs := buildRiskInputs(pairs, p)
	summary := riskcontrol.ProcessAll(inputs)

	riskcontrol.PrintSummary(summary)
	riskcontrol.PrintCompactSummary(summary)

	// ===== 卖出条件单 =====
	fmt.Println("\n=== 卖出条件单 ===")
	for idx, r := range summary.Results {
		minuteResp, err := p.GetMinuteKlines(r.Code, 240)
		if err != nil || minuteResp == nil || len(minuteResp.List) == 0 {
			fmt.Printf("%2d. %-8s %-8s 分时数据不足,跳过\n", idx+1, r.Code, r.Name)
			continue
		}

		var todayBars []provider.Kline
		for _, k := range minuteResp.List {
			if k.Date.Year() == targetDate.Year() && k.Date.YearDay() == targetDate.YearDay() {
				todayBars = append(todayBars, k)
			}
		}
		if len(todayBars) == 0 {
			fmt.Printf("%2d. %-8s %-8s 无当天分时数据\n", idx+1, r.Code, r.Name)
			continue
		}

		n := len(todayBars)
		close := make([]float64, n)
		high := make([]float64, n)
		low := make([]float64, n)
		for j, k := range todayBars {
			close[j] = k.Close
			high[j] = k.High
			low[j] = k.Low
		}

		sig := sellcondition.Check(r.ClosePrice, close, high, low)
		fmt.Printf("%2d. %s %-8s %s\n", idx+1, r.Code, r.Name, sig.String())
	}
}

// --- 选股流程 ---

type stockCtxPair struct {
	Result *selector.StockResult
	Ctx    *selector.Context
}

func batchSelect(stocks []string, p *provider.Provider, workers int) []*stockCtxPair {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		pairs []*stockCtxPair
	)

	jobCh := make(chan string, len(stocks))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range jobCh {
				result, ctx := selectStock(code, p)
				if result != nil {
					mu.Lock()
					pairs = append(pairs, &stockCtxPair{Result: result, Ctx: ctx})
					mu.Unlock()
					fmt.Printf("  ✓ %s %-8s %.2f元  条件:%v\n",
						code, result.Name, result.ClosePrice, result.Reasons)
				}
			}
		}()
	}

	for _, code := range stocks {
		jobCh <- code
	}
	close(jobCh)
	wg.Wait()

	return pairs
}

func selectStock(code string, p *provider.Provider) (*selector.StockResult, *selector.Context) {
	data, err := p.GetStockData(code, minKlines)
	if err != nil {
		return nil, nil
	}

	// K线截断到目标日期
	if !targetDate.IsZero() {
		var filtered []provider.Kline
		for _, k := range data.Klines {
			if !k.Date.After(targetDate) {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) < minKlines {
			return nil, nil
		}
		data.Klines = filtered
	}

	if data.CirculateShares <= 0 {
		return nil, nil
	}

	return selector.ScreenStock(data)
}

// --- 板块过滤 ---

func filterByBoard(codes []string) []string {
	var result []string
	for _, code := range codes {
		bare := code
		if len(code) >= 8 {
			bare = code[2:]
		}
		if len(bare) < 6 {
			continue
		}
		switch {
		case bare[:1] == "6" && bare[:3] != "688":
			result = append(result, code) // 60xxxx 沪主板
		case bare[:1] == "0":
			result = append(result, code) // 00xxxx 深主板
		case bare[:2] == "30":
			result = append(result, code) // 30xxxx 创业板
		case bare[:3] == "688":
			result = append(result, code) // 688xxx 科创板
		}
	}
	return result
}

// --- 风控输入构建(含板块资金流向+涨停梯队) ---

func buildRiskInputs(pairs []*stockCtxPair, p *provider.Provider) []*riskcontrol.RiskInput {
	inputs := make([]*riskcontrol.RiskInput, 0, len(pairs))
	isFriday := targetDate.Weekday() == time.Friday

	for _, pair := range pairs {
		r := pair.Result
		ctx := pair.Ctx
		n := ctx.N
		sector := p.GetStockSector(r.Code)

		input := &riskcontrol.RiskInput{
			Code:       r.Code,
			Name:       r.Name,
			ClosePrice: r.ClosePrice,
			MarketCap:  r.ClosePrice * ctx.Data.CirculateShares,
			OrigScore:  r.Score,
			Reasons:    r.Reasons,
			Sector:     sector,
			TurnRate:   ctx.TurnRate(n),
			Winner250:  ctx.Winner(n, 250),
			Diff:       ctx.Diff[n],
			IsFriday:   isFriday,

			N:       n,
			Close:   ctx.Close,
			Open:    ctx.Open,
			High:    ctx.High,
			Low:     ctx.Low,
			Volume:  ctx.Volume,
			Amount:  ctx.Amount,
			MA5:     ctx.MA5,
			MA10:    ctx.MA10,
			VOL5:    ctx.VOL5,
			DIF:     ctx.DIF,
			DEA:     ctx.DEA,
			MACDBar: ctx.MACDBar,
			K:       ctx.K,
			D:       ctx.D,
			J:       ctx.J,
		}

		if minuteResp, err := p.GetMinuteKlines(r.Code, 30); err == nil && minuteResp != nil {
			n := len(minuteResp.List)
			input.MinuteN = n
			input.MinuteClose = make([]float64, n)
			input.MinuteOpen = make([]float64, n)
			input.MinuteHigh = make([]float64, n)
			input.MinuteLow = make([]float64, n)
			input.MinuteVol = make([]float64, n)
			input.MinuteTime = make([]int64, n)
			for i, k := range minuteResp.List {
				input.MinuteClose[i] = k.Close
				input.MinuteOpen[i] = k.Open
				input.MinuteHigh[i] = k.High
				input.MinuteLow[i] = k.Low
				input.MinuteVol[i] = k.Volume
				input.MinuteTime[i] = k.Date.Unix()
			}
		}

		inputs = append(inputs, input)
	}

	// ===== 后处理: 填充板块资金流向 + 涨停梯队 =====
	type sectorInfo struct {
		flow    float64
		limitUp int
	}
	sectorCache := make(map[string]*sectorInfo)

	for _, in := range inputs {
		if in.Sector == "" {
			continue
		}
		if _, exists := sectorCache[in.Sector]; exists {
			continue
		}
		info := &sectorInfo{}

		if flowMap, err := p.GetSectorFundFlow(); err == nil {
			for _, f := range flowMap {
				if f.SectorName == in.Sector {
					info.flow = f.MainFlow
					break
				}
			}
		}

		if blocks, err := reader.LoadBlockMap(p.TDXDir); err == nil {
			for _, b := range blocks {
				if b.Name == in.Sector && (strings.HasPrefix(b.Code, "881") || strings.HasPrefix(b.Code, "880")) {
					if limitStocks, err := p.GetSectorLimitUpStocks(b.Code); err == nil {
						info.limitUp = len(limitStocks)
					}
					break
				}
			}
		}

		sectorCache[in.Sector] = info
	}
	for _, in := range inputs {
		if info, ok := sectorCache[in.Sector]; ok {
			in.SectorMainFlow = info.flow
			in.SectorLimitUp = info.limitUp
		}
	}

	return inputs
}
