package riskcontrol

import (
	"testing"
)

func TestProcessAll_NilInput(t *testing.T) {
	s := ProcessAll(nil)
	if s == nil {
		t.Fatal("expected non-nil summary")
	}
	if s.TotalStocks != 0 {
		t.Errorf("expected 0 stocks, got %d", s.TotalStocks)
	}
	if len(s.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(s.Results))
	}
}

func TestProcessAll_EmptyInput(t *testing.T) {
	s := ProcessAll([]*RiskInput{})
	if s.TotalStocks != 0 {
		t.Errorf("expected 0 stocks, got %d", s.TotalStocks)
	}
}

func TestProcessAll_GreenRating(t *testing.T) {
	// 构造一个应得🟢评级的输入:
	// - MACD红柱持续放大
	// - 板块梯队支撑
	// - 无高危共振
	n := 100
	closeP := ascending(n, 10.0, 0.02) // 持续小幅上涨
	openP := make([]float64, n)
	highP := make([]float64, n)
	lowP := make([]float64, n)
	for i := 0; i < n; i++ {
		openP[i] = closeP[i] - 0.01
		highP[i] = closeP[i] + 0.02
		lowP[i] = closeP[i] - 0.02
	}

	inputs := []*RiskInput{{
		Code:       "600000",
		Name:       "浦发银行",
		ClosePrice: closeP[n-1],
		MarketCap:  5e9,
		TurnRate:   5.0,
		Winner250:  50.0,
		Diff:       0.5,
		IsFriday:   false,

		N:       n - 1,
		Close:   closeP,
		Open:    openP,
		High:    highP,
		Low:     lowP,
		Volume:  steadyVol(n, 5e7),
		Amount:  steadyAmount(n, 5e7, closeP),
		MA5:     ascending(n, 9.9, 0.02),
		MA10:    ascending(n, 9.8, 0.02),
		VOL5:    steadyVol(n, 3e7),
		DIF:     ascending(n, -0.1, 0.01),
		DEA:     ascending(n, -0.15, 0.01),
		MACDBar: ascending(n, 0.1, 0.01),
		K:       ascending(n, 40, 0.3),
		D:       ascending(n, 38, 0.25),
		J:       ascending(n, 44, 0.4),

		Sector:         "创业板",
		SectorGroupSize: 3,
		HasSectorTeam:   true,
		SectorMainFlow:  2e8,
		SectorLimitUp:   2,
	}}

	s := ProcessAll(inputs)
	if s.TotalStocks != 1 {
		t.Fatalf("expected 1 result, got %d", s.TotalStocks)
	}
	r := s.Results[0]
	if r.Color != ColorGreen {
		t.Errorf("expected 🟢(green), got %s", r.Color.Label())
	}
	if r.Suggestion != "主仓重仓" {
		t.Errorf("expected 主仓重仓, got %s", r.Suggestion)
	}
}

func TestProcessAll_DualResonance_ShouldDowngrade(t *testing.T) {
	// 筹码≥80% + J>80 → 双重高危,不应该是🟢
	n := 100
	closeP := ascending(n, 10.0, 0.01)

	inputs := []*RiskInput{{
		Code:       "000001",
		Name:       "平安银行",
		ClosePrice: closeP[n-1],
		MarketCap:  3e9,
		TurnRate:   8.0,
		Winner250:  85.0, // ≥85% 禁止第一重仓
		Diff:       1.0,

		N:       n - 1,
		Close:   closeP,
		Open:    closeP,
		High:    closeP,
		Low:     closeP,
		Volume:  steadyVol(n, 5e7),
		Amount:  steadyAmount(n, 5e7, closeP),
		MA5:     ascending(n, 9.9, 0.01),
		MA10:    ascending(n, 9.8, 0.01),
		VOL5:    steadyVol(n, 3e7),
		DIF:     ascending(n, 0.1, 0.01),
		DEA:     ascending(n, 0.08, 0.01),
		MACDBar: ascending(n, 0.1, 0.005),
		K:       ascending(n, 70, 0.2),
		D:       ascending(n, 65, 0.2),
		J:       ascending(n, 80, 0.5), // J>80
		Sector:  "银行",
	}}

	s := ProcessAll(inputs)
	if s.TotalStocks != 1 {
		t.Fatalf("expected 1 result, got %d", s.TotalStocks)
	}
	r := s.Results[0]
	if r.Color == ColorGreen {
		t.Error("high risk stock should not be 🟢")
	}
	if r.RiskScore == 100 {
		t.Error("high risk stock should not have perfect score")
	}
	if len(r.RiskFlags) == 0 {
		t.Error("high risk stock should have risk flags")
	}
}

// 辅助: 生成单调递增序列
func ascending(n int, start, step float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = start + float64(i)*step
	}
	return s
}

// 辅助: 生成稳定成交量
func steadyVol(n int, vol float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = vol
	}
	return s
}

// 辅助: 生成成交额
func steadyAmount(n int, vol float64, price []float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = vol * price[i]
	}
	return s
}
