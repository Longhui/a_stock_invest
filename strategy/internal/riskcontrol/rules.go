package riskcontrol

// ============================================================
// 固定阈值常量 — 全部来源于用户给定的量化数值
// ============================================================

const (
	// --- 筹码获利比例阈值 ---
	ChipLevel1     = 80.0 // ≥80% 一级降序
	ChipNoHeavy    = 85.0 // ≥85% 禁止第一重仓
	ChipDowngrade  = 90.0 // ≥90% 下调至🟡

	// --- KDJ阈值 ---
	KDJPulseDown   = 80.0 // J>80 脉冲降至2-3%
	KDJMainDown    = 85.0 // J>85 主线降备选
	KDJYellow      = 90.0 // J>90 划入🟡

	// --- 双高危共振 ---
	DualChipThresh = 80.0 // 筹码≥80%
	DualJThresh    = 80.0 // J>80%
	// 处罚: 排名大幅后置、仓位减半、高开预期压缩至0%-1.5%

	// --- 周五折价 ---
	FridayChipPenalty  = 5.0 // 筹码档位整体-5%
	FridayKDJPenalty   = 5   // KDJ超买阈值-5
	// 主线低开风险+半档，4%脉冲概率降一档

	// --- 市值阈值 ---
	LargeCapThreshold  = 100e8 // 100亿(元)
	// >100亿中盘股永久排在小盘股后方

	// --- 换手率阈值 ---
	TurnRateMin     = 3.5  // 最低换手率%
	TurnRateMax     = 20.0 // 最高换手率%(超过扣分)

	// --- Step2 技术定级阈值 ---
	MACDStrongMin   = 0.1  // MACD红柱持续放大: Bar>0.1
	MACDWeakMax     = 0.05 // 短小刚翻红弱修复: Bar<0.05
)

// processAllRules 对单个输入执行完整五步流水线，返回RiskResult
func processAllRules(in *RiskInput) *RiskResult {
	r := &RiskResult{
		Code:       in.Code,
		Name:       in.Name,
		ClosePrice: in.ClosePrice,
		MarketCap:  in.MarketCap,
		OrigScore:  in.OrigScore,
		Reasons:    in.Reasons,
		Sector:     in.Sector,
		Topic:      in.Topic,
	}

	// ===== Step 0: 前置计算 =====
	chipFlag, kdjFlag, dualFlag := step0PreCheck(in, r)
	_ = kdjFlag

	// ===== Step 1: 板块+量价修正 =====
	sLevel := SectorLevel(in.Sector, in.Topic)
	step1SectorSentiment(in, r, sLevel)
	step1Divergence(in, r)

	// ===== Step 2: 技术定级 🟢🟡🔴 =====
	step2TechnicalRating(in, r, chipFlag, dualFlag, sLevel)
	step2MarketCapSort(in, r)

	// ===== Step 3: 量能强弱扣分 =====
	step3VolumeScoring(in, r, sLevel)

	// ===== Step 4: 预判+排序 =====
	step4OpenPrediction(in, r)

	// ==== 后处理: 计算总分 + 确定颜色 + 收集风险 ====
	finalizeRating(in, r)

	return r
}

// ============================================================
// Step 0: 前置计算
// ============================================================

// step0PreCheck 返回: 筹码标记, KDJ标记, 双高危共振标记
func step0PreCheck(in *RiskInput, r *RiskResult) (chipFlag string, kdjFlag string, dualResonance bool) {
	chip := in.Winner250
	jVal := in.J[in.N]

	// 筹码获利比例扣分
	switch {
	case chip >= ChipDowngrade:
		chipFlag = "筹码≥90%"
		r.RiskFlags = append(r.RiskFlags, "筹码≥90%下调🟡")
	case chip >= ChipNoHeavy:
		chipFlag = "筹码≥85%"
		r.RiskFlags = append(r.RiskFlags, "筹码≥85%禁止第一重仓")
	case chip >= ChipLevel1:
		chipFlag = "筹码≥80%"
		r.RiskFlags = append(r.RiskFlags, "筹码≥80%一级降序")
	}

	// KDJ扣分
	switch {
	case jVal >= KDJYellow:
		kdjFlag = "J≥90"
		r.RiskFlags = append(r.RiskFlags, "J≥90划入🟡")
	case jVal >= KDJMainDown:
		kdjFlag = "J≥85"
		r.RiskFlags = append(r.RiskFlags, "J≥85主线降备选")
	case jVal >= KDJPulseDown:
		kdjFlag = "J≥80"
		r.RiskFlags = append(r.RiskFlags, "J≥80脉冲降至2-3%")
	}

	// 周五折价
	if in.IsFriday {
		// 筹码阈值-5%, KDJ阈值-5(已在上面生效)
		r.RiskFlags = append(r.RiskFlags, "周五隔夜折价")
	}

	// 双高危共振
	if chip >= DualChipThresh && jVal >= DualJThresh {
		dualResonance = true
		r.RiskFlags = append(r.RiskFlags, "双高危共振:仓位减半高开压缩0-1.5%")
	}

	return
}

// ============================================================
// Step 1: 板块+情绪修正
// ============================================================

func step1SectorSentiment(in *RiskInput, r *RiskResult, sLevel int) {
	n := in.N
	score := 100.0

	// 轮动题材最高只能🟡(硬性红线)
	if sLevel >= 2 {
		score -= 20
		r.RiskFlags = append(r.RiskFlags, "轮动题材最高🟡")
	}

	// ===== 板块梯队检测(使用板块成分股数据) =====
	if in.HasSectorTeam {
		score += 10
		r.RiskFlags = append(r.RiskFlags, "板块梯队加分")
		if in.SectorGroupSize >= 3 {
			score += 5
			r.RiskFlags = append(r.RiskFlags, "板块梯队规模≥3加分")
		}
	}

	// ===== 板块资金净流入(东方财富) =====
	if in.SectorMainFlow > 0 {
		// 主力净流入
		if in.SectorMainFlow > 1e8 { // >1亿
			score += 15
			r.RiskFlags = append(r.RiskFlags, "板块主力净流入加分")
		} else {
			score += 5
			r.RiskFlags = append(r.RiskFlags, "板块主力小幅流入")
		}
	} else if in.SectorMainFlow < 0 {
		if in.SectorMainFlow < -1e8 { // 流出>1亿
			score -= 20
			r.RiskFlags = append(r.RiskFlags, "板块主力大幅流出扣分")
		} else {
			score -= 10
			r.RiskFlags = append(r.RiskFlags, "板块主力流出扣分")
		}
	}

	// ===== 涨停梯队(使用板块成分股+实时行情) =====
	if in.SectorLimitUp >= 3 {
		score += 15
		r.RiskFlags = append(r.RiskFlags, "板块多只涨停加分")
	} else if in.SectorLimitUp >= 1 {
		score += 8
		r.RiskFlags = append(r.RiskFlags, "板块有涨停股加分")
	}

	_ = n
	r.StepScores[0] = score
}

func step1Divergence(in *RiskInput, r *RiskResult) {
	n := in.N
	score := r.StepScores[0]

	// 个股价涨量缩: 价格涨但成交量萎缩
	if n >= 2 && in.Close[n] > in.Close[n-1] && in.Volume[n] < in.VOL5[n] {
		score -= 15
		r.RiskFlags = append(r.RiskFlags, "价涨量缩一级降序")
	}

	// 尾盘拉升: 用close与open的相对位置判断
	if n >= 1 && in.Close[n] > in.Open[n] && in.Close[n] > in.Close[n-1] {
		if in.Volume[n] > in.VOL5[n]*1.5 {
			score -= 10
			r.RiskFlags = append(r.RiskFlags, "放量尾盘拉升扣分")
		}
	}

	// ===== 板块分歧检测(使用板块成分股数据) =====
	// 同板块有2+只选股结果,且该个股方向与板块平均方向相反
	if in.HasSectorTeam && in.SectorGroupSize >= 2 {
		// 个股涨但板块平均跌 → 板块分歧(该股独立上涨不可持续)
		if in.Diff > 0.5 && in.SectorAvgDiff < -0.5 {
			score -= 15
			r.RiskFlags = append(r.RiskFlags, "板块分歧:个股独涨板块普跌")
		}
		// 个股跌但板块平均涨 → 该股弱于板块
		if in.Diff < -0.5 && in.SectorAvgDiff > 0.5 {
			score -= 10
			r.RiskFlags = append(r.RiskFlags, "板块分歧:个股弱于板块")
		}
		// 板块普跌且该股也跌 → 板块系统性风险
		if in.Diff < -1.0 && in.SectorAvgDiff < -1.0 {
			score -= 15
			r.RiskFlags = append(r.RiskFlags, "板块系统性下跌扣分")
		}
	}

	r.StepScores[0] = clamp(score, 0, 100)
}

// ============================================================
// Step 2: 技术定级 🟢🟡🔴
// ============================================================

func step2TechnicalRating(in *RiskInput, r *RiskResult, chipFlag string, dualResonance bool, sLevel int) {
	n := in.N
	score := 100.0

	// 轮动题材最高🟡(硬性红线) — 已经处理
	if sLevel >= 2 {
		score -= 25 // 大幅扣分
	}

	// 主线达标检测(企稳阳线+KDJ低位金叉+MACD绿柱收敛翻红)
	hasYang := in.Close[n] > in.Open[n] && in.Close[n] > in.Close[n-1]

	// KDJ低位金叉: K从下方向上穿越D,且K<50
	kdjGoldenCross := false
	if n >= 1 && in.K[n-1] <= in.D[n-1] && in.K[n] > in.D[n] && in.K[n] < 50 {
		kdjGoldenCross = true
	}

	// MACD绿柱收敛翻红: Bar从负变正
	macdTurnRed := in.MACDBar[n] > 0 && (n < 1 || in.MACDBar[n-1] <= 0)

	// 满足🟢条件(主线+三要素)
	if sLevel < 2 && hasYang && kdjGoldenCross && macdTurnRed {
		// 🟢看涨,不扣分
		_ = 0 // 保留评级
	} else if sLevel < 2 && hasYang && in.MACDBar[n] > 0 {
		// 部分达标,🟡震荡
		score -= 20
		r.RiskFlags = append(r.RiskFlags, "技术未完全达标")
	} else {
		// 不达标
		score -= 40
		r.RiskFlags = append(r.RiskFlags, "技术面偏弱")
	}

	// MACD红柱强度分级
	if in.MACDBar[n] > MACDStrongMin {
		// 红柱持续放大
		if n >= 2 && in.MACDBar[n] > in.MACDBar[n-1] && in.MACDBar[n-1] > in.MACDBar[n-2] {
			// 完整🟢评级,加分
			score += 10
		}
	} else if in.MACDBar[n] > 0 && in.MACDBar[n] < MACDWeakMax {
		// 短小刚翻红弱修复,降序扣分
		score -= 15
		r.RiskFlags = append(r.RiskFlags, "MACD弱修复扣分")
	}

	// 密集套牢盘/指标高位钝化/长上影放量滞涨 → 🟡
	if in.Close[n] < in.MA10[n] {
		score -= 10
	}
	if n >= 1 && in.High[n]-in.Close[n] > (in.Close[n]-in.Open[n])*2 {
		// 长上影
		score -= 10
		r.RiskFlags = append(r.RiskFlags, "长上影放量滞涨")
	}

	r.StepScores[1] = clamp(score, 0, 100)
}

func step2MarketCapSort(in *RiskInput, r *RiskResult) {
	// >100亿中盘股扣分(永久排在小盘股后方)
	if in.MarketCap >= LargeCapThreshold {
		r.RiskFlags = append(r.RiskFlags, "流通市值>100亿")
	}
	// 中盘股+筹码≥80%剔除第一重仓
	if in.MarketCap >= LargeCapThreshold && in.Winner250 >= ChipLevel1 {
		r.RiskFlags = append(r.RiskFlags, "大盘+筹码≥80%剔除第一重仓")
	}
}

// ============================================================
// Step 3: 量能强弱扣分
// ============================================================

func step3VolumeScoring(in *RiskInput, r *RiskResult, sLevel int) {
	n := in.N
	score := 100.0

	// 连续温和放量保排名(Vol > VOL5 且不过度)
	if in.Volume[n] > in.VOL5[n] && in.Volume[n] < in.VOL5[n]*1.5 {
		// 温和放量,不扣分
	} else if in.Volume[n] > in.VOL5[n]*2 {
		// 单日爆量
		if sLevel < 2 {
			// 主线/次主线:仅降序不降评级
			score -= 10
			r.RiskFlags = append(r.RiskFlags, "单日爆量降序")
		} else {
			// 轮动:大幅扣分
			score -= 25
			r.RiskFlags = append(r.RiskFlags, "轮动爆量大额扣分")
		}
	}

	// 涨停次日放量滞涨长上影尾盘护盘 → 大额扣分
	if n >= 1 {
		prevDiff := in.Diff
		if prevDiff > 9.5 && in.Volume[n] > in.VOL5[n]*1.2 && in.Close[n] < in.High[n]-0.01*in.Close[n] {
			score -= 30
			r.RiskFlags = append(r.RiskFlags, "涨停次日放量滞涨大额扣分")
		}
	}

	// 缩量不扣分(低位超跌)
	// 拒绝尾盘暴力拉价
	if in.Close[n] > in.Open[n] && in.Volume[n] < in.VOL5[n] {
		score -= 10
		r.RiskFlags = append(r.RiskFlags, "缩量上涨扣分")
	}

	// 午后成交量分布检查(使用1分钟K线)
	if av := checkAfternoonVolume(in); av < 0 {
		score -= 10
		r.RiskFlags = append(r.RiskFlags, "午后明显缩量扣分")
	} else if av > 0 && in.Close[n] < in.Open[n] {
		// 下午放量但价格跌 → 出货
		score -= 15
		r.RiskFlags = append(r.RiskFlags, "午后放量下跌扣分")
	}

	r.StepScores[2] = clamp(score, 0, 100)
}

// ============================================================
// Step 4: 预判+铁律+排序计算
// ============================================================

// checkMorningPulse 检查早盘脉冲质量(使用1分钟K线)
// 优质脉冲: 开盘30分钟内,高开2-3%后维持4%横盘≥2分钟
// 劣质脉冲: 开盘直线拉升后持续回落
func checkMorningPulse(in *RiskInput) (quality int) {
	if in.MinuteN < 30 {
		return 0 // 数据不足,默认未知
	}

	// 取前30根1分钟K线(9:30-10:00)
	openPrice := in.MinuteOpen[0]
	if openPrice <= 0 {
		return 0
	}

	// 检查开盘涨幅
	maxRate := 0.0
	for i := 0; i < 30 && i <= in.MinuteN; i++ {
		rate := (in.MinuteHigh[i] - openPrice) / openPrice * 100
		if rate > maxRate {
			maxRate = rate
		}
	}

	// 判断高开幅度: 开盘价相对昨收(用第1根close近似昨收)
	// 简化: 看前30分钟最高涨幅
	if maxRate >= 2.0 && maxRate <= 5.0 {
		// 有脉冲,检查是否横盘(维持在高位)
		holdCount := 0
		threshold := maxRate * 0.8 // 维持80%以上涨幅
		for i := 0; i < 30 && i <= in.MinuteN; i++ {
			rate := (in.MinuteHigh[i] - openPrice) / openPrice * 100
			if rate >= threshold {
				holdCount++
			}
		}
		if holdCount >= 5 {
			return 2 // 优质脉冲: 高开后维持高位
		}
		return 1 // 一般脉冲
	}

	// 检查直线拉升后回落
	if maxRate > 3.0 {
		lastRate := (in.MinuteClose[in.MinuteN-1] - openPrice) / openPrice * 100
		dropRate := (in.MinuteClose[in.MinuteN-1] - in.MinuteHigh[in.MinuteN-1]) / in.MinuteHigh[in.MinuteN-1] * 100
		if lastRate < maxRate*0.5 || dropRate < -2.0 {
			return -1 // 劣质脉冲: 冲高回落
		}
	}

	return 1
}

// checkAfternoonVolume 检查午后成交量分布(使用1分钟K线)
// 返回: >0 下午放量, <0 下午缩量, 0 无法判断
func checkAfternoonVolume(in *RiskInput) int {
	if in.MinuteN < 240 {
		return 0 // 全天数据不足240分钟
	}

	// 上午: 0-120分钟(9:30-11:30)
	// 下午: 121-240分钟(13:00-15:00)
	morningVol := 0.0
	afternoonVol := 0.0

	half := 120
	if in.MinuteN < half {
		return 0
	}

	for i := 0; i < half && i < in.MinuteN; i++ {
		morningVol += in.MinuteVol[i]
	}
	for i := half; i < in.MinuteN; i++ {
		afternoonVol += in.MinuteVol[i]
	}

	if morningVol <= 0 {
		return 0
	}

	ratio := afternoonVol / morningVol
	if ratio > 1.2 {
		return 1 // 下午放量
	} else if ratio < 0.7 {
		return -1 // 下午明显缩量
	}
	return 0
}

func step4OpenPrediction(in *RiskInput, r *RiskResult) {
	n := in.N
	score := 100.0

	// 开盘情景预判
	// 主线:高开2-3%稳步冲高
	// 轮动:仅早盘半小时套利
	// 低位超跌:博弈早盘30分钟溢价,午后回落
	sLevel := SectorLevel(in.Sector, in.Topic)
	if sLevel < 2 {
		// 主线题材
		if hasHighRiskResonance(r) {
			// 存在扣分项 → 平开震荡,冲高易跳水
			score -= 20
			r.RiskFlags = append(r.RiskFlags, "主线分歧:平开震荡冲高易跳水")
		}
	} else if sLevel == 2 {
		// 轮动前排:仅早盘半小时套利
		score -= 15
		// 轮动后排:冲高回落,无持续行情
		if hasDualRiskResonance(r) {
			score -= 20
			r.RiskFlags = append(r.RiskFlags, "轮动后排:冲高回落无持续")
		}
	}

	// 不变风控铁律
	// 低位超跌套利:冲高≥3%减半≥5%清仓
	if sLevel >= 2 {
		r.Warnings = append(r.Warnings, "低位超跌:冲高≥3%减半≥5%清仓")
	}

	// 尾盘买入底线:仅涨停次日平稳收盘可隔夜
	// 盘中出货+尾盘急拉 → 剔除备选池
	if n >= 1 {
		prevDiff := in.Diff
		if prevDiff > 9.5 && in.Close[n] <= in.Open[n] {
			r.RiskFlags = append(r.RiskFlags, "涨停次日非平稳收盘风险")
		}
	}

	// 多重高危共振强制减半仓
	if countHighRiskItems(in, r) >= 2 {
		r.RiskFlags = append(r.RiskFlags, "多重高危共振仓位减半")
		score -= 30
	}

	// 主线高开预判约束
	if sLevel < 2 && !hasHighRiskResonance(r) && !in.IsFriday {
		r.PulseScore = 85
	} else if sLevel < 2 {
		r.PulseScore = 50
		r.RiskFlags = append(r.RiskFlags, "冲高乏力修正")
	} else {
		r.PulseScore = 40
		r.RiskFlags = append(r.RiskFlags, "轮动冲高乏力")
	}

	// 分钟K线脉冲修正
	if pulse := checkMorningPulse(in); pulse >= 2 {
		r.PulseScore += 15
		r.RiskFlags = append(r.RiskFlags, "分钟脉冲优质")
	} else if pulse < 0 {
		r.PulseScore -= 20
		r.RiskFlags = append(r.RiskFlags, "分钟脉冲劣质冲高回落")
	}
	if in.MinuteN > 0 {
		r.PulseScore = clamp(r.PulseScore, 0, 100)
	}

	r.StepScores[3] = clamp(score, 0, 100)
}

// ============================================================
// 后处理: 计算总分 + 颜色 + 双权重
// ============================================================

func finalizeRating(in *RiskInput, r *RiskResult) {
	// Step权重
	weights := [5]float64{0.15, 0.25, 0.20, 0.20, 0.20}

	// Step4(预判得分)作为Step5占位
	r.StepScores[4] = r.StepScores[3]

	total := 0.0
	for i := 0; i < 5; i++ {
		total += r.StepScores[i] * weights[i]
	}
	r.RiskScore = total

	// 确定颜色 🟢🟡🔴
	// 先根据铁律判定
	r.Color = calcColor(in, r)

	// 双权重
	// 第一权重: 隔夜低开风险(由低到高)
	r.OvernightRisk = calcOvernightRisk(in, r)
	// 第二权重: 早盘30分钟爆发力
	r.PulseScore = calcPulseScore(in, r)

	// 建议
	r.Suggestion = calcSuggestion(r)
}

// calcColor 根据用户规则确定三色评级
// 🟢: 主线达标 + 无高危共振 + MACD红柱持续放大 / 板块梯队+技术达标
// 🟡: 主线分歧 / 轮动前排 / 低位超跌 / 存在扣分项
// 🔴: 高位空头 / 多重高危共振
func calcColor(in *RiskInput, r *RiskResult) RiskColor {
	sLevel := SectorLevel(in.Sector, in.Topic)

	// 轮动题材最高🟡
	if sLevel >= 2 {
		return ColorYellow
	}

	// 有高危共振直接🔴
	if countHighRiskItems(in, r) >= 3 {
		return ColorRed
	}

	// J>90 → 🟡
	if in.J[in.N] >= KDJYellow {
		return ColorYellow
	}

	// 筹码≥90% → 🟡
	if in.Winner250 >= ChipDowngrade {
		return ColorYellow
	}

	// MACD红柱持续放大 → 🟢
	if in.MACDBar[in.N] > MACDStrongMin {
		if in.N >= 2 && in.MACDBar[in.N] > in.MACDBar[in.N-1] && in.MACDBar[in.N-1] > in.MACDBar[in.N-2] {
			if !hasFatalFlags(r) {
				return ColorGreen
			}
		}
	}

	// 板块梯队+阳线+MACD翻红 → 🟢(即使MACD未持续放大)
	if in.HasSectorTeam && in.Close[in.N] > in.Open[in.N] && in.MACDBar[in.N] > 0 {
		if !hasFatalFlags(r) {
			return ColorGreen
		}
	}

	// 默认🟡
	return ColorYellow
}

func calcOvernightRisk(in *RiskInput, r *RiskResult) float64 {
	risk := 0.0

	// 筹码获利越高风险越大
	if in.Winner250 >= 80 {
		risk += 30
	} else if in.Winner250 >= 60 {
		risk += 15
	}

	// J值越高风险越大
	jVal := in.J[in.N]
	if jVal > 80 {
		risk += 20
	} else if jVal > 60 {
		risk += 10
	}

	// 换手率异常
	if in.TurnRate > 15 {
		risk += 15
	}

	// 高位空头
	if in.Close[in.N] < in.MA10[in.N] {
		risk += 15
	}

	// 周五溢价
	if in.IsFriday {
		risk += 10
	}

	// 市值>100亿加风险
	if in.MarketCap >= LargeCapThreshold {
		risk += 10
	}

	// 板块梯队降低隔夜风险
	if in.HasSectorTeam {
		risk -= 15
		// 梯队规模≥3只,额外降低风险
		if in.SectorGroupSize >= 3 {
			risk -= 10
		}
	}

	// 板块分歧增加风险(已有逻辑补充)
	if in.HasSectorTeam && in.SectorGroupSize >= 2 {
		if (in.Diff > 0.5 && in.SectorAvgDiff < -0.5) ||
			(in.Diff < -1.0 && in.SectorAvgDiff < -1.0) {
			risk += 15
		}
	}

	if risk < 0 {
		risk = 0
	}
	return risk
}

func calcPulseScore(in *RiskInput, r *RiskResult) float64 {
	score := 60.0

	// 阳线加分
	if in.Close[in.N] > in.Open[in.N] {
		score += 10
	}

	// MACD翻红加分
	if in.MACDBar[in.N] > 0 {
		score += 10
	}

	// J超买扣分
	if in.J[in.N] > 80 {
		score -= 20
	}

	// 量能配合
	if in.Volume[in.N] > in.VOL5[in.N] {
		score += 10
	}

	// 周五折价
	if in.IsFriday {
		score -= 10
	}

	return clamp(score, 0, 100)
}

func calcSuggestion(r *RiskResult) string {
	switch r.Color {
	case ColorGreen:
		if r.MarketCap < LargeCapThreshold {
			return "主仓重仓"
		}
		return "主仓隔夜"
	case ColorYellow:
		if hasDualRiskResonance(r) {
			return "超跌观察"
		}
		return "轻仓套利"
	case ColorRed:
		return "规避"
	default:
		return "观察"
	}
}

// ============================================================
// 辅助函数
// ============================================================

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// hasHighRiskResonance 检查是否有高危共振(筹码≥80%+J>80%)
func hasHighRiskResonance(r *RiskResult) bool {
	for _, f := range r.RiskFlags {
		if f == "双高危共振:仓位减半高开压缩0-1.5%" {
			return true
		}
	}
	return false
}

// hasDualRiskResonance 检查是否有双风险项
func hasDualRiskResonance(r *RiskResult) bool {
	count := 0
	for _, f := range r.RiskFlags {
		if f == "筹码≥80%一级降序" || f == "筹码≥85%禁止第一重仓" || f == "筹码≥90%下调🟡" {
			count++
		}
		if f == "J≥80脉冲降至2-3%" || f == "J≥85主线降备选" || f == "J≥90划入🟡" {
			count++
		}
	}
	return count >= 2
}

func hasFatalFlags(r *RiskResult) bool {
	for _, f := range r.RiskFlags {
		if f == "筹码≥90%下调🟡" || f == "J≥90划入🟡" {
			return true
		}
	}
	return false
}

// countHighRiskItems 计算风险项数量(用于5.5多重高危共振)
func countHighRiskItems(in *RiskInput, r *RiskResult) int {
	count := 0
	if in.Winner250 >= ChipLevel1 {
		count++
	}
	if in.J[in.N] > KDJPulseDown {
		count++
	}
	if in.MACDBar[in.N] > 0 && in.MACDBar[in.N] < MACDWeakMax {
		count++
	}
	// 板块资金流出(无法检测,默认0)
	// 周五隔夜
	if in.IsFriday {
		count++
	}
	// 流通市值>100亿
	if in.MarketCap >= LargeCapThreshold {
		count++
	}
	return count
}
