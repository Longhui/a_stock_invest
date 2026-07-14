package riskcontrol

import "sort"

// ProcessAll 执行完整风控流水线
// 输入: RiskInput 列表(由 main.go 的适配函数构建)
// 输出: RiskSummary(含排序后的 RiskResult 列表 + 统计)
func ProcessAll(inputs []*RiskInput) *RiskSummary {
	// ===== 预处理: 板块分组 =====
	// 先填充板块分类(如 main.go 未传入,用启发式回退)
	for _, in := range inputs {
		if in.Sector == "" {
			in.Sector = ClassifySector(in.Code)
		}
		if in.Topic == "" {
			in.Topic = ClassifyTopic(in.Code)
		}
	}

	// 按板块分组,计算板块上下文信息
	sectorGroups := make(map[string][]*RiskInput)
	for _, in := range inputs {
		sectorGroups[in.Sector] = append(sectorGroups[in.Sector], in)
	}
	for _, group := range sectorGroups {
		if len(group) < 2 {
			continue
		}
		// 计算同板块平均涨跌幅
		avgDiff := 0.0
		for _, in := range group {
			avgDiff += in.Diff
		}
		avgDiff /= float64(len(group))

		for _, in := range group {
			in.SectorGroupSize = len(group)
			in.HasSectorTeam = true
			in.SectorAvgDiff = avgDiff
		}
	}

	// ===== 个股逐只执行五步流水线 =====
	results := make([]*RiskResult, 0, len(inputs))
	for _, in := range inputs {
		r := processAllRules(in)
		results = append(results, r)
	}

	// ===== 双权重排序 =====
	// 第一权重: OvernightRisk 升序(低开风险越低越靠前)
	// 第二权重: PulseScore 降序(爆发力越高越靠前)
	// 附加规则: >100亿中盘股永久排在小盘股后方
	sort.SliceStable(results, func(i, j int) bool {
		ri, rj := results[i], results[j]

		// 大盘股永远排在小盘股后面
		mi := ri.MarketCap >= LargeCapThreshold
		mj := rj.MarketCap >= LargeCapThreshold
		if mi != mj {
			return mj // 小盘股在前
		}

		// 第一权重: 隔夜低开风险(越低越好)
		if ri.OvernightRisk != rj.OvernightRisk {
			return ri.OvernightRisk < rj.OvernightRisk
		}
		// 第二权重: 早盘爆发力(越高越好)
		return ri.PulseScore > rj.PulseScore
	})

	return &RiskSummary{
		TotalStocks: len(results),
		Results:     results,
	}
}
