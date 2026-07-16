package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"stock-strategy/internal/riskcontrol"
)

// BridgeOutput Go→Python 桥接输出结构
type BridgeOutput struct {
	Date       string             `json:"date"`
	MACDOK     bool               `json:"macd_ok"`
	Candidates []CandidateOutput  `json:"candidates"`
}

// CandidateOutput 单个候选的输出（剔除字段冗余后，仅保留 Python 执行层需要的字段）
type CandidateOutput struct {
	Code          string   `json:"code"`           // 聚宽格式，如 000001.XSHE
	Name          string   `json:"name"`
	ClosePrice    float64  `json:"close_price"`
	Score         float64  `json:"score"`
	Color         string   `json:"color"`          // green / yellow / red
	Suggestion    string   `json:"suggestion"`
	Reasons       []string `json:"reasons"`
	RiskFlags     []string `json:"risk_flags"`
	Warnings      []string `json:"warnings"`
	OvernightRisk float64  `json:"overnight_risk"`
	PulseScore    float64  `json:"pulse_score"`
	MarketCap     float64  `json:"market_cap"`
}

// BuildBridgeOutput 从风控结果构建桥接输出
func BuildBridgeOutput(dateStr string, macdOK bool, summary *riskcontrol.RiskSummary) *BridgeOutput {
	out := &BridgeOutput{
		Date:       dateStr,
		MACDOK:     macdOK,
		Candidates: make([]CandidateOutput, 0),
	}

	if summary == nil {
		return out
	}

	for _, r := range summary.Results {
		out.Candidates = append(out.Candidates, CandidateOutput{
			Code:          codeToJQ(r.Code),
			Name:          r.Name,
			ClosePrice:    r.ClosePrice,
			Score:         r.RiskScore,
			Color:         colorToString(r.Color),
			Suggestion:    r.Suggestion,
			Reasons:       r.Reasons,
			RiskFlags:     r.RiskFlags,
			Warnings:      r.Warnings,
			OvernightRisk: r.OvernightRisk,
			PulseScore:    r.PulseScore,
			MarketCap:     r.MarketCap,
		})
	}

	return out
}

// OutputJSON 输出 JSON 到 stdout
func (o *BridgeOutput) OutputJSON() {
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		fmt.Printf(`{"error": %q}`, err.Error())
		return
	}
	fmt.Println(string(data))
}

// codeToJQ 将 6 位纯代码转为聚宽格式
// 000001 → 000001.XSHE
// 600036 → 600036.XSHG
// 300750 → 300750.XSHE
// 688xxx → 688xxx.XSHG
func codeToJQ(code string) string {
	bare := code
	if len(code) >= 8 {
		// 已经带前缀 sh/sz，剥离后判断
		bare = code[2:]
	}
	if len(bare) < 6 {
		return code
	}
	bare = bare[:6]

	switch {
	case strings.HasPrefix(bare, "6"):
		return bare + ".XSHG"
	case strings.HasPrefix(bare, "0") || strings.HasPrefix(bare, "3"):
		return bare + ".XSHE"
	case strings.HasPrefix(bare, "8") || strings.HasPrefix(bare, "4"):
		return bare + ".BJ"
	default:
		return bare
	}
}

func colorToString(c riskcontrol.RiskColor) string {
	switch c {
	case riskcontrol.ColorGreen:
		return "green"
	case riskcontrol.ColorYellow:
		return "yellow"
	case riskcontrol.ColorRed:
		return "red"
	default:
		return "unknown"
	}
}
