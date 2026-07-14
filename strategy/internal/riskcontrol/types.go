package riskcontrol

// RiskColor 三色评级
type RiskColor int

const (
	ColorRed    RiskColor = iota // 🔴 高位空头/多重共振
	ColorYellow                  // 🟡 震荡/分歧/轮动
	ColorGreen                   // 🟢 主线看涨
)

func (c RiskColor) Label() string {
	switch c {
	case ColorGreen:
		return "🟢看涨"
	case ColorYellow:
		return "🟡震荡"
	case ColorRed:
		return "🔴易跌"
	default:
		return "⚪未知"
	}
}

func (c RiskColor) Tag() string {
	switch c {
	case ColorGreen:
		return "🟢"
	case ColorYellow:
		return "🟡"
	case ColorRed:
		return "🔴"
	default:
		return "⚪"
	}
}

// RiskInput 单只股票风控输入数据
// 由 main.go 的适配函数从 selector.Context 提取
type RiskInput struct {
	Code       string
	Name       string
	ClosePrice float64
	MarketCap  float64 // 流通市值(元) = 价格 * 流通股本
	OrigScore  float64 // 原始策略评分
	Reasons    []string

	// 标量值
	TurnRate  float64 // 换手率%
	Winner250 float64 // 筹码获利比例%
	Diff      float64 // 最新涨跌幅%

	// 完整序列(引用ctx底层数组,不拷贝)
	N          int
	Close      []float64
	Open       []float64
	High       []float64
	Low        []float64
	Volume     []float64
	Amount     []float64
	MA5        []float64
	MA10       []float64
	VOL5       []float64
	DIF        []float64
	DEA        []float64
	MACDBar    []float64
	K          []float64
	D          []float64
	J          []float64

	// 板块分类
	Sector string
	Topic  string

	// 板块上下文(由 engine.go 预处理填充)
	SectorGroupSize int     // 同板块选股结果数量(≥2表示有梯队)
	HasSectorTeam   bool    // 策略结果中是否有同板块队友
	SectorAvgDiff   float64 // 同板块策略股的平均涨跌幅(用于分歧检测)
	SectorMainFlow  float64 // 板块主力净流入(元), 正=流入, 负=流出
	SectorLimitUp   int     // 板块内涨停个股数量

	// 外部标记
	IsFriday bool // 是否周五

	// 1分钟K线(用于脉冲分析/尾盘拉升判断)
	MinuteN     int
	MinuteClose []float64
	MinuteOpen  []float64
	MinuteHigh  []float64
	MinuteLow   []float64
	MinuteVol   []float64
	MinuteTime  []int64 // Unix时间戳
}

// RiskResult 单只股票风控结果
type RiskResult struct {
	Code       string
	Name       string
	ClosePrice float64
	MarketCap  float64
	OrigScore  float64
	Reasons    []string

	Color     RiskColor
	RiskScore float64 // 总分(0-100,越高越安全)
	RiskFlags []string // 触发的风险标记
	Warnings  []string // 风险提示文字

	Sector string
	Topic  string

	// Step明细
	StepScores [5]float64

	// 双权重排序
	OvernightRisk float64 // 第一权重:隔夜低开风险(越低越好)
	PulseScore    float64 // 第二权重:早盘30分钟爆发力(越高越好)

	// 最终建议
	Suggestion string // 主仓重仓/轻仓套利/超跌观察/规避
}

// RiskSummary 风控汇总
type RiskSummary struct {
	TotalStocks int
	Results     []*RiskResult
}

func (s *RiskSummary) GreenCount() int {
	n := 0
	for _, r := range s.Results {
		if r.Color == ColorGreen {
			n++
		}
	}
	return n
}

func (s *RiskSummary) YellowCount() int {
	n := 0
	for _, r := range s.Results {
		if r.Color == ColorYellow {
			n++
		}
	}
	return n
}

func (s *RiskSummary) RedCount() int {
	n := 0
	for _, r := range s.Results {
		if r.Color == ColorRed {
			n++
		}
	}
	return n
}
