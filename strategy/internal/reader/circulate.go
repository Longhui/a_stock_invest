package reader

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// CirculateInfo 流通股本信息
type CirculateInfo struct {
	Code            string  // 股票代码
	Name            string  // 股票名称
	TotalShares     float64 // 总股本(股)
	CirculateShares float64 // 流通股本(股)
	TurnoverRate    float64 // 换手率(%)
	UpdateTime      time.Time
}

// FetchCirculateShares 从腾讯股票API获取流通股本
// 腾讯API: https://qt.gtimg.cn/q={market}{code}
// 返回格式: v_{market}{code}="f1~f2~...~fN~"
// 字段73(0-indexed 72): 流通股本(股), 字段74(0-indexed 73): 总股本(股)
func FetchCirculateShares(code string) (*CirculateInfo, error) {
	marketCode := getTencentMarketCode(code)
	if marketCode == "" {
		return nil, fmt.Errorf("不支持股票代码: %s", code)
	}

	url := fmt.Sprintf("https://qt.gtimg.cn/q=%s", marketCode)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求腾讯API失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 腾讯API返回GBK编码，转换为UTF-8
	utf8Body, err := io.ReadAll(transform.NewReader(
		io.NopCloser(strings.NewReader(string(body))),
		simplifiedchinese.GBK.NewDecoder(),
	))
	if err != nil {
		utf8Body = body
	}

	// 解析响应: v_{market}{code}="f1~f2~...~fN~"
	text := string(utf8Body)
	start := strings.IndexByte(text, '"')
	if start < 0 {
		return nil, fmt.Errorf("响应格式异常: %s", string(utf8Body))
	}
	end := strings.LastIndexByte(text, '"')
	if end <= start {
		return nil, fmt.Errorf("响应格式异常(引号): %s", string(utf8Body))
	}
	fields := strings.Split(text[start+1:end], "~")
	if len(fields) < 74 {
		return nil, fmt.Errorf("字段不足(共%d个): %s", len(fields), code)
	}

	// 字段说明(0-indexed):
	//   1: 股票名称
	//   2: 股票代码
	//  72: 流通股本(股)
	//  73: 总股本(股)
	circulateStr := strings.TrimSpace(fields[72])
	totalStr := strings.TrimSpace(fields[73])
	name := fields[1]

	circulate := parseFloat(circulateStr)
	total := parseFloat(totalStr)

	if circulate <= 0 {
		return nil, fmt.Errorf("未获取到流通股本数据(code=%s)", code)
	}

	return &CirculateInfo{
		Code:            code,
		Name:            name,
		TotalShares:     total,
		CirculateShares: circulate,
		UpdateTime:      time.Now(),
	}, nil
}

// getTencentMarketCode 转换股票代码为腾讯API格式
// 6位纯代码 → 加市场前缀; 8位(带前缀) → 保留前两位
func getTencentMarketCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) == 8 {
		// 带前缀: sh600036, sz000001, bj...
		prefix := strings.ToLower(code[:2])
		realCode := code[2:]
		switch prefix {
		case "sh", "sz", "bj":
			return prefix + realCode
		default:
			return ""
		}
	}
	if len(code) == 6 {
		switch {
		case code[:1] == "6":
			return "sh" + code
		case code[:1] == "0" || code[:2] == "30":
			return "sz" + code
		case code[:1] == "8" || code[:2] == "92" || code[:2] == "43":
			return "bj" + code
		default:
			return ""
		}
	}
	return ""
}

// parseFloat 解析字符串为float64，忽略错误
func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return val
}
