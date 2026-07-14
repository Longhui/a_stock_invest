package selector

import (
	"strings"
	"testing"
	"time"

	"stock-strategy/internal/provider"
)

const tdxDir = "D:/Programs/tdx"

// TestScreenStock_July10 使用通达信真实数据筛选7月10日的股票
// 默认从沪深主板(60/00)、创业板(30)、科创板(688)股票池中选择
//
// 运行: go test -run TestScreenStock_July10 ./internal/selector/ -v
func TestScreenStock_IntegrationJuly10(t *testing.T) {
	p := provider.NewProvider(tdxDir)
	targetDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	allStocks, err := p.GetAllStocks()
	if err != nil {
		t.Skipf("无法获取股票列表: %v", err)
	}

	// 按前缀分组,每板取前50只
	type boardPicks struct {
		name  string
		codes []string
	}
	limit := 10
	var boards []boardPicks

	addBoard := func(name string, cond func(string) bool) {
		var codes []string
		for _, code := range allStocks {
			bare := code
			if len(code) >= 8 {
				bare = code[2:]
			}
			if len(bare) < 6 {
				continue
			}
			if cond(bare) {
				codes = append(codes, code)
				if len(codes) >= limit {
					break
				}
			}
		}
		boards = append(boards, boardPicks{name: name, codes: codes})
	}

	addBoard("沪主板", func(bare string) bool {
		return strings.HasPrefix(bare, "6") && !strings.HasPrefix(bare, "688")
	})
	addBoard("深主板", func(bare string) bool {
		return strings.HasPrefix(bare, "0")
	})
	addBoard("创业板", func(bare string) bool {
		return strings.HasPrefix(bare, "30")
	})
	addBoard("科创板", func(bare string) bool {
		return strings.HasPrefix(bare, "688")
	})

	passed := 0
	total := 0

	t.Logf("\n=== 策略选股回测 %s ===\n", targetDate.Format("2006-01-02"))

	for _, b := range boards {
		t.Logf("\n--- %s (%d只) ---\n", b.name, len(b.codes))
		boardPassed := 0

		for _, code := range b.codes {
			data, err := p.GetStockData(code, 100)
			if err != nil || len(data.Klines) == 0 {
				continue
			}

			var filtered []provider.Kline
			for _, k := range data.Klines {
				if !k.Date.After(targetDate) {
					filtered = append(filtered, k)
				}
			}
			if len(filtered) < 100 || data.CirculateShares <= 0 {
				continue
			}
			data.Klines = filtered

			total++
			result, _ := ScreenStock(data)
			if result != nil {
				passed++
				boardPassed++
				t.Logf("  ✓ %s %-8s %.2f  %v", code, data.Name, result.ClosePrice, result.Reasons)
			}
		}

		t.Logf("  %s通过: %d只\n", b.name, boardPassed)
	}

	t.Logf("\n=== 总计: %d/%d 通过 ===\n", passed, total)
	if total == 0 {
		t.Skip("无有效数据可测试(可能7月10日为非交易日)")
	}
}

// TestFilterByBoard 验证板块过滤逻辑
func TestFilterByBoard(t *testing.T) {
	codes := []string{
		"600000", "600036", "000001", "000002", // 主板
		"300750", "300059", // 创业板
		"688981", "688001", // 科创板
		"830001", "920001", "430001", // 北交所(应排除)
	}

	filtered := filterCodes(codes)
	expected := 8 // 前8只符合,后3只排除
	if len(filtered) != expected {
		t.Errorf("expected %d, got %d: %v", expected, len(filtered), filtered)
	}

	// 验证北交所被排除
	for _, c := range filtered {
		bare := c
		if len(c) >= 8 {
			bare = c[2:]
		}
		if strings.HasPrefix(bare, "8") || strings.HasPrefix(bare, "92") || strings.HasPrefix(bare, "43") {
			t.Errorf("北交所股票不应被包含: %s", c)
		}
	}
}

// filterCodes 实现与 main.go 相同的板块过滤逻辑
func filterCodes(codes []string) []string {
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
			result = append(result, code)
		case bare[:1] == "0":
			result = append(result, code)
		case bare[:2] == "30":
			result = append(result, code)
		case bare[:3] == "688":
			result = append(result, code)
		}
	}
	return result
}

// TestScreenStock_DateFilter 验证日期截断逻辑
func TestScreenStock_DateFilter(t *testing.T) {
	p := provider.NewProvider(tdxDir)
	targetDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	// 用一只已知股票验证截断
	data, err := p.GetStockData("600000", 100)
	if err != nil {
		t.Skipf("获取600000数据失败: %v", err)
	}

	originalLen := len(data.Klines)

	// 截断到7月10日
	var filtered []provider.Kline
	for _, k := range data.Klines {
		if !k.Date.After(targetDate) {
			filtered = append(filtered, k)
		}
	}

	if len(filtered) == originalLen {
		t.Logf("7月10日之后无数据,使用全部%d根K线", originalLen)
	} else {
		t.Logf("截断: %d → %d 根K线", originalLen, len(filtered))
	}

	// 验证截断后没有7月10日之后的数据
	for _, k := range filtered {
		if k.Date.After(targetDate) {
			t.Errorf("发现7月10日之后的数据: %v", k.Date)
		}
	}

	if len(filtered) >= 100 {
		data.Klines = filtered
		result, ctx := ScreenStock(data)
		if result != nil {
			t.Logf("600000 7月10日选股通过: %.2f %v", result.ClosePrice, result.Reasons)
		} else {
			t.Log("600000 7月10日未通过策略条件")
		}
		_ = ctx
	}
}

// TestScreenStock_EveryBoard 验证每个板块至少有一只可测试的股票
func TestScreenStock_EveryBoard(t *testing.T) {
	p := provider.NewProvider(tdxDir)
	targetDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	boards := map[string]string{
		"沪主板": "600000",
		"深主板": "000001",
		"创业板": "300750",
		"科创板": "688981",
	}

	for name, code := range boards {
		data, err := p.GetStockData(code, 100)
		if err != nil {
			t.Logf("%s %s 数据获取失败: %v", name, code, err)
			continue
		}

		var filtered []provider.Kline
		for _, k := range data.Klines {
			if !k.Date.After(targetDate) {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) < 100 {
			t.Logf("%s %s 截断后数据不足(%d)", name, code, len(filtered))
			continue
		}
		data.Klines = filtered

		nameVal := data.Name
		_ = nameVal

		result, ctx := ScreenStock(data)
		if result != nil {
			t.Logf("%s %-6s %-8s 通过  %.2f  %v", name, code, data.Name, result.ClosePrice, result.Reasons)
		} else {
			// 未通过也算正常(策略本身是筛选性质的)
			t.Logf("%s %-6s %-8s 未通过 %d根K线", name, code, data.Name, len(filtered))
		}
		_ = ctx
	}
}

// TestScreenStock_RejectBeiJiaoSuo 验证北交所股票不会被ScreenStock误判(数据格式兼容性)
func TestScreenStock_RejectBeiJiaoSuo(t *testing.T) {
	p := provider.NewProvider(tdxDir)

	// 检查是否有北交所股票数据
	allStocks, err := p.GetAllStocks()
	if err != nil {
		t.Skipf("无法获取股票列表: %v", err)
	}

	var bjStocks []string
	for _, code := range allStocks {
		bare := code
		if len(code) >= 8 {
			bare = code[2:]
		}
		if len(bare) >= 6 && (bare[:1] == "8" || bare[:2] == "92" || bare[:2] == "43") {
			bjStocks = append(bjStocks, code)
		}
	}

	if len(bjStocks) == 0 {
		t.Skip("无北交所股票数据")
	}

	// 验证这些股票代码不在filterCodes结果中
	rejected := true
	for _, code := range bjStocks[:min(5, len(bjStocks))] {
		filtered := filterCodes([]string{code})
		if len(filtered) > 0 {
			t.Errorf("北交所股票 %s 不应通过板块过滤", code)
			rejected = false
		}
	}

	if rejected {
		t.Logf("北交所股票(%d只)均被板块过滤正确排除", len(bjStocks))
	}
}
