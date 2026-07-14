package tail_20_2

import (
	"strings"
	"sync"
	"time"

	"stock-strategy/internal/provider"
	"stock-strategy/internal/reader"
	"stock-strategy/internal/riskcontrol"
	"stock-strategy/internal/selector"
)

// ============================================================
// 1. MACD红柱判断（大盘环境过滤）
// ============================================================

// CheckMACD 检查上证指数60分钟MACD是否为红柱
// 使用截至当日首根60分K线的数据
// idx60Klines: 上证指数60分钟K线序列
// today: 当前交易日
// 返回 true 表示 MACD Bar > 0（红柱，允许交易）
func CheckMACD(idx60Klines []provider.Kline, today time.Time) bool {
	// 找到当日第一根60分钟K线的索引
	firstIdx := -1
	for i, k := range idx60Klines {
		if IsSameDay(k.Date, today) {
			firstIdx = i
			break
		}
	}
	if firstIdx < 30 {
		return false // 数据不足
	}

	// 提取截至当日首根60分K线的收盘价序列
	closeP := make([]float64, firstIdx+1)
	for i := 0; i <= firstIdx; i++ {
		closeP[i] = idx60Klines[i].Close
	}

	bar := Calc60MinMACD(closeP)
	return bar > 0
}

// Calc60MinMACD 计算60分钟K线MACD红柱值
// closePrices: 收盘价序列
// 返回值 > 0 为红柱
func Calc60MinMACD(closePrices []float64) float64 {
	n := len(closePrices)
	if n < 30 {
		return 0
	}
	ema12 := CalcEMA(closePrices, 12)
	ema26 := CalcEMA(closePrices, 26)
	dif := make([]float64, n)
	for i := 0; i < n; i++ {
		dif[i] = ema12[i] - ema26[i]
	}
	dea := CalcEMA(dif, 9)
	return (dif[n-1] - dea[n-1]) * 2
}

// CalcEMA 计算指数移动平均
func CalcEMA(data []float64, period int) []float64 {
	n := len(data)
	if n == 0 {
		return nil
	}
	result := make([]float64, n)
	alpha := 2.0 / float64(period+1)
	result[0] = data[0]
	for i := 1; i < n; i++ {
		result[i] = alpha*data[i] + (1-alpha)*result[i-1]
	}
	return result
}

// ============================================================
// 2. 条件卖出判断
// ============================================================

// CheckSell 检查单只持仓是否触发卖出条件
// buyPrice: 买入价
// high/low/close: 当日日K线的最高/最低/收盘价
// stopLossPct: 止损比例（如0.02=2%）
// 返回卖出决策（ShouldSell=false表示不卖出，由调用方决定兜底）
func CheckSell(buyPrice float64, high, low, close float64, stopLossPct float64) *SellDecision {
	if buyPrice <= 0 {
		return nil
	}

	slRatio := stopLossPct
	if slRatio <= 0 {
		slRatio = 0.05
	}

	// 止损: 日内最低 < 买入价的(1-StopLossPct)
	if low <= buyPrice*(1-slRatio) {
		sellPrice := buyPrice * (1 - slRatio)
		if low > sellPrice {
			sellPrice = low
		}
		return &SellDecision{
			ShouldSell: true,
			SellPrice:  sellPrice,
			Reason:     "止损",
		}
	}

	// 止盈判断: 日内最高涨超2%, 且收盘回落超过峰值1.5%
	if high >= buyPrice*1.02 {
		peak := high
		retraceLevel := peak * 0.985
		if close <= retraceLevel {
			return &SellDecision{
				ShouldSell: true,
				SellPrice:  close,
				Reason:     "止盈",
			}
		}
		// 涨超2%但未回落 → 收盘卖出
		return &SellDecision{
			ShouldSell: true,
			SellPrice:  close,
			Reason:     "收盘卖出",
		}
	}

	// 未涨超2% → 收盘卖出
	return &SellDecision{
		ShouldSell: true,
		SellPrice:  close,
		Reason:     "收盘卖出",
	}
}

// ============================================================
// 3. 策略选股
// ============================================================

// SelectCandidates 批量选股：遍历全市场，筛选满足策略条件的股票
// stockCache: 预加载的日K线缓存
// codes: 待筛选的股票代码列表
// today: 当前交易日（用于截断K线到当日）
// 返回通过筛选的候选列表
func SelectCandidates(stockCache map[string]*StockDataCache, codes []string, today time.Time) []*CandidateResult {
	if len(codes) == 0 {
		return nil
	}

	workers := 8
	jobCh := make(chan string, len(codes))
	resultCh := make(chan *CandidateResult, len(codes))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range jobCh {
				candidate := screenOneStock(stockCache, code, today)
				if candidate != nil {
					resultCh <- candidate
				}
			}
		}()
	}

	for _, code := range codes {
		jobCh <- code
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	var candidates []*CandidateResult
	for c := range resultCh {
		candidates = append(candidates, c)
	}
	return candidates
}

// screenOneStock 对单只股票执行截断 → 选股
func screenOneStock(stockCache map[string]*StockDataCache, code string, today time.Time) *CandidateResult {
	cache, ok := stockCache[code]
	if !ok || cache == nil || len(cache.Klines) == 0 || cache.CirculateShares <= 0 {
		return nil
	}

	// 截断K线到今日（不含今日之后的数据）
	var filtered []Kline
	for _, k := range cache.Klines {
		if !k.Date.After(today) {
			filtered = append(filtered, k)
		}
	}
	if len(filtered) < 100 {
		return nil
	}

	// 转成 provider.Kline 格式给 selector
	pKlines := make([]provider.Kline, len(filtered))
	for i, k := range filtered {
		pKlines[i] = provider.Kline{
			Code:   code,
			Date:   k.Date,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		}
	}

	data := &provider.StockData{
		Code:            code,
		Name:            cache.Name,
		Klines:          pKlines,
		CirculateShares: cache.CirculateShares,
	}

	result, ctx := selector.ScreenStock(data)
	if result == nil || ctx == nil {
		return nil
	}

	return &CandidateResult{
		Result: result,
		Ctx:    ctx,
		Code:   code,
	}
}

// ============================================================
// 3b. 策略选股（实时模式 — 直接通过 provider 逐只获取数据）
// ============================================================

// SelectCandidatesLive 实时选股：通过 provider 逐只获取数据并筛选
// codes: 待筛选的股票代码列表
// prov: 数据提供者
// date: 目标日期（用于截断K线，zero=使用全部数据）
// minKlines: 最少需要的K线数
// 返回通过筛选的候选列表
func SelectCandidatesLive(codes []string, prov *provider.Provider, date time.Time, minKlines int) []*CandidateResult {
	if len(codes) == 0 {
		return nil
	}

	workers := 8
	jobCh := make(chan string, len(codes))
	resultCh := make(chan *CandidateResult, len(codes))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range jobCh {
				candidate := screenOneStockLive(code, prov, date, minKlines)
				if candidate != nil {
					resultCh <- candidate
				}
			}
		}()
	}

	for _, code := range codes {
		jobCh <- code
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	var candidates []*CandidateResult
	for c := range resultCh {
		candidates = append(candidates, c)
	}
	return candidates
}

// screenOneStockLive 对单只股票执行实时获取 → 选股
func screenOneStockLive(code string, prov *provider.Provider, date time.Time, minKlines int) *CandidateResult {
	data, err := prov.GetStockData(code, minKlines)
	if err != nil || data == nil || len(data.Klines) == 0 || data.CirculateShares <= 0 {
		return nil
	}

	// 截断K线到目标日期（用于回测指定日期）
	if !date.IsZero() {
		var filtered []provider.Kline
		for _, k := range data.Klines {
			if !k.Date.After(date) {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) < minKlines {
			return nil
		}
		data.Klines = filtered
	}

	result, ctx := selector.ScreenStock(data)
	if result == nil || ctx == nil {
		return nil
	}
	return &CandidateResult{
		Result: result,
		Ctx:    ctx,
		Code:   code,
	}
}

// ============================================================
// 3c. MACD实时检查（通过 provider 的 API）
// ============================================================

// CheckMACDLive 通过 provider API 实时检查大盘60分钟MACD
// 返回 true 表示红柱（允许交易）
func CheckMACDLive(prov *provider.Provider) (bool, error) {
	return prov.CheckMarketMACD()
}

// ============================================================
// 4. 风控输入构建
// ============================================================

// BuildRiskInputs 从选股候选结果构建风控输入数据
// 包含板块信息、1分钟K线、板块资金流向等
func BuildRiskInputs(pairs []*CandidateResult, prov *provider.Provider, today time.Time) []*riskcontrol.RiskInput {
	inputs := make([]*riskcontrol.RiskInput, 0, len(pairs))
	isFriday := today.Weekday() == time.Friday

	for _, pair := range pairs {
		r := pair.Result
		ctx := pair.Ctx
		n := ctx.N
		sector := prov.GetStockSector(r.Code)

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
			N:          n,
			Close:      ctx.Close,
			Open:       ctx.Open,
			High:       ctx.High,
			Low:        ctx.Low,
			Volume:     ctx.Volume,
			Amount:     ctx.Amount,
			MA5:        ctx.MA5,
			MA10:       ctx.MA10,
			VOL5:       ctx.VOL5,
			DIF:        ctx.DIF,
			DEA:        ctx.DEA,
			MACDBar:    ctx.MACDBar,
			K:          ctx.K,
			D:          ctx.D,
			J:          ctx.J,
		}

		// 加载1分钟K线（用于脉冲分析）
		if minuteResp, err := prov.GetMinuteKlines(r.Code, 30); err == nil && minuteResp != nil {
			input.MinuteN = len(minuteResp.List)
			input.MinuteClose = make([]float64, len(minuteResp.List))
			input.MinuteOpen = make([]float64, len(minuteResp.List))
			input.MinuteHigh = make([]float64, len(minuteResp.List))
			input.MinuteLow = make([]float64, len(minuteResp.List))
			input.MinuteVol = make([]float64, len(minuteResp.List))
			input.MinuteTime = make([]int64, len(minuteResp.List))
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

	// 板块资金流向 + 涨停梯队
	type si struct {
		flow    float64
		limitUp int
	}
	sectorCache := make(map[string]*si)
	for _, in := range inputs {
		if in.Sector == "" {
			continue
		}
		if _, exists := sectorCache[in.Sector]; exists {
			continue
		}
		info := &si{}
		if flowMap, err := prov.GetSectorFundFlow(); err == nil {
			for _, f := range flowMap {
				if f.SectorName == in.Sector {
					info.flow = f.MainFlow
					break
				}
			}
		}
		if blocks, err := reader.LoadBlockMap(prov.TDXDir); err == nil {
			for _, b := range blocks {
				if b.Name == in.Sector && (strings.HasPrefix(b.Code, "881") || strings.HasPrefix(b.Code, "880")) {
					if limitStocks, err := prov.GetSectorLimitUpStocks(b.Code); err == nil {
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
