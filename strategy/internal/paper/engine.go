package paper

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"stock-strategy/internal/notify"
	"stock-strategy/internal/provider"
	"stock-strategy/internal/riskcontrol"
	"stock-strategy/internal/strategy/tail_20_2"
)

// Engine 模拟盘交易引擎
type Engine struct {
	cfg     PaperConfig
	prov    *provider.Provider
	state   *PaperState
	today   time.Time
	logFile *os.File
}

// NewEngine 创建模拟盘引擎
func NewEngine(cfg PaperConfig) *Engine {
	return &Engine{
		cfg:   cfg,
		today: time.Now().Truncate(24 * time.Hour),
	}
}

// initLog 按日打开日志文件
func (e *Engine) initLog() {
	path := fmt.Sprintf("paper_%s.log", e.today.Format("2006-01-02"))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[WARN] 无法创建日志文件 %s: %v\n", path, err)
		return
	}
	e.logFile = f
}

// closeLog 关闭日志文件
func (e *Engine) closeLog() {
	if e.logFile != nil {
		e.logFile.Close()
		e.logFile = nil
	}
}

// logf 同时输出到控制台和日志文件
func (e *Engine) logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(msg)
	if e.logFile != nil {
		e.logFile.WriteString(msg)
	}
}


// SetDate 指定日期（用于测试/补跑）
func (e *Engine) SetDate(t time.Time) {
	e.today = t.Truncate(24 * time.Hour)
}

// ============================================================
// Run — 完整流程 (先卖后买, 等价于 settle)
// ============================================================

func (e *Engine) Run() {
	e.RunSettle()
}

// ============================================================
// RunSell — 单次止盈止损检查（手动执行）
// ============================================================

// RunSell 执行一次止盈止损检查，触发条件的卖出，未触发的保留
func (e *Engine) RunSell() {
	if !e.initEnv("卖出") {
		return
	}
	defer e.closeLog()
	if len(e.state.Positions) == 0 {
		e.logf("当前无持仓。\n")
		return
	}
	e.initProvider()
	defer e.prov.Close()

	e.logf("--- 单次检查 ---\n")
	sold := e.sellOnePass()
	if sold {
		saveStateSafe(e.state)
	} else {
		e.logf("  无触发卖出\n")
	}
	e.logf("\n")

	remaining := len(e.state.Positions)
	if remaining > 0 {
		e.logf("剩余 %d 只持仓，可执行 monitor 持续监控。\n", remaining)
	} else {
		e.logf("全部卖出。\n")
	}
}

// ============================================================
// RunMonitor — 持续监控（每2分钟检查，触发即卖，直到清仓或尾盘）
// ============================================================

// RunMonitor 持续监控持仓，每2分钟检查止盈止损
//
// 流程:
//
//	触发止损 → 立即卖出
//	触发止盈 → 立即卖出
//	其他情况  → 保留继续监控
//	14:40后  → 退出监控，提示执行 settle
//	全部卖完 → 自动结束
func (e *Engine) RunMonitor() {
	if !e.initEnv("持续监控") {
		return
	}
	defer e.closeLog()

	e.initProvider()
	defer e.prov.Close()

	hasPositions := len(e.state.Positions) > 0
	if hasPositions {
		e.logf("持仓: ")
		for i, p := range e.state.Positions {
			if i > 0 {
				e.logf(", ")
			}
			e.logf("%s %s(%.2f)", p.Code, p.Name, p.BuyPrice)
		}
		e.logf("\n")
		e.logf("09:35~14:50 每2分钟检查止盈止损，尾盘自动清算。\n")
	} else {
		e.logf("当前无持仓，等待14:50自动选股买入。\n")
	}
	e.logf("\n")

	printedWait := false

	for {
		// 14:50 → 强制清仓剩余持仓 + 选股买入 + 记账
		if isTimeGE(14, 50) && e.shouldRunBuy() {
			e.doSettle()
			break
		}
		if !e.shouldRunBuy() {
			fmt.Println("今日已完成清算。")
			break
		}

		// 止盈止损检查
		if len(e.state.Positions) > 0 {
			sold := e.sellOnePass()
			if sold {
				saveStateSafe(e.state)
			}
		}

		if len(e.state.Positions) == 0 {
			if !printedWait {
				fmt.Printf("[%s] 已全部卖出，等待14:50选股买入...\n", time.Now().Format("15:04"))
				printedWait = true
			}
		} else {
			fmt.Printf("[%s] 等待2分钟... 剩余%d只\n",
				time.Now().Format("15:04"), len(e.state.Positions))
		}

		time.Sleep(2 * time.Minute)
	}

	fmt.Println("监控结束。")
}

// doSettle 执行尾盘清算（强制清仓 + MACD + 买入 + 记账）
// 由 RunMonitor 在 14:50 自动调用
func (e *Engine) doSettle() {
	e.logf("\n=== 14:50 尾盘清算 ===\n")

	// 强制卖出剩余持仓（白天没触发止盈止损的，尾盘清掉）
	e.sellFinal()
	saveStateSafe(e.state)

	macdOK := e.checkMACD()
	if macdOK {
		e.buyStocks()
	}

	e.recordDailyValue()
	saveStateSafe(e.state)
	e.logf("%s", e.formatSummary())
	e.sendSettlementReport("尾盘清算")
}

// isTimeGE 判断当前时间是否 >= HH:mm
func isTimeGE(h, m int) bool {
	now := time.Now()
	return now.Hour() > h || (now.Hour() == h && now.Minute() >= m)
}

// ============================================================
// RunBuy — 仅买入（14:50）
// ============================================================

func (e *Engine) RunBuy() {
	if !e.initEnv("买入") {
		return
	}
	defer e.closeLog()
	if !e.shouldRunBuy() {
		e.logf("今日已执行买入，跳过。\n")
		return
	}
	e.initProvider()
	defer e.prov.Close()

	macdOK := e.checkMACD()
	if macdOK {
		e.buyStocks()
	}

	e.recordDailyValue()
	saveStateSafe(e.state)
	e.logf("%s", e.formatSummary())
}

// ============================================================
// RunSettle — 尾盘清算（14:50执行）
//
// 1. 强制卖出所有剩余持仓（完整 CheckSell：止盈/止损/收盘卖出）
// 2. MACD检查 + 选股买入
// 3. 日终记账
// ============================================================

func (e *Engine) RunSettle() {
	if !e.initEnv("尾盘清算") {
		return
	}
	defer e.closeLog()
	if !e.shouldRunBuy() {
		e.logf("今日已完成清算。\n")
		return
	}

	e.initProvider()
	defer e.prov.Close()

	// 仅选股买入（卖出由 monitor 全天处理）
	macdOK := e.checkMACD()
	if macdOK {
		e.buyStocks()
	}

	e.recordDailyValue()
	saveStateSafe(e.state)
	e.logf("%s", e.formatSummary())
	e.sendSettlementReport("尾盘清算")
}

// ============================================================
// 环境初始化
// ============================================================

func (e *Engine) initEnv(phase string) bool {
	e.initLog()
	e.logf("=== 模拟盘交易 [%s] ===\n", phase)
	e.logf("日期: %s\n", e.today.Format("2006-01-02"))
	e.logf("配置: %s\n", e.cfg)

	if !isTradingDay(e.today) {
		e.logf("非交易日，跳过。\n")
		e.closeLog()
		return false
	}

	var err error
	e.state, err = loadState(e.cfg)
	if err != nil {
		log.Fatalf("加载状态失败: %v", err)
	}
	e.logf("\n")
	return true
}

func (e *Engine) initProvider() {
	e.prov = provider.NewProvider(e.cfg.TDXDir)
}

func (e *Engine) shouldRunBuy() bool {
	for _, dv := range e.state.DailyValues {
		if dv.Date.Equal(e.today) {
			return false
		}
	}
	return true
}

// ============================================================
// 卖出 — 监控模式（止盈/止损，不强制清仓）
// ============================================================

// sellOnePass 单次检查：止损→卖，止盈→卖，其他→保留
// 返回是否卖出了至少一只
func (e *Engine) sellOnePass() bool {
	if len(e.state.Positions) == 0 {
		return false
	}

	var remaining []StoredPosition
	soldAny := false

	for _, pos := range e.state.Positions {
		high, low, close, ok := getLiveDailyHLOC(e.prov, pos.Code)
		if !ok {
			remaining = append(remaining, pos)
			continue
		}

		decision := checkTrigger(pos.BuyPrice, high, low, close, e.cfg.StopLossPct)
		if decision != nil {
			e.executeSell(&pos, decision.SellPrice, decision.Reason)
			soldAny = true
		} else {
			remaining = append(remaining, pos)
		}
	}

	e.state.Positions = remaining
	return soldAny
}

// checkTrigger 检查止盈止损是否触发（监控用）
// 止损: 低点 <= 买入价×(1-止损%)

// 止盈: 最高 >= 买入价×1.02 且 收盘 <= 最高×0.985（回落确认）
// 未触发: 返回 nil（不卖）
//
// 与 tail_20_2.CheckSell 的区别: 未触发时返回 nil 而非"收盘卖出"
func checkTrigger(buyPrice, high, low, close, stopLossPct float64) *tail_20_2.SellDecision {
	slRatio := stopLossPct
	if slRatio <= 0 {
		slRatio = 0.05
	}

	// 止损
	if low <= buyPrice*(1-slRatio) {
		sellPrice := buyPrice * (1 - slRatio)
		if low > sellPrice {
			sellPrice = low
		}
		return &tail_20_2.SellDecision{
			ShouldSell: true,
			SellPrice:  sellPrice,
			Reason:     "止损",
		}
	}

	// 止盈: 涨超2% + 回落1.5%
	if high >= buyPrice*1.02 {
		if close <= high*0.985 {
			return &tail_20_2.SellDecision{
				ShouldSell: true,
				SellPrice:  close,
				Reason:     "止盈",
			}
		}
		// 涨超但未回落 → 继续观察
	}

	return nil
}

// ============================================================
// 卖出 — 尾盘清算（强制清仓）
// ============================================================

// sellFinal 强制卖出所有持仓（使用完整 CheckSell）
func (e *Engine) sellFinal() {
	if len(e.state.Positions) == 0 {
		e.logf("[清算] 无持仓\n")
		return
	}

	e.logf("--- 尾盘卖出 ---\n")

	var remaining []StoredPosition
	for _, pos := range e.state.Positions {
		high, low, close, ok := getLiveDailyHLOC(e.prov, pos.Code)
		if !ok {
			e.logf("  %-8s 无数据，保留\n", pos.Code)
			remaining = append(remaining, pos)
			continue
		}

		decision := tail_20_2.CheckSell(pos.BuyPrice, high, low, close, e.cfg.StopLossPct)
		if decision != nil && decision.ShouldSell {
			e.executeSell(&pos, decision.SellPrice, decision.Reason)
		} else {
			remaining = append(remaining, pos)
		}
	}

	e.state.Positions = remaining
	e.logf("\n")
}

// ============================================================
// 行情数据
// ============================================================

// getLiveDailyHLOC 从1分钟K线计算今日高开低收
func getLiveDailyHLOC(prov *provider.Provider, code string) (high, low, close float64, ok bool) {
	resp, err := prov.GetMinuteKlines(code, 1)
	if err != nil || resp == nil || len(resp.List) == 0 {
		return 0, 0, 0, false
	}

	high = resp.List[0].High
	low = resp.List[0].Low
	close = resp.List[len(resp.List)-1].Close

	for _, k := range resp.List {
		if k.High > high {
			high = k.High
		}
		if k.Low < low {
			low = k.Low
		}
	}
	return high, low, close, true
}

// currentPrice 获取股票当前价格
func currentPrice(prov *provider.Provider, code string, fallback float64) float64 {
	resp, err := prov.GetMinuteKlines(code, 1)
	if err == nil && resp != nil && len(resp.List) > 0 {
		return resp.List[len(resp.List)-1].Close
	}
	klines, err := prov.GetKlines(code, 1)
	if err == nil && len(klines) > 0 {
		return klines[len(klines)-1].Close
	}
	return fallback
}

// ============================================================
// 执行交易
// ============================================================

// executeSell 执行卖出
// 注意: 调用方负责从 Positions 中移除
func (e *Engine) executeSell(pos *StoredPosition, price float64, reason string) {
	if price <= 0 {
		return
	}

	amount := price * float64(pos.Shares)
	fee := amount * e.cfg.FeeRate
	netAmount := amount - fee

	e.state.Cash += netAmount
	e.state.TotalFee += fee

	e.state.Trades = append(e.state.Trades, StoredTrade{
		Date: e.today, Code: pos.Code, Name: pos.Name,
		Dir: "卖出", Price: price, Shares: pos.Shares,
		Amount: amount, Fee: fee, Reason: reason,
	})

	pnlPct := (price - pos.BuyPrice) / pos.BuyPrice * 100
	buyCost := pos.BuyPrice * float64(pos.Shares)

	sign := "+"
	if pnlPct < 0 {
		sign = ""
	}

	if reason == "止盈" {
		e.state.WinCount++
		e.logf("  %-8s %-10s %.2f→%.2f (%s%.2f%%) 止盈 ✓\n",
			pos.Code, pos.Name, pos.BuyPrice, price, sign, pnlPct)
	} else if reason == "止损" {
		e.state.LoseCount++
		e.logf("  %-8s %-10s %.2f→%.2f (%s%.2f%%) 止损 ✗\n",
			pos.Code, pos.Name, pos.BuyPrice, price, sign, pnlPct)
	} else {
		if netAmount >= buyCost {
			e.state.WinCount++
		} else {
			e.state.LoseCount++
		}
		e.logf("  %-8s %-10s %.2f→%.2f (%s%.2f%%) %s\n",
			pos.Code, pos.Name, pos.BuyPrice, price, sign, pnlPct, reason)
	}
}

// executeBuy 执行买入
func (e *Engine) executeBuy(code, name string, price float64) bool {
	if e.state.Cash <= 0 || price <= 0 {
		return false
	}

	total := e.state.Cash + e.positionCost()
	targetAmount := total * e.cfg.PositionPct
	if targetAmount > e.state.Cash {
		targetAmount = e.state.Cash
	}

	fee := targetAmount * e.cfg.FeeRate
	available := targetAmount - fee
	shares := (int(available / price) / 100) * 100
	if shares <= 0 {
		return false
	}

	actualAmount := float64(shares) * price
	actualFee := actualAmount * e.cfg.FeeRate
	totalCost := actualAmount + actualFee

	if totalCost > e.state.Cash {
		return false
	}

	e.state.Cash -= totalCost
	e.state.TotalFee += actualFee

	e.state.Positions = append(e.state.Positions, StoredPosition{
		Code: code, Name: name, BuyDate: e.today,
		BuyPrice: price, Shares: shares,
	})

	e.state.Trades = append(e.state.Trades, StoredTrade{
		Date: e.today, Code: code, Name: name,
		Dir: "买入", Price: price, Shares: shares,
		Amount: actualAmount, Fee: actualFee, Reason: "策略买入",
	})

	e.logf("  %-8s %-10s %.2f × %-5d股 = %s\n",
		code, name, price, shares, formatAmount(actualAmount))
	return true
}

// ============================================================
// MACD 检查
// ============================================================

func (e *Engine) checkMACD() bool {
	e.logf("--- 大盘MACD ---\n")
	macdOK, err := tail_20_2.CheckMACDLive(e.prov)
	if err != nil {
		e.logf("  MACD检查失败: %v (忽略检查)\n", err)
		return true
	}
	if macdOK {
		e.logf("  ✓ 红柱，允许入场\n")
	} else {
		e.logf("  ✗ 绿柱，禁止入场\n")
	}
	e.logf("\n")
	return macdOK
}

// ============================================================
// 买入
// ============================================================

func (e *Engine) buyStocks() {
	slots := e.cfg.MaxPositions - len(e.state.Positions)
	if slots <= 0 {
		e.logf("[买入] 持仓已满\n")
		return
	}

	e.logf("--- 买入 ---\n")

	allCodes, err := tail_20_2.GetAllStockCodes(e.prov)
	if err != nil {
		e.logf("  获取股票列表失败: %v\n", err)
		return
	}

	// 检查本地日K线是否包含最新交易日数据
	if e.prov.NeedRefresh("000001") {
		e.logf("  本地数据已过期，正在从服务器刷新日K线(%d只)...\n", len(allCodes))
		e.prov.SetForceAPI(true)
		e.prov.RefreshDailyData(allCodes)
		e.logf("  刷新完成\n\n")
	} else {
		e.logf("  本地数据已是最新\n")
	}

	e.logf("  正在选股(%d只)...\n", len(allCodes))
	candidates := tail_20_2.SelectCandidatesLive(allCodes, e.prov, e.today, 100)

	// 确保退出选股后关闭强制模式，不影响后续操作
	defer e.prov.SetForceAPI(false)
	if len(candidates) == 0 {
		e.logf("  无符合条件的股票\n\n")
		return
	}
	e.logf("  初选通过: %d只\n", len(candidates))
for _, c := range candidates {
		e.logf("    %-8s %-10s 收盘%.2f 评分%.0f\n", c.Code, c.Result.Name, c.Result.ClosePrice, c.Result.Score)
	}

	inputs := tail_20_2.BuildRiskInputs(candidates, e.prov, e.today)
	summary := riskcontrol.ProcessAll(inputs)

	e.logf("  风控排序:\n")
	for i, r := range summary.Results {
		mark := "  "
		if i < slots {
			mark = "→"
		}
		e.logf("    %s %-8s %-10s 风险%s 脉冲%.1f\n", mark, r.Code, r.Name, r.Color.Tag(), r.PulseScore)
	}

	buyCount := 0
	for _, r := range summary.Results {
		if buyCount >= slots {
			break
		}
		if e.hasPosition(r.Code) {
			continue
		}

		buyPrice := currentPrice(e.prov, r.Code, r.ClosePrice)
		if e.executeBuy(r.Code, r.Name, buyPrice) {
			buyCount++
		}
	}

	if buyCount == 0 {
		e.logf("  未能买入\n")
	} else {
		e.logf("  成功买入 %d 只\n", buyCount)
	}
	e.logf("\n")
}

// ============================================================
// 组合记账
// ============================================================

func (e *Engine) recordDailyValue() {
	posVal := e.positionCost()
	total := e.state.Cash + posVal

	e.state.DailyValues = append(e.state.DailyValues, DailyRecord{
		Date:          e.today,
		Cash:          e.state.Cash,
		PositionValue: posVal,
		TotalValue:    total,
		PositionCount: len(e.state.Positions),
	})

	if total > e.state.PeakCapital {
		e.state.PeakCapital = total
	}
	dd := 0.0
	if e.state.PeakCapital > 0 {
		dd = (e.state.PeakCapital - total) / e.state.PeakCapital * 100
	}
	if dd > e.state.MaxDrawdown {
		e.state.MaxDrawdown = dd
	}
}

// ============================================================
// 辅助方法
// ============================================================

// positionCost 持仓总市值(按买入价)
func (e *Engine) positionCost() float64 {
	var total float64
	for _, p := range e.state.Positions {
		total += p.BuyPrice * float64(p.Shares)
	}
	return total
}

// currentTotal 当前总资产
func (e *Engine) currentTotal() float64 {
	return e.state.Cash + e.positionCost()
}

func (e *Engine) hasPosition(code string) bool {
	for _, p := range e.state.Positions {
		if p.Code == code {
			return true
		}
	}
	return false
}

// ============================================================
// 邮件通知
// ============================================================

// sendSettlementReport 发送清算结果邮件
func (e *Engine) sendSettlementReport(phase string) {
	cfg := notify.EnvMailConfig()
	if cfg == nil {
		return // 未配置邮件，跳过
	}

	body := e.formatSettlementBody(phase)
	subject := fmt.Sprintf("模拟盘%s报告 - %s", phase, e.today.Format("2006-01-02"))

	if err := notify.SendMail(cfg, subject, body); err != nil {
		fmt.Printf("  发送邮件失败: %v\n", err)
	} else {
		fmt.Println("  清算报告已发送至邮箱")
	}
}

// SendReport 手动发送清算报告邮件（从已保存的状态文件读取）
func (e *Engine) SendReport() {
	var err error
	e.state, err = loadState(e.cfg)
	if err != nil {
		log.Fatalf("加载状态失败: %v", err)
	}
	// 取最后一条有记录的日期作为报告日期
	if len(e.state.DailyValues) > 0 {
		e.today = e.state.DailyValues[len(e.state.DailyValues)-1].Date
	}
	e.sendSettlementReport("清算")
}

// formatSettlementBody 格式化清算报告正文
func (e *Engine) formatSettlementBody(phase string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== 模拟盘%s报告 ===\n", phase)
	fmt.Fprintf(&b, "日期: %s\n\n", e.today.Format("2006-01-02"))

	// 今日成交
	var todaySells, todayBuys []StoredTrade
	for _, t := range e.state.Trades {
		if !t.Date.Equal(e.today) {
			continue
		}
		if t.Dir == "买入" {
			todayBuys = append(todayBuys, t)
		} else {
			todaySells = append(todaySells, t)
		}
	}

	if len(todaySells) > 0 {
		b.WriteString("【卖出】\n")
		for _, t := range todaySells {
			icon := "→"
			switch t.Reason {
			case "止盈":
				icon = "✓"
			case "止损":
				icon = "✗"
			}
			fmt.Fprintf(&b, "  %s %s  %.2f×%d股  %s%s\n",
				t.Code, t.Name, t.Price, t.Shares, t.Reason, icon)
		}
	}
	if len(todayBuys) > 0 {
		b.WriteString("【买入】\n")
		for _, t := range todayBuys {
			fmt.Fprintf(&b, "  %s %s  %.2f×%d股\n",
				t.Code, t.Name, t.Price, t.Shares)
		}
	}

	// 收益情况
	dvs := e.state.DailyValues
	if len(dvs) > 0 {
		last := dvs[len(dvs)-1]
		initCap := e.cfg.InitCapital
		totalRet := (last.TotalValue - initCap) / initCap * 100

		fmt.Fprintf(&b, "\n当日总资产: %s\n", formatAmount(last.TotalValue))
		fmt.Fprintf(&b, "累计收益: %s (%+.2f%%)\n",
			formatAmount(last.TotalValue-initCap), totalRet)

		if len(dvs) >= 2 {
			prev := dvs[len(dvs)-2]
			dayPnl := last.TotalValue - prev.TotalValue
			dayPnlPct := (last.TotalValue - prev.TotalValue) / prev.TotalValue * 100
			fmt.Fprintf(&b, "当日收益: %s (%+.2f%%)\n",
				formatAmount(dayPnl), dayPnlPct)
		}

		fmt.Fprintf(&b, "最大回撤: %.2f%%\n", e.state.MaxDrawdown)
	}

	// 当前持仓
	n := len(e.state.Positions)
	if n > 0 {
		fmt.Fprintf(&b, "\n当前持仓: %d只\n", n)
		for _, p := range e.state.Positions {
			cost := p.BuyPrice * float64(p.Shares)
			fmt.Fprintf(&b, "  %s %s  买入价%.2f  %d股  成本%s\n",
				p.Code, p.Name, p.BuyPrice, p.Shares, formatAmount(cost))
		}
	} else {
		b.WriteString("\n当前持仓: 空仓\n")
	}

	fmt.Fprintf(&b, "\n可用现金: %s\n", formatAmount(e.state.Cash))

	return b.String()
}
