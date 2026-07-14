package selector

// TDX选股策略的Go实现
// 两套条件同时满足合并选股 | 去冗余+去互斥+剔除昨日涨停+筹码约束+前置阴后3/4连阳

// CheckStock 对一只股票执行完整策略检查，返回是否通过及原因
func CheckStock(ctx *Context) *StockResult {
	if ctx == nil || ctx.N < 60 {
		return nil
	}

	n := ctx.N
	result := &StockResult{
		Code: ctx.Last().Code,
	}

	// ========== 公共变量 ==========
	turnRate := ctx.TurnRate(n)
	yangLine := ctx.Close[n] > ctx.Open[n] && ctx.Close[n] > ctx.Close[n-1]
	maCond := ctx.Close[n] > ctx.MA5[n]
	volNormal := ctx.Volume[n] > ctx.VOL5[n] && turnRate > 3.5
	turnCond := turnRate > 5 && turnRate < 15
	volCond := volNormal || turnCond
	huanShouFilter := turnRate > 3.5
	chipProfit := ctx.Winner(n, 250) // 使用250个交易日(约1年)计算筹码分布
	chipCond := chipProfit > 15 && chipProfit < 85

	// ========== 涨幅约束 ==========
	zdf := ctx.Diff[n]
	var zdfCond bool
	if turnRate > 5 {
		zdfCond = zdf < 5.2
	} else if turnRate > 4 {
		zdfCond = zdf < 4.6
	} else {
		zdfCond = zdf < 4.1
	}

	// ========== MACD ==========
	macdBar := ctx.MACDBar[n]
	macdBarLimit := macdBar > -0.7
	diffNotDown := ctx.DIF[n] >= ctx.DIF[n-1]-0.02
	macdBarUp := macdBar > ctx.MACDBar[n-1]
	macdCond := macdBarLimit && macdBarUp && diffNotDown

	// ========== KDJ ==========
	kdjCond := ctx.J[n] < 88
	kdjCross := Cross(ctx.K, ctx.D, n)

	// ========== 风险过滤条件 ==========
	badFilter := ctx.DIF[n] > 0 && ctx.DEA[n] > 0 && ctx.MACDBar[n] < 0 && kdjCross && ctx.D[n] > 60

	// MACD特殊条件
	macdJC3 := Cross(ctx.DIF, ctx.DEA, n-3)
	macdJC2 := Cross(ctx.DIF, ctx.DEA, n-2)
	macdSpec := ctx.MACDBar[n] > 0 && ctx.MACDBar[n] < 0.2 && ctx.DIF[n] < -0.3

	var filter1 bool
	if n >= 5 {
		filter1 = macdJC3 && ctx.MACDBar[n-2] > 0 && ctx.MACDBar[n-2] < 0.2 &&
			ctx.DIF[n-2] < -0.3 &&
			ctx.MACDBar[n-1] > 0 && ctx.MACDBar[n-1] < 0.2 &&
			ctx.DIF[n-1] < -0.3 &&
			macdSpec
	}
	var filter2 bool
	if n >= 3 {
		filter2 = macdJC2 && ctx.MACDBar[n-1] > 0 && ctx.MACDBar[n-1] < 0.2 &&
			ctx.DIF[n-1] < -0.3 &&
			macdSpec
	}

	// ========== KDJ高位见顶过滤 ==========
	var dangerTop bool
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
		highOver := maxFloat(maxFloat(aJ, bJ), maxFloat(cJ, dJ)) > 95
		dangerTop = fourV && highOver
	}

	// ========== 第一组 XG1 ==========
	xg1 := yangLine && maCond && volCond && macdCond && kdjCond &&
		zdfCond && !badFilter && !filter1 && !filter2 &&
		!dangerTop && huanShouFilter && chipCond

	// ========== 第二组 XG2 ==========
	jc1 := Cross(ctx.MA5, ctx.MA10, n) && ctx.D[n] < 60
	jc2 := kdjCross && ctx.D[n] < 60
	jc3 := Cross(ctx.DIF, ctx.DEA, n)
	jc4 := Cross(ctx.K, ctx.D, n-1) && ctx.D[n-1] < 60

	turnMin := 3.5
	turnMax := 15.0
	xg2 := (jc1 || jc2 || jc3 || jc4) && turnRate > turnMin && turnRate < turnMax && ctx.DIF[n] > -1

	// ========== 剔除昨日涨停 ==========
	lastZT := false
	if n >= 1 {
		code := ctx.Last().Code
		isChiNext := isChiNextStock(code)
		lastZT = CheckLastZT(ctx.Diff[n-1], isChiNext)
	}
	notLastZT := !lastZT

	// ========== K线结构: 前置阴线 + 3/4连阳 ==========
	var cond3Y, cond4Y bool
	if n >= 4 {
		// 恰好3连阳: 往前第4根是阴线, 之后连续3根阳线
		cond3Y = countOpen(ctx.Close, ctx.Open, n-2, 3) && ctx.Close[n-3] <= ctx.Open[n-3]
	}
	if n >= 5 {
		// 恰好4连阳: 往前第5根是阴线, 之后连续4根阳线
		cond4Y = countOpen(ctx.Close, ctx.Open, n-3, 4) && ctx.Close[n-4] <= ctx.Open[n-4]
	}
	klineStruct := cond3Y || cond4Y

	// ========== 最终结果 ==========
	passed := xg1 && xg2 && notLastZT && klineStruct
	if !passed {
		return nil
	}

	// 记录满足条件
	result.Reasons = make([]string, 0)
	if yangLine { result.Reasons = append(result.Reasons, "阳线") }
	if maCond { result.Reasons = append(result.Reasons, "MA5之上") }
	if volCond { result.Reasons = append(result.Reasons, "放量") }
	if macdCond { result.Reasons = append(result.Reasons, "MACD多头") }
	if kdjCond { result.Reasons = append(result.Reasons, "KDJ未超买") }
	if chipCond { result.Reasons = append(result.Reasons, "获利比例适中") }
	if cond3Y || cond4Y { result.Reasons = append(result.Reasons, "阴线后连阳") }
	result.ClosePrice = ctx.Close[n]
	result.Score = float64(len(result.Reasons))

	return result
}

// countOpen 检查连续N根K线是否都是阳线(C>O)
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

// isChiNextStock 判断是否为创业板(30xxxx)或科创板(688xxx)
func isChiNextStock(code string) bool {
	if len(code) >= 8 {
		code = code[2:] // 去掉交易所前缀
	}
	if len(code) >= 6 {
		return code[:2] == "30" || code[:3] == "688"
	}
	return false
}
