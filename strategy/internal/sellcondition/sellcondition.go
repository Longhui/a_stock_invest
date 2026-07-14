package sellcondition

import "fmt"

// Check 模拟当天分时K线的卖出条件单触发情况
//
// 条件1(止盈): 涨2%后从最高点回落幅度达1.5%,连续满足4根1分钟K线确认去毛刺
// 条件2(止损): 盘中跌超5%立即触发
//
// ref: 参考买入价(当天开盘价)
// close/high/low: 1分钟K线序列(全天约240根)
func Check(ref float64, close, high, low []float64) *Signal {
	s := &Signal{
		ReferencePrice: ref,
	}
	if ref <= 0 || len(close) == 0 {
		return s
	}

	var (
		activated    bool
		peak         float64
		confirmCount int
		tpPrice      float64
	)

	for i := 0; i < len(close); i++ {
		// ===== 止损检查(始终有效) =====
		if !s.StopLossTriggered && low[i] > 0 && low[i] <= ref*0.95 {
			s.StopLossTriggered = true
			s.StopLossPrice = low[i]
			s.StopLossTime = i
		}

		if !activated {
			if close[i] >= ref*1.02 {
				activated = true
				peak = high[i]
			}
		} else {
			if high[i] > peak {
				peak = high[i]
			}

			retraceLevel := peak * 0.985
			if close[i] <= retraceLevel {
				if !s.TakeProfitTriggered {
					tpPrice = close[i]
				}
				confirmCount++
				if confirmCount >= 4 && !s.TakeProfitTriggered {
					s.TakeProfitTriggered = true
					s.TakeProfitPrice = tpPrice
					s.TakeProfitTime = i
				}
			} else {
				confirmCount = 0
			}
		}
	}

	s.ConsecutiveCount = confirmCount
	s.PeakPrice = peak
	return s
}

// Signal 卖出条件单检查结果
type Signal struct {
	ReferencePrice float64

	TakeProfitTriggered bool
	TakeProfitPrice     float64
	TakeProfitTime      int

	StopLossTriggered bool
	StopLossPrice     float64
	StopLossTime      int

	PeakPrice        float64
	ConsecutiveCount int
}

func (s *Signal) String() string {
	line := ""

	if s.TakeProfitTriggered {
		line += fmt.Sprintf("止盈触发 @%.2f(第%dm)", s.TakeProfitPrice, s.TakeProfitTime)
	} else if s.PeakPrice > 0 {
		line += fmt.Sprintf("止盈:涨2%%过,待回落(高位%.2f,已确认%d次)", s.PeakPrice, s.ConsecutiveCount)
	} else {
		line += "止盈:待涨2%激活"
	}

	line += " | "

	if s.StopLossTriggered {
		line += fmt.Sprintf("止损触发 @%.2f(第%dm)", s.StopLossPrice, s.StopLossTime)
	} else {
		line += "止损:-5%未触发"
	}

	return line
}
