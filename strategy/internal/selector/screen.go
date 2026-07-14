package selector

import "stock-strategy/internal/provider"

// ScreenStock 对单只股票执行完整选股流程
// data: K线数据 + 流通股本(由外部提供,便于测试)
// 返回选股结果和运算上下文,nil表示未通过
// 测试时可构造 provider.StockData 传入:
//
//	data := &provider.StockData{
//	    Code: "600000",
//	    Name: "测试股票",
//	    Klines: []provider.Kline{...},
//	    CirculateShares: 1e9,
//	}
//	result, ctx := selector.ScreenStock(data)
func ScreenStock(data *provider.StockData) (*StockResult, *Context) {
	if data == nil || len(data.Klines) == 0 {
		return nil, nil
	}

	ctx := NewContext(data)
	if ctx == nil {
		return nil, nil
	}

	result := CheckStock(ctx)
	if result != nil {
		result.Name = data.Name
	}
	return result, ctx
}
