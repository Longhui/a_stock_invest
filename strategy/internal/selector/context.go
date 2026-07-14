package selector

import (
	"stock-strategy/internal/provider"
)

// StockResult 选股结果
type StockResult struct {
	Code         string
	Name         string
	Reasons      []string  // 满足条件说明
	Score        float64
	ClosePrice   float64
}

// Context 单只股票的运算上下文(一次计算、多次复用)
type Context struct {
	Data   *provider.StockData
	Klines []provider.Kline
	N      int // 最新K线索引(len-1)

	// === 预计算的公共变量 ===
	Close   []float64
	Open    []float64
	High    []float64
	Low     []float64
	Volume  []float64
	Amount  []float64
	MA5     []float64
	MA10    []float64
	VOL5    []float64
	Diff    []float64 // 涨跌幅

	// MACD
	DIF      []float64
	DEA      []float64
	MACDBar  []float64

	// KDJ
	K []float64
	D []float64
	J []float64
}

func NewContext(data *provider.StockData) *Context {
	klines := data.Klines
	n := len(klines)
	if n == 0 {
		return nil
	}

	// 拷贝K线数据到切片
	closeP := make([]float64, n)
	openP := make([]float64, n)
	highP := make([]float64, n)
	lowP := make([]float64, n)
	vol := make([]float64, n)
	amount := make([]float64, n)
	diff := make([]float64, n)

	for i, k := range klines {
		closeP[i] = k.Close
		openP[i] = k.Open
		highP[i] = k.High
		lowP[i] = k.Low
		vol[i] = k.Volume
		amount[i] = k.Amount
		if i > 0 {
			diff[i] = (closeP[i] - closeP[i-1]) / closeP[i-1] * 100
		}
	}

	// 均线
	ma5 := calcMA(closeP, 5)
	ma10 := calcMA(closeP, 10)
	vol5 := calcMA(vol, 5)

	// MACD: DIF = EMA(CLOSE,12) - EMA(CLOSE,26)
	ema12 := calcEMA(closeP, 12)
	ema26 := calcEMA(closeP, 26)
	dif := make([]float64, n)
	for i := 0; i < n; i++ {
		dif[i] = ema12[i] - ema26[i]
	}
	deaVal := calcEMA(dif, 9)
	macdBar := make([]float64, n)
	for i := 0; i < n; i++ {
		macdBar[i] = (dif[i] - deaVal[i]) * 2
	}

	// KDJ
	rsv := calcRSV(closeP, highP, lowP, 9)
	k := calcSMA(rsv, 3, 1)
	d := calcSMA(k, 3, 1)
	j := make([]float64, n)
	for i := 0; i < n; i++ {
		j[i] = 3*k[i] - 2*d[i]
	}

	return &Context{
		Data:    data,
		Klines:  klines,
		N:       n - 1,
		Close:   closeP,
		Open:    openP,
		High:    highP,
		Low:     lowP,
		Volume:  vol,
		Amount:  amount,
		Diff:    diff,
		MA5:     ma5,
		MA10:    ma10,
		VOL5:    vol5,
		DIF:     dif,
		DEA:     deaVal,
		MACDBar: macdBar,
		K:       k,
		D:       d,
		J:       j,
	}
}

func (ctx *Context) Last() provider.Kline   { return ctx.Klines[ctx.N] }
func (ctx *Context) Ref(idx int) provider.Kline {
	if ctx.N-idx < 0 {
		return provider.Kline{}
	}
	return ctx.Klines[ctx.N-idx]
}

// TurnRate 换手率(%) = 成交量(股) / 流通股本(股) * 100
func (ctx *Context) TurnRate(idx int) float64 {
	if ctx.Data.CirculateShares <= 0 {
		return 0
	}
	return ctx.Volume[idx] / ctx.Data.CirculateShares * 100
}

// Winner 简化版筹码获利比例: 最近N日内收盘价低于当前收盘价的比例
func (ctx *Context) Winner(idx int, lookback int) float64 {
	if idx < lookback {
		lookback = idx
	}
	currentClose := ctx.Close[idx]
	count := 0
	for i := idx - lookback; i <= idx; i++ {
		if i >= 0 && ctx.Close[i] <= currentClose {
			count++
		}
	}
	return float64(count) / float64(lookback+1) * 100
}

// Cross 判断是否金叉(上穿)
func Cross(a, b []float64, idx int) bool {
	if idx <= 0 {
		return false
	}
	return a[idx-1] <= b[idx-1] && a[idx] > b[idx]
}

// === 内部计算函数 ===
func calcMA(data []float64, period int) []float64 {
	n := len(data)
	result := make([]float64, n)
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += data[i]
		if i >= period-1 {
			if i >= period {
				sum -= data[i-period]
			}
			result[i] = sum / float64(period)
		}
	}
	return result
}

func calcEMA(data []float64, period int) []float64 {
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

func calcSMA(data []float64, period, weight int) []float64 {
	n := len(data)
	if n == 0 {
		return nil
	}
	result := make([]float64, n)
	result[0] = data[0]
	for i := 1; i < n; i++ {
		result[i] = (float64(weight)*data[i] + float64(period-weight)*result[i-1]) / float64(period)
	}
	return result
}

func calcRSV(close, high, low []float64, period int) []float64 {
	n := len(close)
	result := make([]float64, n)
	for i := period - 1; i < n; i++ {
		hh := high[i-period+1]
		ll := low[i-period+1]
		for j := i - period + 2; j <= i; j++ {
			if high[j] > hh {
				hh = high[j]
			}
			if low[j] < ll {
				ll = low[j]
			}
		}
		if hh == ll {
			result[i] = 50
		} else {
			result[i] = (close[i] - ll) / (hh - ll) * 100
		}
	}
	return result
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// CheckLastZT 检查昨日是否涨停
// 沪/深主板: >=9.8% 且 C=H; 创业板/科创板: >=19.5% 且 C=H
func CheckLastZT(diff float64, isChiNext bool) bool {
	threshold := 9.8
	if isChiNext {
		threshold = 19.5
	}
	return diff >= threshold
}
