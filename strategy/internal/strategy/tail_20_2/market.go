package tail_20_2

import (
	"fmt"
	"sync"
	"time"

	"stock-strategy/internal/provider"
)

// ============================================================
// 市场数据加载
// ============================================================

// LoadAllStockData 并发预加载所有股票的日K线到内存
func LoadAllStockData(prov *provider.Provider, codes []string) map[string]*StockDataCache {
	type result struct {
		code  string
		cache *StockDataCache
	}

	jobs := make(chan string, len(codes))
	results := make(chan result, len(codes))
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range jobs {
				data, err := prov.GetStockData(code, 100)
				if err != nil || data == nil || len(data.Klines) == 0 {
					continue
				}
				klines := make([]Kline, len(data.Klines))
				for i, k := range data.Klines {
					klines[i] = Kline{
						Date:   k.Date,
						Open:   k.Open,
						High:   k.High,
						Low:    k.Low,
						Close:  k.Close,
						Volume: k.Volume,
					}
				}
				results <- result{
					code: code,
					cache: &StockDataCache{
						Name:            data.Name,
						Klines:          klines,
						CirculateShares: data.CirculateShares,
					},
				}
			}
		}()
	}

	for _, code := range codes {
		jobs <- code
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	cache := make(map[string]*StockDataCache)
	count := 0
	for r := range results {
		cache[r.code] = r.cache
		count++
		if count%1000 == 0 {
			fmt.Printf("  已加载 %d/%d 只\n", count, len(codes))
		}
	}
	fmt.Printf("  数据加载完成: %d 只股票\n", count)
	return cache
}

// LoadIndex60MinKlines 加载上证指数60分钟K线
func LoadIndex60MinKlines(prov *provider.Provider) ([]provider.Kline, error) {
	klines, err := prov.GetIndex60MinKlines()
	if err != nil {
		return nil, fmt.Errorf("获取指数60分钟K线失败: %w", err)
	}
	fmt.Printf("  共 %d 根60分钟K线\n", len(klines))
	return klines, nil
}

// GetAllStockCodes 获取全市场股票代码并过滤板块
// 返回过滤后的股票代码列表
func GetAllStockCodes(prov *provider.Provider) ([]string, error) {
	all, err := prov.GetAllStocks()
	if err != nil {
		return nil, fmt.Errorf("获取股票列表失败: %w", err)
	}
	filtered := FilterByBoard(all)
	fmt.Printf("板块过滤: %d → %d 只\n", len(all), len(filtered))
	return filtered, nil
}

// FilterByBoard 板块过滤：只保留沪深主板(60/00)、创业板(30)、科创板(688)
func FilterByBoard(codes []string) []string {
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

// IsChiNextStock 判断是否为创业板(30)或科创板(688)
func IsChiNextStock(code string) bool {
	bare := code
	if len(code) >= 8 {
		bare = code[2:]
	}
	if len(bare) >= 6 {
		return bare[:2] == "30" || bare[:3] == "688"
	}
	return false
}

// IsSameDay 判断两个 time.Time 是否为同一日
func IsSameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
