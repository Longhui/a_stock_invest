package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"stock-strategy/internal/reader"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

// Kline 统一的K线数据结构
type Kline struct {
	Code      string
	Date      time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64 // 成交量(股)
	Amount    float64 // 成交额(元)
}

// StockData 一只股票的完整数据
type StockData struct {
	Code            string
	Name            string
	Klines          []Kline
	CirculateShares float64 // 流通股本(股)
}

// Provider 数据提供者(本地优先+tdx-api回退)
type Provider struct {
	TDXDir    string    // 通达信安装目录
	CacheDir  string    // 缓存目录
	client    *tdx.Client

	// 板块数据缓存
	sectorMap     reader.SectorMap
	sectorOnce    sync.Once
	sectorMapErr  error
}

func NewProvider(tdxDir string) *Provider {
	return &Provider{
		TDXDir:   tdxDir,
		CacheDir: tdxDir + "/T0002/cache",
	}
}

// GetKlines 获取K线数据(本地优先→缓存→tdx-api回退)
func (p *Provider) GetKlines(code string, minBars int) ([]Kline, error) {
	// 1. 尝试本地 .day 文件(通达信原始数据)
	path, exists, err := reader.DayFileExists(p.TDXDir, code)
	if err == nil && exists {
		records, err := reader.ReadDayFile(path)
		if err == nil && len(records) >= minBars {
			return convertToKlines(records, code), nil
		}
	}

	// 2. 检查本地缓存
	cachePath := p.cacheFilePath(code)
	if cachePath != "" {
		cacheRecords, err := reader.ReadDayFile(cachePath)
		if err == nil && len(cacheRecords) >= minBars {
			return convertToKlines(cacheRecords, code), nil
		}
	}

	// 3. 本地/缓存都不够 → tdx-api回退并缓存
	return p.getKlinesFromAPI(code, minBars)
}

func (p *Provider) cacheFilePath(code string) string {
	if p.CacheDir == "" {
		return ""
	}
	dir := p.CacheDir + "/cache_klines"
	return dir + "/" + code + ".day"
}

func (p *Provider) getKlinesFromAPI(code string, minBars int) ([]Kline, error) {
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}

	// 指数用 GetIndexDayAll，股票用 GetKlineDayAll
	var resp *protocol.KlineResp
	if isIndex(code) {
		resp, err = client.GetIndexDayAll(code)
	} else {
		resp, err = client.GetKlineDayAll(code)
	}
	if err != nil {
		return nil, fmt.Errorf("tdx-api获取K线失败: %w", err)
	}
	if resp == nil || len(resp.List) < minBars {
		return nil, fmt.Errorf("tdx-api返回数据不足%d条(实际%d)", minBars, len(resp.List))
	}

	klines := make([]Kline, len(resp.List))
	records := make([]reader.DayRecord, len(resp.List))
	for i, k := range resp.List {
		klines[i] = Kline{
			Code:   code,
			Date:   k.Time,
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: float64(k.Volume),
			Amount: k.Amount.Float64(),
		}
		records[i] = reader.DayRecord{
			Date:   k.Time,
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: int64(k.Volume),
			Amount: k.Amount.Float64(),
		}
	}

	// 保存到本地缓存
	if cachePath := p.cacheFilePath(code); cachePath != "" {
		cacheDir := p.CacheDir + "/cache_klines"
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			if err := reader.SaveDayFile(cachePath, records); err != nil {
				fmt.Printf("  缓存%s失败: %v\n", code, err)
			}
		}
	}

	return klines, nil
}

// GetMinuteKlines 获取1分钟K线(最近N根)
// 可用来判断早盘脉冲、尾盘拉升等分时行为
func (p *Provider) GetMinuteKlines(code string, minBars int) (*KlineResp, error) {
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}

	// 优先从本地缓存加载
	cachePath := p.cacheMinuteFilePath(code)
	if cachePath != "" {
		records, err := reader.ReadDayFile(cachePath)
		if err == nil && len(records) >= minBars {
			return &KlineResp{List: convertToKlines(records, code)}, nil
		}
	}

	// TDX-API获取1分钟K线
	resp, err := client.GetKlineMinuteAll(code)
	if err != nil {
		return nil, fmt.Errorf("获取1分钟K线失败: %w", err)
	}
	if resp == nil || len(resp.List) < minBars {
		return nil, fmt.Errorf("1分钟K线数据不足%d条(实际%d)", minBars, len(resp.List))
	}

	klines := make([]Kline, len(resp.List))
	records := make([]reader.DayRecord, len(resp.List))
	for i, k := range resp.List {
		klines[i] = Kline{
			Code:   code,
			Date:   k.Time,
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: float64(k.Volume),
			Amount: k.Amount.Float64(),
		}
		records[i] = reader.DayRecord{
			Date:   k.Time,
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: int64(k.Volume),
			Amount: k.Amount.Float64(),
		}
	}

	// 缓存到本地
	if cachePath := p.cacheMinuteFilePath(code); cachePath != "" {
		cacheDir := p.CacheDir + "/cache_minkline"
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			if err := reader.SaveDayFile(cachePath, records); err != nil {
				fmt.Printf("  缓存1分钟K线%s失败: %v\n", code, err)
			}
		}
	}

	return &KlineResp{List: klines}, nil
}

// KlineResp 1分钟K线响应
type KlineResp struct {
	List []Kline
}

func (p *Provider) cacheMinuteFilePath(code string) string {
	if p.CacheDir == "" {
		return ""
	}
	return p.CacheDir + "/cache_minkline/" + code + ".day"
}

// GetCirculateShares 获取流通股本
func (p *Provider) GetCirculateShares(code string) (float64, error) {
	info, err := reader.FetchCirculateShares(code)
	if err != nil {
		return 0, err
	}
	return info.CirculateShares, nil
}

// GetStockData 获取一票股票的完整数据（含K线和财务）
func (p *Provider) GetStockData(code string, minBars int) (*StockData, error) {
	klines, err := p.GetKlines(code, minBars)
	if err != nil {
		return nil, err
	}

	info, err := reader.FetchCirculateShares(code)
	if err != nil {
		return &StockData{
			Code:   code,
			Klines: klines,
		}, nil
	}

	return &StockData{
		Code:            code,
		Name:            info.Name,
		Klines:          klines,
		CirculateShares: info.CirculateShares,
	}, nil
}

// GetIndex60MinKlines 获取上证指数60分钟K线原始数据
// 返回完整60分钟K线列表(用于回测引擎自行计算MACD)
func (p *Provider) GetIndex60MinKlines() ([]Kline, error) {
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetKline60MinuteAll("000001")
	if err != nil {
		return nil, fmt.Errorf("获取上证指数60分钟K线失败: %w", err)
	}
	if resp == nil || len(resp.List) == 0 {
		return nil, fmt.Errorf("上证指数60分钟K线数据为空")
	}
	klines := make([]Kline, len(resp.List))
	for i, k := range resp.List {
		klines[i] = Kline{
			Date:   k.Time,
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: float64(k.Volume),
			Amount: k.Amount.Float64(),
		}
	}
	return klines, nil
}

// CheckMarketMACD 检查大盘60分钟MACD是否翻红
// 使用上证指数(sh000001)60分钟K线，MACD柱>0返回true(可入场)
func (p *Provider) CheckMarketMACD() (bool, error) {
	client, err := p.getClient()
	if err != nil {
		return false, err
	}

	// 获取上证指数60分钟K线(使用stock方式查询，index方式可能超时)
	resp, err := client.GetKline60MinuteAll("000001")
	if err != nil {
		return false, fmt.Errorf("获取上证指数60分钟K线失败: %w", err)
	}
	if resp == nil || len(resp.List) < 30 {
		return false, fmt.Errorf("上证指数60分钟K线数据不足(实际%d条)", len(resp.List))
	}

	// 提取收盘价
	list := resp.List
	n := len(list)
	closeP := make([]float64, n)
	for i, k := range list {
		closeP[i] = k.Close.Float64()
	}

	// 计算MACD: DIF = EMA(close,12) - EMA(close,26)
	ema12 := calcEMA(closeP, 12)
	ema26 := calcEMA(closeP, 26)
	dif := make([]float64, n)
	for i := 0; i < n; i++ {
		dif[i] = ema12[i] - ema26[i]
	}
	dea := calcEMA(dif, 9)
	macdBar := (dif[n-1] - dea[n-1]) * 2

	fmt.Printf("  上证指数60分钟MACD: DIF=%.4f, DEA=%.4f, BAR=%.4f\n",
		dif[n-1], dea[n-1], macdBar)

	return macdBar > 0, nil
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

func (p *Provider) getClient() (*tdx.Client, error) {
	if p.client != nil {
		return p.client, nil
	}
	c, err := tdx.DialDefault(tdx.WithDebug(false))
	if err != nil {
		return nil, fmt.Errorf("连接tdx服务器失败: %w", err)
	}
	p.client = c
	return c, nil
}

// GetAllStocks 获取全市场股票代码列表(本地优先)
func (p *Provider) GetAllStocks() ([]string, error) {
	// 优先从本地 .day 文件扫描
	if stocks := p.scanLocalStocks(); len(stocks) > 0 {
		return stocks, nil
	}
	// 回退: tdx-api网络获取
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetStockAll()
}

// scanLocalStocks 扫描本地 vipdoc 目录获取股票代码
func (p *Provider) scanLocalStocks() []string {
	var stocks []string
	for _, market := range []string{"sh", "sz", "bj"} {
		dir := p.TDXDir + "/vipdoc/" + market + "/lday"
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if len(name) == 12 && name[len(name)-4:] == ".day" {
				code := name[2:8] // sh600036.day → 600036
				stocks = append(stocks, code)
			}
		}
	}
	return stocks
}

// Close 关闭连接
func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

// GetBlockStocks 获取板块成分股(通过TDX网络协议)
// blockCode: 6位板块代码,如 881001(煤炭), 880506(5G概念)
func (p *Provider) GetBlockStocks(blockCode string) ([]string, error) {
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.GetBlockStocks(blockCode)
	if err != nil {
		return nil, fmt.Errorf("获取板块%s成分股失败: %w", blockCode, err)
	}
	return resp.List, nil
}

// GetSectorMap 获取股票→行业板块映射
// 首次调用时通过BlockMapXML.dat+TDX协议构建,结果缓存到内存和本地文件
// 只包含行业板块(881xxx)和概念板块(880xxx)
func (p *Provider) GetSectorMap() (reader.SectorMap, error) {
	p.sectorOnce.Do(func() {
		// 1. 尝试从本地缓存加载
		cachePath := p.CacheDir + "/sectormap.json"
		if data, err := os.ReadFile(cachePath); err == nil {
			var m reader.SectorMap
			if json.Unmarshal(data, &m) == nil && len(m) > 0 {
				p.sectorMap = m
				return
			}
		}

		// 2. 从BlockMapXML.dat加载板块列表
		blocks, err := reader.LoadBlockMap(p.TDXDir)
		if err != nil {
			p.sectorMapErr = fmt.Errorf("加载板块列表失败: %w", err)
			return
		}

		// 3. 并发查询各板块成分股,构建股票→板块映射
		// 使用8个worker并发查询,避免串行耗时
		type blockResult struct {
			code   string
			stocks []string
		}
		resultCh := make(chan blockResult, len(blocks))
		jobCh := make(chan string, len(blocks))
		var wg sync.WaitGroup
		var mu sync.Mutex // 保护client串行访问

		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for code := range jobCh {
					mu.Lock()
					stocks, err := p.GetBlockStocks(code)
					mu.Unlock()
					if err == nil && len(stocks) > 0 {
						resultCh <- blockResult{code, stocks}
					}
				}
			}()
		}
		for _, b := range blocks {
			prefix := b.Code
			if strings.HasPrefix(prefix, "881") || strings.HasPrefix(prefix, "880") {
				jobCh <- b.Code
			}
		}
		close(jobCh)
		wg.Wait()
		close(resultCh)

		// 4. 构建反向映射: 股票代码 → 板块名称
		sectorMap := make(reader.SectorMap)
		for r := range resultCh {
			for _, stock := range r.stocks {
				// 从板块成分股列表中找到板块名称
				for _, b := range blocks {
					if b.Code == r.code {
						sectorMap[stock] = append(sectorMap[stock], b.Name)
						break
					}
				}
			}
		}

		p.sectorMap = sectorMap

		// 5. 缓存到本地
		if len(p.sectorMap) > 0 {
			if data, err := json.Marshal(p.sectorMap); err == nil {
				if cacheDir := p.CacheDir; cacheDir != "" {
					os.MkdirAll(cacheDir, 0755)
					os.WriteFile(cachePath, data, 0644)
				}
			}
		}
	})

	return p.sectorMap, p.sectorMapErr
}

// GetStockSector 查询单只股票的行业板块名称
func (p *Provider) GetStockSector(code string) string {
	m, err := p.GetSectorMap()
	if err != nil || m == nil {
		return ""
	}
	return m.SectorName(code)
}

// -----------------------------------------------
// 实时行情 & 板块资金流向 & 涨停梯队
// -----------------------------------------------

// QuoteSnapshot 实时快照
type QuoteSnapshot struct {
	Code     string
	Price    float64
	PreClose float64
	Open     float64
	High     float64
	Low      float64
	LimitUp  bool // 是否涨停
}

// SectorFundFlow 板块资金流向
type SectorFundFlow struct {
	SectorCode string  // 板块代码
	SectorName string  // 板块名称
	MainFlow   float64 // 主力净流入(元), 正=流入, 负=流出
	MainPct    float64 // 主力净占比(%)
}

// LimitUpInfo 涨停股信息
type LimitUpInfo struct {
	Code       string
	Name       string
	BoardCount int // 连板天数(0=未涨停,1=首板,2=二连板,...)
}

// GetQuoteStocks 批量获取实时行情(含涨停判断)
func (p *Provider) GetQuoteStocks(codes []string) ([]QuoteSnapshot, error) {
	client, err := p.getClient()
	if err != nil {
		return nil, err
	}

	// 批量取行情(每次最多80只,拆包)
	batchSize := 80
	snapshots := make([]QuoteSnapshot, 0, len(codes))

	for i := 0; i < len(codes); i += batchSize {
		end := i + batchSize
		if end > len(codes) {
			end = len(codes)
		}
		batch := codes[i:end]

		quotes, err := client.GetQuote(batch...)
		if err != nil {
			return nil, fmt.Errorf("获取行情失败: %w", err)
		}

		for _, q := range quotes {
			last := q.K.Last.Float64()
			closeP := q.K.Close.Float64()
			snapshots = append(snapshots, QuoteSnapshot{
				Code:     q.Code,
				Price:    closeP,
				PreClose: last,
				Open:     q.K.Open.Float64(),
				High:     q.K.High.Float64(),
				Low:      q.K.Low.Float64(),
				LimitUp:  last > 0 && isLimitUp(closeP, last, q.Code),
			})
		}
	}

	return snapshots, nil
}

// isLimitUp 判断是否涨停
// 主板(60/00开头,除30/688): 10%, 创业板(30): 20%, 科创板(688): 20%, 北交所(8/43/92): 30%
func isLimitUp(price, preClose float64, code string) bool {
	if preClose <= 0 {
		return false
	}
	rate := (price - preClose) / preClose * 100
	threshold := 9.5 // 默认10%档(留0.5%余量)

	bare := code
	if len(code) >= 8 {
		bare = code[2:]
	}
	if len(bare) >= 3 && bare[:3] == "688" {
		threshold = 19.5
	} else if len(bare) >= 2 && bare[:2] == "30" {
		threshold = 19.5
	} else if len(bare) >= 1 && (bare[:1] == "8" || bare[:1] == "4") {
		threshold = 29.5
	}

	return rate >= threshold
}

// limitUpThreshold 获取涨停幅度阈值(用于连板判断)
func limitUpThreshold(code string) float64 {
	bare := code
	if len(code) >= 8 {
		bare = code[2:]
	}
	switch {
	case len(bare) >= 3 && bare[:3] == "688":
		return 19.5
	case len(bare) >= 2 && bare[:2] == "30":
		return 19.5
	case len(bare) >= 1 && (bare[:1] == "8" || bare[:1] == "4"):
		return 29.5
	default:
		return 9.5
	}
}

// ------------------------------------------------------------
// 板块资金流向(东方财富 push2.eastmoney.com)
// ------------------------------------------------------------

type eastMoneyFlowResp struct {
	Data struct {
		Total int                 `json:"total"`
		Diff  []eastMoneyFlowItem `json:"diff"`
	} `json:"data"`
}

type eastMoneyFlowItem struct {
	Code string  `json:"f12"`  // 板块代码
	Name string  `json:"f14"`  // 板块名称
	Flow float64 `json:"f62"`  // 主力净流入(元)
	Pct  float64 `json:"f184"` // 主力净占比(%)
}

// GetSectorFundFlow 获取行业板块资金流向(东方财富公开接口)
// 返回 map[板块代码(6位)]SectorFundFlow
func (p *Provider) GetSectorFundFlow() (map[string]SectorFundFlow, error) {
	url := "https://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=500&po=1&np=1&fltt=2&invt=2&fid=f62&fs=m:90+t:2&fields=f12,f14,f62,f184&ut=b2884a393a59ad64002292a3e90d46a5"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "http://quote.eastmoney.com/")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求东方财富板块资金流失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result eastMoneyFlowResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析板块资金流失败: %w", err)
	}

	flowMap := make(map[string]SectorFundFlow, len(result.Data.Diff))
	for _, item := range result.Data.Diff {
		code := item.Code
		if len(code) > 6 {
			code = code[len(code)-6:]
		}
		flowMap[code] = SectorFundFlow{
			SectorCode: code,
			SectorName: item.Name,
			MainFlow:   item.Flow,
			MainPct:    item.Pct,
		}
	}

	return flowMap, nil
}

// ------------------------------------------------------------
// 涨停梯队检测(使用板块成分股+实时行情+日K线)
// ------------------------------------------------------------

// GetSectorLimitUpStocks 获取某板块的涨停股列表(含连板天数)
func (p *Provider) GetSectorLimitUpStocks(sectorCode string) ([]LimitUpInfo, error) {
	// 1. 获取板块成分股
	stocks, err := p.GetBlockStocks(sectorCode)
	if err != nil {
		return nil, err
	}
	if len(stocks) == 0 {
		return nil, nil
	}

	// 2. 批量获取实时行情,筛选涨停股
	quotes, err := p.GetQuoteStocks(stocks)
	if err != nil {
		return nil, err
	}

	var limitCodes []string
	for _, q := range quotes {
		if q.LimitUp {
			limitCodes = append(limitCodes, q.Code)
		}
	}
	if len(limitCodes) == 0 {
		return nil, nil
	}

	// 3. 遍历涨停股,从日K线判断连板天数
	results := make([]LimitUpInfo, 0, len(limitCodes))
	for _, code := range limitCodes {
		klines, err := p.GetKlines(code, 10)
		if err != nil || len(klines) < 2 {
			results = append(results, LimitUpInfo{Code: code, BoardCount: 1})
			continue
		}

		boardCount := countConsecutiveLimitUp(klines, code)
		results = append(results, LimitUpInfo{
			Code:       code,
			BoardCount: boardCount,
		})
	}

	return results, nil
}

// countConsecutiveLimitUp 从日K线计算连续涨停天数
func countConsecutiveLimitUp(klines []Kline, code string) int {
	if len(klines) < 2 {
		return 0
	}

	threshold := limitUpThreshold(code)
	count := 0

	for i := len(klines) - 1; i >= 1; i-- {
		cur := klines[i]
		prev := klines[i-1]
		if prev.Close <= 0 {
			break
		}
		rate := (cur.Close - prev.Close) / prev.Close * 100
		if rate >= threshold {
			count++
		} else {
			break
		}
	}

	return count
}

func convertToKlines(records []reader.DayRecord, code string) []Kline {
	klines := make([]Kline, len(records))
	for i, r := range records {
		klines[i] = Kline{
			Code:   code,
			Date:   r.Date,
			Open:   r.Open,
			High:   r.High,
			Low:    r.Low,
			Close:  r.Close,
			Volume: float64(r.Volume),
			Amount: r.Amount,
		}
	}
	return klines
}

func isIndex(code string) bool {
	c := code
	if len(c) >= 8 {
		c = c[2:]
	}
	if len(c) >= 2 && c[:2] == "00" && len(c) == 6 {
		return c != "000001" // 平安银行是股票, 上证指数是sh000001/000001
	}
	return false
}
