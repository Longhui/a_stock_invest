package selector

import (
	"math"
	"testing"
	"time"

	"stock-strategy/internal/provider"
)

func TestScreenStock_NilData(t *testing.T) {
	r, ctx := ScreenStock(nil)
	if r != nil || ctx != nil {
		t.Error("nil data should return nil result and ctx")
	}
}

func TestScreenStock_EmptyKlines(t *testing.T) {
	data := &provider.StockData{
		Code:            "600000",
		Name:            "测试",
		Klines:          []provider.Kline{},
		CirculateShares: 1e9,
	}
	r, ctx := ScreenStock(data)
	if r != nil || ctx != nil {
		t.Error("empty klines should return nil")
	}
}

func TestScreenStock_TooFewKlines(t *testing.T) {
	klines := make([]provider.Kline, 10)
	for i := range klines {
		klines[i] = provider.Kline{
			Date:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			Open:   10,
			High:   10.5,
			Low:    9.5,
			Close:  10,
			Volume: 1e7,
		}
	}
	data := &provider.StockData{
		Code:            "600000",
		Name:            "测试",
		Klines:          klines,
		CirculateShares: 1e9,
	}
	r, ctx := ScreenStock(data)
	if r != nil {
		t.Error("too few klines (<60) should return nil result")
	}
	_ = ctx
}

func TestScreenStock_ContextCreated(t *testing.T) {
	// 验证 ScreenStock 正确处理数据并创建运算上下文
	// (策略选股条件复杂,通过与否取决于数据形态,此处只检查基本流程)
	data := makeBasicData()
	r, ctx := ScreenStock(data)
	// ScreenStock 总会尝试创建 Context,但不一定选到股票
	if ctx == nil {
		t.Fatal("expected non-nil context for valid data")
	}
	if ctx.N < 60 {
		t.Errorf("expected at least 60 bars, got %d", len(ctx.Close))
	}
	if ctx.Close[ctx.N] <= 0 {
		t.Error("close price should be positive")
	}
	_ = r // r may be nil if data doesn't match strategy
}

// makeBasicData 构造一组基础K线数据(用于测试流程完整性)
func makeBasicData() *provider.StockData {
	n := 100
	klines := make([]provider.Kline, n)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		price := 10.0 + float64(i)*0.01 // 每日上涨0.01
		klines[i] = provider.Kline{
			Code:   "600000",
			Date:   base.AddDate(0, 0, i),
			Open:   price - 0.02,
			High:   price + 0.03,
			Low:    price - 0.03,
			Close:  price,
			Volume: 1e7,
			Amount: 1e7 * price,
		}
	}

	return &provider.StockData{
		Code:            "600000",
		Name:            "浦发银行",
		Klines:          klines,
		CirculateShares: 1e9,
	}
}

// makePassingData 构造一组满足选股策略的K线数据(供开发调试用)
// 设计思路:
//   - 70根K线(满足最少60根)
//   - 早期(0-24)区间震荡+部分高价,控制筹码获利比例<85%
//   - 中期(25-44)下跌后低位震荡,KDJ回落到低位
//   - 后期(45-64)企稳回升,均线多头
//   - 最后5根: 阴线(65) + 4连阳(66-69)
func makePassingData() *provider.StockData {
	n := 70
	klines := make([]provider.Kline, n)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// 收盘价序列(设计确保筹码获利比例<85%,即至少12根>10.5)
	closes := []float64{
		// 0-9: 震荡,5根>10.5
		10.8, 10.3, 10.9, 10.2, 10.7, 9.8, 10.6, 10.1, 10.5, 10.0,
		// 10-19: 震荡,4根>10.5
		10.9, 10.2, 10.7, 10.1, 10.8, 10.0, 10.6, 10.3, 10.4, 9.9,
		// 20-24: 2根>10.5
		10.7, 10.1, 10.6, 10.2, 10.3,
		// 25-29: 开始回落(均<10.5)
		10.2, 10.0, 9.8, 9.7, 9.6,
		// 30-34: 继续下跌
		9.5, 9.4, 9.3, 9.35, 9.4,
		// 35-39: 低位整固
		9.45, 9.5, 9.48, 9.52, 9.55,
		// 40-44: 企稳回升
		9.6, 9.65, 9.7, 9.75, 9.8,
		// 45-49: 继续回升
		9.85, 9.9, 9.95, 10.0, 10.05,
		// 50-54: 站上10元
		10.1, 10.15, 10.2, 10.22, 10.25,
		// 55-59: 稳步上升
		10.28, 10.3, 10.32, 10.35, 10.38,
		// 60-64: 高位盘整
		10.36, 10.34, 10.32, 10.3, 10.33,
		// 65: 阴线(前置阴线)
		10.28,
		// 66-69: 4连阳
		10.35, 10.40, 10.45, 10.50,
	}
	opens := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	volumes := make([]float64, n)

	for i, c := range closes {
		// 开盘: 阴线开高走低,阳线开低走高
		if i == 65 {
			opens[i] = c + 0.05 // 阴线: 开>收
		} else {
			opens[i] = c - 0.03 // 阳线: 开<收
		}
		if opens[i] < 0 {
			opens[i] = c * 0.99
		}

		// 高低: 保证包含开收
		highs[i] = math.Max(opens[i], c) + 0.05
		lows[i] = math.Min(opens[i], c) - 0.05
		if lows[i] < 0 {
			lows[i] = 0
		}

		// 成交量: 前期缩量,后期放量
		switch {
		case i < 25:
			volumes[i] = 2e7 + float64(i)*1e5 // 2千万~2.25千万
		case i < 40:
			volumes[i] = 1.5e7 // 缩量下跌
		case i < 60:
			volumes[i] = 2.5e7 + float64(i-40)*2e5 // 逐渐放量
		default:
			volumes[i] = 5e7 // 放量突破
		}

		klines[i] = provider.Kline{
			Code:   "600000",
			Date:   base.AddDate(0, 0, i),
			Open:   opens[i],
			High:   highs[i],
			Low:    lows[i],
			Close:  c,
			Volume: volumes[i],
			Amount: volumes[i] * (opens[i] + c) / 2,
		}
	}

	return &provider.StockData{
		Code:            "600000",
		Name:            "浦发银行",
		Klines:          klines,
		CirculateShares: 1e9, // 10亿股
	}
}
