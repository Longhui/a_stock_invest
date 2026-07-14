package backtest

import (
	"fmt"
	"math"
	"time"

	"stock-strategy/internal/strategy/tail_20_2"
)

// executeBuy 买入: 按总资产的 PositionPct 比例买入
func (e *Engine) executeBuy(code, name string, price float64, date time.Time) {
	if e.cash <= 0 || price <= 0 {
		return
	}

	// 计算买入金额 = 当前总资产 * 仓位比例
	total := e.cash + e.positionValue()
	targetAmount := total * e.cfg.PositionPct
	if targetAmount > e.cash {
		targetAmount = e.cash
	}

	fee := targetAmount * e.cfg.FeeRate
	available := targetAmount - fee
	shares := int(math.Floor(available / price))
	if shares <= 0 {
		return
	}

	actualAmount := float64(shares) * price
	actualFee := actualAmount * e.cfg.FeeRate
	totalCost := actualAmount + actualFee

	if totalCost > e.cash {
		return
	}

	e.cash -= totalCost
	e.totalFee += actualFee

	e.positions = append(e.positions, Position{
		Code:     code,
		Name:     name,
		BuyDate:  date,
		BuyPrice: price,
		Shares:   shares,
	})

	e.trades = append(e.trades, Trade{
		Date:   date,
		Code:   code,
		Name:   name,
		Dir:    "买入",
		Price:  price,
		Shares: shares,
		Amount: actualAmount,
		Fee:    actualFee,
		Reason: "策略买入",
	})
}

// executeSell 卖出指定持仓
func (e *Engine) executeSell(pos *Position, price float64, date time.Time, reason string) {
	if price <= 0 {
		return
	}

	amount := price * float64(pos.Shares)
	fee := amount * e.cfg.FeeRate
	netAmount := amount - fee

	e.cash += netAmount
	e.totalFee += fee

	e.trades = append(e.trades, Trade{
		Date:   date,
		Code:   pos.Code,
		Name:   pos.Name,
		Dir:    "卖出",
		Price:  price,
		Shares: pos.Shares,
		Amount: amount,
		Fee:    fee,
		Reason: reason,
	})

	// 统计盈亏
	buyCost := pos.BuyPrice * float64(pos.Shares)
	if reason == "止盈" {
		e.winCount++
	} else if reason == "止损" || reason == "收盘卖出" {
		if netAmount < buyCost {
			e.loseCount++
		} else {
			e.winCount++
		}
	}

	// 移除持仓
	e.removePosition(pos.Code)
}

// removePosition 从持仓列表中移除
func (e *Engine) removePosition(code string) {
	for i, p := range e.positions {
		if p.Code == code {
			e.positions = append(e.positions[:i], e.positions[i+1:]...)
			return
		}
	}
}

// positionValue 计算当前持仓总市值(使用买入价)
func (e *Engine) positionValue() float64 {
	var total float64
	for _, p := range e.positions {
		total += p.BuyPrice * float64(p.Shares)
	}
	return total
}

// positionValueAt 计算指定日期的持仓市值(使用当日收盘价)
func (e *Engine) positionValueAt(date time.Time) float64 {
	var total float64
	for _, p := range e.positions {
		price := p.BuyPrice // 默认用买入价
		if cache, ok := e.stockCache[p.Code]; ok && cache != nil {
			for _, k := range cache.Klines {
				if tail_20_2.IsSameDay(k.Date, date) {
					price = k.Close
					break
				}
			}
		}
		total += price * float64(p.Shares)
	}
	return total
}

// positionByCode 查找持仓
func (e *Engine) positionByCode(code string) *Position {
	for i := range e.positions {
		if e.positions[i].Code == code {
			return &e.positions[i]
		}
	}
	return nil
}

// sellAllPositions 清仓所有持仓(回测结束用)
func (e *Engine) sellAllPositions(date time.Time, reason string) {
	for i := range e.positions {
		p := &e.positions[i]
		price := p.BuyPrice
		e.executeSell(p, price, date, reason)
	}
}

// recordDailyValue 记录每日净值(以当日收盘价计算持仓市值)
func (e *Engine) recordDailyValue(date time.Time) {
	posVal := e.positionValueAt(date)
	total := e.cash + posVal

	e.dailyValues = append(e.dailyValues, DailyValue{
		Date:          date,
		Cash:          e.cash,
		PositionValue: posVal,
		TotalValue:    total,
		PositionCount: len(e.positions),
	})

	// 更新最大回撤
	if total > e.peakCapital {
		e.peakCapital = total
	}
	drawdown := 0.0
	if e.peakCapital > 0 {
		drawdown = (e.peakCapital - total) / e.peakCapital * 100
	}
	if drawdown > e.maxDrawdown {
		e.maxDrawdown = drawdown
	}
}

// hasPosition 是否已持仓某只
func (e *Engine) hasPosition(code string) bool {
	for _, p := range e.positions {
		if p.Code == code {
			return true
		}
	}
	return false
}

// logTrade 打印成交记录(调试用)
func (e *Engine) logTrade(t *Trade) {
	if e.debug {
		fmt.Printf("  [交易] %s %s %s %.2f×%d 手续费%.2f %s\n",
			t.Date.Format("01-02"), t.Dir, t.Code, t.Price, t.Shares, t.Fee, t.Reason)
	}
}
