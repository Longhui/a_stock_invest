package sellcondition

import (
	"testing"
)

func TestCheck_NilData(t *testing.T) {
	sig := Check(100, nil, nil, nil)
	if sig == nil {
		t.Fatal("expected non-nil signal")
	}
	if sig.TakeProfitTriggered || sig.StopLossTriggered {
		t.Error("nil data should not trigger any signal")
	}
}
func TestCheck_StopLoss(t *testing.T) {
	// 价格一路下跌,第4根触碰-5%
	ref := 100.0
	close := []float64{100, 99, 98, 97, 96, 95, 94}
	high := []float64{101, 100, 99, 98, 97, 96, 95}
	low := []float64{99, 98, 97, 96, 95, 94, 93}

	sig := Check(ref, close, high, low)
	if !sig.StopLossTriggered {
		t.Fatal("expected stop loss to trigger")
	}
	if sig.StopLossTime != 4 { // low[4]=95 = ref*0.95
		t.Errorf("expected stop at index 4, got %d", sig.StopLossTime)
	}
	if sig.StopLossPrice != 95 {
		t.Errorf("expected stop price 95, got %.2f", sig.StopLossPrice)
	}
}

func TestCheck_TakeProfit(t *testing.T) {
	// 价格涨2%后从高点回落1.5%,连续4根确认
	// ref=100, 涨2%→102激活, peak=105(最高), 回落1.5%→103.425
	ref := 100.0
	close := []float64{
		100, 101, 102, // idx2: 涨超2%激活⚠
		103, 104, 105, // idx5: 冲到105(peak)
		104, 103.5, 103.2, 103.1, 103.05, 103.0, // 开始回落(103.425以下)
		// idx6-11: close在103-104之间,多数<103.425
		// idx6=104 > 103.425 → 不计数,重置
		// idx7=103.5 > 103.425 → 不计数
		// idx8=103.2 < 103.425 → 计数1
		// idx9=103.1 < 103.425 → 计数2
		// idx10=103.05 < 103.425 → 计数3
		// idx11=103.0 < 103.425 → 计数4 → 触发!
	}
	// 修正: 先缓缓回落到103.425以下,确保连续4根
	close = []float64{
		// 0-2: 冲高
		100, 101, 102,
		// 3-5: 继续涨到顶点
		103, 104, 105,
		// 6: 小幅回落但还在103.425以上,计数重置
		104.5,
		// 7-10: 连续4根在103.425以下 → 触发
		103.3, 103.2, 103.1, 103.0,
	}
	high := []float64{
		100.5, 102, 103,
		104, 105, 106,
		105,
		104, 103.8, 103.5, 103.5,
	}
	low := []float64{
		99.5, 100, 101.5,
		102.5, 103, 104,
		103.8,
		103, 102.8, 102.7, 102.5,
	}

	sig := Check(ref, close, high, low)
	if !sig.TakeProfitTriggered {
		t.Fatal("expected take profit to trigger")
	}
	if sig.TakeProfitTime == 0 {
		t.Fatal("expected take profit time to be set")
	}
}

func TestCheck_NoSignal(t *testing.T) {
	// 价格横盘,既不涨2%也不跌5%
	ref := 100.0
	close := make([]float64, 240)
	high := make([]float64, 240)
	low := make([]float64, 240)
	for i := 0; i < 240; i++ {
		close[i] = 100 + float64(i)*0.001 // 缓慢上涨不到1%
		high[i] = close[i] + 0.1
		low[i] = close[i] - 0.1
	}

	sig := Check(ref, close, high, low)
	if sig.TakeProfitTriggered {
		t.Error("no take profit expected")
	}
	if sig.StopLossTriggered {
		t.Error("no stop loss expected")
	}
}

func TestCheck_StopLossBeforeTakeProfit(t *testing.T) {
	// 先涨1%后跌5%,止损应优先触发
	ref := 100.0
	close := []float64{
		100, 101, 100, 99, 98, 97, 96, 95,
	}
	high := []float64{
		101, 102, 101, 100, 99, 98, 97, 96,
	}
	low := []float64{
		99, 100, 99, 98, 97, 96, 95, 94,
	}

	sig := Check(ref, close, high, low)
	if !sig.StopLossTriggered {
		t.Fatal("expected stop loss to trigger")
	}
	if sig.TakeProfitTriggered {
		t.Error("stop loss should take priority, no take profit")
	}
}

// TestCheck_TakeProfitExact4Bars 精确验证4根连续确认去毛刺
func TestCheck_TakeProfitExact4Bars(t *testing.T) {
	ref := 100.0

	// 涨到102以上(激活)→ peak=105(精确设定high) → 回落阈值=105*0.985=103.425
	// 只有连续3根<103.425,第4根回到以上 → 不应触发
	close3 := []float64{
		100, 101, 102, 103, 104, 105, // 0-5: peak at 5
		103.3, 103.2, 103.1, // 6-8: 3根 < 103.425
		103.5, // 9: > 103.425 → 重置
		103.2, // 10: 计数1
	}
	high3 := []float64{
		100.3, 101.3, 102.3, 103.3, 104.3, 105.0, // peak精确=105
		103.6, 103.5, 103.4, 103.8, 103.5,
	}
	low3 := []float64{
		99.8, 100.8, 101.8, 102.8, 103.8, 104.8,
		103.1, 103.0, 102.9, 103.3, 103.0,
	}
	sig3 := Check(ref, close3, high3, low3)
	if sig3.TakeProfitTriggered {
		t.Error("only 3 consecutive bars below threshold, should NOT trigger")
	}

	// 连续4根≤103.4 → 应触发
	close4 := []float64{
		100, 101, 102, 103, 104, 105, // 0-5: peak at 5
		103.3, 103.2, 103.1, 103.0, // 6-9: 4根 < 103.425
	}
	high4 := []float64{
		100.3, 101.3, 102.3, 103.3, 104.3, 105.0,
		103.6, 103.5, 103.4, 103.3,
	}
	low4 := []float64{
		99.8, 100.8, 101.8, 102.8, 103.8, 104.8,
		103.1, 103.0, 102.9, 102.8,
	}
	sig4 := Check(ref, close4, high4, low4)
	if !sig4.TakeProfitTriggered {
		t.Error("4 consecutive bars below threshold should trigger")
	}
}
