package backtest

import (
	"fmt"
	"log"
	"time"

	"stock-strategy/internal/provider"
	"stock-strategy/internal/riskcontrol"
	"stock-strategy/internal/strategy/tail_20_2"
)

// Engine 回测引擎
type Engine struct {
	cfg    BacktestConfig
	prov   *provider.Provider
	debug  bool

	// 资金状态
	cash        float64
	positions   []Position
	trades      []Trade
	dailyValues []DailyValue
	totalFee    float64
	winCount    int
	loseCount   int
	peakCapital float64
	maxDrawdown float64

	// 缓存（由策略层填充）
	stockCache     map[string]*tail_20_2.StockDataCache
	allCodes       []string
	idx60Klines    []provider.Kline
}

// NewEngine 创建回测引擎
func NewEngine(cfg BacktestConfig) *Engine {
	return &Engine{
		cfg:         cfg,
		cash:        cfg.InitCapital,
		peakCapital: cfg.InitCapital,
		stockCache:  make(map[string]*tail_20_2.StockDataCache),
	}
}

// SetDebug 开启调试输出
func (e *Engine) SetDebug(on bool) {
	e.debug = on
}

// Run 执行回测
func (e *Engine) Run() *BacktestSummary {
	log.SetFlags(log.Ltime | log.Lshortfile)

	fmt.Println("=== 隔夜交易回测系统 ===")
	fmt.Println(e.cfg.String())
	fmt.Println()

	// 1. 初始化数据提供者
	fmt.Println("正在初始化数据...")
	e.prov = provider.NewProvider(e.cfg.TDXDir)
	defer e.prov.Close()

	// 2. 获取全市场股票并板块过滤（策略层）
	var err error
	e.allCodes, err = tail_20_2.GetAllStockCodes(e.prov)
	if err != nil {
		log.Fatalf("获取股票列表失败: %v", err)
	}

	// 3. 预加载日K线（策略层）
	fmt.Println("预加载日K线数据(首次较慢,后续为本地读取)...")
	e.stockCache = tail_20_2.LoadAllStockData(e.prov, e.allCodes)

	// 4. 加载指数60分钟K线（策略层）
	fmt.Println("加载上证指数60分钟K线...")
	e.idx60Klines, err = tail_20_2.LoadIndex60MinKlines(e.prov)
	if err != nil {
		log.Fatalf("获取指数60分钟K线失败: %v", err)
	}

	// 5. 逐日回测
	totalDays := countTradingDays(e.cfg.StartDate, e.cfg.EndDate)
	fmt.Printf("\n开始逐日回测(%d个交易日)...\n\n", totalDays)

	dayCount := 0
	for d := e.cfg.StartDate; !d.After(e.cfg.EndDate); d = d.AddDate(0, 0, 1) {
		if !isTradingDay(d) {
			continue
		}
		dayCount++

		if e.debug || dayCount%5 == 0 || dayCount == 1 {
			fmt.Printf("[%3d/%d] %s 持仓%d 资金%.0f\n",
				dayCount, totalDays, d.Format("01-02"), len(e.positions), e.cash)
		}

		// A. 卖出昨日持仓（决策：策略层，执行：引擎）
		e.sellPositions(d)

		// B. 9:55 大盘MACD检查（策略层）
		macdOK := tail_20_2.CheckMACD(e.idx60Klines, d)

		// C. 14:50 选股买入（决策：策略层，执行：引擎）
		if macdOK && len(e.positions) < e.cfg.MaxPositions {
			e.buyStocks(d)
		}

		// D. 日终记账
		e.recordDailyValue(d)
	}

	return e.buildSummary()
}

// ============================================================
// 卖出（策略决策 → 引擎执行）
// ============================================================

func (e *Engine) sellPositions(today time.Time) {
	if len(e.positions) == 0 {
		return
	}

	positions := make([]Position, len(e.positions))
	copy(positions, e.positions)

	for _, pos := range positions {
		cache := e.stockCache[pos.Code]
		if cache == nil || len(cache.Klines) == 0 {
			continue
		}

		// 从缓存中找到今日日K线
		var todayBar *tail_20_2.Kline
		for i, k := range cache.Klines {
			if tail_20_2.IsSameDay(k.Date, today) {
				todayBar = &cache.Klines[i]
				break
			}
		}
		if todayBar == nil {
			continue // 该日无数据
		}

		// 策略层决策：是否卖出、什么价格、什么原因
		decision := tail_20_2.CheckSell(
			pos.BuyPrice,
			todayBar.High,
			todayBar.Low,
			todayBar.Close,
			e.cfg.StopLossPct,
		)
		if decision != nil && decision.ShouldSell {
			e.executeSell(&pos, decision.SellPrice, today, decision.Reason)
		}
	}
}

// ============================================================
// 买入（策略决策 → 引擎执行）
// ============================================================

func (e *Engine) buyStocks(today time.Time) {
	slots := e.cfg.MaxPositions - len(e.positions)
	if slots <= 0 {
		return
	}

	// 策略层：选股筛选
	candidates := tail_20_2.SelectCandidates(e.stockCache, e.allCodes, today)
	if len(candidates) == 0 {
		return
	}

	// 策略层：构建风控数据
	inputs := tail_20_2.BuildRiskInputs(candidates, e.prov, today)

	// 风控排序
	summary := riskcontrol.ProcessAll(inputs)

	// 引擎层：执行买入
	buyCount := 0
	for _, r := range summary.Results {
		if buyCount >= slots {
			break
		}
		if e.hasPosition(r.Code) {
			continue
		}
		e.executeBuy(r.Code, r.Name, r.ClosePrice, today)
		if len(e.trades) > 0 {
			last := e.trades[len(e.trades)-1]
			if last.Dir == "买入" && last.Code == r.Code {
				buyCount++
			}
		}
	}
}

// ============================================================
// 汇总
// ============================================================

func (e *Engine) buildSummary() *BacktestSummary {
	initCapital := e.cfg.InitCapital
	finalValue := e.cash + e.positionValue()
	totalReturn := (finalValue - initCapital) / initCapital * 100

	buyCount := 0
	sellCount := 0
	for _, t := range e.trades {
		switch t.Dir {
		case "买入":
			buyCount++
		case "卖出":
			sellCount++
		}
	}

	winRate := 0.0
	if e.winCount+e.loseCount > 0 {
		winRate = float64(e.winCount) / float64(e.winCount+e.loseCount) * 100
	}

	return &BacktestSummary{
		Config:      e.cfg,
		Trades:      e.trades,
		DailyValues: e.dailyValues,
		TotalReturn: totalReturn,
		MaxDrawdown: e.maxDrawdown,
		TotalFee:    e.totalFee,
		TradeCount:  len(e.trades),
		BuyCount:    buyCount,
		SellCount:   sellCount,
		WinCount:    e.winCount,
		LoseCount:   e.loseCount,
		WinRate:     winRate,
	}
}
